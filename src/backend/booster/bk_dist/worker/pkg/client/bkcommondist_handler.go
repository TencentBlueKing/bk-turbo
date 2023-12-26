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
	"context"
	"net"
	"runtime/debug"
	"strings"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// NewCommonRemoteWorkerWithSlot get a new remote worker SDK with specifiled size of slot
func NewCommonRemoteWorkerWithSlot(ctx context.Context, size int64) dcSDK.RemoteWorker {
	return &RemoteWorker{
		slot: newSlot(ctx, size),
	}
}

// NewCommonRemoteWorker get a new remote worker SDK
func NewCommonRemoteWorker() dcSDK.RemoteWorker {
	return &RemoteWorker{}
}

// RemoteWorker 作为链接管理单元, 通过实例化不同的handler来提供参数隔离的服务,
// 但最终的连接池都是用的同一个
// TODO: 统一管理连接池
type RemoteWorker struct {
	slot *slot
}

// Handler get a remote handler
func (rw *RemoteWorker) Handler(
	ioTimeout int,
	stats *dcSDK.ControllerJobStats,
	updateJobStatsFunc func(),
	sandbox *syscall.Sandbox) dcSDK.RemoteWorkerHandler {
	if stats == nil {
		stats = &dcSDK.ControllerJobStats{}
	}

	if updateJobStatsFunc == nil {
		updateJobStatsFunc = func() {}
	}

	if sandbox == nil {
		sandbox = &syscall.Sandbox{}
	}

	return &CommonRemoteHandler{
		parent:             rw,
		sandbox:            sandbox,
		recordStats:        stats,
		updateJobStatsFunc: updateJobStatsFunc,
		ioTimeout:          ioTimeout,
		slot:               rw.slot,
	}
}

// CommonRemoteHandler remote executor for bk-common
type CommonRemoteHandler struct {
	parent             *RemoteWorker
	sandbox            *syscall.Sandbox
	recordStats        *dcSDK.ControllerJobStats
	updateJobStatsFunc func()
	ioTimeout          int
	slot               *slot
}

func getRealServer(server string) string {
	if strings.Count(server, ":") >= 2 { //ipv6 real ip
		// The port starts after the last colon.
		i := strings.LastIndex(server, ":")
		return "[" + server[:i] + "]" + server[i:]
	}
	return server //ipv4
}

// ExecuteSyncTime get the target server's current timestamp
func (r *CommonRemoteHandler) ExecuteSyncTime(server string) (int64, error) {
	client := NewTCPClient(r.ioTimeout)
	if err := client.Connect(getRealServer(server)); err != nil {
		blog.Warnf("error: %v", err)
		return 0, err
	}
	defer func() {
		_ = client.Close()
	}()

	// compress and prepare request
	messages, err := encodeSynctimeReq()
	if err != nil {
		blog.Warnf("error: %v", err)
		return 0, err
	}

	// send request
	err = sendMessages(client, messages)
	if err != nil {
		blog.Warnf("error: %v", err)
		return 0, err
	}

	// receive result
	rsp, err := receiveSynctimeRsp(client)

	if err != nil {
		blog.Warnf("error: %v", err)
		return 0, err
	}

	blog.Debugf("remote task done, get remote time:%d", rsp.GetTimenanosecond())
	return rsp.GetTimenanosecond(), nil
}

// ExecuteTask do execution in remote and get back the result(and files)
func (r *CommonRemoteHandler) ExecuteTask(
	server *dcProtocol.Host,
	req *dcSDK.BKDistCommand) (*dcSDK.BKDistResult, error) {
	// record the exit status.
	defer func() {
		r.updateJobStatsFunc()
	}()
	blog.Debugf("execute remote task with server %s", server)
	r.recordStats.RemoteWorker = server.Server

	client := NewTCPClient(r.ioTimeout)
	if err := client.Connect(getRealServer(server.Server)); err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}
	defer func() {
		_ = client.Close()
	}()

	blog.Debugf("protocol: execute dist task commands: %v", req.Commands)
	r.recordStats.RemoteWorkTimeoutSec = client.timeout

	var err error
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
	err = sendMessages(client, messages)
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendEndTime)
	if err != nil {
		r.recordStats.RemoteWorkFatal = true
		r.checkIfIOTimeout(err)
		blog.Warnf("error: %v", err)
		return nil, err
	}

	// record the receive starting status, receiving should be waiting for a while.
	r.updateJobStatsFunc()
	// receive result
	data, err := receiveCommonDispatchRsp(client, r.sandbox)
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkReceiveEndTime)
	r.recordStatsFromDispatchResp(data, server)

	if err != nil {
		r.recordStats.RemoteWorkFatal = true
		r.checkIfIOTimeout(err)
		blog.Warnf("error: %v", err)
		return nil, err
	}

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

// ExecuteTaskWithoutSaveFile same as ExecuteTask but do not write file to disk directly,
// the result file will be kept in memory and wait for custom process
func (r *CommonRemoteHandler) ExecuteTaskWithoutSaveFile(
	server *dcProtocol.Host,
	req *dcSDK.BKDistCommand) (*dcSDK.BKDistResult, error) {
	// record the exit status.
	defer func() {
		r.updateJobStatsFunc()
	}()
	blog.Debugf("execute remote task with server %s and do not save file", server)
	r.recordStats.RemoteWorker = server.Server

	client := NewTCPClient(r.ioTimeout)
	if err := client.Connect(getRealServer(server.Server)); err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}
	defer func() {
		_ = client.Close()
	}()

	blog.Debugf("protocol: execute dist task commands: %v", req.Commands)
	r.recordStats.RemoteWorkTimeoutSec = client.timeout

	var err error
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
	err = sendMessages(client, messages)
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendEndTime)
	if err != nil {
		r.recordStats.RemoteWorkFatal = true
		blog.Warnf("error: %v", err)
		return nil, err
	}

	debug.FreeOSMemory() // free memory anyway

	// record the receive starting status, receiving should be waiting for a while.
	r.updateJobStatsFunc()
	// receive result
	data, err := receiveCommonDispatchRspWithoutSaveFile(client)
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkReceiveEndTime)
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

func (r *CommonRemoteHandler) checkIfIOTimeout(remoteErr error) {
	if remoteErr == nil {
		return
	}

	if err, ok := remoteErr.(net.Error); ok && err.Timeout() {
		r.recordStats.RemoteWorkTimeout = true
	}
}

func (r *CommonRemoteHandler) recordStatsFromDispatchResp(
	resp *protocol.PBBodyDispatchTaskRsp,
	server *dcProtocol.Host) {
	if resp == nil {
		return
	}

	if len(resp.Results) == 0 {
		return
	}

	var delta int64 = 0
	if server != nil {
		delta = server.TimeDelta
		blog.Debugf("server(%s) delta time: %d", server.Server, server.TimeDelta)
	}

	result := resp.Results[0]
	for _, s := range result.Stats {
		if s.Key == nil || s.Time == nil {
			continue
		}

		switch *s.Key {
		case protocol.BKStatKeyStartTime:
			r.recordStats.RemoteWorkProcessStartTime = dcSDK.StatsTime(time.Unix(0, *s.Time-delta).Local())
		case protocol.BKStatKeyEndTime:
			r.recordStats.RemoteWorkProcessEndTime = dcSDK.StatsTime(time.Unix(0, *s.Time-delta).Local())
			r.recordStats.RemoteWorkReceiveStartTime = dcSDK.StatsTime(time.Unix(0, *s.Time-delta).Local())
		}
	}
}

// EncodeCommonDistTask encode request command info into protocol.Message
func EncodeCommonDistTask(req *dcSDK.BKDistCommand) ([]protocol.Message, error) {
	blog.Debugf("encodeBKCommonDistTask now")

	// ++ by tomtian for debug
	// debugRecordFileName(req)
	// --

	return encodeCommonDispatchReq(req)
}

// EncodeSendFileReq encode request files into protocol.Message
func EncodeSendFileReq(req *dcSDK.BKDistFileSender, sandbox *syscall.Sandbox) ([]protocol.Message, error) {
	blog.Debugf("encodeBKCommonDistFiles now")

	return encodeSendFileReq(req, sandbox)
}

// ExecuteSendFile send files to remote server
func (r *CommonRemoteHandler) ExecuteSendFile(
	server *dcProtocol.Host,
	req *dcSDK.BKDistFileSender,
	sandbox *syscall.Sandbox,
	mgr dcSDK.LockMgr) (*dcSDK.BKSendFileResult, error) {

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
	var locksize int64
	memorylocked := false
	if r.slot != nil {
		for _, v := range req.Files {
			totalsize += v.FileSize
		}
		// 考虑到文件需要读到内存，然后压缩，以及后续的pb协议打包，需要的内存大小至少是两倍
		locksize = totalsize * 3
		if r.slot.Lock(locksize) {
			memorylocked = true
			blog.Debugf("remotehandle: succeed to get one memory lock")
		}
	}

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

			if memorylocked {
				r.slot.Unlock(locksize)
				blog.Debugf("remotehandle: succeed to release one memory lock")
			}

			return nil, err
		}
	}

	debug.FreeOSMemory() // free memory anyway

	blog.Debugf("success pack-up to server %s", server)

	blog.Debugf("remote: finished encode request for send cork %d files with size:%d to server %s",
		len(req.Files), totalsize, server.Server)

	// send request
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonStartTime)
	// record the send starting status, sending should be waiting for a while.
	r.updateJobStatsFunc()

	// ready to send now
	t := time.Now().Local()
	client := NewTCPClient(r.ioTimeout)
	if err := client.Connect(getRealServer(server.Server)); err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}
	d := time.Now().Sub(t)
	if d > 200*time.Millisecond {
		blog.Debugf("TCP Connect to long to server(%s): %s", server.Server, d.String())
	}
	defer func() {
		_ = client.Close()
	}()

	blog.Debugf("success connect to server %s", server)

	err = sendMessages(client, messages)
	if err != nil {
		blog.Warnf("error: %v", err)

		if memorylocked {
			r.slot.Unlock(locksize)
			blog.Debugf("remotehandle: succeed to release one memory lock")
		}

		return nil, err
	}

	blog.Debugf("remote: finished send request for send cork %d files with size:%d to server %s",
		len(req.Files), totalsize, server.Server)

	debug.FreeOSMemory() // free memory anyway

	if memorylocked {
		r.slot.Unlock(locksize)
		blog.Debugf("remotehandle: succeed to release one memory lock")
	}

	blog.Debugf("success sent to server %s", server)
	// receive result
	data, err := receiveSendFileRsp(client)
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonEndTime)

	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	result, err := decodeSendFileRsp(data)
	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	blog.Debugf("remote: finished receive request for send cork %d files with size:%d to server %s",
		len(req.Files), totalsize, server.Server)

	blog.Debugf("send file task done *")

	return result, nil
}

// ExecuteCheckCache check file cache in remote worker
func (r *CommonRemoteHandler) ExecuteCheckCache(
	server *dcProtocol.Host,
	req *dcSDK.BKDistFileSender,
	sandbox *syscall.Sandbox) ([]bool, error) {
	blog.Debugf("start check cache to server %s", server)

	// record the exit status.
	t := time.Now().Local()
	client := NewTCPClient(r.ioTimeout)
	if err := client.Connect(getRealServer(server.Server)); err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}
	d := time.Now().Sub(t)
	if d > 200*time.Millisecond {
		blog.Debugf("TCP Connect to long to server(%s): %s", server.Server, d.String())
	}
	defer func() {
		_ = client.Close()
	}()

	messages, err := encodeCheckCacheReq(req, sandbox)
	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	err = sendMessages(client, messages)
	if err != nil {
		blog.Warnf("error: %v", err)
		return nil, err
	}

	blog.Debugf("check cache success sent to server %s", server)

	// receive result
	data, err := receiveCheckCacheRsp(client)
	dcSDK.StatsTimeNow(&r.recordStats.RemoteWorkSendCommonEndTime)

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
