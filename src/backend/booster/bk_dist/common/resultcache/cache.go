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

	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/cespare/xxhash/v2"
)

type Result struct {
	Suffix          string
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
	return fmt.Sprintf("%s_%d_%s%s", r.HashStr, r.RealSize, r.CompressType.String(), r.Suffix)
}

func getFileNameWithoutExtension(fullPath string) string {
	fileNameWithExt := filepath.Base(fullPath)
	fileName := strings.TrimSuffix(fileNameWithExt, filepath.Ext(fileNameWithExt))
	return fileName
}

func ToResult(f string) *Result {
	blog.Infof("resultcache: ready load file %s to result", f)

	r := &Result{}
	r.Suffix = filepath.Ext(f)

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
			if compressType == protocol.CompressLZ4 {
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
		blog.Errorf("resultcache: create file %s error: %v", fp, err)
		return err
	}
	defer f.Close()

	_, err = f.Write(r.CompressDataBuf)
	if err != nil {
		blog.Errorf("resultcache: save file [%s] error: %v", fp, err)
		return err
	}

	blog.Infof("resultcache: succeed save file %s", fp)
	return nil
}

type CacheWay int

const (
	CacheLocal CacheWay = iota
	CacheRemote
	CacheBoth
)

var (
	instance *ResultCache
	once     sync.Once
)

type ResultCache struct {
	LocalSaveDir string
	RemoteServer string
}

func GetInstance(localdir string, server string) *ResultCache {
	once.Do(func() {
		if localdir == "" {
			localdir = dcUtil.GetResultCacheDir()
		}
		instance = &ResultCache{
			LocalSaveDir: localdir,
			RemoteServer: server,
		}
	})
	return instance
}

func (rc *ResultCache) GetResult(key string, cw CacheWay) ([]*Result, error) {
	if cw == CacheLocal {
		return rc.getLocal(key)
	}

	err := fmt.Errorf("not supported cacheway:%d", cw)
	return nil, err
}

func (rc *ResultCache) PutResult(key string, rs []*Result, cw CacheWay) error {
	if len(rs) == 0 {
		return nil
	}

	if cw == CacheLocal {
		return rc.putLocal(key, rs)
	}

	err := fmt.Errorf("not supported cacheway:%d", cw)
	return err
}

func (rc *ResultCache) getDir(key string) (string, error) {
	if len(key) < 3 {
		err := fmt.Errorf("key %s it too short", key)
		blog.Warnf("resultcache: save local with error:%v", err)
		return "", err
	}

	dir := filepath.Join(rc.LocalSaveDir, string(key[0]), string(key[1]), key)
	return dir, nil
}

func (rc *ResultCache) getLocal(key string) ([]*Result, error) {
	dir, err := rc.getDir(key)
	if err != nil {
		blog.Warnf("resultcache: get dir by key %s with error:%v", key, err)
		return nil, err
	}

	f, err := os.Open(dir)
	if err != nil {
		blog.Warnf("resultcache: failed to open dir %s with error:%v", dir, err)
		return nil, err
	}
	fis, _ := f.Readdir(-1)
	f.Close()

	rs := make([]*Result, 0, len(fis))
	for _, fi := range fis {
		f := filepath.Join(dir, fi.Name())
		r := ToResult(f)
		if r != nil {
			rs = append(rs, r)
		}
	}

	return rs, nil
}

func (rc *ResultCache) putLocal(key string, rs []*Result) error {
	dir, err := rc.getDir(key)
	if err != nil {
		blog.Warnf("resultcache: get dir by key %s with error:%v", key, err)
		return err
	}

	os.RemoveAll(dir)
	if !dcFile.Stat(dir).Exist() {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			blog.Warnf("resultcache: create dir %s with error:%v", dir, err)
			return err
		}
	}

	for _, r := range rs {
		err = r.save(dir)
		if err != nil {
			return err
		}
	}

	return nil
}
