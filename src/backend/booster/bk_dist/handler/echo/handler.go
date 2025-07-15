/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package echo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	dcConfig "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/config"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
)

const (
	hookConfig = ".\\bk_counter_rules.json"
)

// NewEcho return echo handler
func NewEcho() (handler.Handler, error) {
	return &Echo{
		sandbox: &dcSyscall.Sandbox{},
	}, nil
}

// Echo describe a handler which conside the echo parameters as input/output files,
// these files will be sent to worker and return by worker.
type Echo struct {
	sandbox    *dcSyscall.Sandbox
	inputFiles []string
	count      bool
}

// InitSandbox to init sandbox
func (c *Echo) InitSandbox(sandbox *dcSyscall.Sandbox) {
	c.sandbox = sandbox
}

// InitExtra receive the extra data after resources applied.
func (c *Echo) InitExtra(extra []byte) {
}

// ResultExtra return the extra data for recording in project info.
func (c *Echo) ResultExtra() []byte {
	return nil
}

// RenderArgs receive the origin command from user, and decide whether it should be rendered before execution.
func (c *Echo) RenderArgs(config dcType.BoosterConfig, originArgs string) string {

	// nothing need to be rendered.
	return originArgs
}

// PreWork provide a chance to do some process before all processes begin.
func (c *Echo) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork provide a chance to do some process after all processes finish.
func (c *Echo) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig open the preload config file and return the settings.
func (c *Echo) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	configfile := hookConfig

	if config.Works.HookConfigPath != "" {
		configfile = config.Works.HookConfigPath
	}

	f, err := os.Open(dcConfig.GetFile(configfile))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	var pConfig dcSDK.PreloadConfig
	if err = codec.DecJSONReader(f, &pConfig); err != nil {
		return nil, err
	}

	return &pConfig, nil
}

// GetFilterRules return the sending file filter rules.
func (c *Echo) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {

	// nothing need to be filtered.
	return nil, nil
}

func (c *Echo) CanExecuteWithLocalIdleResource(command []string) bool {
	return true
}

// PreExecuteNeedLock decide whether should lock when executor do the pre-process
func (c *Echo) PreExecuteNeedLock(command []string) bool {
	return false
}

// PostExecuteNeedLock decide whether should lock when executor do the post-process
func (c *Echo) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return true
}

// LocalExecuteNeed decode whether execute local cmd by this handle
func (c *Echo) LocalExecuteNeed(command []string) bool {
	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (c *Echo) LocalLockWeight(command []string) int32 {
	return 1
}

// LocalExecute execute local cmd by this handle
func (c *Echo) LocalExecute(command []string) dcType.BKDistCommonError {
	return dcType.ErrorNone
}

// PreLockWeight decide pre-execute lock weight, default 1
func (c *Echo) PreLockWeight(command []string) int32 {
	return 1
}

// PreExecute do the pre-process in one executor command.
// PreExecute should analyse the input command and generate the DistCommand to send to remote.
func (c *Echo) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	upperCommand := strings.ToUpper(command[0])
	if len(command) < 2 || (!strings.HasSuffix(upperCommand, "ECHO") &&
		!strings.HasSuffix(upperCommand, "ECHO.EXE")) {
		blog.Warnf("echo: invalid command,len(command):[%d], command[0]:[%s]", len(command), command[0])
		return nil, dcType.ErrorUnknown
	}

	inputfiles := make([]dcSDK.FileDesc, 0, 1)
	resultfiles := make([]string, 0, 1)
	for i := 1; i < len(command); i++ {
		newinput := command[i]
		existed, fileSize, modifyTime, fileMode := dcFile.Stat(command[i]).Batch()
		if !existed {
			blog.Infof("echo: input file %s not exist", command[i])

			newinput, _ = filepath.Abs(command[i])
			existed, fileSize, modifyTime, fileMode = dcFile.Stat(newinput).Batch()
			if !existed {
				blog.Infof("echo: input file %s not exist", newinput)

				exepath, _ := exec.LookPath(command[0])
				newinput = filepath.Join(filepath.Dir(exepath), command[i])
				existed, fileSize, modifyTime, fileMode = dcFile.Stat(newinput).Batch()
				if !existed {
					blog.Infof("echo: input file %s not exist", newinput)
					return nil, dcType.ErrorUnknown
				}
			}
		}

		inputfiles = append(inputfiles, dcSDK.FileDesc{
			FilePath:       newinput,
			Compresstype:   protocol.CompressLZ4,
			FileSize:       fileSize,
			Lastmodifytime: modifyTime,
			Md5:            "",
			Filemode:       fileMode,
		})

		resultfiles = append(resultfiles, command[i])
	}

	return &dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir:     "",
				ExePath:     "",
				ExeName:     command[0],
				Params:      command[1:],
				Inputfiles:  inputfiles,
				ResultFiles: resultfiles,
			},
		},
		CustomSave: true,
	}, dcType.ErrorNone
}

// NeedRemoteResource check whether this command need remote resource
func (c *Echo) NeedRemoteResource(command []string) bool {
	return true
}

// RemoteRetryTimes will return the remote retry times
func (c *Echo) RemoteRetryTimes() int {
	return 0
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (c *Echo) NeedRetryOnRemoteFail(command []string) bool {
	return false
}

// OnRemoteFail give chance to try other way if failed to remote execute
func (c *Echo) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (c *Echo) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	return 1
}

// PostExecute do the post-process in one executor command.
// PostExecute should check the DistResult and judge whether the remote processing succeeded.
// Also PostExecute should manages the output message.
func (c *Echo) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	// do not save result to disk
	if r == nil || len(r.Results) == 0 {
		blog.Warnf("echo: result data is invalid")
		return dcType.ErrorUnknown
	}

	result := r.Results[0]
	if len(result.ResultFiles) == 0 {
		blog.Warnf("echo: not found result file, retcode %d, error message:[%s], output message:[%s]",
			result.RetCode,
			result.ErrorMessage,
			result.OutputMessage)

		return dcType.ErrorUnknown
	}

	for _, v := range result.ResultFiles {
		blog.Infof("echo: received result file[%s],len[%d]", v.FilePath, len(v.Buffer))
	}
	return dcType.ErrorNone
}

// FinalExecute provide a chance to do process before the process exit.
func (c *Echo) FinalExecute([]string) {
	return
}

// SupportResultCache check whether this command support result cache
func (c *Echo) SupportResultCache(command []string) int {
	return 0
}

func (c *Echo) GetResultCacheKey(command []string) string {
	return ""
}
