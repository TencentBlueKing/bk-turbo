/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package basic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

var (
	uniqRemoteToolchainID = ""
)

func uniqFiles(files []dcSDK.FileDesc) ([]dcSDK.FileDesc, error) {
	uniqfiles := make([]dcSDK.FileDesc, 0, 0)
	uniqmap := make(map[string]bool)
	for _, f := range files {
		uniqkey := fmt.Sprintf("%s_^|^_%s", f.FilePath, f.Targetrelativepath)
		if _, ok := uniqmap[uniqkey]; !ok {
			uniqmap[uniqkey] = true
			uniqfiles = append(uniqfiles, f)
		} else {
			blog.Infof("basic: file: %s %s repreated", f.FilePath, f.Targetrelativepath)
		}
	}

	blog.Infof("basic: uniq before file num: %d after num: %d", len(files), len(uniqfiles))

	// ++for debug by tomtian
	for _, v := range uniqfiles {
		blog.Debugf("basic: toolchain file: %s", v.FilePath)
	}
	// --
	return uniqfiles, nil
}

func getToolChainFiles(t *types.ToolChain) ([]dcSDK.FileDesc, error) {
	if t == nil {
		return nil, fmt.Errorf("tool chain is nil when get tool chain files")
	}

	sdkOneToolchain := dcSDK.OneToolChain{
		ToolName:               t.ToolName,
		ToolLocalFullPath:      t.ToolLocalFullPath,
		ToolRemoteRelativePath: t.ToolRemoteRelativePath,
		Files:                  t.Files,
	}

	sdkToolchain := &dcSDK.Toolchain{
		Toolchains: []dcSDK.OneToolChain{sdkOneToolchain},
	}

	files, err := sdkToolchain.ToFileDesc()
	if err != nil {
		return nil, err
	}

	return files, err
}

func diffToolChainFiles(oldfs, newfs *[]dcSDK.FileDesc) (bool, string, error) {
	same := true
	diffdesc := ""

	if oldfs != nil && newfs == nil {
		same = false
		diffdesc = fmt.Sprintf("new file list is nil")
		return same, diffdesc, nil
	}

	blog.Infof("basic: old files[%d], new files[%d]", len(*oldfs), len(*newfs))

	newfiles := make([]string, 0, 0)
	deletefiles := make([]string, 0, 0)
	samenamefiles := make([]string, 0, 0)
	sizechangedfiles := make([]string, 0, 0) // only compare size now, not md5

	// check samename / new / sizechanged
	found := false
	for _, newf := range *newfs {
		found = false
		for _, oldf := range *oldfs {
			if newf.FilePath == oldf.FilePath {
				found = true
				samenamefiles = append(samenamefiles, newf.FilePath)
				if newf.FileSize != oldf.FileSize {
					blog.Infof("basic: file[%s] size changed, need send again", newf.FilePath)
					sizechangedfiles = append(sizechangedfiles, newf.FilePath)
				} else if newf.Lastmodifytime != oldf.Lastmodifytime {
					blog.Infof("basic: file[%s] moidfy time changed, need send again", newf.FilePath)
					sizechangedfiles = append(sizechangedfiles, newf.FilePath)
				}
				break
			}
		}
		if !found {
			newfiles = append(newfiles, newf.FilePath)
		}
	}

	// check deletefiles
	if len(samenamefiles) != len(*oldfs) {
		for _, oldf := range *oldfs {
			found = false
			for _, newf := range *newfs {
				if oldf.FilePath == newf.FilePath {
					found = true
					break
				}
			}
			if !found {
				deletefiles = append(deletefiles, oldf.FilePath)
			}
		}
	}

	// maybe we can ignore delete files
	if len(newfiles) > 0 || len(sizechangedfiles) > 0 || len(deletefiles) > 0 {
		same = false
		diffdesc = fmt.Sprintf("new files[%v], size changed files[%v], deleted files[%v]",
			newfiles, sizechangedfiles, deletefiles)
		return same, diffdesc, nil
	}

	blog.Infof("basic: new files[%d], size changed files[%d], deleted files[%d] same files[%d]",
		len(newfiles), len(sizechangedfiles), len(deletefiles), len(samenamefiles))

	return same, diffdesc, nil
}

func replaceTaskID(uniqid string, toolchain *types.ToolChain) error {
	blog.Debugf("basic: try to render tool chain with ID: %s, toolchain:%+v", uniqid, *toolchain)

	if strings.Contains(toolchain.ToolRemoteRelativePath, toolchainTaskIDKey) {
		toolchain.ToolRemoteRelativePath = strings.Replace(
			toolchain.ToolRemoteRelativePath, toolchainTaskIDKey, uniqid, -1)
	}

	for i, f := range toolchain.Files {
		if strings.Contains(f.RemoteRelativePath, toolchainTaskIDKey) {
			toolchain.Files[i].RemoteRelativePath = strings.Replace(
				f.RemoteRelativePath, toolchainTaskIDKey, uniqid, -1)
		}
	}

	return nil
}

func getRelativeFiles(f []dcSDK.FileDesc) *[]dcSDK.FileDesc {
	if f == nil {
		return nil
	}

	newf := make([]dcSDK.FileDesc, len(f))
	copy(newf, f)

	for i := range newf {
		if filepath.IsAbs(newf[i].Targetrelativepath) {
			vol := filepath.VolumeName(newf[i].Targetrelativepath)
			if vol != "" {
				newf[i].Targetrelativepath = strings.Replace(newf[i].Targetrelativepath, vol, getToolchainID(), 1)
			} else {
				newf[i].Targetrelativepath = filepath.Join(getToolchainID(), newf[i].Targetrelativepath)
			}
		} else {
			newf[i].Targetrelativepath = filepath.Join(getToolchainID(), newf[i].Targetrelativepath)
		}
	}

	return &newf
}

func getRelativeToolChainRemotePath(p string) string {
	if p == "" {
		return p
	}

	if filepath.IsAbs(p) {
		vol := filepath.VolumeName(p)
		if vol != "" {
			return strings.Replace(p, vol, getToolchainID(), 1)
		} else {
			return filepath.Join(getToolchainID(), p)
		}
	}

	return filepath.Join(getToolchainID(), p)
}

func getToolchainID() string {
	if uniqRemoteToolchainID != "" {
		return uniqRemoteToolchainID
	}

	// id 作为远程的目录名，尽量简短，避免触发windows路径过长的问题
	uniqRemoteToolchainID = fmt.Sprintf("tc_%s", dcUtil.UniqID())
	return uniqRemoteToolchainID
}

// ++++++++++++++++++++to search toolchain++++++++++++++++++++

func searchToolChain(cmd string) (*types.ToolChain, error) {
	blog.Infof("basic: real start search toolchian for cmd:%s", cmd)
	defer blog.Infof("basic: end search toolchian for cmd:%s", cmd)

	cmdbase := filepath.Base(cmd)
	switch cmdbase {
	case "clang", "clang++":
		return searchClang(cmd)
	case "gcc", "g++":
		return searchGcc(cmd)
	case "cc", "c++":
		return searchGcc(cmd)
	}

	return nil, nil
}

func searchCC(exe string) []string {
	// search cc1
	cmd := fmt.Sprintf("%s -print-prog-name=cc1", exe)
	blog.Infof("basic: ready run cmd:[%s] for exe:%s", cmd, exe)
	sandbox := dcSyscall.Sandbox{}
	_, out, _, err := sandbox.ExecScriptsWithMessage(cmd)
	if err != nil {
		blog.Warnf("basic: search as with out:%s,error:%+v", out, err)
		return nil
	}

	blog.Infof("basic: got output:[%s] for cmd:[%s]", out, cmd)

	cc1 := strings.TrimSpace(string(out))
	i := dcFile.Lstat(cc1)
	if !i.Exist() {
		err := fmt.Errorf("file %s not existed", cc1)
		blog.Errorf("basic: %v", err)
		return nil
	}

	// cc1plus
	cc1plus := cc1 + "plus"
	i = dcFile.Lstat(cc1plus)
	if !i.Exist() {
		err := fmt.Errorf("file %s not existed", cc1plus)
		blog.Errorf("basic: %v", err)
		return nil
	}

	return []string{cc1, cc1plus}
}

func searchAS(exe string) []string {
	cmd := "which as"
	blog.Infof("basic: ready run cmd:[%s] for exe:%s", cmd, exe)
	sandbox := dcSyscall.Sandbox{}
	_, out, _, err := sandbox.ExecScriptsWithMessage(cmd)
	if err != nil {
		blog.Warnf("basic: search as with out:%s,error:%+v", out, err)
		return nil
	}

	blog.Infof("basic: got output:[%s] for cmd:[%s]", out, cmd)
	f := strings.TrimSpace(string(out))
	i := dcFile.Lstat(f)
	if !i.Exist() {
		err := fmt.Errorf("file %s not existed", f)
		blog.Errorf("basic: %v", err)
		return nil
	}

	return []string{f}
}

var soWhiteList = []string{
	"libstdc++",
	"libmpc",
	"libmpfr",
	"libgmp",
	"libopcodes",
	"libbfd",
}

var specialSOFiles = []string{
	"/lib64/libstdc++.so.6",
}

func getLddFiles(exe string) []string {
	cmd := fmt.Sprintf("ldd %s", exe)
	blog.Infof("basic: ready run cmd:[%s] for exe:%s", cmd, exe)
	sandbox := dcSyscall.Sandbox{}
	_, out, _, err := sandbox.ExecScriptsWithMessage(cmd)
	if err != nil {
		blog.Warnf("basic: search as with out:%s,error:%+v", out, err)
		return nil
	}

	blog.Infof("basic: got output:[%s] for cmd:[%s]", out, cmd)
	fields := strings.Fields(string(out))
	files := []string{}
	for _, f := range fields {
		// 这个好像会导致异常，先屏蔽
		if strings.HasSuffix(f, "libc.so.6") {
			continue
		}
		if filepath.IsAbs(f) && strings.Contains(f, ".so") {
			inWhiteList := false
			for _, w := range soWhiteList {
				if strings.Contains(f, w) {
					inWhiteList = true
					break
				}
			}

			if inWhiteList && dcFile.Lstat(f).Exist() {
				files = append(files, f)
			}
		}
	}

	blog.Infof("basic: got ldd files:%v for exe:%s", files, exe)
	return files
}

func searchGcc(cmd string) (*types.ToolChain, error) {
	blog.Infof("basic: search gcc toolchain with exe:%s", cmd)

	i := dcFile.Lstat(cmd)
	if !i.Exist() {
		err := fmt.Errorf("cmd %s not existed", cmd)
		blog.Errorf("basic: %v", err)
		return nil, err
	}

	t := &types.ToolChain{
		ToolKey:                cmd,
		ToolName:               filepath.Base(cmd),
		ToolLocalFullPath:      cmd,
		ToolRemoteRelativePath: filepath.Dir(cmd),
		Files:                  make([]dcSDK.ToolFile, 0),
		Timestamp:              time.Now().Local().UnixNano(),
	}

	exefiles := []string{cmd}

	// search cc1 / cc1plus
	fs := searchCC(cmd)
	for _, i := range fs {
		t.Files = append(t.Files, dcSDK.ToolFile{
			LocalFullPath:      i,
			RemoteRelativePath: filepath.Dir(i),
		})
		exefiles = append(exefiles, i)
	}

	// search as
	fs = searchAS(cmd)
	for _, i := range fs {
		t.Files = append(t.Files, dcSDK.ToolFile{
			LocalFullPath:      i,
			RemoteRelativePath: filepath.Dir(i),
		})
		exefiles = append(exefiles, i)
	}

	// ldd files for executable files
	for _, i := range exefiles {
		fs = getLddFiles(i)
		for _, i := range fs {
			t.Files = append(t.Files, dcSDK.ToolFile{
				LocalFullPath:      i,
				RemoteRelativePath: filepath.Dir(i),
			})
		}
	}

	// add special so
	for _, f := range specialSOFiles {
		if dcFile.Lstat(f).Exist() {
			t.Files = append(t.Files, dcSDK.ToolFile{
				LocalFullPath:      f,
				RemoteRelativePath: filepath.Dir(f),
			})
		}
	}

	blog.Infof("basic: got gcc/g++ toolchian:%+v", *t)
	return t, nil
}

// search clang crtbegin.o
func searchCrtbegin(exe string) []string {
	cmd := fmt.Sprintf("%s -v", exe)
	blog.Infof("basic: ready run cmd:[%s]", cmd)
	sandbox := dcSyscall.Sandbox{}
	_, out, errmsg, err := sandbox.ExecScriptsWithMessage(cmd)
	if err != nil {
		blog.Warnf("basic: search clang crtbegin with out:%s,error:%+v", out, err)
		return nil
	}
	blog.Infof("basic: got output:[%s] errmsg:[%s] for cmd:[%s]", out, errmsg, cmd)

	// resolve output message
	gccpath := ""
	key := "Selected GCC installation:"
	lines := strings.Split(string(errmsg), "\n")
	for _, l := range lines {
		blog.Infof("basic: check line:[%s]", l)
		if strings.HasPrefix(l, key) {
			fields := strings.Split(l, ":")
			blog.Infof("basic: got fields:%+v,len(fields):%d", fields, len(fields))
			if len(fields) == 2 {
				gccpath = strings.TrimSpace(fields[1])
				i := dcFile.Lstat(gccpath)
				if !i.Exist() {
					err := fmt.Errorf("path %s not existed", gccpath)
					blog.Errorf("basic: %v", err)
					return nil
				} else {
					blog.Infof("basic: gcc path:[%s] existed", gccpath)
				}
			}
			break
		}
	}

	fs := make([]string, 0, 2)
	if gccpath != "" {
		err := filepath.Walk(gccpath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasPrefix(info.Name(), "crt") && strings.HasSuffix(info.Name(), ".o") {
				fs = append(fs, path)
			}
			return nil
		})

		if err != nil {
			blog.Warnf("basic: walk dir:%s with error:%v", gccpath, err)
		}
	}

	if len(fs) > 0 {
		blog.Infof("basic: got crtbegin files:%v", fs)
		return fs
	}

	return nil
}

func searchClang(cmd string) (*types.ToolChain, error) {
	blog.Infof("basic: search clang toolchain with exe:%s", cmd)

	i := dcFile.Lstat(cmd)
	if !i.Exist() {
		err := fmt.Errorf("cmd %s not existed", cmd)
		blog.Errorf("basic: %v", err)
		return nil, err
	}

	t := &types.ToolChain{
		ToolKey:                cmd,
		ToolName:               filepath.Base(cmd),
		ToolLocalFullPath:      cmd,
		ToolRemoteRelativePath: filepath.Dir(cmd),
		Files:                  make([]dcSDK.ToolFile, 0),
		Timestamp:              time.Now().Local().UnixNano(),
	}

	// search clang crtbegin.o
	fs := searchCrtbegin(cmd)
	for _, i := range fs {
		t.Files = append(t.Files, dcSDK.ToolFile{
			LocalFullPath:      i,
			RemoteRelativePath: filepath.Dir(i),
		})
	}

	exefiles := []string{cmd}

	// ldd files for executable files
	for _, i := range exefiles {
		fs = getLddFiles(i)
		for _, i := range fs {
			t.Files = append(t.Files, dcSDK.ToolFile{
				LocalFullPath:      i,
				RemoteRelativePath: filepath.Dir(i),
			})
		}
	}

	// add special so
	for _, f := range specialSOFiles {
		if dcFile.Lstat(f).Exist() {
			t.Files = append(t.Files, dcSDK.ToolFile{
				LocalFullPath:      f,
				RemoteRelativePath: filepath.Dir(f),
			})
		}
	}

	blog.Infof("basic: got clang/clang++ toolchian:%+v", *t)
	return t, nil
}

// --------------------to search toolchain--------------------
