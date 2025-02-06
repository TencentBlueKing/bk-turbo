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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
)

// define const vars
const (
	ControllerScheme = "http"
	ControllerIP     = "127.0.0.1"
	ControllerPort   = 30117
)

// AvailableResp describe the response of available api
type AvailableResp struct {
	PID int32 `json:"pid"`
}

// Flags define flags needed by shader tool
type Flags struct {
	ToolDir         string
	JobDir          string
	JobJSONPrefix   string
	JobStartIndex   int32
	CommitSuicide   bool
	Port            int32
	LogLevel        string
	LogDir          string
	ProcessInfoFile string
}

// Action define shader action
type Action struct {
	Index    uint64 `json:"index"`
	Cmd      string `json:"cmd"`
	Arg      string `json:"arg"`
	Running  bool   `json:"running"`
	Finished bool   `json:"finished"`
}

// UE4Action define ue4 action
type UE4Action struct {
	ToolJSONFile string          `json:"tool_json_file"`
	ToolJSON     dcSDK.Toolchain `json:"tool_json"`
	Actions      []Action        `json:"shaders"`
}

// ApplyParameters define parameters to apply resource
type ApplyParameters struct {
	ProjectID                     string            `json:"project_id"`
	Scene                         string            `json:"scene"`
	ServerHost                    string            `json:"server_host"`
	ResultCacheList               []string          `json:"result_cache_list"`
	BatchMode                     bool              `json:"batch_mode"`
	WorkerList                    []string          `json:"specific_host_list"`
	NeedApply                     bool              `json:"need_apply"`
	ControllerDynamicPort         bool              `json:"controller_dynamic_port" value:"false" usage:"if true, controller will listen dynamic port"`
	ShaderDynamicPort             bool              `json:"shader_dynamic_port" value:"false" usage:"if true, shader will listen dynamic port"`
	BuildID                       string            `json:"build_id"`
	ShaderToolIdleRunSeconds      int               `json:"shader_tool_idle_run_seconds"`
	ControllerIdleRunSeconds      int               `json:"controller_idle_run_seconds" value:"120" usage:"controller remain time after there is no active work (seconds)"`
	ControllerNoBatchWait         bool              `json:"controller_no_batch_wait" value:"false" usage:"if true, controller will unregister immediately when no more running task"`
	ControllerSendCork            bool              `json:"controller_send_cork" value:"false" usage:"if true, controller will send file with cork"`
	ControllerSendFileMemoryLimit int64             `json:"controller_send_file_memory" value:"4294967296" usage:"memory limit when send file with cork"`
	ControllerNetErrorLimit       int               `json:"controller_net_error_limit" value:"5" usage:"judge net error if failed times over this"`
	ControllerRemoteRetryTimes    int               `json:"controller_remote_retry_times" value:"0" usage:"default remote retry times"`
	ControllerEnableLink          bool              `json:"controller_enable_link" value:"false" usage:"if true, controller will enable dist link"`
	ControllerEnableLib           bool              `json:"controller_enable_lib" value:"false" usage:"if true, controller will enable dist lib"`
	ControllerLongTCP             bool              `json:"controller_long_tcp" value:"false" usage:"if true, controller will connect to remote worker with long tcp connection"`
	ControllerWorkerOfferSlot     bool              `json:"controller_worker_offer_slot" value:"false" usage:"if true, controller will get slot by worker offer"`
	LimitPerWorker                int               `json:"limit_per_worker"`
	MaxLocalTotalJobs             int               `json:"max_Local_total_jobs"`
	MaxLocalPreJobs               int               `json:"max_Local_pre_jobs"`
	MaxLocalExeJobs               int               `json:"max_Local_exe_jobs"`
	MaxLocalPostJobs              int               `json:"max_Local_post_jobs"`
	Env                           map[string]string `json:"env"`
	ContinueOnError               bool              `json:"continue_on_error" usage:"if false, the program will stop on error immediately"`
	ControllerUseLocalCPUPercent  int               `json:"controller_use_local_cpu_percent"`
	ControllerResultCacheIndexNum int               `json:"controller_result_cache_index_num" value:"0" usage:"specify index number for local result cache"`
	ControllerResultCacheFileNum  int               `json:"controller_result_cache_file_num" value:"0" usage:"specify file number for local result cache"`
}

// Actionresult define action result
type Actionresult struct {
	Index     uint64
	Finished  bool
	Succeed   bool
	Outputmsg string
	Errormsg  string
	Exitcode  int
	Err       error
}

func uniqueAndCheck(strlist []string, allindex map[string]bool) []string {
	keys := make(map[string]bool)
	list := make([]string, 0, 0)
	for _, entry := range strlist {
		// remove "-1"
		if entry == "-1" {
			continue
		}

		// remove which not in actions list
		if _, ok := allindex[entry]; !ok {
			continue
		}

		if _, ok := keys[entry]; !ok {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// ResourceStatus save resource status
type ResourceStatus int

// define file send status
const (
	ResourceInit ResourceStatus = iota
	ResourceApplying
	ResourceApplySucceed
	ResourceApplyFailed
	ResourceUnknown = 99
)

var (
	fileStatusMap = map[ResourceStatus]string{
		ResourceInit:         "init",
		ResourceApplying:     "applying",
		ResourceApplySucceed: "applysucceed",
		ResourceApplyFailed:  "applyfailed",
		ResourceUnknown:      "unknown",
	}
)

// String return the string of FileSendStatus
func (f ResourceStatus) String() string {
	if v, ok := fileStatusMap[f]; ok {
		return v
	}

	return "unknown"
}

// SetLogLevel to set log level
func SetLogLevel(level string) {
	if level == "" {
		level = env.GetEnv(env.KeyUserDefinedLogLevel)
	}

	switch level {
	case dcUtil.PrintDebug.String():
		blog.SetV(3)
		blog.SetStderrLevel(blog.StderrLevelInfo)
	case dcUtil.PrintInfo.String():
		blog.SetStderrLevel(blog.StderrLevelInfo)
	case dcUtil.PrintWarn.String():
		blog.SetStderrLevel(blog.StderrLevelWarning)
	case dcUtil.PrintError.String():
		blog.SetStderrLevel(blog.StderrLevelError)
	case dcUtil.PrintNothing.String():
		blog.SetStderrLevel(blog.StderrLevelNothing)
	default:
		// default to be error printer.
		blog.SetStderrLevel(blog.StderrLevelInfo)
	}
}

// ++ 增加公共函数，用于从配置文件获取环境变量变设置到当前
func getProjectSettingFile() (string, error) {
	exepath := dcUtil.GetExcPath()
	if exepath != "" {
		jsonfile := filepath.Join(exepath, "bk_project_setting.json")
		if dcFile.Stat(jsonfile).Exist() {
			return jsonfile, nil
		}
	}

	return "", fmt.Errorf("not found project setting file")
}

func resolveApplyJSON(filename string) (*ApplyParameters, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var t ApplyParameters
	if err = codec.DecJSON(data, &t); err != nil {
		return nil, err
	}

	return &t, nil
}

func FreshEnvFromProjectSetting() error {
	projectSettingFile, err := getProjectSettingFile()
	if err != nil {
		return err
	}

	settings, err := resolveApplyJSON(projectSettingFile)
	if err != nil {
		return err
	}

	for k, v := range settings.Env {
		os.Setenv(k, v)
	}

	return nil
}

// --
