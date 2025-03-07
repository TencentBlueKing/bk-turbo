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
	"fmt"
	"time"

	dcConfig "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/longtcp"
	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/resultcache"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

var ()

// Handle4QueryResultCacheIndex handler for query result cache index
type Handle4QueryResultCacheIndex struct {
}

// NewHandle4ReportResultCache return Handle4QueryResultCache
func NewHandle4QueryResultCacheIndex() *Handle4QueryResultCacheIndex {
	return &Handle4QueryResultCacheIndex{}
}

// ReceiveBody receive body for this cmd
func (h *Handle4QueryResultCacheIndex) ReceiveBody(
	client *protocol.TCPClient,
	head *dcProtocol.PBHead,
	basedir string,
	c chan<- string) (interface{}, error) {
	// recieve body
	req, err := protocol.ReceiveBKQueryResultCacheIndex(client, head, basedir, FilepathMapping, c)
	if err != nil {
		blog.Errorf("failed to receive dispatch req body error:%v", err)
		return nil, err
	}

	blog.Infof("succeed to receive dispatch req body")
	return req, nil
}

// Handle to handle this cmd
func (h *Handle4QueryResultCacheIndex) Handle(
	client *protocol.TCPClient,
	head *dcProtocol.PBHead,
	body interface{},
	receivedtime time.Time,
	basedir string,
	cmdreplacerules []dcConfig.CmdReplaceRule,
	id *longtcp.MessageID,
	s *longtcp.Session) error {
	blog.Infof("handle with base dir:%s", basedir)
	defer func() {
		blog.Infof("handle out for base dir:%s", basedir)
	}()

	// convert to req
	req, ok := body.(*dcProtocol.PBBodyQueryResultCacheIndexReq)
	if !ok {
		err := fmt.Errorf("failed to get body from interface")
		blog.Errorf("%v", err)
		return err
	}

	group_value := ""
	for _, v := range req.Attributes {
		if *v.Key == resultcache.GroupKey {
			group_value = *v.Value
			break
		}
	}

	data, err := resultcache.GetInstance(config.GlobalResultCacheDir,
		config.GlobalMaxFileNumber,
		config.GlobalMaxIndexNumber).GetRecordGroup(group_value)
	if err != nil {
		blog.Warnf("query result cache index data with error:%v", err)
	}

	// encode response to messages
	messages, err := protocol.EncodeBKQueryResultCacheIndexRsp(data)
	if err != nil {
		blog.Errorf("failed to encode rsp to messages for error:%v", err)
	}
	blog.Infof("succeed to encode query result cache index response to messages")

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
