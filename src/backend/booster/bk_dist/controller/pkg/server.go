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
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/flock"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/api"

	// 初始化api资源
	v1 "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/api/v1"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/dashboard"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/manager"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/http/httpserver"
)

var (
	lockfile            = "bk-dist-controller.lock"
	listenPortBlackList = []int{30118} // 先写死，后面如果有需要，考虑改成可配置的
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
		blog.Errorf("[controller]: failed to start with error:%v\n", err)
		return false
	}
	blog.Infof("[controller]: ready lock file: %s\n", f)

	if !file.Stat(f).Exist() {
		newf, err := os.OpenFile(f, os.O_CREATE, os.ModePerm)
		if err != nil {
			blog.Infof("[controller]: create file: %s failed with error:%v", f, err)
		} else {
			blog.Infof("[controller]: created lock file: %s", f)
			newf.Close()
		}
	}

	flag, err := flock.TryLock(f)
	if err != nil {
		blog.Errorf("[controller]: failed to start with error:%v\n", err)
		return false
	}
	if !flag {
		blog.Infof("[controller]: program is maybe running for lock file has been locked \n")
		return false
	}

	return true
}

func unlock() {
	flock.Unlock()
}

// Server local server
type Server struct {
	conf       *config.ServerConfig
	manager    types.Mgr
	httpServer *httpserver.HTTPServer
}

// NewServer return local server
func NewServer(conf *config.ServerConfig) (*Server, error) {
	s := &Server{conf: conf}

	// Http server
	port := s.conf.Port
	if s.conf.DynamicPort {
		port = 0
	}
	s.httpServer = httpserver.NewHTTPServer(port, s.conf.Address, "")
	if s.conf.ServerCert.IsSSL {
		s.httpServer.SetSSL(
			s.conf.ServerCert.CAFile, s.conf.ServerCert.CertFile, s.conf.ServerCert.KeyFile, s.conf.ServerCert.CertPwd)
	}

	return s, nil
}

// Start : start listen and serve
func (server *Server) Start() error {
	var err error
	server.manager = manager.NewMgr(server.conf)
	go server.manager.Run()

	a := api.GetAPIResource()
	a.Manager = server.manager
	a.Conf = server.conf
	if err = api.InitActionsFunc(); err != nil {
		return err
	}

	if err = a.RegisterWebServer(server.httpServer); err != nil {
		return err
	}

	if err = dashboard.RegisterStaticServer(server.httpServer); err != nil {
		return err
	}

	if server.conf.DynamicPort {
		return server.ListenAndServeWithDynamicPort()
	}

	return server.httpServer.ListenAndServe()
}

func (server *Server) ListenAndServeWithDynamicPort() error {
	srv, ln, port, err := server.tryListen()
	if err != nil || srv == nil || ln == nil || port <= 0 {
		v1.SaveControllerInfo(0, 0, false, err.Error(), "", v1.ControllerProcessfile)
		return err
	}

	err = v1.SaveControllerInfo(os.Getpid(), port, true, "", "", v1.ControllerProcessfile)
	if err != nil {
		blog.Errorf("[controller]: save process info failed with error: %v", err)
		return err
	}

	return server.httpServer.Serve(srv, ln)
}

func (server *Server) tryListen() (*http.Server, net.Listener, int, error) {
	for i := 0; i < maxTryListenTimes; i++ {
		srv, ln, err := server.httpServer.Listen()
		if err != nil {
			blog.Errorf("[controller]: listen failed with error: %v", err)
			continue
		}

		port := ln.Addr().(*net.TCPAddr).Port
		blog.Infof("[controller]: listen with port %d", port)

		inBlack := false
		for _, p := range listenPortBlackList {
			if p == port {
				inBlack = true
				break
			}
		}

		if inBlack {
			blog.Infof("[controller]: port %d in blacklist, ignore this port", port)
			ln.Close()
			continue
		}

		return srv, ln, port, nil
	}

	blog.Errorf("[controller]: listen failed after %d try!!", maxTryListenTimes)
	return nil, nil, 0, fmt.Errorf("bind listen port failed")
}

// Run brings up the server
func Run(conf *config.ServerConfig) error {
	// avoid lauched multiple instances with diffrent ports
	if conf.DynamicPort {
		if !lock() {
			return types.ErrFileLock
		}
		defer unlock()

		conf.Port = 0
	}

	server, err := NewServer(conf)
	if err != nil {
		blog.Errorf("[controller]: init server failed: %v", err)
		return err
	}

	return server.Start()
}
