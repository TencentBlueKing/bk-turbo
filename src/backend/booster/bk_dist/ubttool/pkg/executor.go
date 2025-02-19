/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package pkg

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcTypes "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	v1 "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/api/v1"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// Executor define dist executor
type Executor struct {
	taskID string

	bt    dcTypes.BoosterType
	work  dcSDK.ControllerWorkSDK
	stats *dcSDK.ControllerJobStats

	outputmsg []byte
	errormsg  []byte

	counter count32

	// whether use web socket
	usewebsocket bool
}

// NewExecutor return new Executor
func NewExecutor() *Executor {
	// for debug by tomtian
	c := dcSDK.GetControllerConfigFromEnv()
	blog.Debugf("ubtexecutor: got config [%+v],env:%+v", c, os.Environ())

	return &Executor{
		bt:     dcTypes.GetBoosterType(env.GetEnv(env.BoosterType)),
		work:   v1.NewSDK(dcSDK.GetControllerConfigFromEnv()).GetWork(env.GetEnv(env.KeyExecutorControllerWorkID)),
		taskID: env.GetEnv(env.KeyExecutorTaskID),
		stats:  &dcSDK.ControllerJobStats{},
	}
}

// Valid
func (d *Executor) Valid() bool {
	return d.work.ID() != ""
}

// Update update with env
func (d *Executor) Update() {
	d.work = v1.NewSDK(dcSDK.GetControllerConfigFromEnv()).GetWork(env.GetEnv(env.KeyExecutorControllerWorkID))
	d.taskID = env.GetEnv(env.KeyExecutorTaskID)
}

// Run main function entry
func (d *Executor) Run(fullargs []string, workdir string) (int, string, error) {
	blog.Infof("ubtexecutor: command [%s] begins", strings.Join(fullargs, " "))
	for i, v := range fullargs {
		blog.Debugf("ubtexecutor: arg[%d] : [%s]", i, v)
	}
	defer blog.Infof("ubtexecutor: command [%s] finished", strings.Join(fullargs, " "))

	// work available, run work with executor-progress
	return d.runWork(fullargs, workdir)
}

func (d *Executor) runWork(fullargs []string, workdir string) (int, string, error) {
	// d.initStats()

	// ignore argv[0], it's itself
	var retcode int
	var retmsg string
	var r *dcSDK.LocalTaskResult
	var err error
	if d.usewebsocket {
		retcode, retmsg, r, err = d.work.Job(d.getStats(fullargs)).ExecuteLocalTaskWithWebSocket(fullargs, workdir)
	} else {
		retcode, retmsg, r, err = d.work.Job(d.getStats(fullargs)).ExecuteLocalTask(fullargs, workdir)
	}

	if err != nil || retcode != 0 {
		if r != nil {
			blog.Errorf("ubtexecutor: execute failed, error: %v, ret code: %d,retmsg:%s,outmsg:%s,errmsg:%s,cmd:%v",
				err, retcode, retmsg, string(r.Stdout), string(r.Stderr), fullargs)
		} else {
			blog.Errorf("ubtexecutor: execute failed, ret code:%d retmsg:%s error: %v,cmd:%v", retcode, retmsg, err, fullargs)
		}
		return retcode, retmsg, err
	}

	// https://docs.microsoft.com/en-us/windows/win32/intl/code-page-identifiers
	// 65001 means utf8, we will try convert gbk to utf8 which is not valid utf8
	charcode := 0
	if runtime.GOOS == "windows" {
		charcode = dcSyscall.GetConsoleCP()
	}

	// 如果控制台字符集不是utf8，则我们默认为gbk（936），这种情况下，控制台应该可以正确显示中文字符集，无需处理
	// 如果控制台字符集是utf8（65001），则检查字节流中是否都是合法的utf8字符，如果不是，当做gbk处理，尝试将gbk字符串转为utf8

	blog.Infof("ubtexecutor: got result with exit code: %d, outmsg:%s,errmsg:%s,cmd:%v,charcode:%d",
		r.ExitCode, string(r.Stdout), string(r.Stderr), fullargs, charcode)

	if len(r.Stdout) > 1 {
		d.outputmsg = r.Stdout
		hasNewline := bytes.HasSuffix(r.Stdout, []byte("\n"))

		output := r.Stdout
		if charcode == 65001 {
			isUTF8 := utf8.Valid(r.Stdout)
			blog.Infof("ubtexecutor: string [%s] is utf8? %v,charcode:%d", string(r.Stdout), isUTF8, charcode)
			if !isUTF8 {
				// 尝试转为utf8
				utf8bytes, err1 := simplifiedchinese.GBK.NewDecoder().Bytes(r.Stdout)
				if err1 != nil {
					blog.Infof("ubtexecutor: convert gbk string [%s] to utf8 failed with error:%v", string((r.Stdout)), err1)
				} else {
					output = utf8bytes
				}
			}
		} else if charcode == 936 {
			isUTF8 := utf8.Valid(r.Stdout)
			blog.Infof("ubtexecutor: r.Stdout is utf8?%v", isUTF8)

			// 如果是utf8编码，将其从utf8转换为 GBK
			if isUTF8 {
				reader := transform.NewReader(strings.NewReader(string(r.Stdout)), simplifiedchinese.GBK.NewEncoder())
				gbkBytes, err := io.ReadAll(reader)
				if err != nil {
					blog.Infof("ubtexecutor: transform chardet with error:%v", err)
				} else {
					output = gbkBytes
				}
			}
		}

		if hasNewline {
			_, _ = fmt.Fprintf(os.Stdout, "%s", string(output))
		} else {
			_, _ = fmt.Fprintf(os.Stdout, "%s\n", string(output))
		}
	}

	if len(r.Stderr) > 0 {
		d.errormsg = r.Stderr
		hasNewline := bytes.HasSuffix(r.Stderr, []byte("\n"))

		output := r.Stderr
		if charcode == 65001 {
			isUTF8 := utf8.Valid(r.Stderr)
			blog.Infof("ubtexecutor: string [%s] is utf8? %v,charcode:%d", string(r.Stderr), isUTF8, charcode)
			if !isUTF8 {
				// 尝试转为utf8
				utf8bytes, err1 := simplifiedchinese.GBK.NewDecoder().Bytes(r.Stderr)
				if err1 != nil {
					blog.Infof("ubtexecutor: convert gbk string [%s] to utf8 failed with error:%v", string((r.Stderr)), err1)
				} else {
					output = utf8bytes
				}
			}
		} else if charcode == 936 {
			isUTF8 := utf8.Valid(r.Stderr)
			// 如果是utf8编码，将其从utf8转换为 GBK
			if isUTF8 {
				reader := transform.NewReader(strings.NewReader(string(r.Stderr)), simplifiedchinese.GBK.NewEncoder())
				gbkBytes, err := io.ReadAll(reader)
				if err != nil {
					blog.Infof("ubtexecutor: transform chardet with error:%v", err)
				} else {
					output = gbkBytes
				}
			}
		}

		if hasNewline {
			_, _ = fmt.Fprintf(os.Stderr, "%s", string(output))
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "%s\n", string(output))
		}
	}

	if r.ExitCode != 0 {
		blog.Errorf("ubtexecutor: execute failed, error: %v, exit code: %d, outmsg:%s,errmsg:%s,cmd:%v",
			err, r.ExitCode, string(r.Stdout), string(r.Stderr), fullargs)
		return r.ExitCode, retmsg, err
	}

	return 0, retmsg, nil
}

func (d *Executor) getStats(fullargs []string) *dcSDK.ControllerJobStats {
	stats := dcSDK.ControllerJobStats{
		Pid:         os.Getpid(),
		ID:          fmt.Sprintf("%d_%d", d.counter.inc(), time.Now().UnixNano()),
		WorkID:      d.work.ID(),
		TaskID:      d.taskID,
		BoosterType: d.bt.String(),
		OriginArgs:  fullargs,
	}

	return &stats
}
