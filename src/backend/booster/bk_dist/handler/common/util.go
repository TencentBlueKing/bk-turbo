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
	"strings"
	"sync"

	"github.com/Tencent/bk-ci/src/booster/bk_dist/common/env"
	dcFile "github.com/Tencent/bk-ci/src/booster/bk_dist/common/file"
	"github.com/Tencent/bk-ci/src/booster/bk_dist/common/protocol"
	dcSyscall "github.com/Tencent/bk-ci/src/booster/bk_dist/common/syscall"
	"github.com/Tencent/bk-ci/src/booster/bk_dist/common/types"
	"github.com/Tencent/bk-ci/src/booster/common/blog"
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

// 支持并发read，但会有重复Stat操作，考虑并发和去重的平衡
func GetFileInfo(fs []string, mustexisted bool, notdir bool) []*dcFile.Info {
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
			continue
		}
		if notdir && i.Basic().IsDir() {
			continue
		}
		is = append(is, i)
	}
	fileInfoCacheLock.RUnlock()

	blog.Infof("common util: got %d file stat and %d not found", len(is), len(notfound))
	if len(notfound) == 0 {
		return is
	}

	// query
	tempis := make(map[string]*dcFile.Info, len(notfound))
	for _, f := range notfound {
		i := dcFile.Stat(f)
		tempis[f] = i

		if mustexisted && !i.Exist() {
			continue
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

	return is
}
