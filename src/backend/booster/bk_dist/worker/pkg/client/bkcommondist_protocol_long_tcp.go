/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package client

import (
	"fmt"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"

	"github.com/gogo/protobuf/proto"
)

// --------------------------for long tcp----------------------------------------------------
func encodeLongTCPHandshakeReq() []byte {
	blog.Debugf("encode long tcp handshake request to message now")

	// encode head
	var bodylen int32
	var filebuflen int64
	cmdtype := protocol.PBCmdType_LONGTCPHANDSHAKEREQ
	pbhead := protocol.PBHead{
		Version: &bkdistcmdversion,
		Magic:   &bkdistcmdmagic,
		Bodylen: &bodylen,
		Buflen:  &filebuflen,
		Cmdtype: &cmdtype,
	}
	headdata, err := proto.Marshal(&pbhead)
	if err != nil {
		blog.Warnf("failed to proto.Marshal pbhead with error:%v", err)
		return nil
	}
	blog.Debugf("encode head[%s] to size %d", pbhead.String(), pbhead.XXX_Size())

	headtokendata, err := bkformatTokenInt(protocol.TOEKNHEADFLAG, pbhead.XXX_Size())
	if err != nil {
		blog.Warnf("failed to format head token with error:%v", err)
		return nil
	}

	return append(headtokendata, headdata...)
}

func decodeCommonHead(data []byte) (*protocol.PBHead, int, error) {
	blog.Debugf("decode bk-common-disptach head now")

	offset := 0
	datalen := len(data)
	if datalen < protocol.TOKENBUFLEN {
		blog.Warnf("data length is less than protocol.TOKENBUFLEN")
		return nil, 0, fmt.Errorf("data length is less than protocol.TOKENBUFLEN")
	}

	// resolve head token
	headlen, _, err := bkreadTokenInt(data[0:protocol.TOKENBUFLEN], protocol.TOEKNHEADFLAG)
	if err != nil {
		blog.Warnf("failed to get head token with error:%s", err)
		return nil, offset, err
	}
	if err != nil || headlen <= 0 {
		err := fmt.Errorf("headlen %d is invalid", headlen)
		blog.Warnf("got invalid head token len %d", headlen)
		return nil, offset, err
	}

	if datalen < protocol.TOKENBUFLEN+headlen {
		blog.Warnf("data length is less than protocol.TOKENBUFLEN + headlen")
		return nil, 0, fmt.Errorf("data length is less than protocol.TOKENBUFLEN + headlen")
	}

	head := protocol.PBHead{}
	err = proto.Unmarshal(data[protocol.TOKENBUFLEN:protocol.TOKENBUFLEN+headlen], &head)
	if err != nil {
		blog.Warnf("failed to decode pbhead error: %v", err)
	} else {
		blog.Debugf("succeed to decode pbhead %s", head.String())
		if err := checkHead(&head); err != nil {
			blog.Warnf("failed to check head for error: %v", err)
			return nil, offset, err
		}
	}

	offset = protocol.TOKENBUFLEN + headlen
	return &head, offset, nil
}

func decodeCommonDispatchRspLongTCP(
	data []byte,
	savefile bool,
	sandbox *syscall.Sandbox) (*protocol.PBBodyDispatchTaskRsp, error) {
	blog.Debugf("decode data to bk-common-disptach response")

	// decode head
	head, offset, err := decodeCommonHead(data)
	if err != nil {
		return nil, err
	}

	if head.GetCmdtype() != protocol.PBCmdType_DISPATCHTASKRSP {
		err := fmt.Errorf("unknown cmd type %v", head.GetCmdtype())
		blog.Warnf("%v", err)
		return nil, err
	}

	bodylen := head.GetBodylen()
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < offset+int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d with head len:%d total len:%d",
			bodylen, buflen, offset, datalen)
		blog.Warnf("%v", err)
		return nil, err
	}
	blog.Infof("after docode data got headlen %d bodylen %d buflen %d totallen %d", offset, bodylen, buflen, datalen)

	// decode body
	body := protocol.PBBodyDispatchTaskRsp{}
	err = proto.Unmarshal(data[offset:offset+int(bodylen)], &body)

	if err != nil {
		blog.Warnf("failed to decode pbbody error: %v", err)
	} else {
		blog.Debugf("succeed to decode pbbody ")
	}

	// receive buf and save to files
	if buflen > 0 {
		if err := decodeCommonDispatchRspBufLongTCP(data[offset+int(bodylen):datalen], &body, buflen, savefile, sandbox); err != nil {
			return nil, err
		}
	}

	return &body, nil
}

func decodeCommonDispatchRspBufLongTCP(
	data []byte,
	body *protocol.PBBodyDispatchTaskRsp,
	buflen int64,
	savefile bool,
	sandbox *syscall.Sandbox) error {
	blog.Debugf("decode bk-common-disptach response buf")

	datalen := buflen

	checkMd5 := false
	if savefile {
		if env.GetEnv(env.KeyCommonCheckMd5) == "true" {
			checkMd5 = true
		}
	}

	var bufstartoffset int64
	var bufendoffset int64
	for _, r := range body.GetResults() {
		for _, rf := range r.GetResultfiles() {
			compressedsize := rf.GetCompressedsize()
			if compressedsize > int64(datalen)-bufendoffset {
				err := fmt.Errorf("received buf is not complete, expected[%d], left[%d]",
					compressedsize, int64(datalen)-bufendoffset)
				blog.Warnf("%v", err)
				return err
			}
			bufstartoffset = bufendoffset
			bufendoffset += compressedsize

			if !savefile {
				rf.Buffer = data[bufstartoffset:bufendoffset]
			} else {
				if err := saveResultFile(rf, data[bufstartoffset:bufendoffset], sandbox); err != nil {
					blog.Warnf("failed to save file [%s], err:%v", rf.String(), err)
					return err
				}

				srcMd5 := rf.GetMd5()
				if checkMd5 && srcMd5 != "" {
					curMd5, _ := dcFile.Stat(rf.GetFullpath()).Md5()
					if srcMd5 != curMd5 {
						return fmt.Errorf("file:%s, src md5:%s, received md5:%s",
							rf.GetFullpath(), srcMd5, curMd5)
					}
				}
			}
		}
	}

	return nil
}

func decodeSendFileRspLongTCP(data []byte) (*protocol.PBBodySendFileRsp, error) {
	blog.Debugf("decode data to send file response")

	// decode head
	head, offset, err := decodeCommonHead(data)
	if err != nil {
		return nil, err
	}

	if head.GetCmdtype() != protocol.PBCmdType_SENDFILERSP {
		err := fmt.Errorf("unknown cmd type %v", head.GetCmdtype())
		blog.Warnf("%v", err)
		return nil, err
	}

	bodylen := head.GetBodylen()
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < offset+int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d with head len:%d total len:%d",
			bodylen, buflen, offset, datalen)
		blog.Warnf("%v", err)
		return nil, err
	}
	blog.Infof("after docode data got headlen %d bodylen %d buflen %d totallen %d", offset, bodylen, buflen, datalen)

	// decode body
	body := protocol.PBBodySendFileRsp{}
	err = proto.Unmarshal(data[offset:offset+int(bodylen)], &body)

	if err != nil {
		blog.Warnf("decode pbbody failed with error: %v", err)
	} else {
		blog.Debugf("succeed to decode pbbody ")
	}

	return &body, nil
}

func decodeCheckCacheRspLongTCP(data []byte) (*protocol.PBBodyCheckCacheRsp, error) {
	blog.Debugf("decode check cache response now")

	// decode head
	head, offset, err := decodeCommonHead(data)
	if err != nil {
		return nil, err
	}

	if head.GetCmdtype() != protocol.PBCmdType_CHECKCACHERSP {
		err := fmt.Errorf("unknown cmd type %v", head.GetCmdtype())
		blog.Warnf("%v", err)
		return nil, err
	}

	bodylen := head.GetBodylen()
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < offset+int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d with head len:%d total len:%d",
			bodylen, buflen, offset, datalen)
		blog.Warnf("%v", err)
		return nil, err
	}
	blog.Infof("after docode data got headlen %d bodylen %d buflen %d totallen %d", offset, bodylen, buflen, datalen)

	// decode body
	body := protocol.PBBodyCheckCacheRsp{}
	err = proto.Unmarshal(data[offset:offset+int(bodylen)], &body)

	if err != nil {
		blog.Warnf("decode pbbody failed with error: %v", err)
	} else {
		blog.Debugf("succeed to decode pbbody ")
	}

	return &body, nil
}

func decodeSynctimeRspLongTCP(data []byte) (*protocol.PBBodySyncTimeRsp, error) {
	blog.Debugf("decode sync time response now")

	// decode head
	head, offset, err := decodeCommonHead(data)
	if err != nil {
		return nil, err
	}

	if head.GetCmdtype() != protocol.PBCmdType_SYNCTIMERSP {
		err := fmt.Errorf("unknown cmd type %v", head.GetCmdtype())
		blog.Warnf("%v", err)
		return nil, err
	}

	bodylen := head.GetBodylen()
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < offset+int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d with head len:%d total len:%d",
			bodylen, buflen, offset, datalen)
		blog.Warnf("%v", err)
		return nil, err
	}
	blog.Infof("after docode data got headlen %d bodylen %d buflen %d totallen %d", offset, bodylen, buflen, datalen)

	// decode body
	body := protocol.PBBodySyncTimeRsp{}
	err = proto.Unmarshal(data[offset:offset+int(bodylen)], &body)

	if err != nil {
		blog.Warnf("decode pbbody failed with error: %v", err)
	} else {
		blog.Debugf("succeed to decode pbbody ")
	}

	return &body, nil
}
