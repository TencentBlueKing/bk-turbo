/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package config

import (
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/conf"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/static"

	dcConfig "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/config"
)

// ServerConfig : server config
type ServerConfig struct {
	conf.FileConfig
	conf.ServiceConfig
	conf.LogConfig
	conf.ProcessConfig
	conf.ServerOnlyCertConfig
	conf.LocalConfig

	ServerCert *CertConfig // cert of the server
	WorkerConfig
}

// CertConfig  configuration of Cert
type CertConfig struct {
	CAFile   string
	CertFile string
	KeyFile  string
	CertPwd  string
	IsSSL    bool
}

// WorkerConfig  configuration of worker
type WorkerConfig struct {
	//MaxParallelJobs int    `json:"max_parallel_jobs" value:"8" usage:"max parallel jobs"`
	DefaultWorkDir string `json:"default_work_dir" value:"./default_work_dir" usage:"default work dir to execute scmd"`
	// CommonFileTypes []string `json:"need_clean_files" value:".pch,.gch" usage:"file types which need to save as common files"`
	CmdReplaceRules []dcConfig.CmdReplaceRule `json:"cmd_replace_rules" value:"" usage:"rules to replace input cmd"`
	CleanTempFiles  bool                      `json:"clean_temp_files" value:"true" usage:"enable temp files clean when task finished"`

	// p2p上报
	P2P           bool   `json:"p2p" value:"false" usage:"enable p2p mode"`
	P2PServer     string `json:"p2p_server" value:"" usage:"p2p server to report"`
	P2PGroupLabel string `json:"p2p_group_label" value:"" usage:"p2p group label to report"`

	// 资源使用配置
	MaxExecuteCPUPercent    int `json:"max_execute_cpu_percent" value:"80" usage:"[0~100], max execute cpu percent ,-1 means total cpu num - 1"`
	MaxExecuteMemoryPercent int `json:"max_execute_memory_percent" value:"80" usage:"max execute memory percent "`

	MaxSlotCPUPercent    int `json:"max_slot_cpu_percent" value:"80" usage:"[0~100], max slot cpu percent ,-1 means total cpu num - 1"`
	MaxSlotMemoryPercent int `json:"max_slot_memory_percent" value:"80" usage:"max slot memory percent "`

	// 是否支持绝对路径
	SupportAbsPath bool `json:"support_abs_Path" value:"true" usage:"whether support absolute path"`

	// 是否自动重启
	AutoRestart          bool `json:"auto_restart" value:"false" usage:"whether support auto restart"`
	RestartThresholdSecs int  `json:"restart_threshold_secs" value:"21600" usage:"restart when running time over this"`

	// 是否自动升级
	AutoUpgrade   bool   `json:"auto_upgrade" value:"false" usage:"whether support auto upgrade"`
	UpgradeServer string `json:"upgrade_server" value:"" usage:"upgrade server"`

	// slot由worker提供，而不是客户端指定
	OfferSlot bool `json:"offer_slot" value:"true" usage:"whether support offer slot"`

	// result cache switch
	ResultCache bool `json:"result_cache" value:"false" usage:"whether support result cache"`
	// result cache dir
	ResultCacheDir string `json:"result_cache_dir" value:"" usage:"result cache dir"`
	// max result files
	MaxResultFileNumber int `json:"max_result_file_number" value:"0" usage:"max cache file number"`
	// max result files
	MaxResultIndexNumber int `json:"max_result_index_number" value:"0" usage:"max cache index number"`
}

// NewConfig : return config of server
func NewConfig() *ServerConfig {
	return &ServerConfig{
		ServerCert: &CertConfig{
			CertPwd: static.ServerCertPwd,
			IsSSL:   false,
		},
	}
}

// Parse : parse server config
func (dsc *ServerConfig) Parse() {
	conf.Parse(dsc)

	dsc.ServerCert.CertFile = dsc.ServerCertFile
	dsc.ServerCert.KeyFile = dsc.ServerKeyFile
	dsc.ServerCert.CAFile = dsc.CAFile

	if dsc.ServerCert.CertFile != "" && dsc.ServerCert.KeyFile != "" {
		dsc.ServerCert.IsSSL = true
	}
}

var (
	GlobalResultCacheDir = ""
	GlobalMaxFileNumber  = 0
	GlobalMaxIndexNumber = 0
)
