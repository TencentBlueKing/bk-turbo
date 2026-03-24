/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package client

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"runtime/debug"
	"sync"
	"time"

	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/shirou/gopsutil/mem"
)

const (
	cleanInterval = 100 * time.Millisecond

	// 这儿是压缩后的大小，一般为原始大小的1/3不到
	smallFileSize = 1 * 1024 * 1024
	bigFileSize   = 30 * 1024 * 1024

	maxCachedSeconds = 300 * time.Second

	defaultCacheSize = int64(4 * 1024 * 1024 * 1024)
	reservedSize     = int64(2 * 1024 * 1024 * 1024)
)

var (
	errorNotRunning  = fmt.Errorf("this handle not running")
	errorContextExit = fmt.Errorf("context exit")
	errorNotInCache  = fmt.Errorf("not in cache")
)

//
func newFileDataCache(ctx context.Context) *fileDataCache {
	blog.Infof("filedatacache: newFileDataCache now ")

	ret := &fileDataCache{
		queryChan: make(chanChanRequest, 1000),
		handling:  false,
		cache:     newInnerFileDataCache(),
	}

	ret.Handle(ctx)

	return ret
}

type fileDataResult struct {
	buf *[]byte
	err error
}

type chanFileDataResult chan fileDataResult

type chanFileDataRequest struct {
	result   chanFileDataResult
	fullpath string
	// 只查询缓存，不触发文件的读取，如果不存在，返回nil
	readanyway bool
}

type chanChanRequest chan chanFileDataRequest

type fileDataCache struct {
	ctx       context.Context
	queryChan chanChanRequest
	handling  bool
	cache     *innerFileDataCache
}

// brings handler up and begin to handle requests
func (fdc *fileDataCache) Handle(ctx context.Context) {
	if fdc.handling {
		return
	}

	fdc.handling = true

	go fdc.handle(ctx)
}

func (fdc *fileDataCache) Query(fullpath string, readanyway bool) (*[]byte, error) {
	if !fdc.handling {
		return nil, errorNotRunning
	}

	blog.Infof("filedatacache: ready query file %s now", fullpath)
	msg := chanFileDataRequest{
		fullpath:   fullpath,
		result:     make(chanFileDataResult, 1),
		readanyway: readanyway,
	}
	fdc.queryChan <- msg

	select {
	case <-fdc.ctx.Done():
		return nil, errorContextExit

	// wait result
	case r := <-msg.result:
		return r.buf, r.err
	}
}

func (fdc *fileDataCache) handle(ctx context.Context) {
	blog.Infof("filedatacache: handle now")

	fdc.ctx = ctx
	tick := time.NewTicker(cleanInterval)
	defer tick.Stop()

	// TODO : 需要清理缓存，否则内存会爆
	for {
		select {
		case <-ctx.Done():
			fdc.handling = false
			return
		case req := <-fdc.queryChan:
			fdc.cache.query(req)
		case <-tick.C:
			fdc.cache.clean()
		}
	}
}

// ---------------------------------------------------------

type dataCacheStatus int

const (
	dataCacheInit dataCacheStatus = iota
	dataCacheReading
	dataCacheCompressing
	dataCacheCompressed
)

type oneFileCache struct {
	fullpath         string
	filesize         int64
	compressedBuf    *[]byte
	compressedBufLen int64
	modifiedTime     time.Time
	err              error
	status           dataCacheStatus
	hits             int

	pushTime time.Time

	lock sync.RWMutex
}

type innerFileDataCache struct {
	lock    sync.RWMutex
	files   map[string]*oneFileCache
	maxSize int64
	curSize int64
}

func newInnerFileDataCache() *innerFileDataCache {
	size := defaultCacheSize
	v, err := mem.VirtualMemory()
	if err == nil {
		realmem := int64(v.Total) / 4
		if realmem > 0 {
			size = realmem
		}
	}

	return &innerFileDataCache{
		files:   make(map[string]*oneFileCache, 2000),
		maxSize: size,
		curSize: 0,
	}
}

// 清理缓存
func (ifdc *innerFileDataCache) clean() {
	blog.Debugf("filedatacache: clean now")

	ifdc.lock.Lock()
	defer ifdc.lock.Unlock()

	if len(ifdc.files) > 0 {
		// 先清理缓存超时的
		// ifdc.cleanByTime()

		ifdc.curSize = 0
		maxhits := 0
		for _, v := range ifdc.files {
			ifdc.curSize += v.compressedBufLen
			if v.hits > maxhits {
				maxhits = v.hits
			}
		}
		blog.Debugf("filedatacache: clean check cursize:%d,maxsize:%d",
			ifdc.curSize, ifdc.maxSize)

		needclean := false
		// 按指定的缓存大小清理
		if ifdc.curSize > ifdc.maxSize {
			blog.Infof("filedatacache: cursize:%d,maxsize:%d need clean now",
				ifdc.curSize, ifdc.maxSize)

			needclean = true
			cleanSize := ifdc.cleanBySize(ifdc.curSize-ifdc.maxSize, maxhits)
			ifdc.curSize -= cleanSize

			blog.Infof("filedatacache: after clean by size cursize:%d,maxsize:%d ",
				ifdc.curSize, ifdc.maxSize)
		}

		// 按系统当前内存情况清理
		v, err := mem.VirtualMemory()
		if err != nil {
			blog.Warnf("filedatacache get virtual memory failed with error:%v", err)
		} else {
			if int64(v.Available) < reservedSize {
				blog.Infof("filedatacache: cursize:%d,maxsize:%d Available:%d reservedSize:%d"+
					" need clean now",
					ifdc.curSize, ifdc.maxSize, v.Available, reservedSize)

				needclean = true
				cleanSize := ifdc.cleanBySize(reservedSize-int64(v.Available), maxhits)
				ifdc.curSize -= cleanSize

				blog.Infof("filedatacache: after clean by real memory "+
					"cursize:%d,maxsize:%d Available:%d reservedSize:%d"+
					" need clean now",
					ifdc.curSize, ifdc.maxSize, v.Available, reservedSize)
			}
		}

		if needclean {
			debug.FreeOSMemory() // 尽快将内存还给系统
		}
	}
}

// 清理超时的缓存
func (ifdc *innerFileDataCache) cleanByTime() int64 {
	var gatherSize int64
	cleankeys := []string{}

	now := time.Now()
	for k, v := range ifdc.files {
		if now.Sub(v.pushTime) > maxCachedSeconds {
			cleankeys = append(cleankeys, k)
			gatherSize += v.compressedBufLen
			blog.Infof("filedatacache: file %s cached over %v ,clean now",
				v.fullpath, maxCachedSeconds)
		}
	}

	for _, k := range cleankeys {
		delete(ifdc.files, k)
	}

	if gatherSize > 0 {
		blog.Infof("filedatacache: clean over time %d files,total size: %d",
			len(cleankeys), gatherSize)
	}

	return gatherSize
}

// 考虑优先清理中等大小的文件（1MB~100MB)
// 保留小文件，可以减少读取的文件数，提高磁盘读写能力
// 保留大文件，可以减少数据压缩的开销
func (ifdc *innerFileDataCache) cleanBySize(size int64, maxhits int) int64 {
	var gatherSize int64
	cleankeys := []string{}
	enought := false

	// 先清理hit最多的
	if maxhits > 0 {
		blog.Infof("filedatacache: ready clean files with hits: %d", maxhits)
		for k, v := range ifdc.files {
			if v.hits == maxhits {
				cleankeys = append(cleankeys, k)
				blog.Infof("filedatacache: clean hits files %s with size: %d hits: %d",
					k, v.compressedBufLen, v.hits)
				gatherSize += v.compressedBufLen
				if gatherSize > size {
					enought = true
					break
				}
			}
		}

		for _, k := range cleankeys {
			delete(ifdc.files, k)
		}

		cleankeys = []string{}
	}

	for k, v := range ifdc.files {
		if v.compressedBufLen >= smallFileSize &&
			v.compressedBufLen <= bigFileSize {
			cleankeys = append(cleankeys, k)
			blog.Infof("filedatacache: clean middle files %s with size: %d hits: %d",
				k, v.compressedBufLen, v.hits)
			gatherSize += v.compressedBufLen
			if gatherSize > size {
				enought = true
				break
			}
		}
	}

	if !enought {
		for k, v := range ifdc.files {
			if v.compressedBufLen < smallFileSize &&
				v.compressedBufLen > 0 {
				cleankeys = append(cleankeys, k)
				blog.Infof("filedatacache: clean small files %s with size: %d hits: %d",
					k, v.compressedBufLen, v.hits)
				gatherSize += v.compressedBufLen
				if gatherSize > size {
					enought = true
					break
				}
			}
		}
	}

	if !enought {
		for k, v := range ifdc.files {
			if v.compressedBufLen > bigFileSize {
				cleankeys = append(cleankeys, k)
				blog.Infof("filedatacache: clean big files %s with size: %d hits: %d",
					k, v.compressedBufLen, v.hits)
				gatherSize += v.compressedBufLen
				if gatherSize > size {
					enought = true
					break
				}
			}
		}
	}

	for _, k := range cleankeys {
		delete(ifdc.files, k)
	}

	return gatherSize
}

// 查询缓存
func (ifdc *innerFileDataCache) query(req chanFileDataRequest) {
	blog.Debugf("filedatacache: innerFileDataCache query now")

	ifdc.lock.Lock()
	defer ifdc.lock.Unlock()

	fileInfo, err := os.Lstat(req.fullpath)
	if err != nil {
		req.result <- fileDataResult{
			buf: nil,
			err: err,
		}
		return
	}

	var f *oneFileCache
	var ok bool
	if f, ok = ifdc.files[req.fullpath]; !ok {
		// 如果没有指定要读，则返回
		if !req.readanyway {
			result := fileDataResult{
				buf: nil,
				err: errorNotInCache,
			}
			req.result <- result
			return
		}

		f = &oneFileCache{
			fullpath:         req.fullpath,
			filesize:         fileInfo.Size(),
			compressedBuf:    nil,
			compressedBufLen: 0,
			modifiedTime:     fileInfo.ModTime(),
			err:              nil,
			status:           dataCacheInit,
			hits:             0,
			pushTime:         time.Now(),
		}

		ifdc.files[req.fullpath] = f
	}

	go ifdc.check(f, req.result, fileInfo, req.readanyway)
}

// 该文件可能在处理中，用锁互斥，保证只有一个在操作中
func (ifdc *innerFileDataCache) check(
	f *oneFileCache,
	c chanFileDataResult,
	fi fs.FileInfo,
	readanyway bool) {
	blog.Debugf("filedatacache: innerFileDataCache check %s now", f.fullpath)
	f.lock.Lock()
	defer f.lock.Unlock()

	notChanged := f.modifiedTime.Equal(fi.ModTime())
	if notChanged {
		if f.err != nil {
			result := fileDataResult{
				buf: nil,
				err: f.err,
			}
			c <- result
			blog.Infof("filedatacache: file %s hit cache with error:%v", f.fullpath, f.err)
			return
		}

		if f.status == dataCacheCompressed {
			result := fileDataResult{
				buf: f.compressedBuf,
				err: nil,
			}
			c <- result
			f.hits += 1
			blog.Infof("filedatacache: file %s origin size %d compressed size %d hit cache succeed",
				f.fullpath, f.filesize, f.compressedBufLen)
			return
		}

		// ! should not come this
		if f.status == dataCacheReading || f.status == dataCacheCompressing {
			blog.Warnf("filedatacache: why come this with status:%d???", f.status)
		}
	}

	if !readanyway {
		result := fileDataResult{
			buf: nil,
			err: nil,
		}
		c <- result
		return
	}

	// reload anyway
	ifdc.load(f, c, fi)
}

func (ifdc *innerFileDataCache) load(f *oneFileCache, c chanFileDataResult, fi fs.FileInfo) {
	blog.Debugf("filedatacache: innerFileDataCache load %s now", f.fullpath)
	var outdata []byte
	var result fileDataResult

	f.status = dataCacheReading
	// read file
	data, err := ioutil.ReadFile(f.fullpath)
	if err != nil {
		blog.Warnf("filedatacache: read file[%s] failed with error:%v", f.fullpath, err)
		goto ERROR
	}

	// 压缩数据，先只支持LZ4
	f.status = dataCacheCompressing
	outdata, err = dcUtil.Lz4Compress(data)
	if err != nil {
		blog.Warnf("filedatacache: compress file[%s] failed with error:%v", f.fullpath, err)
		goto ERROR
	}

	f.compressedBuf = &outdata
	f.compressedBufLen = int64(len(outdata))
	f.err = nil
	f.status = dataCacheCompressed
	f.pushTime = time.Now()

	result = fileDataResult{
		buf: f.compressedBuf,
		err: nil,
	}
	c <- result
	blog.Infof("filedatacache: file %s origin size %d compressed size %d load succeed",
		f.fullpath, f.filesize, f.compressedBufLen)
	return

ERROR:
	f.compressedBuf = nil
	f.err = err
	result = fileDataResult{
		buf: nil,
		err: f.err,
	}
	c <- result
	return
}
