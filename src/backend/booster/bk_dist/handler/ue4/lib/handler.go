/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package lib

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// TaskLib 定义了lib.exe链接的描述处理对象, 一般用来处理ue4-win下的lib链接
type TaskLib struct {
	sandbox *dcSyscall.Sandbox

	// different stages args
	originArgs  []string
	ensuredArgs []string
	scannedArgs []string

	// file names
	inputFile    []string
	outputFile   []string
	responseFile string
}

// NewTaskLib get a new lib handler
func NewTaskLib() *TaskLib {
	return &TaskLib{
		sandbox: &dcSyscall.Sandbox{},
	}
}

// InitSandbox set sandbox to task-lib
func (l *TaskLib) InitSandbox(sandbox *dcSyscall.Sandbox) {
	l.sandbox = sandbox
}

// InitExtra no need
func (l *TaskLib) InitExtra(extra []byte) {
}

// ResultExtra no need
func (l *TaskLib) ResultExtra() []byte {
	return nil
}

// RenderArgs no need change
func (l *TaskLib) RenderArgs(config dcType.BoosterConfig, originArgs string) string {
	return originArgs
}

// PreWork no need
func (l *TaskLib) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork no need
func (l *TaskLib) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig no preload config need
func (l *TaskLib) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	return nil, nil
}

func (l *TaskLib) CanExecuteWithLocalIdleResource(command []string) bool {
	if l.sandbox.Env.GetEnv(env.KeyExecutorUELibNotUseLocal) == "true" {
		return false
	}

	return true
}

// PreExecuteNeedLock 没有在本地执行的预处理步骤, 无需pre-lock
func (l *TaskLib) PreExecuteNeedLock(command []string) bool {
	return false
}

// PostExecuteNeedLock 防止回传的文件读写跑满本机磁盘
func (l *TaskLib) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return true
}

// PreLockWeight decide pre-execute lock weight, default 1
func (l *TaskLib) PreLockWeight(command []string) int32 {
	return 1
}

// PreExecute 预处理
func (l *TaskLib) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return l.preExecute(command)
}

// NeedRemoteResource check whether this command need remote resource
func (l *TaskLib) NeedRemoteResource(command []string) bool {
	return true
}

// RemoteRetryTimes will return the remote retry times
func (l *TaskLib) RemoteRetryTimes() int {
	return 1
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (l *TaskLib) NeedRetryOnRemoteFail(command []string) bool {
	return false
}

// OnRemoteFail give chance to try other way if failed to remote execute
func (l *TaskLib) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (l *TaskLib) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	return 1
}

// PostExecute 后置处理
func (l *TaskLib) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	return l.postExecute(r)
}

// LocalExecuteNeed no need
func (l *TaskLib) LocalExecuteNeed(command []string) bool {
	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (l *TaskLib) LocalLockWeight(command []string) int32 {
	envvalue := l.sandbox.Env.GetEnv(env.KeyExecutorUELibLocalCPUWeight)
	if envvalue != "" {
		w, err := strconv.Atoi(envvalue)
		if err == nil && w > 0 && w <= runtime.NumCPU() {
			return int32(w)
		}
	}

	return 1
}

// LocalExecute no need
func (l *TaskLib) LocalExecute(command []string) dcType.BKDistCommonError {
	return dcType.ErrorNone
}

// FinalExecute no need
func (l *TaskLib) FinalExecute([]string) {
}

// GetFilterRules add file send filter
func (l *TaskLib) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	return nil, nil
}

func (l *TaskLib) workerSupportAbsPath() bool {
	v := l.sandbox.Env.GetEnv(env.KeyWorkerSupportAbsPath)
	if v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return true
}

func (l *TaskLib) preExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("lib: start pre execute for: %v", command)

	if !l.workerSupportAbsPath() {
		blog.Infof("lib: remote worker do not support absolute path")
		return nil, dcType.ErrorUnknown
	}

	l.originArgs = command
	responseFile, args, err := ensureCompiler(command)
	if err != nil {
		blog.Errorf("lib: pre execute ensure compiler failed %v: %v", args, err)
		return nil, dcType.ErrorUnknown
	}

	if responseFile != "" && !filepath.IsAbs(responseFile) {
		responseFile, _ = filepath.Abs(filepath.Join(l.sandbox.Dir, responseFile))
	}

	l.responseFile = responseFile
	l.ensuredArgs = args

	if err = l.scan(args); err != nil {
		blog.Errorf("lib: scan args[%v] failed : %v", args, err)
		return nil, dcType.ErrorUnknown
	}

	// add response file as input
	if responseFile != "" {
		l.inputFile = append(l.inputFile, responseFile)
	}

	inputFiles := make([]dcSDK.FileDesc, 0, 0)
	for _, v := range l.inputFile {
		existed, fileSize, modifyTime, fileMode := dcFile.Stat(v).Batch()
		if !existed {
			err := fmt.Errorf("input pre file %s not existed", v)
			blog.Errorf("%v", err)
			return nil, dcType.ErrorUnknown
		}

		// generate the input files for pre-process file
		inputFiles = append(inputFiles, dcSDK.FileDesc{
			FilePath:           v,
			Compresstype:       protocol.CompressLZ4,
			FileSize:           fileSize,
			Lastmodifytime:     modifyTime,
			Md5:                "",
			Filemode:           fileMode,
			Targetrelativepath: filepath.Dir(v),
			NoDuplicated:       true,
		})
	}

	blog.Infof("lib: success done pre execute for: %v", command)

	// to check whether need to compile with response file
	// exeName := l.scannedArgs[0]
	// params := l.scannedArgs[1:]

	exeName := command[0]
	params := command[1:]

	return &dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir:         l.sandbox.Dir,
				ExePath:         "",
				ExeName:         exeName,
				ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
				Params:          params,
				Inputfiles:      inputFiles,
				ResultFiles:     l.outputFile,
			},
		},
		CustomSave: true,
	}, dcType.ErrorNone
}

func (l *TaskLib) postExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	blog.Infof("lib: start post execute for: %v", l.originArgs)
	if r == nil || len(r.Results) == 0 {
		blog.Warnf("lib: parameter is invalid")
		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: fmt.Errorf("parameter is invalid"),
		}
	}

	if len(r.Results[0].ResultFiles) > 0 {
		for _, f := range r.Results[0].ResultFiles {
			if f.Buffer != nil {
				if err := saveResultFile(&f); err != nil {
					blog.Errorf("lib: failed to save file [%s] with error:%v", f.FilePath, err)
					return dcType.BKDistCommonError{
						Code:  dcType.UnknowCode,
						Error: err,
					}
				}
			}
		}
	}

	if r.Results[0].RetCode == 0 {
		blog.Infof("lib: success done post execute for: %v", l.originArgs)
		return dcType.ErrorNone
	}

	blog.Warnf("lib: failed to remote execute, retcode %d, error message:%s, output message:%s",
		r.Results[0].RetCode,
		r.Results[0].ErrorMessage,
		r.Results[0].OutputMessage)

	return dcType.BKDistCommonError{
		Code:  dcType.UnknowCode,
		Error: fmt.Errorf(string(r.Results[0].ErrorMessage)),
	}
}

func (l *TaskLib) scan(args []string) error {
	blog.Infof("lib: scan begin got args: %v", args)

	var err error

	scannedData, err := scanArgs(args, l.sandbox.Dir)
	if err != nil {
		blog.Errorf("lib: scan args failed %v: %v", args, err)
		return err
	}

	l.scannedArgs = scannedData.args
	l.inputFile = scannedData.inputFile
	l.outputFile = scannedData.outputFile

	blog.Infof("lib: scan success for enter args: %v", args)
	return nil
}

// SupportResultCache check whether this command support result cache
func (l *TaskLib) SupportResultCache(command []string) int {
	return 0
}

func (l *TaskLib) GetResultCacheKey(command []string) string {
	return ""
}
