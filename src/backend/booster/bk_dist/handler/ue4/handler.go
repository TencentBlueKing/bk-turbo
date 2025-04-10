/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package ue4

import (
	"fmt"
	"os"
	"path/filepath"

	dcConfig "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/astc"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/cc"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/cl"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/clfilter"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/lib"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/link"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/linkfilter"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/ue4/shader"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
)

const (
	defaultCLCompiler                 = "cl.exe"
	defaultShaderCompiler             = "ShaderCompileWorker.exe"
	defaultShaderCompilerMac          = "ShaderCompileWorker"
	defaultClangCompiler              = "clang.exe"
	defaultClangPlusPlusCompiler      = "clang++.exe"
	defaultClangLinuxCompiler         = "clang"
	defaultClangPlusPlusLinuxCompiler = "clang++"
	defaultLibCompiler                = "lib.exe"
	defaultLinkCompiler               = "link.exe"
	defaultCLFilterCompiler           = "cl-filter.exe"
	defaultLinkFilterCompiler         = "link-filter.exe"
	defaultAstcsse2Compiler           = "astcenc-sse2.exe"
	defaultAstcCompiler               = "astcenc.exe"
	defaultClangPS5Compiler           = "prospero-clang.exe" // regard as clang
	defaultClangCLCompiler            = "clang-cl.exe"       // regard as clang

	hookConfigPathDefault  = "bk_default_rules.json"
	hookConfigPathCCCommon = "bk_cl_rules.json"
)

// NewUE4 get ue4 scene handler
func NewUE4() (handler.Handler, error) {
	return &UE4{
		sandbox: &dcSyscall.Sandbox{},
	}, nil
}

// UE4 定义了ue4场景下的各类子场景总和,
// 包含了win/mac/linux平台下的c/c++代码编译, 链接, shader编译等
type UE4 struct {
	sandbox                  *dcSyscall.Sandbox
	innerhandler             handler.Handler
	innerhandlersanboxinited bool
}

// InitSandbox set sandbox to ue4 scene handler
func (u *UE4) InitSandbox(sandbox *dcSyscall.Sandbox) {
	u.sandbox = sandbox
	// if u.innerhandler != nil && !u.innerhandlersanboxinited {
	// 	u.innerhandler.InitSandbox(sandbox)
	// 	u.innerhandlersanboxinited = true
	// }
	u.initInnerHandleSanbox()
}

// InitExtra no need
func (u *UE4) InitExtra(extra []byte) {
}

// ResultExtra no need
func (u *UE4) ResultExtra() []byte {
	return nil
}

// RenderArgs no need change
func (u *UE4) RenderArgs(config dcType.BoosterConfig, originArgs string) string {
	return originArgs
}

// PreWork no need
func (u *UE4) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork no need
func (u *UE4) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig get preload config
func (u *UE4) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	return getPreloadConfig(u.getPreLoadConfigPath(config))
}

func getPreloadConfig(configPath string) (*dcSDK.PreloadConfig, error) {
	f, err := os.Open(configPath)
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

func (u *UE4) getPreLoadConfigPath(config dcType.BoosterConfig) string {
	if config.Works.HookConfigPath != "" {
		return config.Works.HookConfigPath
	}

	// degrade will not contain the UE4
	if config.Works.Degraded {
		return dcConfig.GetFile(hookConfigPathDefault)
	}

	return dcConfig.GetFile(hookConfigPathCCCommon)
}

// GetFilterRules will return filter rule to booster
func (u *UE4) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	return []dcSDK.FilterRuleItem{
		{
			Rule:       dcSDK.FilterRuleFileSuffix,
			Operator:   dcSDK.FilterRuleOperatorEqual,
			Standard:   ".pch",
			HandleType: dcSDK.FilterRuleHandleAllDistribution,
		},
		{
			Rule:       dcSDK.FilterRuleFileSuffix,
			Operator:   dcSDK.FilterRuleOperatorEqual,
			Standard:   ".gch",
			HandleType: dcSDK.FilterRuleHandleAllDistribution,
		},
	}, nil
}

func (u *UE4) CanExecuteWithLocalIdleResource(command []string) bool {
	if u.innerhandler == nil {
		u.initInnerHandle(command)
	}
	if u.innerhandler != nil {
		// if u.sandbox != nil {
		// 	u.innerhandler.InitSandbox(u.sandbox.Fork())
		// }
		u.initInnerHandleSanbox()
		return u.innerhandler.CanExecuteWithLocalIdleResource(command)
	}

	return true
}

// PreExecuteNeedLock 防止预处理跑满本机CPU
func (u *UE4) PreExecuteNeedLock(command []string) bool {
	return true
}

// PostExecuteNeedLock 防止回传的文件读写跑满本机磁盘
func (u *UE4) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return true
}

// PreLockWeight decide pre-execute lock weight, default 1
func (u *UE4) PreLockWeight(command []string) int32 {
	if u.innerhandler == nil {
		u.initInnerHandle(command)
	}
	if u.innerhandler != nil {
		// if u.sandbox != nil {
		// 	u.innerhandler.InitSandbox(u.sandbox.Fork())
		// }
		u.initInnerHandleSanbox()
		return u.innerhandler.PreLockWeight(command)
	}
	return 1
}

// PreExecute 预处理, 根据不同的command来确定不同的子场景
func (u *UE4) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	if command == nil || len(command) == 0 {
		blog.Warnf("command is nil")
		return nil, dcType.ErrorUnknown
	}

	u.initInnerHandle(command)

	if u.innerhandler != nil {
		// if u.sandbox != nil {
		// 	u.innerhandler.InitSandbox(u.sandbox.Fork())
		// }
		u.initInnerHandleSanbox()
		return u.innerhandler.PreExecute(command)
	}

	blog.Warnf("not support for command %s", command[0])
	return nil, dcType.BKDistCommonError{
		Code:  dcType.UnknowCode,
		Error: fmt.Errorf("not support for command %s", command[0]),
	}
}

// PreExecute 预处理, 根据不同的command来确定不同的子场景
func (u *UE4) initInnerHandle(command []string) {
	if command == nil || len(command) == 0 {
		return
	}

	if u.innerhandler == nil {
		exe := filepath.Base(command[0])
		switch exe {
		case defaultCLCompiler:
			{
				u.innerhandler = cl.NewTaskCL()
				blog.Debugf("ue4: innerhandle with cl for command[%s]", command[0])
			}
		case defaultCLFilterCompiler:
			{
				u.innerhandler = clfilter.NewTaskCLFilter()
				blog.Debugf("ue4: innerhandle with clfilter for command[%s]", command[0])
			}
		case defaultShaderCompiler, defaultShaderCompilerMac:
			{
				u.innerhandler = shader.NewUE4Shader()
				blog.Debugf("ue4: innerhandle with shader for command[%s]", command[0])
			}
		case defaultClangCompiler, defaultClangPlusPlusCompiler,
			defaultClangLinuxCompiler, defaultClangPlusPlusLinuxCompiler,
			defaultClangPS5Compiler, defaultClangCLCompiler:
			{
				u.innerhandler = cc.NewTaskCC()
				blog.Debugf("ue4: innerhandle with cc for command[%s]", command[0])
			}
		case defaultAstcsse2Compiler, defaultAstcCompiler:
			{
				u.innerhandler = astc.NewTextureCompressor()
				blog.Debugf("ue4: innerhandle with clfilter for command[%s]", command[0])
			}
		case defaultLinkFilterCompiler:
			{
				if env.GetEnv(env.KeyExecutorEnableLink) == "true" {
					u.innerhandler = linkfilter.NewTaskLinkFilter()
					blog.Debugf("ue4: innerhandle with linkfilter for command[%s]", command[0])
				}
			}
		case defaultLibCompiler:
			{
				if env.GetEnv(env.KeyExecutorEnableLib) == "true" {
					u.innerhandler = lib.NewTaskLib()
					blog.Debugf("ue4: innerhandle with lib for command[%s]", command[0])
				}
			}
		case defaultLinkCompiler:
			{
				if env.GetEnv(env.KeyExecutorEnableLink) == "true" {
					u.innerhandler = link.NewTaskLink()
					blog.Debugf("ue4: innerhandle with link for command[%s]", command[0])
				}
			}
		}
	}
}

func (u *UE4) initInnerHandleSanbox() {
	if u.innerhandler != nil && !u.innerhandlersanboxinited && u.sandbox != nil {
		u.innerhandler.InitSandbox(u.sandbox)
		u.innerhandlersanboxinited = true
	}
}

// NeedRemoteResource check whether this command need remote resource
func (u *UE4) NeedRemoteResource(command []string) bool {
	if u.innerhandler != nil {
		return u.innerhandler.NeedRemoteResource(command)
	}

	u.initInnerHandle(command)

	if u.innerhandler != nil {
		return u.innerhandler.NeedRemoteResource(command)
	}

	return false
}

// RemoteRetryTimes will return the remote retry times
func (u *UE4) RemoteRetryTimes() int {
	if u.innerhandler != nil {
		return u.innerhandler.RemoteRetryTimes()
	}

	return 0
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (u *UE4) NeedRetryOnRemoteFail(command []string) bool {
	if u.innerhandler != nil {
		return u.innerhandler.NeedRetryOnRemoteFail(command)
	}

	return false
}

// OnRemoteFail give chance to try other way if failed to remote execute
func (u *UE4) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	if u.innerhandler != nil {
		return u.innerhandler.OnRemoteFail(command)
	}

	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (u *UE4) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	if u.innerhandler != nil {
		return u.innerhandler.PostLockWeight(result)
	}

	return 1
}

// PostExecute 后置处理
func (u *UE4) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	if u.innerhandler != nil {
		return u.innerhandler.PostExecute(r)
	}

	blog.Warnf("innerhandler is nil when ready post execute")
	return dcType.ErrorUnknown
}

// LocalExecuteNeed no need
func (u *UE4) LocalExecuteNeed(command []string) bool {
	if u.innerhandler != nil {
		return u.innerhandler.LocalExecuteNeed(command)
	}

	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (u *UE4) LocalLockWeight(command []string) int32 {
	if u.innerhandler == nil {
		u.initInnerHandle(command)
	}

	if u.innerhandler != nil {
		// if u.sandbox != nil {
		// 	u.innerhandler.InitSandbox(u.sandbox.Fork())
		// }
		u.initInnerHandleSanbox()
		return u.innerhandler.LocalLockWeight(command)
	}

	return 1
}

// LocalExecute no need
func (u *UE4) LocalExecute(command []string) dcType.BKDistCommonError {
	if u.innerhandler != nil {
		return u.innerhandler.LocalExecute(command)
	}

	return dcType.ErrorNone
}

// FinalExecute 清理临时文件
func (u *UE4) FinalExecute(args []string) {
	if u.innerhandler != nil {
		u.innerhandler.FinalExecute(args)
	}
}

// SupportResultCache check whether this command support result cache
func (u *UE4) SupportResultCache(command []string) int {
	if u.innerhandler == nil {
		u.initInnerHandle(command)
	}

	if u.innerhandler != nil {
		u.initInnerHandleSanbox()
		return u.innerhandler.SupportResultCache(command)
	}

	return 0
}

func (u *UE4) GetResultCacheKey(command []string) string {
	if u.innerhandler == nil {
		u.initInnerHandle(command)
	}

	if u.innerhandler != nil {
		u.initInnerHandleSanbox()
		return u.innerhandler.GetResultCacheKey(command)
	}

	return ""
}
