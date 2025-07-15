/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package shader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

var (
	locallockweight int32 = 0
)

// NewUE4Shader get a new shader handler
func NewUE4Shader() *UE4Shader {
	return &UE4Shader{
		sandbox: &dcSyscall.Sandbox{},
	}
}

// UE4Shader 定义了shader编译的描述处理对象, 一般用来处理ue4下的shader编译
type UE4Shader struct {
	sandbox *dcSyscall.Sandbox

	outputTempFile string
	outputRealFile string
}

// InitSandbox set sandbox to ue4-shader
func (u *UE4Shader) InitSandbox(sandbox *dcSyscall.Sandbox) {
	u.sandbox = sandbox
}

// InitExtra no need
func (u *UE4Shader) InitExtra(extra []byte) {
}

// ResultExtra no need
func (u *UE4Shader) ResultExtra() []byte {
	return nil
}

// RenderArgs no need change
func (u *UE4Shader) RenderArgs(config dcType.BoosterConfig, originArgs string) string {
	return originArgs
}

// PreWork no need
func (u *UE4Shader) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork no need
func (u *UE4Shader) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig no preload config need
func (u *UE4Shader) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	return nil, nil
}

// GetFilterRules no filter rules need
func (u *UE4Shader) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	return nil, nil
}

func (u *UE4Shader) CanExecuteWithLocalIdleResource(command []string) bool {
	// only for debug
	blog.Infof("shader: BK_DIST_UE_SHADER_NOT_USE_LOCAL=[%v]", u.sandbox.Env.GetEnv(env.KeyExecutorUEShaderNotUseLocal))

	if u.sandbox.Env.GetEnv(env.KeyExecutorUEShaderNotUseLocal) == "true" {
		return false
	}

	return true
}

// PreExecuteNeedLock 没有在本地执行的预处理步骤, 无需pre-lock
func (u *UE4Shader) PreExecuteNeedLock(command []string) bool {
	return false
}

// PostExecuteNeedLock 无需post-lock
func (u *UE4Shader) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return false
}

// PreLockWeight decide pre-execute lock weight, default 1
func (u *UE4Shader) PreLockWeight(command []string) int32 {
	return 1
}

func (u *UE4Shader) isNewShader() bool {
	return u.sandbox.Env.GetEnv(env.KeyExecutorNewShader) != ""
}

// PreExecute 预处理
func (u *UE4Shader) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("shader: ready pre execute with command[%v]", command)

	// to support ue 5.3 macos
	if u.isNewShader() {
		return u.PreExecuteNew(command)
	}

	if len(command) < 6 {
		blog.Warnf("shader: invalid command")
		return nil, dcType.ErrorUnknown
	}

	filedir, _ := filepath.Abs(command[1])
	inputFile := ""

	params := []string{"\"\""}
	for _, v := range command[2:] {
		blog.Debugf("shader: handle with argv [%s]", v)
		if strings.HasSuffix(v, ".in") {
			if !filepath.IsAbs(v) {
				inputFile = filepath.Join(filedir, v)
			}
			params = append(params, inputFile)
		} else if strings.HasSuffix(v, ".out") {
			if !filepath.IsAbs(v) {
				u.outputRealFile = filepath.Join(filedir, v)
				u.outputTempFile = filepath.Join(filedir, "bktemp_"+v)
			}
			params = append(params, u.outputTempFile)
		} else {
			params = append(params, v)
		}
	}

	info := dcFile.Stat(inputFile)
	existed, fileSize, modifyTime, fileMode := info.Batch()
	if !existed {
		blog.Warnf("shader: input file %s not exist with error:%v", inputFile, info.Error())
		return nil, dcType.ErrorUnknown
	}

	inputfiles := []dcSDK.FileDesc{{
		FilePath:       inputFile,
		Compresstype:   protocol.CompressLZ4,
		FileSize:       fileSize,
		Lastmodifytime: modifyTime,
		Md5:            "",
		Filemode:       fileMode,
	}}

	exeName := filepath.Base(command[0])
	// add exe file into Inputfiles with size 0
	value := u.sandbox.Env.GetEnv(env.KeyExecutorToolchainPathMap)
	if value != "" {
		toolmap, err := dcSDK.ResolveToolchainEnvValue(value)
		if err == nil && len(toolmap) > 0 {
			if v, ok := toolmap[exeName]; ok {
				blog.Debugf("shader: found exe[%s] relative path[%s]", exeName, v)
				inputfiles = append(inputfiles, dcSDK.FileDesc{
					FilePath:           exeName,
					Compresstype:       protocol.CompressLZ4,
					FileSize:           -1,
					Lastmodifytime:     modifyTime,
					Md5:                "",
					Targetrelativepath: v,
				})
			}
		}
	}

	return &dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir:         "",
				ExePath:         "",
				ExeName:         exeName,
				ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
				Params:          params,
				Inputfiles:      inputfiles,
				ResultFiles:     []string{u.outputTempFile},
			},
		},
		CustomSave: true,
	}, dcType.ErrorNone
}

func (u *UE4Shader) PreExecuteNew(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Debugf("shader: ready pre execute new with command[%v]", command)

	if len(command) < 6 {
		blog.Warnf("shader: invalid command")
		return nil, dcType.ErrorUnknown
	}

	// 注意：command[1]的路径后面可能有/，这个不能去掉，否则shader会编译失败
	filedir := ""
	if !filepath.IsAbs(command[1]) {
		filedir, _ = filepath.Abs(command[1])
	} else {
		filedir = command[1]
	}
	inputFile := ""

	params := []string{filedir}
	for _, v := range command[2:] {
		blog.Debugf("shader: handle with argv [%s]", v)
		if strings.HasSuffix(v, ".in") {
			if !filepath.IsAbs(v) {
				inputFile = filepath.Join(filedir, v)
			}
		} else if strings.HasSuffix(v, ".out") {
			if !filepath.IsAbs(v) {
				u.outputRealFile = filepath.Join(filedir, v)
				u.outputTempFile = filepath.Join(filedir, "bktemp_"+v)
			}
			params = append(params, "bktemp_"+v)
			continue
		}

		params = append(params, v)
	}
	blog.Infof("shader: ready pre execute new with params[%v]", params)

	info := dcFile.Stat(inputFile)
	existed, fileSize, modifyTime, fileMode := info.Batch()
	if !existed {
		blog.Warnf("shader: input file %s not exist with error:%v", inputFile, info.Error())
		return nil, dcType.ErrorUnknown
	}

	inputfiles := []dcSDK.FileDesc{{
		FilePath:           inputFile,
		Compresstype:       protocol.CompressLZ4,
		FileSize:           fileSize,
		Lastmodifytime:     modifyTime,
		Md5:                "",
		Filemode:           fileMode,
		Targetrelativepath: filedir,
	}}

	exeName := filepath.Base(command[0])
	// add exe file into Inputfiles with size 0
	value := u.sandbox.Env.GetEnv(env.KeyExecutorToolchainPathMap)
	if value != "" {
		toolmap, err := dcSDK.ResolveToolchainEnvValue(value)
		if err == nil && len(toolmap) > 0 {
			if v, ok := toolmap[exeName]; ok {
				blog.Debugf("shader: found exe[%s] relative path[%s]", exeName, v)
				inputfiles = append(inputfiles, dcSDK.FileDesc{
					FilePath:           exeName,
					Compresstype:       protocol.CompressLZ4,
					FileSize:           -1,
					Lastmodifytime:     modifyTime,
					Md5:                "",
					Targetrelativepath: v,
				})
			}
		}
	}

	return &dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir:         filedir,
				ExePath:         "",
				ExeName:         exeName,
				ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
				Params:          params,
				Inputfiles:      inputfiles,
				ResultFiles:     []string{u.outputTempFile},
			},
		},
		CustomSave: true,
	}, dcType.ErrorNone
}

// NeedRemoteResource check whether this command need remote resource
func (u *UE4Shader) NeedRemoteResource(command []string) bool {
	return true
}

// RemoteRetryTimes will return the remote retry times
func (u *UE4Shader) RemoteRetryTimes() int {
	return 1
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (u *UE4Shader) NeedRetryOnRemoteFail(command []string) bool {
	return false
}

// OnRemoteFail give chance to try other way if failed to remote execute
func (u *UE4Shader) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (u *UE4Shader) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	return 1
}

// PostExecute 后置处理
func (u *UE4Shader) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	if r == nil || len(r.Results) == 0 {
		blog.Warnf("shader: result data is invalid")
		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: fmt.Errorf("parameter is invalid"),
		}
	}
	result := r.Results[0]

	if result.RetCode != 0 {
		errMsg := result.ErrorMessage
		if len(result.ResultFiles) > 0 {
			msgs := [][]byte{result.ErrorMessage, result.ResultFiles[0].Buffer}
			sep := []byte("||") // 分隔符
			errMsg = bytes.Join(msgs, sep)
		}
		blog.Warnf("shader: failed to remote execute, retcode %d, error message:[%s], output message:[%s]",
			result.RetCode,
			errMsg,
			result.OutputMessage)

		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: fmt.Errorf("%s", errMsg),
		}
	}

	if len(result.ResultFiles) == 0 {
		blog.Warnf("shader: not found result file, retcode %d, error message:[%s], output message:[%s]",
			result.RetCode,
			result.ErrorMessage,
			result.OutputMessage)

		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: fmt.Errorf("%s", result.ErrorMessage),
		}
	}

	err := checkAndsaveResultFile(&result.ResultFiles[0])
	if err != nil {
		blog.Infof("shader: failed to check and save shader result file[%s],error:[%v]",
			result.ResultFiles[0].FilePath, err)

		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: err,
		}
	}

	// move result temp to real
	blog.Infof("shader: ready rename file from [%s] to [%s]", u.outputTempFile, u.outputRealFile)
	_ = os.Rename(u.outputTempFile, u.outputRealFile)

	return dcType.ErrorNone
}

// LocalExecuteNeed no need
func (u *UE4Shader) LocalExecuteNeed(command []string) bool {
	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (u *UE4Shader) LocalLockWeight(command []string) int32 {
	envvalue := u.sandbox.Env.GetEnv(env.KeyExecutorUEShaderLocalCPUWeight)
	if envvalue != "" {
		w, err := strconv.Atoi(envvalue)
		if err == nil && w > 0 && w <= runtime.NumCPU() {
			return int32(w)
		}
	}

	/*	// default setting of ue 4.26
		; Make sure we don't starve loading threads
		NumUnusedShaderCompilingThreads=3
		; Make sure the game has enough cores available to maintain reasonable performance
		NumUnusedShaderCompilingThreadsDuringGame=4
		; Batching multiple jobs to reduce file overhead, but not so many that latency of blocking compiles is hurt
		MaxShaderJobBatchSize=10
	*/
	// we will reserve 4 cpu as the default setting of ue 4.26
	if locallockweight > 0 {
		return locallockweight
	}

	var reserved int32 = 4
	cpunum := int32(runtime.NumCPU())
	maxlockweight := cpunum - reserved
	var weight int32 = 5 // assume shader batch size is 5
	if weight >= maxlockweight {
		weight = maxlockweight - 1 // to ensure 2 jobs running
	}
	if weight < 2 {
		weight = 2 // to ensure 2 jobs running anyway
	}
	locallockweight = weight

	return locallockweight
}

// LocalExecute no need
func (u *UE4Shader) LocalExecute(command []string) dcType.BKDistCommonError {
	return dcType.ErrorNone
}

// FinalExecute no need
func (u *UE4Shader) FinalExecute([]string) {
}

// SupportResultCache check whether this command support result cache
func (u *UE4Shader) SupportResultCache(command []string) int {
	return 0
}

func (u *UE4Shader) GetResultCacheKey(command []string) string {
	return ""
}
