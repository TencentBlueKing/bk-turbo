/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package cc

import (
	// "bytes"

	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	dcEnv "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcPump "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/pump"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/resultcache"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	commonUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/handler/common"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/cespare/xxhash"
)

const (
	MaxWindowsCommandLength = 30000

	appendEnvKey = "INCLUDE="
	osWindows    = "windows"
)

var (
	// ForceLocalFileKeys force some module to compile locally
	// ForceLocalFileKeys = []string{}

	DefaultForceLocalResponseFileKeys = make([]string, 0, 0)
	// DefaultForceLocalCppFileKeys force some cpp to compile locally
	DefaultForceLocalCppFileKeys = make([]string, 0, 0)
)

// TaskCC 定义了c/c++编译的描述处理对象, 一般用来处理ue4-mac下的clang编译
type TaskCC struct {
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
	// 在rsp中-I后面的参数，需要将这些目录全部发送到远端
	// 有特殊场景：编译不需要该路径下的文件，但需要该路径作为跳板，去查找其它相对路径下的头文件（或其它依赖文件）
	includePaths []string
	// 在rsp中 -include后面的参数，将这些文件发送到远端
	includeFiles []string

	// forcedepend 是我们主动导出依赖文件，showinclude 是编译命令已经指定了导出依赖文件
	forcedepend          bool
	pumpremote           bool
	needcopypumpheadfile bool
	pumpremotefailed     bool

	// 当前是pch/gch的生成，pump模式下我们需要关注并保存其依赖关系
	// 方便在后续pump模式下，发送完整的pch/gch列表
	needresolvepchdepend bool
	objectpchfile        string

	pchFileDesc *dcSDK.FileDesc

	// for /showIncludes
	showinclude          bool
	preprocessedErrorBuf string

	ForceLocalResponseFileKeys []string
	ForceLocalCppFileKeys      []string
}

// NewTaskCC get a new task-cc handler
func NewTaskCC() *TaskCC {
	key1 := make([]string, len(DefaultForceLocalResponseFileKeys))
	copy(key1, DefaultForceLocalResponseFileKeys)

	key2 := make([]string, len(DefaultForceLocalCppFileKeys))
	copy(key2, DefaultForceLocalCppFileKeys)

	return &TaskCC{
		sandbox:                    &dcSyscall.Sandbox{},
		tmpFileList:                make([]string, 0, 10),
		ForceLocalResponseFileKeys: key1,
		ForceLocalCppFileKeys:      key2,
	}
}

// GetPreprocessedBuf return preprocessedErrorBuf
func (cc *TaskCC) GetPreprocessedBuf() string {
	return cc.preprocessedErrorBuf
}

// InitSandbox set sandbox to task-cc
func (cc *TaskCC) InitSandbox(sandbox *dcSyscall.Sandbox) {
	cc.sandbox = sandbox
}

func (cc *TaskCC) CanExecuteWithLocalIdleResource(command []string) bool {
	if cc.sandbox.Env.GetEnv(dcEnv.KeyExecutorUECCNotUseLocal) == "true" {
		return false
	}

	return true
}

// PreExecuteNeedLock 需要pre-lock来保证预处理不会跑满本地资源
func (cc *TaskCC) PreExecuteNeedLock(command []string) bool {
	return true
}

// PostExecuteNeedLock 不需要post-lock
func (cc *TaskCC) PostExecuteNeedLock(result *dcSDK.BKDistResult) bool {
	return false
}

// PreLockWeight decide pre-execute lock weight, default 1
func (cc *TaskCC) PreLockWeight(command []string) int32 {
	return 1
}

// PreExecute 预处理
func (cc *TaskCC) PreExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	return cc.preExecute(command)
}

// LocalExecuteNeed 无需自定义本地处理
func (cc *TaskCC) LocalExecuteNeed(command []string) bool {
	return false
}

// LocalLockWeight decide local-execute lock weight, default 1
func (cc *TaskCC) LocalLockWeight(command []string) int32 {
	envvalue := cc.sandbox.Env.GetEnv(dcEnv.KeyExecutorUECCLocalCPUWeight)
	if envvalue != "" {
		w, err := strconv.Atoi(envvalue)
		if err == nil && w > 0 && w <= runtime.NumCPU() {
			return int32(w)
		}
	}

	return 1
}

// LocalExecute 无需自定义本地处理
func (cc *TaskCC) LocalExecute(command []string) dcType.BKDistCommonError {
	return dcType.ErrorNone
}

// NeedRemoteResource check whether this command need remote resource
func (cc *TaskCC) NeedRemoteResource(command []string) bool {
	return true
}

// RemoteRetryTimes will return the remote retry times
func (cc *TaskCC) RemoteRetryTimes() int {
	return 1
}

// NeedRetryOnRemoteFail check whether need retry on remote fail
func (cc *TaskCC) NeedRetryOnRemoteFail(command []string) bool {
	return cc.pumpremote
}

// TODO : OnRemoteFail give chance to try other way if failed to remote execute
func (cc *TaskCC) OnRemoteFail(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("cc: start OnRemoteFail for: %v", command)

	if cc.pumpremote {
		blog.Infof("cc: set pumpremotefailed to true now")
		cc.pumpremotefailed = true
		cc.needcopypumpheadfile = true
		cc.pumpremote = false
		return cc.preExecute(command)
	}

	return nil, dcType.ErrorNone
}

// PostLockWeight decide post-execute lock weight, default 1
func (cc *TaskCC) PostLockWeight(result *dcSDK.BKDistResult) int32 {
	return 1
}

// PostExecute 后置处理, 判断远程执行的结果是否正确
func (cc *TaskCC) PostExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	return cc.postExecute(r)
}

// FinalExecute 清理临时文件
func (cc *TaskCC) FinalExecute(args []string) {
	cc.finalExecute(args)
}

// GetFilterRules add file send filter
func (cc *TaskCC) GetFilterRules() ([]dcSDK.FilterRuleItem, error) {
	// return []dcSDK.FilterRuleItem{
	// 	{
	// 		Rule:       dcSDK.FilterRuleFileSuffix,
	// 		Operator:   dcSDK.FilterRuleOperatorEqual,
	// 		Standard:   ".gch",
	// 		HandleType: dcSDK.FilterRuleHandleAllDistribution,
	// 	},
	// }, nil
	return nil, nil
}

func (cc *TaskCC) getIncludeExe() (string, error) {
	blog.Debugf("cc: ready get include exe")

	target := "bk-includes"
	if runtime.GOOS == osWindows {
		target = "bk-includes.exe"
	}

	includePath, err := dcUtil.CheckExecutable(target)
	if err != nil {
		// blog.Infof("cc: not found exe file with default path, info: %v", err)

		includePath, err = dcUtil.CheckFileWithCallerPath(target)
		if err != nil {
			blog.Errorf("cc: not found exe file with error: %v", err)
			return includePath, err
		}
	}
	absPath, err := filepath.Abs(includePath)
	if err == nil {
		includePath = absPath
	}
	includePath = dcUtil.QuoteSpacePath(includePath)
	// blog.Infof("cc: got include exe file full path: %s", includePath)

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

func (cc *TaskCC) analyzeIncludes(dependf string, workdir string) ([]*dcFile.Info, error) {
	data, err := ioutil.ReadFile(dependf)
	if err != nil {
		return nil, err
	}

	sep := "\n"
	if runtime.GOOS == osWindows {
		sep = "\r\n"
	}
	lines := strings.Split(string(data), sep)
	uniqlines := dcUtil.UniqArr(lines)
	blog.Infof("cc: got %d uniq include file from file: %s", len(uniqlines), dependf)

	return dcFile.GetFileInfo(uniqlines, false, false, dcPump.SupportPumpLstatByDir(cc.sandbox.Env))
}

func (cc *TaskCC) checkFstat(f string, workdir string) (*dcFile.Info, error) {
	if !filepath.IsAbs(f) {
		f, _ = filepath.Abs(filepath.Join(workdir, f))
	}
	fstat := dcFile.Stat(f)
	if fstat.Exist() && !fstat.Basic().IsDir() {
		return fstat, nil
	}

	return nil, nil
}

// func formatFilePath(f string) string {
// 	f = strings.Replace(f, "\\", "/", -1)
// 	f = strings.Replace(f, "//", "/", -1)

// 	// 去掉路径中的..
// 	if strings.Contains(f, "..") {
// 		p := strings.Split(f, "/")

// 		var newPath []string
// 		for _, v := range p {
// 			if v == ".." {
// 				newPath = newPath[:len(newPath)-1]
// 			} else {
// 				newPath = append(newPath, v)
// 			}
// 		}
// 		f = strings.Join(newPath, "/")
// 	}

// 	return f
// }

func (cc *TaskCC) resolveDependFile(sep, workdir string, includes *[]string) error {
	data, err := os.ReadFile(cc.sourcedependfile)
	if err != nil {
		blog.Warnf("cc: copy pump head failed to read depned file: %s with err:%v", cc.sourcedependfile, err)
		return err
	}

	// sep := "\n"
	// if runtime.GOOS == osWindows {
	// 	sep = "\r\n"
	// }
	lines := strings.Split(string(data), sep)
	// includes := []string{}
	for _, l := range lines {
		l = strings.Trim(l, " \r\n\\")
		// TODO : the file path maybe contains space, should support this condition
		fields := strings.Split(l, " ")
		if len(fields) >= 1 {
			// for i, f := range fields {
			for index := len(fields) - 1; index >= 0; index-- {
				var targetf string
				// /xx/xx/aa .cpp /xx/xx/aa .h  /xx/xx/aa .o
				// 支持文件名后缀前有空格的情况，但没有支持路径中间有空格的，比如 /xx /xx /aa.cpp
				// 向前依附到不为空的字符串为止
				if fields[index] == ".cpp" || fields[index] == ".h" || fields[index] == ".o" {
					for targetindex := index - 1; targetindex >= 0; targetindex-- {
						if len(fields[targetindex]) > 0 {
							fields[targetindex] = strings.Trim(fields[targetindex], "\\")
							targetf = strings.Join(fields[targetindex:index+1], " ")
							index = targetindex
							break
						}
					}
				} else if len(fields[index]) > 0 {
					targetf = fields[index]
				} else {
					continue
				}

				if strings.HasSuffix(targetf, ":") {
					continue
				}
				if !filepath.IsAbs(targetf) {
					targetf, _ = filepath.Abs(filepath.Join(workdir, targetf))
				}

				*includes = append(*includes, dcUtil.FormatFilePath(targetf))

				// 如果是链接，则将相关指向的文件都包含进来
				fs := dcUtil.GetAllLinkFiles(targetf)
				if len(fs) > 0 {
					*includes = append(*includes, fs...)
				}
			}
		}
	}

	return nil
}

func (cc *TaskCC) copyPumpHeadFile(workdir string) error {
	blog.Infof("cc: copy pump head file: %s to: %s", cc.sourcedependfile, cc.pumpHeadFile)

	sep := "\n"
	if runtime.GOOS == osWindows {
		sep = "\r\n"
	}

	includes := []string{}
	err := cc.resolveDependFile(sep, workdir, &includes)
	if err != nil {
		return err
	}

	// data, err := ioutil.ReadFile(cc.sourcedependfile)
	// if err != nil {
	// 	blog.Warnf("cc: copy pump head failed to read depned file: %s with err:%v", cc.sourcedependfile, err)
	// 	return err
	// }

	// lines := strings.Split(string(data), sep)
	// // includes := []string{}
	// for _, l := range lines {
	// 	l = strings.Trim(l, " \r\n\\")
	// 	// TODO : the file path maybe contains space, should support this condition
	// 	fields := strings.Split(l, " ")
	// 	if len(fields) >= 1 {
	// 		// for i, f := range fields {
	// 		for index := len(fields) - 1; index >= 0; index-- {
	// 			var targetf string
	// 			// /xx/xx/aa .cpp /xx/xx/aa .h  /xx/xx/aa .o
	// 			// 支持文件名后缀前有空格的情况，但没有支持路径中间有空格的，比如 /xx /xx /aa.cpp
	// 			// 向前依附到不为空的字符串为止
	// 			if fields[index] == ".cpp" || fields[index] == ".h" || fields[index] == ".o" {
	// 				for targetindex := index - 1; targetindex >= 0; targetindex-- {
	// 					if len(fields[targetindex]) > 0 {
	// 						fields[targetindex] = strings.Trim(fields[targetindex], "\\")
	// 						targetf = strings.Join(fields[targetindex:index+1], " ")
	// 						index = targetindex
	// 						break
	// 					}
	// 				}
	// 			} else if len(fields[index]) > 0 {
	// 				targetf = fields[index]
	// 			} else {
	// 				continue
	// 			}

	// 			if strings.HasSuffix(targetf, ".o:") {
	// 				continue
	// 			}
	// 			if !filepath.IsAbs(targetf) {
	// 				targetf, _ = filepath.Abs(filepath.Join(workdir, targetf))
	// 			}

	// 			includes = append(includes, dcUtil.FormatFilePath(targetf))
	// 		}
	// 	}
	// }

	// copy includeRspFiles
	if len(cc.includeRspFiles) > 0 {
		for _, l := range cc.includeRspFiles {
			blog.Infof("cc: ready add rsp file: %s", l)
			if !filepath.IsAbs(l) {
				l, _ = filepath.Abs(filepath.Join(workdir, l))
			}
			includes = append(includes, dcUtil.FormatFilePath(l))
		}
	}

	// copy includePaths
	if len(cc.includePaths) > 0 {
		for _, l := range cc.includePaths {
			blog.Infof("cc: ready add include path: %s", l)
			if !filepath.IsAbs(l) {
				l, _ = filepath.Abs(filepath.Join(workdir, l))
			}
			includes = append(includes, dcUtil.FormatFilePath(l))
		}
	}

	// copy include files
	if len(cc.includeFiles) > 0 {
		for _, l := range cc.includeFiles {
			blog.Infof("cc: ready add include file: %s", l)
			if !filepath.IsAbs(l) {
				l, _ = filepath.Abs(filepath.Join(workdir, l))
			}
			includes = append(includes, dcUtil.FormatFilePath(l))
		}
	}

	blog.Infof("cc: copy pump head got %d uniq include file from file: %s", len(includes), cc.sourcedependfile)

	if len(includes) == 0 {
		blog.Warnf("cc: depend file: %s is invalid", cc.sourcedependfile)
		return ErrorInvalidDependFile
	}

	// for i := range includes {
	// 	includes[i] = strings.Replace(includes[i], "\\", "/", -1)
	// }
	uniqlines := dcUtil.UniqArr(includes)

	// TODO : append symlink or symlinked if need
	links, _ := getIncludeLinks(cc.sandbox.Env, uniqlines)
	if links != nil {
		uniqlines = append(uniqlines, links...)
	}

	// TODO :将链接路径找出并放到前面
	linkdirs := dcUtil.GetAllLinkDir(uniqlines)
	if len(linkdirs) > 0 {
		uniqlines = append(linkdirs, uniqlines...)
	}

	// TODO : save to cc.pumpHeadFile
	newdata := strings.Join(uniqlines, sep)
	err = ioutil.WriteFile(cc.pumpHeadFile, []byte(newdata), os.ModePerm)
	if err != nil {
		blog.Warnf("cc: copy pump head failed to write file: %s with err:%v", cc.pumpHeadFile, err)
		return err
	} else {
		blog.Infof("cc: copy pump head succeed to write file: %s", cc.pumpHeadFile)
	}

	return nil
}

func (cc *TaskCC) getPumpDir(env *dcEnv.Sandbox) (string, error) {
	pumpdir := dcPump.PumpCacheDir(env)
	if pumpdir == "" {
		pumpdir = dcUtil.GetPumpCacheDir()
	}

	if !dcFile.Stat(pumpdir).Exist() {
		if err := os.MkdirAll(pumpdir, os.ModePerm); err != nil {
			return "", err
		}
	}

	return pumpdir, nil
}

// search all include files for this compile command
func (cc *TaskCC) Includes(responseFile string, args []string, workdir string, forcefresh bool) ([]*dcFile.Info, error) {
	// pumpdir := dcPump.PumpCacheDir(cc.sandbox.Env)
	// if pumpdir == "" {
	// 	pumpdir = dcUtil.GetPumpCacheDir()
	// }

	// if !dcFile.Stat(pumpdir).Exist() {
	// 	if err := os.MkdirAll(pumpdir, os.ModePerm); err != nil {
	// 		return nil, err
	// 	}
	// }

	pumpdir, err := cc.getPumpDir(cc.sandbox.Env)
	if err != nil {
		return nil, err
	}

	// TOOD : maybe we should pass responseFile to calc md5, to ensure unique
	// var err error
	cc.pumpHeadFile, err = getPumpIncludeFile(pumpdir, "pump_heads", ".txt", args, workdir)
	if err != nil {
		blog.Errorf("cc: do includes get output file failed: %v", err)
		return nil, err
	}

	existed, fileSize, _, _ := dcFile.Stat(cc.pumpHeadFile).Batch()
	if dcPump.IsPumpCache(cc.sandbox.Env) && !forcefresh && existed && fileSize > 0 {
		return cc.analyzeIncludes(cc.pumpHeadFile, workdir)
	}

	return nil, ErrorNoPumpHeadFile
}

func (cc *TaskCC) forceDepend(arg *ccArgs) error {
	if arg.hasDependencies {
		if len(arg.mfOutputFile) > 0 { // 已经指定了依赖文件
			dependfile := arg.mfOutputFile[0]
			if !filepath.IsAbs(dependfile) {
				dependfile, _ = filepath.Abs(filepath.Join(cc.sandbox.Dir, dependfile))
			}
			cc.sourcedependfile = dependfile
		} else {
			if arg.outputFile != "" { // 从 -o 得到依赖文件，替换后缀
				ext := filepath.Ext(arg.outputFile)
				withoutext := strings.TrimSuffix(arg.outputFile, ext)
				dependfile := withoutext + ".d"
				if !filepath.IsAbs(dependfile) {
					dependfile, _ = filepath.Abs(filepath.Join(cc.sandbox.Dir, dependfile))
				}
				cc.sourcedependfile = dependfile
			} else { // 从输入文件得到，只取文件名
				if arg.inputFile != "" {
					ext := filepath.Ext(arg.inputFile)
					base := filepath.Base(arg.inputFile)
					withoutext := strings.TrimSuffix(base, ext)
					dependfile := withoutext + ".d"
					if !filepath.IsAbs(dependfile) {
						dependfile, _ = filepath.Abs(filepath.Join(cc.sandbox.Dir, dependfile))
					}
					cc.sourcedependfile = dependfile
				}
			}
		}
	} else {
		cc.sourcedependfile = makeTmpFileName(commonUtil.GetHandlerTmpDir(cc.sandbox), "cc_depend", ".d")
		cc.sourcedependfile = strings.Replace(cc.sourcedependfile, "\\", "/", -1)
		cc.addTmpFile(cc.sourcedependfile)
	}

	if cc.sourcedependfile != "" {
		blog.Infof("cc: got depend file: %s", cc.sourcedependfile)
		cc.forcedepend = true
	} else {
		blog.Warnf("cc: failed to get depend file with scan arg:%v", *arg)
	}

	return nil
}

func (cc *TaskCC) inPumpBlack(responseFile string, args []string) (bool, error) {
	// obtain black key set by booster
	blackkeystr := cc.sandbox.Env.GetEnv(dcEnv.KeyExecutorPumpBlackKeys)
	if blackkeystr != "" {
		// blog.Infof("cc: got pump black key string: %s", blackkeystr)
		blacklist := strings.Split(blackkeystr, dcEnv.CommonBKEnvSepKey)
		if len(blacklist) > 0 {
			for _, v := range blacklist {
				if v != "" && strings.Contains(responseFile, v) {
					blog.Infof("cc: found response %s is in pump blacklist", responseFile)
					return true, nil
				}

				for _, v1 := range args {
					if strings.HasSuffix(v1, ".cpp") && strings.Contains(v1, v) {
						blog.Infof("cc: found arg %s is in pump blacklist", v1)
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

var (
	// 缓存单个pch文件的依赖列表
	pchDependHeadLock sync.RWMutex
	pchDependHead     map[string][]*dcFile.Info = make(map[string][]*dcFile.Info, 20)
)

func setPchDependHead(f string, heads []*dcFile.Info) {
	pchDependHeadLock.Lock()
	defer pchDependHeadLock.Unlock()

	pchDependHead[f] = heads
}

func getPchDependHead(f string) []*dcFile.Info {
	pchDependHeadLock.RLock()
	defer pchDependHeadLock.RUnlock()

	heads, ok := pchDependHead[f]
	if ok {
		return heads
	}

	return nil
}

func (cc *TaskCC) getPchDepends(fs []*dcFile.Info) ([]*dcFile.Info, error) {
	// 取出依赖的所有gch文件
	gchfiles := []string{}
	for _, v := range fs {
		if strings.HasSuffix(v.Path(), ".gch") {
			gchfiles = append(gchfiles, v.Path())
		}
	}

	if len(gchfiles) > 0 {
		blog.Infof("cc: got pch files:%v", gchfiles)

		gchdependfiles := []string{}
		// 再得到这些gch文件的依赖的其它gch文件
		for _, v := range gchfiles {
			dfs := dcPump.GetPchDepend(v)
			if len(dfs) > 0 {
				for _, v1 := range dfs {
					newfile := true
					for _, v2 := range gchdependfiles {
						if v1 == v2 {
							newfile = false
							break
						}
					}

					if newfile {
						gchdependfiles = append(gchdependfiles, v1)
					}
				}
			}
		}

		// 得到最终的 文件信息列表
		if len(gchdependfiles) > 0 {
			blog.Infof("cc: got pch depends files:%v", gchdependfiles)
			fs, err := dcFile.GetFileInfo(gchdependfiles, false, false, dcPump.SupportPumpLstatByDir(cc.sandbox.Env))
			if err != nil {
				return nil, err
			}

			// 获取每个pch的依赖列表
			for _, v := range gchdependfiles {
				df, err := cc.getPumpFileByPCHFullPath(v)
				if err != nil {
					continue
				}

				// 尝试从缓存取
				fsv := getPchDependHead(df)
				if fsv != nil && len(fsv) > 0 {
					fs = append(fs, fsv...)
				} else {
					fsv, err := cc.analyzeIncludes(df, cc.sandbox.Dir)
					if err == nil && len(fsv) > 0 {
						fs = append(fs, fsv...)
						// 添加到缓存
						setPchDependHead(df, fsv)
					}
				}
			}

			blog.Infof("cc: got pch total %d depends files", len(fs))
			return fs, nil
		}
	}

	return nil, nil
}

// first error means real error when try pump, second is notify error
func (cc *TaskCC) trypump(command []string) (*dcSDK.BKDistCommand, error, error) {
	blog.Infof("cc: trypump: %v", command)

	// TODO : !! ensureCompilerRaw changed the command slice, it maybe not we need !!
	tstart := time.Now().Local()
	responseFile, args, showinclude, sourcedependfile, objectfile, pchfile, err := ensureCompilerRaw(command, cc.sandbox.Dir)
	if err != nil {
		blog.Debugf("cc: pre execute ensure compiler failed %v: %v", args, err)
		return nil, err, nil
	} else {
		blog.Infof("cc: after parse command, got responseFile:%s,sourcedepent:%s,objectfile:%s,pchfile:%s",
			responseFile, sourcedependfile, objectfile, pchfile)
	}
	tend := time.Now().Local()
	blog.Debugf("cc: trypump time record: %s for ensureCompilerRaw for rsp file:%s", tend.Sub(tstart), responseFile)

	// if sourcedependfile == "" {
	// 	blog.Infof("cc: trypump not found depend file, do nothing")
	// 	return nil, ErrorNoDependFile
	// }

	tstart = tend

	// TODO : 关注 gch 文件的依赖列表
	if strings.HasSuffix(objectfile, ".gch") && sourcedependfile != "" {
		cc.needresolvepchdepend = true
		cc.sourcedependfile = sourcedependfile
		cc.objectpchfile = objectfile
		blog.Infof("cc: need resolve dpend for gch file:%s", objectfile)
	}

	ccargs, err := scanArgs(args)
	if err != nil {
		blog.Debugf("cc: try pump not support, scan args %v: %v", args, err)
		return nil, err, ErrorNotSupportRemote
	}

	inblack, _ := cc.inPumpBlack(responseFile, args)
	if inblack {
		return nil, ErrorInPumpBlack, nil
	}

	tend = time.Now().Local()
	blog.Debugf("cc: trypump time record: %s for scanArgs for rsp file:%s", tend.Sub(tstart), responseFile)
	tstart = tend

	if cc.sourcedependfile == "" {
		if sourcedependfile != "" {
			cc.sourcedependfile = sourcedependfile
		} else {
			// TODO : 我们可以主动加上 /showIncludes 参数得到依赖列表，生成一个临时的 cl.sourcedependfile 文件
			blog.Infof("cc: trypump not found depend file, try append it")
			if cc.forceDepend(ccargs) != nil {
				return nil, ErrorNoDependFile, nil
			}
		}
	}
	cc.showinclude = showinclude
	cc.needcopypumpheadfile = true

	cc.responseFile = responseFile
	cc.pumpArgs = args

	includes, err := cc.Includes(responseFile, args, cc.sandbox.Dir, false)

	// TODO : 添加pch的依赖列表
	if len(includes) > 0 {
		pchdepends, err := cc.getPchDepends(includes)
		if err == nil && len(pchdepends) > 0 {
			includes = append(includes, pchdepends...)
		}
	}

	tend = time.Now().Local()
	blog.Debugf("cc: trypump time record: %s for Includes for rsp file:%s", tend.Sub(tstart), responseFile)
	tstart = tend

	// if sourcedependfile != "" {
	// 	cc.needcopypumpheadfile = true
	// }

	if err == nil {
		// add pch file as input
		if pchfile != "" {
			// includes = append(includes, pchfile)
			finfo, _ := cc.checkFstat(pchfile, cc.sandbox.Dir)
			if finfo != nil {
				includes = append(includes, finfo)
			}
		}

		// add response file as input
		if responseFile != "" {
			// includes = append(includes, responseFile)
			finfo, _ := cc.checkFstat(responseFile, cc.sandbox.Dir)
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
			// existed, fileSize, modifyTime, fileMode := dcFile.Stat(f).Batch()
			existed, fileSize, modifyTime, fileMode := f.Batch()
			fpath := f.Path()
			if !existed {
				err := fmt.Errorf("input file %s not existed", fpath)
				blog.Errorf("cc: %v", err)
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
				LinkTarget:         f.LinkTarget,
				NoDuplicated:       true,
				Priority:           dcSDK.GetPriority(f),
			})
			// priority++
			// blog.Infof("cc: added include file:%s with modify time %d", fpath, modifyTime)

			blog.Debugf("cc: added include file:%s for object:%s", fpath, objectfile)
		}

		results := []string{objectfile}
		// add source depend file as result
		if sourcedependfile != "" {
			results = append(results, sourcedependfile)
		}

		// set env which need append to remote
		envs := []string{}
		for _, v := range cc.sandbox.Env.Source() {
			if strings.HasPrefix(v, appendEnvKey) {
				envs = append(envs, v)
				// set flag we hope append env, not overwrite
				flag := fmt.Sprintf("%s=true", dcEnv.GetEnvKey(dcEnv.KeyRemoteEnvAppend))
				envs = append(envs, flag)
				break
			}
		}
		blog.Infof("cc: env which ready sent to remote:[%v]", envs)

		exeName := command[0]
		params := command[1:]
		blog.Infof("cc: parse command,server command:[%s %s],dir[%s]",
			exeName, strings.Join(params, " "), cc.sandbox.Dir)
		return &dcSDK.BKDistCommand{
			Commands: []dcSDK.BKCommand{
				{
					WorkDir:         cc.sandbox.Dir,
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
	blog.Debugf("cc: trypump time record: %s for return dcSDK.BKCommand for rsp file:%s", tend.Sub(tstart), responseFile)

	return nil, err, nil
}

func (cc *TaskCC) isPumpActionNumSatisfied() (bool, error) {
	minnum := dcPump.PumpMinActionNum(cc.sandbox.Env)
	if minnum <= 0 {
		return true, nil
	}

	curbatchsize := 0
	strsize := cc.sandbox.Env.GetEnv(dcEnv.KeyExecutorTotalActionNum)
	if strsize != "" {
		size, err := strconv.Atoi(strsize)
		if err != nil {
			return true, err
		} else {
			curbatchsize = size
		}
	}

	blog.Infof("cc: check pump action num with min:%d: current batch num:%d", minnum, curbatchsize)

	return int32(curbatchsize) > minnum, nil
}

func (cc *TaskCC) workerSupportAbsPath() bool {
	v := cc.sandbox.Env.GetEnv(dcEnv.KeyWorkerSupportAbsPath)
	if v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return true
}

func (cc *TaskCC) preExecute(command []string) (*dcSDK.BKDistCommand, dcType.BKDistCommonError) {
	blog.Infof("cc: start pre execute for: %v", command)

	// debugRecordFileName(fmt.Sprintf("cc: start pre execute for: %v", command))

	cc.originArgs = command

	// ++ try with pump,only support windows now
	if !cc.hasResultIndex() {
		if !cc.pumpremotefailed && dcPump.SupportPump(cc.sandbox.Env) && cc.workerSupportAbsPath() {
			if satisfied, _ := cc.isPumpActionNumSatisfied(); satisfied {
				req, err, notifyerr := cc.trypump(command)
				if err != nil {
					if notifyerr == ErrorNotSupportRemote {
						blog.Warnf("cc: pre execute failed to try pump %v: %v", command, err)
						return nil, dcType.BKDistCommonError{
							Code:  dcType.UnknowCode,
							Error: err,
						}
					}
				} else {
					// for debug
					blog.Debugf("cc: after try pump, req: %+v", *req)
					cc.pumpremote = true
					return req, dcType.ErrorNone
				}
			}
		}
	}
	// --

	responseFile, args, err := ensureCompiler(command, cc.sandbox.Dir)
	if err != nil {
		blog.Warnf("cc: pre execute ensure compiler %v: %v", args, err)
		return nil, dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: err,
		}
	}

	// obtain force key set by booster
	forcekeystr := cc.sandbox.Env.GetEnv(dcEnv.KeyExecutorForceLocalKeys)
	if forcekeystr != "" {
		blog.Infof("cc: got force local key string: %s", forcekeystr)
		forcekeylist := strings.Split(forcekeystr, dcEnv.CommonBKEnvSepKey)
		if len(forcekeylist) > 0 {
			cc.ForceLocalResponseFileKeys = append(cc.ForceLocalResponseFileKeys, forcekeylist...)
			cc.ForceLocalCppFileKeys = append(cc.ForceLocalCppFileKeys, forcekeylist...)
			blog.Infof("cc: ForceLocalResponseFileKeys: %v, ForceLocalCppFileKeys: %v",
				cc.ForceLocalResponseFileKeys, cc.ForceLocalCppFileKeys)
		}
	}

	if responseFile != "" {
		for _, v := range cc.ForceLocalResponseFileKeys {
			if v != "" && strings.Contains(responseFile, v) {
				blog.Warnf("cc: pre execute found response %s is in force local list, do not deal now",
					responseFile)
				// blog.Warnf("response file %s is in force local list", responseFile)
				return nil, dcType.ErrorPreForceLocal
			}
		}
	}

	for _, v := range args {
		if strings.HasSuffix(v, ".cpp") {
			for _, v1 := range cc.ForceLocalCppFileKeys {
				if v1 != "" && strings.Contains(v, v1) {
					blog.Warnf("cc: pre execute found %s is in force local list, do not deal now", v)
					// return nil, fmt.Errorf("arg %s is in force local cpp list", v)
					return nil, dcType.ErrorPreForceLocal
				}
			}
			break
		}
	}

	cc.responseFile = responseFile
	cc.ensuredArgs = args

	// debugRecordFileName("preBuild begin")

	if cc.forcedepend {
		// TODO : 如果存在 -MMD，则替换为 -MD，否则，追加 -MD
		hasFlag := false
		for i := range args {
			if args[i] == "-MMD" {
				args[i] = "-MD"
				hasFlag = true
				break
			}
		}
		if !hasFlag {
			args = append(args, "-MD")
		}
		args = append(args, "-MF")
		args = append(args, cc.sourcedependfile)
	}

	if err = cc.preBuild(args); err != nil {
		blog.Warnf("cc: pre execute pre-build %v: %v", args, err)
		return nil, dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: err,
		}
	}

	// debugRecordFileName("FileInfo begin")

	existed, fileSize, modifyTime, fileMode := dcFile.Stat(cc.preprocessedFile).Batch()
	if !existed {
		err := fmt.Errorf("result file %s not existed", cc.preprocessedFile)
		blog.Errorf("%v", err)
		return nil, dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: fmt.Errorf("%s not existed", cc.preprocessedFile),
		}
	}

	// generate the input files for pre-process file
	inputFiles := []dcSDK.FileDesc{{
		FilePath:           cc.preprocessedFile,
		Compresstype:       protocol.CompressLZ4,
		FileSize:           fileSize,
		Lastmodifytime:     modifyTime,
		Md5:                "",
		Filemode:           fileMode,
		Targetrelativepath: filepath.Dir(cc.preprocessedFile),
	}}

	// if there is a pch file, add it into the inputFiles, it should be also sent to remote
	if cc.pchFileDesc != nil {
		inputFiles = append(inputFiles, *cc.pchFileDesc)
	}

	// debugRecordFileName(fmt.Sprintf("cc: success done pre execute for: %v", command))

	blog.Infof("cc: success done pre execute for: %v", command)
	blog.Infof("cc: going to execute compile: %v", cc.serverSideArgs)

	// to check whether need to compile with response file
	exeName := cc.serverSideArgs[0]
	params := cc.serverSideArgs[1:]

	return &dcSDK.BKDistCommand{
		Commands: []dcSDK.BKCommand{
			{
				WorkDir:         "",
				ExePath:         "",
				ExeName:         exeName,
				ExeToolChainKey: dcSDK.GetJsonToolChainKey(command[0]),
				Params:          params,
				Inputfiles:      inputFiles,
				ResultFiles: []string{
					cc.outputFile,
				},
			},
		},
		CustomSave: true,
	}, dcType.ErrorNone
}

func (cc *TaskCC) postExecute(r *dcSDK.BKDistResult) dcType.BKDistCommonError {
	blog.Infof("cc: start post execute for: %v", cc.originArgs)
	if r == nil || len(r.Results) == 0 {
		blog.Warnf("cc: parameter is invalid")
		return dcType.BKDistCommonError{
			Code:  dcType.UnknowCode,
			Error: fmt.Errorf("parameter is invalid"),
		}
	}

	resultfilenum := 0
	// by tomtian 20201224,to ensure existed result file
	if len(r.Results[0].ResultFiles) == 0 {
		blog.Warnf("cc: not found result file for: %v", cc.originArgs)
		goto ERROREND
	}
	blog.Infof("cc: found %d result files for result[0]", len(r.Results[0].ResultFiles))

	// resultfilenum := 0
	if len(r.Results[0].ResultFiles) > 0 {
		for _, f := range r.Results[0].ResultFiles {
			if f.Buffer != nil {
				if err := saveResultFile(&f, cc.sandbox.Dir); err != nil {
					blog.Errorf("cc: failed to save file [%s] with error:%v", f.FilePath, err)
					return dcType.BKDistCommonError{
						Code:  dcType.UnknowCode,
						Error: err,
					}
				}
				resultfilenum++
			}
		}
	}

	// by tomtian 20201224,to ensure existed result file
	if resultfilenum == 0 {
		blog.Warnf("cc: not found result file for: %v", cc.originArgs)
		goto ERROREND
	}

	if r.Results[0].RetCode == 0 {
		blog.Infof("cc: success done post execute for: %v", cc.originArgs)
		// set output to inputFile
		// r.Results[0].OutputMessage = []byte(filepath.Base(cc.inputFile))
		// if remote succeed with pump,do not need copy head file
		if cc.pumpremote {
			cc.needcopypumpheadfile = false
		}
		return dcType.ErrorNone
	}

ERROREND:
	// write error message into
	if cc.saveTemp() && len(r.Results[0].ErrorMessage) > 0 {
		// make the tmp file for storing the stderr from server compiler.
		stderrFile, err := makeTmpFile(commonUtil.GetHandlerTmpDir(cc.sandbox), "cc_server_stderr", ".txt")
		if err != nil {
			blog.Warnf("cc: make tmp file for stderr from server failed: %v", err)
		} else {
			if f, err := os.OpenFile(stderrFile, os.O_RDWR, 0644); err == nil {
				_, _ = f.Write(r.Results[0].ErrorMessage)
				_ = f.Close()
				blog.Debugf("cc: save error message to %s for: %s", stderrFile, cc.originArgs)
			}
		}
	}

	if cc.pumpremote {
		blog.Infof("cc: ready remove pump head file: %s after failed pump remote, generate it next time", cc.pumpHeadFile)
		os.Remove(cc.pumpHeadFile)
	}

	blog.Warnf("cc: failed to remote execute, retcode %d, error message:%s, output message:%s",
		r.Results[0].RetCode,
		r.Results[0].ErrorMessage,
		r.Results[0].OutputMessage)

	return dcType.BKDistCommonError{
		Code:  dcType.UnknowCode,
		Error: fmt.Errorf(string(r.Results[0].ErrorMessage)),
	}
}

func (cc *TaskCC) resolvePchDepend(workdir string) error {
	blog.Infof("cc: ready resolve pch depend file: %s", cc.sourcedependfile)

	sep := "\n"
	if runtime.GOOS == osWindows {
		sep = "\r\n"
	}

	includes := []string{}
	err := cc.resolveDependFile(sep, workdir, &includes)
	if err != nil {
		return err
	}

	if len(includes) > 0 {
		if !filepath.IsAbs(cc.objectpchfile) {
			cc.objectpchfile, _ = filepath.Abs(filepath.Join(workdir, cc.objectpchfile))
		}

		gchfile := []string{}
		for _, v := range includes {
			if strings.HasSuffix(v, ".gch") {
				newfile := true
				for _, v1 := range gchfile {
					if v == v1 {
						newfile = false
						break
					}
				}

				if newfile {
					blog.Infof("cc: found pch depend %s->%s", cc.objectpchfile, v)
					gchfile = append(gchfile, v)
				}
			}
		}

		if len(gchfile) > 0 {
			dcPump.SetPchDepend(cc.objectpchfile, gchfile)
		}
	}

	return nil
}

func (cc *TaskCC) getPumpFileByPCHFullPath(f string) (string, error) {
	pumpdir, err := cc.getPumpDir(cc.sandbox.Env)
	if err == nil {
		args := []string{f}
		return getPumpIncludeFile(pumpdir, "pch_depend", ".txt", args, cc.sandbox.Dir)
	}

	blog.Warnf("cc: got pch:%s depend file with err:%v", f, err)
	return "", err
}

func (cc *TaskCC) finalExecute([]string) {
	go func() {
		if cc.needcopypumpheadfile {
			cc.copyPumpHeadFile(cc.sandbox.Dir)
		}

		// TODO : 解析pch的依赖关系，并将该gch的依赖列表保存起来
		if cc.needresolvepchdepend {
			// 解析并保存pch的依赖关系
			cc.resolvePchDepend(cc.sandbox.Dir)

			// 并将该gch的依赖列表保存起来
			var err error
			cc.pumpHeadFile, err = cc.getPumpFileByPCHFullPath(cc.objectpchfile)
			if err != nil {
				blog.Warnf("cc: failed to get pump head file with pch file:%s err:%v", cc.objectpchfile, err)
			} else {
				cc.copyPumpHeadFile(cc.sandbox.Dir)
			}
		}

		if cc.saveTemp() {
			return
		}

		cc.cleanTmpFile()
	}()
}

func (cc *TaskCC) saveTemp() bool {
	return commonUtil.GetHandlerEnv(cc.sandbox, envSaveTempFile) != ""
}

func (cc *TaskCC) preBuild(args []string) error {
	// debugRecordFileName(fmt.Sprintf("cc: preBuild in..."))

	blog.Infof("cc: pre-build begin got args: %v", args)

	// debugRecordFileName(fmt.Sprintf("cc: pre-build ready expandPreprocessorOptions"))

	var err error
	// expand args to make the following process easier.
	if cc.expandArgs, err = expandPreprocessorOptions(args); err != nil {
		blog.Warnf("cc: pre-build error, expand pre-process options %v: %v", args, err)
		return err
	}

	// debugRecordFileName(fmt.Sprintf("cc: pre-build ready scanArgs"))

	// scan the args, check if it can be compiled remotely, wrap some un-used options,
	// and get the real input&output file.
	scannedData, err := scanArgs(cc.expandArgs)
	if err != nil {
		blog.Warnf("cc: pre-build not support, scan args %v: %v", cc.expandArgs, err)
		return err
	}
	cc.scannedArgs = scannedData.args
	cc.firstIncludeFile = getFirstIncludeFile(scannedData.args)
	cc.inputFile = scannedData.inputFile
	cc.outputFile = scannedData.outputFile
	cc.includeRspFiles = scannedData.includeRspFiles
	cc.includePaths = scannedData.includePaths
	cc.includeFiles = scannedData.includeFiles

	// TODO : resolve rsp files recursively
	if len(cc.includeRspFiles) > 0 {
		checkedRspFiles := []string{}
		for _, f := range cc.includeRspFiles {
			scanRspFilesRecursively(f, cc.sandbox.Dir, &cc.includePaths, &cc.includeFiles, &checkedRspFiles)
		}
		cc.includeRspFiles = append(cc.includeRspFiles, checkedRspFiles...)
	}

	// handle the cross-compile issues.
	targetArgs := cc.scannedArgs
	if commonUtil.GetHandlerEnv(cc.sandbox, envNoRewriteCross) != "1" {
		var targetArgsGeneric, targetArgsACT, targetArgsGRF []string

		if targetArgsGeneric, err = rewriteGenericCompiler(targetArgs); err != nil {
			blog.Warnf("cc: pre-build error, rewrite generic compiler %v: %v", targetArgs, err)
			return err
		}

		if targetArgsACT, err = addClangTarget(targetArgsGeneric); err != nil {
			blog.Warnf("cc: pre-build error, add clang target %v: %v", targetArgsGeneric, err)
			return err
		}

		if targetArgsGRF, err = gccRewriteFqn(targetArgsACT); err != nil {
			blog.Warnf("cc: pre-build error, gcc rewrite fqn %v: %v", targetArgsACT, err)
			return err
		}

		targetArgs = targetArgsGRF
	}
	cc.rewriteCrossArgs = targetArgs

	// debugRecordFileName(fmt.Sprintf("cc: pre-build ready scanPchFile"))

	// handle the pch options
	finalArgs := cc.scanPchFile(targetArgs)

	// debugRecordFileName(fmt.Sprintf("cc: pre-build ready makeTmpFile"))

	// debugRecordFileName(fmt.Sprintf("cc: pre-build ready doPreProcess"))

	// do the pre-process, store result in the file.
	if cc.preprocessedFile, err = cc.doPreProcess(finalArgs, cc.inputFile); err != nil {
		blog.Warnf("cc: pre-build error, do pre-process %v: %v", finalArgs, err)
		return err
	}

	// debugRecordFileName(fmt.Sprintf("cc: pre-build ready stripLocalArgs"))

	// strip the args and get the server side args.
	serverSideArgs := stripLocalArgs(finalArgs, cc.sandbox.Env)

	// replace the input file into preprocessedFile, for the next server side process.
	for index := range serverSideArgs {
		if serverSideArgs[index] == cc.inputFile {
			serverSideArgs[index] = cc.preprocessedFile
			break
		}
	}

	// avoid error : does not allow 'register' storage class specifier
	prefix := "-std=c++"
	for _, v := range serverSideArgs {
		if strings.HasPrefix(v, prefix) {
			if len(v) > len(prefix) {
				version, err := strconv.Atoi(v[len(prefix):])
				if err == nil && version > 14 {
					serverSideArgs = append(serverSideArgs, "-Wno-register")
					blog.Infof("cc: found %s,ready add [-Wno-register]", v)
				}
			}
			break
		}
	}

	// quota result file if it's path contains space
	if runtime.GOOS == osWindows {
		if hasSpace(cc.outputFile) && !strings.HasPrefix(cc.outputFile, "\"") {
			for index := range serverSideArgs {
				if strings.HasPrefix(serverSideArgs[index], "-o") {
					if len(serverSideArgs[index]) > 2 {
						serverSideArgs[index] = fmt.Sprintf("-o\"%s\"", cc.outputFile)
					} else {
						index++
						if index < len(serverSideArgs) {
							serverSideArgs[index] = fmt.Sprintf("\"%s\"", cc.outputFile)
						}
					}
					break
				}
			}
		}
	}

	cc.serverSideArgs = serverSideArgs

	if cc.SupportResultCache(args) != resultcache.CacheTypeNone {
		cc.resultCacheArgs = make([]string, len(cc.serverSideArgs))
		copy(cc.resultCacheArgs, cc.serverSideArgs)
		for index := range cc.resultCacheArgs {
			if cc.resultCacheArgs[index] == cc.preprocessedFile {
				cc.resultCacheArgs[index] = cc.inputFile
				break
			}
		}
	}

	// debugRecordFileName(fmt.Sprintf("cc: pre-build finished"))

	blog.Infof("cc: pre-build success for enter args: %v", args)
	return nil
}

func (cc *TaskCC) addTmpFile(filename string) {
	cc.tmpFileList = append(cc.tmpFileList, filename)
}

func (cc *TaskCC) cleanTmpFile() {
	for _, filename := range cc.tmpFileList {
		if err := os.Remove(filename); err != nil {
			blog.Infof("cc: clean tmp file %s failed: %v", filename, err)
		}
	}
}

// If the input filename is a plain source file rather than a
// preprocessed source file, then preprocess it to a temporary file
// and return the name.
//
// The preprocessor may still be running when we return; you have to
// wait for cpp_pid to exit before the output is complete.  This
// allows us to overlap opening the TCP socket, which probably doesn't
// use many cycles, with running the preprocessor.
func (cc *TaskCC) doPreProcess(args []string, inputFile string) (string, error) {

	// debugRecordFileName(fmt.Sprintf("cc: doPreProcess in..."))

	if isPreprocessedFile(inputFile) {
		blog.Infof("cc: input \"%s\" is already preprocessed", inputFile)

		// input file already preprocessed
		return inputFile, nil
	}

	outputExt := getPreprocessedExt(inputFile)
	outputFile, err := makeTmpFile(commonUtil.GetHandlerTmpDir(cc.sandbox), "cc", outputExt)
	if err != nil {
		blog.Warnf("cc: do pre-process get output file failed: %v", err)
		return "", err
	}
	cc.addTmpFile(outputFile)

	newArgs, err := setActionOptionE(stripDashO(args))
	if err != nil {
		blog.Warnf("cc: set action option -E: %v", err)
		return "", err
	}
	cc.preProcessArgs = newArgs

	var execName string
	var execArgs []string
	flag, rspfile, err := cc.needSaveResponseFile(newArgs)
	if flag && err == nil {
		rspargs := []string{newArgs[0], fmt.Sprintf("@%s", rspfile)}
		blog.Infof("cc: going to execute pre-process: %s", strings.Join(rspargs, " "))
		execName = rspargs[0]
		execArgs = rspargs[1:]
		cc.addTmpFile(rspfile)

	} else {
		blog.Infof("cc: going to execute pre-process: %s", strings.Join(newArgs, " "))
		execName = newArgs[0]
		execArgs = newArgs[1:]
	}

	output, err := os.OpenFile(outputFile, os.O_WRONLY, 0666)
	if err != nil {
		blog.Errorf("cc: failed to open output file \"%s\" when pre-processing: %v", outputFile, err)
		return "", err
	}
	defer func() {
		_ = output.Close()
	}()
	sandbox := cc.sandbox.Fork()
	sandbox.Stdout = output

	var errBuf bytes.Buffer
	sandbox.Stderr = &errBuf

	if _, err = sandbox.ExecCommand(execName, execArgs...); err != nil {
		blog.Errorf("cc: failed to do pre-process %s %s: %v", execName, strings.Join(execArgs, " "), err)
		return "", err
	}
	blog.Infof("cc: success to execute pre-process and get %s: %s", outputFile, strings.Join(newArgs, " "))
	cc.preprocessedErrorBuf = errBuf.String()

	return outputFile, nil
}

// try to get pch file desc and the args according to firstIncludeFile
// if pch is valid, there must be a option -include xx.h(xx.hpp)
// and must be the first seen -include option(if there are multiple -include)
func (cc *TaskCC) scanPchFile(args []string) []string {
	if cc.firstIncludeFile == "" {
		return args
	}

	filename := cc.firstIncludeFile
	if !strings.HasSuffix(cc.firstIncludeFile, ".gch") {
		filename = fmt.Sprintf("%s.gch", cc.firstIncludeFile)
	}

	existed, fileSize, modifyTime, fileMode := dcFile.Stat(filename).Batch()
	if !existed {
		blog.Debugf("cc: try to get pch file for %s but %s is not exist", cc.firstIncludeFile, filename)
		return args
	}
	cc.pchFile = filename

	blog.Debugf("cc: success to find pch file %s for %s", filename, cc.firstIncludeFile)

	cc.pchFileDesc = &dcSDK.FileDesc{
		FilePath:           filename,
		Compresstype:       protocol.CompressLZ4,
		FileSize:           fileSize,
		Lastmodifytime:     modifyTime,
		Md5:                "",
		Filemode:           fileMode,
		Targetrelativepath: filepath.Dir(filename),
	}

	for _, arg := range args {
		if arg == pchPreProcessOption {
			return args
		}
	}

	return append(args, pchPreProcessOption)
}

func (cc *TaskCC) needSaveResponseFile(args []string) (bool, string, error) {
	exe := args[0]
	if strings.HasSuffix(exe, "clang.exe") || strings.HasSuffix(exe, "clang++.exe") {
		if len(args) > 1 {
			fullArgs := MakeCmdLine(args[1:])
			// if len(fullArgs) >= MaxWindowsCommandLength {
			if cc.responseFile != "" || len(fullArgs) >= MaxWindowsCommandLength {
				rspFile, err := makeTmpFile(commonUtil.GetHandlerTmpDir(cc.sandbox), "cc", ".response")
				if err != nil {
					return false, "", fmt.Errorf("cc: cmd too long and failed to create rsp file")
				}

				err = ioutil.WriteFile(rspFile, []byte(fullArgs), os.ModePerm)
				if err != nil {
					blog.Errorf("cc: failed to write data to file[%s], error:%v", rspFile, err)
					return false, "", err
				}

				return true, rspFile, nil
			}
		}
	}

	return false, "", nil
}

// SupportResultCache check whether this command support result cache
func (cc *TaskCC) SupportResultCache(command []string) int {
	if cc.sandbox != nil {
		if str := cc.sandbox.Env.GetEnv(dcEnv.KeyExecutorResultCacheType); str != "" {
			i, err := strconv.Atoi(str)
			if err == nil {
				return i
			}
		}
	}

	return 0
}

// hasResultIndex check whether the env of hasresultindex set
func (cc *TaskCC) hasResultIndex() bool {
	return cc.sandbox.Env.GetEnv(dcEnv.KeyExecutorHasResultIndex) != ""
}

func (cc *TaskCC) GetResultCacheKey(command []string) string {
	if cc.preprocessedFile == "" {
		blog.Infof("cc: cc.preprocessedFile is null , no need to get result cache key")
		return ""
	}
	if !dcFile.Stat(cc.preprocessedFile).Exist() {
		blog.Warnf("cc: cc.preprocessedFile %s not existed when get result cache key", cc.preprocessedFile)
		return ""
	}

	// ext from cc.preprocessedFile
	ext := filepath.Ext(cc.preprocessedFile)

	// cc_mtime cc_name from compile tool
	cchash, err := dcUtil.HashFile(command[0])
	if err != nil {
		blog.Warnf("cc: hash file %s with error: %v", command[0], err)
		return ""
	}

	// LANG and LC_ALL  from env , ignore in windows now
	// cwd  from work dir , ignore now

	// arg from cc.resultCacheArgs
	argstring := strings.Join(cc.resultCacheArgs, " ")
	arghash := xxhash.Sum64([]byte(argstring))

	// cpp content from cl.preprocessedFile
	cpphash, err := dcUtil.HashFile(cc.preprocessedFile)
	if err != nil {
		blog.Warnf("cc: hash file %s with error: %v", cc.preprocessedFile, err)
		return ""
	}

	// cppstderr from cc.preprocessedErrorBuf
	cppstderrhash := xxhash.Sum64([]byte(cc.preprocessedErrorBuf))

	fullstring := fmt.Sprintf("%s_%x_%x_%x_%x", ext, cchash, arghash, cpphash, cppstderrhash)
	fullstringhash := xxhash.Sum64([]byte(fullstring))

	blog.Infof("cc: got hash key %x for string[%s] cmd:[%s]",
		fullstringhash, fullstring, strings.Join(command, " "))

	return fmt.Sprintf("%x", fullstringhash)
}
