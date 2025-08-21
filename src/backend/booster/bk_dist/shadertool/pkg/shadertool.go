/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package pkg

import (
	"container/list"
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/booster/pkg"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcType "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/types"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/api"
	v1 "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/api/v1"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/shadertool/common"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/http/httpserver"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/util"

	"github.com/google/shlex"
	"github.com/shirou/gopsutil/process"
)

// const vars
const (
	OSWindows                        = "windows"
	CheckShaderJSONFileTickMillisecs = 10
	TryRunJobsTickMillisecs          = 10
	MaxJobExecuteWaitSecs            = 259200 // 3600*24*3
	CheckJobsExecuteTickSecs         = 60
	DefaultMaxParallelJobs           = 240 // ok for most machines
	MaxApplyResourceWaitSecs         = 120 // default stage_timeout is 60
	CheckOutputEnvJSONTickMillisecs  = 100
	CheckApplyCmdTickSecs            = 5
	CheckOldFilesTickSecs            = 600
	CheckCommitSuicideSecs           = 20
	CheckQuitTicksecs                = 1

	DevOPSProcessTreeKillKey = "DEVOPS_DONT_KILL_PROCESS_TREE"

	UbaTemplateToolKey    = "${tool_key}"
	UbaTemplateToolKeyDir = "${tool_key_dir}"
	UbaTemplateHostIP     = "${host_ip}"
)

// vars
var (
	IdleKeepSecs = 100 //

	FailedActionMaxCount = 20
)

// NewShaderTool get a new ShaderTool
func NewShaderTool(flagsparam *common.Flags) *ShaderTool {
	blog.Debugf("ShaderTool: new helptool with flags:%+v", *flagsparam)

	return &ShaderTool{
		flags: flagsparam,
		// controller:     v1.NewSDK(config),
		// controllerconfig: config,
		actionlist:     list.New(),
		finishednumber: 0,
		runningnumber:  0,
		maxjobs:        0,
		actionchan:     nil,
		resourcestatus: common.ResourceInit,
		// removelist:     []string{},
		failedActions: make(map[int][]*common.FailedAction),
		availableTime: make(map[int]time.Time),
	}
}

// ShaderTool describe the shader tool handler
type ShaderTool struct {
	flags *common.Flags
	pCtx  context.Context

	controllerconfig *dcSDK.ControllerConfig
	controller       dcSDK.ControllerSDK
	booster          *pkg.Booster
	executor         *Executor

	projectSettingFile string

	// lock for actions
	actionlock  sync.RWMutex
	actionlist  *list.List
	actionindex uint64
	actionchan  chan common.Actionresult

	failedactionlock sync.RWMutex
	failedActions    map[int][]*common.FailedAction
	availableTime    map[int]time.Time

	lasttoolchain *dcSDK.Toolchain

	// finishednumberlock sync.Mutex
	finishednumber int32
	runningnumber  int32
	maxjobs        int32

	ppid int32

	resourcelock sync.RWMutex
	// applycmd       *exec.Cmd
	resourcestatus common.ResourceStatus
	applying       bool

	lastjobfinishedtime int64

	// removelist []string

	httpserver *httpserver.HTTPServer
	httphandle *HTTPHandle

	// settings
	settings *common.ApplyParameters

	// whether use web socket
	usewebsocket bool
}

// Run run the tool
func (h *ShaderTool) Run(ctx context.Context) (int, error) {
	h.pCtx = ctx
	return h.run(ctx)
}

func (h *ShaderTool) run(pCtx context.Context) (int, error) {
	// update path env
	dcSyscall.AddPath2Env(h.flags.ToolDir)
	err := h.initsettings()
	if err != nil {
		return GetErrorCode(err), err
	}

	blog.Infof("ShaderTool: try to find controller or launch it")
	err = h.launchController()
	if err != nil {
		blog.Errorf("ShaderTool: ensure controller failed: %v", err)
		return GetErrorCode(ErrorFailedLaunchController), ErrorFailedLaunchController
	}
	blog.Infof("ShaderTool: success to connect to controller")

	if h.flags.CommitSuicide {
		go h.ensureCommitSuicide(pCtx)
	}

	go h.executeShaders(pCtx)
	go h.checkQuit(pCtx)

	// main loop
	err = h.server(pCtx)
	if err != nil {
		blog.Errorf("ShaderTool: failed to server with error: %v", err)
	}

	return 0, nil
}

func (h *ShaderTool) getControllerConfig() dcSDK.ControllerConfig {
	if h.controllerconfig != nil {
		return *h.controllerconfig
	}

	h.controllerconfig = &dcSDK.ControllerConfig{
		NoLocal: false,
		Scheme:  common.ControllerScheme,
		IP:      common.ControllerIP,
		Port:    common.ControllerPort,
		Timeout: 5 * time.Second,
		LogDir:  h.flags.LogDir,
		LogVerbosity: func() int {
			// debug模式下, --v=3
			if h.flags.LogLevel == dcUtil.PrintDebug.String() {
				return 3
			}
			return 0
		}(),
		RemainTime:          h.settings.ControllerIdleRunSeconds,
		NoWait:              h.settings.ControllerNoBatchWait,
		SendCork:            h.settings.ControllerSendCork,
		SendFileMemoryLimit: h.settings.ControllerSendFileMemoryLimit,
		NetErrorLimit:       h.settings.ControllerNetErrorLimit,
		RemoteRetryTimes:    h.settings.ControllerRemoteRetryTimes,
		EnableLink:          h.settings.ControllerEnableLink,
		EnableLib:           h.settings.ControllerEnableLib,
		LongTCP:             h.settings.ControllerLongTCP,
		DynamicPort:         h.settings.ControllerDynamicPort,
		WorkerOfferSlot:     h.settings.ControllerWorkerOfferSlot,
		UseLocalCPUPercent:  h.settings.ControllerUseLocalCPUPercent,
		ResultCacheIndexNum: h.settings.ControllerResultCacheIndexNum,
		ResultCacheFileNum:  h.settings.ControllerResultCacheFileNum,
		PreferLocal:         h.settings.ControllerPreferLocal,
	}

	return *h.controllerconfig
}

func (h *ShaderTool) initsettings() error {
	var err error
	h.projectSettingFile, err = h.getProjectSettingFile()
	if err != nil {
		return err
	}

	h.settings, err = resolveApplyJSON(h.projectSettingFile)
	if err != nil {
		return err
	}

	blog.Infof("ShaderTool: loaded setting:%+v", *h.settings)

	for k, v := range h.settings.Env {
		blog.Infof("ShaderTool: set env %s=%s", k, v)
		os.Setenv(k, v)

		if k == "BK_DIST_LOG_LEVEL" {
			common.SetLogLevel(v)
		} else if k == "BK_DIST_USE_WEBSOCKET" {
			h.usewebsocket = true
		}
	}
	os.Setenv(DevOPSProcessTreeKillKey, "true")

	h.controller = v1.NewSDK(h.getControllerConfig())

	return nil
}

func (h *ShaderTool) launchController() error {
	if h.controller == nil {
		h.controller = v1.NewSDK(h.getControllerConfig())
	}

	var err error
	var port int
	for i := 0; i < 4; i++ {
		blog.Infof("ShaderTool: try launch controller for the [%d] times", i)
		_, port, err = h.controller.EnsureServer()
		if err != nil {
			blog.Warnf("ShaderTool: ensure controller failed with error: %v for the [%d] times", err, i)
			continue
		}

		blog.Infof("ShaderTool: success to launch controller port[%d] for the [%d] times", port, i)
		h.controllerconfig.Port = port
		os.Setenv(env.GetEnvKey(env.KeyExecutorControllerPort), strconv.Itoa(port))
		blog.Infof("ShaderTool: set env %s=%d", env.GetEnvKey(env.KeyExecutorControllerPort), port)

		return nil
	}
	return err
}

func (h *ShaderTool) exitWithError(err error) {
	blog.CloseLogs()
	os.Exit(GetErrorCode(err))
}

func (h *ShaderTool) ensureCommitSuicide(ctx context.Context) {
	if !h.flags.CommitSuicide {
		return
	}

	h.ppid = int32(os.Getppid())
	blog.Infof("ShaderTool: ensureCommitSuicide succeed to get father id[%d]", h.ppid)

	h.runCommitSuicideCheck(ctx)
}

// // Clean to clear temp files
// func (h *ShaderTool) Clean() {
// 	blog.Infof("ShaderTool: clean...")

// 	for _, v := range h.removelist {
// 		os.Remove(v)
// 		blog.Infof("ShaderTool: removed temp file %s", v)
// 	}
// }

func (h *ShaderTool) executeShaders(ctx context.Context) error {
	blog.Infof("ShaderTool: execute shaders")

	ticker := time.NewTicker(TryRunJobsTickMillisecs * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			blog.Infof("ShaderTool: load shaders from json file canceled by context")
			return nil

		case <-ticker.C:
			// blog.Debugf("ShaderTool: main loop tick")
			err := h.tryExecuteActions(ctx)
			if err != nil && err != ErrorNoActionsToRun {
				blog.Errorf("ShaderTool: failed to execute shader actions with error:%v,exit now", err)
				h.ReleaseResource()
				h.commitSuicide()
				// return err
			}
		}
	}
}

func (h *ShaderTool) holdingresource() bool {
	return h.resourcestatus == common.ResourceApplySucceed
}

// quit when no task and no resource for some time
func (h *ShaderTool) checkQuit(ctx context.Context) error {
	blog.Infof("ShaderTool: check quit")

	if h.settings.ShaderToolIdleRunSeconds < 0 {
		blog.Infof("ShaderTool: do not check quit for setting is %d, less 0", h.settings.ShaderToolIdleRunSeconds)
		return nil
	}

	ticker := time.NewTicker(CheckQuitTicksecs * time.Second)
	var readyquittime int64
	var startseconds int64 = time.Now().Unix()

	for {
		select {
		case <-ctx.Done():
			blog.Infof("ShaderTool: check quit canceled by context")
			return nil

		case <-ticker.C:
			// for debug
			blog.Debugf("ShaderTool: h.applying:%t, h.hasJobs():%t, h.holdingresource():%t",
				h.applying, h.hasJobs(), h.holdingresource())

			// quit when no task and no resource for some time
			if !h.applying && !h.hasJobs() && !h.holdingresource() {
				nowseconds := time.Now().Unix()
				if nowseconds-startseconds < 30 && h.finishednumber == 0 {
					break
				}

				if readyquittime == 0 {
					readyquittime = nowseconds
				}

				remainfailedactions := false
				if h.failedActions != nil && len(h.failedActions) > 0 {
					h.failedactionlock.Lock()
					for k, v := range h.failedActions {
						if len(v) > 0 {
							blog.Infof("ShaderTool: remain %d failed actions for pid:%d to notify",
								len(v), k)

							if t, ok := h.availableTime[k]; ok {
								blog.Infof("ShaderTool: last available time for pid:%d is %s, now is %s",
									k, t.String(), time.Now().String())
								if time.Now().Sub(t) < time.Duration(h.settings.ShaderToolIdleRunSeconds)*time.Second {
									blog.Infof("ShaderTool: last available time for pid:%d is less than %d seconds,do not exit now",
										k, h.settings.ShaderToolIdleRunSeconds)

									remainfailedactions = true
									break
								} else {
									blog.Infof("ShaderTool: last available time for pid:%d is over than %d seconds, remove failed actions",
										k, h.settings.ShaderToolIdleRunSeconds)
									delete(h.failedActions, k)
								}
							}
						}
					}
					h.failedactionlock.Unlock()
				}
				if remainfailedactions {
					break
				}

				if nowseconds-readyquittime > int64(h.settings.ShaderToolIdleRunSeconds) {
					blog.Infof("ShaderTool: process idle for %d seconds,over than %d, ready exit now",
						nowseconds-readyquittime, h.settings.ShaderToolIdleRunSeconds)
					h.ReleaseResource()
					h.commitSuicide()
				}
			} else {
				readyquittime = 0
			}
		}
	}
}

// execute actions got from ready queue
func (h *ShaderTool) tryExecuteActions(ctx context.Context) error {
	if h.actionlist.Len() <= 0 {
		return nil
	}

	blog.Infof("ShaderTool: try to run actions")

	// try to apply resource and update env
	err := h.applyResource(ctx)
	if err != nil {
		return err
	}

	if h.executor == nil {
		if h.booster != nil {
			env := h.booster.GetWorkersEnv()
			if env != nil {
				for k, v := range env {
					blog.Infof("ShaderTool: set env %s=%s", k, v)
					os.Setenv(k, v)
				}
			}
		}

		if h.controllerconfig.DynamicPort && h.controllerconfig.Port > 0 {
			os.Setenv(env.GetEnvKey(env.KeyExecutorControllerPort), strconv.Itoa(h.controllerconfig.Port))
			blog.Infof("ShaderTool: set env %s=%d]", env.GetEnvKey(env.KeyExecutorControllerPort), h.controllerconfig.Port)
		}

		h.executor = NewExecutor()
		h.executor.usewebsocket = h.usewebsocket
	}

	if !h.executor.Valid() {
		blog.Errorf("ShaderTool: ensure controller failed: %v", ErrorInvalidWorkID)
		return ErrorInvalidWorkID
	}

	h.maxjobs = DefaultMaxParallelJobs
	// get max jobs from env
	maxjobstr := env.GetEnv(env.KeyCommonUE4MaxJobs)
	if maxjobstr != "" {
		maxjobs, err := strconv.Atoi(maxjobstr)
		if err != nil {
			blog.Infof("ShaderTool: failed to get jobs by UE4_MAX_PROCESS with error:%v", err)
		} else {
			h.maxjobs = int32(maxjobs)
		}
	}

	// execute actions no more than max jobs
	blog.Infof("ShaderTool: try to run actions up to %d jobs", h.maxjobs)
	h.actionchan = make(chan common.Actionresult, h.maxjobs)

	// execute first batch actions
	h.selectActionsToExecute()
	if h.runningnumber <= 0 {
		blog.Infof("ShaderTool: no actions to execute")
		return ErrorNoActionsToRun
	}

	for {
		tick := time.NewTicker(CheckJobsExecuteTickSecs * time.Second)
		starttime := time.Now()
		select {
		case r := <-h.actionchan:
			blog.Infof("ShaderTool: got action result:%+v", r)
			// // TODO : return error info to ue editor
			// if r.Exitcode != 0 || r.Err != nil {
			// 	err := fmt.Errorf("exit code:%d,error:%v", r.Exitcode, r.Err)
			// 	blog.Errorf("ShaderTool: %v", err)
			// 	return err
			// }
			if r.Finished {
				h.onActionFinished(r)
			}
			h.selectActionsToExecute()
			if h.runningnumber <= 0 {
				blog.Infof("ShaderTool: no actions to execute")
				return ErrorNoActionsToRun
			}

		case <-tick.C:
			curtime := time.Now()
			blog.Infof("ShaderTool: start time [%s] current time [%s] ", starttime, curtime)
			if curtime.Sub(starttime) > (time.Duration(MaxJobExecuteWaitSecs) * time.Second) {
				blog.Errorf("ShaderTool: faile to execute actions with error:%v", ErrorOverMaxTime)
				return ErrorOverMaxTime
			}
		}
	}
}

func (h *ShaderTool) selectActionsToExecute() error {
	h.actionlock.Lock()
	defer h.actionlock.Unlock()

	blog.Debugf("ShaderTool: remain %d shader jobs before selectActionsToExecute", h.actionlist.Len())
	for e := h.actionlist.Front(); e != nil; e = e.Next() {
		// can modify element here ???
		a := e.Value.(*common.Action)
		// blog.Infof("ShaderTool: check action %+v", a)
		if !a.Running {
			a.Running = true
			h.runningnumber++
			go h.executeOneAction(a, h.actionchan)
		}
		if h.runningnumber > h.maxjobs {
			break
		}
	}
	blog.Debugf("ShaderTool: remain total %d shader jobs,%d running jobs after selectActionsToExecute",
		h.actionlist.Len(), h.runningnumber)

	return nil
}

func (h *ShaderTool) executeOneAction(action *common.Action, actionchan chan common.Actionresult) error {
	blog.Infof("ShaderTool: ready execute actions:%+v", *action)

	fullargs := []string{action.Cmd}
	args, _ := shlex.Split(replaceWithNextExclude(action.Arg, '\\', "\\\\", []byte{'"'}))
	fullargs = append(fullargs, args...)

	// try again if failed after sleep some time
	var retcode int
	retmsg := ""
	waitsecs := 5
	var err error
	var localr *dcSDK.LocalTaskResult
	for try := 0; try < 3; try++ {
		retcode, retmsg, localr, err = h.executor.Run(fullargs, "", action.Attributes)
		if retcode != int(api.ServerErrOK) {
			blog.Warnf("ShaderTool: failed to execute action with ret code:%d error [%+v] for %d times, actions:%+v", retcode, err, try+1, action)

			if retcode == int(api.ServerErrWorkNotFound) {
				h.dealWorkNotFound(retcode, retmsg)
				continue
			} else {
				break
			}

			// time.Sleep(time.Duration(waitsecs) * time.Second)
			// waitsecs = waitsecs * 2
			// continue
		}

		if err != nil {
			blog.Warnf("ShaderTool: failed to execute action with error [%+v] for %d times, actions:%+v", err, try+1, *action)
			time.Sleep(time.Duration(waitsecs) * time.Second)
			waitsecs = waitsecs * 2
			continue
		}
		break
	}

	if localr != nil {
		r := common.Actionresult{
			Index:     action.Index,
			Finished:  true,
			Succeed:   err == nil,
			Outputmsg: string(localr.Stdout),
			Errormsg:  string(localr.Stderr),
			Exitcode:  retcode,
			Err:       err,
		}

		actionchan <- r
		return nil
	}

	r := common.Actionresult{
		Index:     action.Index,
		Finished:  true,
		Succeed:   err == nil,
		Outputmsg: "",
		Errormsg:  "",
		Exitcode:  retcode,
		Err:       err,
	}

	actionchan <- r

	return nil
}

// update all actions and ready actions
func (h *ShaderTool) onActionFinished(r common.Actionresult) error {
	blog.Infof("ShaderTool: action %v finished", r)

	h.finishednumber++
	h.runningnumber--
	h.lastjobfinishedtime = time.Now().Unix()
	blog.Infof("ShaderTool: finished : %d running : %d", h.finishednumber, h.runningnumber)

	var failedAction *common.Action = nil

	// update with index
	h.actionlock.Lock()
	index := r.Index
	inlist := false
	// delete from ready array
	for e := h.actionlist.Front(); e != nil; e = e.Next() {
		a := e.Value.(*common.Action)
		if a.Index == index {
			inlist = true
			if r.Exitcode != 0 || r.Err != nil {
				// h.onActionFailed(a, r)
				failedAction = a
			}
			h.actionlist.Remove(e)
			blog.Infof("ShaderTool: remove action %v from list", a)
			break
		}
	}
	h.actionlock.Unlock()

	if !inlist {
		blog.Errorf("ShaderTool: action %v not in list", r)
	}

	if failedAction != nil {
		h.onActionFailed(failedAction, r)
	}

	return nil
}

func (h *ShaderTool) onActionFailed(a *common.Action, r common.Actionresult) error {
	blog.Infof("ShaderTool: action %v failed", a)
	processid := 0
	args, _ := shlex.Split(replaceWithNextExclude(a.Arg, '\\', "\\\\", []byte{'"'}))
	if len(args) > 3 {
		arg1, err1 := strconv.Atoi(args[1])
		arg2, err2 := strconv.Atoi(args[2])
		if err1 == nil && err2 == nil && arg2 == 0 {
			processid = arg1
			blog.Infof("ShaderTool: got failed process id %d for action %v", processid, a)
		}
		inputfile := filepath.Join(args[0], args[3])
		inputfileinfo := dcFile.Stat(inputfile)
		if inputfileinfo.Exist() {
			inputfilesize := inputfileinfo.Size()
			failedaction := &common.FailedAction{
				Index: a.Index,
				// Cmd:           a.Cmd,
				// Arg:           a.Arg,
				Errormsg:      r.Errormsg,
				Exitcode:      r.Exitcode,
				ProcessID:     processid,
				InputFile:     inputfile,
				InputFileSize: inputfilesize,
			}

			h.failedactionlock.Lock()
			defer h.failedactionlock.Unlock()

			if _, exists := h.failedActions[processid]; !exists {
				h.failedActions[processid] = []*common.FailedAction{failedaction} // 显式初始化空切片
			} else {
				h.failedActions[processid] = append(h.failedActions[processid], failedaction)
			}
			blog.Infof("ShaderTool: got total %d failed after add action %v",
				len(h.failedActions[processid]), failedaction)
		}
	}
	return nil
}

func (h *ShaderTool) recordAvailableTime(processid int) {
	h.availableTime[processid] = time.Now()
}

func (h *ShaderTool) getAndRemoveFailedActions(processid int) ([]common.FailedAction, error) {
	blog.Infof("ShaderTool: getAndRemoveFailedActions for processid %d", processid)
	h.failedactionlock.Lock()
	defer h.failedactionlock.Unlock()

	h.recordAvailableTime(processid)

	// limit failed actions to FailedActionMaxCount
	if _, exists := h.failedActions[processid]; exists {
		var failedactions []common.FailedAction
		if len(h.failedActions[processid]) < FailedActionMaxCount {
			for _, v := range h.failedActions[processid] {
				failedactions = append(failedactions, *v)
			}
			delete(h.failedActions, processid)
			return failedactions, nil
		} else {
			for _, v := range h.failedActions[processid][:FailedActionMaxCount] {
				failedactions = append(failedactions, *v)
			}
			h.failedActions[processid] = h.failedActions[processid][FailedActionMaxCount:]
			return failedactions, nil
		}
	}

	return nil, nil
}

func (h *ShaderTool) runCommitSuicideCheck(ctx context.Context) {
	ticker := time.NewTicker(CheckCommitSuicideSecs * time.Second)

	for {
		select {
		case <-ctx.Done():
			blog.Infof("ShaderTool: run commit suicide check canceled by context")
			return

		case <-ticker.C:
			blog.Debugf("ShaderTool: run commit suicide check with parent process %d", h.ppid)
			if ok, err := process.PidExists(h.ppid); err == nil && !ok {

				// // clean before kill
				// h.Clean()
				h.ReleaseResource()

				// p, err := process.NewProcess(int32(os.Getpid()))
				// if err == nil {
				// 	blog.Infof("ShaderTool: ready commit suicide for parent process %d not existed", h.ppid)
				// 	_ = p.Kill()
				// }
				blog.Infof("booster: commit suicide for parent process %d not existed", h.ppid)
				h.commitSuicide()
			}
		}
	}
}

func (h *ShaderTool) commitSuicide() {
	blog.CloseLogs()
	// p, err := process.NewProcess(int32(os.Getpid()))
	// if err == nil {
	// 	blog.Infof("ShaderTool: commit suicide ")
	// 	_ = p.Kill()
	// }

	os.Exit(0)
}

func (h *ShaderTool) hasJobs() bool {
	return !(h.actionlist.Len() == 0 && h.runningnumber == 0)
}

// release resource after some idle duration
func (h *ShaderTool) watchResource() error {
	blog.Infof("ShaderTool: watch resource in...")

	keepseconds := IdleKeepSecs
	// get idle keep seconds from env
	str := env.GetEnv(env.KeyExecutorIdleKeepSecs)
	if str != "" {
		envkeepsecond, err := strconv.Atoi(str)
		if err != nil {
			blog.Infof("ShaderTool: failed to get keep resource idle for seconds with error:%v", err)
		} else {
			keepseconds = envkeepsecond
			blog.Infof("ShaderTool: succeed to get keep resource idle for seconds %d", keepseconds)
		}
	}

	ticker := time.NewTicker(CheckApplyCmdTickSecs * time.Second)
	for {
		select {
		case <-ticker.C:
			if h.lastjobfinishedtime > 0 && !h.hasJobs() {
				nowsecs := time.Now().Unix()
				if nowsecs-h.lastjobfinishedtime > int64(keepseconds) {
					blog.Infof("ShaderTool: resource idle for %d seconds, ready to release now", nowsecs-h.lastjobfinishedtime)
					// release resource after some idle duration
					h.resourcelock.Lock()
					defer h.resourcelock.Unlock()
					h.ReleaseResource()
					return nil
				}
			}
		}
	}
}

// ReleaseResource release resource after some idle duration
func (h *ShaderTool) ReleaseResource() {
	blog.Infof("ShaderTool: release resource in...")
	if h.booster != nil {
		err := h.booster.UnregisterWork()
		if err != nil {
			blog.Infof("ShaderTool: failed to release resource with error:%v", err)
		}
	}
	h.resourcestatus = common.ResourceInit
	h.booster = nil
	h.executor = nil

	// close long tcp connection if need
	if h.usewebsocket {
		closeConnections()
	}
}

func (h *ShaderTool) getProjectSettingFile() (string, error) {
	jsonfile := filepath.Join(h.flags.ToolDir, "bk_project_setting.json")
	if dcFile.Stat(jsonfile).Exist() {
		return jsonfile, nil
	}

	exepath := dcUtil.GetExcPath()
	if exepath != "" {
		jsonfile = filepath.Join(exepath, "bk_project_setting.json")
		if dcFile.Stat(jsonfile).Exist() {
			return jsonfile, nil
		}
	}

	return "", ErrorProjectSettingNotExisted
}

func (h *ShaderTool) newBooster() (*pkg.Booster, error) {
	blog.Infof("ShaderTool: new booster in...")

	// get current running dir.
	// if failed, it doesn't matter, just give a warning.
	runDir, err := os.Getwd()
	if err != nil {
		blog.Warnf("booster-command: get working dir failed: %v", err)
	}

	// get current user, use the login username.
	// if failed, it doesn't matter, just give a warning.
	usr, err := user.Current()
	if err != nil {
		blog.Warnf("booster-command: get current user failed: %v", err)
	}

	// decide which server to connect to.
	ServerHost := h.settings.ServerHost
	projectID := h.settings.ProjectID
	buildID := h.settings.BuildID

	// generate a new booster.
	cmdConfig := dcType.BoosterConfig{
		Type:      dcType.BoosterType(h.settings.Scene),
		ProjectID: projectID,
		BuildID:   buildID,
		BatchMode: h.settings.BatchMode,
		Args:      "",
		Cmd:       strings.Join(os.Args, " "),
		Works: dcType.BoosterWorks{
			Stdout:            os.Stdout,
			Stderr:            os.Stderr,
			RunDir:            runDir,
			User:              usr.Username,
			WorkerList:        h.settings.WorkerList,
			LimitPerWorker:    h.settings.LimitPerWorker,
			MaxLocalTotalJobs: defaultCPULimit(h.settings.MaxLocalTotalJobs),
			MaxLocalPreJobs:   h.settings.MaxLocalPreJobs,
			MaxLocalExeJobs:   h.settings.MaxLocalExeJobs,
			MaxLocalPostJobs:  h.settings.MaxLocalPostJobs,
			ResultCacheList:   h.settings.ResultCacheList,
		},

		Transport: dcType.BoosterTransport{
			ServerHost:             ServerHost,
			Timeout:                5 * time.Second,
			HeartBeatTick:          5 * time.Second,
			InspectTaskTick:        1 * time.Second,
			TaskPreparingTimeout:   60 * time.Second,
			PrintTaskInfoEveryTime: 5,
			CommitSuicideCheckTick: 5 * time.Second,
		},

		Controller: sdk.ControllerConfig{
			NoLocal:     false,
			Scheme:      common.ControllerScheme,
			IP:          common.ControllerIP,
			Port:        common.ControllerPort,
			Timeout:     5 * time.Second,
			DynamicPort: h.settings.ControllerDynamicPort,
		},
	}

	return pkg.NewBooster(cmdConfig)
}

// apply resource
func (h *ShaderTool) applyResource(pCtx context.Context) error {
	blog.Infof("ShaderTool: apply and start resource in...")

	// ensure only one do this
	h.resourcelock.Lock()
	defer h.resourcelock.Unlock()

	h.applying = true
	defer func() {
		h.applying = false
	}()

	// ? should we apply again if failed ?
	if h.resourcestatus == common.ResourceApplySucceed || h.resourcestatus == common.ResourceApplyFailed {
		blog.Infof("ShaderTool: resource applied, do nothing now")
		return nil
	}

	if h.controller == nil {
		return ErrorContorllerNil
	}

	var err error
	if h.booster == nil {
		h.booster, err = h.newBooster()
		if err != nil || h.booster == nil {
			blog.Errorf("ShaderTool: failed to new booster: %v", err)
			return err
		}
	}

	err = h.launchController()
	if err != nil {
		blog.Errorf("ShaderTool: ensure controller failed: %v", err)
		return err
	}

	if h.resourcestatus == common.ResourceInit {
		h.resourcestatus = common.ResourceApplying
	}

	err = h.booster.RegisterWork()
	if err != nil {
		h.resourcestatus = common.ResourceApplyFailed
		return err
	}

	event, err := h.booster.Wait4TaskReady(pCtx)
	if err != nil {
		blog.Errorf("booster: run booster wait for task ready failed: %v", err)
		h.resourcestatus = common.ResourceApplyFailed
		return err
	}

	for {
		select {
		case <-pCtx.Done():
			blog.Warnf("booster: run booster for task canceled by context")
			h.resourcestatus = common.ResourceApplyFailed
			return ErrContextCanceled

		case e := <-event:
			if err = h.booster.ParseEvent(e); err != nil {
				h.resourcestatus = common.ResourceApplyFailed
			} else {
				h.lastjobfinishedtime = time.Now().Unix()
				h.resourcestatus = common.ResourceApplySucceed
			}

			// watch resource anyway
			go h.watchResource()

			err = h.booster.SetSettings()
			if err != nil {
				h.ReleaseResource()
				return err
			}
			err = h.booster.StartControllerWork()
			if err != nil {
				h.ReleaseResource()
				return err
			}

			return nil
		}
	}
}

// 过滤ip，客户端判断不保险，最好是能够从server拿到ip
func isIPValid(ip string) bool {
	if strings.HasPrefix(ip, "192.") ||
		strings.HasPrefix(ip, "172.") ||
		strings.HasPrefix(ip, "127.") {
		return false
	}

	return true
}

func (h *ShaderTool) adjustActions4UBAAgent(all *common.UE4Action, t *dcSDK.Toolchain) {
	if len(t.Toolchains) == 0 {
		blog.Errorf("tool chain is empty")
		return
	}

	if len(all.Actions) > 0 && all.Actions[0].Adjust {
		for i := range all.Actions {
			if all.Actions[i].Cmd == UbaTemplateToolKey {
				all.Actions[i].Cmd = t.Toolchains[0].ToolKey
			}

			if all.Actions[i].Workdir == UbaTemplateToolKeyDir {
				all.Actions[i].Workdir = filepath.Dir(t.Toolchains[0].ToolKey)
			}

			ips := util.GetIPAddress()
			if len(ips) > 0 {
				ip := ips[0]
				for _, ipstr := range ips {
					if isIPValid(ipstr) {
						ip = ipstr
						break
					}
				}
				if strings.ContainsAny(all.Actions[i].Arg, UbaTemplateHostIP) {
					all.Actions[i].Arg = strings.Replace(all.Actions[i].Arg, UbaTemplateHostIP, ip, -1)
				}
			}
		}

		blog.Infof("after adjust,actions: %v", *all)
		return
	}

	blog.Infof("do not need adjust action")
}

// apply resource
func (h *ShaderTool) shaders(actions *common.UE4Action, updatetoolchain string) error {
	blog.Infof("ShaderTool: shaders with updatetoolchain[%s]", updatetoolchain)

	// apply resource before set tool chain, otherwize we will get degraded for set tool chain script
	err := h.applyResource(h.pCtx)
	if err != nil {
		// return err
		blog.Errorf("ShaderTool: failed to apply resource with error:%v", err)
	}

	if updatetoolchain == "" ||
		updatetoolchain == "1" ||
		h.lasttoolchain == nil {
		err = h.booster.SetToolChainWithJSON(&actions.ToolJSON)
		if err != nil {
			blog.Errorf("ShaderTool: failed to set tool chain with error:%v", err)
			return err
		}

		h.lasttoolchain = &actions.ToolJSON
	} else {
		blog.Infof("ShaderTool: not update tool chain with update flag[%s]", updatetoolchain)
	}

	h.actionlock.Lock()
	defer h.actionlock.Unlock()

	h.adjustActions4UBAAgent(actions, &actions.ToolJSON)

	for _, a := range actions.Actions {
		temp := a
		temp.Running = false
		temp.Index = h.actionindex
		h.actionindex++
		h.actionlist.PushBack(&temp)
	}
	blog.Infof("ShaderTool: got %d shader jobs, total jobs in queue: %d", len(actions.Actions), h.actionlist.Len())

	// awake action execution
	if h.actionchan != nil {
		r := common.Actionresult{
			Index:     0,
			Finished:  false,
			Succeed:   err == nil,
			Outputmsg: "",
			Errormsg:  "",
		}

		h.actionchan <- r
	}

	return nil
}

func (h *ShaderTool) setToolChain() error {
	blog.Infof("ShaderTool: set toolchain in...")

	var err error
	if h.booster == nil {
		h.booster, err = h.newBooster()
		if err != nil || h.booster == nil {
			blog.Errorf("ShaderTool: failed to new booster: %v", err)
			return err
		}
	}

	err = h.booster.SetToolChainWithJSON(h.lasttoolchain)
	if err != nil {
		blog.Errorf("ShaderTool: failed to set tool chain, error: %v", err)
	}

	return err
}

func (h *ShaderTool) dealWorkNotFound(retcode int, retmsg string) error {
	blog.Infof("ShaderTool: deal for work not found with code:%d, msg:%s", retcode, retmsg)

	if retmsg == "" {
		return nil
	}

	msgs := strings.Split(retmsg, "|")
	if len(msgs) < 2 {
		return nil
	}

	var workerid v1.WorkerChanged
	if err := codec.DecJSON([]byte(msgs[1]), &workerid); err != nil {
		blog.Errorf("UBTTool: decode param %s failed: %v", msgs[1], err)
		return err
	}

	if workerid.NewWorkID != "" {
		// update local workid with new workid
		env.SetEnv(env.KeyExecutorControllerWorkID, workerid.NewWorkID)
		if h.executor != nil {
			h.executor.Update()
		}

		// set tool chain with new workid
		h.setToolChain()
	}

	return nil
}
