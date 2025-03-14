/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package resultcache

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

type ResultCacheMgr interface {
	// 文件
	GetResult(groupkey, resultkey string, needUnzip bool) ([]*Result, error)
	PutResult(groupkey, resultkey string, rs []*Result, record Record) error

	// 索引
	GetRecordGroup(key string) ([]byte, error)
	PutRecord(record Record) error
	DeleteRecord(record Record) error
}

const (
	CacheTypeNone = iota
	CacheTypeLocal
	CacheTypeRemote
	CacheTypeBothLocalAndRemote = CacheTypeLocal & CacheTypeRemote
)

var (
	instance *ResultCache
	once     sync.Once
)

type ResultCache struct {
	indexmgr IndexMgr
	filemgr  FileMgr
}

func GetInstance(localdir string, maxFileNum, maxIndexNum int) ResultCacheMgr {
	once.Do(func() {
		if localdir == "" {
			localdir = dcUtil.GetResultCacheDir()
		}

		if !file.Stat(localdir).Exist() {
			os.MkdirAll(localdir, os.ModePerm)
		}

		filedir := filepath.Join(localdir, "file")
		if !file.Stat(filedir).Exist() {
			os.MkdirAll(filedir, os.ModePerm)
		}

		indexdir := filepath.Join(localdir, "index")
		if !file.Stat(indexdir).Exist() {
			os.MkdirAll(indexdir, os.ModePerm)
		}

		blog.Infof("resultcache: init cache with dir %s,max file number %d,max index number %d",
			localdir, maxFileNum, maxIndexNum)

		instance = &ResultCache{
			indexmgr: NewIndexMgrWithARC(indexdir, maxIndexNum),
			filemgr:  NewFileMgrWithARC(filedir, maxFileNum),
		}

		instance.Init()
	})
	return instance
}

func (rc *ResultCache) GetResult(groupkey, resultkey string, needUnzip bool) ([]*Result, error) {
	return rc.filemgr.GetResult(groupkey, resultkey, needUnzip)
}

func (rc *ResultCache) PutResult(groupkey, resultkey string, rs []*Result, record Record) error {
	if len(rs) == 0 {
		return nil
	}

	return rc.filemgr.PutResult(groupkey, resultkey, rs, record)
}

func (rc *ResultCache) GetRecordGroup(key string) ([]byte, error) {
	return rc.indexmgr.GetRecordGroup(key)
}

func (rc *ResultCache) PutRecord(record Record) error {
	return rc.indexmgr.PutRecord(record)
}

func (rc *ResultCache) DeleteRecord(record Record) error {
	return rc.indexmgr.DeleteRecord(record)
}

func (rc *ResultCache) Init() {
	rc.indexmgr.Init()

	rc.filemgr.Init()
}
