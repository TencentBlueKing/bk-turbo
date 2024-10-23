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
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/websocket"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
	v2 "github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/pkg/api/v2"
	"github.com/emicklei/go-restful"
)

// WorkRegisterConfig describe the config of registering work
type WorkRegisterConfig struct {
	BatchMode        bool           `json:"batch_mode"`
	ServerHost       string         `json:"server_host"`
	SpecificHostList []string       `json:"specific_host_list"`
	NeedApply        bool           `json:"need_apply"`
	Apply            *v2.ParamApply `json:"apply"`
}

// WorkUnregisterConfig describe the config of unregistering work
type WorkUnregisterConfig struct {
	Force         bool             `json:"force"`
	TimeoutBefore time.Duration    `json:"timeout_before"`
	Release       *v2.ParamRelease `json:"release"`
}

// WorkSettings describe the work settings
type WorkSettings struct {
	TaskID          string
	ProjectID       string
	Scene           string
	UsageLimit      map[dcSDK.JobUsage]int
	LocalTotalLimit int
	Preload         *dcSDK.PreloadConfig
	FilterRules     dcSDK.FilterRules
	Degraded        bool
	GlobalSlots     bool
}

// Dump encode work settings to json bytes
func (ws *WorkSettings) Dump() []byte {
	var data []byte
	_ = codec.EncJSON(*ws, &data)
	return data
}

// WorkerKeyConfig describe the work unique key
type WorkerKeyConfig struct {
	BatchMode bool
	ProjectID string
	Scene     string
}

// Equal check if the two key are point to one work
func (wkc *WorkerKeyConfig) Equal(other *WorkerKeyConfig) bool {
	if other != nil {
		return wkc.BatchMode == other.BatchMode && wkc.ProjectID == other.ProjectID && wkc.Scene == other.Scene
	}

	return false
}

// CommonConfig common work config
type CommonConfig struct {
	Configkey dcSDK.CommonConfigKey
	WorkerKey WorkerKeyConfig
	Data      []byte
	Config    interface{}
}

// Equal check if the two key are point to one work config
func (ccf *CommonConfig) KeyEqual(other *CommonConfig) bool {
	if other != nil {
		return (&ccf.WorkerKey).Equal(&other.WorkerKey) && ccf.Configkey == other.Configkey
	}

	return false
}

// ToolChain describe the toolchain info
type ToolChain struct {
	ToolKey                string
	ToolName               string
	ToolLocalFullPath      string
	ToolRemoteRelativePath string
	Files                  []dcSDK.ToolFile
	Timestamp              int64
}

// WorkStats describe the work stats
type WorkStats struct {
	Success bool `json:"success"`
}

// JobStats describe the single job stats
type JobStats struct {
	Pid         int    `json:"pid"`
	WorkID      string `json:"work_id"`
	TaskID      string `json:"task_id"`
	BoosterType string `json:"booster_type"`

	Success           bool `json:"success"`
	PreWorkSuccess    bool `json:"pre_work_success"`
	RemoteWorkSuccess bool `json:"remote_work_success"`
	PostWorkSuccess   bool `json:"post_work_success"`
	FinalWorkSuccess  bool `json:"final_work_success"`
	LocalWorkSuccess  bool `json:"local_work_success"`

	OriginArgs []string `json:"origin_args"`

	EnterTime int64 `json:"enter_time"`
	LeaveTime int64 `json:"leave_time"`

	PreWorkEnterTime  int64 `json:"pre_work_enter_time"`
	PreWorkLeaveTime  int64 `json:"pre_work_leave_time"`
	PreWorkLockTime   int64 `json:"pre_work_lock_time"`
	PreWorkUnlockTime int64 `json:"pre_work_unlock_time"`
	PreWorkStartTime  int64 `json:"pre_work_start_time"`
	PreWorkEndTime    int64 `json:"pre_work_end_time"`

	PostWorkEnterTime  int64 `json:"post_work_enter_time"`
	PostWorkLeaveTime  int64 `json:"post_work_leave_time"`
	PostWorkLockTime   int64 `json:"post_work_lock_time"`
	PostWorkUnlockTime int64 `json:"post_work_unlock_time"`
	PostWorkStartTime  int64 `json:"post_work_start_time"`
	PostWorkEndTime    int64 `json:"post_work_end_time"`

	FinalWorkStartTime int64 `json:"final_work_start_time"`
	FinalWorkEndTime   int64 `json:"final_work_end_time"`

	RemoteWorkEnterTime           int64 `json:"remote_work_enter_time"`
	RemoteWorkLeaveTime           int64 `json:"remote_work_leave_time"`
	RemoteWorkLockTime            int64 `json:"remote_work_lock_time"`
	RemoteWorkUnlockTime          int64 `json:"remote_work_unlock_time"`
	RemoteWorkStartTime           int64 `json:"remote_work_start_time"`
	RemoteWorkEndTime             int64 `json:"remote_work_end_time"`
	RemoteWorkPackStartTime       int64 `json:"remote_work_pack_start_time"`
	RemoteWorkPackEndTime         int64 `json:"remote_work_pack_end_time"`
	RemoteWorkSendStartTime       int64 `json:"remote_work_send_start_time"`
	RemoteWorkSendEndTime         int64 `json:"remote_work_send_end_time"`
	RemoteWorkPackCommonStartTime int64 `json:"remote_work_pack_common_start_time"`
	RemoteWorkPackCommonEndTime   int64 `json:"remote_work_pack_common_end_time"`
	RemoteWorkSendCommonStartTime int64 `json:"remote_work_send_common_start_time"`
	RemoteWorkSendCommonEndTime   int64 `json:"remote_work_send_common_end_time"`
	RemoteWorkProcessStartTime    int64 `json:"remote_work_process_start_time"`
	RemoteWorkProcessEndTime      int64 `json:"remote_work_process_end_time"`
	RemoteWorkReceiveStartTime    int64 `json:"remote_work_receive_start_time"`
	RemoteWorkReceiveEndTime      int64 `json:"remote_work_receive_end_time"`
	RemoteWorkUnpackStartTime     int64 `json:"remote_work_unpack_start_time"`
	RemoteWorkUnpackEndTime       int64 `json:"remote_work_unpack_end_time"`

	LocalWorkEnterTime  int64 `json:"local_work_enter_time"`
	LocalWorkLeaveTime  int64 `json:"local_work_leave_time"`
	LocalWorkLockTime   int64 `json:"local_work_lock_time"`
	LocalWorkUnlockTime int64 `json:"local_work_unlock_time"`
	LocalWorkStartTime  int64 `json:"local_work_start_time"`
	LocalWorkEndTime    int64 `json:"local_work_end_time"`
}

// LocalSlotsOccupyConfig describe the local slot lock config
type LocalSlotsOccupyConfig struct {
	Usage  dcSDK.JobUsage
	Weight int32 `json:"weight"`
}

// LocalSlotsFreeConfig describe the local slot unlock config
type LocalSlotsFreeConfig struct {
	Usage  dcSDK.JobUsage
	Weight int32 `json:"weight"`
}

// RemoteTaskExecuteRequest describe the remote task execution param
type RemoteTaskExecuteRequest struct {
	Pid           int
	Server        *dcProtocol.Host
	Req           *dcSDK.BKDistCommand
	Stats         *dcSDK.ControllerJobStats
	Sandbox       *dcSyscall.Sandbox
	IOTimeout     int
	BanWorkerList []*dcProtocol.Host

	HttpConnKey   string
	HttpConnCache *HttpConnCache
}

// RemoteTaskExecuteResult describe the remote task execution result
type RemoteTaskExecuteResult struct {
	Result *dcSDK.BKDistResult
}

// RemoteTaskSendFileRequest describe the file sending param
type RemoteTaskSendFileRequest struct {
	Pid     int
	Server  *dcProtocol.Host
	Req     []dcSDK.FileDesc
	Sandbox *dcSyscall.Sandbox
	Stats   *dcSDK.ControllerJobStats
}

// RemoteTaskSendFileResult describe the file sending result
type RemoteTaskSendFileResult struct {
	Result *dcSDK.BKSendFileResult
}

// ++++++++++++++++ TODO : 增加http连接缓存
func getHttpNetKey(req *restful.Request) string {
	return fmt.Sprintf("%s_%d", req.Request.RemoteAddr, time.Now().Nanosecond())
}

type connStatus int

const (
	Connected connStatus = iota
	Disconnected
	Unknown = 99
)

type httpConn struct {
	status         connStatus
	delayCleanSecs int
	ErrorStartTime time.Time
}

type HttpConnCache struct {
	cache map[string]*httpConn
	mutex sync.RWMutex
}

func NewHttpConnCache() *HttpConnCache {
	return &HttpConnCache{
		cache: make(map[string]*httpConn),
	}
}

func (c *HttpConnCache) InsertOnce(key string, delayCleanSecs int) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	_, ok := c.cache[key]
	if !ok {
		c.cache[key] = &httpConn{status: Connected, delayCleanSecs: delayCleanSecs}
		return true
	} else {
		return false
	}
}

func (c *HttpConnCache) IsConnStatusOk(key string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	v, ok := c.cache[key]
	if !ok {
		return false
	}

	return v.status == Connected
}

func (c *HttpConnCache) OnConnStatusError(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	v, ok := c.cache[key]
	if ok {
		if v.delayCleanSecs == 0 {
			delete(c.cache, key)
		} else {
			v.status = Disconnected
			v.ErrorStartTime = time.Now()
		}
	}
}

func (c *HttpConnCache) Check() {
	// blog.Infof("types: httpconncache: start go routine to check http conn status")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mutex.Lock()
			for k, v := range c.cache {
				if v.status != Connected {
					old := v.ErrorStartTime.Add(time.Duration(v.delayCleanSecs) * time.Second)
					if old.Before(time.Now()) {
						delete(c.cache, k)
						// blog.Infof("types: httpconncache: deleted %s from cache after wait", k)
					}
				}
			}
			c.mutex.Unlock()
		}
	}
}

// ----------------

// LocalTaskExecuteRequest describe the local task execution param
type LocalTaskExecuteRequest struct {
	Pid          int
	Dir          string
	User         user.User
	Commands     []string
	Environments []string
	Stats        *dcSDK.ControllerJobStats

	// 该请求的执行是否依赖http连接状态，默认依赖，即ignoreHttpStatus为false;
	// 如果依赖连接状态，当连接断开时，该请求直接返回失败
	ignoreHttpStatus  bool
	checkedHttpStatus bool
	HttpConnKey       string
	HttpConnCache     *HttpConnCache
}

// 这个正常只有一个协程调用，无需加锁
func (r *LocalTaskExecuteRequest) InitHttpConnStatus(req *restful.Request,
	cache *HttpConnCache,
	s *websocket.Session,
	delaysecs int) error {
	shouldcheck := r.shouldCheckHttpStatus()
	if shouldcheck && req != nil && cache != nil {
		key := ""
		if s == nil {
			key = getHttpNetKey(req)
		} else {
			key = s.GetConnKey()
		}

		// TODO : 检查该连接的检查协程是否已启动
		firstInsert := cache.InsertOnce(key, delaysecs)
		if firstInsert {
			blog.Infof("types: httpconncache: inserted %s to cache", key)

			// 确保协程启动
			var wg = sync.WaitGroup{}
			wg.Add(1)
			if s == nil {
				go func(wg *sync.WaitGroup) {
					wg.Done()
					ctx := req.Request.Context()
					select {
					case <-ctx.Done():
						err := ctx.Err()
						cache.OnConnStatusError(key)
						blog.Infof("types: httpconncache: found %s disconnected with error:%v", key, err)
					}
				}(&wg)
			} else {
				go func(wg *sync.WaitGroup) {
					wg.Done()
					ticker := time.NewTicker(3 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-ticker.C:
							if !s.IsValid() {
								cache.OnConnStatusError(key)
								blog.Infof("types: httpconncache: found websocket %s disconnected", key)
								return
							}
						}
					}
				}(&wg)
			}
			wg.Wait()
		}

		r.HttpConnKey = key
		r.HttpConnCache = cache
		return nil
	} else {
		r.HttpConnKey = ""
		r.HttpConnCache = nil
		return nil
	}
}

func IsHttpConnStatusOk(cache *HttpConnCache, key string) bool {
	if cache == nil || key == "" {
		return true
	}

	return cache.IsConnStatusOk(key)
}

func (r *LocalTaskExecuteRequest) shouldCheckHttpStatus() bool {
	if r.checkedHttpStatus {
		return !r.ignoreHttpStatus
	}

	// check env.KeyExecutorIgnoreHttpStatus
	for _, e := range r.Environments {
		if strings.HasPrefix(e, env.GetEnvKey(env.KeyExecutorIgnoreHttpStatus)) {
			for i := 0; i < len(e); i++ {
				if e[i] == '=' {
					b, err1 := strconv.ParseBool(e[i+1:])
					if err1 == nil {
						r.ignoreHttpStatus = b
						r.checkedHttpStatus = true
						return !r.ignoreHttpStatus
					} else {
						r.ignoreHttpStatus = false
						r.checkedHttpStatus = true
						return !r.ignoreHttpStatus
					}
				}
			}
			break
		}
	}

	r.ignoreHttpStatus = false
	r.checkedHttpStatus = true

	return !r.ignoreHttpStatus
}

// LocalTaskExecuteResult describe the local task execution result
type LocalTaskExecuteResult struct {
	Result *dcSDK.LocalTaskResult
}

// FileSendStatus save file send status
type FileSendStatus int

// define file send status
const (
	FileSendInit FileSendStatus = iota
	FileSending
	FileSendSucceed
	FileSendFailed
	FileSendUnknown = 99
)

var (
	fileStatusMap = map[FileSendStatus]string{
		FileSendInit:    "sendinit",
		FileSending:     "sending",
		FileSendSucceed: "sendsucceed",
		FileSendFailed:  "sendfailed",
		FileSendUnknown: "unknown",
	}
)

// String return the string of FileSendStatus
func (f FileSendStatus) String() string {
	if v, ok := fileStatusMap[f]; ok {
		return v
	}

	return "unknown"
}

func (f FileSendStatus) IsFinished() bool {
	return f == FileSendSucceed || f == FileSendFailed
}

// FileCollectionInfo save file collection send status
type FileCollectionInfo struct {
	UniqID     string           `json:"uniq_id"`
	SendStatus FileSendStatus   `json:"send_status"`
	Files      []dcSDK.FileDesc `json:"files"`
	Timestamp  int64            `json:"timestamp"`
}

// FileInfo record file info
type FileInfo struct {
	FullPath           string         `json:"full_path"`
	Size               int64          `json:"size"`
	LastModifyTime     int64          `json:"last_modify_time"`
	Md5                string         `json:"md5"`
	TargetRelativePath string         `json:"target_relative_path"`
	FileMode           uint32         `json:"file_mode"`
	LinkTarget         string         `json:"link_target"`
	SendStatus         FileSendStatus `json:"send_status"`
	FailCount          int            `json:"fail_count"`
}

// Match check if the FileDesc is point to some file as this FileInfo
func (f *FileInfo) Match(other dcSDK.FileDesc) bool {
	if os.FileMode(f.FileMode)&os.ModeSymlink != 0 && os.FileMode(other.Filemode)&os.ModeSymlink != 0 {
		return f.LinkTarget == other.LinkTarget
	}

	if os.FileMode(f.FileMode).IsDir() && os.FileMode(other.Filemode).IsDir() {
		return f.FullPath == other.FilePath && f.TargetRelativePath == other.Targetrelativepath
	}

	return f.FullPath == other.FilePath &&
		f.TargetRelativePath == other.Targetrelativepath &&
		f.Size == other.FileSize &&
		f.LastModifyTime == other.Lastmodifytime
}

func (f *FileInfo) copy() *FileInfo {
	return &FileInfo{
		FullPath:       f.FullPath,
		Size:           f.Size,
		LastModifyTime: f.LastModifyTime,
		Md5:            f.Md5,
		SendStatus:     f.SendStatus,
	}
}

// FilesByServer record files info to send for one server
type FilesByServer struct {
	Server string
	Files  []*FileInfo
}

func (f *FilesByServer) copy() *FilesByServer {
	newf := FilesByServer{
		Server: f.Server,
	}

	for _, v := range f.Files {
		newf.Files = append(newf.Files, v.copy())
	}

	return &newf
}

// FilesDetails describe the files details and the target server to send to.
type FilesDetails struct {
	Servers []*dcProtocol.Host
	File    dcSDK.FileDesc
}
