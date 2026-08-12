/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package api

import (
	"net/http"

	http2 "github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/http"

	"github.com/emicklei/go-restful"
)

// ErrorCode describe the error from http handler
type ErrorCode interface {
	// get error string
	String() string

	// get error code int
	Int() int
}

// RestResponse contains all response information need by a http handler
type RestResponse struct {
	Resp     *restful.Response
	HTTPCode int

	Data    interface{}
	ErrCode ErrorCode
	Message string
	Extra   map[string]interface{}

	WrapFunc func([]byte) []byte
}

// ReturnRest do the return work according to a RestResponse
func ReturnRest(resp *RestResponse) {
	if resp.HTTPCode == 0 {
		resp.HTTPCode = http.StatusOK
	}

	if resp.ErrCode == nil {
		resp.ErrCode = ServerErrOK
	}

	if resp.Message == "" {
		resp.Message = resp.ErrCode.String()
	} else {
		// message 会与 ErrCode 文案拼接; 失败时 createResponseEx 再加进程名前缀。
		// 细因(engine error)无独立 reason 字段, client 不宜依赖整段 message 作跨版本分支。
		resp.Message = resp.ErrCode.String() + " | " + resp.Message
	}

	result, _ := http2.GetResponseEx(resp.ErrCode.Int(), resp.Message, resp.Data, resp.Extra)

	if resp.WrapFunc != nil {
		result = resp.WrapFunc(result)
	}
	resp.Resp.WriteHeader(resp.HTTPCode)
	_, _ = resp.Resp.Write(result)
}
