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
	"time"

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
	blog.Infof("ready get tcp session with server %s (%s:%d) with long tcp",
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
	ret := session.Send(reqdata, true, int32(r.ioTimeout), func() error {
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
	blog.Infof("remotehandle: start send %d files to server %s", len(req.Files), server)
	r.recordStats.RemoteWorker = server.Server

	if len(req.Files) == 0 {
		blog.Infof("no files need sent")
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
	var locksize int64
	memorylocked := false
	t1 := time.Now()
	t2 := t1
	tinit := time.Now()

	var err error
	var dlocallock, dencodereq, dmemorylock time.Duration
	messages := req.Messages

	// 如果只有一个文件，且已经在缓存里了，则跳过内存锁和本地锁
	onefileincache := false
	if len(req.Files) == 1 && r.fileCache != nil && messages == nil {
		_, err := r.fileCache.Query(req.Files[0].FilePath, false)
		if err == nil {
			onefileincache = true
		}
	}

	if onefileincache {
		// 有可能在这时，缓存被清理了，先忽略
		messages, err = encodeSendFileReq(req, sandbox, r.fileCache)
		blog.Infof("remotehandle: encode file %s without memory lock", req.Files[0].FilePath)
	}

	if messages == nil {
		// 加本地资源锁
		locallocked := false
		if mgr != nil {
			if mgr.LockSlots(dcSDK.JobUsageDefault, 1) {
				locallocked = true
				blog.Infof("remotehandle: succeed to get one default lock")
			}
		}

		t2 = time.Now()
		dlocallock = t2.Sub(t1)
		t1 = t2

		if r.slot != nil && messages == nil {
			for _, v := range req.Files {
				totalsize += v.FileSize
			}
			// 考虑到文件需要读到内存，然后压缩，以及后续的pb协议打包，需要的内存大小至少是两倍
			// r.fileCache 有内存检查，如果打开了该选项，限制可以放松点，加快速度
			if r.fileCache == nil {
				locksize = totalsize * 3
			} else {
				locksize = totalsize
			}
			if locksize > 0 {
				if r.slot.Lock(locksize) {
					memorylocked = true
					blog.Infof("remotehandle: succeed to get memory lock with size %d", totalsize)
				}
			}
		}

		defer func() {
			if memorylocked {
				r.slot.Unlock(locksize)
				blog.Infof("remotehandle: succeed to release memory lock with size %d", totalsize)
				memorylocked = false
			}
		}()
		t2 = time.Now()
		dmemorylock = t2.Sub(t1)
		t1 = t2

		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkPackCommonStartTime)
		messages, err = encodeSendFileReq(req, sandbox, r.fileCache)
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkPackCommonEndTime)

		t2 = time.Now()
		dencodereq = t2.Sub(t1)
		t1 = t2

		if locallocked {
			mgr.UnlockSlots(dcSDK.JobUsageDefault, 1)
			blog.Infof("remotehandle: succeed to release one default lock")
		}

		if err != nil {
			blog.Warnf("remotehandle: error: %v", err)
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

	blog.Infof("remotehandle: success connect to server %s", server)

	reqdata := [][]byte{}
	for _, m := range messages {
		reqdata = append(reqdata, m.Data)
	}

	var dsend time.Duration
	ret := session.Send(reqdata, true, int32(r.ioTimeout), func() error {
		t2 = time.Now()
		dsend = t2.Sub(t1)
		t1 = t2
		dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonEndTime)
		if memorylocked {
			r.slot.Unlock(locksize)
			blog.Infof("remotehandle: succeed to release memory lock with size %d", locksize)
			memorylocked = false
		}
		return nil
	})
	if ret.Err != nil {
		blog.Warnf("remotehandle: send file failed with error: %v", ret.Err)
		return nil, ret.Err
	}

	t2 = time.Now()
	drecv := t2.Sub(t1)
	t1 = t2

	debug.FreeOSMemory() // free memory anyway

	blog.Infof("remotehandle: success sent to server %s", server)
	data, err := decodeSendFileRspLongTCP(ret.Data)

	if err != nil {
		blog.Warnf("remotehandle: error: %v", err)
		return nil, err
	}

	result, err := decodeSendFileRsp(data)
	if err != nil {
		blog.Warnf("remotehandle: error: %v", err)
		return nil, err
	}

	t2 = time.Now()
	ddecode := t2.Sub(t1)
	t1 = t2

	dtotal := t2.Sub(tinit)

	blog.Infof("remotehandle: longtcp send common file stat, total %d files size:%d server:%s "+
		"memory lock : %f , local lock : %f , "+
		"encode req : %f , send : %f , recv : %f , "+
		"decode response : %f , total : %f ",
		len(req.Files), totalsize, server.Server,
		dmemorylock.Seconds(), dlocallock.Seconds(),
		dencodereq.Seconds(), dsend.Seconds(), drecv.Seconds(),
		ddecode.Seconds(), dtotal.Seconds())

	// blog.Debugf("remotehandle: send file task done *")

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
	ret := session.Send(reqdata, true, int32(r.ioTimeout), func() error {
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
	ret := session.Send(reqdata, true, int32(r.ioTimeout), nil)
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
