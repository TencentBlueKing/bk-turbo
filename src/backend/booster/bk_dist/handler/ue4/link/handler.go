/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package link

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcEnv "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

const (
	// do not distribute if over this
	MaxInputFileSize = 1024 * 1024 * 50

	appendEnvKey = "LIB="
)

var (
	// ForceLocalFileKeys force some module to compile locally
	// mscoree.lib missed for SwarmInterface
	ForceLocalFileKeys = []string{"UE4Editor-SwarmInterface"}
)

// TaskLink 定义了link.exe链接的描述处理对象, 一般用来处理ue4-win下的link链接
type TaskLink struct {
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

// NewTaskLink get a new link handler
func NewTaskLink() *TaskLink {
	return &TaskLink{
		sandbox: &dcSyscall.Sandbox{},
	}
}

// InitSandbox set sandbox to task-link
func (l *TaskLink) InitSandbox(sandbox *dcSyscall.Sandbox) {
	l.sandbox = sandbox
}

// InitExtra no need
func (l *TaskLink) InitExtra(extra []byte) {
}

// ResultExtra no need
func (l *TaskLink) ResultExtra() []byte {
	return nil
}

// RenderArgs no need change
func (l *TaskLink) RenderArgs(config dcType.BoosterConfig, originArgs string) string {
	return originArgs
}

// PreWork no need
func (l *TaskLink) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork no need
func (l *TaskLink) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig no preload config need
func (l *TaskLink) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	return nil, nil
}

func (l *TaskLink) CanExecuteWithLocalIdleResource(command []string) bool {
	if l.sandbox.Env.GetEnv(env.KeyExecutorUELinkNotUseLocal) == "true" {
		return false
	}

	return true
}

// PreExecuteNeedLock 没有在本地执行的预处理步骤, 无需pre-lock
func (l *TaskLink) PreExecuteNeedLock(command []string) bool {
	return false
}

// PostExecuteNeedLock 防止回传的文件读写跑满本机磁盘
func (l *TaskLink) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return true
}

// PreLockWeight decide pre-execute lock weight, default 1
func (l *TaskLink) PreLockWeight(command []string) int32 {
	return 1
}

// PreExecute 预处理
func (l *TaskLink) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return l.preExecute(command)
}

// NeedRemoteResource check whether this command need remote resource
func (l *TaskLink) NeedRemoteResource(command []string) bool {
	return true
}

// RemoteRetryTimes will return the remote retry times
func (l *TaskLink) RemoteRetryTimes() int {
	return 1
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (l *TaskLink) NeedRetryOnRemoteFail(command []string) bool {
	return false
}

// OnRemoteFail give chance to try other way if failed to remote execute
func (l *TaskLink) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (l *TaskLink) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	return 1
}

// PostExecute 后置处理
func (l *TaskLink) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	return l.postExecute(r)
}

// LocalExecuteNeed no need
func (l *TaskLink) LocalExecuteNeed(command []string) bool {
	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (l *TaskLink) LocalLockWeight(command []string) int32 {
	envvalue := l.sandbox.Env.GetEnv(env.KeyExecutorUELinkLocalCPUWeight)
	if envvalue != "" {
		w, err := strconv.Atoi(envvalue)
		if err == nil && w > 0 && w <= runtime.NumCPU() {
			return int32(w)
		}
	}

	return 1
}

// LocalExecute no need
func (l *TaskLink) LocalExecute(command []string) dcType.BKDistCommonError {
	return dcType.ErrorNone
}

// FinalExecute no need
func (l *TaskLink) FinalExecute([]string) {
}

// GetFilterRules add file send filter
func (l *TaskLink) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	return nil, nil
}

func (l *TaskLink) workerSupportAbsPath() bool {
	v := l.sandbox.Env.GetEnv(env.KeyWorkerSupportAbsPath)
	if v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return true
}

func (l *TaskLink) preExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("link: start pre execute for: %v", command)

	if !l.workerSupportAbsPath() {
		blog.Infof("link: remote worker do not support absolute path")
		return nil, dcType.ErrorUnknown
	}

	l.originArgs = command
	responseFile, args, err := ensureCompiler(command)
	if err != nil {
		blog.Errorf("link: pre execute ensure compiler failed %v: %v", args, err)
		return nil, dcType.ErrorUnknown
	}

	for _, v := range ForceLocalFileKeys {
		if strings.Contains(responseFile, v) {
			blog.Errorf("link: pre execute found response %s is in force local list, do not deal now", responseFile)
			return nil, dcType.ErrorPreForceLocal
		}
	}

	if responseFile != "" && !filepath.IsAbs(responseFile) {
		responseFile, _ = filepath.Abs(filepath.Join(l.sandbox.Dir, responseFile))
	}

	l.responseFile = responseFile
	l.ensuredArgs = args

	if err = l.scan(args); err != nil {
		blog.Warnf("link: scan command[%v] with error : %v", command, err)
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
			err := fmt.Errorf("input file %s not existed", v)
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

	// ++ search additional exe files
	searchpath := l.sandbox.Env.GetOriginEnv("Path")
	mtpath, err := dcUtil.LookPath("mt", searchpath, "exe")
	if err == nil {
		addDir := filepath.Dir(mtpath)

		tempInputs := make([]string, 0, 5)
		tempInputs = append(tempInputs, mtpath)
		tempInputs = append(tempInputs, filepath.Join(addDir, "midlrtmd.dll"))
		tempInputs = append(tempInputs, filepath.Join(addDir, "rc.exe"))
		tempInputs = append(tempInputs, filepath.Join(addDir, "rcdll.dll"))
		tempInputs = append(tempInputs, filepath.Join(addDir, "ServicingCommon.dll"))
		blog.Debugf("link: found additional files:%v", tempInputs)

		for _, v := range tempInputs {
			existed, fileSize, modifyTime, fileMode := dcFile.Stat(v).Batch()
			if !existed {
				err := fmt.Errorf("input tool file %s not existed", v)
				blog.Infof("%v", err)
				// return nil, err
				continue
			}

			// generate the input files for pre-process file
			inputFiles = append(inputFiles, dcSDK.FileDesc{
				FilePath:           v,
				Compresstype:       protocol.CompressLZ4,
				FileSize:           fileSize,
				Lastmodifytime:     modifyTime,
				Md5:                "",
				Filemode:           fileMode,
				Targetrelativepath: "C:\\Windows\\System32",
				NoDuplicated:       true,
			})
		}
	} else {
		blog.Infof("link: not found mt.exe at path:%s", searchpath)
	}
	// --

	blog.Debugf("link: success done pre execute for: %v", command)

	// // ++ for debug
	// var totalinputsize int64
	// for _, v := range inputFiles {
	// 	totalinputsize += v.FileSize
	// }
	// blog.Infof("link: [%d] input files, total size[%d]", len(inputFiles), totalinputsize)

	// set env which need append to remote
	envs := []string{}
	envlibpath := ""
	for _, v := range l.sandbox.Env.Source() {
		if strings.HasPrefix(v, appendEnvKey) {
			envlibpath = v
			envs = append(envs, v)
			// set flag we hope append env, not overwrite
			flag := fmt.Sprintf("%s=true", dcEnv.GetEnvKey(env.KeyRemoteEnvAppend))
			envs = append(envs, flag)
			break
		}
	}
	blog.Debugf("link: env which ready sent to remote:[%v]", envs)

	if envlibpath != "" {
		fields1 := strings.Split(envlibpath, "=")
		if len(fields1) > 1 {
			fields2 := strings.Split(fields1[1], ";")
			if len(fields2) > 0 {
				files, _ := getAllLibFiles(fields2, []string{".lib", ".Lib"})
				if len(files) > 0 {
					blog.Debugf("link: ready add addittional lib files:%v", files)
					for _, v := range files {
						existed, fileSize, modifyTime, fileMode := dcFile.Stat(v).Batch()
						if !existed {
							err := fmt.Errorf("input lib file %s not existed", v)
							blog.Errorf("%v", err)
							continue
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
				}
			}
		}
	}

	// if totalinputsize > MaxInputFileSize {
	// 	err := fmt.Errorf("input files total size %d over max size %d", totalinputsize, MaxInputFileSize)
	// 	blog.Errorf("link: pre execute ensure compiler failed %v: %v", args, err)
	// 	return nil, err
	// }
	// --

	// to check whether need to compile with response file
	// exeName := l.scannedArgs[0]
	// params := l.scannedArgs[1:]

	exeName := command[0]
	params := command[1:]

	req := dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir:         l.sandbox.Dir,
				ExePath:         "",
				ExeName:         exeName,
				ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
				Params:          params,
				Inputfiles:      inputFiles,
				ResultFiles:     l.outputFile,
				Env:             envs,
			},
		},
		CustomSave: true,
	}

	blog.Debugf("link: after pre,full command[%v]", req)

	return &req, dcType.ErrorNone
}

func (l *TaskLink) postExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	blog.Infof("link: start post execute for: %v", l.originArgs)
	if r == nil || len(r.Results) == 0 {
		blog.Warnf("link: parameter is invalid")
		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: fmt.Errorf("parameter is invalid"),
		}
	}

	if len(r.Results[0].ResultFiles) > 0 {
		for _, f := range r.Results[0].ResultFiles {
			if f.Buffer != nil {
				if err := saveResultFile(&f); err != nil {
					blog.Errorf("link: failed to save file [%s] with error:%v", f.FilePath, err)
					return dcType.BKDistCommonError{
						Code:  dcType.UnknowCode,
						Error: err,
					}
				}
			}
		}
	}

	if r.Results[0].RetCode == 0 {
		blog.Infof("link: success done post execute for: %v", l.originArgs)
		return dcType.ErrorNone
	}

	blog.Warnf("link: failed to remote execute, retcode %d, error message:%s, output message:%s",
		r.Results[0].RetCode,
		r.Results[0].ErrorMessage,
		r.Results[0].OutputMessage)

	return dcType.BKDistCommonError{
		Code:  dcType.UnknowCode,
		Error: fmt.Errorf(string(r.Results[0].ErrorMessage)),
	}
}

func (l *TaskLink) scan(args []string) error {
	blog.Debugf("link: scan begin got args: %v", args)

	var err error

	scannedData, err := scanArgs(args, l.sandbox.Dir)
	if err != nil {
		// blog.Warnf("link: scan args failed %v: %v", args, err)
		return err
	}

	l.scannedArgs = scannedData.args
	l.inputFile = scannedData.inputFile
	l.outputFile = scannedData.outputFile

	blog.Debugf("link: scan success for enter args: %v", args)
	return nil
}

// SupportResultCache check whether this command support result cache
func (l *TaskLink) SupportResultCache(command []string) int {
	return 0
}

func (l *TaskLink) GetResultCacheKey(command []string) string {
	return ""
}
