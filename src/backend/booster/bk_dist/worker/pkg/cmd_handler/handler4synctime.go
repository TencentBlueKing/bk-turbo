/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package pbcmd

import (
	"time"

	dcConfig "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/longtcp"
	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

var ()

// Handle4SyncTime handler for dispatch task request
type Handle4SyncTime struct {
}

// NewHandle4SyncTime return Handle4SyncTime
func NewHandle4SyncTime() *Handle4SyncTime {
	return &Handle4SyncTime{}
}

// ReceiveBody receive body for this cmd
func (h *Handle4SyncTime) ReceiveBody(client *protocol.TCPClient,
	head *dcProtocol.PBHead,
	basedir string,
	c chan<- string) (interface{}, error) {

	blog.Infof("not implement now")
	return nil, nil
}

// Handle to handle this cmd
func (h *Handle4SyncTime) Handle(client *protocol.TCPClient,
	head *dcProtocol.PBHead,
	body interface{},
	receivedtime time.Time,
	basedir string,
	cmdreplacerules []dcConfig.CmdReplaceRule,
	id *longtcp.MessageID,
	s *longtcp.Session) error {

	go h.handle(client, receivedtime, id, s)
	return nil
}

// Handle to handle this cmd
func (h *Handle4SyncTime) handle(
	client *protocol.TCPClient,
	receivedtime time.Time,
	id *longtcp.MessageID,
	s *longtcp.Session) error {
	blog.Infof("handle to sync time")
	defer func() {
		blog.Infof("handle out for sync time")
		if client != nil {
			client.Close()
		}
	}()

	// encode response to messages
	messages, err := protocol.EncodeBKSyncTimeRsp(receivedtime)
	if err != nil {
		blog.Errorf("failed to encode rsp to messages for error:%v", err)
	}
	blog.Infof("succeed to encode dispatch response to messages")

	// send response
	if client != nil {
		err = protocol.SendMessages(client, &messages)
	} else {
		rspdata := [][]byte{}
		for _, m := range messages {
			rspdata = append(rspdata, m.Data)
		}
		ret := s.SendWithID(*id, rspdata, false)
		err = ret.Err
	}
	if err != nil {
		blog.Errorf("failed to send messages for error:%v", err)
	}
	blog.Infof("succeed to send messages")

	return nil
}
