/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package ubaagent

import (
	"strings"

	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// NewUbaAgent get a new UbaAgent handler
func NewUbaAgent() handler.Handler {
	return &UbaAgent{
		sandbox: &dcSyscall.Sandbox{},
	}
}

// UbaAgent describe the handler of UbaAgent.exe
type UbaAgent struct {
	sandbox *dcSyscall.Sandbox
	jobID   string
}

// InitSandbox init sandbox
func (ua *UbaAgent) InitSandbox(sandbox *dcSyscall.Sandbox) {
	ua.sandbox = sandbox
}

// SetJobID set jobID to task
func (ua *UbaAgent) SetJobID(jobID string) {
	ua.jobID = jobID
}

// InitExtra no need
func (ua *UbaAgent) InitExtra(extra []byte) {
}

// ResultExtra no need
func (ua *UbaAgent) ResultExtra() []byte {
	return nil
}

// RenderArgs no need change
func (ua *UbaAgent) RenderArgs(config dcType.BoosterConfig, originArgs string) string {
	return originArgs
}

// PreWork no need
func (ua *UbaAgent) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork no need
func (ua *UbaAgent) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig no need
func (ua *UbaAgent) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	return nil, nil
}

// GetFilterRules no need
func (ua *UbaAgent) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	return nil, nil
}

func (cc *UbaAgent) CanExecuteWithLocalIdleResource(command []string) bool {
	return false
}

// PreExecuteNeedLock no need
func (ua *UbaAgent) PreExecuteNeedLock(command []string) bool {
	return false
}

// PostExecuteNeedLock no need
func (ua *UbaAgent) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return false
}

// PreLockWeight decide pre-execute lock weight, default 1
func (ua *UbaAgent) PreLockWeight(command []string) int32 {
	return 1
}

// PreExecute just return the origin cmd
func (ua *UbaAgent) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("ua(%s): PreExecute with command: %v", ua.jobID, command)

	exepath := command[0]
	params := command[1:]
	if strings.Contains(command[1], "BCS_RANDHOSTPORT_FOR_CONTAINER_PORT") {
		exepath = "/bin/bash"
		params = append([]string{"-c"}, command[1:]...)
	}

	return &dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir:         ua.sandbox.Dir,
				ExePath:         exepath,
				ExeName:         "",
				ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
				Params:          params,
				Inputfiles:      []dcSDK.FileDesc{},
				ResultFiles:     []string{},
			},
		},
	}, dcType.ErrorNone
}

// RemoteRetryTimes will return the remote retry times
func (ua *UbaAgent) RemoteRetryTimes() int {
	return 0
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (ua *UbaAgent) NeedRetryOnRemoteFail(command []string) bool {
	return false
}

// OnRemoteFail give chance to try other way if failed to remote execute
func (ua *UbaAgent) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (ua *UbaAgent) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	return 1
}

// PostExecute judge the result
func (ua *UbaAgent) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	blog.Infof("ua(%s): PostExecute with result: %+v", ua.jobID, *r)
	return dcType.ErrorNone
}

// LocalExecuteNeed no need
func (ua *UbaAgent) LocalExecuteNeed(command []string) bool {
	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (ua *UbaAgent) LocalLockWeight(command []string) int32 {
	return 1
}

// NeedRemoteResource check whether this command need remote resource
func (ua *UbaAgent) NeedRemoteResource(command []string) bool {
	return true
}

// LocalExecute no need
func (ua *UbaAgent) LocalExecute(command []string) dcType.BKDistCommonError {
	return dcType.ErrorNone
}

// FinalExecute no need
func (ua *UbaAgent) FinalExecute([]string) {
}

// SupportResultCache check whether this command support result cache
func (ua *UbaAgent) SupportResultCache(command []string) int {
	return 0
}

func (ua *UbaAgent) GetResultCacheKey(command []string) string {
	return ""
}
