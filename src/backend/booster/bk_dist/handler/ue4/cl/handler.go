/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package cl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	dcConfig "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/config"
	dcEnv "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcPump "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/pump"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/resultcache"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler"
	commonUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/common"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/cespare/xxhash/v2"
)

const (
	hookConfigPathDefault  = "bk_default_rules.json"
	hookConfigPathCCCommon = "bk_cl_rules.json"

	MaxWindowsCommandLength = 30000

	appendEnvKey = "INCLUDE="
	osWindows    = "windows"
)

var (
	// ForceLocalFileKeys force some module to compile locally
	// /GL for ProxyLOD
	// wrong inlcude for NetCore by user
	// unsuport PUSH_MACRO and POP_MACRO in NvClothIncludes.h for ClothingSystemRuntime
	DefaultForceLocalResponseFileKeys = []string{
		"dte80a",
		"ProxyLOD",
		"NetCore",
		"ClothingSystemRuntime",
		"Module.LuaTools.cpp",
		"Module.Client.1_of_4.cpp",
		"Module.HoloLensTargetPlatform.cpp",
		"msado15.cpp",
	}
	// ForceLocalCppFileKeys force some cpp to compile locally
	DefaultForceLocalCppFileKeys = []string{
		"dte80a",
		"ProxyLOD",
		"NetCore",
		"ClothingSystemRuntime",
		"Module.LuaTools.cpp",
		"Module.Client.1_of_4.cpp",
		"Module.HoloLensTargetPlatform.cpp",
		"msado15.cpp",
	}
	// DisabledWarnings for ue4 ,disable some warnings
	DisabledWarnings = []string{"/wd4828"}
)

// TaskCL 定义了cl.exe编译的描述处理对象, 一般用来处理ue4-win下的cl编译
type TaskCL struct {
	sandbox *dcSyscall.Sandbox

	ccacheEnable bool

	// tmp file list to clean
	tmpFileList []string

	// different stages args
	originArgs       []string
	ensuredArgs      []string
	expandArgs       []string
	scannedArgs      []string
	rewriteCrossArgs []string
	preProcessArgs   []string
	serverSideArgs   []string
	resultCacheArgs  []string
	pumpArgs         []string

	// file names
	inputFile        string
	preprocessedFile string
	outputFile       string
	firstIncludeFile string
	pchFile          string
	responseFile     string
	sourcedependfile string
	pumpHeadFile     string
	includeRspFiles  []string // 在rsp中通过@指定的其它rsp文件，需要发送到远端
	// 在rsp中/I后面的参数，需要将这些目录全部发送到远端
	// 有特殊场景：编译不需要该路径下的文件，但需要该路径作为跳板，去查找其它相对路径下的头文件（或其它依赖文件）
	includePaths []string
	logfilesarif string

	// forcedepend 是我们主动导出依赖文件，showinclude 是编译命令已经指定了导出依赖文件
	forcedepend          bool
	pumpremote           bool
	needcopypumpheadfile bool
	pumpremotefailed     bool

	// how to save result file
	customSave bool

	// to save preprocessed file content
	preprocessedBuffer []byte

	// for /showIncludes
	showinclude          bool
	preprocessedErrorBuf string

	pchFileDesc *dcSDK.FileDesc

	ForceLocalResponseFileKeys []string
	ForceLocalCppFileKeys      []string
}

// NewTaskCL get a new cl-handler
func NewTaskCL() handler.Handler {
	key1 := make([]string, len(DefaultForceLocalResponseFileKeys))
	copy(key1, DefaultForceLocalResponseFileKeys)

	key2 := make([]string, len(DefaultForceLocalCppFileKeys))
	copy(key2, DefaultForceLocalCppFileKeys)

	return &TaskCL{
		sandbox:                    &dcSyscall.Sandbox{},
		tmpFileList:                make([]string, 0, 10),
		ForceLocalResponseFileKeys: key1,
		ForceLocalCppFileKeys:      key2,
	}
}

// ++for cl-filter
func NewCL() *TaskCL {
	key1 := make([]string, len(DefaultForceLocalResponseFileKeys))
	copy(key1, DefaultForceLocalResponseFileKeys)

	key2 := make([]string, len(DefaultForceLocalCppFileKeys))
	copy(key2, DefaultForceLocalCppFileKeys)

	return &TaskCL{
		sandbox:                    &dcSyscall.Sandbox{},
		tmpFileList:                make([]string, 0, 10),
		ForceLocalResponseFileKeys: key1,
		ForceLocalCppFileKeys:      key2,
	}
}

func (cl *TaskCL) SetDepend(f string) {
	cl.sourcedependfile = f
}

// GetPreprocessedBuf return preprocessedErrorBuf
func (cl *TaskCL) GetPreprocessedBuf() string {
	return cl.preprocessedErrorBuf
}

// --

// InitSandbox set sandbox to task-cl
func (cl *TaskCL) InitSandbox(sandbox *dcSyscall.Sandbox) {
	cl.sandbox = sandbox
}

// InitExtra no need
func (cl *TaskCL) InitExtra(extra []byte) {
}

// ResultExtra no need
func (cl *TaskCL) ResultExtra() []byte {
	return nil
}

// RenderArgs no need change
func (cl *TaskCL) RenderArgs(config dcType.BoosterConfig, originArgs string) string {
	return originArgs
}

// PreWork no need
func (cl *TaskCL) PreWork(config *dcType.BoosterConfig) error {
	return nil
}

// PostWork no need
func (cl *TaskCL) PostWork(config *dcType.BoosterConfig) error {
	return nil
}

// GetPreloadConfig get preload config
func (cl *TaskCL) GetPreloadConfig(config dcType.BoosterConfig) (*dcSDK.PreloadConfig, error) {
	return getPreloadConfig(cl.getPreLoadConfigPath(config))
}

func (cl *TaskCL) getPreLoadConfigPath(config dcType.BoosterConfig) string {
	if config.Works.HookConfigPath != "" {
		return config.Works.HookConfigPath
	}

	// degrade will not contain the CL
	if config.Works.Degraded {
		return dcConfig.GetFile(hookConfigPathDefault)
	}

	return dcConfig.GetFile(hookConfigPathCCCommon)
}

func (cl *TaskCL) CanExecuteWithLocalIdleResource(command []string) bool {
	if cl.sandbox.Env.GetEnv(dcEnv.KeyExecutorUECLNotUseLocal) == "true" {
		return false
	}

	return true
}

// PreExecuteNeedLock 防止预处理跑满本机CPU
func (cl *TaskCL) PreExecuteNeedLock(command []string) bool {
	return true
}

// PostExecuteNeedLock 防止回传的文件读写跑满本机磁盘
func (cl *TaskCL) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	// to avoid memory overflow when pump
	if dcPump.SupportPump(cl.sandbox.Env) {
		// return false
		return true
	} else {
		return true
	}
}

// PreLockWeight decide pre-execute lock weight, default 1
func (cl *TaskCL) PreLockWeight(command []string) int32 {
	return 1
}

// PreExecute 预处理
func (cl *TaskCL) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return cl.preExecute(command)
}

// NeedRemoteResource check whether this command need remote resource
func (cl *TaskCL) NeedRemoteResource(command []string) bool {
	return true
}

// RemoteRetryTimes will return the remote retry times
func (cl *TaskCL) RemoteRetryTimes() int {
	return 1
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (cl *TaskCL) NeedRetryOnRemoteFail(command []string) bool {
	return cl.pumpremote
}

// TODO : OnRemoteFail give chance to try other way if failed to remote execute
func (cl *TaskCL) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("cl: start OnRemoteFail for: %v", command)

	if cl.pumpremote {
		blog.Infof("cl: set pumpremotefailed to true now")
		cl.pumpremotefailed = true
		cl.needcopypumpheadfile = true
		cl.pumpremote = false
		return cl.preExecute(command)
	}
	return nil, dcType.ErrorNone
}

// LocalLockWeight decide local-execute lock weight, default 1
func (cl *TaskCL) LocalLockWeight(command []string) int32 {
	envvalue := cl.sandbox.Env.GetEnv(dcEnv.KeyExecutorUECLLocalCPUWeight)
	if envvalue != "" {
		w, err := strconv.Atoi(envvalue)
		if err == nil && w > 0 && w <= runtime.NumCPU() {
			return int32(w)
		}
	}

	return 1
}

// PostExecute 后置处理
func (cl *TaskCL) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	return cl.postExecute(r, false)
}

// PostExecuteByCLFilter 后置处理，由clfilter调用
func (cl *TaskCL) PostExecuteByCLFilter(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	return cl.postExecute(r, true)
}

// LocalExecuteNeed no need
func (cl *TaskCL) LocalExecuteNeed(command []string) bool {
	return false
}

// PostLockWeight decide post-execute lock weight, default 1
func (cl *TaskCL) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	return 1
}

// LocalExecute no need
func (cl *TaskCL) LocalExecute(command []string) dcType.BKDistCommonError {
	if len(command) < 1 {
		blog.Warnf("cl: failed to execute command, for args is empty")
		return dcType.ErrorUnknown
	}

	sandbox := cl.sandbox.Fork()
	flag, rspfile, err := cl.needSaveResponseFile(command)
	if flag && err == nil {
		code, err := sandbox.ExecCommand(command[0], fmt.Sprintf("@%s", rspfile))
		return dcType.BKDistCommonError{Code: code, Error: err}
	} else {
		code, err := sandbox.ExecCommand(command[0], command[1:]...)
		return dcType.BKDistCommonError{Code: code, Error: err}
	}
}

// FinalExecute 清理临时文件
func (cl *TaskCL) FinalExecute(args []string) {
	cl.finalExecute(args)
}

// GetFilterRules add file send filter
func (cl *TaskCL) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	// return []dcSDK.FilterRuleItem{
	// 	{
	// 		Rule:     dcSDK.FilterRuleFileSuffix,
	// 		Operator: dcSDK.FilterRuleOperatorEqual,
	// 		Standard: ".pch",
	// 	},
	// }, nil
	return nil, nil
}

func (cl *TaskCL) getIncludeExe() (string, error) {
	blog.Debugf("cl: ready get include exe")

	target := "bk-includes"
	if runtime.GOOS == osWindows {
		target = "bk-includes.exe"
	}

	includePath, err := dcUtil.CheckExecutable(target)
	if err != nil {
		// blog.Infof("cl: not found exe file with default path, info: %v", err)

		includePath, err = dcUtil.CheckFileWithCallerPath(target)
		if err != nil {
			blog.Errorf("cl: not found exe file with error: %v", err)
			return includePath, err
		}
	}
	absPath, err := filepath.Abs(includePath)
	if err == nil {
		includePath = absPath
	}
	includePath = dcUtil.QuoteSpacePath(includePath)
	// blog.Infof("cl: got include exe file full path: %s", includePath)

	return includePath, nil
}

// func uniqArr(arr []string) []string {
// 	newarr := make([]string, 0)
// 	tempMap := make(map[string]bool, len(newarr))
// 	for _, v := range arr {
// 		if tempMap[v] == false {
// 			tempMap[v] = true
// 			newarr = append(newarr, v)
// 		}
// 	}

// 	return newarr
// }

func (cl *TaskCL) analyzeIncludes(f string, workdir string) ([]*dcFile.Info, error) {
	data, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\r\n")
	uniqlines := dcUtil.UniqArr(lines)
	blog.Infof("cl: got %d uniq include file from file: %s", len(uniqlines), f)

	// if dcPump.SupportPumpStatCache(cl.sandbox.Env) {
	// return commonUtil.GetFileInfo(uniqlines, true, true, dcPump.SupportPumpLstatByDir(cl.sandbox.Env))
	return dcFile.GetFileInfo(uniqlines, false, false, dcPump.SupportPumpLstatByDir(cl.sandbox.Env))
	// } else {
	// 	includes := []*dcFile.Info{}
	// 	for _, l := range uniqlines {
	// 		if !filepath.IsAbs(l) {
	// 			l, _ = filepath.Abs(filepath.Join(workdir, l))
	// 		}
	// 		fstat := dcFile.Stat(l)
	// 		if fstat.Exist() && !fstat.Basic().IsDir() {
	// 			includes = append(includes, fstat)
	// 		} else {
	// 			blog.Warnf("cl: do not deal include file: %s in file:%s for not existed or is dir", l, f)
	// 			// return fail if not existed
	// 			return nil, fmt.Errorf("%s not existed", f)
	// 		}
	// 	}

	// 	return includes, nil
	// }
}

func (cl *TaskCL) checkFstat(f string, workdir string) (*dcFile.Info, error) {
	if !filepath.IsAbs(f) {
		f, _ = filepath.Abs(filepath.Join(workdir, f))
	}
	fstat := dcFile.Stat(f)
	if fstat.Exist() && !fstat.Basic().IsDir() {
		return fstat, nil
	}

	return nil, nil
}

type sourceDependenciesData struct {
	Source         string   `json:"Source"`
	ProvidedModule string   `json:"ProvidedModule"`
	PCH            string   `json:"PCH"`
	Includes       []string `json:"Includes"`
}

type sourceDependencies struct {
	Version string                 `json:"Version"`
	Data    sourceDependenciesData `json:"Data"`
}

// func formatFilePath(f string) string {
// 	f = strings.Replace(f, "/", "\\", -1)
// 	f = strings.Replace(f, "\\\\", "\\", -1)

// 	// 去掉路径中的..
// 	if strings.Contains(f, "..") {
// 		p := strings.Split(f, "\\")

// 		var newPath []string
// 		for _, v := range p {
// 			if v == ".." {
// 				newPath = newPath[:len(newPath)-1]
// 			} else {
// 				newPath = append(newPath, v)
// 			}
// 		}
// 		f = strings.Join(newPath, "\\")
// 	}

// 	return f
// }

func (cl *TaskCL) copyPumpHeadFile(workdir string) error {
	blog.Infof("cl: copy pump head file: %s to: %s", cl.sourcedependfile, cl.pumpHeadFile)

	// 只拷贝由加速编译预处理生成的依赖文件；非加速模式下生成的依赖文件不完整，去掉了系统文件
	if cl.inputFile == "" {
		blog.Infof("cl: not found input file,so do not copy depend file: %s with err:%v", cl.sourcedependfile, ErrorNotRemoteTask)
		return ErrorNotRemoteTask
	}

	data, err := ioutil.ReadFile(cl.sourcedependfile)
	if err != nil {
		blog.Warnf("cl: copy pump head failed to read depend file: %s with err:%v", cl.sourcedependfile, err)
		return err
	}

	sep := "\n"
	if runtime.GOOS == osWindows {
		sep = "\r\n"
	}

	includes := []string{}
	if strings.HasSuffix(cl.sourcedependfile, ".json") {
		var depend sourceDependencies
		if err := json.Unmarshal(data, &depend); err == nil {
			l := depend.Data.Source
			if !filepath.IsAbs(l) {
				l, _ = filepath.Abs(filepath.Join(workdir, l))
			}
			includes = append(includes, dcUtil.FormatFilePath(l))

			for _, l := range depend.Data.Includes {
				if !filepath.IsAbs(l) {
					l, _ = filepath.Abs(filepath.Join(workdir, l))
				}
				includes = append(includes, dcUtil.FormatFilePath(l))

				// 如果是链接，则将相关指向的文件都包含进来
				fs := dcUtil.GetAllLinkFiles(l)
				if len(fs) > 0 {
					includes = append(includes, fs...)
				}
			}
		} else {
			blog.Warnf("cl: failed to resolve depend file: %s with err:%s", cl.sourcedependfile, err)
			return err
		}
	} else {
		lines := strings.Split(string(data), sep)
		for _, l := range lines {
			l = strings.Trim(l, " \r\n\\")
			if !filepath.IsAbs(l) {
				l, _ = filepath.Abs(filepath.Join(workdir, l))
			}
			includes = append(includes, dcUtil.FormatFilePath(l))

			// 如果是链接，则将相关指向的文件都包含进来
			fs := dcUtil.GetAllLinkFiles(l)
			if len(fs) > 0 {
				includes = append(includes, fs...)
			}
		}
	}

	// copy input file
	if cl.inputFile != "" {
		l := cl.inputFile
		if !filepath.IsAbs(l) {
			l, _ = filepath.Abs(filepath.Join(workdir, l))
		}
		includes = append(includes, dcUtil.FormatFilePath(l))
	}

	// copy includeRspFiles
	if len(cl.includeRspFiles) > 0 {
		for _, l := range cl.includeRspFiles {
			blog.Infof("cl: ready add rsp file: %s", l)
			if !filepath.IsAbs(l) {
				l, _ = filepath.Abs(filepath.Join(workdir, l))
			}
			includes = append(includes, dcUtil.FormatFilePath(l))
		}
	}

	// copy includePaths
	if len(cl.includePaths) > 0 {
		for _, l := range cl.includePaths {
			blog.Infof("cl: ready add include path: %s", l)
			if !filepath.IsAbs(l) {
				l, _ = filepath.Abs(filepath.Join(workdir, l))
			}
			includes = append(includes, dcUtil.FormatFilePath(l))
		}
	}

	blog.Infof("cl: copy pump head got %d uniq include file from file: %s", len(includes), cl.sourcedependfile)

	if len(includes) == 0 {
		blog.Warnf("cl: depend file: %s data:[%s] is invalid", cl.sourcedependfile, string(data))
		return ErrorInvalidDependFile
	}

	// for i := range includes {
	// 	includes[i] = strings.Replace(includes[i], "/", "\\", -1)
	// }
	uniqlines := dcUtil.UniqArr(includes)

	if dcPump.PumpCorrectCap(cl.sandbox.Env) {
		uniqlines, _ = dcUtil.CorrectPathCap(uniqlines)
	}

	// TODO : save to cc.pumpHeadFile
	newdata := strings.Join(uniqlines, sep)
	err = ioutil.WriteFile(cl.pumpHeadFile, []byte(newdata), os.ModePerm)
	if err != nil {
		blog.Warnf("cl: copy pump head failed to write file: %s with err:%v", cl.pumpHeadFile, err)
		return err
	} else {
		blog.Infof("cl: copy pump head succeed to write file: %s", cl.pumpHeadFile)
	}

	return nil
}

// search all include files for this compile command
func (cl *TaskCL) Includes(responseFile string, args []string, workdir string, forcefresh bool) ([]*dcFile.Info, error) {
	pumpdir := dcPump.PumpCacheDir(cl.sandbox.Env)
	if pumpdir == "" {
		pumpdir = dcUtil.GetPumpCacheDir()
	}

	if !dcFile.Stat(pumpdir).Exist() {
		if err := os.MkdirAll(pumpdir, os.ModePerm); err != nil {
			return nil, err
		}
	}

	// TOOD : maybe we should pass responseFile to calc md5, to ensure unique
	var err error
	cl.pumpHeadFile, err = getPumpIncludeFile(pumpdir, "pump_heads", ".txt", args, workdir)
	if err != nil {
		blog.Errorf("cl: do includes get output file failed: %v", err)
		return nil, err
	}

	existed, fileSize, _, _ := dcFile.Stat(cl.pumpHeadFile).Batch()
	if dcPump.IsPumpCache(cl.sandbox.Env) && !forcefresh && existed && fileSize > 0 {
		return cl.analyzeIncludes(cl.pumpHeadFile, workdir)
	}

	return nil, ErrorNoPumpHeadFile
}

func (cl *TaskCL) forceDepend() error {
	cl.sourcedependfile = makeTmpFileName(commonUtil.GetHandlerTmpDir(cl.sandbox), "cl_depend", ".txt")
	cl.addTmpFile(cl.sourcedependfile)

	cl.forcedepend = true
	// args = append(args, "/showIncludes")

	return nil
}

func (cl *TaskCL) inPumpBlack(responseFile string, args []string) (bool, error) {
	// obtain black key set by booster
	blackkeystr := cl.sandbox.Env.GetEnv(dcEnv.KeyExecutorPumpBlackKeys)
	if blackkeystr != "" {
		// blog.Infof("cl: got pump black key string: %s", blackkeystr)
		blacklist := strings.Split(blackkeystr, dcEnv.CommonBKEnvSepKey)
		if len(blacklist) > 0 {
			for _, v := range blacklist {
				if v != "" && strings.Contains(responseFile, v) {
					blog.Infof("cl: found response %s is in pump blacklist", responseFile)
					return true, nil
				}

				for _, v1 := range args {
					if strings.HasSuffix(v1, ".cpp") && strings.Contains(v1, v) {
						blog.Infof("cl: found arg %s is in pump blacklist", v1)
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

// first error means real error when try pump, second is notify error
func (cl *TaskCL) trypump(command []string) (*dcSDK.BKDistCommand, error, error) {
	blog.Infof("cl: trypump: %v", command)

	// TODO : !! ensureCompilerRaw changed the command slice, it maybe not we need !!
	tstart := time.Now().Local()
	responseFile, args, showinclude, sourcedependfile, objectfile, pchfile, err := ensureCompilerRaw(command, cl.sandbox.Dir)
	if err != nil {
		blog.Debugf("cl: pre execute ensure compiler failed %v: %v", args, err)
		return nil, err, nil
	} else {
		blog.Infof("cl: after parse command, got responseFile:%s,sourcedepent:%s,objectfile:%s,pchfile:%s",
			responseFile, sourcedependfile, objectfile, pchfile)
	}
	tend := time.Now().Local()
	blog.Debugf("cl: trypump time record: %s for ensureCompilerRaw for rsp file:%s", tend.Sub(tstart), responseFile)

	tstart = tend

	// check whether support remote execute
	scandata, err := scanArgs(args)
	if err != nil {
		blog.Debugf("cl: try pump not support, scan args %v: %v", args, err)
		return nil, err, ErrorNotSupportRemote
	}
	cl.logfilesarif = scandata.logfilesarif

	inblack, _ := cl.inPumpBlack(responseFile, args)
	if inblack {
		return nil, ErrorInPumpBlack, nil
	}

	if cl.sourcedependfile == "" {
		if sourcedependfile != "" {
			cl.sourcedependfile = sourcedependfile
		} else {
			// TODO : 我们可以主动加上 /showIncludes 参数得到依赖列表，生成一个临时的 cl.sourcedependfile 文件
			blog.Infof("cl: trypump not found depend file, try append it")
			if cl.forceDepend() != nil {
				return nil, ErrorNoDependFile, nil
			}
		}
	}
	cl.showinclude = showinclude
	cl.needcopypumpheadfile = true

	tend = time.Now().Local()
	blog.Debugf("cl: trypump time record: %s for scanArgs for rsp file:%s", tend.Sub(tstart), responseFile)
	tstart = tend

	cl.responseFile = responseFile
	cl.pumpArgs = args

	// if cl.sourcedependfile != "" {
	// cl.needcopypumpheadfile = true
	// }

	includes, err := cl.Includes(responseFile, args, cl.sandbox.Dir, false)

	tend = time.Now().Local()
	blog.Debugf("cl: trypump time record: %s for Includes for rsp file:%s", tend.Sub(tstart), responseFile)
	tstart = tend

	if err == nil {
		// add pch file as input
		if pchfile != "" {
			// includes = append(includes, pchfile)
			finfo, _ := cl.checkFstat(pchfile, cl.sandbox.Dir)
			if finfo != nil {
				includes = append(includes, finfo)
			}
		}

		// add response file as input
		if responseFile != "" {
			// includes = append(includes, responseFile)
			finfo, _ := cl.checkFstat(responseFile, cl.sandbox.Dir)
			if finfo != nil {
				includes = append(includes, finfo)
			}
		}

		oldlen := len(includes)
		includes = dcFile.Uniq(includes)
		blog.Infof("cc: parse command,got total %d uniq %d includes files", oldlen, len(includes))

		inputFiles := []dcSDK.FileDesc{}
		// priority := dcSDK.MaxFileDescPriority
		for _, f := range includes {
			// TODO : 前面的 cl.Includes 已经调用过 dcFile.Stat 了，考虑将结果传过来，避免再次调用
			// existed, fileSize, modifyTime, fileMode := dcFile.Stat(f).Batch()
			existed, fileSize, modifyTime, fileMode := f.Batch()
			fpath := f.Path()
			if !existed {
				err := fmt.Errorf("input response file %s not existed", fpath)
				blog.Errorf("%v", err)
				return nil, err, nil
			}
			inputFiles = append(inputFiles, dcSDK.FileDesc{
				FilePath:           fpath,
				Compresstype:       protocol.CompressLZ4,
				FileSize:           fileSize,
				Lastmodifytime:     modifyTime,
				Md5:                "",
				Filemode:           fileMode,
				Targetrelativepath: filepath.Dir(fpath),
				NoDuplicated:       true,
				Priority:           dcSDK.GetPriority(f),
			})
			// priority++

			blog.Debugf("cl: added include file:%s for object:%s", fpath, objectfile)
		}

		results := []string{objectfile}
		// add source depend file as result
		if sourcedependfile != "" {
			results = append(results, sourcedependfile)
		}
		if cl.logfilesarif != "" {
			if !filepath.IsAbs(cl.logfilesarif) {
				cl.logfilesarif, _ = filepath.Abs(filepath.Join(cl.sandbox.Dir, cl.logfilesarif))
			}
			results = append(results, cl.logfilesarif)
		}

		// set env which need append to remote
		envs := []string{}
		for _, v := range cl.sandbox.Env.Source() {
			if strings.HasPrefix(v, appendEnvKey) {
				envs = append(envs, v)
				// set flag we hope append env, not overwrite
				flag := fmt.Sprintf("%s=true", dcEnv.GetEnvKey(dcEnv.KeyRemoteEnvAppend))
				envs = append(envs, flag)
				break
			}
		}
		blog.Infof("cl: env which ready sent to remote:[%v]", envs)

		exeName := command[0]
		params := command[1:]
		blog.Infof("cl: parse command,server command:[%s %s],dir[%s]",
			exeName, strings.Join(params, " "), cl.sandbox.Dir)

		cl.customSave = true
		return &dcSDK.BKDistCommand{
			Commands: []dcSDK.BKCommand{
				{
					WorkDir:         cl.sandbox.Dir,
					ExePath:         "",
					ExeName:         exeName,
					ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
					Params:          params,
					Inputfiles:      inputFiles,
					ResultFiles:     results,
					Env:             envs,
				},
			},
			CustomSave: true,
		}, nil, nil
	}

	tend = time.Now().Local()
	blog.Debugf("cl: trypump time record: %s for return dcSDK.BKCommand for rsp file:%s", tend.Sub(tstart), responseFile)

	return nil, err, nil
}

func (cl *TaskCL) isPumpActionNumSatisfied() (bool, error) {
	minnum := dcPump.PumpMinActionNum(cl.sandbox.Env)
	if minnum <= 0 {
		return true, nil
	}

	curbatchsize := 0
	strsize := cl.sandbox.Env.GetEnv(dcEnv.KeyExecutorTotalActionNum)
	if strsize != "" {
		size, err := strconv.Atoi(strsize)
		if err != nil {
			return true, err
		} else {
			curbatchsize = size
		}
	}

	blog.Infof("cl: check pump action num with min:%d: current batch num:%d", minnum, curbatchsize)

	return int32(curbatchsize) > minnum, nil
}

func (cl *TaskCL) workerSupportAbsPath() bool {
	v := cl.sandbox.Env.GetEnv(dcEnv.KeyWorkerSupportAbsPath)
	if v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return true
}

func (cl *TaskCL) preExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("cl: start pre execute for: %v", command)

	// debugRecordFileName(fmt.Sprintf("cl: start pre execute for: %v", command))

	cl.originArgs = command

	// ++ try with pump,only support windows now
	if !cl.hasResultIndex() {
		if !cl.pumpremotefailed && dcPump.SupportPump(cl.sandbox.Env) && cl.workerSupportAbsPath() {
			if satisfied, _ := cl.isPumpActionNumSatisfied(); satisfied {
				req, err, notifyerr := cl.trypump(command)
				if err != nil {
					if notifyerr == ErrorNotSupportRemote {
						blog.Warnf("cl: pre execute failed to try pump %v: %v", command, err)
						return nil, dcType.BKDistCommonError{
							Code:  dcType.UnknowCode,
							Error: err,
						}
					}
				} else {
					cl.pumpremote = true
					return req, dcType.ErrorNone
				}
			}
		}
	}
	// --

	responseFile, args, showinclude, err := ensureCompiler(command, cl.sandbox.Dir)
	if err != nil {
		blog.Warnf("cl: pre execute ensure compiler failed %v: %v", args, err)
		return nil, dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: err,
		}
	}

	// obtain force key set by booster
	if cl.sandbox != nil && cl.sandbox.Env != nil {
		forcekeystr := cl.sandbox.Env.GetEnv(dcEnv.KeyExecutorForceLocalKeys)
		if forcekeystr != "" {
			blog.Infof("cl: got force local key string: %s", forcekeystr)
			forcekeylist := strings.Split(forcekeystr, dcEnv.CommonBKEnvSepKey)
			if len(forcekeylist) > 0 {
				cl.ForceLocalResponseFileKeys = append(cl.ForceLocalResponseFileKeys, forcekeylist...)
				cl.ForceLocalCppFileKeys = append(cl.ForceLocalCppFileKeys, forcekeylist...)
				blog.Infof("cl: ForceLocalResponseFileKeys: %v, ForceLocalCppFileKeys: %v",
					cl.ForceLocalResponseFileKeys, cl.ForceLocalCppFileKeys)
			}
		}
	}

	// ++ by tomtian 20201030
	if responseFile != "" {
		for _, v := range cl.ForceLocalResponseFileKeys {
			if v != "" && strings.Contains(responseFile, v) {
				blog.Warnf("cl: pre execute found response %s is in force local list, do not deal now",
					responseFile)
				return nil, dcType.ErrorPreForceLocal
			}
		}
	}
	// --

	for _, v := range args {
		if strings.HasSuffix(v, ".cpp") {
			for _, v1 := range cl.ForceLocalCppFileKeys {
				if v1 != "" && strings.Contains(v, v1) {
					blog.Warnf("cl: pre execute found %s is in force local list, do not deal now", v)
					return nil, dcType.ErrorPreForceLocal
				}
			}
			break
		}
	}

	cl.responseFile = responseFile
	cl.ensuredArgs = args
	cl.showinclude = showinclude

	// debugRecordFileName("preBuild begin")

	tstart := time.Now().Local()

	if cl.forcedepend {
		args = append(args, "/showIncludes")
	}

	if err = cl.preBuild(args); err != nil {
		blog.Debugf("cl: pre execute pre-build %v: %v", args, err)
		return nil, dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: err,
		}
	}

	tend := time.Now().Local()
	blog.Debugf("cl: trypump time record: %s for preBuild for rsp file:%s", tend.Sub(tstart), responseFile)

	// debugRecordFileName("FileInfo begin")

	var existed bool
	var fileSize int64
	var modifyTime int64
	var fileMode uint32
	if cl.preprocessedBuffer != nil {
		fileSize = int64(len(cl.preprocessedBuffer))
		modifyTime = 0
		fileMode = uint32(os.ModePerm)
	} else {
		existed, fileSize, modifyTime, fileMode = dcFile.Stat(cl.preprocessedFile).Batch()
		if !existed {
			blog.Errorf("cl: input pre file %s not existed", cl.preprocessedFile)
			return nil, dcType.BKDistCommonError{
				Code:  dcType.UnknowCode,
				Error: fmt.Errorf("%s not existed", cl.preprocessedFile),
			}
		}
	}

	// generate the input files for pre-process file
	inputFiles := []dcSDK.FileDesc{{
		FilePath:           cl.preprocessedFile,
		Compresstype:       protocol.CompressLZ4,
		FileSize:           fileSize,
		Lastmodifytime:     modifyTime,
		Md5:                "",
		Filemode:           fileMode,
		Buffer:             cl.preprocessedBuffer,
		Targetrelativepath: filepath.Dir(cl.preprocessedFile),
	}}

	// if there is a pch file, add it into the inputFiles, it should be also sent to remote
	if cl.pchFileDesc != nil {
		inputFiles = append(inputFiles, *cl.pchFileDesc)
	}

	// debugRecordFileName(fmt.Sprintf("cl: success done pre execute for: %v", command))

	blog.Infof("cl: success done pre execute for: %v", command)
	blog.Infof("cl: going to execute compile: %v", cl.serverSideArgs)

	// to check whether need to compile with response file
	exeName := cl.serverSideArgs[0]
	params := cl.serverSideArgs[1:]
	flag, rspfile, err := cl.needSaveResponseFile(cl.serverSideArgs)
	if flag && err == nil {
		cl.addTmpFile(rspfile)
		params = []string{fmt.Sprintf("@%s", rspfile)}
		existed, fileSize, modifyTime, fileMode := dcFile.Stat(rspfile).Batch()
		if !existed {
			blog.Errorf("cl: input response file %s not existed", rspfile)
			return nil, dcType.BKDistCommonError{
				Code:  dcType.UnknowCode,
				Error: fmt.Errorf("%s not existed", rspfile),
			}
		}
		inputFiles = append(inputFiles, dcSDK.FileDesc{
			FilePath:       rspfile,
			Compresstype:   protocol.CompressLZ4,
			FileSize:       fileSize,
			Lastmodifytime: modifyTime,
			Md5:            "",
			Filemode:       fileMode,
		})
	}

	cl.customSave = true
	results := []string{cl.outputFile}
	if cl.logfilesarif != "" {
		results = append(results, cl.logfilesarif)
	}

	return &dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir:         "",
				ExePath:         "",
				ExeName:         exeName,
				ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
				Params:          params,
				Inputfiles:      inputFiles,
				ResultFiles:     results,
			},
		},
		CustomSave: true,
	}, dcType.ErrorNone
}

func (cl *TaskCL) postExecute(r *dcSDK.BKDistResult, byclfilter bool) dcType.BKDistCommonError {
	blog.Infof("cl: start post execute for: %v", cl.originArgs)

	resultfilenum := 0
	var dealError error

	if r == nil || len(r.Results) == 0 {
		// blog.Warnf("cl: parameter is invalid")
		// return dcType.BKDistCommonError{
		// 	Code:  dcType.UnknowCode,
		// 	Error: fmt.Errorf("parameter is invalid"),
		// }
		goto ERROREND
	}

	// resultfilenum := 0
	// by tomtian 20201224,to ensure existed result file
	if len(r.Results[0].ResultFiles) == 0 {
		blog.Warnf("cl: not found result file for: %v", cl.originArgs)
		goto ERROREND
	}
	blog.Infof("cl: found %d result files for result[0]", len(r.Results[0].ResultFiles))

	// resultfilenum := 0
	if len(r.Results[0].ResultFiles) > 0 {
		for _, f := range r.Results[0].ResultFiles {
			if f.Buffer != nil {
				if err := saveResultFile(&f, cl.sandbox.Dir); err != nil {
					blog.Errorf("cl: failed to save file [%s]", f.FilePath)
					// return dcType.BKDistCommonError{
					// 	Code:  dcType.UnknowCode,
					// 	Error: err,
					// }
					dealError = err
					goto ERROREND
				}
				resultfilenum++
			}
		}
	}

	// by tomtian 20201224,to ensure existed result file
	if resultfilenum == 0 && cl.customSave {
		blog.Warnf("cl: not found result file for: %v", cl.originArgs)
		goto ERROREND
	}

	blog.Infof("cl: output [%s] errormessage [%s]", r.Results[0].OutputMessage, r.Results[0].ErrorMessage)

	if r.Results[0].RetCode == 0 {
		blog.Infof("cl: success done post execute for: %v", cl.originArgs)
		if cl.showinclude {
			// if !dcPump.SupportPump(cl.sandbox.Env) {
			if cl.preprocessedErrorBuf != "" {
				// simulate output with preprocessed error output
				r.Results[0].OutputMessage = []byte(cl.preprocessedErrorBuf)
			}
		} else if cl.forcedepend {
			if cl.preprocessedErrorBuf != "" {
				cl.parseOutput(cl.preprocessedErrorBuf)
			}
		}

		// simulate output with inputFile
		if !byclfilter {
			r.Results[0].OutputMessage = []byte(filepath.Base(cl.inputFile))
		}

		// if remote succeed with pump,do not need copy head file
		if cl.pumpremote {
			cl.needcopypumpheadfile = false
		}

		return dcType.ErrorNone
	}

ERROREND:
	// 如果预处理模式下远程失败，则提前生成pump的依赖文件
	// 因为默认本地命令生成的依赖文件不全
	if !cl.pumpremote && cl.needcopypumpheadfile {
		if cl.forcedepend && cl.preprocessedErrorBuf != "" {
			cl.parseOutput(cl.preprocessedErrorBuf)
		}

		cl.copyPumpHeadFile(cl.sandbox.Dir)
		cl.needcopypumpheadfile = false
	}

	if r == nil || len(r.Results) == 0 {
		blog.Warnf("cl: parameter is invalid")
		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: fmt.Errorf("parameter is invalid"),
		}
	}

	if dealError != nil {
		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: dealError,
		}
	}

	// write error message into
	if cl.saveTemp() && len(r.Results[0].ErrorMessage) > 0 {
		// make the tmp file for storing the stderr from server compiler.
		stderrFile, err := makeTmpFile(commonUtil.GetHandlerTmpDir(cl.sandbox), "cl_server_stderr", ".txt")
		if err != nil {
			blog.Warnf("cl: make tmp file for stderr from server failed: %v", err)
		} else {
			if f, err := os.OpenFile(stderrFile, os.O_RDWR, 0644); err == nil {
				_, _ = f.Write(r.Results[0].ErrorMessage)
				_ = f.Close()
				blog.Debugf("cl: save error message to %s for: %s", stderrFile, cl.originArgs)
			}
		}
	}

	if cl.pumpremote {
		blog.Infof("cl: ready remove pump head file: %s after failed pump remote, generate it next time",
			cl.pumpHeadFile)
		os.Remove(cl.pumpHeadFile)
	}

	blog.Warnf("cl: failed to remote execute, retcode %d, error message:%s, output message:%s",
		r.Results[0].RetCode,
		r.Results[0].ErrorMessage,
		r.Results[0].OutputMessage)

	return dcType.BKDistCommonError{
		Code:  dcType.UnknowCode,
		Error: fmt.Errorf(string(r.Results[0].ErrorMessage)),
	}
}

func (cl *TaskCL) finalExecute([]string) {
	go func() {
		if cl.needcopypumpheadfile {
			cl.copyPumpHeadFile(cl.sandbox.Dir)
		}

		if cl.saveTemp() {
			return
		}

		cl.cleanTmpFile()
	}()
}

func (cl *TaskCL) saveTemp() bool {
	return commonUtil.GetHandlerEnv(cl.sandbox, envSaveTempFile) != ""
}

func (cl *TaskCL) preBuild(args []string) error {
	blog.Debugf("cl: pre-build begin got args: %v", args)

	var err error
	cl.expandArgs = args

	// scan the args, check if it can be compiled remotely, wrap some un-used options,
	// and get the real input&output file.
	scannedData, err := scanArgs(cl.expandArgs)
	if err != nil {
		// blog.Warnf("cl: pre-build not support, scan args %v: %v", cl.expandArgs, err)
		return err
	}
	cl.scannedArgs = scannedData.args
	// ++ by tomtian 20201126, pch has no effect for compile
	// cl.firstIncludeFile = getFirstIncludeFile(scannedData.args)
	cl.firstIncludeFile = ""
	// --
	cl.inputFile = scannedData.inputFile
	cl.outputFile = scannedData.outputFile
	cl.rewriteCrossArgs = cl.scannedArgs
	cl.includeRspFiles = scannedData.includeRspFiles
	cl.includePaths = scannedData.includePaths
	cl.logfilesarif = scannedData.logfilesarif

	// handle the pch options
	finalArgs := cl.scanPchFile(cl.scannedArgs)

	// disable some warning here
	for _, v := range DisabledWarnings {
		finalArgs = append(finalArgs, v)
	}

	// do the pre-process, store result in the file.
	if cl.preprocessedFile, cl.preprocessedBuffer, err = cl.doPreProcess(finalArgs, cl.inputFile); err != nil {
		blog.Errorf("cl: pre-build error, do pre-process %v: %v", finalArgs, err)
		return err
	}

	// strip the args and get the server side args.
	serverSideArgs := stripLocalArgs(finalArgs)

	// replace the input file into preprocessedFile, for the next server side process.
	for index := range serverSideArgs {
		if serverSideArgs[index] == cl.inputFile {
			serverSideArgs[index] = cl.preprocessedFile
			break
		}
	}

	if !scannedData.specifiedSourceType {
		if filepath.Ext(cl.preprocessedFile) == ".ii" {
			serverSideArgs = append(serverSideArgs, "/TP")
		} else if filepath.Ext(cl.preprocessedFile) == ".i" {
			serverSideArgs = append(serverSideArgs, "/TC")
		}
	}

	// quota result file if it's path contains space
	if runtime.GOOS == osWindows {
		if hasSpace(cl.outputFile) && !strings.HasPrefix(cl.outputFile, "\"") {
			for index := range serverSideArgs {
				if strings.HasPrefix(serverSideArgs[index], "/Fo") {
					if len(serverSideArgs[index]) > 3 {
						serverSideArgs[index] = fmt.Sprintf("/Fo\"%s\"", cl.outputFile)
					} else {
						index++
						if index < len(serverSideArgs) {
							serverSideArgs[index] = fmt.Sprintf("\"%s\"", cl.outputFile)
						}
					}
					break
				}
			}
		}
	}

	cl.serverSideArgs = serverSideArgs

	if cl.SupportResultCache(args) != resultcache.CacheTypeNone {
		cl.resultCacheArgs = make([]string, len(cl.serverSideArgs))
		copy(cl.resultCacheArgs, cl.serverSideArgs)
		for index := range cl.resultCacheArgs {
			if cl.resultCacheArgs[index] == cl.preprocessedFile {
				cl.resultCacheArgs[index] = cl.inputFile
				break
			}
		}
	}

	blog.Infof("cl: pre-build success for enter args: %v", args)
	return nil
}

func (cl *TaskCL) addTmpFile(filename string) {
	cl.tmpFileList = append(cl.tmpFileList, filename)
}

func (cl *TaskCL) cleanTmpFile() {
	for _, filename := range cl.tmpFileList {
		if err := os.Remove(filename); err != nil {
			blog.Debugf("cl: clean tmp file %s failed: %v", filename, err)
		}
	}
}

func formatArg(arg string) string {
	if arg != "" && strings.HasPrefix(arg, "\"") && strings.HasSuffix(arg, "\"") {
		return strings.Trim(arg, "\"")
	}

	return arg
}

// If the input filename is a plain source file rather than a
// preprocessed source file, then preprocess it to a temporary file
// and return the name.
//
// The preprocessor may still be running when we return; you have to
// wait for cpp_pid to exit before the output is complete.  This
// allows us to overlap opening the TCP socket, which probably doesn't
// use many cycles, with running the preprocessor.
func (cl *TaskCL) doPreProcess(args []string, inputFile string) (string, []byte, error) {
	if isPreprocessedFile(inputFile) {
		blog.Infof("cl: input \"%s\" is already preprocessed", inputFile)

		// input file already preprocessed
		return inputFile, nil, nil
	}

	// to check whether need save to memroy
	savetomemroy := false
	if cl.sandbox.Env.IsSet(dcEnv.KeyExecutorWriteMemory) {
		savetomemroy = true
		blog.Infof("cl: ready save processed file to memory")
	}

	outputExt := getPreprocessedExt(inputFile)
	var err error
	outputFile := ""
	if !savetomemroy {
		outputFile, err = makeTmpFile(commonUtil.GetHandlerTmpDir(cl.sandbox), "cl", outputExt)
		if err != nil {
			blog.Errorf("cl: do pre-process get output file failed: %v", err)
			return "", nil, err
		}
		cl.addTmpFile(outputFile)
	} else {
		outputFile = makeTmpFileName(commonUtil.GetHandlerTmpDir(cl.sandbox), "cl", outputExt)
	}

	newArgs, err := setActionOptionE(stripDashO(args))
	if err != nil {
		blog.Warnf("cl: set action option /E : %v", err)
		return "", nil, err
	}

	tempArgs := []string{newArgs[0]}
	if len(newArgs) > 1 {
		for _, v := range newArgs[1:] {
			tempArgs = append(tempArgs, formatArg(v))
		}
	}
	newArgs = tempArgs

	cl.preProcessArgs = newArgs

	var execName string
	var execArgs []string
	flag, rspfile, err := cl.needSaveResponseFile(newArgs)
	if flag && err == nil {
		rspargs := []string{newArgs[0], fmt.Sprintf("@%s", rspfile)}
		blog.Infof("cl: going to execute pre-process: %s", strings.Join(rspargs, " "))
		execName = rspargs[0]
		execArgs = rspargs[1:]
		cl.addTmpFile(rspfile)
	} else {
		blog.Infof("cl: going to execute pre-process: %s", strings.Join(newArgs, " "))
		execName = newArgs[0]
		execArgs = newArgs[1:]
	}

	sandbox := cl.sandbox.Fork()
	var outBuf bytes.Buffer

	if !savetomemroy {
		output, err := os.OpenFile(outputFile, os.O_WRONLY, 0666)
		if err != nil {
			blog.Errorf("cl: failed to open output file \"%s\" when pre-processing: %v", outputFile, err)
			return "", nil, err
		}
		defer func() {
			_ = output.Close()
		}()

		sandbox.Stdout = output
	} else {
		sandbox.Stdout = &outBuf
	}

	var errBuf bytes.Buffer
	sandbox.Stderr = &errBuf

	if _, err = sandbox.ExecCommand(execName, execArgs...); err != nil {
		blog.Warnf("cl: failed to do pre-process %s: %v, %s",
			strings.Join(newArgs, " "), err, errBuf.String())
		return "", nil, err
	}
	blog.Infof("cl: success to execute pre-process and get %s: %s", outputFile, strings.Join(newArgs, " "))

	// if cl.showinclude || cl.forcedepend {
	cl.preprocessedErrorBuf = errBuf.String()
	// }

	if !savetomemroy {
		return outputFile, nil, nil
	}

	return outputFile, outBuf.Bytes(), nil
}

// TODO : not ok for windows pch
func (cl *TaskCL) scanPchFile(args []string) []string {
	//  ++ by tomtian, do not send pch files, it has no effect for compile
	return args
	// --

	//if cl.firstIncludeFile == "" {
	//	return args
	//}
	//
	//filename := cl.firstIncludeFile
	//if !strings.HasSuffix(cl.firstIncludeFile, ".pch") {
	//	filename = fmt.Sprintf("%s.pch", cl.firstIncludeFile)
	//}
	//
	//existed, fileSize, modifyTime, fileMode := dcFile.Stat(filename).Batch()
	//if !existed {
	//	blog.Debugf("cl: try to get pch file for %s but %s is not exist", cl.firstIncludeFile, filename)
	//	return args
	//}
	//cl.pchFile = filename
	//
	//blog.Debugf("cl: success to find pch file %s for %s", filename, cl.firstIncludeFile)
	//
	//cl.pchFileDesc = &dcSDK.FileDesc{
	//	FilePath:       filename,
	//	Compresstype:   protocol.CompressLZ4,
	//	FileSize:       fileSize,
	//	Lastmodifytime: modifyTime,
	//	Md5:            "",
	//	Filemode:       fileMode,
	//}

	// for _, arg := range args {
	// 	if arg == pchPreProcessOption {
	// 		return args
	// 	}
	// }

	// return append(args, pchPreProcessOption)
}

func (cl *TaskCL) needSaveResponseFile(args []string) (bool, string, error) {
	exe := args[0]
	if strings.HasSuffix(exe, "cl.exe") {
		if len(args) > 1 {
			fullArgs := MakeCmdLine(args[1:])
			if len(fullArgs) >= MaxWindowsCommandLength {
				rspFile, err := makeTmpFile(commonUtil.GetHandlerTmpDir(cl.sandbox), "cl", ".response")
				if err != nil {
					return false, "", fmt.Errorf("cl: cmd too long and failed to create rsp file")
				}

				err = ioutil.WriteFile(rspFile, []byte(fullArgs), os.ModePerm)
				if err != nil {
					blog.Errorf("cl: failed to write data to file[%s], error:%v", rspFile, err)
					return false, "", err
				}

				return true, rspFile, nil
			}
		}
	}

	return false, "", nil
}

// copied from clfilter handle
func (cl *TaskCL) parseOutput(s string) (string, error) {
	blog.Debugf("cl: start parse output: %s", s)

	output := make([]string, 0, 0)
	includes := make([]string, 0, 0)

	reader := bufio.NewReader(strings.NewReader(s))
	var line string
	var err error
	for {
		line, err = reader.ReadString('\n')
		if err != nil && err != io.EOF {
			break
		}

		// Process the line here.
		// Note: including file: Runtime\Core\Public\HAL/ThreadHeartBeat.h
		// Note: including file:     C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Tools\MSVC\14.31.31103\INCLUDE\sal.h
		columns := strings.Split(line, ":")
		if len(columns) == 3 {
			includefile := strings.Trim(columns[2], " \r\n")
			if !filepath.IsAbs(includefile) {
				includefile, _ = filepath.Abs(filepath.Join(cl.sandbox.Dir, includefile))
			}
			existed, _, _, _ := dcFile.Stat(includefile).Batch()
			if existed {
				includes = append(includes, includefile)
			} else {
				blog.Infof("cl: includefile [%s] not existed", includefile)
				output = append(output, line)
			}
		} else if len(columns) == 4 {
			includefile := columns[2] + ":" + columns[3]
			includefile = strings.Trim(includefile, " \r\n")
			existed, _, _, _ := dcFile.Stat(includefile).Batch()
			if existed {
				includes = append(includes, includefile)
			} else {
				blog.Infof("cl: includefile [%s] not existed", includefile)
				output = append(output, line)
			}
		} else {
			output = append(output, line)
		}

		if err != nil {
			break
		}
	}
	blog.Debugf("cl: got output: [%v], includes:[%v]", output, includes)

	// save includes to cl.sourcedependfile
	if len(includes) > 0 {
		f, err := os.Create(cl.sourcedependfile)
		if err != nil {
			blog.Errorf("cl: create file %s error: [%s]", cl.sourcedependfile, err.Error())
		} else {
			defer func() {
				_ = f.Close()
			}()
			_, err := f.Write([]byte(strings.Join(includes, "\r\n")))
			if err != nil {
				blog.Errorf("cl: save depend file [%s] error: [%s]", cl.sourcedependfile, err.Error())
				return strings.Join(output, "\n"), err
			}
		}
	}

	return strings.Join(output, ""), nil
}

// SupportResultCache check whether this command support result cache
func (cl *TaskCL) SupportResultCache(command []string) int {
	if cl.sandbox != nil {
		if str := cl.sandbox.Env.GetEnv(dcEnv.KeyExecutorResultCacheType); str != "" {
			i, err := strconv.Atoi(str)
			if err == nil {
				return i
			}
		}
	}

	return 0
}

// hasResultIndex check whether the env of hasresultindex set
func (cl *TaskCL) hasResultIndex() bool {
	return cl.sandbox.Env.GetEnv(dcEnv.KeyExecutorHasResultIndex) != ""
}

func (cl *TaskCL) GetResultCacheKey(command []string) string {
	if cl.preprocessedFile == "" {
		blog.Infof("cl: cl.preprocessedFile is null , no need to get result cache key")
		return ""
	}
	if !dcFile.Stat(cl.preprocessedFile).Exist() {
		blog.Warnf("cl: cl.preprocessedFile %s not existed when get result cache key", cl.preprocessedFile)
		return ""
	}

	// ext from cl.preprocessedFile
	ext := filepath.Ext(cl.preprocessedFile)

	// cc_mtime cc_name from compile tool
	cchash, err := dcUtil.HashFile(command[0])
	if err != nil {
		blog.Warnf("cl: hash file %s with error: %v", command[0], err)
		return ""
	}

	// LANG and LC_ALL  from env , ignore in windows now
	// cwd  from work dir , ignore now

	// arg from cl.resultCacheArgs
	argstring := strings.Join(cl.resultCacheArgs, " ")
	arghash := xxhash.Sum64([]byte(argstring))

	// cpp content from cl.preprocessedFile
	cpphash, err := dcUtil.HashFile(cl.preprocessedFile)
	if err != nil {
		blog.Warnf("cl: hash file %s with error: %v", cl.preprocessedFile, err)
		return ""
	}

	// cppstderr from cl.preprocessedErrorBuf
	cppstderrhash := xxhash.Sum64([]byte(cl.preprocessedErrorBuf))

	fullstring := fmt.Sprintf("%s_%x_%x_%x_%x", ext, cchash, arghash, cpphash, cppstderrhash)
	fullstringhash := xxhash.Sum64([]byte(fullstring))

	blog.Infof("cl: got hash key %x for string[%s] cmd:[%s]",
		fullstringhash, fullstring, strings.Join(command, " "))

	return fmt.Sprintf("%x", fullstringhash)
}
