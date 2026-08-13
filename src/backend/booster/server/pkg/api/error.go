/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package api

// ServerErrCode 为 HTTP 响应大类 code(如心跳失败均为 UpdateHeartbeatFailed=4)。
// 细因见 server/pkg/engine/error.go、server/pkg/types/error.go, 经 message 返回, 无独立 reason code。
// ServerErrorCode implements the ErrorCode
type ServerErrCode int

const (
	ServerErrOK ServerErrCode = iota
	ServerErrInvalidParam
	ServerErrApplyResourceFailed
	ServerErrRequestTaskInfoFailed
	ServerErrUpdateHeartbeatFailed
	ServerErrReleaseResourceFailed
	ServerErrPreProcessFailed
	ServerErrRedirectFailed
	ServerErrEncodeJSONFailed
	ServerErrGetServersFailed
	ServerErrSendMessageFailed
	ServerErrUnknownMessageType
	ServerErrReadJSONFailed
)

var serverErrCode = map[ServerErrCode]string{
	ServerErrOK:                    "request OK",
	ServerErrInvalidParam:          "invalid param",
	ServerErrApplyResourceFailed:   "apply resource failed",
	ServerErrRequestTaskInfoFailed: "request task info failed",
	ServerErrUpdateHeartbeatFailed: "update heartbeat failed",
	ServerErrReleaseResourceFailed: "release resource failed",
	ServerErrPreProcessFailed:      "pre process failed",
	ServerErrRedirectFailed:        "redirect failed",
	ServerErrEncodeJSONFailed:      "encode json failed",
	ServerErrGetServersFailed:      "get servers failed",
	ServerErrSendMessageFailed:     "send message failed",
	ServerErrUnknownMessageType:    "unknown message type",
	ServerErrReadJSONFailed:        "read json failed",
}

// String get error string from error code
func (sec ServerErrCode) String() string {
	if _, ok := serverErrCode[sec]; !ok {
		return "unknown server error"
	}
	return serverErrCode[sec]
}

// Int get code int from error code
func (sec ServerErrCode) Int() int {
	return int(sec)
}
