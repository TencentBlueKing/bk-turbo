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

// Handle4QueryResultCacheFile handler for query result cache index
type Handle4QueryResultCacheFile struct {
}

// NewHandle4QueryResultCacheFile return Handle4QueryResultCacheFile
func NewHandle4QueryResultCacheFile() *Handle4QueryResultCacheFile {
	return &Handle4QueryResultCacheFile{}
}

// ReceiveBody receive body for this cmd
func (h *Handle4QueryResultCacheFile) ReceiveBody(
	client *protocol.TCPClient,
	head *dcProtocol.PBHead,
	basedir string,
	c chan<- string) (interface{}, error) {
	// recieve body
	req, err := protocol.ReceiveBKQueryResultCacheFile(client, head, basedir, FilepathMapping, c)
	if err != nil {
		blog.Errorf("failed to receive query result cache file body error:%v", err)
		return nil, err
	}

	blog.Infof("succeed to receive query result cache file body")
	return req, nil
}

// Handle to handle this cmd
func (h *Handle4QueryResultCacheFile) Handle(
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
	req, ok := body.(*dcProtocol.PBBodyQueryResultCacheFileReq)
	if !ok {
		err := fmt.Errorf("failed to get body from interface")
		blog.Errorf("%v", err)
		return err
	}

	group_id := ""
	result_id := ""
	for _, v := range req.Attributes {
		if *v.Key == resultcache.ResultKey {
			result_id = *v.Value
			continue
		}
		if *v.Key == resultcache.GroupKey {
			group_id = *v.Value
			continue
		}
	}

	results, err := resultcache.GetInstance(config.GlobalResultCacheDir,
		config.GlobalMaxFileNumber,
		config.GlobalMaxIndexNumber).GetResult(group_id, result_id, false)
	if err != nil {
		blog.Warnf("query result cache file data with error:%v", err)
	}

	var resultfiles []*dcProtocol.PBFileDesc = nil
	if len(results) > 0 {
		for _, v := range results {
			size := int64(v.RealSize)
			compresstype := dcProtocol.PBCompressType_NONE
			if v.CompressType == dcProtocol.CompressLZ4 {
				compresstype = dcProtocol.PBCompressType_LZ4
			}
			var Compressedsize int64 = int64(len(v.CompressDataBuf))
			resultfiles = append(resultfiles, &dcProtocol.PBFileDesc{
				Fullpath:       &v.FileName,
				Size:           &size,
				Md5:            &v.HashStr,
				Compresstype:   &compresstype,
				Compressedsize: &Compressedsize,
				Buffer:         v.CompressDataBuf,
			})
		}
	}

	// encode response to messages
	messages, err := protocol.EncodeBKQueryResultCacheFileRsp(resultfiles)
	if err != nil {
		blog.Errorf("failed to encode rsp to messages for error:%v", err)
	}
	blog.Infof("succeed to encode query result cache file response to messages")

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
