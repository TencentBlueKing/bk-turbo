/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package astc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// NewTextureCompressor get a new tc handler
func NewTextureCompressor() handler.Handler {
	return &TextureCompressor{
		sandbox: &dcSyscall.Sandbox{},
	}
}

type tcType string

const (
	unknown tcType = "unknown"
	astcArm tcType = "astc-arm"
)

// return inputfile,tempoutput,finaloutput,params,error
func (t tcType) scanParam(param []string) (string, string, string, []string, error) {
	switch t {
	case astcArm:
		// astc at least has 4 options: -cs input output 6x6
		if len(param) < 4 {
			return "", "", "", nil, fmt.Errorf("invalid astc command, too few params")
		}

		abspathinput, _ := filepath.Abs(param[1])
		abspathoutput, _ := filepath.Abs(param[2])
		base := filepath.Base(abspathoutput)
		dir := filepath.Dir(abspathoutput)
		outputTempFile := filepath.Join(dir, "bktemp_"+base)

		newparam := make([]string, len(param))
		copy(newparam, param)
		newparam[2] = outputTempFile
		return abspathinput, outputTempFile, abspathoutput, newparam, nil

	default:
		return "", "", "", nil, fmt.Errorf("invalid command, unsupported type %s for seeking output file", t)
	}
}

func getTCType(command string) (tcType, error) {
	switch filepath.Base(command) {
	case "astcenc", "astcenc.exe", "astcenc-sse2.exe":
		return astcArm, nil
	default:
		return unknown, fmt.Errorf("unknown texture compressor type")
	}
}

// TextureCompressor describe the handler to handle texture compress in unity3d
type TextureCompressor struct {
	sandbox *dcSyscall.Sandbox

	outputTempFile string
	outputRealFile string
}

// InitSandbox init sandbox
func (tc *TextureCompressor) InitSandbox(sandbox *dcSyscall.Sandbox) {
	tc.sandbox = sandbox
}

// InitExtra no need
func (tc *TextureCompressor) InitExtra(extra []byte) {

}

// ResultExtra no need
func (tc *TextureCompressor) ResultExtra() []byte {
	return nil
}

// RenderArgs no need change
func (tc *TextureCompressor) RenderArgs(config dcType.BoosterConfig, originArgs string) string {
	return originArgs
}

// PreWork no need
func (tc *TextureCompressor) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork no need
func (tc *TextureCompressor) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig no need
func (tc *TextureCompressor) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	return nil, nil
}

// GetFilterRules no need
func (tc *TextureCompressor) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	return nil, nil
}

func (cc *TextureCompressor) CanExecuteWithLocalIdleResource(command []string) bool {
	return true
}

// PreExecuteNeedLock no need
func (tc *TextureCompressor) PreExecuteNeedLock(command []string) bool {
	return false
}

// PostExecuteNeedLock no need
func (tc *TextureCompressor) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return false
}

// PreLockWeight decide pre-execute lock weight, default 1
func (tc *TextureCompressor) PreLockWeight(command []string) int32 {
	return 1
}

// PreExecute parse the input and output file, and then just run the origin command in remote
func (tc *TextureCompressor) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	if len(command) == 0 {
		blog.Warnf("astc: invalid command")
		return nil, dcType.ErrorUnknown
	}

	blog.Infof("astc: pre execute for: %v", command)

	t, err := getTCType(command[0])
	if err != nil {
		blog.Warnf("astc: get tc type with error:%v", err)
		return nil, dcType.ErrorUnknown
	}

	var inputFile string
	var newparam []string
	inputFile, tc.outputTempFile, tc.outputRealFile, newparam, err = t.scanParam(command[1:])
	if err != nil {
		blog.Warnf("astc: scan param with error:%v", err)
		return nil, dcType.ErrorUnknown
	}
	blog.Infof("astc: got new param [%s]", strings.Join(newparam, " "))

	existed, fileSize, modifyTime, fileMode := dcFile.Stat(inputFile).Batch()
	if !existed {
		blog.Warnf("astc: input file %s not exist", inputFile)
		return nil, dcType.ErrorUnknown
	}

	return &dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir: "",
				ExePath: "",
				// ExeName: filepath.Base(command[0]),
				ExeName:         command[0],
				ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
				Params:          newparam,
				Inputfiles: []dcSDK.FileDesc{{
					FilePath:           inputFile,
					Compresstype:       protocol.CompressLZ4,
					FileSize:           fileSize,
					Lastmodifytime:     modifyTime,
					Md5:                "",
					Filemode:           fileMode,
					Targetrelativepath: filepath.Dir(inputFile),
				}},
				ResultFiles: []string{tc.outputTempFile},
			},
		},
	}, dcType.ErrorNone
}

// RemoteRetryTimes will return the remote retry times
func (tc *TextureCompressor) RemoteRetryTimes() int {
	return 0
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (tc *TextureCompressor) NeedRetryOnRemoteFail(command []string) bool {
	return false
}

// OnRemoteFail give chance to try other way if failed to remote execute
func (tc *TextureCompressor) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (tc *TextureCompressor) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	return 1
}

// PostExecute judge the result
func (tc *TextureCompressor) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	if r == nil || len(r.Results) == 0 {
		blog.Warnf("astc: parameter is invalid")
		return dcType.ErrorUnknown
	}
	result := r.Results[0]

	if result.RetCode != 0 {
		blog.Warnf("astc: failed to execute on remote: %s", string(result.ErrorMessage))
		return dcType.ErrorUnknown
	}

	if len(result.ResultFiles) == 0 {
		blog.Warnf("astc: not found result file, retcode %d, error message:[%s], output message:[%s]",
			result.RetCode,
			result.ErrorMessage,
			result.OutputMessage)
		return dcType.ErrorUnknown
	}

	// move result temp to real
	blog.Infof("astc: ready rename file from [%s] to [%s]", tc.outputTempFile, tc.outputRealFile)
	_ = os.Rename(tc.outputTempFile, tc.outputRealFile)

	return dcType.ErrorNone
}

// LocalExecuteNeed no need
func (tc *TextureCompressor) LocalExecuteNeed(command []string) bool {
	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (tc *TextureCompressor) LocalLockWeight(command []string) int32 {
	return 1
}

// NeedRemoteResource check whether this command need remote resource
func (tc *TextureCompressor) NeedRemoteResource(command []string) bool {
	return true
}

// LocalExecute no need
func (tc *TextureCompressor) LocalExecute(command []string) dcType.BKDistCommonError {
	return dcType.ErrorNone
}

// FinalExecute no need
func (tc *TextureCompressor) FinalExecute([]string) {
}
