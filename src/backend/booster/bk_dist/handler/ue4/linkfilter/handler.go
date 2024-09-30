/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package linkfilter

import (
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/link"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// TaskLinkFilter 定义了link-filter编译的描述处理对象, 一般用来处理ue4-win下的link编译
// link-filter是ue4拉起的编译器, 套了一层link
type TaskLinkFilter struct {
	sandbox *dcSyscall.Sandbox

	// tmp file list to clean
	tmpFileList []string

	// different stages args
	originArgs []string
	linkArgs   []string

	handle *link.TaskLink
}

// NewTaskLinkFilter get a new link-filter handler
func NewTaskLinkFilter() handler.Handler {

	return &TaskLinkFilter{
		sandbox:     &dcSyscall.Sandbox{},
		tmpFileList: make([]string, 0, 10),
		handle:      link.NewTaskLink(),
	}
}

// InitSandbox set sandbox to link-filter
func (lf *TaskLinkFilter) InitSandbox(sandbox *dcSyscall.Sandbox) {
	lf.sandbox = sandbox
	if lf.handle != nil {
		lf.handle.InitSandbox(sandbox)
	}
}

// InitExtra no need
func (lf *TaskLinkFilter) InitExtra(extra []byte) {
}

// ResultExtra no need
func (lf *TaskLinkFilter) ResultExtra() []byte {
	return nil
}

// RenderArgs no need change
func (lf *TaskLinkFilter) RenderArgs(config dcType.BoosterConfig, originArgs string) string {
	return originArgs
}

// PreWork no need
func (lf *TaskLinkFilter) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork no need
func (lf *TaskLinkFilter) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig no preload config need
func (lf *TaskLinkFilter) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	return nil, nil
}

func (lf *TaskLinkFilter) CanExecuteWithLocalIdleResource(command []string) bool {
	if lf.handle != nil {
		return lf.handle.CanExecuteWithLocalIdleResource(command)
	}

	return true
}

// PreExecuteNeedLock 防止预处理跑满本机CPU
func (lf *TaskLinkFilter) PreExecuteNeedLock(command []string) bool {
	return true
}

// PostExecuteNeedLock 防止回传的文件读写跑满本机磁盘
func (lf *TaskLinkFilter) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return true
}

// PreLockWeight decide pre-execute lock weight, default 1
func (lf *TaskLinkFilter) PreLockWeight(command []string) int32 {
	if lf.handle != nil {
		return lf.handle.PreLockWeight(command)
	}
	return 1
}

// PreExecute 预处理, 复用cl-handler的逻辑
func (lf *TaskLinkFilter) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return lf.preExecute(command)
}

// NeedRemoteResource check whether this command need remote resource
func (lf *TaskLinkFilter) NeedRemoteResource(command []string) bool {
	return true
}

// RemoteRetryTimes will return the remote retry times
func (lf *TaskLinkFilter) RemoteRetryTimes() int {
	return 1
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (lf *TaskLinkFilter) NeedRetryOnRemoteFail(command []string) bool {
	return false
}

// OnRemoteFail give chance to try other way if failed to remote execute
func (lf *TaskLinkFilter) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (lf *TaskLinkFilter) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	if lf.handle != nil {
		return lf.handle.PostLockWeight(result)
	}
	return 1
}

// PostExecute 后置处理, 复用cl-handler的逻辑
func (lf *TaskLinkFilter) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	return lf.postExecute(r)
}

// LocalExecuteNeed no need
func (lf *TaskLinkFilter) LocalExecuteNeed(command []string) bool {
	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (lf *TaskLinkFilter) LocalLockWeight(command []string) int32 {
	if lf.handle != nil {
		return lf.handle.LocalLockWeight(command)
	}
	return 1
}

// LocalExecute no need
func (cf *TaskLinkFilter) LocalExecute(command []string) dcType.BKDistCommonError {
	return dcType.ErrorNone
}

// FinalExecute 清理临时文件
func (lf *TaskLinkFilter) FinalExecute(args []string) {
	if lf.handle != nil {
		lf.handle.FinalExecute(args)
	}
}

// GetFilterRules add file send filter
func (lf *TaskLinkFilter) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	return nil, nil
}

func (lf *TaskLinkFilter) preExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("lf: start pre execute for: %v", command)

	if lf.handle == nil {
		blog.Warnf("lf: inner handle is nil")
		return nil, dcType.ErrorUnknown
	}

	lf.originArgs = command
	args, err := ensureCompiler(command)
	if err != nil {
		blog.Errorf("lf: pre execute ensure compiler failed %v: %v", args, err)
		return nil, dcType.ErrorUnknown
	}

	lf.linkArgs = args
	blog.Infof("lf: after pre execute, linl cmd:[%s]", lf.linkArgs)

	lf.handle.InitSandbox(lf.sandbox)

	return lf.handle.PreExecute(args)
}

func (lf *TaskLinkFilter) postExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	blog.Infof("lf: start post execute for: %v", lf.originArgs)

	if lf.handle == nil {
		blog.Warnf("lf: inner handle is nil")
		return dcType.ErrorUnknown
	}

	return lf.handle.PostExecute(r)
}
