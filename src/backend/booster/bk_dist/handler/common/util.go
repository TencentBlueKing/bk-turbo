/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package common

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// GetHandlerEnv get env by booster type
func GetHandlerEnv(sandBox *dcSyscall.Sandbox, key string) string {
	format := "%s_%s"
	if sandBox == nil || sandBox.Env == nil {
		return env.GetEnv(fmt.Sprintf(format, strings.ToUpper(env.GetEnv(env.BoosterType)), key))
	}

	return sandBox.Env.GetEnv(fmt.Sprintf(format, strings.ToUpper(sandBox.Env.GetEnv(env.BoosterType)), key))
}

// GetHandlerTmpDir get temp dir by booster type
func GetHandlerTmpDir(sandBox *dcSyscall.Sandbox) string {
	var baseTmpDir, bt string
	if sandBox == nil {
		baseTmpDir = os.TempDir()
		bt = env.GetEnv(env.BoosterType)
	} else {
		if baseTmpDir = sandBox.Env.GetOriginEnv("TMPDIR"); baseTmpDir == "" {
			baseTmpDir = os.TempDir()
		}

		bt = sandBox.Env.GetEnv(env.BoosterType)
	}

	if baseTmpDir != "" {
		fullTmpDir := path.Join(baseTmpDir, protocol.BKDistDir, types.GetBoosterType(bt).String())
		if !dcFile.Stat(fullTmpDir).Exist() {
			if err := os.MkdirAll(fullTmpDir, os.ModePerm); err != nil {
				blog.Warnf("common util: create tmp dir failed with error:%v", err)
				return ""
			}
		}
		return fullTmpDir
	}

	return ""
}

var (
	fileInfoCacheLock sync.RWMutex
	fileInfoCache     = map[string]*dcFile.Info{}
)

func ResetFileInfoCache() {
	fileInfoCacheLock.Lock()
	defer fileInfoCacheLock.Unlock()

	fileInfoCache = map[string]*dcFile.Info{}
}

// 支持并发read，但会有重复Stat操作，考虑并发和去重的平衡
func GetFileInfo(fs []string, mustexisted bool, notdir bool, statbysearchdir bool) ([]*dcFile.Info, error) {
	// read
	fileInfoCacheLock.RLock()
	notfound := []string{}
	is := make([]*dcFile.Info, 0, len(fs))
	for _, f := range fs {
		i, ok := fileInfoCache[f]
		if !ok {
			notfound = append(notfound, f)
			continue
		}

		if !i.Exist() {
			if mustexisted {
				// continue
				// TODO : return fail if not existed
				blog.Warnf("common util: depend file:%s not existed ", f)
				return nil, fmt.Errorf("%s not existed", f)
			} else {
				continue
			}
		}

		if notdir && i.Basic().IsDir() {
			continue
		}
		is = append(is, i)
	}
	fileInfoCacheLock.RUnlock()

	blog.Infof("common util: got %d file stat and %d not found", len(is), len(notfound))
	if len(notfound) == 0 {
		return is, nil
	}

	// query
	tempis := make(map[string]*dcFile.Info, len(notfound))
	for _, notf := range notfound {
		tempf := notf
		try := 0
		maxtry := 10
		for {
			var i *dcFile.Info
			if statbysearchdir {
				i = dcFile.GetFileInfoByEnumDir(tempf)
			} else {
				i = dcFile.Lstat(tempf)
			}
			tempis[tempf] = i
			try++

			if !i.Exist() {
				if mustexisted {
					// TODO : return fail if not existed
					// continue
					blog.Warnf("common util: depend file:%s not existed ", tempf)
					return nil, fmt.Errorf("%s not existed", tempf)
				} else {
					// continue
					break
				}
			}

			loopagain := false
			if i.Basic().Mode()&os.ModeSymlink != 0 {
				originFile, err := os.Readlink(tempf)
				if err == nil {
					if !filepath.IsAbs(originFile) {
						originFile, err = filepath.Abs(filepath.Join(filepath.Dir(tempf), originFile))
						if err == nil {
							i.LinkTarget = originFile
							blog.Infof("common util: symlink %s to %s", tempf, originFile)
						} else {
							blog.Infof("common util: symlink %s origin %s, got abs path error:%s",
								tempf, originFile, err)
						}
					} else {
						i.LinkTarget = originFile
						blog.Infof("common util: symlink %s to %s", tempf, originFile)
					}

					// 如果是链接，并且指向了其它文件，则需要将指向的文件也包含进来
					// 增加寻找次数限制，避免死循环
					if try < maxtry {
						loopagain = true
						tempf = originFile
					}
				} else {
					blog.Infof("common util: symlink %s Readlink error:%s", tempf, err)
				}
			}

			if notdir && i.Basic().IsDir() {
				continue
			}

			is = append(is, i)

			if !loopagain {
				break
			}
		}
	}

	// write
	go func(tempis *map[string]*dcFile.Info) {
		fileInfoCacheLock.Lock()
		for f, i := range *tempis {
			fileInfoCache[f] = i
		}
		fileInfoCacheLock.Unlock()
	}(&tempis)

	return is, nil
}

//-----------------------------------------------------------------------

var (
	commonTargetPathSep = string(filepath.Separator)
	commonInitPathSep1  = ""
	commonInitPathSep2  = ""
)

var (
	pathmapLock sync.RWMutex
	pathmap     map[string]string = make(map[string]string, 10000)
)

func init() {
	if runtime.GOOS == "windows" {
		commonInitPathSep1 = "/"
		commonInitPathSep2 = "\\\\"
	} else {
		commonInitPathSep1 = "\\"
		commonInitPathSep2 = "//"
	}
}

// 在指定目录下找到正确的文件名（大小写）
func getWindowsRealName(inputdir, inputname string) (string, error) {
	files, err := os.ReadDir(inputdir)
	if err != nil {
		fmt.Printf("check dir:%s with error:%v\r\n", inputdir, err)
		return "", err
	}
	for _, file := range files {
		if strings.EqualFold(file.Name(), inputname) {
			return FormatFilePath(filepath.Join(inputdir, file.Name())), nil
		}
	}

	return "", fmt.Errorf("%s not exist", inputname)
}

func getPath(inputPath string) (string, bool) {
	pathmapLock.RLock()
	newpath, ok := pathmap[inputPath]
	pathmapLock.RUnlock()

	return newpath, ok
}

func putPath(oldPath, newPath string) {
	pathmapLock.Lock()
	pathmap[oldPath] = newPath
	pathmapLock.Unlock()

	return
}

// 根据指定的完整路径得到正确的大小写的完整路径
func getWindowsFullRealPath(inputPath string) (string, error) {
	newpath, ok := getPath(inputPath)
	if ok {
		return newpath, nil
	}

	// 先检查目录是否在缓存，如果在，只需要检查文件名
	realPath := inputPath
	inputdir, inputfile := filepath.Split(inputPath)
	inputdir = strings.TrimRight(inputdir, commonTargetPathSep)

	newpath, ok = getPath(inputdir)
	if ok {
		newPath, err := getWindowsRealName(newpath, inputfile)
		if err == nil {
			realPath = newPath
			// 将结果记录下来
			putPath(inputPath, realPath)
			return realPath, nil
		} else {
			// 将结果记录下来
			putPath(inputPath, inputPath)
			return inputPath, err
		}
	}

	// 完整目录逐级检查，并将逐级目录放入缓存
	parts := strings.Split(inputPath, commonTargetPathSep)
	oldpath := []string{}
	if len(parts) > 0 {
		oldpath = append(oldpath, parts[0])

		realPath = parts[0]
		if strings.HasSuffix(realPath, ":") {
			realPath = strings.ToUpper(realPath + commonTargetPathSep)
		}

		for i, part := range parts {
			if i == 0 {
				continue
			}
			if part == "" {
				continue
			}

			oldpath = append(oldpath, part)

			files, err := os.ReadDir(realPath)
			if err != nil {
				fmt.Printf("check dir:%s with error:%v\r\n", realPath, err)
				// 将结果记录下来
				putPath(inputPath, inputPath)
				return inputPath, err
			}

			for _, file := range files {
				if strings.EqualFold(file.Name(), part) {
					realPath = filepath.Join(realPath, file.Name())

					// 将过程中的路径记录下来
					putPath(strings.Join(oldpath, commonTargetPathSep), FormatFilePath(realPath))

					break
				}
			}
		}
	}

	// 将结果记录下来
	realPath = FormatFilePath(realPath)
	putPath(inputPath, realPath)

	return realPath, nil
}

func CorrectPathCap(inputPaths []string) ([]string, error) {
	newpaths := make([]string, 0, len(inputPaths))
	for _, oldpath := range inputPaths {
		newpath, err := getWindowsFullRealPath(oldpath)
		if err == nil {
			newpaths = append(newpaths, newpath)
		} else {
			newpaths = append(newpaths, oldpath)
		}
	}

	return newpaths, nil
}

// ----------------------------------------------------------------------
func FormatFilePath(f string) string {
	f = strings.Replace(f, commonInitPathSep1, commonTargetPathSep, -1)
	f = strings.Replace(f, commonInitPathSep2, commonTargetPathSep, -1)

	// 去掉路径中的..
	if strings.Contains(f, "..") {
		p := strings.Split(f, commonTargetPathSep)

		var newPath []string
		for _, v := range p {
			if v == ".." {
				newPath = newPath[:len(newPath)-1]
			} else {
				newPath = append(newPath, v)
			}
		}
		f = strings.Join(newPath, commonTargetPathSep)
	}

	return f
}

func GetAllLinkFiles(f string) []string {
	fs := []string{}
	tempf := f
	// avoid dead loop
	maxTry := 10
	try := 0
	for {
		loopagain := false
		i := dcFile.Lstat(tempf)
		if i.Exist() && i.Basic().Mode()&os.ModeSymlink != 0 {
			originFile, err := os.Readlink(tempf)
			if err == nil {
				if !filepath.IsAbs(originFile) {
					originFile, _ = filepath.Abs(filepath.Join(filepath.Dir(tempf), originFile))
				}
				fs = append(fs, FormatFilePath(originFile))

				loopagain = true
				tempf = originFile

				try++
				if try >= maxTry {
					loopagain = false
					blog.Infof("common util: symlink %s may be drop in dead loop", tempf)
					break
				}
			}
		}

		if !loopagain {
			break
		}
	}

	return fs
}

func UniqArr(arr []string) []string {
	newarr := make([]string, 0)
	tempMap := make(map[string]bool, len(newarr))
	for _, v := range arr {
		if tempMap[v] == false {
			tempMap[v] = true
			newarr = append(newarr, v)
		}
	}

	return newarr
}

func getSubdirs(path string) []string {
	var subdirs []string

	// 循环获取每级子目录
	for {
		subdirs = append([]string{path}, subdirs...)
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}

	return subdirs
}

// 获取依赖文件的路径中是链接的路径
func GetAllLinkDir(files []string) []string {
	dirs := make([]string, 0, len(files))
	for _, f := range files {
		dirs = append(dirs, filepath.Dir(f))
	}

	uniqdirs := UniqArr(dirs)
	if len(uniqdirs) > 0 {
		subdirs := []string{}
		for _, d := range uniqdirs {
			subdirs = append(subdirs, getSubdirs(d)...)
		}

		uniqsubdirs := UniqArr(subdirs)
		blog.Infof("common util: got all uniq sub dirs:%v", uniqsubdirs)

		linkdirs := []string{}
		for _, d := range uniqsubdirs {
			i := dcFile.Lstat(d)
			if i.Exist() && i.Basic().Mode()&os.ModeSymlink != 0 {
				fs := GetAllLinkFiles(d)
				if len(fs) > 0 {
					for i := len(fs) - 1; i >= 0; i-- {
						linkdirs = append(linkdirs, fs[i])
					}
					linkdirs = append(linkdirs, d)
				}
			}
		}

		blog.Infof("common util: got all link sub dirs:%v", linkdirs)
		return linkdirs
	}

	return nil
}

func GetPriority(i *dcFile.Info) dcSDK.FileDescPriority {
	isLink := i.Basic().Mode()&os.ModeSymlink != 0
	if !isLink {
		if i.Basic().IsDir() {
			return dcSDK.RealDirPriority
		} else {
			return dcSDK.RealFilePriority
		}
	}

	// symlink 需要判断是指向文件还是目录
	if i.LinkTarget != "" {
		targetfs, err := GetFileInfo([]string{i.LinkTarget}, true, false, false)
		if err == nil && len(targetfs) > 0 {
			if targetfs[0].Basic().IsDir() {
				return dcSDK.LinkDirPriority
			} else {
				return dcSDK.LinkFilePriority
			}
		}
	}

	// 尝试读文件
	linkpath := i.Path()
	targetPath, err := os.Readlink(linkpath)
	if err != nil {
		blog.Infof("common util: Error reading symbolic link: %v", err)
		return dcSDK.LinkFilePriority
	}

	// 获取符号链接指向路径的文件信息
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		blog.Infof("common util: Error getting target file info: %v", err)
		return dcSDK.LinkFilePriority
	}

	// 判断符号链接指向的路径是否是目录
	if targetInfo.IsDir() {
		blog.Infof("common util: %s is a symbolic link to a directory", linkpath)
		return dcSDK.LinkDirPriority
	} else {
		blog.Infof("common util: %s is a symbolic link, but not to a directory", linkpath)
		return dcSDK.LinkFilePriority
	}
}
