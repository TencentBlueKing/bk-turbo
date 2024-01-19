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
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/longtcp"
	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// --------------------------for long tcp----------------------------------------------------

// ExecuteTaskWithoutSaveFileLongTCP same as ExecuteTaskWithoutSaveFile but with long tcp connection
func (r *CommonRemoteHandler) ExecuteTaskWithoutSaveFileLongTCP(
	server *dcProtocol.Host,
	req *dcSDK.BKDistCommand) (*dcSDK.BKDistResult, error) {
	return r.executeTaskLongTCP(server, req, false)
}

// ExecuteTaskLongTCP same as ExecuteTask but with long tcp connection
func (r *CommonRemoteHandler) ExecuteTaskLongTCP(
	server *dcProtocol.Host,
	req *dcSDK.BKDistCommand) (*dcSDK.BKDistResult, error) {
	return r.executeTaskLongTCP(server, req, true)
}

// ExecuteTaskWithoutSaveFileLongTCP same as ExecuteTaskWithoutSaveFile but with long tcp connection
func (r *CommonRemoteHandler) getTCPSession(
	server string) (*longtcp.Session, error) {
	ip := ""
	var port int
	// The port starts after the last colon.
	i := strings.LastIndex(server, ":")
	if i > 0 && i < len(server)-1 {
		ip = server[:i]
		port, _ = strconv.Atoi(server[i+1:])
	}
	blog.Infof("ready execute remote task to server %s (%s:%d) with long tcp",
		server, ip, port)

	sp := longtcp.GetGlobalSessionPool(ip, int32(port), r.ioTimeout, encodeLongTCPHandshakeReq, 48, nil)
	session, err := sp.GetSession()
	if err != nil && session == nil {
		blog.Warnf("get tcp session failed with error: %v", err)
		return nil, err
	}

	return session, nil
}

// ExecuteTaskWithoutSaveFileLongTCP same as ExecuteTaskWithoutSaveFile but with long tcp connection
func (r *CommonRemoteHandler) executeTaskLongTCP(
	server *dcProtocol.Host,
	req *dcSDK.BKDistCommand,
	savefile bool) (*dcSDK.BKDistResult, error) {
	// record the exit status.
	defer func() {
		r.updateJobStatsFunc()
	}()
	blog.Debugf("execute remote task with server %s and do not save file", server)
	r.recordStats.RemoteWorker = server.Server

	session, err := r.getTCPSession(server.Server)
	if err != nil && session == nil {
		blog.Warnf("get tcp session failed with error: %v", err)
		return nil, err
	}

	blog.Debugf("protocol: execute dist task commands: %v", req.Commands)
	r.recordStats.RemoteWorkTimeoutSec = r.ioTimeout

	// var err error
	messages := req.Messages

	// if raw messages not specified, then generate message from commands
	if messages == nil {
		// compress and prepare request
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkPackStartTime)
		// record the pack starting status, packing should be waiting for a while.
		r.updateJobStatsFunc()
		messages, err = EncodeCommonDistTask(req)
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkPackEndTime)
		if err != nil {
			blog.Warnf("error: %v", err)
			return nil, err
		}
	}

	// send request
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendStartTime)
	// record the send starting status, sending should be waiting for a while.
	r.updateJobStatsFunc()

	reqdata := [][]byte{}
	for _, m := range messages {
		reqdata = append(reqdata, m.Data)
	}
	ret := session.Send(reqdata, true, func() error {
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendEndTime)
		return nil
	})
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkReceiveEndTime)

	if ret.Err != nil {
		r.recordStats.RemoteWorkFatal = true
		blog.Warnf("execute remote task with long tcp failed with error: %v", ret.Err)
		return nil, ret.Err
	}
	blog.Infof("execute remote task to server %s with long tcp got id:%s data length:%d",
		server, ret.TCPHead.UniqID, ret.TCPHead.DataLen)

	debug.FreeOSMemory() // free memory anyway

	// record the receive starting status, receiving should be waiting for a while.
	r.updateJobStatsFunc()
	// decode data to result
	data, err := decodeCommonDispatchRspLongTCP(ret.Data, savefile, r.sandbox)
	// dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkReceiveEndTime)
	r.recordStatsFromDispatchResp(data, server)

	if err != nil {
		r.recordStats.RemoteWorkFatal = true
		r.checkIfIOTimeout(err)
		blog.Warnf("error: %v", err)
		return nil, err
	}

	debug.FreeOSMemory() // free memory anyway

	// get stat data from response here and decompress
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkUnpackStartTime)
	// record the decode starting status, decoding should be waiting for a while.
	r.updateJobStatsFunc()
	result, err := decodeCommonDispatchRsp(data)
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkUnpackEndTime)
	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	blog.Debugf("remote task done *")
	r.recordStats.RemoteWorkSuccess = true
	return result, nil
}

func (r *CommonRemoteHandler) ExecuteSendFileLongTCP(
	server *dcProtocol.Host,
	req *dcSDK.BKDistFileSender,
	sandbox *syscall.Sandbox,
	mgr sdk.LockMgr) (*dcSDK.BKSendFileResult, error) {
	// record the exit status.
	defer func() {
		r.updateJobStatsFunc()
	}()
	blog.Debugf("start send files to server %s", server)
	r.recordStats.RemoteWorker = server.Server

	if len(req.Files) == 0 {
		blog.Debugf("no files need sent")
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkPackCommonStartTime)
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkPackCommonEndTime)
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonStartTime)
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonEndTime)
		return &dcSDK.BKSendFileResult{
			Results: []dcSDK.FileResult{{RetCode: 0}},
		}, nil
	}

	// 加内存锁
	var totalsize int64
	memorylocked := false
	if r.slot != nil {
		for _, v := range req.Files {
			totalsize += v.FileSize
		}
		if r.slot.Lock(totalsize) {
			memorylocked = true
			blog.Debugf("remotehandle: succeed to get lock with size %d", totalsize)
		}
	}

	defer func() {
		if memorylocked {
			r.slot.Unlock(totalsize)
			blog.Debugf("remotehandle: succeed to release lock with size %d", totalsize)
			memorylocked = false
		}
	}()

	var err error
	messages := req.Messages
	if messages == nil {
		// 加本地资源锁
		locallocked := false
		if mgr != nil {
			if mgr.LockSlots(dcSDK.JobUsageLocalExe, 1) {
				locallocked = true
				blog.Debugf("remotehandle: succeed to get one local lock")
			}
		}
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkPackCommonStartTime)
		messages, err = encodeSendFileReq(req, sandbox)
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkPackCommonEndTime)

		if locallocked {
			mgr.UnlockSlots(dcSDK.JobUsageLocalExe, 1)
			blog.Debugf("remotehandle: succeed to release one local lock")
		}

		if err != nil {
			blog.Warnf("error: %v", err)
			return nil, err
		}
	}

	debug.FreeOSMemory() // free memory anyway

	blog.Debugf("success pack-up to server %s", server)

	// send request
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonStartTime)
	// record the send starting status, sending should be waiting for a while.
	r.updateJobStatsFunc()

	session, err := r.getTCPSession(server.Server)
	if err != nil && session == nil {
		blog.Warnf("get tcp session failed with error: %v", err)
		return nil, err
	}

	blog.Debugf("success connect to server %s", server)

	reqdata := [][]byte{}
	for _, m := range messages {
		reqdata = append(reqdata, m.Data)
	}
	ret := session.Send(reqdata, true, func() error {
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonEndTime)
		if memorylocked {
			r.slot.Unlock(totalsize)
			blog.Debugf("remotehandle: succeed to release lock with size %d", totalsize)
			memorylocked = false
		}
		return nil
	})
	if ret.Err != nil {
		blog.Warnf("send file failed with error: %v", ret.Err)
		return nil, ret.Err
	}

	debug.FreeOSMemory() // free memory anyway

	blog.Debugf("success sent to server %s", server)
	data, err := decodeSendFileRspLongTCP(ret.Data)

	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	result, err := decodeSendFileRsp(data)
	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	blog.Debugf("send file task done *")

	return result, nil
}

func (r *CommonRemoteHandler) ExecuteCheckCacheLongTCP(
	server *dcProtocol.Host,
	req *dcSDK.BKDistFileSender,
	sandbox *syscall.Sandbox) ([]bool, error) {
	blog.Debugf("start check cache to server %s", server)

	session, err := r.getTCPSession(server.Server)
	if err != nil && session == nil {
		blog.Warnf("get tcp session failed with error: %v", err)
		return nil, err
	}

	messages, err := encodeCheckCacheReq(req, sandbox)
	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	reqdata := [][]byte{}
	for _, m := range messages {
		reqdata = append(reqdata, m.Data)
	}
	ret := session.Send(reqdata, true, func() error {
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonEndTime)
		return nil
	})
	if ret.Err != nil {
		blog.Warnf("error: %v", ret.Err)
		return nil, ret.Err
	}

	blog.Debugf("check cache success sent to server %s", server)

	// receive result
	data, err := decodeCheckCacheRspLongTCP(ret.Data)

	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	result, err := decodeCheckCacheRsp(data)
	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	blog.Debugf("check cache task done *")

	return result, nil
}

func (r *CommonRemoteHandler) ExecuteSyncTimeLongTCP(server string) (int64, error) {
	blog.Debugf("start sync time to server %s", server)

	session, err := r.getTCPSession(server)
	if err != nil && session == nil {
		blog.Warnf("get tcp session failed with error: %v", err)
		return 0, err
	}

	// compress and prepare request
	messages, err := encodeSynctimeReq()
	if err != nil {
		blog.Warnf("error: %v", err)
		return 0, err
	}

	// send request
	reqdata := [][]byte{}
	for _, m := range messages {
		reqdata = append(reqdata, m.Data)
	}
	ret := session.Send(reqdata, true, nil)
	if ret.Err != nil {
		blog.Warnf("error: %v", ret.Err)
		return 0, ret.Err
	}

	// receive result
	rsp, err := decodeSynctimeRspLongTCP(ret.Data)

	if err != nil {
		blog.Warnf("error: %v", err)
		return 0, err
	}

	blog.Debugf("remote task done, get remote time:%d", rsp.GetTimenanosecond())
	return rsp.GetTimenanosecond(), nil
}
