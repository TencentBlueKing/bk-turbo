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
	"path/filepath"
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

// Handle4ReportResultCache handler for report result cache
type Handle4ReportResultCache struct {
}

// NewHandle4ReportResultCache return Handle4ReportResultCache
func NewHandle4ReportResultCache() *Handle4ReportResultCache {
	return &Handle4ReportResultCache{}
}

// ReceiveBody receive body for this cmd
func (h *Handle4ReportResultCache) ReceiveBody(
	client *protocol.TCPClient,
	head *dcProtocol.PBHead,
	basedir string,
	c chan<- string) (interface{}, error) {
	// recieve body
	req, err := protocol.ReceiveBKReportResultCache(client, head, basedir, FilepathMapping, c)
	if err != nil {
		blog.Errorf("receive report result cache body with error:%v", err)
		return nil, err
	}

	blog.Infof("succeed to receive report result cache body")
	return req, nil
}

// Handle to handle this cmd
func (h *Handle4ReportResultCache) Handle(
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
	req, ok := body.(*dcProtocol.PBBodyReportResultCacheReq)
	if !ok {
		err := fmt.Errorf("failed to get body from interface")
		blog.Errorf("%v", err)
		return err
	}

	// save report info and *protocol.PBResult
	blog.Infof("got %d Attributes %d files", len(req.Attributes), len(req.Resultfiles))
	if len(req.Attributes) > 0 {
		record := resultcache.Record{}
		for _, v := range req.Attributes {
			record[*v.Key] = *v.Value
		}
		resultcache.GetInstance(config.GlobalResultCacheDir,
			config.GlobalMaxFileNumber,
			config.GlobalMaxIndexNumber).PutRecord(record)
		if len(req.Resultfiles) > 0 {
			if hashstr := record.GetStringByKey(resultcache.ResultKey); hashstr != "" {
				resultlen := len(req.Resultfiles)
				rs := make([]*resultcache.Result, 0, resultlen)
				for _, v := range req.Resultfiles {
					rs = append(rs, &resultcache.Result{
						FileName:        filepath.Base(*v.Fullpath),
						CompressDataBuf: v.Buffer,
						CompressType:    dcProtocol.CompressType(*v.Compresstype),
						RealSize:        uint64(*v.Size),
						HashStr:         "",
					})
				}
				groupid := record.GetStringByKey(resultcache.GroupKey)
				resultcache.GetInstance(config.GlobalResultCacheDir,
					config.GlobalMaxFileNumber,
					config.GlobalMaxIndexNumber).PutResult(groupid, hashstr, rs, record)
			}
		}
	}

	// encode response to messages
	messages, err := protocol.EncodeBKReportResultCacheRsp()
	if err != nil {
		blog.Errorf("failed to encode rsp to messages for error:%v", err)
	}
	blog.Infof("succeed to encode report result cache response to messages")

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
