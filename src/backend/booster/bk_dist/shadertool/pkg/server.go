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
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/flock"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	v1 "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/api/v1"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/http/httpserver"
)

var (
	lockfile            = "bk-shader-tool.lock"
	listenPortBlackList = []int{30117} // 先写死，后面如果有需要，考虑改成可配置的
	maxTryListenTimes   = 10
)

func getLockFile() (string, error) {
	dir := util.GetGlobalDir()
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return "", err
	}
	return filepath.Join(dir, lockfile), nil
}

func lock() bool {
	f, err := getLockFile()
	if err != nil {
		blog.Errorf("[shadertool]: failed to start with error:%v\n", err)
		return false
	}
	blog.Infof("[shadertool]: ready lock file: %s\n", f)

	if !file.Stat(f).Exist() {
		newf, err := os.OpenFile(f, os.O_CREATE, os.ModePerm)
		if err != nil {
			blog.Infof("[shadertool]: create file: %s failed with error:%v", f, err)
		} else {
			blog.Infof("[shadertool]: created lock file: %s", f)
			newf.Close()
		}
	}

	flag, err := flock.TryLock(f)
	if err != nil {
		blog.Errorf("[shadertool]: failed to start with error:%v\n", err)
		return false
	}
	if !flag {
		blog.Infof("[shadertool]: program is maybe running for lock file has been locked \n")
		return false
	}

	return true
}

func unlock() {
	flock.Unlock()
}

func (h *ShaderTool) getProcessFilePath() (string, string) {
	if h.flags.ProcessInfoFile != "" {
		d := filepath.Dir(h.flags.ProcessInfoFile)
		f := filepath.Base(h.flags.ProcessInfoFile)
		return d, f
	}

	return "", v1.ShaderProcessfile
}

func (h *ShaderTool) ListenAndServeWithDynamicPort() error {
	d, f := h.getProcessFilePath()

	srv, ln, port, err := h.tryListen()
	if err != nil || srv == nil || ln == nil || port <= 0 {
		v1.SaveControllerInfo(0, 0, false, err.Error(), d, f)
		return err
	}

	err = v1.SaveControllerInfo(os.Getpid(), port, true, "", d, f)
	if err != nil {
		blog.Errorf("[shadertool]: save process info failed with error: %v", err)
		return err
	}

	return h.httpserver.Serve(srv, ln)
}

func (h *ShaderTool) tryListen() (*http.Server, net.Listener, int, error) {
	for i := 0; i < maxTryListenTimes; i++ {
		srv, ln, err := h.httpserver.Listen()
		if err != nil {
			blog.Errorf("[shadertool]: listen failed with error: %v", err)
			continue
		}

		port := ln.Addr().(*net.TCPAddr).Port
		blog.Infof("[shadertool]: listen with port %d", port)

		inBlack := false
		for _, p := range listenPortBlackList {
			if p == port {
				inBlack = true
				break
			}
		}

		if inBlack {
			blog.Infof("[shadertool]: port %d in blacklist, ignore this port", port)
			ln.Close()
			continue
		}

		return srv, ln, port, nil
	}

	blog.Errorf("[shadertool]: listen failed after %d try!!", maxTryListenTimes)
	return nil, nil, 0, fmt.Errorf("bind listen port failed")
}

func (h *ShaderTool) server(ctx context.Context) error {
	blog.Infof("[shadertool]: server")

	withDynamicPort := h.settings.ShaderDynamicPort || h.flags.Port == 0

	if withDynamicPort {
		if !lock() {
			return types.ErrFileLock
		}
		defer unlock()

		h.flags.Port = 0
	}

	h.httpserver = httpserver.NewHTTPServer(uint(h.flags.Port), "127.0.0.1", "")
	var err error

	h.httphandle, err = NewHTTPHandle(h)
	if h.httphandle == nil || err != nil {
		return ErrInitHTTPHandle
	}

	h.httpserver.RegisterWebServer(PathV1, nil, h.httphandle.GetActions())

	if withDynamicPort {
		return h.ListenAndServeWithDynamicPort()
	}

	return h.httpserver.ListenAndServe()
}
