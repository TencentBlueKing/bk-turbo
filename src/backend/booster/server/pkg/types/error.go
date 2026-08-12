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
)

// manager 层错误。与 engine/error.go 相同, Error() 字符串会进入 HTTP message, 无独立 reason code。
// 修改文案前确认是否有 client/handler 依赖 strings.Contains(见 server/pkg/api/v2/handler.go apply 等)。
var (
	// ErrorManagerNotRunning: server 重启或 manager 未就绪。UpdateHeartbeat 失败时 HTTP code 仍为 4,
	// message 含本字符串; client 必须继续等待, 不可与 ErrorUnterminatedTaskNoFound 同样处理。
	ErrorManagerNotRunning = fmt.Errorf("manager is not running")

	ErrorInvalidIPV4          = fmt.Errorf("invalid ip v4")
	ErrorIPNotAllowed         = fmt.Errorf("ip not allowed")
	ErrorConcurrencyLimit     = fmt.Errorf("the task concurrency reaches the limits")
	ErrorGenerateTaskIDFailed = fmt.Errorf("generate task id failed")

	// ErrorTaskAlreadyTerminated: ReleaseResource 时对已终止 task 返回 HTTP code=0(与心跳 code=4 不同)。
	ErrorTaskAlreadyTerminated = fmt.Errorf("task is already in terminated status")

	ErrorLeaderNoFound   = fmt.Errorf("leader no found")
	ErrorDataPathNoFound = fmt.Errorf("data path no found")
)
