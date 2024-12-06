/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/client"
	workerType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

const (
	// cork机制用于减少和worker之间的短连接
	// corkSize = 1024 * 10
	corkSize = 0
	// corkMaxSize   = 1024 * 1024 * 50
	corkMaxSize = 1024 * 1024 * 10
	// corkMaxSize   = 1024 * 1024 * 1024
	largeFileSize = 1024 * 1024 * 100 // 100MB
)

// NewMgr get a new Remote Mgr
func NewMgr(pCtx context.Context, work *types.Work) types.RemoteMgr {
	ctx, _ := context.WithCancel(pCtx)

	blog.Infof("remote: new remote mgr with corkSize:%d corkMaxSize:%d largeFileSize:%d",
		corkSize, corkMaxSize, largeFileSize)

	var remoteSlotMgr RemoteSlotMgr
	if work.Config().WorkerOfferSlot {
		remoteSlotMgr = newWorkerOfferResource(nil)
	} else {
		remoteSlotMgr = newResource(nil)
	}

	return &Mgr{
		ctx:      ctx,
		work:     work,
		resource: remoteSlotMgr,
		remoteWorker: client.NewCommonRemoteWorkerWithSlot(
			ctx,
			work.Config().SendFileMemoryLimit,
			work.Config().SendMemoryCache,
		),
		checkSendFileTick:     100 * time.Millisecond,
		fileSendMap:           make(map[string]*fileSendMap),
		failFileSendMap:       make(map[string]*fileSendMap),
		fileCollectionSendMap: make(map[string]*[]*types.FileCollectionInfo),
		fileMessageBank:       newFileMessageBank(),
		conf:                  work.Config(),
		resourceCheckTick:     5 * time.Second,
		workerCheckTick:       5 * time.Second,
		retryCheckTick:        10 * time.Second,
		sendCorkTick:          10 * time.Millisecond,
		corkSize:              corkSize,
		corkMaxSize:           corkMaxSize,
		corkFiles:             make(map[string]*[]*corkFile, 0),
		// memSlot:               newMemorySlot(work.Config().SendFileMemoryLimit),
		largeFileSize: largeFileSize,
	}
}

const (
	syncHostTimeTimes = 3
)

// Mgr describe the remote manager
// provides the actions handler to remote workers
type Mgr struct {
	ctx context.Context

	work *types.Work
	// resource     *resource
	resource     RemoteSlotMgr
	remoteWorker dcSDK.RemoteWorker

	// memSlot *memorySlot

	checkSendFileTick time.Duration

	fileSendMutex sync.RWMutex
	fileSendMap   map[string]*fileSendMap

	failFileSendMutex sync.RWMutex
	failFileSendMap   map[string]*fileSendMap

	fileCollectionSendMutex sync.RWMutex
	fileCollectionSendMap   map[string]*[]*types.FileCollectionInfo

	fileMessageBank *fileMessageBank

	// initCancel context.CancelFunc

	conf *config.ServerConfig

	resourceCheckTick time.Duration
	workerCheckTick   time.Duration
	retryCheckTick    time.Duration
	lastUsed          uint64 // only accurate to second now
	lastApplied       uint64 // only accurate to second now
	remotejobs        int64  // save job number which using remote worker

	sendCorkTick time.Duration
	sendCorkChan chan bool
	corkMutex    sync.RWMutex
	corkSize     int64
	corkMaxSize  int64
	corkFiles    map[string]*[]*corkFile

	largeFileSize int64
}

type fileSendMap struct {
	sync.RWMutex
	cache map[string]*[]*types.FileInfo
}

func (fsm *fileSendMap) matchOrInsert(desc dcSDK.FileDesc) (*types.FileInfo, bool) {
	fsm.Lock()
	defer fsm.Unlock()

	if fsm.cache == nil {
		fsm.cache = make(map[string]*[]*types.FileInfo)
	}

	info := &types.FileInfo{
		FullPath:           desc.FilePath,
		Size:               desc.FileSize,
		LastModifyTime:     desc.Lastmodifytime,
		Md5:                desc.Md5,
		TargetRelativePath: desc.Targetrelativepath,
		FileMode:           desc.Filemode,
		LinkTarget:         desc.LinkTarget,
		SendStatus:         types.FileSending,
	}

	c, ok := fsm.cache[desc.FilePath]
	if !ok || c == nil || len(*c) == 0 {
		infoList := []*types.FileInfo{info}
		fsm.cache[desc.FilePath] = &infoList
		return info, false
	}

	for _, ci := range *c {
		if ci.Match(desc) {
			return ci, true
		}
	}

	*c = append(*c, info)
	return info, false
}

// 仅匹配失败文件，不执行插入
func (fsm *fileSendMap) matchFail(desc dcSDK.FileDesc, query bool) (*types.FileInfo, bool, error) {
	fsm.Lock()
	defer fsm.Unlock()

	if fsm.cache == nil {
		return nil, false, errors.New("file cache not found")
	}

	c, ok := fsm.cache[desc.FilePath]
	if !ok || c == nil || len(*c) == 0 {
		return nil, false, fmt.Errorf("file %s not found, file cache is nil", desc.FilePath)
	}

	for _, ci := range *c {
		if ci.Match(desc) {
			if ci.SendStatus == types.FileSendFailed && !query {
				ci.SendStatus = types.FileSending
				return ci, false, nil
			}
			return ci, true, nil
		}
	}
	return nil, false, fmt.Errorf("file %s not found", desc.FilePath)
}

// 仅匹配失败文件，不执行插入
func (fsm *fileSendMap) matchFails(descs []*dcSDK.FileDesc) []matchResult {
	fsm.Lock()
	defer fsm.Unlock()

	if fsm.cache == nil {
		fsm.cache = make(map[string]*[]*types.FileInfo)
		blog.Warnf("file: fail cache not found")
	}

	result := make([]matchResult, 0, len(descs))
	for _, desc := range descs {
		info := &types.FileInfo{
			FullPath:           desc.FilePath,
			Size:               desc.FileSize,
			LastModifyTime:     desc.Lastmodifytime,
			Md5:                desc.Md5,
			TargetRelativePath: desc.Targetrelativepath,
			FileMode:           desc.Filemode,
			LinkTarget:         desc.LinkTarget,
			SendStatus:         types.FileSendFailed,
		}
		c, ok := fsm.cache[desc.FilePath]
		if !ok || c == nil || len(*c) == 0 {
			//失败文件未找到，直接返回失败，不插入
			result = append(result, matchResult{
				info:  info,
				match: true,
			})
			blog.Warnf("file: fail file %s not found", desc.FilePath)
			continue
		}
		matched := false
		for _, ci := range *c {
			if ci.Match(*desc) {
				fileMatched := true
				if ci.SendStatus == types.FileSendFailed {
					ci.SendStatus = types.FileSending
					fileMatched = false
				}
				result = append(result, matchResult{
					info:  ci,
					match: fileMatched,
				})
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		//失败文件未找到，直接返回失败，不插入
		blog.Warnf("fail: fail file %s not found", desc.FilePath)
		result = append(result, matchResult{
			info:  info,
			match: true,
		})
	}
	return result
}

func (fsm *fileSendMap) matchOrInserts(descs []*dcSDK.FileDesc) []matchResult {
	fsm.Lock()
	defer fsm.Unlock()

	if fsm.cache == nil {
		fsm.cache = make(map[string]*[]*types.FileInfo)
	}

	result := make([]matchResult, 0, len(descs))
	for _, desc := range descs {
		info := &types.FileInfo{
			FullPath:           desc.FilePath,
			Size:               desc.FileSize,
			LastModifyTime:     desc.Lastmodifytime,
			Md5:                desc.Md5,
			TargetRelativePath: desc.Targetrelativepath,
			FileMode:           desc.Filemode,
			LinkTarget:         desc.LinkTarget,
			SendStatus:         types.FileSending,
		}

		c, ok := fsm.cache[desc.FilePath]
		if !ok || c == nil || len(*c) == 0 {
			infoList := []*types.FileInfo{info}
			fsm.cache[desc.FilePath] = &infoList
			result = append(result, matchResult{
				info:  info,
				match: false,
			})
			continue
		}

		matched := false
		for _, ci := range *c {
			if ci.Match(*desc) {
				result = append(result, matchResult{
					info:  ci,
					match: true,
				})
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		*c = append(*c, info)
		result = append(result, matchResult{
			info:  info,
			match: false,
		})
	}

	return result
}

func (fsm *fileSendMap) updateFailStatus(desc dcSDK.FileDesc, status types.FileSendStatus) {
	fsm.Lock()
	defer fsm.Unlock()

	info := &types.FileInfo{
		FullPath:           desc.FilePath,
		Size:               desc.FileSize,
		LastModifyTime:     desc.Lastmodifytime,
		Md5:                desc.Md5,
		TargetRelativePath: desc.Targetrelativepath,
		FileMode:           desc.Filemode,
		LinkTarget:         desc.LinkTarget,
		SendStatus:         status,
	}
	if fsm.cache == nil {
		fsm.cache = make(map[string]*[]*types.FileInfo)
	}
	fc, ok := fsm.cache[info.FullPath]
	if !ok || fc == nil || len(*fc) == 0 {
		infoList := []*types.FileInfo{info}
		fsm.cache[info.FullPath] = &infoList
		blog.Debugf("file: update failed files with add:%v", info)
		return
	}
	for _, ci := range *fc {
		if ci.Match(desc) {
			blog.Debugf("file: update failed files with refresh before:%v", ci)
			ci.SendStatus = status
			blog.Debugf("file: update failed files with refresh:%v", ci)
			return
		}
	}
	*fc = append(*fc, info)
}

func (fsm *fileSendMap) updateStatus(desc dcSDK.FileDesc, status types.FileSendStatus) {
	fsm.Lock()
	defer fsm.Unlock()

	if fsm.cache == nil {
		fsm.cache = make(map[string]*[]*types.FileInfo)
	}

	info := &types.FileInfo{
		FullPath:           desc.FilePath,
		Size:               desc.FileSize,
		LastModifyTime:     desc.Lastmodifytime,
		Md5:                desc.Md5,
		TargetRelativePath: desc.Targetrelativepath,
		FileMode:           desc.Filemode,
		LinkTarget:         desc.LinkTarget,
		SendStatus:         status,
	}

	c, ok := fsm.cache[desc.FilePath]
	if !ok || c == nil || len(*c) == 0 {
		infoList := []*types.FileInfo{info}
		fsm.cache[desc.FilePath] = &infoList
		return
	}

	for _, ci := range *c {
		if ci.Match(desc) {
			ci.SendStatus = status
			return
		}
	}

	*c = append(*c, info)
}

func (fsm *fileSendMap) isFilesSendFailed(descs []dcSDK.FileDesc) bool {
	fsm.RLock()
	defer fsm.RUnlock()

	if fsm.cache == nil {
		return false
	}
	for _, desc := range descs {
		c, ok := fsm.cache[desc.FilePath]
		if !ok || c == nil || len(*c) == 0 {
			continue
		}
		for _, ci := range *c {
			if ci.Match(desc) {
				return ci.SendStatus != types.FileSendSucceed
			}
		}
	}

	return false
}

func (fsm *fileSendMap) getFailFiles() []dcSDK.FileDesc {
	fsm.RLock()
	defer fsm.RUnlock()

	failFiles := make([]dcSDK.FileDesc, 0)
	for _, v := range fsm.cache {
		if v == nil || len(*v) == 0 {
			continue
		}
		for _, ci := range *v {
			if ci.SendStatus != types.FileSendFailed {
				continue
			}
			_, err := os.Stat(ci.FullPath)
			if os.IsNotExist(err) {
				blog.Warnf("remote: get fail file %s not exist", ci.FullPath)
				continue
			}
			failFiles = append(failFiles, dcSDK.FileDesc{
				FilePath:           ci.FullPath,
				Compresstype:       dcProtocol.CompressLZ4,
				FileSize:           ci.Size,
				Lastmodifytime:     ci.LastModifyTime,
				Md5:                ci.Md5,
				Targetrelativepath: ci.TargetRelativePath,
				Filemode:           ci.FileMode,
				LinkTarget:         ci.LinkTarget,
				NoDuplicated:       true,
				Retry:              true,
			})
		}
	}
	return failFiles
}

// Init do the initialization for remote manager
// !! only call once !!
func (m *Mgr) Init() {
	blog.Infof("remote: init for work:%s", m.work.ID())

	// settings := m.work.Basic().Settings()
	// m.resource = newResource(m.syncHostTimeNoWait(m.work.Resource().GetHosts()), settings.UsageLimit)
	// m.resource = newResource(m.syncHostTimeNoWait(m.work.Resource().GetHosts()))

	// if m.initCancel != nil {
	// 	m.initCancel()
	// }
	ctx, _ := context.WithCancel(m.ctx)
	// m.initCancel = cancel

	m.resource.Handle(ctx)

	// m.memSlot.Handle(ctx)

	// register call back for resource changed
	m.work.Resource().RegisterCallback(m.callback4ResChanged)

	if m.conf.AutoResourceMgr {
		go m.resourceCheck(ctx)
	}
	if m.work.ID() != "" {
		go m.workerCheck(ctx)
		go m.retryFailFiles(ctx)
		go m.retrySendToolChains(ctx)
	}

	if m.conf.SendCork {
		m.sendCorkChan = make(chan bool, 1000)
		go m.sendFilesWithCorkTick(ctx)
	}
}

func (m *Mgr) Start() {
	blog.Infof("remote: start for work:%s", m.work.ID())
}

func (m *Mgr) callback4ResChanged() error {
	blog.Infof("remote: resource changed call back for work:%s", m.work.ID())

	// TODO : deal with p2p resource

	hl := m.work.Resource().GetHosts()
	m.resource.Reset(hl)
	if hl != nil && len(hl) > 0 {
		m.setLastApplied(uint64(time.Now().Local().Unix()))
		m.syncHostTimeNoWait(hl)
	}

	// if all workers released, we shoud clean the cache now
	if hl == nil || len(hl) == 0 {
		m.cleanFileCache()

		// TODO : do other cleans here
	}
	return nil
}

func (m *Mgr) cleanFileCache() {
	blog.Infof("remote: clean all file cache when all resource released for work:%s", m.work.ID())

	m.fileSendMutex.Lock()
	m.fileSendMap = make(map[string]*fileSendMap)
	m.fileSendMutex.Unlock()

	m.fileCollectionSendMutex.Lock()
	m.fileCollectionSendMap = make(map[string]*[]*types.FileCollectionInfo)
	m.fileCollectionSendMutex.Unlock()

	m.corkMutex.Lock()
	m.corkFiles = make(map[string]*[]*corkFile, 0)
	m.corkMutex.Unlock()
}

func (m *Mgr) setLastUsed(v uint64) {
	atomic.StoreUint64(&m.lastUsed, v)
}

func (m *Mgr) getLastUsed() uint64 {
	return atomic.LoadUint64(&m.lastUsed)
}

func (m *Mgr) setLastApplied(v uint64) {
	atomic.StoreUint64(&m.lastApplied, v)
}

func (m *Mgr) getLastApplied() uint64 {
	return atomic.LoadUint64(&m.lastApplied)
}

// IncRemoteJobs inc remote jobs
func (m *Mgr) IncRemoteJobs() {
	atomic.AddInt64(&m.remotejobs, 1)
}

// DecRemoteJobs dec remote jobs
func (m *Mgr) DecRemoteJobs() {
	atomic.AddInt64(&m.remotejobs, -1)
}

func (m *Mgr) getRemoteJobs() int64 {
	return atomic.LoadInt64(&m.remotejobs)
}

func (m *Mgr) resourceCheck(ctx context.Context) {
	blog.Infof("remote: run remote resource check tick for work: %s", m.work.ID())
	ticker := time.NewTicker(m.resourceCheckTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			blog.Infof("remote: run remote resource check for work(%s) canceled by context", m.work.ID())
			return

		case <-ticker.C:
			if m.getRemoteJobs() <= 0 { // no remote worker in use
				needfree := false
				// 从最近一次使用后的时间开始计算空闲时间
				lastused := m.getLastUsed()
				if lastused > 0 {
					nowsecs := time.Now().Local().Unix()
					if int(uint64(nowsecs)-lastused) > m.conf.ResIdleSecsForFree {
						blog.Infof("remote: ready release remote resource for work(%s) %d"+
							" seconds no used since last used %d",
							m.work.ID(), int(uint64(nowsecs)-lastused), lastused)

						needfree = true
					}
				} else {
					// 从资源申请成功的时间开始计算空闲时间
					lastapplied := m.getLastApplied()
					if lastapplied > 0 {
						nowsecs := time.Now().Local().Unix()
						if int(uint64(nowsecs)-lastapplied) > m.conf.ResIdleSecsForFree {
							blog.Infof("remote: ready release remote resource for work(%s) %d"+
								" seconds no used since last applied %d",
								m.work.ID(), int(uint64(nowsecs)-lastapplied), lastapplied)

							needfree = true
						}
					}
				}

				if needfree {
					// disable all workers and release
					m.resource.DisableAllWorker()
					// clean file cache
					m.cleanFileCache()
					// notify resource release
					m.work.Resource().Release(nil)
					// send and reset stat data
					m.work.Resource().SendAndResetStats(false, []int64{0})

					// 重置最近一次使用时间
					m.setLastUsed(0)
					// 重置资源申请成功的时间
					m.setLastApplied(0)
				}
			}
		}
	}
}

// workerCheck check disconnected worker and recover it when it's available
func (m *Mgr) workerCheck(ctx context.Context) {
	blog.Infof("remote: run worker check tick for work: %s", m.work.ID())
	ticker := time.NewTicker(m.workerCheckTick)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			blog.Infof("remote: run worker check for work(%s) canceled by context", m.work.ID())
			return
		//recover dead worker
		case <-ticker.C:
			handler := m.remoteWorker.Handler(0, nil, nil, nil)
			for _, w := range m.resource.GetDeadWorkers() {
				go func(w *worker) {
					// do not use long tcp here, it's only check
					_, err := handler.ExecuteSyncTime(w.host.Server)
					if err != nil {
						blog.Debugf("remote: try to sync time for host(%s) failed: %v", w.host.Server, err)
						return
					}
					m.resource.RecoverDeadWorker(w)
				}(w)

			}
		}
	}
}

func (m *Mgr) retrySendToolChains(ctx context.Context) {
	ticker := time.NewTicker(m.workerCheckTick)
	defer ticker.Stop()

	var workerStatus sync.Map
	for {
		select {
		case <-ctx.Done():
			blog.Infof("remote: run toolchain check for work(%s) canceled by context", m.work.ID())
			return
		case <-ticker.C:
			if m.failFileSendMap == nil || len(m.failFileSendMap) == 0 {
				continue
			}
			handler := m.remoteWorker.Handler(0, nil, nil, nil)
			hosts := m.work.Resource().GetHosts()
			count := 0
			wg := make(chan string, len(hosts))
			for _, h := range hosts {
				workerNeedRetry := true
				if v, ok := workerStatus.Load(h.Server); ok {
					workerNeedRetry = v.(bool)
				}
				if !workerNeedRetry {
					continue
				}
				fileCollections := m.getFailedFileCollectionByHost(h.Server)
				if len(fileCollections) == 0 {
					continue
				}
				workerStatus.Store(h.Server, false)
				count++
				go m.retrySendToolChain(handler, &types.RemoteTaskExecuteRequest{
					Pid:     0,
					Server:  h,
					Sandbox: &dcSyscall.Sandbox{Dir: ""},
					Stats:   &dcSDK.ControllerJobStats{},
				}, fileCollections, wg)
			}
			go func() {
				for i := 0; i < count; i++ {
					host := <-wg
					workerStatus.Store(host, true)
				}
			}()
		}
	}
}

func (m *Mgr) retryFailFiles(ctx context.Context) {
	ticker := time.NewTicker(m.retryCheckTick)
	defer ticker.Stop()

	var workerStatus sync.Map
	for {
		select {
		case <-ctx.Done():
			blog.Infof("remote: run failfiles check for work(%s) canceled by context", m.work.ID())
			return
		case <-ticker.C:
			if m.failFileSendMap == nil || len(m.failFileSendMap) == 0 {
				continue
			}
			hosts := m.work.Resource().GetHosts()
			wg := make(chan string, len(hosts))
			count := 0
			for _, h := range hosts {
				workerNeedRetry := true
				if v, ok := workerStatus.Load(h.Server); ok {
					workerNeedRetry = v.(bool)
				}
				if !workerNeedRetry {
					continue
				}
				m.failFileSendMutex.Lock()
				sendMap := m.failFileSendMap[h.Server]
				if sendMap == nil {
					m.failFileSendMutex.Unlock()
					blog.Infof("remote: send file for work(%s) with no send map", m.work.ID())
					continue
				}
				m.failFileSendMutex.Unlock()

				failFiles := sendMap.getFailFiles()
				if len(failFiles) == 0 {
					continue
				}
				workerStatus.Store(h.Server, false)
				count++
				go m.retrySendFiles(h, failFiles, wg)
			}
			go func() {
				for i := 0; i < count; i++ {
					host := <-wg
					workerStatus.Store(host, true)
				}
			}()
		}
	}
}

func checkHttpConn(req *types.RemoteTaskExecuteRequest) (*types.RemoteTaskExecuteResult, error) {
	if !types.IsHttpConnStatusOk(req.HttpConnCache, req.HttpConnKey) {
		blog.Errorf("remote: httpconncache exit execute pid(%d) for http connection[%s] error",
			req.Pid, req.HttpConnKey)
		return nil, types.ErrLocalHttpConnDisconnected
	}

	return nil, nil
}

// ExecuteTask run the task in remote worker and ensure the dependent files
func (m *Mgr) ExecuteTask(req *types.RemoteTaskExecuteRequest) (*types.RemoteTaskExecuteResult, error) {
	if m.TotalSlots() <= 0 {
		return nil, types.ErrNoAvailableWorkFound
	}

	if req.Sandbox == nil {
		req.Sandbox = &dcSyscall.Sandbox{}
	}

	blog.Infof("remote: try to execute remote task for work(%s) from pid(%d) with timeout(%d)",
		m.work.ID(), req.Pid, req.IOTimeout)
	defer m.work.Basic().UpdateJobStats(req.Stats)

	dcSDK.StatsTimeNow(&req.Stats.RemoteWorkEnterTime)
	defer dcSDK.StatsTimeNow(&req.Stats.RemoteWorkLeaveTime)
	m.work.Basic().UpdateJobStats(req.Stats)

	hosts := m.work.Resource().GetHosts()
	for _, c := range req.Req.Commands {
		for _, s := range hosts {
			m.failFileSendMutex.Lock()
			f := m.failFileSendMap[s.Server]
			m.failFileSendMutex.Unlock()
			if f == nil {
				continue
			}
			if f.isFilesSendFailed(c.Inputfiles) {
				matched := false
				for _, h := range req.BanWorkerList {
					if h.Equal(s) {
						matched = true
						break
					}
				}
				if !matched {
					req.BanWorkerList = append(req.BanWorkerList, s)
				}
			}
		}
	}
	if len(req.BanWorkerList) == len(hosts) {
		return nil, errors.New("no available worker, all worker are banned")
	}

	blog.Debugf("remote: try to execute remote task for work(%s) from pid(%d) with ban worker list %d, %v", m.work.ID(), req.Pid, len(req.BanWorkerList), req.BanWorkerList)
	// 如果有超过100MB的大文件，则在选择host时，作为选择条件
	fpath, _ := getMaxSizeFile(req, m.largeFileSize)
	req.Server = m.lockSlots(dcSDK.JobUsageRemoteExe, fpath, req.BanWorkerList)
	if req.Server == nil {
		blog.Infof("remote: no available worker for work(%s) from pid(%d) with(%d) ban worker",
			m.work.ID(), req.Pid, len(req.BanWorkerList))
		return nil, errors.New("no available worker")
	}
	blog.Infof("remote: selected host(%s) with large file(%s) for work(%s) from pid(%d)",
		req.Server.Server, fpath, m.work.ID(), req.Pid)

	dcSDK.StatsTimeNow(&req.Stats.RemoteWorkLockTime)
	defer dcSDK.StatsTimeNow(&req.Stats.RemoteWorkUnlockTime)
	defer m.unlockSlots(dcSDK.JobUsageRemoteExe, req.Server)
	m.work.Basic().UpdateJobStats(req.Stats)

	handler := m.remoteWorker.Handler(req.IOTimeout, req.Stats, func() {
		m.work.Basic().UpdateJobStats(req.Stats)
	}, req.Sandbox)

	m.IncRemoteJobs()
	defer func() {
		m.setLastUsed(uint64(time.Now().Local().Unix()))
		m.DecRemoteJobs()
	}()

	ret, err := checkHttpConn(req)
	if err != nil {
		return ret, err
	}

	// 1. send toolchain if required  2. adjust exe remote path for req
	err = m.sendToolchain(handler, req)
	if err != nil {
		blog.Errorf("remote: execute remote task for work(%s) from pid(%d) to server(%s), "+
			"ensure tool chain failed: %v, going to disable host(%s)",
			m.work.ID(), req.Pid, req.Server.Server, err, req.Server.Server)

		m.resource.DisableWorker(req.Server)
		return nil, err
	}

	ret, err = checkHttpConn(req)
	if err != nil {
		return ret, err
	}

	remoteDirs, err := m.ensureFilesWithPriority(handler, req.Pid, req.Sandbox, getFileDetailsFromExecuteRequest(req))
	if err != nil {
		matched := false
		for _, h := range req.BanWorkerList {
			if h.Equal(req.Server) {
				matched = true
				break
			}
		}
		if !matched {
			req.BanWorkerList = append(req.BanWorkerList, req.Server)
		}
		var banlistStr string
		for _, s := range req.BanWorkerList {
			banlistStr = banlistStr + s.Server + ","
		}
		blog.Errorf("remote: execute remote task for work(%s) from pid(%d) to server(%s), "+
			"ensure files failed: %v, after add failed server, banworkerlist is %s", m.work.ID(), req.Pid, req.Server.Server, err, banlistStr)
		return nil, err
	}
	if err = updateTaskRequestInputFilesReady(req, remoteDirs); err != nil {
		blog.Errorf("remote: execute remote task for work(%s) from pid(%d) to server(%s), "+
			"update task input files ready failed: %v", m.work.ID(), req.Pid, req.Server.Server, err)
		return nil, err
	}

	dcSDK.StatsTimeNow(&req.Stats.RemoteWorkStartTime)
	m.work.Basic().UpdateJobStats(req.Stats)

	blog.Infof("remote: try to real execute remote task for work(%s) from pid(%d) with timeout(%d) after send files",
		m.work.ID(), req.Pid, req.IOTimeout)

	ret, err = checkHttpConn(req)
	if err != nil {
		return ret, err
	}

	var result *dcSDK.BKDistResult
	if m.conf.LongTCP {
		if !req.Req.CustomSave {
			result, err = handler.ExecuteTaskLongTCP(req.Server, req.Req)
		} else {
			result, err = handler.ExecuteTaskWithoutSaveFileLongTCP(req.Server, req.Req)
		}
	} else {
		if !req.Req.CustomSave {
			result, err = handler.ExecuteTask(req.Server, req.Req)
		} else {
			result, err = handler.ExecuteTaskWithoutSaveFile(req.Server, req.Req)
		}
	}

	dcSDK.StatsTimeNow(&req.Stats.RemoteWorkEndTime)
	if err != nil {
		if isCaredNetError(err) {
			m.handleNetError(req, err)
		}

		req.BanWorkerList = append(req.BanWorkerList, req.Server)
		blog.Errorf("remote: execute remote task for work(%s) from pid(%d) to server(%s), "+
			"remote execute failed: %v", m.work.ID(), req.Pid, req.Server.Server, err)
		return nil, err
	}

	req.Stats.RemoteWorkSuccess = true
	blog.Infof("remote: success to execute remote task for work(%s) from pid(%d) to server(%s)",
		m.work.ID(), req.Pid, req.Server.Server)
	return &types.RemoteTaskExecuteResult{
		Result: result,
	}, nil
}

// SendFiles send the specific files to remote workers
func (m *Mgr) SendFiles(req *types.RemoteTaskSendFileRequest) ([]string, error) {
	return m.ensureFilesWithPriority(
		m.remoteWorker.Handler(
			0,
			req.Stats,
			func() {
				m.work.Basic().UpdateJobStats(req.Stats)
			},
			nil,
		),
		req.Pid,
		req.Sandbox,
		getFileDetailsFromSendFileRequest(req),
	)
}

func (m *Mgr) retrySendFiles(h *dcProtocol.Host, failFiles []dcSDK.FileDesc, host chan string) {
	blog.Infof("remote: try to retry send fail file for work(%s) from pid(%d) to server %s with fail files %v", m.work.ID(), 1, h.Server, len(failFiles))
	_, err := m.SendFiles(&types.RemoteTaskSendFileRequest{
		Pid:     1,
		Req:     failFiles,
		Server:  h,
		Sandbox: &dcSyscall.Sandbox{Dir: ""},
		Stats:   &dcSDK.ControllerJobStats{},
	})
	if err != nil {
		blog.Errorf("remote: try to retry send fail file for work(%s) from pid(%d) to server %s failed: %v", m.work.ID(), 1, h.Server, err)
	} else {
		blog.Infof("remote: success to retry send fail file for work(%s) from pid(%d) to server %s with fail files %v", m.work.ID(), 1, h.Server, len(failFiles))
	}
	host <- h.Server
}

func (m *Mgr) ensureFilesWithPriority(
	handler dcSDK.RemoteWorkerHandler,
	pid int,
	sandbox *dcSyscall.Sandbox,
	fileDetails []*types.FilesDetails) ([]string, error) {

	// 刷新优先级，windows的先不实现
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		freshPriority(fileDetails)
		for _, v := range fileDetails {
			blog.Debugf("remote: after fresh Priority, file:%+v", *v)
		}
	}

	fileMap := make(map[dcSDK.FileDescPriority]*[]*types.FilesDetails)
	posMap := make(map[dcSDK.FileDescPriority]*[]int)
	var maxP dcSDK.FileDescPriority = 0

	for index, fd := range fileDetails {
		p, _ := posMap[fd.File.Priority]
		f, ok := fileMap[fd.File.Priority]
		if !ok {
			fp := make([]*types.FilesDetails, 0, 10)
			f = &fp
			fileMap[fd.File.Priority] = f

			pp := make([]int, 0, 10)
			p = &pp
			posMap[fd.File.Priority] = p
		}

		*f = append(*f, fd)
		*p = append(*p, index)
		if fd.File.Priority > maxP {
			maxP = fd.File.Priority
		}
	}

	result := make([]string, len(fileDetails))
	for i := dcSDK.MaxFileDescPriority; i <= maxP; i++ {
		p, _ := posMap[i]
		f, ok := fileMap[i]
		if !ok {
			continue
		}

		blog.Infof("remote: try to ensure priority(%d) files(%d) for work(%s) from pid(%d) dir(%s) to server",
			i, len(*f), m.work.ID(), pid, sandbox.Dir)
		r, err := m.ensureFiles(handler, pid, sandbox, *f)
		if err != nil {
			return nil, err
		}

		for index, path := range r {
			result[(*p)[index]] = path
		}
	}
	return result, nil
}

func (m *Mgr) checkAndSendCorkFiles(fs []*corkFile, server string, wg chan error, retry bool) {
	totalFileNum := len(fs)
	descs := make([]*dcSDK.FileDesc, 0, totalFileNum)
	for _, v := range fs {
		descs = append(descs, v.file)
	}
	//results文件需要保证发送顺序和fs相同，否则无法使用fs对应的resultchan
	results := m.checkOrLockCorkFiles(server, descs, retry)
	blog.Debugf("remote: got %d results for %d cork files count:%d for work(%s) to server(%s)",
		len(results), len(descs), m.work.ID(), server)
	needSendCorkFiles := make([]*corkFile, 0, totalFileNum)
	for i, v := range results {
		if v.match {
			// 已发送完成的不启动协程了
			if v.info.SendStatus == types.FileSendSucceed {
				wg <- nil
				continue
			} else if v.info.SendStatus == types.FileSendFailed {
				wg <- types.ErrSendFileFailed
				continue
			}
		} else {
			// 不在缓存，意味着之前没有发送过
			(fs)[i].resultchan = make(chan corkFileResult, 1)
			needSendCorkFiles = append(needSendCorkFiles, (fs)[i])
		}
		blog.Debugf("remote: start to ensure single cork file %s:%s for work(%s)  to server(%s)", results[i].info.FullPath, (fs)[i].file.FilePath, m.work.ID(), server)
		// 启动协程跟踪未发送完成的文件
		c := (fs)[i]
		go func(err chan<- error, c *corkFile, r matchResult, i int) {
			err <- m.ensureSingleCorkFile(c, r)
		}(wg, c, v, i)
	}

	// TODO : 检查是否在server端有缓存了，如果有，则无需发送，调用 checkBatchCache

	blog.Debugf("total %d cork files, need send %d files to server(%s)", totalFileNum, len(needSendCorkFiles), server)
	// append to cork files queue
	_ = m.appendCorkFiles(server, needSendCorkFiles)

	// notify send
	m.sendCorkChan <- true
}

// ensureFiles 确保提供的文件被传输到目标worker的目标目录上
// 同时结合settings.FilterRules来防止相同的文件被重复传输
// 返回一个列表, 表示文件在远程的目标目录
func (m *Mgr) ensureFiles(
	handler dcSDK.RemoteWorkerHandler,
	pid int,
	sandbox *dcSyscall.Sandbox,
	fileDetails []*types.FilesDetails) ([]string, error) {

	//settings := m.work.Basic().Settings()
	blog.Infof("remote: try to ensure multi %d files for work(%s) from pid(%d) dir(%s) to server",
		len(fileDetails), m.work.ID(), pid, sandbox.Dir)
	//blog.Debugf("remote: try to ensure multi %d files for work(%s) from pid(%d) dir(%s) to server: %v",
	//	len(fileDetails), m.work.ID(), pid, sandbox.Dir, fileDetails)
	//rules := settings.FilterRules

	// pump模式下，一次编译依赖的可能有上千个文件，现在的流程会随机的添加到cork发送队列
	// 需要保证一次编译的依赖同时插入到cork发送队列，这样可以尽快的启动远程编译，避免远程编译等待太久
	var err error
	wg := make(chan error, len(fileDetails)+1)
	count := 0
	r := make([]string, 0, 10)
	// cleaner := make([]dcSDK.FileDesc, 0, 10)
	corkFiles := make(map[string]*[]*corkFile, 0)
	// 单文件发送模式，复用corkFile结构体
	singleFiles := make([]*corkFile, 0)
	// allServerCorkFiles := make(map[string]*[]*corkFile, 0)
	filesNum := len(fileDetails)
	for _, fd := range fileDetails {
		blog.Debugf("remote: debug try to ensure file %+v", *fd)

		// 修改远程目录
		f := fd.File
		if f.Targetrelativepath == "" {
			f.Targetrelativepath = m.getRemoteFileBaseDir()
		}
		sender := &dcSDK.BKDistFileSender{Files: []dcSDK.FileDesc{f}}

		//_, t, _ := rules.Satisfy(fd.File.FilePath)
		t := dcSDK.FilterRuleHandleDefault
		//blog.Debugf("remote: ensure file %s and match rule %d", fd.File.FilePath, t)
		if f.AllDistributed {
			t = dcSDK.FilterRuleHandleAllDistribution
		}
		if f.NoDuplicated {
			t = dcSDK.FilterRuleHandleDeduplication
		}
		if f.CompressedSize == -1 || f.FileSize == -1 {
			//t = dcSDK.FilterRuleHandleDefault
			r = append(r, "")
			continue
		}
		blog.Debugf("remote: ensure file %s and match rule %d", fd.File.FilePath, t)
		servers := make([]*dcProtocol.Host, 0, 0)
		switch t {
		//此处会导致文件不被发送，注释掉保证文件都在此处发送，共用内存锁避免OOM
		/*case dcSDK.FilterRuleHandleDefault:
		r = append(r, "")
		continue
		*/
		case dcSDK.FilterRuleHandleAllDistribution:
			// cleaner = append(cleaner, f)
			if err = m.fileMessageBank.ensure(sender, sandbox); err != nil {
				return nil, err
			}

			// 该文件需要被分发到所有的机器上
			servers = m.work.Resource().GetHosts()
			if len(servers) > 0 {
				for _, s := range servers {
					existed := false
					for _, s1 := range fd.Servers {
						if s.Equal(s1) {
							existed = true
							break
						}
					}

					if !existed {
						fd.Servers = append(fd.Servers, s)
					}
				}
			}
		}
		r = append(r, f.Targetrelativepath)

		//blog.Debugf("remote: debug ensure into fd.Servers")
		for _, s := range fd.Servers {
			if s == nil {
				continue
			}
			count++
			if !m.conf.SendCork {
				// 复用corkFile结构体保存待发送文件列表
				singleFiles = append(singleFiles, &corkFile{
					handler:    handler,
					host:       s,
					sandbox:    sandbox,
					file:       &f,
					resultchan: nil,
				})
			} else {
				// for send cork
				cf := &corkFile{
					handler:    handler,
					host:       s,
					sandbox:    sandbox,
					file:       &f,
					resultchan: nil,
				}
				l, ok := corkFiles[s.Server]
				if !ok {
					// 预先分配好队列，避免频繁内存分配
					newl := make([]*corkFile, 0, filesNum)
					newl = append(newl, cf)
					corkFiles[s.Server] = &newl
				} else {
					*l = append(*l, cf)
				}
			}
		}
	}

	if count > 0 {
		receiveResult := make(chan error)

		// 启动接收协程
		go func() {
			for i := 0; i < count; i++ {
				if err = <-wg; err != nil {
					blog.Warnf("remote: failed to ensure multi %d files for work(%s) from pid(%d) to server with err:%v",
						count, m.work.ID(), pid, err)

					// 异常情况下启动一个协程将剩余消息收完，避免发送协程阻塞
					i++
					if i < count {
						go func(i, count int, c <-chan error) {
							for ; i < count; i++ {
								_ = <-c
							}
						}(i, count, wg)
					}

					receiveResult <- err
					return
				}
			}
			receiveResult <- nil
			return
		}()

		// 发送
		if m.conf.SendCork {
			// 批量发送模式
			blog.Debugf("remote: ready to ensure multi %d cork files for work(%s) from pid(%d) to server",
				count, m.work.ID(), pid)

			for server, fs := range corkFiles {
				totalFileNum := len(*fs)
				fsRetry := make([]*corkFile, 0, totalFileNum)
				fsNew := make([]*corkFile, 0, totalFileNum)
				for _, f := range *fs {
					if f.file.Retry {
						fsRetry = append(fsRetry, f)
					} else {
						fsNew = append(fsNew, f)
					}
				}
				//批量发送重试文件
				m.checkAndSendCorkFiles(fsRetry, server, wg, true)
				//批量发送新文件
				m.checkAndSendCorkFiles(fsNew, server, wg, false)
			}
		} else {
			// 单个文件发送模式
			for _, f := range singleFiles {
				sender := &dcSDK.BKDistFileSender{Files: []dcSDK.FileDesc{*f.file}}
				go func(err chan<- error, host *dcProtocol.Host, req *dcSDK.BKDistFileSender) {
					t := time.Now().Local()
					err <- m.ensureSingleFile(handler, host, req, sandbox)
					d := time.Now().Local().Sub(t)
					if d > 200*time.Millisecond {
						blog.Debugf("remote: single file cost time for work(%s) from pid(%d) to server(%s): %s, %s",
							m.work.ID(), pid, host.Server, d.String(), req.Files[0].FilePath)
					}
				}(wg, f.host, sender)
			}
		}

		// 等待接收协程完成或者报错
		err := <-receiveResult
		if err != nil {
			blog.Infof("remote: failed to ensure multi %d files for work(%s) from pid(%d) to server with error:%v",
				count, m.work.ID(), pid, err)
			return nil, err
		}

		blog.Infof("remote: success to ensure multi %d files for work(%s) from pid(%d) to server",
			count, m.work.ID(), pid)
		return r, nil
	}

	blog.Infof("remote: success to ensure multi %d files for work(%s) from pid(%d) to server",
		count, m.work.ID(), pid)

	return r, nil
}

// ensureSingleFile 保证给到的第一个文件被正确分发到目标机器上, 若给到的文件多于一个, 多余的部分会被忽略
func (m *Mgr) ensureSingleFile(
	handler dcSDK.RemoteWorkerHandler,
	host *dcProtocol.Host,
	req *dcSDK.BKDistFileSender,
	sandbox *dcSyscall.Sandbox) (err error) {
	if len(req.Files) == 0 {
		return fmt.Errorf("empty files")
	}
	req.Files = req.Files[:1]
	desc := req.Files[0]
	blog.Debugf("remote: try to ensure single file(%s) for work(%s) to server(%s) with retry %v",
		desc.FilePath, m.work.ID(), host.Server, desc.Retry)
	var status types.FileSendStatus
	var ok bool
	if desc.Retry {
		status, ok, err = m.checkOrLockSendFailFile(host.Server, desc, false)
		if err != nil { // 没找到文件不处理，直接返回不影响其他失败文件发送
			blog.Warnf("remote: checkOrLockSendFailFile(%s) failed: %v", host.Server, err)
			return err
		}
	} else {
		status, ok = m.checkOrLockSendFile(host.Server, desc)
	}
	// 已经有人发送了文件, 等待文件就绪
	if ok {
		blog.Debugf("remote: try to ensure single file(%s) for work(%s) to server(%s), "+
			"some one is sending this file", desc.FilePath, m.work.ID(), host.Server)
		tick := time.NewTicker(m.checkSendFileTick)
		defer tick.Stop()

		for status == types.FileSending {
			select {
			case <-tick.C:
				// 不是发送文件的goroutine，不需要修改状态，仅查询状态
				if desc.Retry {
					status, _, _ = m.checkOrLockSendFailFile(host.Server, desc, true)
				} else {
					status, _ = m.checkOrLockSendFile(host.Server, desc)
				}
			}
		}

		switch status {
		case types.FileSendFailed:
			blog.Errorf("remote: failed to ensure single file(%s) for work(%s) to server(%s), "+
				"file already sent and failed with retry %v", desc.FilePath, m.work.ID(), host.Server, desc.Retry)
			return types.ErrSendFileFailed
		case types.FileSendSucceed:
			blog.Debugf("remote: success to ensure single file(%s) for work(%s) to server(%s)",
				desc.FilePath, m.work.ID(), host.Server)
			return nil
		default:
			return fmt.Errorf("unknown file send status: %s", status.String())
		}
	}

	if m.checkSingleCache(handler, host, desc, sandbox) {
		m.updateSendFile(host.Server, desc, types.FileSendSucceed)
		return nil
	}

	// // send like tcp cork
	// if m.conf.SendCork {
	// 	retcode, err := m.sendFileWithCork(handler, &desc, host, sandbox)
	// 	if err != nil || retcode != 0 {
	// 		blog.Warnf("remote: execute send cork file(%s) for work(%s) to server(%s) failed: %v, retcode:%d",
	// 			desc.FilePath, m.work.ID(), host.Server, err, retcode)
	// 	} else {
	// 		blog.Debugf("remote: execute send cork file(%s) for work(%s) to server(%s) succeed",
	// 			desc.FilePath, m.work.ID(), host.Server)
	// 		return nil
	// 	}
	// }

	blog.Debugf("remote: try to ensure single file(%s) for work(%s) to server(%s), going to send this file with retry %v",
		desc.FilePath, m.work.ID(), host.Server, desc.Retry)
	req.Messages = m.fileMessageBank.get(desc)

	// 同步发送文件
	t := time.Now().Local()
	var result *dcSDK.BKSendFileResult
	if m.conf.LongTCP {
		result, err = handler.ExecuteSendFileLongTCP(host, req, sandbox, m.work.LockMgr())
	} else {
		result, err = handler.ExecuteSendFile(host, req, sandbox, m.work.LockMgr())
	}
	defer func() {
		status := types.FileSendSucceed
		if err != nil {
			status = types.FileSendFailed
		}
		m.updateSendFile(host.Server, desc, status)
	}()
	d := time.Now().Local().Sub(t)
	if d > 200*time.Millisecond {
		blog.Infof("remote: single file real sending file for work(%s) to server(%s): %s, %s",
			m.work.ID(), host.Server, d.String(), req.Files[0].FilePath)
	}

	if err != nil {
		blog.Errorf("remote: execute send file(%s) for work(%s) to server(%s) failed: %v",
			desc.FilePath, m.work.ID(), host.Server, err)
		return err
	}

	if retCode := result.Results[0].RetCode; retCode != 0 {
		return fmt.Errorf("remote: send files(%s) for work(%s) to server(%s) failed, got retCode %d",
			desc.FilePath, m.work.ID(), host.Server, retCode)
	}

	blog.Debugf("remote: success to execute send file(%s) for work(%s) to server(%s) with retry %v",
		desc.FilePath, m.work.ID(), host.Server, desc.Retry)
	return nil
}

// ensureSingleCorkFile 保证给到的第一个文件被正确分发到目标机器上, 若给到的文件多于一个, 多余的部分会被忽略
func (m *Mgr) ensureSingleCorkFile(c *corkFile, r matchResult) (err error) {
	status := r.info.SendStatus
	host := c.host
	desc := c.file

	blog.Debugf("remote: start ensure single cork file(%s) for work(%s) to server(%s)",
		desc.FilePath, m.work.ID(), host.Server)

	// 已经有人发送了文件, 等待文件就绪
	if r.match {
		blog.Debugf("remote: try to ensure single cork file(%s) for work(%s) to server(%s), "+
			"some one is sending this file", desc.FilePath, m.work.ID(), host.Server)
		tick := time.NewTicker(m.checkSendFileTick)
		defer tick.Stop()

		for status == types.FileSending {
			select {
			case <-tick.C:
				// 不是发送文件的goroutine，不能修改状态
				if desc.Retry {
					status, _, _ = m.checkOrLockSendFailFile(host.Server, *desc, true)
				} else {
					status, _ = m.checkOrLockSendFile(host.Server, *desc)
				}
			}
		}

		switch status {
		case types.FileSendFailed:
			blog.Errorf("remote: end ensure single cork file(%s) for work(%s) to server(%s), "+
				"file already sent and failed", desc.FilePath, m.work.ID(), host.Server)
			return types.ErrSendFileFailed
		case types.FileSendSucceed:
			blog.Debugf("remote: end ensure single cork file(%s) for work(%s) to server(%s) succeed",
				desc.FilePath, m.work.ID(), host.Server)
			return nil
		default:
			blog.Errorf("remote: end ensure single cork file(%s) for work(%s) to server(%s), "+
				" with unknown status", desc.FilePath, m.work.ID(), host.Server)
			return fmt.Errorf("unknown cork file send status: %s", status.String())
		}
	}

	// send like tcp cork
	blog.Debugf("remote: start wait result for send single cork file(%s) for work(%s) to server(%s)",
		desc.FilePath, m.work.ID(), host.Server)
	retcode, err := m.waitCorkFileResult(c)
	// blog.Debugf("remote: end wait result for send single cork file(%s) for work(%s) to server(%s)",
	// 	desc.FilePath, m.work.ID(), host.Server)
	if err != nil {
		blog.Warnf("remote: end ensure single cork file(%s) for work(%s) to server(%s) failed: %v, retcode:%d",
			desc.FilePath, m.work.ID(), host.Server, err, retcode)
		return err
	} else if retcode != 0 {
		blog.Warnf("remote: end ensure single cork file(%s) for work(%s) to server(%s) failed: %v, retcode:%d",
			desc.FilePath, m.work.ID(), host.Server, err, retcode)
		return fmt.Errorf("remote: send cork files(%s) for work(%s) to server(%s) failed, got retCode %d",
			desc.FilePath, m.work.ID(), host.Server, retcode)
	} else {
		blog.Debugf("remote: end ensure single cork file(%s) for work(%s) to server(%s) succeed",
			desc.FilePath, m.work.ID(), host.Server)
		return nil
	}
}

func (m *Mgr) checkSingleCache(
	handler dcSDK.RemoteWorkerHandler,
	host *dcProtocol.Host,
	desc dcSDK.FileDesc,
	sandbox *dcSyscall.Sandbox) bool {
	if !workerSideCache(sandbox) {
		return false
	}

	blog.Debugf("remote: try to check cache for single file(%s) for work(%s) to server(%s)",
		desc.FilePath, m.work.ID(), host.Server)
	var r []bool
	var err error
	if m.conf.LongTCP {
		r, err = handler.ExecuteCheckCacheLongTCP(host, &dcSDK.BKDistFileSender{Files: []dcSDK.FileDesc{desc}}, sandbox)
	} else {
		r, err = handler.ExecuteCheckCache(host, &dcSDK.BKDistFileSender{Files: []dcSDK.FileDesc{desc}}, sandbox)
	}
	if err != nil {
		blog.Warnf("remote: try to check cache for single file(%s) for work(%s) to server(%s) failed: %v",
			desc.FilePath, m.work.ID(), host.Server, err)
		return false
	}

	if len(r) == 0 || !r[0] {
		blog.Debugf("remote: try to check cache for single file(%s) for work(%s) to server(%s) not hit cache",
			desc.FilePath, m.work.ID(), host.Server)
		return false
	}

	blog.Debugf("remote: success to check cache for single file(%s) for work(%s) to server(%s) and hit cache",
		desc.FilePath, m.work.ID(), host.Server)
	return true
}

func (m *Mgr) checkBatchCache(
	handler dcSDK.RemoteWorkerHandler,
	host *dcProtocol.Host,
	desc []dcSDK.FileDesc,
	sandbox *dcSyscall.Sandbox) []bool {
	r := make([]bool, 0, len(desc))
	for i := 0; i < len(desc); i++ {
		r = append(r, false)
	}

	if !workerSideCache(sandbox) {
		return r
	}

	blog.Debugf("remote: try to check cache for batch file for work(%s) to server(%s)", m.work.ID(), host.Server)
	var result []bool
	var err error
	if m.conf.LongTCP {
		result, err = handler.ExecuteCheckCacheLongTCP(host, &dcSDK.BKDistFileSender{Files: desc}, sandbox)
	} else {
		result, err = handler.ExecuteCheckCache(host, &dcSDK.BKDistFileSender{Files: desc}, sandbox)
	}
	if err != nil {
		blog.Warnf("remote: try to check cache for batch file for work(%s) to server(%s) failed: %v",
			m.work.ID(), host.Server, err)
		return r
	}

	blog.Debugf("remote: success to check cache for batch file for work(%s) to server(%s) and get result",
		m.work.ID(), host.Server)
	return result
}

// checkOrLockFile 检查目标file的sendStatus, 如果已经被发送, 则返回当前状态和true; 如果没有被发送过, 则将其置于sending, 并返回false
func (m *Mgr) checkOrLockSendFile(server string, desc dcSDK.FileDesc) (types.FileSendStatus, bool) {
	t1 := time.Now().Local()
	m.fileSendMutex.Lock()
	t2 := time.Now().Local()
	if d1 := t2.Sub(t1); d1 > 50*time.Millisecond {
		// blog.Debugf("check cache lock wait too long server(%s): %s", server, d1.String())
	}

	defer func() {
		if d2 := time.Now().Local().Sub(t2); d2 > 50*time.Millisecond {
			// blog.Debugf("check cache process wait too long server(%s): %s", server, d2.String())
		}
	}()

	target, ok := m.fileSendMap[server]
	if !ok {
		target = &fileSendMap{}
		m.fileSendMap[server] = target
	}
	m.fileSendMutex.Unlock()

	info, match := target.matchOrInsert(desc)
	return info.SendStatus, match
}

func (m *Mgr) checkOrLockSendFailFile(server string, desc dcSDK.FileDesc, query bool) (types.FileSendStatus, bool, error) {
	m.failFileSendMutex.Lock()
	target, ok := m.failFileSendMap[server]
	if !ok {
		target = &fileSendMap{}
		m.failFileSendMap[server] = target
	}
	m.failFileSendMutex.Unlock()

	info, match, err := target.matchFail(desc, query)
	if err != nil {
		blog.Errorf("remote: check or lock send fail file failed: %v", err)
		return types.FileSendUnknown, false, err
	}
	if info == nil {
		blog.Errorf("remote: check or lock send fail file failed: file is nil")
		return types.FileSendUnknown, false, errors.New("file is nil")
	}
	return info.SendStatus, match, nil
}

type matchResult struct {
	info  *types.FileInfo
	match bool
}

// checkOrLockCorkFiles 批量检查目标file的sendStatus, 如果已经被发送, 则返回当前状态和true; 如果没有被发送过, 则将其置于sending, 并返回false
func (m *Mgr) checkOrLockCorkFiles(server string, descs []*dcSDK.FileDesc, retry bool) []matchResult {
	//批量检查首次发送文件
	if !retry {
		//blog.Debugf("remote: execute remote task to server(%s) for descs(%d)", server, len(newDescs))
		m.fileSendMutex.Lock()
		target, ok := m.fileSendMap[server]
		if !ok {
			target = &fileSendMap{}
			m.fileSendMap[server] = target
		}
		m.fileSendMutex.Unlock()
		return target.matchOrInserts(descs)
	} else { //批量检查重试文件
		//blog.Debugf("remote: execute remote task to server(%s) for retry descs(%d)", server, len(retryDescs))
		m.failFileSendMutex.Lock()
		target, ok := m.failFileSendMap[server]
		if !ok {
			target = &fileSendMap{}
			m.failFileSendMap[server] = target
		}
		m.failFileSendMutex.Unlock()
		return target.matchFails(descs)
	}
}

func (m *Mgr) needToUpdateFail(desc dcSDK.FileDesc, status types.FileSendStatus) bool {
	if status == types.FileSendSucceed && !desc.Retry {
		return false
	}
	if strings.HasSuffix(desc.FilePath, ".ii") {
		return false
	}
	if strings.HasSuffix(desc.FilePath, ".i") {
		return false
	}
	return true
}

func (m *Mgr) updateSendFile(server string, desc dcSDK.FileDesc, status types.FileSendStatus) {
	if status == types.FileSendSucceed || !desc.Retry {
		m.fileSendMutex.Lock()
		target, ok := m.fileSendMap[server]
		if !ok {
			target = &fileSendMap{}
			m.fileSendMap[server] = target
		}
		m.fileSendMutex.Unlock()
		target.updateStatus(desc, status)
	}

	if m.needToUpdateFail(desc, status) {
		m.failFileSendMutex.Lock()
		failTarget, ok := m.failFileSendMap[server]
		if !ok {
			failTarget = &fileSendMap{}
			m.failFileSendMap[server] = failTarget
		}
		m.failFileSendMutex.Unlock()

		failTarget.updateFailStatus(desc, status)
	}
}

func (m *Mgr) sendToolchain(handler dcSDK.RemoteWorkerHandler, req *types.RemoteTaskExecuteRequest) error {
	// TODO : update all file path for p2p
	fileCollections := m.getToolChainFromExecuteRequest(req)
	if fileCollections != nil && len(fileCollections) > 0 {
		err := m.sendFileCollectionOnce(handler, req.Pid, req.Sandbox, req.Server, fileCollections)
		if err != nil {
			blog.Errorf("remote: execute remote task for work(%s) from pid(%d) to server(%s), "+
				"ensure tool chain files failed: %v", m.work.ID(), req.Pid, req.Server.Server, err)
			return err
		}

		// reset tool chain info if changed, then send again until old tool chain finished sending,
		// to avoid write same file on remote worker
		toolChainChanged, _ := m.isToolChainChanged(req, req.Server.Server)
		finished, _ := m.isToolChainFinished(req, req.Server.Server)
		for toolChainChanged || !finished {
			blog.Infof("remote: found tool chain changed, ready clear tool chain status")
			m.clearOldFileCollectionFromCache(req.Server.Server, fileCollections)
			fileCollections = m.getToolChainFromExecuteRequest(req)
			if fileCollections != nil && len(fileCollections) > 0 {
				blog.Infof("remote: found tool chain changed, send toolchain to server[%s] again",
					req.Server.Server)
				err = m.sendFileCollectionOnce(handler, req.Pid, req.Sandbox, req.Server, fileCollections)
				if err != nil {
					blog.Errorf("remote: execute remote task for work(%s) from pid(%d) to server(%s), "+
						"ensure tool chain files failed: %v", m.work.ID(), req.Pid, req.Server.Server, err)
					return err
				}
			}
			toolChainChanged, _ = m.isToolChainChanged(req, req.Server.Server)
			finished, _ = m.isToolChainFinished(req, req.Server.Server)
		}

		// TODO : insert exe path as input with p2p path, not tool chain path
		_ = m.updateToolChainPath(req)

		// TODO : update result abs path to relative if need
		_ = m.updateResultPath(req)
	}

	return nil
}

// getFailedFileCollectionByHost 返回失败文件集合
func (m *Mgr) getFailedFileCollectionByHost(server string) []*types.FileCollectionInfo {
	m.fileCollectionSendMutex.RLock()
	defer m.fileCollectionSendMutex.RUnlock()

	target, ok := m.fileCollectionSendMap[server]
	if !ok {
		blog.Debugf("remote: no found host(%s) in file send cache", server)
		return nil
	}
	fcs := make([]*types.FileCollectionInfo, 0)

	for _, re := range *target {
		re.Retry = true
		if re.SendStatus == types.FileSendFailed {
			fcs = append(fcs, re)
		}
	}
	return fcs
}

// retry send failed tool chain
func (m *Mgr) retrySendToolChain(handler dcSDK.RemoteWorkerHandler, req *types.RemoteTaskExecuteRequest, fileCollections []*types.FileCollectionInfo, host chan string) {
	blog.Infof("remote: retry send tool chain for work(%s) from pid(%d) to server(%s)",
		m.work.ID(), req.Pid, req.Server.Server)
	err := m.sendFileCollectionOnce(handler, req.Pid, req.Sandbox, req.Server, fileCollections)
	if err != nil {
		blog.Errorf("remote: retry send tool chain for work(%s) from pid(%d) to server(%s), "+
			"send tool chain files failed: %v", m.work.ID(), req.Pid, req.Server.Server, err)

	} else {
		// enable worker
		m.resource.EnableWorker(req.Server)
		blog.Infof("remote: success to retry send tool chain for work(%s) from pid(%d) to server(%s)", m.work.ID(), req.Pid, req.Server.Server)
	}
	host <- req.Server.Server
}

func (m *Mgr) sendFileCollectionOnce(
	handler dcSDK.RemoteWorkerHandler,
	pid int,
	sandbox *dcSyscall.Sandbox,
	server *dcProtocol.Host,
	filecollections []*types.FileCollectionInfo) error {
	blog.Infof("remote: try to send %d file collection for work(%s) from pid(%d) dir(%s) to server",
		len(filecollections), m.work.ID(), pid, sandbox.Dir)

	var err error
	wg := make(chan error, len(filecollections)+1)
	count := 0
	for _, fc := range filecollections {
		count++
		go func(err chan<- error, host *dcProtocol.Host, filecollection *types.FileCollectionInfo) {
			err <- m.ensureOneFileCollection(handler, pid, host, filecollection, sandbox)
			// err <- m.ensureOneFileCollectionByFiles(handler, pid, host, filecollection, sandbox)
		}(wg, server, fc)
	}

	for i := 0; i < count; i++ {
		if err = <-wg; err != nil {
			// 异常情况下启动一个协程将消息收完，避免发送协程阻塞
			i++
			if i < count {
				go func(i, count int, c <-chan error) {
					for ; i < count; i++ {
						<-c
					}
				}(i, count, wg)
			}

			return err
		}
	}
	blog.Infof("remote: success to send %d file collection for work(%s) from pid(%d) to server",
		count, m.work.ID(), pid)

	return nil
}

func (m *Mgr) getRetryFileDetails(server string, fc *types.FileCollectionInfo, Servers []*dcProtocol.Host) ([]*types.FilesDetails, error) {
	fileDetails := make([]*types.FilesDetails, 0, len(fc.Files))
	m.failFileSendMutex.Lock()
	failsendMap := m.failFileSendMap[server]
	if failsendMap == nil {
		m.failFileSendMutex.Unlock()
		err := fmt.Errorf("remote: send file for work(%s) with no send map", m.work.ID())
		blog.Errorf(err.Error())
		return fileDetails, err
	}
	m.failFileSendMutex.Unlock()

	failsendMap.Lock()
	defer failsendMap.Unlock()

	for _, f := range fc.Files {
		f.NoDuplicated = true
		for _, c := range failsendMap.cache {
			for _, d := range *c {
				if d.Match(f) {
					f.Retry = fc.Retry
					break
				}
			}
			if f.Retry {
				break
			}
		}
		fileDetails = append(fileDetails, &types.FilesDetails{
			Servers: Servers,
			File:    f,
		})
	}
	return fileDetails, nil
}

// ensureOneFileCollection 保证给到的第一个文件集合被正确分发到目标机器上
func (m *Mgr) ensureOneFileCollection(
	handler dcSDK.RemoteWorkerHandler,
	pid int,
	host *dcProtocol.Host,
	fc *types.FileCollectionInfo,
	sandbox *dcSyscall.Sandbox) (err error) {
	blog.Infof("remote: try to ensure one file collection(%s) for work(%s) to server(%s)",
		fc.UniqID, m.work.ID(), host.Server)

	status, ok := m.checkOrLockFileCollection(host.Server, fc)

	// 已经有人发送了文件, 等待文件就绪
	if ok {
		blog.Infof("remote: try to ensure one file collection(%s) timestamp(%d) for work(%s) to server(%s), "+
			"dealing(dealed) by other", fc.UniqID, fc.Timestamp, m.work.ID(), host.Server)
		tick := time.NewTicker(m.checkSendFileTick)
		defer tick.Stop()

		for status == types.FileSending {
			select {
			case <-tick.C:
				status, _ = m.getCachedToolChainStatus(host.Server, fc.UniqID)
				// FileSendUnknown means this collection cleard from cache, just wait for sending again
				if status == types.FileSendUnknown {
					status = types.FileSending
				}
			}
		}

		switch status {
		case types.FileSendFailed:
			return types.ErrSendFileFailed
		case types.FileSendSucceed:
			blog.Infof("remote: success to ensure one file collection(%s) timestamp(%d) "+
				"for work(%s) to server(%s)", fc.UniqID, fc.Timestamp, m.work.ID(), host.Server)
			return nil
		default:
			return fmt.Errorf("unknown file send status: %s", status.String())
		}
	}

	needSentFiles := make([]dcSDK.FileDesc, 0, len(fc.Files))
	hit := 0
	for i, b := range m.checkBatchCache(handler, host, fc.Files, sandbox) {
		if b {
			hit++
			continue
		}

		needSentFiles = append(needSentFiles, fc.Files[i])
	}

	blog.Infof("remote: try to ensure one file collection(%s) timestamp(%d) filenum(%d) cache-hit(%d) "+
		"for work(%s) to server(%s), going to send this collection",
		fc.UniqID, fc.Timestamp, len(needSentFiles), hit, m.work.ID(), host.Server)

	// ！！ 这个地方不需要了，需要注释掉，影响性能
	// req := &dcSDK.BKDistFileSender{Files: needSentFiles}
	// if req.Messages, err = client.EncodeSendFileReq(req, sandbox); err != nil {
	// 	return err
	// }

	// // 同步发送文件
	// result, err := handler.ExecuteSendFile(host, req, sandbox, m.work.LockMgr())
	// defer func() {
	// 	status := types.FileSendSucceed
	// 	if err != nil {
	// 		status = types.FileSendFailed
	// 	}
	// 	m.updateFileCollectionStatus(host.Server, fc, status)
	// }()

	// if err != nil {
	// 	blog.Errorf("remote: execute send file collection(%s) for work(%s) to server(%s) failed: %v",
	// 		fc.UniqID, m.work.ID(), host.Server, err)
	// 	return err
	// }

	// if retCode := result.Results[0].RetCode; retCode != 0 {
	// 	return fmt.Errorf("remote: send files collection(%s) for work(%s) to server(%s) failed, got retCode %d",
	// 		fc.UniqID, m.work.ID(), host.Server, retCode)
	// }

	Servers := make([]*dcProtocol.Host, 0, 1)
	Servers = append(Servers, host)

	fileDetails := make([]*types.FilesDetails, 0, len(fc.Files))

	if fc.Retry {
		fileDetails, err = m.getRetryFileDetails(host.Server, fc, Servers)
		if err != nil {
			return err
		}
	} else {
		for _, f := range fc.Files {
			f.NoDuplicated = true
			fileDetails = append(fileDetails, &types.FilesDetails{
				Servers: Servers,
				File:    f,
			})
		}
	}

	_, err = m.ensureFilesWithPriority(handler, pid, sandbox, fileDetails)
	defer func() {
		status := types.FileSendSucceed
		if err != nil {
			status = types.FileSendFailed
		}
		m.updateFileCollectionStatus(host.Server, fc, status)
	}()

	if err != nil {
		blog.Errorf("remote: execute send file collection(%s) for work(%s) to server(%s) failed : %v ",
			fc.UniqID, m.work.ID(), host.Server, err)
		return err
	}

	blog.Debugf("remote: success to execute send file collection(%s) files(%+v) timestamp(%d) filenum(%d) "+
		"for work(%s) to server(%s)", fc.UniqID, fc.Files, fc.Timestamp, len(fc.Files), m.work.ID(), host.Server)
	return nil
}

// checkOrLockFileCollection 检查目标file collection的sendStatus, 如果已经被发送, 则返回当前状态和true; 如果没有被发送过,
// 则将其置于sending, 并返回false
func (m *Mgr) checkOrLockFileCollection(server string, fc *types.FileCollectionInfo) (types.FileSendStatus, bool) {
	m.fileCollectionSendMutex.Lock()
	defer m.fileCollectionSendMutex.Unlock()

	target, ok := m.fileCollectionSendMap[server]
	if !ok {
		filecollections := make([]*types.FileCollectionInfo, 0, 10)
		m.fileCollectionSendMap[server] = &filecollections
		target = m.fileCollectionSendMap[server]
	}

	for _, f := range *target {
		if f.UniqID == fc.UniqID {
			// set status to sending if fc send failed
			if f.SendStatus == types.FileSendFailed && fc.Retry {
				f.SendStatus = types.FileSending
				return f.SendStatus, false
			}
			return f.SendStatus, true
		}
	}

	fc.SendStatus = types.FileSending
	*target = append(*target, fc)

	return types.FileSending, false
}

func (m *Mgr) updateFileCollectionStatus(server string, fc *types.FileCollectionInfo, status types.FileSendStatus) {
	m.fileCollectionSendMutex.Lock()
	defer m.fileCollectionSendMutex.Unlock()

	blog.Infof("remote: ready add collection(%s) server(%s) timestamp(%d) status(%s) to cache",
		fc.UniqID, server, fc.Timestamp, status.String())

	target, ok := m.fileCollectionSendMap[server]
	if !ok {
		filecollections := make([]*types.FileCollectionInfo, 0, 10)
		m.fileCollectionSendMap[server] = &filecollections
		target = m.fileCollectionSendMap[server]
	}

	for _, f := range *target {
		if f.UniqID == fc.UniqID {
			f.SendStatus = status
			return
		}
	}

	fc.SendStatus = status
	*target = append(*target, fc)
	blog.Infof("remote: finishend add collection(%s) server(%s) timestamp(%d) status(%d) to cache",
		fc.UniqID, server, fc.Timestamp, status)

	return
}

// to ensure clear only once
func (m *Mgr) clearOldFileCollectionFromCache(server string, fcs []*types.FileCollectionInfo) {
	m.fileCollectionSendMutex.Lock()
	defer m.fileCollectionSendMutex.Unlock()

	target, ok := m.fileCollectionSendMap[server]
	if !ok {
		return
	}

	i := 0
	needdelete := false
	for _, f := range *target {
		needdelete = false
		for _, fc := range fcs {
			if f.UniqID == fc.UniqID {
				timestamp, _ := m.work.Basic().GetToolChainTimestamp(f.UniqID)
				if f.Timestamp != timestamp {
					blog.Infof("remote: clear collection(%s) server(%s) timestamp(%d) "+
						"new timestamp(%d) from cache", f.UniqID, server, f.Timestamp, timestamp)
					needdelete = true
					break
				}
			}
		}
		// save values not delete
		if !needdelete {
			(*target)[i] = f
			i++
		}
	}

	// Prevent memory leak by erasing truncated values
	for j := i; j < len(*target); j++ {
		(*target)[j] = nil
	}
	*target = (*target)[:i]

	return
}

func (m *Mgr) getCachedToolChainTimestamp(server string, toolChainKey string) (int64, error) {
	m.fileCollectionSendMutex.RLock()
	defer m.fileCollectionSendMutex.RUnlock()

	target, ok := m.fileCollectionSendMap[server]
	if !ok {
		return 0, fmt.Errorf("toolchain [%s] not existed in cache", toolChainKey)
	}

	for _, f := range *target {
		if f.UniqID == toolChainKey {
			return f.Timestamp, nil
		}
	}

	return 0, fmt.Errorf("toolchain [%s] not existed in cache", toolChainKey)
}

func (m *Mgr) getCachedToolChainStatus(server string, toolChainKey string) (types.FileSendStatus, error) {
	m.fileCollectionSendMutex.RLock()
	defer m.fileCollectionSendMutex.RUnlock()

	target, ok := m.fileCollectionSendMap[server]
	if !ok {
		return types.FileSendUnknown, nil
	}

	for _, f := range *target {
		if f.UniqID == toolChainKey {
			return f.SendStatus, nil
		}
	}

	return types.FileSendUnknown, nil
}

func (m *Mgr) lockSlots(usage dcSDK.JobUsage, f string, banWorkerList []*dcProtocol.Host) *dcProtocol.Host {
	return m.resource.Lock(usage, f, banWorkerList)
}

func (m *Mgr) unlockSlots(usage dcSDK.JobUsage, host *dcProtocol.Host) {
	m.resource.Unlock(usage, host)
}

// TotalSlots return available total slots
func (m *Mgr) TotalSlots() int {
	return m.resource.TotalSlots()
}

func (m *Mgr) getRemoteFileBaseDir() string {
	return fmt.Sprintf("common_%s", m.work.ID())
}

func (m *Mgr) syncHostTimeNoWait(hostList []*dcProtocol.Host) []*dcProtocol.Host {
	go m.syncHostTime(hostList)
	return hostList
}

func (m *Mgr) syncHostTime(hostList []*dcProtocol.Host) []*dcProtocol.Host {
	blog.Infof("remote: try to sync time for hosts list")

	handler := m.remoteWorker.Handler(0, nil, nil, nil)
	counter := make(map[string]int64, 50)
	div := make(map[string]int64, 50)
	var lock sync.Mutex

	var wg sync.WaitGroup
	for _, host := range hostList {
		ip := getIPFromServer(host.Server)
		if _, ok := counter[ip]; ok {
			continue
		}

		counter[ip] = 0
		div[ip] = 0

		for i := 0; i < syncHostTimeTimes; i++ {
			wg.Add(1)
			go func(h *dcProtocol.Host) {
				defer wg.Done()

				t1 := time.Now().Local().UnixNano()
				var remoteTime int64
				var err error
				if m.conf.LongTCP {
					remoteTime, err = handler.ExecuteSyncTimeLongTCP(h.Server)
				} else {
					remoteTime, err = handler.ExecuteSyncTime(h.Server)
				}
				if err != nil {
					blog.Warnf("remote: try to sync time for host(%s) failed: %v", h.Server, err)
					return
				}

				t2 := time.Now().Local().UnixNano()

				deltaTime := remoteTime - (t1+t2)/2
				lock.Lock()
				counter[getIPFromServer(h.Server)] += deltaTime
				div[ip]++
				lock.Unlock()
				blog.Debugf("remote: success to sync time from host(%s), get delta time: %d",
					h.Server, deltaTime)
			}(host)
		}
	}
	wg.Wait()

	for _, host := range hostList {
		ip := getIPFromServer(host.Server)
		if _, ok := counter[ip]; !ok {
			continue
		}

		if div[ip] <= 0 {
			continue
		}
		host.TimeDelta = counter[ip] / div[ip]
		blog.Infof("remote: success to sync time for host(%s), set delta time: %d", host.Server, host.TimeDelta)
	}

	blog.Infof("remote: success to sync time for hosts list")
	return hostList
}

func (m *Mgr) getToolFileInfoByKey(key string) *types.FileCollectionInfo {
	if !m.work.Resource().SupportAbsPath() {
		blog.Infof("remote: ready get relative toolchain files for %s", key)
		toolchainfiles, timestamp, err := m.work.Basic().GetToolChainRelativeFiles(key)
		if err == nil && len(toolchainfiles) > 0 {
			blog.Debugf("remote: got toolchain files for %s:%v", key, toolchainfiles)
			return &types.FileCollectionInfo{
				UniqID:     key,
				Files:      toolchainfiles,
				SendStatus: types.FileSending,
				Timestamp:  timestamp,
			}
		}
	} else {
		blog.Infof("remote: ready get normal toolchain files for %s", key)
		toolchainfiles, timestamp, err := m.work.Basic().GetToolChainFiles(key)
		if err == nil && len(toolchainfiles) > 0 {
			blog.Debugf("remote: got toolchain files for %s:%v", key, toolchainfiles)
			return &types.FileCollectionInfo{
				UniqID:     key,
				Files:      toolchainfiles,
				SendStatus: types.FileSending,
				Timestamp:  timestamp,
			}
		}
	}
	return nil
}

func (m *Mgr) getToolChainFromExecuteRequest(req *types.RemoteTaskExecuteRequest) []*types.FileCollectionInfo {
	blog.Debugf("remote: get toolchain with req:[%+v]", *req)
	fd := make([]*types.FileCollectionInfo, 0, 2)

	for _, c := range req.Req.Commands {
		blog.Debugf("remote: ready get toolchain with key:[%s]", c.ExeToolChainKey)
		//add additional files to all workers
		if additionfd := m.getToolFileInfoByKey(dcSDK.GetAdditionFileKey()); additionfd != nil {
			fd = append(fd, additionfd)
		}
		if c.ExeToolChainKey != "" {
			toolfd := m.getToolFileInfoByKey(c.ExeToolChainKey)
			if toolfd != nil {
				fd = append(fd, toolfd)
			} else {
				// TODO : 如果环境变量中指定了需要自动探测工具链，则需要自动探测
				if dcSyscall.NeedSearchToolchain(req.Sandbox.Env) {
					path := req.Sandbox.Env.GetOriginEnv("PATH")
					blog.Infof("remote: start search toolchain with key:%s path:%s", c.ExeToolChainKey, path)
					err := m.work.Basic().SearchToolChain(c.ExeToolChainKey, path)
					blog.Infof("remote: end search toolchain with key:%s error:%v", c.ExeToolChainKey, err)

					if err == nil {
						toolfd = m.getToolFileInfoByKey(c.ExeToolChainKey)
						if toolfd != nil {
							fd = append(fd, toolfd)
						}
					}
				}
			}
		}
	}

	return fd
}

func (m *Mgr) isToolChainChanged(req *types.RemoteTaskExecuteRequest, server string) (bool, error) {
	blog.Debugf("remote: check tool chain changed with req:[%+v]", *req)

	for _, c := range req.Req.Commands {
		blog.Debugf("remote: ready check toolchain changed with key:[%s]", c.ExeToolChainKey)
		//check additional files toolchain
		timestamp, err := m.work.Basic().GetToolChainTimestamp(dcSDK.GetAdditionFileKey())
		if err == nil {
			timestampcached, _ := m.getCachedToolChainTimestamp(server, dcSDK.GetAdditionFileKey())
			if timestamp != timestampcached {
				blog.Infof("remote: found collection(%s) server(%s) cached timestamp(%d) "+
					"newly timestamp(%d) changed", dcSDK.GetAdditionFileKey(), server, timestampcached, timestamp)
				return true, nil
			}
		}

		if c.ExeToolChainKey != "" {
			timestamp, err := m.work.Basic().GetToolChainTimestamp(c.ExeToolChainKey)
			if err == nil {
				timestampcached, _ := m.getCachedToolChainTimestamp(server, c.ExeToolChainKey)
				if timestamp != timestampcached {
					blog.Infof("remote: found collection(%s) server(%s) cached timestamp(%d) "+
						"newly timestamp(%d) changed", c.ExeToolChainKey, server, timestampcached, timestamp)
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func (m *Mgr) isToolChainFinished(req *types.RemoteTaskExecuteRequest, server string) (bool, error) {
	blog.Debugf("remote: check tool chain finished with req:[%+v]", *req)

	allfinished := true
	for _, c := range req.Req.Commands {
		blog.Debugf("remote: ready check toolchain finished with key:[%s]", c.ExeToolChainKey)
		//check additional files toolchain
		if m.work.Basic().IsToolChainExsited(dcSDK.GetAdditionFileKey()) {
			status, err := m.getCachedToolChainStatus(server, dcSDK.GetAdditionFileKey())
			if err == nil {
				if status != types.FileSendSucceed && status != types.FileSendFailed {
					blog.Infof("remote: found collection(%s) server(%s) status(%d) not finished",
						dcSDK.GetAdditionFileKey(), server, status)
					allfinished = false
					return allfinished, nil
				}
			}
		}
		if c.ExeToolChainKey != "" {
			if m.work.Basic().IsToolChainExsited(c.ExeToolChainKey) {
				status, err := m.getCachedToolChainStatus(server, c.ExeToolChainKey)
				if err == nil {
					if status != types.FileSendSucceed && status != types.FileSendFailed {
						blog.Infof("remote: found collection(%s) server(%s) status(%d) not finished",
							c.ExeToolChainKey, server, status)
						allfinished = false
						return allfinished, nil
					}
				}
			}
		}
	}

	return allfinished, nil
}

func (m *Mgr) updateToolChainPath(req *types.RemoteTaskExecuteRequest) error {
	for i, c := range req.Req.Commands {
		if c.ExeToolChainKey != "" {
			remotepath := ""
			var err error
			if !m.work.Resource().SupportAbsPath() {
				remotepath, err = m.work.Basic().GetToolChainRelativeRemotePath(c.ExeToolChainKey)
			} else {
				remotepath, err = m.work.Basic().GetToolChainRemotePath(c.ExeToolChainKey)
			}
			if err != nil {
				return fmt.Errorf("not found remote path for toolchain %s", c.ExeToolChainKey)
			}
			blog.Debugf("remote: before update toolchain with key:[%s],remotepath:[%s],inputfiles:%+v",
				c.ExeToolChainKey, remotepath, req.Req.Commands[i].Inputfiles)
			req.Req.Commands[i].Inputfiles = append(req.Req.Commands[i].Inputfiles, dcSDK.FileDesc{
				FilePath:           c.ExeName,
				Compresstype:       dcProtocol.CompressLZ4,
				FileSize:           -1,
				Lastmodifytime:     0,
				Md5:                "",
				Targetrelativepath: remotepath,
			})
			blog.Debugf("remote: after update toolchain with key:[%s],remotepath:[%s],inputfiles:%+v",
				c.ExeToolChainKey, remotepath, req.Req.Commands[i].Inputfiles)
		}
	}
	return nil
}

// 如果worker不支持绝对路径，则需要将结果文件改成相对路径
// 即通过当前的绝对路径，得到一个相对路径，将绝对路径和相对同时传给远端
// 远端用相对路径执行命令，返回时指定为绝对路径
// 比如 绝对路径为： c:\path\1.result 则改为:  c:\path\1.result<!|!>.\1.result
// 其中 <!|!> 为连接符号， 同时将命令中的结果文件对应的参数替换为相对路径
func (m *Mgr) updateResultPath(req *types.RemoteTaskExecuteRequest) error {
	if !m.work.Resource().SupportAbsPath() {
		for i := range req.Req.Commands {
			for j, v := range req.Req.Commands[i].ResultFiles {
				if filepath.IsAbs(v) {
					relative := filepath.Join(".", filepath.Base(v))
					req.Req.Commands[i].ResultFiles[j] = v + workerType.FileConnectFlag + relative

					for k, p := range req.Req.Commands[i].Params {
						// 将参数中涉及到结果文件的本地绝对路径替换为远程的相对路径
						req.Req.Commands[i].Params[k] = strings.Replace(p, v, relative, 1)
					}
				}
			}
		}
	}
	return nil
}

type corkFileResult struct {
	retcode int32
	err     error
}

type corkFile struct {
	handler    dcSDK.RemoteWorkerHandler
	host       *dcProtocol.Host
	sandbox    *dcSyscall.Sandbox
	file       *dcSDK.FileDesc
	resultchan chan corkFileResult
}

func (m *Mgr) sendFileWithCork(handler dcSDK.RemoteWorkerHandler,
	f *dcSDK.FileDesc,
	host *dcProtocol.Host,
	sandbox *dcSyscall.Sandbox) (int32, error) {
	cf := corkFile{
		handler:    handler,
		host:       host,
		sandbox:    sandbox,
		file:       f,
		resultchan: make(chan corkFileResult, 1),
	}

	// append to file queue
	m.corkMutex.Lock()
	if l, ok := m.corkFiles[host.Server]; ok {
		*l = append(*l, &cf)
	} else {
		newl := []*corkFile{&cf}
		m.corkFiles[host.Server] = &newl
	}
	m.corkMutex.Unlock()

	// notify send
	m.sendCorkChan <- true

	// wait for result
	msg := <-cf.resultchan
	return msg.retcode, msg.err
}

func (m *Mgr) appendCorkFiles(server string, cfs []*corkFile) error {
	// append to file queue
	m.corkMutex.Lock()
	if l, ok := m.corkFiles[server]; ok {
		*l = append(*l, cfs...)
	} else {
		// 队列分配大点，避免频繁分配
		newlen := len(cfs) * 10
		newl := make([]*corkFile, 0, newlen)
		newl = append(newl, cfs...)
		m.corkFiles[server] = &newl
	}
	m.corkMutex.Unlock()

	// for _, v := range cfs {
	// 	blog.Infof("remote: appended cork file[%s] ", v.file.FilePath)
	// }

	// notify send
	m.sendCorkChan <- true

	return nil
}

// wait for result
func (m *Mgr) waitCorkFileResult(cf *corkFile) (int32, error) {
	msg := <-cf.resultchan
	return msg.retcode, msg.err
}

func (m *Mgr) getCorkFiles(sendanyway bool) []*[]*corkFile {
	m.corkMutex.Lock()
	defer m.corkMutex.Unlock()

	if len(m.corkFiles) == 0 {
		return nil
	}

	result := make([]*[]*corkFile, 0)
	for _, v := range m.corkFiles {
		var totalsize int64
		index := -1
		for index = range *v {
			// debug by tming, 如果有大文件，则考虑将大文件放到下一批再发送
			// 避免大文件影响当批其它文件的发送时间
			if (*v)[index].file.FileSize > m.corkMaxSize && index > 0 {
				blog.Debugf("remote: send cork files do not send now for file:%s size:%d for work: %s",
					(*v)[index].file.FilePath, (*v)[index].file.FileSize, m.work.ID())
				index -= 1
				sendanyway = true
				break
			}

			totalsize += (*v)[index].file.FileSize
			// 如果数据包超过 m.corkMaxSize 了，则停止获取，下次再发
			if totalsize > m.corkMaxSize {
				break
			}
		}

		if index >= 0 {
			if sendanyway || totalsize > m.corkSize {
				// get files
				num := index + 1
				start := 0
				dstfiles := make([]*corkFile, num)
				copy(dstfiles, (*v)[start:start+num])

				// remove files
				if num < len(*v) {
					*v = (*v)[start+num:]
				} else {
					*v = make([]*corkFile, 0)
				}

				result = append(result, &dstfiles)
			}
		}
	}

	return result
}

func (m *Mgr) sendFilesWithCorkTick(ctx context.Context) {
	blog.Infof("remote: start send files with cork tick for work: %s", m.work.ID())
	ticker := time.NewTicker(m.sendCorkTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			blog.Infof("remote: run send files with cork for work(%s) canceled by context", m.work.ID())
			return

		case <-ticker.C:
			// check whether have files to send
			files := m.getCorkFiles(true)
			if len(files) > 0 {
				m.sendFilesWithCork(files)
			}
		case <-m.sendCorkChan:
			// check whether enought files to send
			files := m.getCorkFiles(false)
			if len(files) > 0 {
				m.sendFilesWithCork(files)
			}
		}
	}
}

func (m *Mgr) sendFilesWithCork(files []*[]*corkFile) {
	for _, v := range files {
		if v != nil && len(*v) > 0 {
			go m.sendFilesWithCorkSameHost(*v)
		}
	}
}

func (m *Mgr) sendFilesWithCorkSameHost(files []*corkFile) {

	var totalsize int64
	req := &dcSDK.BKDistFileSender{}
	req.Files = make([]dcSDK.FileDesc, 0, len(files))
	for _, v := range files {
		req.Files = append(req.Files, *v.file)
		totalsize += v.file.FileSize
	}
	host := files[0].host
	sandbox := files[0].sandbox
	handler := files[0].handler

	blog.Debugf("remote: start send cork %d files with size:%d to server %s for work: %s",
		len(files), totalsize, host.Server, m.work.ID())

	// // in queue to limit total size of sending files, maybe we should deal if failed
	// // blog.Infof("remote: try to get memory lock with file size:%d", totalsize)
	// if m.memSlot.Lock(totalsize) {
	// 	// blog.Infof("remote: succeed to get memory lock with file size:%d", totalsize)
	// 	defer func() {
	// 		m.memSlot.Unlock(totalsize)
	// 		// total, occu := m.memSlot.GetStatus()
	// 		// blog.Infof("remote: succeed to free memory lock with file size:%d,occupy:%d, total:%d", totalsize, occu, total)
	// 	}()
	// 	blog.Infof("remote: got memroy lock for send cork %d files with size:%d to server %s for work: %s",
	// 		len(files), totalsize, host.Server, m.work.ID())

	// } else {
	// 	blog.Infof("remote: failed to get memory lock with file size:%d", totalsize)
	// }

	// add retry here
	var result *dcSDK.BKSendFileResult
	waitsecs := 5
	var err error
	for i := 0; i < 4; i++ {
		if m.conf.LongTCP {
			result, err = handler.ExecuteSendFileLongTCP(host, req, sandbox, m.work.LockMgr())
		} else {
			result, err = handler.ExecuteSendFile(host, req, sandbox, m.work.LockMgr())
		}
		if err == nil {
			break
		} else {
			blog.Warnf("remote: failed to send cork %d files with size:%d to server:%s for the %dth times with error:%v",
				len(files), totalsize, host.Server, i, err)
			time.Sleep(time.Duration(waitsecs) * time.Second)
			waitsecs = waitsecs * 2
		}
	}

	// free memory anyway after sent file
	debug.FreeOSMemory()

	status := types.FileSendSucceed
	if err != nil {
		status = types.FileSendFailed
	}

	var retcode int32
	if err != nil {
		retcode = -1
	} else {
		retcode = result.Results[0].RetCode
	}
	resultchan := corkFileResult{
		retcode: retcode,
		err:     err,
	}

	for _, v := range files {
		m.updateSendFile(host.Server, *v.file, status)
		blog.Debugf("remote: ready ensure single cork file(%s) to status(%s) for work(%s) to server(%s)",
			v.file.FilePath, status, m.work.ID(), host.Server)
		v.resultchan <- resultchan
		// blog.Infof("remote: end send file[%s] with cork to server %s tick for work: %s with err:%v, retcode:%d",
		// 	v.file.FilePath, host.Server, m.work.ID(), err, retcode)
	}

	blog.Debugf("remote: end send cork %d files with size:%d to server %s for work: %s with err:%v, retcode:%d",
		len(files), totalsize, host.Server, m.work.ID(), err, retcode)
}

func (m *Mgr) handleNetError(req *types.RemoteTaskExecuteRequest, err error) {
	for _, w := range m.resource.GetWorkers() {
		if !w.host.Equal(req.Server) {
			continue
		}
		m.resource.CountWorkerError(w)
		if m.resource.IsWorkerDead(w, m.conf.NetErrorLimit) {
			m.resource.WorkerDead(w)
			blog.Errorf("remote: server(%s) in work(%s) has the %dth continuous net errors:(%v), "+
				"make it dead", req.Server.Server, m.work.ID(), w.continuousNetErrors, err)
		}
		break
	}
}
