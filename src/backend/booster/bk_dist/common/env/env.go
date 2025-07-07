/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package env

import (
	"os"
)

// define const string
const (
	CommonEnvPrefix = "BK_DIST_"

	HostList    = "BOOSTER_HOSTS"
	BoosterType = "BOOSTER_TYPE"
	ProjectID   = "BOOSTER_PROJECT_ID"
	BatchMode   = "BOOSTER_BATCH_MODE"

	KeyExecutorLogLevel                = "LOG_LEVEL"
	KeyExecutorHookPreloadLibraryLinux = "LD_PRELOAD"
	KeyExecutorHookPreloadLibraryMacos = "DYLD_INSERT_LIBRARIES"
	KeyExecutorHookConfigContent       = "HOOK_CONFIG_CONTENT"
	KeyExecutorHookConfigContentRaw    = "HOOK_CONFIG_CONTENT_RAW"
	KeyExecutorControllerScheme        = "CONTROLLER_SCHEME"
	KeyExecutorControllerIP            = "CONTROLLER_IP"
	KeyExecutorControllerPort          = "CONTROLLER_PORT"
	KeyExecutorControllerNoLocal       = "CONTROLLER_NO_LOCAL"
	KeyExecutorControllerWorkID        = "CONTROLLER_WORK_ID"
	KeyExecutorTaskID                  = "TASK_ID"
	KeyExecutorSkipSeparating          = "SKIP_SEPARATING"
	KeyExecutorSkipLocalRetry          = "SKIP_LOCAL_RETRY"
	KeyExecutorIOTimeout               = "IO_TIMEOUT"
	KeyExecutorIOTimeout4UBA           = "IO_TIMEOUT_4_UBA"
	KeyExecutorToolchainPathMap        = "TOOLCHAIN_PATH_MAP"
	KeyExecutorEnableLib               = "ENABLE_LIB"
	KeyExecutorEnableLink              = "ENABLE_LINK"
	KeyExecutorSupportDirectives       = "SUPPORT_DIRECTIVES"
	KeyExecutorPump                    = "PUMP"
	KeyExecutorPumpDisableMacro        = "PUMP_DISABLE_MACRO"
	KeyExecutorPumpIncludeSysHeader    = "PUMP_INCLUDE_SYS_HEADER"
	KeyExecutorPumpCheck               = "PUMP_CHECK"
	KeyExecutorPumpCache               = "PUMP_CACHE"             // cache pump inlude files
	KeyExecutorPumpCacheDir            = "PUMP_CACHE_DIR"         // cache pump inlude files
	KeyExecutorPumpCacheSizeMaxMB      = "PUMP_CACHE_SIZE_MAX_MB" // cache pump inlude files
	KeyExecutorPumpBlackKeys           = "PUMP_BLACK_KEYS"
	KeyExecutorPumpMinActionNum        = "PUMP_MIN_ACTION_NUM"
	KeyExecutorPumpDisableStatCache    = "PUMP_DISABLE_STAT_CACHE"
	KeyExecutorPumpSearchLink          = "PUMP_SEARCH_LINK"
	KeyExecutorPumpSearchLinkResult    = "PUMP_SEARCH_LINK_RESULT"
	KeyExecutorPumpLstatByDir          = "PUMP_LSTAT_BY_DIR"
	KeyExecutorPumpCorrectCap          = "PUMP_CORRECT_CAP"
	KeyExecutorForceLocalKeys          = "FORCE_LOCAL_KEYS"
	KeyExecutorCCEnsureFileOwnerkey    = "CC_ENSURE_OWNER"
	KeyExecutorEnvProfile              = "ENV_PROFILE"
	KeyExecutorWorkerSideCache         = "WORKER_SIDE_CACHE"
	KeyExecutorLocalRecord             = "LOCAL_RECORD"
	KeyExecutorWriteMemory             = "WRITE_MEMORY"
	KeyExecutorIdleKeepSecs            = "IDLE_KEEP_SECS"
	KeyExecutorTotalActionNum          = "TOTAL_ACTION_NUM"
	KeyExecutorUseWebSocket            = "USE_WEBSOCKET"
	KeyExecutorNewShader               = "NEW_SHADER"
	KeyExecutorSearchToolchain         = "SEARCH_TOOLCHAIN"
	KeyExecutorUECLNotUseLocal         = "UE_CL_NOT_USE_LOCAL"
	KeyExecutorUECCNotUseLocal         = "UE_CC_NOT_USE_LOCAL"
	KeyExecutorUELibNotUseLocal        = "UE_LIB_NOT_USE_LOCAL"
	KeyExecutorUELinkNotUseLocal       = "UE_LINK_NOT_USE_LOCAL"
	KeyExecutorUEShaderNotUseLocal     = "UE_SHADER_NOT_USE_LOCAL"
	KeyExecutorUECLLocalCPUWeight      = "UE_CL_LOCAL_CPU_WEIGHT"
	KeyExecutorUECCLocalCPUWeight      = "UE_CC_LOCAL_CPU_WEIGHT"
	KeyExecutorUELibLocalCPUWeight     = "UE_LIB_LOCAL_CPU_WEIGHT"
	KeyExecutorUELinkLocalCPUWeight    = "UE_LINK_LOCAL_CPU_WEIGHT"
	KeyExecutorUEShaderLocalCPUWeight  = "UE_SHADER_LOCAL_CPU_WEIGHT"
	KeyExecutorIgnoreHttpStatus        = "IGNORE_HTTP_STATUS"
	KeyExecutorResultCacheType         = "RESULT_CACHE_TYPE"
	KeyExecutorResultCacheTriggleSecs  = "RESULT_CACHE_TRIGGLE_SECS"
	KeyExecutorHasResultIndex          = "HAS_RESULT_INDEX"

	KeyUserDefinedLogLevel         = "USER_DEFINED_LOG_LEVEL"
	KeyUserDefinedExecutorLogLevel = "USER_DEFINED_EXECUTOR_LOG_LEVEL"
	KeyUserDefinedJobs             = "USER_DEFINED_JOBS"
	KeyUserDefinedMaxJobs          = "USER_DEFINED_MAX_JOBS"
	KeyUserDefinedMaxLocalJobs     = "USER_DEFINED_MAX_LOCAL_JOBS"
	KeyUserDefinedMaxLocalExeJobs  = "USER_DEFINED_MAX_LOCAL_EXE_JOBS"
	KeyUserDefinedMaxLocalPreJobs  = "USER_DEFINED_MAX_LOCAL_PRE_JOBS"
	KeyUserDefinedMaxLocalPostJobs = "USER_DEFINED_MAX_LOCAL_POST_JOBS"
	KeyUserDefinedIOTimeoutSecs    = "USER_DEFINED_IO_TIMEOUT_SECS"
	KeyUserDefinedForceLocalList   = "USER_DEFINED_FORCE_LOCAL_LIST"

	KeyCommonCheckMd5      = "CHECK_MD5"
	KeyCommonUE4MaxProcess = "UE4_MAX_PROCESS"
	// for ubt tool
	KeyCommonUE4MaxJobs = "UE4_MAX_JOBS"

	KeyWorkerPort           = "PORT_4_WORKER"        // port for worker,default is 30811
	KeyWorkerMaxProcess     = "MAX_PROCESS_4_WORKER" // max process number, default is 8
	KeyWorkerMaxJobs        = "MAX_JOBS_4_WORKER"    // max parallel jobs
	KeyWorkerWhiteIP        = "WHITE_IP"             // such as "192.168.0.1 192.168.0.2 192.168.0.3 0.0.0.0"
	KeyWorkerCacheEnable    = "CACHE_ENABLE"
	KeyWorkerCacheDir       = "CACHE_DIR"
	KeyWorkerCachePoolSize  = "CACHE_POOL_SIZE"
	KeyWorkerCacheMinSize   = "CACHE_MIN_SIZE"
	KeyWorkerMemPerJob      = "MEM_PER_JOB_4_WORKER" // memory per job
	KeyWorkerSupportAbsPath = "SUPPORT_ABS_PATH"     // whether support abs path
	KeyWorkerWithRootEnv    = "WITH_ROOT_ENV"

	KeyCustomSetting = "CUSTOM_SETTINGS"

	CommonBKEnvSepKey = "!!|!!"

	KeyRemoteEnvAppend    = "REMOTE_ENV_APPEND"
	KeyRemoteEnvOverwrite = "REMOTE_ENV_OVERWRITE"
)

// GetEnvKey return env value by specified key
func GetEnvKey(key string) string {
	// preload env key is set for system, not need prefix.
	if key == KeyExecutorHookPreloadLibraryLinux ||
		key == KeyExecutorHookPreloadLibraryMacos {
		return key
	}

	return CommonEnvPrefix + key
}

// GetEnv get env
func GetEnv(key string) string {
	return os.Getenv(GetEnvKey(key))
}

// SetEnv set env
func SetEnv(key, value string) error {
	return os.Setenv(GetEnvKey(key), value)
}

// UnsetEnv unset env
func UnsetEnv(key string) error {
	return os.Unsetenv(GetEnvKey(key))
}
