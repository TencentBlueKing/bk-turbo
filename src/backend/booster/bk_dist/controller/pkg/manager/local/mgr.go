/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package local

import (
	"context"
	"fmt"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/manager/recorder"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/client"

	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/resultcache"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/manager/analyser"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/util"
)

// 一个方案id对应的result cache相关信息
type data4resultcache struct {
	lock sync.RWMutex

	remoteWorker      dcSDK.RemoteWorker
	resultCacheList   map[string]*protocol.Host
	groupKey          string
	localGroupRecord  *resultcache.RecordGroup
	remoteGroupRecord map[string]*resultcache.RecordGroup

	remoteTriggleSecs int

	localResultCacheIndexNum int
	localResultCacheFileNum  int

	uniqID string
	user   string
	ip     string

	hashlock sync.RWMutex
	hasher   hash
}

func (d *data4resultcache) getHostStringByKey(key string) string {
	if d.hasher == nil {
		return ""
	}

	d.hashlock.RLock()
	defer d.hashlock.RUnlock()

	return d.hasher.GetNode(key)
}

func (d *data4resultcache) getHost(key string) *protocol.Host {
	hoststr := d.getHostStringByKey(key)
	if hoststr == "" {
		return nil
	}
	blog.Infof("local: got host string:%s with key:%s", hoststr, key)

	d.lock.RLock()
	defer d.lock.RUnlock()

	h, ok := d.resultCacheList[hoststr]
	if ok {
		return h
	}

	return nil
}

func (d *data4resultcache) getRecordGroup(key string) *resultcache.RecordGroup {
	hoststr := d.getHostStringByKey(key)
	if hoststr == "" {
		return nil
	}
	blog.Infof("local: got host string:%s with key:%s", hoststr, key)

	d.lock.RLock()
	defer d.lock.RUnlock()

	h, ok := d.remoteGroupRecord[hoststr]
	if ok {
		return h
	}

	return nil
}

func (d *data4resultcache) initCacheList(nodes []string) {
	tmplist := make(map[string]*protocol.Host)
	for _, v := range nodes {
		tmplist[v] = &protocol.Host{
			Server:       v,
			TokenString:  v,
			Hosttype:     protocol.HostRemote,
			Jobs:         1,
			Compresstype: protocol.CompressLZ4,
			Protocol:     "tcp",
		}
	}

	d.resultCacheList = tmplist
}

func (d *data4resultcache) initHash(nodes []string) {
	if len(nodes) == 0 {
		return
	}

	d.hashlock.Lock()
	defer d.hashlock.Unlock()

	d.hasher = NewHashRing(100)
	for _, n := range nodes {
		d.hasher.AddNode(n)
	}
}

// NewMgr get a new LocalMgr
func NewMgr(pCtx context.Context, work *types.Work) types.LocalMgr {
	ctx, _ := context.WithCancel(pCtx)

	return &Mgr{
		ctx:               ctx,
		work:              work,
		resource:          newResource(0, nil),
		pumpFileCache:     analyser.NewFileCache(),
		pumpRootCache:     analyser.NewRootCache(),
		checkApplyTick:    1 * time.Second,
		checkApplyTimeout: 20 * time.Second,
	}
}

// Mgr describe the local manager
// provides the local actions handler for work
type Mgr struct {
	ctx context.Context

	work     *types.Work
	resource *resource
	// initCancel context.CancelFunc

	pumpFileCache *analyser.FileCache
	pumpRootCache *analyser.RootCache

	recorder *recorder.Recorder

	checkApplyTick    time.Duration
	checkApplyTimeout time.Duration

	resultdata *data4resultcache
}

// Init do the initialization for local manager
func (m *Mgr) Init() {
	blog.Infof("local: init for work:%s", m.work.ID())

	dcFile.ResetFileInfoCache()
}

// Start start resource slots for local manager
func (m *Mgr) Start() {
	blog.Infof("local: start for work:%s", m.work.ID())

	settings := m.work.Basic().Settings()
	m.resource = newResource(settings.LocalTotalLimit, settings.UsageLimit)

	m.recorder, _ = m.work.GetRecorder(types.GlobalRecorderKey)

	// if m.initCancel != nil {
	// 	m.initCancel()
	// }
	ctx, _ := context.WithCancel(m.ctx)
	// m.initCancel = cancel

	// get result cache index now
	if settings.ProjectID != "" {
		usrname := ""
		usr, err := user.Current()
		if err != nil {
			blog.Warnf("local: get current user failed: %v", err)
		} else {
			usrname = usr.Username
		}
		ip := ""
		ips := util.GetIPAddress()
		if len(ips) > 0 {
			ip = ips[0]
		}

		m.resultdata = &data4resultcache{
			groupKey: settings.ProjectID,
			remoteWorker: client.NewCommonRemoteWorkerWithSlot(
				ctx,
				m.work.Config().SendFileMemoryLimit,
				m.work.Config().SendMemoryCache,
			),
			resultCacheList:          make(map[string]*protocol.Host),
			remoteGroupRecord:        make(map[string]*resultcache.RecordGroup),
			localResultCacheIndexNum: m.work.Config().ResultCacheIndexNum,
			localResultCacheFileNum:  m.work.Config().ResultCacheFileNum,
			uniqID:                   dcUtil.UniqID(),
			user:                     usrname,
			ip:                       ip,
		}

		rcl := m.work.Basic().GetCacheServer()
		if len(rcl) > 0 {
			m.resultdata.initCacheList(rcl)
			m.resultdata.initHash(rcl)
		}

		go m.initResultCacheIndex(settings.ProjectID)
	}

	m.resource.Handle(ctx)
}

// LockSlots lock a local slot
func (m *Mgr) LockSlots(usage dcSDK.JobUsage, weight int32) bool {
	return m.resource.Lock(usage, weight)
}

// UnlockSlots unlock a local slot
func (m *Mgr) UnlockSlots(usage dcSDK.JobUsage, weight int32) {
	m.resource.Unlock(usage, weight)
}

// TryLockSlots try lock a local slot
func (m *Mgr) TryLockSlots(usage dcSDK.JobUsage, weight int32) (bool, error) {
	return m.resource.TryLock(usage, weight)
}

// GetPumpCache get pump cache in work
func (m *Mgr) GetPumpCache() (*analyser.FileCache, *analyser.RootCache) {
	return m.pumpFileCache, m.pumpRootCache
}

func checkHttpConn(req *types.LocalTaskExecuteRequest) (*types.LocalTaskExecuteResult, error) {
	if !types.IsHttpConnStatusOk(req.HttpConnCache, req.HttpConnKey) {
		blog.Errorf("local: httpconncache exit execute pid(%d) command:[%s] for http connection[%s] error",
			req.Pid, strings.Join(req.Commands, " "), req.HttpConnKey)
		return &types.LocalTaskExecuteResult{
			Result: &dcSDK.LocalTaskResult{
				ExitCode: -1,
				Message:  types.ErrLocalHttpConnDisconnected.Error(),
				Stdout:   nil,
				Stderr:   nil,
			},
		}, types.ErrLocalHttpConnDisconnected
	}

	return nil, nil
}

// ExecuteTask 若是task command本身运行失败, 不作为execute失败, 将结果放在result中返回即可
// 只有筹备执行的过程中失败, 才作为execute失败
func (m *Mgr) ExecuteTask(
	req *types.LocalTaskExecuteRequest,
	globalWork *types.Work,
	canUseLocalIdleResource bool,
	f types.CallbackCheckResource) (*types.LocalTaskExecuteResult, error) {
	blog.Infof("local: try to execute task(%s) for work(%s) from pid(%d) in env(%v) dir(%s)",
		strings.Join(req.Commands, " "), m.work.ID(), req.Pid, req.Environments, req.Dir)

	e, err := newExecutor(m,
		req,
		globalWork,
		m.work.Resource().SupportAbsPath(),
		m.resultdata)
	if err != nil {
		blog.Errorf("local: try to execute task for work(%s) from pid(%d) get executor failed: %v",
			m.work.ID(), req.Pid, err)
		return nil, err
	}

	defer e.executeFinalTask()
	defer e.handleRecord()

	ret, err := checkHttpConn(req)
	if err != nil {
		return ret, err
	}

	// 该work被置为degraded || 该executor被置为degraded, 则直接走本地执行
	if m.work.Basic().Settings().Degraded || e.degrade() {
		blog.Warnf("local: execute task for work(%s) from pid(%d) degrade to local with degraded",
			m.work.ID(), req.Pid)
		return e.executeLocalTask(), nil
	}

	// 历史记录显示该任务多次远程失败，则直接走本地执行
	if e.retryAndSuccessTooManyAndDegradeDirectly() {
		blog.Warnf("local: execute task for work(%s) from pid(%d) degrade to local for too many failed",
			m.work.ID(), req.Pid)
		return e.executeLocalTask(), nil
	}
	resMode := f()
	// TODO : 本地空闲资源执行任务需要更多条件判断
	// 该任务已确定用本地资源运行，则直接走本地执行
	if canUseLocalIdleResource {
		if e.canExecuteWithLocalIdleResource() && resMode == types.LocalResourceMode {
			blog.Infof("local: execute task [%s] for work(%s) from pid(%d) degrade to local with local idle resource",
				req.Commands[0], m.work.ID(), req.Pid)
			return e.executeLocalTask(), nil
		}
	}

	// 优化没有远程资源转本地的逻辑； 如果没有远程资源，则先获取本地锁，然后转本地执行
	// 如果没有本地锁，则先等待，后面有远程资源时，则直接远程，无需全部阻塞在本地执行
	for {
		ret, err := checkHttpConn(req)
		if err != nil {
			return ret, err
		}

		// 先检查是否有远程资源
		if !m.work.Resource().HasAvailableWorkers() {
			// check whether this task need remote worker,
			// apply resource when need, if not in appling, apply then
			if e.needRemoteResource() {
				_, err := m.work.Resource().Apply(nil, false)
				if err != nil {
					blog.Warnf("local: execute task for work(%s) from pid(%d) failed to apply resource with err:%v",
						m.work.ID(), req.Pid, err)
				}
			}

			// 尝试本地执行
			blog.Infof("local: execute task for work(%s) from pid(%d) degrade to try local for no remote workers",
				m.work.ID(), req.Pid)
			ret := e.tryExecuteLocalTask()
			if ret != nil {
				return ret, nil
			}

			// failed to lock local, sleep
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}

	// TODO : check whether need more resource

	for resMode == types.WaitResourceMode {
		blog.Debugf("local: execute task for work(%s) from pid(%d) still wait for resource", m.work.ID(), req.Pid)
		resMode = f()
		switch resMode {
		case types.LocalResourceMode:
			blog.Infof("local: execute task for work(%s) from pid(%d) degrade to local", m.work.ID(), req.Pid)
			return e.executeLocalTask(), nil
		case types.RemoteResourceMode:
			blog.Infof("local: execute task for work(%s) from pid(%d) try to execute with remote resource", m.work.ID(), req.Pid)
			break
		default:
			time.Sleep(1 * time.Second)
		}
	}

	// !! remember dec after finished remote execute !!
	m.work.Basic().Info().IncPrepared()
	m.work.Remote().IncRemoteJobs()

	defer func() {
		m.work.Basic().Info().DecPrepared()
		m.work.Remote().DecRemoteJobs()
	}()

	ret, err = checkHttpConn(req)
	if err != nil {
		return ret, err
	}

	c, err := e.executePreTask()
	if err != nil {
		// m.work.Basic().Info().DecPrepared()
		// m.work.Remote().DecRemoteJobs()
		blog.Warnf("local: execute pre-task for work(%s) from pid(%d) : %v", m.work.ID(), req.Pid, err)
		return e.executeLocalTask(), nil
	}

	// TODO : try to query cache if enabled and with preprocess
	// 需要handle提供cache的key，从c的ResultFiles获取预期的结果列表
	// 如果cache查询到了，则按预期的结果列表保存
	cacheresult, fromlocal := e.getCacheResult(c)
	if cacheresult != nil {
		blog.Infof("local: success to execute task by query cache for work(%s)"+
			" from pid(%d) in env(%v) dir(%s)",
			m.work.ID(),
			req.Pid, req.Environments, req.Dir)
		req.Stats.TBSPreprocessHit = true
		go e.putHitRecord(fromlocal)
		return cacheresult, nil
	}

	var r *types.RemoteTaskExecuteResult
	remoteReq := &types.RemoteTaskExecuteRequest{
		Pid:           req.Pid,
		Req:           c,
		Stats:         req.Stats,
		Sandbox:       e.sandbox,
		IOTimeout:     e.ioTimeout,
		BanWorkerList: []*protocol.Host{},
		HttpConnCache: req.HttpConnCache,
		HttpConnKey:   req.HttpConnKey,
		Attributes:    req.Attributes,
	}

	for i := 0; i < m.getTryTimes(e); i++ {
		ret, err = checkHttpConn(req)
		if err != nil {
			return ret, err
		}

		req.Stats.RemoteTryTimes = i + 1
		r, err = m.work.Remote().ExecuteTask(remoteReq)
		if err != nil {
			blog.Warnf("local: execute remote-task for work(%s) from pid(%d) (%d)try failed: %v",
				m.work.ID(),
				req.Pid,
				i,
				err)
			req.Stats.RemoteErrorMessage = err.Error()
			if !needRetry(req) {
				blog.Warnf("local: execute remote-task for work(%s) from pid(%d) (%d)try failed with error: %v",
					m.work.ID(), req.Pid, i, err)
				break
			}
			// 远程任务失败后，将文件大小和压缩大小都置为初始值，方便其他worker重试
			for i, c := range remoteReq.Req.Commands {
				for j, f := range c.Inputfiles {
					if f.CompressedSize < 0 && f.InitCompressedSize >= 0 {
						remoteReq.Req.Commands[i].Inputfiles[j].CompressedSize = remoteReq.Req.Commands[i].Inputfiles[j].InitCompressedSize
					}
					if f.FileSize < 0 && f.InitFileSize >= 0 {
						remoteReq.Req.Commands[i].Inputfiles[j].FileSize = remoteReq.Req.Commands[i].Inputfiles[j].InitFileSize
					}
				}
			}
			blog.Infof("local: retry remote-task from work(%s) for the(%d) time from pid(%d) "+
				"with error(%v),ban (%d) worker:(%s)",
				m.work.ID(),
				i+1,
				req.Pid,
				err.Error(),
				len(remoteReq.BanWorkerList),
				remoteReq.BanWorkerList)
		} else {
			break
		}
	}

	if e.isUbaCommand {
		blog.Infof("local: return for ubaagent now")
		return &types.LocalTaskExecuteResult{
			Result: &dcSDK.LocalTaskResult{
				ExitCode: 0,
				Stdout:   nil,
				Stderr:   nil,
				Message:  "",
			},
		}, nil
	}

	// m.work.Basic().Info().DecPrepared()
	// m.work.Remote().DecRemoteJobs()
	if err != nil {
		ret, err = checkHttpConn(req)
		if err != nil {
			return ret, err
		}

		if err == types.ErrSendFileFailed {
			blog.Infof("local: retry remote-task failed from work(%s) for (%d) times from pid(%d)"+
				" with send file error, retryOnRemoteFail now",
				m.work.ID(), req.Stats.RemoteTryTimes, req.Pid)

			lr, err := m.retryOnRemoteFail(req, globalWork, e)
			if err == nil && lr != nil {
				return lr, err
			}
		}

		blog.Infof("local: retry remote-task failed from work(%s) for (%d) times from pid(%d), turn it local",
			m.work.ID(), req.Stats.RemoteTryTimes, req.Pid)

		if !req.NeedLocalExecute() {
			blog.Infof("local: command[%s] do not need local execute", strings.Join(req.Commands, " "))
			return nil, err
		}

		return e.executeLocalTask(), nil
	}

	err = e.executePostTask(r.Result)
	if err != nil {
		blog.Warnf("local: execute post-task for work(%s) from pid(%d) failed: %v", m.work.ID(), req.Pid, err)
		req.Stats.RemoteErrorMessage = err.Error()

		ret, err = checkHttpConn(req)
		if err != nil {
			return ret, err
		}

		lr, err := m.retryOnRemoteFail(req, globalWork, e)
		if err == nil && lr != nil {
			return lr, err
		}

		if !e.skipLocalRetry() {
			return e.executeLocalTask(), nil
		}

		blog.Warnf("local: executor skip local retry for work(%s) from pid(%d) "+
			"and return remote err directly: %v", m.work.ID(), req.Pid, err)
		return &types.LocalTaskExecuteResult{
			Result: &dcSDK.LocalTaskResult{
				ExitCode: 1,
				Stdout:   []byte(err.Error()),
				Stderr:   []byte(err.Error()),
				Message:  "executor skip local retry",
			},
		}, nil
	}

	// TODO : try to put to cache if enabled and with preprocess
	// 由handle提供key和结果，结果可能是多个文件，先考虑用后缀标识
	// 这儿的结果文件，可以是解压后的完整文件，也可以是内存中的压缩数据
	// 前期考虑直接用解压后的完整文件，减少压缩和解压的开销，但会增加存储开销
	go func() {
		err := e.putCacheResult(r.Result, req.Stats)
		blog.Infof("local: put to cache with error:%v", err)
	}()

	req.Stats.Success = true
	m.work.Basic().UpdateJobStats(req.Stats)
	blog.Infof("local: success to execute task for work(%s) from pid(%d) in env(%v) dir(%s)",
		m.work.ID(), req.Pid, req.Environments, req.Dir)
	return &types.LocalTaskExecuteResult{
		Result: &dcSDK.LocalTaskResult{
			ExitCode: 0,
			Stdout:   e.Stdout(),
			Stderr:   e.Stderr(),
			Message:  "success to process all steps",
		},
	}, nil
}

// 远程失败后调用handle的特殊处理函数，方便支持某些特殊流程
func (m *Mgr) retryOnRemoteFail(
	req *types.LocalTaskExecuteRequest,
	globalWork *types.Work,
	e *executor) (*types.LocalTaskExecuteResult, error) {
	blog.Infof("local: onRemoteFail with task(%s) for work(%s) from pid(%d) ",
		strings.Join(req.Commands, " "), m.work.ID(), req.Pid)

	m.work.Basic().Info().IncPrepared()
	m.work.Remote().IncRemoteJobs()

	// 重新走流程，比如预处理
	cnew, errnew := e.onRemoteFail()
	if cnew != nil && errnew == nil {
		remoteReqNew := &types.RemoteTaskExecuteRequest{
			Pid:           req.Pid,
			Req:           cnew,
			Stats:         req.Stats,
			Sandbox:       e.sandbox,
			IOTimeout:     e.ioTimeout,
			BanWorkerList: []*protocol.Host{},
		}
		// 重新远程执行命令
		r, err := m.work.Remote().ExecuteTask(remoteReqNew)

		m.work.Basic().Info().DecPrepared()
		m.work.Remote().DecRemoteJobs()

		if err != nil {
			blog.Warnf("local: failed to remote in onRemoteFail from work(%s) from pid(%d) with error(%v)",
				m.work.ID(),
				req.Pid,
				err)
			return nil, err
		}

		blog.Infof("local: succeed to remote in onRemoteFail from work(%s) from pid(%d)", m.work.ID(), req.Pid)
		// 重新post环节
		err = e.executePostTask(r.Result)
		if err != nil {
			blog.Warnf("local: execute post-task in onRemoteFail for work(%s) from pid(%d) failed: %v",
				m.work.ID(),
				req.Pid,
				err)
			return nil, err
		}

		req.Stats.Success = true
		m.work.Basic().UpdateJobStats(req.Stats)
		blog.Infof("local: success to execute post task in onRemoteFail for work(%s) from pid(%d) in env(%v) dir(%s)",
			m.work.ID(), req.Pid, req.Environments, req.Dir)
		return &types.LocalTaskExecuteResult{
			Result: &dcSDK.LocalTaskResult{
				ExitCode: 0,
				Stdout:   e.Stdout(),
				Stderr:   e.Stderr(),
				Message:  "success to process all steps",
			},
		}, nil
	}

	m.work.Basic().Info().DecPrepared()
	m.work.Remote().DecRemoteJobs()

	return nil, nil
}

// Slots get current total and occupied slots
func (m *Mgr) Slots() (int, int) {
	return m.resource.GetStatus()
}

func (m *Mgr) waitApplyFinish() error {
	ctx, _ := context.WithCancel(m.ctx)
	blog.Infof("local: run wait apply finish tick for work(%s)", m.work.ID())
	ticker := time.NewTicker(m.checkApplyTick)
	defer ticker.Stop()
	timer := time.NewTimer(m.checkApplyTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			blog.Infof("local: run wait apply finish tick  for work(%s) canceled by context", m.work.ID())
			return fmt.Errorf("canceld by context")

		case <-ticker.C:
			// get apply status
			if m.work.Resource().IsApplyFinished() {
				return nil
			}

		case <-timer.C:
			// check timeout
			blog.Infof("local: wait apply status timeout for work(%s)", m.work.ID())
			return fmt.Errorf("wait apply status timeout")
		}
	}
}

func needRetry(req *types.LocalTaskExecuteRequest) bool {
	// do not retry if remote timeout
	if req.Stats.RemoteWorkTimeout {
		return false
	}
	return true
}

func (m *Mgr) getTryTimes(e *executor) int {
	// hander 配置优先
	if e.remoteTryTimes() > 1 {
		return e.remoteTryTimes()
	}
	return m.work.Config().RemoteRetryTimes + 1
}

// --------------------------for result cache-----------------------------
func (m *Mgr) initResultCacheIndex(projectid string) {
	// get local firstly
	m.getLocalResultCacheIndex()

	// get remote then
	// TODO : 先从tbs server获取cache列表，然后依次拉取结果索引
	// 如果没有指定cache 列表，则从tbs server拉取
	if len(m.resultdata.resultCacheList) == 0 {
		blog.Infof("local: start get cache list now")
		resouceMgr := m.work.Resource()
		if resouceMgr != nil {
			cachelist, err := resouceMgr.GetCacheList(projectid)
			if err == nil && cachelist != nil && len(*cachelist) > 0 {
				blog.Infof("local: got cache list:%v", *cachelist)
				for _, v := range *cachelist {
					if len(v.Hosts) > 0 {
						m.resultdata.initCacheList(v.Hosts)
						m.resultdata.initHash(v.Hosts)
					}
					if v.RemoteExecuteTimeThreshold > 0 {
						m.resultdata.remoteTriggleSecs = v.RemoteExecuteTimeThreshold
					}
					break
				}
			}
		}
	}

	if len(m.resultdata.resultCacheList) > 0 {
		for k, v := range m.resultdata.resultCacheList {
			host := v
			go m.getRemoteResultCacheIndex(k, host)
		}
	}
}

func (m *Mgr) getLocalResultCacheIndex() {
	data, err := resultcache.GetInstance("",
		m.resultdata.localResultCacheFileNum,
		m.resultdata.localResultCacheIndexNum).GetRecordGroup(m.resultdata.groupKey)
	if err == nil && len(data) > 0 {
		m.resultdata.lock.Lock()
		m.resultdata.localGroupRecord, _ = resultcache.ToRecordGroup(data)
		m.resultdata.lock.Unlock()

		// blog.Infof("local: got local group record:[%s] with key:%s",
		// 	m.resultdata.localGroupRecord.ToString(), m.resultdata.groupKey)
	}
}

func (m *Mgr) getRemoteResultCacheIndex(key string, host *protocol.Host) {
	blog.Infof("local: get remote result cache index from host:%s now", key)
	if m.resultdata.remoteWorker != nil && host != nil {
		handler := m.resultdata.remoteWorker.Handler(0, nil, nil, nil)
		record := resultcache.Record{
			resultcache.GroupKey: m.resultdata.groupKey,
		}
		result, err := handler.ExecuteQueryResultCacheIndex(
			host,
			record,
		)
		if err == nil && result != nil && len(result.ResultIndex) > 0 {
			oneGroupRecord, _ := resultcache.ToRecordGroup(result.ResultIndex)
			if oneGroupRecord != nil {
				m.resultdata.lock.Lock()
				m.resultdata.remoteGroupRecord[key] = oneGroupRecord
				m.resultdata.lock.Unlock()
			}
			blog.Infof("local: got one remote group record len:[%d] with key:%s from host:%s",
				len(result.ResultIndex), m.resultdata.groupKey, key)
			// blog.Infof("local: got one remote group record:[%s] with key:%s from host:%s",
			// 	oneGroupRecord.ToString(), m.resultdata.groupKey, key)
		}
	}
}

func (m *Mgr) hitLocalIndex(record resultcache.Record) (bool, error) {
	m.resultdata.lock.RLock()
	defer m.resultdata.lock.RUnlock()

	if m.resultdata.localGroupRecord == nil {
		return false, nil
	}

	return m.resultdata.localGroupRecord.HitIndex(record)
}

func (m *Mgr) hitRemoteIndex(command string, record resultcache.Record) (bool, error) {
	groupRecord := m.resultdata.getRecordGroup(command)
	if groupRecord == nil {
		return false, nil
	}

	m.resultdata.lock.RLock()
	defer m.resultdata.lock.RUnlock()

	return groupRecord.HitIndex(record)
}

func (m *Mgr) getRemoteResultCacheFile(
	command string,
	groupkey,
	resultkey string) (*dcSDK.BKQueryResultCacheFileResult, error) {

	host := m.resultdata.getHost(command)
	if host == nil {
		return nil, fmt.Errorf("not found any host")
	}

	if m.resultdata.remoteWorker != nil {
		handler := m.resultdata.remoteWorker.Handler(0, nil, nil, nil)
		record := resultcache.Record{
			resultcache.GroupKey:  groupkey,
			resultcache.ResultKey: resultkey,
		}
		result, err := handler.ExecuteQueryResultCacheFile(
			host,
			record,
		)
		if err == nil && result != nil {
			blog.Infof("local: got [%d] result files with key:%s from host:%s",
				len(result.Resultfiles), resultkey, host.Server)
		}

		return result, err
	}

	return nil, fmt.Errorf("not got result")
}

func (m *Mgr) reportRemoteResultCache(
	command string,
	record resultcache.Record,
	results []*dcSDK.FileDesc) (*dcSDK.BKReportResultCacheResult, error) {

	host := m.resultdata.getHost(command)
	if host == nil {
		return nil, nil
	}

	if m.resultdata.remoteWorker != nil {
		handler := m.resultdata.remoteWorker.Handler(0, nil, nil, nil)
		result, err := handler.ExecuteReportResultCache(
			host,
			record,
			results)

		if result != nil {
			blog.Infof("local: report result files to remote with retcode:%d,out message:%s,error message:%s",
				result.RetCode, result.OutputMessage, result.ErrorMessage)
		}

		return result, err
	}

	return nil, nil
}
