/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package handler

import (
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
)

// Handler describe
type Handler interface {
	// InitExtra receive the extra data from project settings and init the handle extra data
	InitExtra(extra []byte)

	// ResultExtra return the extra message to record in project info
	ResultExtra() (extra []byte)

	InitSandbox(sandbox *dcSyscall.Sandbox)

	// PreWork define the work before all executors run.
	PreWork(config *dcType.BoosterConfig) error

	// PostWork define the work after all executors end.
	PostWork(config *dcType.BoosterConfig) error

	// RenderArgs return the actual executing args with the origin args and config
	RenderArgs(config dcType.BoosterConfig, originArgs string) string

	// GetPreloadConfig define the preload config from settings.
	GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error)

	// GetFilterRules return files which will be used by multi-executor, send only once
	GetFilterRules() ([]dcSDK.FilterRuleItem, error)

	// CanExecuteWithLocalIdleResource return whether can execute this with local idle resource,
	// true by default
	CanExecuteWithLocalIdleResource(command []string) bool

	// PreExecuteNeedLock decide whether executor should lock before pre execution
	PreExecuteNeedLock(command []string) bool

	// PreLockWeight decide pre-execute lock weight, default 1
	PreLockWeight(command []string) int32

	// PreExecute will be called before task is distributed
	PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError)

	// LocalExecuteNeed decide whether executor should execute local command
	LocalExecuteNeed(command []string) bool

	// LocalLockWeight decide local-execute lock weight, default 1
	LocalLockWeight(command []string) int32

	// LocalExecute will execute this command by handler
	LocalExecute(command []string) dcType.BKDistCommonError

	// NeedRemoteResource check whether this command need remote resource
	NeedRemoteResource(command []string) bool

	// RemoteRetryTimes will return the remote retry times
	RemoteRetryTimes() int

	// NeedRetryOnRemoteFail check whether need retry on remote fail
	NeedRetryOnRemoteFail(command []string) bool

	// OnRemoteFail give chance to try other way if failed to remote execute
	OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError)

	// PostExecuteNeedLock decide whether executor should lock before post execution
	PostExecuteNeedLock(result *dcSDK.BKDistResult) bool

	// PostLockWeight decide post-execute lock weight, default 1
	PostLockWeight(result *dcSDK.BKDistResult) int32

	// PostExecute will be called after task is distributed and executed
	PostExecute(result *dcSDK.BKDistResult) dcType.BKDistCommonError

	// FinalExecute chance to finalize for handler, must be safe to call in goroutines
	FinalExecute(command []string)
}
