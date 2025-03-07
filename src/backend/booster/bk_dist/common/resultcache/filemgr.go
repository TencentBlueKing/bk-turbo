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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/util"
	"github.com/cespare/xxhash/v2"
)

type FileMgr interface {
	Init()
	GetResult(groupkey, resultkey string, needUnzip bool) ([]*Result, error)
	PutResult(groupkey, resultkey string, rs []*Result, record Record) error
}

type FileMgrWithARC struct {
	// 淘汰算法
	mutexARC sync.Mutex
	arc      *ARC

	filedir string

	// 保存需要删除的文件
	deleteMutex  sync.RWMutex
	deletedFiles map[string]time.Time

	// 文件目录读写锁
	fileDirLock  sync.RWMutex
	fileDirLocks map[string]*sync.RWMutex
}

const (
	DefaultMaxFileNumber = 10000

	DeleteFlag     = "deleted_flag"
	CheckTick      = 300 * time.Second
	DeleteDuration = 30 * time.Minute

	RecordFileName = "tbs_command_record.json"
)

func NewFileMgrWithARC(filedir string, num int) FileMgr {
	if num <= 0 {
		num = DefaultMaxFileNumber
	}

	return &FileMgrWithARC{
		filedir:      filedir,
		arc:          NewARC(num),
		deletedFiles: make(map[string]time.Time),
		fileDirLocks: make(map[string]*sync.RWMutex),
	}
}

func (f *FileMgrWithARC) Init() {
	f.load()
	go f.ticker()
}

func getPathDepth(path string) int {
	trimmedPath := strings.Trim(path, string(filepath.Separator))
	parts := strings.Split(trimmedPath, string(filepath.Separator))
	return len(parts)
}

func (f *FileMgrWithARC) load() {
	f.mutexARC.Lock()
	defer f.mutexARC.Unlock()

	// filedir / groupkey / hash[0] / hash[1] / hash / files..
	// 搜索 filedir 下的所有第4层子目录
	baselen := getPathDepth(f.filedir)
	filepath.Walk(f.filedir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			curpathlen := getPathDepth(path)
			if curpathlen-baselen == 4 {
				dir := path
				if strings.HasSuffix(dir, DeleteFlag) {
					blog.Infof("FileMgrWithARC: load %s to delete cache", dir)
					f.addDelete(dir)
				} else {
					//blog.Infof("FileMgrWithARC:ready load %s", dir)
					_, evict, deleted := f.arc.Put(dir, nil)
					if evict != nil {
						f.deleteDir(evict.(string))
					}
					if deleted != nil {
						f.deleteDir(deleted.(string))
					}
				}
			}
		}

		return nil
	})
}

func (f *FileMgrWithARC) onGetARC(key string) {
	blog.Infof("FileMgrWithARC:on get key %s", key)

	f.mutexARC.Lock()
	defer f.mutexARC.Unlock()

	_, _, evict, deleted := f.arc.Get(key)
	if evict != nil {
		f.deleteDir(evict.(string))
	}
	if deleted != nil {
		f.deleteDir(deleted.(string))
	}
}

func (f *FileMgrWithARC) onPutARC(key string) {
	blog.Infof("FileMgrWithARC:on put key %s", key)

	f.mutexARC.Lock()
	defer f.mutexARC.Unlock()

	_, evict, deleted := f.arc.Put(key, nil)
	if evict != nil {
		f.deleteDir(evict.(string))
	}
	if deleted != nil {
		f.deleteDir(deleted.(string))
	}
}

// 处理被淘汰的目录
// 先简单改个名字，避免当前正在被使用
// 待冷却一段时间（比如访问时间超过半个小时），通过轮询删除
func (f *FileMgrWithARC) deleteDir(dir string) {
	blog.Infof("FileMgrWithARC:ready delete dir %s", dir)
	if file.Stat(dir).Exist() {
		newname := fmt.Sprintf("%s_%s_%d_%s", dir, util.RandomString(8), time.Now().UnixNano(), DeleteFlag)
		err := os.Rename(dir, newname)
		blog.Infof("FileMgrWithARC: rename %s to %s with error:%v", dir, newname, err)
		if err == nil {
			f.addDelete(newname)
		}
	}

	f.deleteDirLock(dir)
}

func (f *FileMgrWithARC) addDelete(key string) {
	f.deleteMutex.Lock()
	defer f.deleteMutex.Unlock()

	f.deletedFiles[key] = time.Now()
}

func (f *FileMgrWithARC) ticker() {
	blog.Infof("FileMgrWithARC: start ticker now")

	ticker := time.NewTicker(CheckTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			blog.Debugf("FileMgrWithARC: on ticker now...")
			// 最合理的策略，是根据最后的访问时间决定是否可以删除；
			// 但当我们去取这个时间时，我们就成了最后的访问者（当观察者介入，就成了被观察世界的一部分）
			// 所以先简单的以该文件放入内存中的时间为标准
			f.deleteMutex.Lock()
			now := time.Now()
			for k, v := range f.deletedFiles {
				if now.After(v.Add(DeleteDuration)) {
					// TODO : 处理下remove的失败？
					err := os.RemoveAll(k)
					blog.Infof("FileMgrWithARC: remove %s with error:%v", k, err)
					delete(f.deletedFiles, k)
				}
			}
			f.deleteMutex.Unlock()
		}
	}
}

type Result struct {
	FileName        string
	CompressDataBuf []byte
	CompressType    protocol.CompressType
	RealSize        uint64
	HashStr         string
}

func (r *Result) generateFileName() string {
	if r.HashStr == "" {
		hashbyte := xxhash.Sum64(r.CompressDataBuf)
		r.HashStr = fmt.Sprintf("%x", hashbyte)
	}
	return fmt.Sprintf("%s_%d_%s_%s", r.HashStr, r.RealSize, r.CompressType.String(), r.FileName)
}

func getFileNameWithoutExtension(fullPath string) string {
	fileNameWithExt := filepath.Base(fullPath)
	fileName := strings.TrimSuffix(fileNameWithExt, filepath.Ext(fileNameWithExt))
	return fileName
}

func ToResult(f string, needUnzip bool) *Result {
	blog.Infof("resultcache: ready load file %s to result", f)

	r := &Result{}
	r.FileName = filepath.Base(f)

	fname := getFileNameWithoutExtension(f)
	fields := strings.Split(fname, "_")
	if len(fields) >= 3 {
		hashstrfromname, realsizestr, compresstypestr := fields[0], fields[1], fields[2]

		realsize, err := strconv.ParseUint(realsizestr, 10, 64)
		if err != nil {
			blog.Warnf("resultcache: file %s name resolve with error:%v", f, err)
			return nil
		}
		r.RealSize = realsize

		compressType := protocol.CompressNone
		if compresstypestr != "" {
			if compresstypestr == "lz4" {
				compressType = protocol.CompressLZ4
			}

			file, err := os.Open(f)
			if err != nil {
				blog.Warnf("resultcache: failed to open file:%s", f)
				return nil
			}
			defer file.Close()

			data, err := io.ReadAll(file)
			if err != nil {
				blog.Warnf("resultcache: file %s read with error:%v", f, err)
				return nil
			}

			hashbyte := xxhash.Sum64(data)
			nowhashstr := fmt.Sprintf("%x", hashbyte)

			if hashstrfromname != nowhashstr {
				blog.Warnf("resultcache: file %s hash %s not equal newly hash %s", f, hashstrfromname, nowhashstr)
				return nil
			}

			// 返回前先解压
			if needUnzip && compressType == protocol.CompressLZ4 {
				dst := make([]byte, realsize)
				outdata, err := dcUtil.Lz4Uncompress(data, dst)
				if err != nil {
					blog.Warnf("resultcache: decompress [%s] with error:%v", f, err)
					return nil
				}
				outlen := len(outdata)
				if uint64(outlen) != realsize {
					blog.Warnf("resultcache: file %s decompress size %d not equal realsize %d", f, outlen, realsize)
					return nil
				}
				blog.Infof("resultcache: file %s decompress from %d to %d", f, len(data), outlen)
				data = outdata
				compressType = protocol.CompressNone
			}

			r.CompressDataBuf = data
			r.CompressType = compressType

			return r
		}
	}

	blog.Warnf("resultcache: file name %s is invalid", f)
	return nil
}

func (r *Result) save(dir string) error {
	fp := filepath.Join(dir, r.generateFileName())
	f, err := os.Create(fp)
	if err != nil {
		blog.Errorf("Result: create file %s error: %v", fp, err)
		return err
	}
	defer f.Close()

	_, err = f.Write(r.CompressDataBuf)
	if err != nil {
		blog.Errorf("Result: save file [%s] error: %v", fp, err)
		return err
	}

	blog.Infof("Result: succeed save file %s with r.CompressDataBuf %v", fp, len(r.CompressDataBuf))
	return nil
}

func getDir(fileDir, groupkey, resultkey string) (string, error) {
	if len(resultkey) < 3 || groupkey == "" {
		err := fmt.Errorf("result key %s too short or group key[%s] invalid",
			resultkey, groupkey)
		blog.Warnf("resultcache: get dir with error:%v", err)
		return "", err
	}

	dir := filepath.Join(fileDir, groupkey, string(resultkey[0]), string(resultkey[1]), resultkey)
	return dir, nil
}

func (fmgr *FileMgrWithARC) GetResult(groupkey, resultkey string, needUnzip bool) ([]*Result, error) {
	dir, err := getDir(fmgr.filedir, groupkey, resultkey)
	if err != nil {
		blog.Warnf("FileMgrWithARC: get dir by groupkey %s resultkey %s with error:%v",
			groupkey, resultkey, err)
		return nil, err
	}

	dirlock := fmgr.getDirLock(dir)
	if dirlock != nil {
		dirlock.RLock()
		defer dirlock.RUnlock()
	}

	f, err := os.Open(dir)
	if err != nil {
		blog.Warnf("FileMgrWithARC: failed to open dir %s with error:%v", dir, err)
		return nil, err
	}
	fis, _ := f.Readdir(-1)
	f.Close()

	rs := make([]*Result, 0, len(fis))
	for _, fi := range fis {
		f := filepath.Join(dir, fi.Name())
		if strings.HasSuffix(f, RecordFileName) {
			continue
		}
		r := ToResult(f, needUnzip)
		if r != nil {
			rs = append(rs, r)
		}
	}

	go fmgr.onGetARC(dir)

	return rs, nil
}

func (fmgr *FileMgrWithARC) PutResult(groupkey, resultkey string, rs []*Result, record Record) error {
	dir, err := getDir(fmgr.filedir, groupkey, resultkey)
	if err != nil {
		blog.Warnf("FileMgrWithARC: get dir by group key %s result key: %s with error:%v",
			groupkey, resultkey, err)
		return err
	}

	// 同一个hashkey，保留一份结果就够了
	if dcFile.Stat(dir).Exist() {
		return nil
	}

	dirlock := fmgr.getDirLock(dir)
	if dirlock != nil {
		dirlock.Lock()
		defer dirlock.Unlock()
	}

	// 同一个hashkey，保留一份结果就够了
	if dcFile.Stat(dir).Exist() {
		return nil
	}

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		blog.Warnf("FileMgrWithARC: create dir %s with error:%v", dir, err)
		return err
	}

	for _, r := range rs {
		err = r.save(dir)
		if err != nil {
			return err
		}
	}

	// 额外存储来源信息
	if record != nil {
		saveRecordFile(dir, record.ToString())
	}

	go fmgr.onPutARC(dir)

	return nil
}

func saveRecordFile(dir string, data string) error {
	if data == "" {
		return nil
	}

	fp := filepath.Join(dir, RecordFileName)
	f, err := os.Create(fp)
	if err != nil {
		blog.Errorf("FileMgrWithARC: create file %s error: %v", fp, err)
		return err
	}
	defer f.Close()

	_, err = f.Write([]byte(data))
	if err != nil {
		blog.Errorf("FileMgrWithARC: save file [%s] error: %v", fp, err)
		return err
	}

	blog.Infof("FileMgrWithARC: succeed save file %s", fp)
	return nil
}

func (fmgr *FileMgrWithARC) getDirLock(dir string) *sync.RWMutex {
	fmgr.fileDirLock.Lock()
	onedirlock, ok := fmgr.fileDirLocks[dir]
	if !ok {
		onedirlock = new(sync.RWMutex)
		fmgr.fileDirLocks[dir] = onedirlock
		blog.Infof("FileMgrWithARC: generate lock for dir %s", dir)
	}
	fmgr.fileDirLock.Unlock()

	return onedirlock
}

func (fmgr *FileMgrWithARC) deleteDirLock(dir string) error {
	fmgr.fileDirLock.Lock()
	_, ok := fmgr.fileDirLocks[dir]
	if ok {
		delete(fmgr.fileDirLocks, dir)
		blog.Infof("FileMgrWithARC: delete lock for dir %s", dir)
	}
	fmgr.fileDirLock.Unlock()

	return nil
}
