/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package types

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/manager/recorder"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
)

const (
	WorkIDLength = 5

	GlobalRecorderKey = "global_records"
)

type UbaInfo struct {
	TraceFile  string            `json:"tracefile"`
	SessionMap map[string]string `json:"sessionmap"`
}

// MgrSet describe the newer functions for work
type MgrSet struct {
	Basic    func(context.Context, *Work) BasicMgr
	Local    func(context.Context, *Work) LocalMgr
	Remote   func(context.Context, *Work) RemoteMgr
	Resource func(context.Context, *Work) ResourceMgr
}

// NewWork get a new Work
func NewWork(id string, conf *config.ServerConfig, mgrSet MgrSet, rp *recorder.RecordersPool) *Work {
	ctx, cancel := context.WithCancel(context.Background())
	work := &Work{
		id:     id,
		conf:   conf,
		ctx:    ctx,
		cancel: cancel,
		rp:     rp,
	}
	work.basic = mgrSet.Basic(ctx, work)
	work.local = mgrSet.Local(ctx, work)
	work.remote = mgrSet.Remote(ctx, work)
	work.resource = mgrSet.Resource(ctx, work)
	work.basic.Info().Init()

	// TODO : work.local need Init also, but it depend basic.Setting
	work.local.Init()
	work.remote.Init()

	return work
}

// Work describe a set of complete actions
// one work, one resource
type Work struct {
	sync.RWMutex
	lockTime  time.Time
	rLockTime time.Time

	id string

	ctx    context.Context
	cancel context.CancelFunc

	conf *config.ServerConfig
	rp   *recorder.RecordersPool

	basic    BasicMgr
	local    LocalMgr
	remote   RemoteMgr
	resource ResourceMgr
}

// Lock lock work
func (w *Work) Lock() {
	enter := time.Now().Local()
	w.RWMutex.Lock()
	w.lockTime = time.Now().Local()
	if duration := w.lockTime.Sub(enter); duration >= 100*time.Millisecond {
		blog.Warnf("work lock wait too long time(%s) from [%s]", duration.String(), dcUtil.GetCaller())
	}
}

// Unlock unlock work
func (w *Work) Unlock() {
	if duration := time.Now().Local().Sub(w.lockTime); duration >= 100*time.Millisecond {
		blog.Warnf("work lock hold too long time(%s) from [%s]", duration.String(), dcUtil.GetCaller())
	}
	w.RWMutex.Unlock()
}

// RLock rLock work
func (w *Work) RLock() {
	enter := time.Now().Local()
	w.RWMutex.RLock()
	w.rLockTime = time.Now().Local()
	if duration := w.rLockTime.Sub(enter); duration >= 100*time.Millisecond {
		blog.Warnf("work rLock wait too long time(%s) from [%s]", duration.String(), dcUtil.GetCaller())
	}
}

// RUnlock rUnlock work
func (w *Work) RUnlock() {
	if duration := time.Now().Local().Sub(w.rLockTime); duration >= 100*time.Millisecond {
		blog.Warnf("work rLock hold too long time(%s) from [%s]", duration.String(), dcUtil.GetCaller())
	}
	w.RWMutex.RUnlock()
}

// Cancel cancel all handlers in work
func (w *Work) Cancel() {
	if w.cancel != nil {
		w.cancel()
	}
}

// ID return work id
func (w *Work) ID() string {
	return w.id
}

// Basic return basic manager
func (w *Work) Basic() BasicMgr {
	return w.basic
}

// Local return local manager
func (w *Work) Local() LocalMgr {
	return w.local
}

// LockMgr return local manager as dcSDK.LockMgr
func (w *Work) LockMgr() dcSDK.LockMgr {
	return w.local
}

// Remote return remote manager
func (w *Work) Remote() RemoteMgr {
	return w.remote
}

// Resource return resource manager
func (w *Work) Resource() ResourceMgr {
	return w.resource
}

// GetRecorder return the Recorder instance for given key
func (w *Work) GetRecorder(key string) (*recorder.Recorder, error) {
	if w.rp == nil {
		return nil, fmt.Errorf("mgr: get recoder failed, recorders pool is empty")
	}

	r, err := w.rp.GetRecorder(key)
	if err != nil {
		blog.Infof("mgr: get recorder for %s failed: %v", key, err)
		return nil, err
	}

	blog.Infof("mgr: success to get recorder for %s", key)
	return r, nil
}

// Config return config
func (w *Work) Config() *config.ServerConfig {
	return w.conf
}

// NewInitWorkInfo get a new work info by id
func NewInitWorkInfo(id string) *WorkInfo {
	info := &WorkInfo{
		workID:       id,
		success:      true,
		commonStatus: &WorkCommonStatus{},
	}

	info.commonStatus.init()
	return info
}

// WorkInfo describe the information in a single work
type WorkInfo struct {
	workID    string
	taskID    string
	projectID string
	scene     string
	success   bool
	batchMode bool

	bazelNoLauncher bool

	lastHeartbeat time.Time

	commonStatus *WorkCommonStatus

	// to record prepared remote task number
	preparedRemoteTasks int32
}

// SetSettings update work settings
func (wi *WorkInfo) SetSettings(settings *WorkSettings) {
	wi.taskID = settings.TaskID
	wi.projectID = settings.ProjectID
	wi.scene = settings.Scene
}

// Dump encode work info into json
func (wi WorkInfo) Dump() []byte {
	var data []byte
	_ = codec.EncJSON(wi, &data)
	return data
}

// WorkID return work id
func (wi WorkInfo) WorkID() string {
	return wi.workID
}

// TaskID return task id
func (wi WorkInfo) TaskID() string {
	return wi.taskID
}

// ProjectID return project id
func (wi WorkInfo) ProjectID() string {
	return wi.projectID
}

// SetProjectID update project id
func (wi *WorkInfo) SetProjectID(projectID string) {
	wi.projectID = projectID
}

// Scene return scene
func (wi WorkInfo) Scene() string {
	return wi.scene
}

// SetScene update scene
func (wi *WorkInfo) SetScene(scene string) {
	wi.scene = scene
}

// Success check if work is successful
func (wi WorkInfo) Success() bool {
	return wi.success
}

// LastHeartbeat return the last heartbeat time
func (wi *WorkInfo) LastHeartbeat() time.Time {
	return wi.lastHeartbeat
}

// UpdateHeartbeat update heartbeat time
func (wi *WorkInfo) UpdateHeartbeat(t time.Time) {
	wi.lastHeartbeat = t
}

// IsBatchMode check if the work is in batch mode
func (wi *WorkInfo) IsBatchMode() bool {
	return wi.batchMode
}

// CommonStatus return work common status
func (wi *WorkInfo) CommonStatus() WorkCommonStatus {
	return *wi.commonStatus
}

// Status return work status
func (wi *WorkInfo) Status() dcSDK.WorkStatus {
	return wi.commonStatus.Status
}

// Init set work init
func (wi *WorkInfo) Init() {
	wi.commonStatus.init()
}

// SetStatusMessage update the status message
func (wi *WorkInfo) SetStatusMessage(s string) {
	wi.commonStatus.setStatusMessage(s)
}

// StatusMessage return the status message
func (wi *WorkInfo) StatusMessage() string {
	return wi.commonStatus.statusMessage()
}

// Register set work register
func (wi *WorkInfo) Register(batchMode bool) {
	wi.batchMode = batchMode
	wi.commonStatus.register()
}

// Start set work start time
func (wi *WorkInfo) RegisterTime(t time.Time) {
	wi.commonStatus.registerTime(t)
}

// Unregister set work unregister
func (wi *WorkInfo) Unregister() {
	wi.commonStatus.unregister()
}

// ResourceApplying set work resource apply
func (wi *WorkInfo) ResourceApplying() {
	wi.commonStatus.resourceApplying()
}

// ResourceApplied set work resource applied
func (wi *WorkInfo) ResourceApplied() {
	wi.commonStatus.resourceApplied()
}

// ResourceApplyFailed set work  resource apply failed
func (wi *WorkInfo) ResourceApplyFailed() {
	wi.commonStatus.resourceApplyFailed()
}

// SetRemovable set work removable
func (wi *WorkInfo) SetRemovable() {
	wi.commonStatus.setRemovable()
}

// Start set work start
func (wi *WorkInfo) Start() {
	wi.commonStatus.start()
}

// Start set work start time
func (wi *WorkInfo) StartTime(t time.Time) {
	wi.commonStatus.startTime(t)
}

// End set work end
func (wi *WorkInfo) End(timeoutBefore time.Duration) {
	wi.commonStatus.end(timeoutBefore)
}

// End set work end time
func (wi *WorkInfo) EndTime(t time.Time) {
	wi.commonStatus.endTime(t)
}

// CanBeRegistered check if work can be registered now
func (wi *WorkInfo) CanBeRegister() bool {
	return wi.commonStatus.Status.CanBeRegistered()
}

// CanBeUnregistered check if work can be unregistered now
func (wi *WorkInfo) CanBeUnregistered() bool {
	return wi.commonStatus.Status.CanBeUnregistered()
}

// CanBeSetSettings check if work can be set settings now
func (wi *WorkInfo) CanBeSetSettings() bool {
	return wi.commonStatus.Status.CanBeSetSettings()
}

// CanBeResourceApplying check if work can apply resource now
func (wi *WorkInfo) CanBeResourceApplying() bool {
	return wi.commonStatus.Status.CanBeResourceApplying()
}

// CanBeResourceApplied check if work can be set resource applied now
func (wi *WorkInfo) CanBeResourceApplied() bool {
	return wi.commonStatus.Status.CanBeResourceApplied()
}

// CanBeResourceApplyFailed check if work can be set resource apply failed now
func (wi *WorkInfo) CanBeResourceApplyFailed() bool {
	return wi.commonStatus.Status.CanBeResourceApplyFailed()
}

// CanBeStart check if work can be started now
func (wi *WorkInfo) CanBeStart() bool {
	return wi.commonStatus.Status.CanBeStart()
}

// CanBeEnd check if work can be ended now
func (wi *WorkInfo) CanBeEnd() bool {
	return wi.commonStatus.Status.CanBeEnd()
}

// CanBeRemoved check if work can be removed now
func (wi *WorkInfo) CanBeRemoved() bool {
	return wi.commonStatus.Status.CanBeRemoved()
}

// CanBeHeartbeat check if work can be updated heartbeat from booster now
func (wi *WorkInfo) CanBeHeartbeat() bool {
	return wi.commonStatus.Status.CanBeHeartbeat()
}

// IsUnregistered check if work is unregistered
func (wi *WorkInfo) IsUnregistered() bool {
	return wi.commonStatus.Status.IsUnregistered()
}

// IsWorking check if work is working
func (wi *WorkInfo) IsWorking() bool {
	return wi.commonStatus.Status.IsWorking()
}

// SetStats update the work stats
func (wi *WorkInfo) SetStats(stats *WorkStats) {
	wi.success = stats.Success
}

// IncPrepared to inc preared remote task number
func (wi *WorkInfo) IncPrepared() {
	atomic.AddInt32(&wi.preparedRemoteTasks, 1)
}

// DecPrepared to inc preared remote task number
func (wi *WorkInfo) DecPrepared() {
	atomic.AddInt32(&wi.preparedRemoteTasks, -1)
}

// GetPrepared to get preared remote task number
func (wi *WorkInfo) GetPrepared() int32 {
	return atomic.LoadInt32(&wi.preparedRemoteTasks)
}

// BazelNoLauncher return bazelNoLauncher
func (wi WorkInfo) BazelNoLauncher() bool {
	return wi.bazelNoLauncher
}

// SetBazelNoLauncher update bazelNoLauncher
func (wi *WorkInfo) SetBazelNoLauncher(flag bool) {
	wi.bazelNoLauncher = flag
}

// WorkCommonStatus describe the work status and actions timestamp
type WorkCommonStatus struct {
	Status                  dcSDK.WorkStatus
	Message                 string
	LastStatusChangeTime    time.Time
	InitTime                time.Time
	StartTime               time.Time
	EndTime                 time.Time
	RegisteredTime          time.Time
	UnregisteredTime        time.Time
	ResourceApplyingTime    time.Time
	ResourceAppliedTime     time.Time
	ResourceApplyFailedTime time.Time
	RemovableTime           time.Time
}

func (wcs *WorkCommonStatus) setStatusMessage(s string) {
	wcs.Message = s
}

func (wcs *WorkCommonStatus) statusMessage() string {
	return wcs.Message
}

func (wcs *WorkCommonStatus) init() {
	wcs.InitTime = now()
	wcs.changeStatus(dcSDK.WorkStatusInit)
}

func (wcs *WorkCommonStatus) register() {
	wcs.RegisteredTime = now()
	wcs.changeStatus(dcSDK.WorkStatusRegistered)
}

func (wcs *WorkCommonStatus) registerTime(t time.Time) {
	wcs.RegisteredTime = t
}

func (wcs *WorkCommonStatus) unregister() {
	wcs.UnregisteredTime = now()
	wcs.changeStatus(dcSDK.WorkStatusUnregistered)
}

func (wcs *WorkCommonStatus) resourceApplying() {
	wcs.ResourceApplyingTime = now()
	wcs.changeStatus(dcSDK.WorkStatusResourceApplying)
}

func (wcs *WorkCommonStatus) resourceApplied() {
	wcs.ResourceAppliedTime = now()
	wcs.changeStatus(dcSDK.WorkStatusResourceApplied)
}

func (wcs *WorkCommonStatus) resourceApplyFailed() {
	wcs.ResourceApplyFailedTime = now()
	wcs.changeStatus(dcSDK.WorkStatusResourceApplyFailed)
}

func (wcs *WorkCommonStatus) setRemovable() {
	wcs.RemovableTime = now()
	wcs.changeStatus(dcSDK.WorkStatusRemovable)
}

func (wcs *WorkCommonStatus) start() {
	wcs.StartTime = now()
	wcs.changeStatus(dcSDK.WorkStatusWorking)
}

func (wcs *WorkCommonStatus) startTime(t time.Time) {
	wcs.StartTime = t
}

func (wcs *WorkCommonStatus) end(timeoutBefore time.Duration) {
	wcs.EndTime = now().Add(-timeoutBefore)
	wcs.changeStatus(dcSDK.WorkStatusEnded)
}

func (wcs *WorkCommonStatus) endTime(t time.Time) {
	wcs.EndTime = t
}

func (wcs *WorkCommonStatus) changeStatus(status dcSDK.WorkStatus) {
	if wcs.Status == status {
		return
	}

	wcs.LastStatusChangeTime = now()
	wcs.Status = status
}

// NewWorkAnalysisStatus get a new WorkAnalysisStatus
func NewWorkAnalysisStatus() *WorkAnalysisStatus {
	return &WorkAnalysisStatus{
		ubainfos: make(map[string]UbaInfo),
		indexMap: make(map[string]int),
		jobs:     make([]*dcSDK.ControllerJobStats, 0, 1000),
	}
}

// WorkAnalysisStatus describe the work analyasus status
type WorkAnalysisStatus struct {
	mutex    sync.RWMutex
	ubainfos map[string]UbaInfo
	indexMap map[string]int
	jobs     []*dcSDK.ControllerJobStats
}

func (was *WorkAnalysisStatus) SetUbaInfo(ubainfo UbaInfo) {
	was.mutex.Lock()
	defer was.mutex.Unlock()
	was.ubainfos[ubainfo.TraceFile] = ubainfo
	blog.Debugf("ubatrace: save UBA file: %s to map", ubainfo.TraceFile)
}

// 将微秒时间戳转换为 time.Time
func MicrosecondsToTime(us int64) time.Time {
	// 1 微秒 = 1000 纳秒
	return time.Unix(0, us*1000)
}

func (was *WorkAnalysisStatus) readUBAFile(ubainfo UbaInfo, taskid, workid string) error {
	blog.Infof("ubatrace: start read UBA file: %s", ubainfo.TraceFile)
	traceView, err := ReadUBAFile(ubainfo.TraceFile)
	if err == nil && traceView != nil {
		for _, s := range traceView.Sessions {
			ip := ""
			if sip, ok := ubainfo.SessionMap[s.Name]; ok {
				ip = sip
			}
			for _, processor := range s.Processors {
				// convert processes to []*dcSDK.ControllerJobStats
				for _, process := range processor.Processes {
					flag := s.FullName + "_" + process.Description
					start := float64(process.Start) / float64(traceView.Frequency)
					stop := float64(process.Stop) / float64(traceView.Frequency)

					// mac 需要乘以 100，Frequency 为 1000000000
					if runtime.GOOS == "darwin" {
						start = start * 100
						stop = stop * 100
					}
					startus := uint64(start*1000000) + traceView.SystemStartTimeUs
					stopus := uint64(stop*1000000) + traceView.SystemStartTimeUs
					fmt.Printf("flag:[%s]start[%s]stop[%s]\n",
						flag,
						formatTimeUS(int64(startus)),
						formatTimeUS(int64(stopus)))

					var stats dcSDK.ControllerJobStats
					stats.ID = flag
					stats.EnterTime = dcSDK.StatsTime(MicrosecondsToTime(int64(startus)))
					stats.LeaveTime = dcSDK.StatsTime(MicrosecondsToTime(int64(stopus)))
					stats.Success = process.ExitCode == 0
					if process.IsRemote {
						stats.RemoteWorkEnterTime = stats.EnterTime
						stats.RemoteWorkLeaveTime = stats.LeaveTime
						stats.RemoteWorkStartTime = stats.EnterTime
						stats.RemoteWorkEndTime = stats.LeaveTime
						stats.RemoteWorkProcessStartTime = stats.EnterTime
						stats.RemoteWorkProcessEndTime = stats.LeaveTime
						stats.RemoteWorker = ip
						stats.PreWorkSuccess = stats.Success
						stats.RemoteWorkSuccess = stats.Success
						stats.PostWorkSuccess = stats.Success
						stats.FinalWorkSuccess = stats.Success
					} else {
						stats.LocalWorkEnterTime = stats.EnterTime
						stats.LocalWorkLeaveTime = stats.LeaveTime
						stats.LocalWorkStartTime = stats.EnterTime
						stats.LocalWorkEndTime = stats.LeaveTime
						stats.LocalWorkLockTime = stats.EnterTime
						stats.LocalWorkUnlockTime = stats.LeaveTime // 方便显示
						stats.LocalWorkSuccess = stats.Success
					}
					if len(stats.OriginArgs) == 0 {
						fileds := strings.Split(process.Description, " ")
						stats.OriginArgs = []string{guessCommand(fileds[0]), process.Description}
					}
					stats.TaskID = taskid
					stats.WorkID = workid

					was.Update(&stats)
				}
			}
		}
	}

	return err
}

func guessCommand(describe string) string {
	suffix := filepath.Ext(describe)
	switch suffix {
	case ".cpp", ".c", ".cc", ".cxx", ".C", ".CXX", ".h":
		return "cl.exe"
	case ".dll":
		return "link.exe"
	case ".lib":
		return "lib.exe"
	case ".rc2", ".rc":
		return "rc.exe"
	case ".ispc":
		return "ispc.exe"
	case ".bat":
		return "cmd.exe"
	case ".target", ".version":
		return "dotnet.exe"
	case ".in":
		return "ShaderCompileWorker.exe"
	}
	return "other.exe"
}

// DumpJobs encode WorkAnalysisStatus into json bytes
func (was *WorkAnalysisStatus) DumpJobs(taskid, workid string) []byte {
	if len(was.ubainfos) > 0 {
		blog.Infof("ubatrace: len(was.ubainfos) > 0")
		for _, ubainfo := range was.ubainfos {
			was.readUBAFile(ubainfo, taskid, workid)
		}
	}

	var data []byte
	_ = codec.EncJSON(was.jobs, &data)
	return data
}

// Update 管理所有的job stats信息, 根据pid和进入executor的时间作为key来存储,
// 默认同一个key的caller需要保证stats是按照时间顺序来update的, 后来的会直接覆盖前者
func (was *WorkAnalysisStatus) Update(stats *dcSDK.ControllerJobStats) {
	key := stats.ID
	if key == "" {
		return
	}

	was.mutex.Lock()
	defer was.mutex.Unlock()

	if _, ok := was.indexMap[key]; !ok {
		was.indexMap[key] = len(was.jobs)
		was.jobs = append(was.jobs, stats)
		return
	}

	was.jobs[was.indexMap[key]] = stats
}

// BasicCount return some counts for job data
func (was *WorkAnalysisStatus) BasicCount() (remoteOK, remoteError, localOK, localError int) {
	was.mutex.RLock()
	defer was.mutex.RUnlock()

	for _, job := range was.jobs {
		if job.RemoteWorkSuccess && job.PostWorkSuccess {
			remoteOK++
			continue
		}

		// remote协议失败， 或协议成功但post检查失败, 均为remote work error
		if (!job.RemoteWorkSuccess && job.RemoteWorkLeaveTime.UnixNano() > 0) ||
			(job.RemoteWorkSuccess && job.PostWorkLeaveTime.UnixNano() > 0 && !job.PostWorkSuccess) {
			remoteError++
		}

		if job.LocalWorkSuccess {
			localOK++
			continue
		}

		if job.LocalWorkLeaveTime.UnixNano() > 0 {
			localError++
		}
	}

	return
}

// GetJobsByIndex get the jobs whose index is larger than the given index
func (was *WorkAnalysisStatus) GetJobsByIndex(index int) []*dcSDK.ControllerJobStats {
	was.mutex.RLock()
	defer was.mutex.RUnlock()

	if index < 0 || index > len(was.jobs) {
		return make([]*dcSDK.ControllerJobStats, 0)
	}

	return was.jobs[index:]
}

// Reset reset stat datas
func (was *WorkAnalysisStatus) Reset() error {
	was.mutex.Lock()
	defer was.mutex.Unlock()

	was.indexMap = make(map[string]int)
	was.jobs = make([]*dcSDK.ControllerJobStats, 0, 1000)

	return nil
}

func now() time.Time {
	return time.Now().Local()
}
