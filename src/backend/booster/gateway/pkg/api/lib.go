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
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	commonTypes "github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/gateway/pkg/types"

	"github.com/emicklei/go-restful"
)

// MasterRequired wrap the api handler and make it only available for master node.
func MasterRequired(f restful.RouteFunction) func(req *restful.Request, resp *restful.Response) {
	return process(f, ProcessMasterOnly)
}

// NoLimit wrap the api handler and will process all requests.
func NoLimit(f restful.RouteFunction) func(req *restful.Request, resp *restful.Response) {
	return process(f, ProcessNoLimit)
}

// AuthRequired wrap the api handler with IP whitelist authentication.
func AuthRequired(f restful.RouteFunction) func(req *restful.Request, resp *restful.Response) {
	return process(f, ProcessAuthRequired)
}

// Process log before and after a request. If options is mater-required, then redirect the request to master node and
// return the data from master node.
func process(f restful.RouteFunction, opts ProcessType) func(req *restful.Request, resp *restful.Response) {
	return func(req *restful.Request, resp *restful.Response) {
		entranceTime := time.Now().Local()
		blog.Infof("Receive %s %s?%s From %s",
			req.Request.Method, req.Request.URL.Path, req.Request.URL.RawQuery, req.Request.RemoteAddr)

		switch opts {
		case ProcessNoLimit:
			f(req, resp)
		case ProcessMasterOnly:
			var isMaster bool
			var leader *types.ServerInfo
			var err error

			if Rd == nil {
				err = fmt.Errorf("rd not init, can not found master")
			} else {
				isMaster, leader, err = Rd.IsMaster()
			}

			if err != nil {
				blog.Errorf("process get master failed url(%s %s?%s): %v",
					req.Request.Method, req.Request.URL.Path, req.Request.URL.RawQuery, err)
				ReturnRest(&RestResponse{Resp: resp, ErrCode: commonTypes.ServerErrPreProcessFailed,
					HTTPCode: http.StatusInternalServerError})
			} else if isMaster {
				f(req, resp)
			} else {
				redirect(leader.GetURI(), req, resp)
			}
		case ProcessAuthRequired:
			if !checkIPWhitelist(req.Request) {
				clientIP, _ := getClientIP(req.Request)
				blog.Errorf("IP %s not in whitelist for url(%s %s?%s)",
					clientIP, req.Request.Method, req.Request.URL.Path, req.Request.URL.RawQuery)
				ReturnRest(&RestResponse{Resp: resp, ErrCode: commonTypes.ServerErrInvalidParam,
					HTTPCode: http.StatusForbidden, Message: "Access denied: IP not in whitelist"})
				return
			}
			f(req, resp)
		}

		useTime := time.Since(entranceTime).Nanoseconds() / 1000 / 1000
		blog.Infof("Return [%d] %dms %s %s To %s",
			resp.StatusCode(), useTime, req.Request.Method, req.Request.URL.Path, req.Request.RemoteAddr)
	}
}

type ProcessType string

const (
	ProcessMasterOnly   ProcessType = "master_only"
	ProcessNoLimit      ProcessType = "no_limit"
	ProcessAuthRequired ProcessType = "auth_required"
)

// redirect is like a proxy, it requests to the other node and return the data from that one.
func redirect(uri string, req *restful.Request, resp *restful.Response) {
	uri += req.Request.URL.Path + "?" + req.Request.URL.RawQuery

	blog.Infof("redirect to uri: %s %s", req.Request.Method, uri)
	r, err := http.NewRequest(req.Request.Method, uri, req.Request.Body)
	if err != nil {
		blog.Errorf("redirect to uri(%s %s), get new request failed: %v", req.Request.Method, uri, err)
		ReturnRest(&RestResponse{Resp: resp, ErrCode: commonTypes.ServerErrRedirectFailed,
			HTTPCode: http.StatusInternalServerError})
		return
	}
	r.Close = true
	rsp, err := defaultClient.Do(r)
	if err != nil {
		blog.Errorf("redirect to uri(%s %s), do request failed: %v", req.Request.Method, uri, err)
		ReturnRest(&RestResponse{Resp: resp, ErrCode: commonTypes.ServerErrRedirectFailed,
			HTTPCode: http.StatusInternalServerError})
		return
	}
	defer func() {
		_ = rsp.Body.Close()
	}()

	data, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		blog.Errorf("redirect to uri(%s %s) failed: %v", req.Request.Method, uri, err)
		ReturnRest(&RestResponse{Resp: resp, ErrCode: commonTypes.ServerErrRedirectFailed,
			HTTPCode: http.StatusInternalServerError})
		return
	}
	_, _ = resp.ResponseWriter.Write(data)
}

var defaultClient = &http.Client{}

// checkIPWhitelist checks if the client IP is in the whitelist.
func checkIPWhitelist(req *http.Request) bool {
	// 如果未启用IP白名单，则允许所有访问
	if GatewayConf == nil || !GatewayConf.IPWhitelist.Enable {
		return true
	}

	clientIP, err := getClientIP(req)
	if err != nil {
		blog.Errorf("get client IP failed: %v", err)
		return false
	}

	parsedClientIP := net.ParseIP(clientIP)
	if parsedClientIP == nil {
		blog.Errorf("invalid client IP: %s", clientIP)
		return false
	}

	// 开启白名单但未配置IP时，拒绝访问，避免误放行
	for _, allowedIP := range GatewayConf.IPWhitelist.WhitelistIP {
		allowedIP = strings.TrimSpace(allowedIP)
		if allowedIP == "" {
			continue
		}

		if strings.Contains(allowedIP, "/") {
			_, ipNet, err := net.ParseCIDR(allowedIP)
			if err != nil {
				blog.Errorf("parse CIDR %s failed: %v", allowedIP, err)
				continue
			}
			if ipNet.Contains(parsedClientIP) {
				return true
			}
			continue
		}

		if allowed := net.ParseIP(allowedIP); allowed != nil && allowed.Equal(parsedClientIP) {
			return true
		}
	}

	return false
}

func getClientIP(req *http.Request) (string, error) {
	if req == nil {
		return "", fmt.Errorf("empty request")
	}

	if xRealIP := strings.TrimSpace(req.Header.Get("X-Real-IP")); xRealIP != "" {
		return parseClientIP(xRealIP)
	}

	if xForwardedFor := req.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
		for _, ip := range strings.Split(xForwardedFor, ",") {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			return parseClientIP(ip)
		}
	}

	return parseClientIP(req.RemoteAddr)
}

// parseClientIP extracts the client IP from remote address.
func parseClientIP(remoteAddr string) (string, error) {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return "", fmt.Errorf("empty IP address")
	}

	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}

	remoteAddr = strings.Trim(remoteAddr, "[]")
	ip := net.ParseIP(remoteAddr)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address: %s", remoteAddr)
	}

	return ip.String(), nil
}
