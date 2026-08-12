/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package engine

import "fmt"

// 本文件定义 engine/manager 内部错误。这些错误的 Error() 字符串会经 HTTP handler 拼入
// APIResponse.message 返回给 client(见 server/pkg/api/writer.go), 但无独立 reason code。
//
// 修改下方任意 Error() 文案前必读:
//   - client 侧可能用 strings.Contains(message, ...) 做分支(如 resource/mgr heartBeatCanStop)
//   - 用户机器上的 booster 版本滞后, 改字符串会导致旧 client 行为异常
//   - 应优先在 API 层增加稳定 reason/code, 而非只改此处文案; 参见 server/pkg/api/v2/handler.go
//
// 与 server/pkg/api/error.go 中 ServerErrCode(HTTP 大类 code)不是同一层: 例如心跳失败 HTTP
// code 恒为 4, 本文件细因体现在 message 末尾。
var (
	ErrorUnknownEngineType = fmt.Errorf("unknown engine type")
	ErrorNoTaskInQueue     = fmt.Errorf("there is no task in the queue")
	ErrorNoEnoughResources = fmt.Errorf("no enough resources")
	ErrorTaskNoFound       = fmt.Errorf("task no found")

	// ErrorUnterminatedTaskNoFound: task 不在 manager layer 缓存(已 release 或不存在)。
	// 经 UpdateHeartbeat 返回时 HTTP code=4, message 含本字符串; client heartBeatCanStop 据此释放 work。
	// 经 ReleaseResource(v2) 返回时 HTTP code=0(幂等)。禁止随意改文案。
	ErrorUnterminatedTaskNoFound = fmt.Errorf("unterminated task no found")

	ErrorProjectNoFound       = fmt.Errorf("project no found")
	ErrorWhitelistNoFound     = fmt.Errorf("whitelist no found")
	ErrorInnerEngineError     = fmt.Errorf("inner engine error")
	ErrorNoSupportDegrade     = fmt.Errorf("no support degrade")
	ErrorNoQueueNameSpecified = fmt.Errorf("no queue name specified")
	ErrorUnknownMessageType   = fmt.Errorf("unknown message type")
	ErrorIPNotSpecified       = fmt.Errorf("ip not specified")
)
