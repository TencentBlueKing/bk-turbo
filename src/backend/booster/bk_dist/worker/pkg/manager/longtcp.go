/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package manager

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/longtcp"
	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	pbcmd "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/cmd_handler"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	commonUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/util"
)

func (o *tcpManager) onLongTCPReceived(id longtcp.MessageID, data []byte, s *longtcp.Session) error {
	blog.Infof("onLongTCPReceived got id:%s total data length:%d", string(id), len(data))

	head, offset, err := protocol.DecodeBKCommonHead(data[0:])
	if err != nil {
		blog.Warnf("decode data failed with error:%v", err)
		s.Clean(err)
		return err
	}

	switch head.GetCmdtype() {
	case dcProtocol.PBCmdType_DISPATCHTASKREQ:
		return o.onRemoteTaskLongTCP(head, id, data[offset:], s)
	// TOOD : support other cmds
	case dcProtocol.PBCmdType_SYNCTIMEREQ:
		return o.onSyncTimeCmdLongTCP(head, id, data[offset:], s)
	case dcProtocol.PBCmdType_SENDFILEREQ:
		return o.onSendFileCmdLongTCP(head, id, data[offset:], s)
	case dcProtocol.PBCmdType_CHECKCACHEREQ:
		return o.onCheckCacheCmdLongTCP(head, id, data[offset:], s)
	default:
		err := fmt.Errorf("unknow cmd %s", head.GetCmdtype())
		blog.Warnf("long tcp got error %v", err)
		return o.onUnknownCmdLongTCP(head, id, data[offset:], s)
	}
}

func (o *tcpManager) onRemoteTaskLongTCP(
	head *dcProtocol.PBHead,
	id longtcp.MessageID,
	data []byte,
	s *longtcp.Session) error {

	blog.Infof("onRemoteTaskLongTCP got id:%s total data length:%d", string(id), len(data))

	bodylen := int(head.GetBodylen())
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d total len %d", bodylen, buflen, datalen)
		blog.Warnf("%v", err)
		s.Clean(err)
		return err
	}

	basedir := commonUtil.RandomString(16)
	body, err := protocol.DecodeBKCommonDispatchReq(data[0:datalen], head, basedir, pbcmd.FilepathMapping, o.filepathchan)
	if body == nil {
		err := fmt.Errorf("body of remote task is nil")
		blog.Errorf("%v", err)
		s.Clean(err)
		return err
	} else if err != nil {
		blog.Errorf("deocde remote task body failed with error:%v", err)
		s.Clean(err)
		return err
	}

	handler := pbcmd.GetHandler(head.GetCmdtype())
	if handler == nil {
		err := fmt.Errorf("failed to get handler for cmd %s", head.GetCmdtype())
		blog.Errorf("%v", err)
		s.Clean(err)
		return err
	}

	curcmd := buffedcmd{
		client:       nil,
		head:         head,
		receivedtime: time.Now(),
		handler:      handler,
		body:         body,
		basedir:      basedir,
		session:      s,
		id:           &id,
	}
	if o.obtainChance() {
		go func() {
			_ = o.dealBufferedCmd(&curcmd)
		}()
		return nil
	}

	o.appendcmd(&curcmd)
	o.checkCmds()

	return nil
}

func (o *tcpManager) onSyncTimeCmdLongTCP(
	head *dcProtocol.PBHead,
	id longtcp.MessageID,
	data []byte,
	s *longtcp.Session) error {

	blog.Infof("onSyncTimeCmdLongTCP got id:%s total data length:%d", string(id), len(data))

	handler := pbcmd.GetHandler(head.GetCmdtype())
	if handler == nil {
		err := fmt.Errorf("failed to get handler for cmd %s", head.GetCmdtype())
		blog.Errorf("%v", err)
		s.Clean(err)
		return err
	}

	return handler.Handle(nil, head, nil, time.Now(), "", nil, &id, s)
}

func (o *tcpManager) onSendFileCmdLongTCP(
	head *dcProtocol.PBHead,
	id longtcp.MessageID,
	data []byte,
	s *longtcp.Session) error {

	blog.Infof("onSendFileCmdLongTCP got id:%s total data length:%d", string(id), len(data))

	bodylen := int(head.GetBodylen())
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d total len %d", bodylen, buflen, datalen)
		blog.Warnf("%v", err)
		s.Clean(err)
		return err
	}

	basedir := commonUtil.RandomString(16)
	body, err := protocol.DecodeBKSendFile(data[0:datalen], head, basedir, pbcmd.FilepathMapping, o.filepathchan, pbcmd.DefaultCM)
	if body == nil {
		err := fmt.Errorf("body of remote task is nil")
		blog.Errorf("%v", err)
		s.Clean(err)
		return err
	} else if err != nil {
		blog.Errorf("deocde remote task body failed with error:%v", err)
		s.Clean(err)
		return err
	}

	debug.FreeOSMemory() // free memory anyway

	handler := pbcmd.GetHandler(head.GetCmdtype())
	if handler == nil {
		err := fmt.Errorf("failed to get handler for cmd %s", head.GetCmdtype())
		blog.Errorf("%v", err)
		s.Clean(err)
		return err
	}

	//
	err = handler.Handle(nil, head, body, time.Now(), basedir, nil, &id, s)
	return err
}

func (o *tcpManager) onCheckCacheCmdLongTCP(
	head *dcProtocol.PBHead,
	id longtcp.MessageID,
	data []byte,
	s *longtcp.Session) error {

	blog.Infof("onCheckCacheCmdLongTCP got id:%s total data length:%d", string(id), len(data))

	bodylen := int(head.GetBodylen())
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d total len %d", bodylen, buflen, datalen)
		blog.Warnf("%v", err)
		s.Clean(err)
		return err
	}

	body, err := protocol.DecodeBKCheckCache(data[0:datalen], head, "", nil, nil)
	if body == nil {
		err := fmt.Errorf("body of remote task is nil")
		blog.Errorf("%v", err)
		s.Clean(err)
		return err
	} else if err != nil {
		blog.Errorf("deocde remote task body failed with error:%v", err)
		s.Clean(err)
		return err
	}

	debug.FreeOSMemory() // free memory anyway

	handler := pbcmd.GetHandler(head.GetCmdtype())
	if handler == nil {
		err := fmt.Errorf("failed to get handler for cmd %s", head.GetCmdtype())
		blog.Errorf("%v", err)
		s.Clean(err)
		return err
	}

	//
	err = handler.Handle(nil, head, body, time.Now(), "", nil, &id, s)
	return err
}

func (o *tcpManager) onUnknownCmdLongTCP(head *dcProtocol.PBHead,
	id longtcp.MessageID,
	data []byte,
	s *longtcp.Session) error {

	blog.Infof("onUnknownCmdLongTCP got id:%s total data length:%d", string(id), len(data))

	handler := pbcmd.GetHandler(head.GetCmdtype())
	if handler == nil {
		err := fmt.Errorf("failed to get handler for cmd %s", head.GetCmdtype())
		blog.Errorf("%v", err)
		s.Clean(err)
		return err
	}

	bodylen := int(head.GetBodylen())
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d total len %d", bodylen, buflen, datalen)
		blog.Warnf("%v", err)
		s.Clean(err)
		return err
	}
	_ = protocol.DecodeUnknown(data[0:datalen], head, "", nil)

	return handler.Handle(nil, head, nil, time.Now(), "", nil, &id, s)
}
