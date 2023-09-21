/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package protocol

import (
	"fmt"

	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/cache"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"

	"github.com/gogo/protobuf/proto"
)

// --------------------------for long tcp----------------------------------------------------

var (
	ErrorNotEnoughtBufData = fmt.Errorf("buf data of file is not enought")
)

func DecodeBKCommonHead(data []byte) (*protocol.PBHead, int, error) {
	blog.Debugf("decode bk-common-disptach head now")

	offset := 0
	datalen := len(data)
	if datalen < protocol.TOKENBUFLEN {
		blog.Warnf("data length is less than protocol.TOKENBUFLEN")
		return nil, 0, fmt.Errorf("data length is less than protocol.TOKENBUFLEN")
	}

	// resolve head token
	headlen, _, err := readTokenInt(data[0:datalen], protocol.TOEKNHEADFLAG)
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

//
func DecodeBKCommonDispatchReq(
	data []byte,
	head *protocol.PBHead,
	basedir string,
	callback PathMapping,
	c chan<- string) (*protocol.PBBodyDispatchTaskReq, error) {
	blog.Debugf("decode bk-common-disptach body now")

	bodylen := head.GetBodylen()
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d,total data len %d", bodylen, buflen, datalen)
		blog.Warnf("%v", err)
		return nil, err
	}

	// TODO : should by cmd type here
	body := protocol.PBBodyDispatchTaskReq{}
	err := proto.Unmarshal(data[0:bodylen], &body)

	if err != nil {
		blog.Warnf("failed to decode pbbody error: %v", err)
	} else {
		blog.Debugf("succeed to decode pbbody [%s]", body.String())
	}

	// receive buf and save to files
	if buflen >= 0 {
		if err := DecodeBKCommonDispatchReqBuf(data[bodylen:datalen], &body, buflen, basedir, callback, c); err != nil {
			return nil, err
		}
	}

	return &body, nil
}

func DecodeBKCommonDispatchReqBuf(
	data []byte,
	body *protocol.PBBodyDispatchTaskReq,
	buflen int64,
	basedir string,
	callback PathMapping,
	c chan<- string) error {
	blog.Debugf("decode bk-common-disptach request buf now")

	datalen := len(data)

	var bufstartoffset int64
	var bufendoffset int64
	var err error
	for _, r := range body.GetCmds() {
		for _, rf := range r.GetInputfiles() {
			// do not need data buf for empty file
			if rf.GetCompressedsize() <= 0 {
				_, _ = saveFile(rf, nil, basedir, callback, c)
				continue
			}

			compressedsize := rf.GetCompressedsize()
			if compressedsize > int64(datalen)-bufendoffset {
				blog.Warnf("not enought buf data for file [%s]", rf.String())
				return ErrorNotEnoughtBufData
			}
			bufstartoffset = bufendoffset
			bufendoffset += compressedsize
			realfilepath := ""
			if realfilepath, err = saveFile(
				rf, data[bufstartoffset:bufendoffset], basedir, callback, c); err != nil {
				blog.Warnf("failed to save file [%s], err:%v", rf.String(), err)
				return err
			}

			blog.Debugf("succeed to save file:%s", rf.GetFullpath())

			// check md5
			srcMd5 := rf.GetMd5()
			if srcMd5 != "" {
				curMd5, _ := dcFile.Stat(realfilepath).Md5()
				blog.Infof("md5:%s for file:%s", curMd5, realfilepath)
				if srcMd5 != curMd5 {
					blog.Warnf("failed to save file [%s], srcMd5:%s,curmd5:%s", rf.String(), srcMd5, curMd5)
					return err
				}
			}
		}
	}

	return nil
}

//
func DecodeBKSendFile(
	data []byte,
	head *protocol.PBHead,
	basedir string,
	callback PathMapping,
	c chan<- string,
	cm cache.Manager) (*protocol.PBBodySendFileReq, error) {
	blog.Debugf("decode bk-common-disptach body now")

	bodylen := head.GetBodylen()
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d,total data len %d", bodylen, buflen, datalen)
		blog.Warnf("%v", err)
		return nil, err
	}

	// TODO : should by cmd type here
	body := protocol.PBBodySendFileReq{}
	err := proto.Unmarshal(data[0:bodylen], &body)

	if err != nil {
		blog.Warnf("failed to decode pbbody error: %v", err)
	} else {
		blog.Debugf("succeed to decode pbbody [%s]", body.String())
	}

	// receive buf and save to files
	if buflen >= 0 {
		if err := DecodeBKSendFileBuf(data[bodylen:datalen], &body, buflen, basedir, callback, c, cm); err != nil {
			return nil, err
		}
	}

	return &body, nil
}

func DecodeBKSendFileBuf(
	data []byte,
	body *protocol.PBBodySendFileReq,
	buflen int64,
	basedir string,
	callback PathMapping,
	c chan<- string,
	cm cache.Manager) error {
	blog.Debugf("decode send file request buf now")

	datalen := len(data)
	var bufstartoffset int64
	var bufendoffset int64
	var err error

	for _, rf := range body.GetInputfiles() {
		// do not need data buf for empty file
		if rf.GetCompressedsize() <= 0 {
			_, _ = saveFile(rf, nil, basedir, callback, c)
			continue
		}

		compressedsize := rf.GetCompressedsize()
		if compressedsize > int64(datalen)-bufendoffset {
			blog.Warnf("not enought buf data for file [%s]", rf.String())
			return err
		}
		bufstartoffset = bufendoffset
		bufendoffset += compressedsize
		realfilepath := ""
		if realfilepath, err = saveFile(rf, data[bufstartoffset:bufendoffset], basedir, callback, c); err != nil {
			blog.Warnf("failed to save file [%s], err:%v", rf.String(), err)
			return err
		}

		blog.Debugf("succeed to save file:%s", rf.GetFullpath())

		// check md5
		srcMd5 := rf.GetMd5()
		if srcMd5 != "" {
			curMd5, _ := dcFile.Stat(realfilepath).Md5()
			blog.Infof("md5:%s for file:%s", curMd5, realfilepath)
			if srcMd5 != curMd5 {
				blog.Warnf("failed to save file [%s], srcMd5:%s,curmd5:%s", rf.String(), srcMd5, curMd5)
				return err
			}
		}

		// try store file to cache
		go storeFile2Cache(cm, realfilepath)
	}

	return nil
}

// ReceiveBKCheckCache receive check cache request and generate the body
func DecodeBKCheckCache(
	data []byte,
	head *protocol.PBHead,
	_ string,
	_ PathMapping,
	_ chan<- string) (*protocol.PBBodyCheckCacheReq, error) {
	blog.Debugf("decode check cache body now")

	bodylen := head.GetBodylen()
	buflen := head.GetBuflen()
	datalen := len(data)
	if bodylen <= 0 || buflen < 0 || datalen < int(bodylen)+int(buflen) {
		err := fmt.Errorf("get invalid body length %d, buf len %d,total data len %d", bodylen, buflen, datalen)
		blog.Warnf("%v", err)
		return nil, err
	}

	// TODO : should by cmd type here
	body := protocol.PBBodyCheckCacheReq{}
	err := proto.Unmarshal(data[0:bodylen], &body)

	if err != nil {
		blog.Warnf("failed to decode pbbody error: %v", err)
	} else {
		blog.Debugf("succeed to decode pbbody")
	}

	return &body, nil
}

// ReceiveUnknown to receive pb command body for unknown cmd
func DecodeUnknown(
	data []byte,
	head *protocol.PBHead,
	_ string,
	_ PathMapping) error {

	bodylen := head.GetBodylen()
	buflen := head.GetBuflen()

	blog.Debugf("received unknown cmd %s with body len %d and buf len %d", head.GetCmdtype(), bodylen, buflen)

	return nil
}
