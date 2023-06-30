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
	"strings"
	"sync"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
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

		if mustexisted && !i.Exist() {
			// continue
			// TODO : return fail if not existed
			blog.Warnf("common util: depend file:%s not existed ", f)
			return nil, fmt.Errorf("%s not existed", f)
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
	for _, f := range notfound {
		var i *dcFile.Info
		if statbysearchdir {
			i = dcFile.GetFileInfoByEnumDir(f)
		} else {
			i = dcFile.Lstat(f)
		}
		tempis[f] = i

		if mustexisted && !i.Exist() {
			// TODO : return fail if not existed
			// continue
			blog.Warnf("common util: depend file:%s not existed ", f)
			return nil, fmt.Errorf("%s not existed", f)
		}

		if i.Basic().Mode()&os.ModeSymlink != 0 {
			originFile, err := os.Readlink(f)
			if err == nil {
				if !filepath.IsAbs(originFile) {
					originFile, err = filepath.Abs(filepath.Join(filepath.Dir(f), originFile))
					if err == nil {
						i.LinkTarget = originFile
						blog.Infof("common util: symlink %s to %s", f, originFile)
					} else {
						blog.Infof("common util: symlink %s origin %s, got abs path error:%s",
							f, originFile, err)
					}
				} else {
					i.LinkTarget = originFile
					blog.Infof("common util: symlink %s to %s", f, originFile)
				}
			} else {
				blog.Infof("common util: symlink %s Readlink error:%s", f, err)
			}
		}

		if notdir && i.Basic().IsDir() {
			continue
		}

		is = append(is, i)
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
