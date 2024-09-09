/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package remote

import (
	"container/list"
	"context"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/client"
	wkclient "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/client"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

const (
	MaxHightPrioritySlot  = 48
	MaxMiddlePrioritySlot = 48

	SlotTCPTimeoutSeconds = 3600 * 24

	MaxQuerySlotBatchSize = 10

	querySlotIntervalTime = 5 * time.Second
	jobCheckIntervalTime  = 20 * time.Second
	slotCheckIntervalTime = 17 * time.Second

	// 最大slot空闲时间，超过考虑释放
	MaxSlotIdleIntervalTime = 30 * time.Second

	// 最大job空闲时间，超过这个考虑转本地
	MaxJosIdleIntervalTime = 60 * time.Second
	// 最大无slot的时间，和上面结合起来，考虑转本地
	MaxNoSlotSIntervalTime = 30 * time.Second

	RefusedRecoverSeconds = 120
)

type remoteSlotOffer struct {
	Host             *dcProtocol.Host `json:"host"`
	AvailableSlotNum int32            `json:"available_slot_num"`
	ReceivedTime     time.Time        `json:"received_time"`
}

func newWorkerOfferResource(hl []*dcProtocol.Host) *workerOffer {
	wl := make([]*worker, 0, len(hl))
	total := 0
	for _, h := range hl {
		if h.Jobs <= 0 {
			continue
		}

		wl = append(wl, &worker{
			disabled:            false,
			host:                h,
			totalSlots:          h.Jobs,
			occupiedSlots:       0,
			continuousNetErrors: 0,
			dead:                false,
			conn:                nil,
			status:              Init,
			priority:            dcSDK.PriorityUnKnown,
		})
		total += h.Jobs
	}

	blog.Infof("worker offer slot: total slots:%d after new resource", total)

	return &workerOffer{
		lockChan:       make(lockWorkerChan, 1000),
		emptyChan:      make(chan bool, 1000),
		slotChan:       make(chan *dcSDK.BKQuerySlotResult, 1000),
		worker:         wl,
		validWorkerNum: len(hl),

		waitingList: list.New(),
		slotQueried: false,

		lastGetSlotTime:   time.Now(),
		localJosBatchSize: runtime.NumCPU() - 2,
	}
}

type workerOffer struct {
	ctx context.Context

	workerLock sync.RWMutex
	worker     []*worker

	// validWorkerNum 现在用于标识有效的worker数
	validWorkerNum int

	lockChan lockWorkerChan

	// trigger when worker change to empty
	emptyChan chan bool

	handling bool

	// to save waiting requests
	waitingList *list.List

	// 用于接收worker提供的slot offer
	slotChan chan *dcSDK.BKQuerySlotResult

	slotQueried bool

	lastGetSlotTime   time.Time
	localJosBatchSize int
}

// reset with []*dcProtocol.Host
// add new hosts and disable released hosts
func (wo *workerOffer) Reset(hl []*dcProtocol.Host) ([]*dcProtocol.Host, error) {
	blog.Infof("worker offer slot: ready reset with %d host", len(hl))

	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	// 将老的worker的tcp connection都关掉
	for _, v := range wo.worker {
		v.resetSlot("reset workers")
		blog.Infof("worker offer slot: set worker %s priority %d to Init",
			v.host.Server, v.priority)
	}

	wl := make([]*worker, 0, len(hl))
	total := 0
	for _, h := range hl {
		if h.Jobs <= 0 {
			continue
		}

		wl = append(wl, &worker{
			disabled:            false,
			host:                h,
			totalSlots:          h.Jobs,
			occupiedSlots:       0,
			continuousNetErrors: 0,
			dead:                false,
			conn:                nil,
			status:              Init,
			priority:            dcSDK.PriorityUnKnown,
		})
		total += h.Jobs
	}

	blog.Infof("worker offer slot: total slots:%d after reset with new resource", total)

	wo.validWorkerNum = len(hl)
	wo.worker = wl
	wo.slotQueried = false

	wo.lastGetSlotTime = time.Now()
	wo.localJosBatchSize = runtime.NumCPU() - 2

	return hl, nil
}

// brings handler up and begin to handle requests
func (wo *workerOffer) Handle(ctx context.Context) {
	if wo.handling {
		return
	}

	wo.handling = true

	go wo.handle(ctx)
}

// Lock get an usage lock, success with true, failed with false
func (wo *workerOffer) Lock(usage dcSDK.JobUsage, f string, banWorkerList []*dcProtocol.Host) *dcProtocol.Host {
	if !wo.handling {
		return nil
	}

	msg := lockWorkerMessage{
		jobUsage:      usage,
		toward:        nil,
		result:        make(chan *dcProtocol.Host, 1),
		largeFile:     f,
		banWorkerList: banWorkerList,
		startWait:     time.Now(),
	}

	// send a new lock request
	wo.lockChan <- msg

	select {
	case <-wo.ctx.Done():
		return &dcProtocol.Host{}

	// wait result
	case h := <-msg.result:
		return h
	}
}

// Unlock release an usage lock
func (wo *workerOffer) Unlock(usage dcSDK.JobUsage, host *dcProtocol.Host) {}

func (wo *workerOffer) TotalSlots() int {
	return wo.validWorkerNum
}

func (wo *workerOffer) DisableWorker(host *dcProtocol.Host) {
	if host == nil {
		return
	}

	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	invalidjobs := 0
	for _, w := range wo.worker {
		if !host.Equal(w.host) {
			continue
		}

		if w.disabled {
			blog.Infof("worker offer slot: host:%v disabled before,do nothing now", *host)
			break
		}

		w.disabled = true
		invalidjobs = w.totalSlots

		w.resetSlot("disable worker")
		blog.Infof("worker offer slot: set worker %s priority %d to Init",
			w.host.Server, w.priority)

		break
	}

	// wo.validWorkerNum may be <= 0
	if invalidjobs > 0 {
		wo.validWorkerNum -= 1
	}

	if wo.validWorkerNum <= 0 {
		wo.emptyChan <- true
	}

	blog.Infof("worker offer slot: total valid worker:%d after disable host:%v", wo.validWorkerNum, *host)
	return
}

func (wo *workerOffer) EnableWorker(host *dcProtocol.Host) {
	if host == nil {
		return
	}
	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	for _, w := range wo.worker {
		if !host.Equal(w.host) {
			continue
		}
		if !w.disabled {
			blog.Infof("remote slot: host:%v enabled before,do nothing now", *host)
			return // already enabled
		}

		w.disabled = false
		w.status = Init
		wo.validWorkerNum += 1
		break
	}

	blog.Infof("remote slot: total slot:%d after enable host:%v", wo.validWorkerNum, *host)
}

func (wo *workerOffer) CanWorkerRetry(host *dcProtocol.Host) bool {
	if host == nil {
		return false
	}

	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	for _, wk := range wo.worker {
		if !wk.host.Equal(host) {
			continue
		}

		if wk.dead {
			blog.Infof("remote slot: host:%v is already dead,do nothing now", host)
			return false
		}

		if wk.status == Retrying {
			blog.Infof("remote slot: host:%v is retrying,do nothing now", host)
			return true
		}
		blog.Info("remote slot: host:%v can retry, change worker from %s to %s", host, wk.status, Retrying)
		wk.status = Retrying
		return false
	}

	return false
}

func (wo *workerOffer) SetWorkerStatus(host *dcProtocol.Host, s Status) {
	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	for _, w := range wo.worker {
		if !host.Equal(w.host) {
			continue
		}

		w.status = s
		break
	}
}

func (wo *workerOffer) WorkerDead(w *worker) {
	if w == nil || w.host == nil {
		return
	}

	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	invalidjobs := 0
	for _, wk := range wo.worker {
		if !wk.host.Equal(w.host) {
			continue
		}

		if wk.dead {
			blog.Infof("worker offer slot: host:%v is already dead,do nothing now", w.host)
			return
		}

		wk.dead = true
		invalidjobs = wk.totalSlots

		wk.resetSlot("worker dead")
		blog.Infof("worker offer slot: set worker %s priority %d to Init",
			wk.host.Server, wk.priority)

		break
	}

	// // !!! wo.validWorkerNum may be <= 0 !!!
	if invalidjobs > 0 {
		wo.validWorkerNum -= 1
	}

	if wo.validWorkerNum <= 0 {
		wo.emptyChan <- true
	}

	blog.Infof("worker offer slot: total valid worker:%d after host is dead:%v", wo.validWorkerNum, w.host)
	return
}

func (wo *workerOffer) DisableAllWorker() {
	blog.Infof("worker offer slot: ready disable all host")

	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	for _, w := range wo.worker {
		if w.disabled {
			continue
		}

		w.disabled = true

		w.resetSlot("disable all workers")
		blog.Infof("worker offer slot: set worker %s priority %d to Init",
			w.host.Server, w.priority)
	}

	wo.worker = make([]*worker, 0)

	wo.validWorkerNum = 0
	if wo.validWorkerNum <= 0 {
		wo.emptyChan <- true
	}

	blog.Infof("worker offer slot: total valid worker:%d after disable all host", wo.validWorkerNum)
	return
}

func (wo *workerOffer) RecoverDeadWorker(w *worker) {
	if w == nil || w.host == nil {
		return
	}
	blog.Infof("worker offer slot: ready enable host(%s)", w.host.Server)

	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	for _, wk := range wo.worker {
		if !w.host.Equal(wk.host) {
			continue
		}

		if !wk.dead {
			blog.Infof("worker offer slot: host:%v is not dead, do nothing now", w.host)
			return
		}

		wk.dead = false
		wk.continuousNetErrors = 0
		wo.validWorkerNum += 1
		break
	}

	blog.Infof("worker offer slot: total valid worker:%d after recover host:%v", wo.validWorkerNum, w.host)
	return
}

func (wo *workerOffer) CountWorkerError(w *worker) {
	if w == nil || w.host == nil {
		return
	}
	blog.Infof("worker offer slot: ready count error from host(%s)", w.host.Server)

	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	for _, wk := range wo.worker {
		if !w.host.Equal(wk.host) {
			continue
		}
		wk.continuousNetErrors++
		break
	}
}

func (wo *workerOffer) GetWorkers() []*worker {
	wo.workerLock.RLock()
	defer wo.workerLock.RUnlock()

	workers := []*worker{}
	for _, w := range wo.worker {
		workers = append(workers, w.copy())
	}
	return workers
}

func (wo *workerOffer) GetDeadWorkers() []*worker {
	wo.workerLock.RLock()
	defer wo.workerLock.RUnlock()

	workers := []*worker{}
	for _, w := range wo.worker {
		if !w.disabled && w.dead {
			workers = append(workers, w.copy())
		}
	}
	return workers
}

func (wo *workerOffer) IsWorkerDead(w *worker, netErrorLimit int) bool {
	wo.workerLock.RLock()
	defer wo.workerLock.RUnlock()

	for _, wk := range wo.worker {
		if !wk.host.Equal(w.host) {
			continue
		}
		if wk.continuousNetErrors >= netErrorLimit {
			return true
		}
		break
	}
	return false
}

func (wo *workerOffer) addWorker(host *dcProtocol.Host) {
	if host == nil || host.Jobs <= 0 {
		return
	}

	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()

	for _, w := range wo.worker {
		if host.Equal(w.host) {
			blog.Infof("worker offer slot: host(%s) existed when add", w.host.Server)
			return
		}
	}

	wo.worker = append(wo.worker, &worker{
		disabled:            false,
		host:                host,
		totalSlots:          host.Jobs,
		occupiedSlots:       0,
		continuousNetErrors: 0,
		dead:                false,
		conn:                nil,
		status:              Init,
	})
	wo.validWorkerNum += 1

	blog.Infof("worker offer slot: total valid worker:%d after add host:%v", wo.validWorkerNum, *host)
	return
}

func (wo *workerOffer) handle(ctx context.Context) {
	wo.ctx = ctx

	tick := time.NewTicker(querySlotIntervalTime)
	defer tick.Stop()

	tick1 := time.NewTicker(jobCheckIntervalTime)
	defer tick1.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-wo.lockChan:
			wo.getSlot(msg)
		case <-wo.emptyChan:
			wo.onSlotEmpty()
		case r := <-wo.slotChan:
			wo.onSlotResult(r)
		case <-tick.C:
			if wo.waitingList.Len() > 0 {
				wo.querySlot()
			}
		case <-tick1.C:
			if wo.waitingList.Len() > 0 {
				wo.jobCheck()
			}
		}
	}
}

func (wo *workerOffer) getSlot(msg lockWorkerMessage) {
	if wo.validWorkerNum <= 0 {
		msg.result <- nil
		return
	}

	wo.waitingList.PushBack(&msg)

	// 第一次发现没有slot
	if !wo.slotQueried {
		wo.slotQueried = true
		blog.Infof("worker offer slot: ready query slot with %d waiting jobs", wo.waitingList.Len())
		wo.querySlot()
	}
}

func (wo *workerOffer) onSlotEmpty() {
	blog.Infof("worker offer slot: on slot empty: valid worker:%d,waiting:%d", wo.validWorkerNum, wo.waitingList.Len())

	for wo.waitingList.Len() > 0 {
		e := wo.waitingList.Front()
		msg := e.Value.(*lockWorkerMessage)
		msg.result <- nil
		wo.waitingList.Remove(e)
	}
}

func (wo *workerOffer) onSlotResult(r *dcSDK.BKQuerySlotResult) error {
	if r == nil {
		blog.Infof("worker offer slot: got nil slot offer")
		return nil
	}

	// update worker status
	wo.workerLock.RLock()
	var targetconn *net.TCPConn
	blog.Infof("worker offer slot: got slot offer:%+v", *r)
	for _, w := range wo.worker {
		if w.host.Equal(r.Host) {
			if w.conn == nil {
				blog.Infof("worker offer slot: worker %s closed, do nothing for slot offer",
					w.host.Server)
				wo.workerLock.RUnlock()
				return nil
			}

			if r.AvailableSlotNum <= 0 {
				if r.Refused > 0 {
					// blog.Infof("worker offer slot: host[%+v] is refused with:%s", *r.Host, r.Message)

					// Refused 状态后续可以重试，需要特殊处理下
					w.status = Refused
					w.lasttime = time.Now().Unix()
					// blog.Infof("worker offer slot: set host[%+v] to busy", *w.host)
					blog.Infof("worker offer slot: set worker %s priority %d to Refused with message:%s",
						w.host.Server, w.priority, r.Message)
				} else {
					// 这个地方可能是网络异常了，置状态为init，后续可以重试
					w.resetSlot("received error")
					blog.Infof("worker offer slot: set worker %s priority %d to Init",
						w.host.Server, w.priority)
				}

				blog.Infof("worker offer slot: got invalid slot result:%+v", *r)
				wo.workerLock.RUnlock()
				return nil
			} else {
				if w.status != InService {
					w.status = InService
					// blog.Infof("worker offer slot: host[%+v] Priority %d is in service now", *r.Host, r.Priority)
					blog.Infof("worker offer slot: set worker %s priority %d to InService",
						w.host.Server, w.priority)
				}
				targetconn = w.conn
			}
			break
		}
	}
	wo.workerLock.RUnlock()

	wo.lastGetSlotTime = time.Now()

	// 确认当前能够消化多少slot（根据当前任务数来计算），并通知worker
	if r.AvailableSlotNum > 0 && targetconn != nil {
		consumed, err := wo.consumeSlot(&remoteSlotOffer{
			Host:             r.Host,
			AvailableSlotNum: r.AvailableSlotNum,
			ReceivedTime:     time.Now(),
		})

		if err != nil {
			blog.Errorf("[slotmgr] failed to consume slot for error:%v", err)
			return err
		}

		return wo.sendSlotRspAck(wkclient.NewTCPClientWithConn(targetconn), int32(consumed))
	}

	return nil
}

// consume slot
func (wo *workerOffer) consumeSlot(r *remoteSlotOffer) (int, error) {
	consumed := 0
	if wo.waitingList.Len() > 0 {
		for e := wo.waitingList.Front(); e != nil && consumed < int(r.AvailableSlotNum); {
			next := e.Next()

			msg := e.Value.(*lockWorkerMessage)
			msg.result <- r.Host
			wo.waitingList.Remove(e)
			consumed += 1

			e = next
		}
	}

	return consumed, nil
}

// send query slot response
func (wo *workerOffer) sendSlotRspAck(
	client *wkclient.TCPClient,
	consumeslotnum int32) error {
	blog.Infof("[slotmgr] send slot response ack to client[%s] with consumed %d slots",
		client.RemoteAddr(), consumeslotnum)

	// encode response to messages
	messages, err := wkclient.EncodeSlotRspAck(consumeslotnum)
	if err != nil {
		blog.Errorf("[slotmgr] failed to encode rsp ack to messages for error:%v", err)
		return err
	}

	// send response
	err = wkclient.SendMessages(client, messages)
	if err != nil {
		blog.Errorf("[slotmgr] failed to send messages to client[%s] for error:%v", client.RemoteAddr(), err)
		return err
	}

	return nil
}

// 选择适当的worker列表和相应的优先级，发送slot请求
// 触发条件：1. 第一次需要slot时，  2. 定时检查，当有任务排队等待slot时
func (wo *workerOffer) querySlot() error {
	blog.Debugf("worker offer slot: start query slot")

	hightSucceed := 0
	hightDetecting := 0
	middleSucceed := 0
	middleDetecting := 0

	initWorker := make([]*worker, 0)
	targetPriority := dcSDK.PriorityLow
	targetSlotNum := 0

	wo.workerLock.RLock()
	defer wo.workerLock.RUnlock()

	for _, w := range wo.worker {
		// refused 一段时间后可以重新尝试
		if w.status == Refused &&
			time.Now().Unix()-w.lasttime > RefusedRecoverSeconds {
			w.resetSlot("re-check refused worker")
			blog.Infof("worker offer slot: set worker %s priority %d to Init",
				w.host.Server, w.priority)
		}

		if w.invalid() {
			continue
		}

		if w.status == Init {
			blog.Infof("worker offer slot: found init worker %s now", w.host.Server)
			initWorker = append(initWorker, w)
			continue
		}

		// TODO : 是否需要区分 p2p 资源和普通加速资源？
		//        比如将p2p资源的Jobs乘以一个系数？
		if w.status == InService ||
			w.status == Detecting ||
			w.status == DetectSucceed {
			switch w.priority {
			case dcSDK.PriorityHight:
				if w.status == InService {
					hightSucceed += w.host.Jobs
				} else if w.status == Detecting || w.status == DetectSucceed {
					hightDetecting += w.host.Jobs
				}
			case dcSDK.PriorityMiddle:
				if w.status == InService {
					middleSucceed += w.host.Jobs
				} else if w.status == Detecting || w.status == DetectSucceed {
					middleDetecting += w.host.Jobs
				}
			}
		}
	}

	var succeedSlots int32
	for len(initWorker) > 0 {
		if hightSucceed < MaxHightPrioritySlot {
			// 已经申请了足够的worker，但有些还没有返回结果，需要等待
			if hightDetecting > 0 && (hightSucceed+hightDetecting) > MaxHightPrioritySlot {
				return nil
			}
			// 高优先级的资源申请不够，需要增加
			targetPriority = dcSDK.PriorityHight
			targetSlotNum = MaxHightPrioritySlot - (hightSucceed + hightDetecting)
		} else if (middleSucceed + middleDetecting) < MaxMiddlePrioritySlot {
			targetPriority = dcSDK.PriorityMiddle
			targetSlotNum = MaxMiddlePrioritySlot - (middleSucceed + middleDetecting)
		}

		// 按需要的核数尝试一批
		tryWorker := make([]*worker, 0)
		trySlots := 0
		for i, v := range initWorker {
			tryWorker = append(tryWorker, v)
			trySlots += v.host.Jobs
			if trySlots >= targetSlotNum {
				if i < len(initWorker)-1 {
					initWorker = initWorker[i+1:]
				} else {
					initWorker = nil
				}
				break
			}
		}

		if trySlots < targetSlotNum {
			initWorker = nil
		}

		remoteWorker := client.NewCommonRemoteWorker()
		handler := remoteWorker.Handler(SlotTCPTimeoutSeconds, nil, nil, nil)

		var wg sync.WaitGroup
		for _, v := range tryWorker {
			wg.Add(1)
			go func(v *worker) {
				defer wg.Done()
				req := &dcSDK.BKQuerySlot{
					Priority:         int32(targetPriority),
					WaitTotalTaskNum: 0,
					TaskType:         "",
				}

				v.priority = targetPriority
				v.status = Detecting
				blog.Infof("worker offer slot: set worker %s priority %d to Detecting",
					v.host.Server, targetPriority)

				// blog.Infof("worker offer slot: ready query slot worker %s with priority %d now",
				// 	v.host.Server, targetPriority)
				c, err := handler.ExecuteQuerySlot(v.host, req, wo.slotChan, SlotTCPTimeoutSeconds)
				if err == nil {
					v.status = DetectSucceed
					blog.Infof("worker offer slot: set worker %s priority %d to DetectSucceed",
						v.host.Server, targetPriority)
					v.conn = c

					atomic.AddInt32(&succeedSlots, int32(v.host.Jobs))
				} else {
					v.status = DetectFailed
					blog.Infof("worker offer slot: set worker %s priority %d to DetectFailed",
						v.host.Server, targetPriority)
				}
			}(v)
		}
		wg.Wait()

		if succeedSlots >= int32(targetSlotNum) {
			blog.Infof("worker offer slot: got enought slot[%d]", succeedSlots)
			return nil
		}
	}

	blog.Infof("worker offer slot: not got enought slot after try %d workers", len(initWorker))

	return nil
}

// 检查等待的任务列表，避免任务一直阻塞
func (wo *workerOffer) jobCheck() error {
	toLocalNum := 0
	for wo.waitingList.Len() > 0 &&
		wo.lastGetSlotTime.Add(MaxNoSlotSIntervalTime).Before(time.Now()) {

		e := wo.waitingList.Front()
		msg := e.Value.(*lockWorkerMessage)
		if msg.startWait.Add(MaxJosIdleIntervalTime).Before(time.Now()) {
			msg.result <- nil
			wo.waitingList.Remove(e)

			toLocalNum += 1
			if toLocalNum >= wo.localJosBatchSize {
				blog.Infof("worker offer slot: set %d job to local for no remote slot", toLocalNum)
				break
			}
		}
	}

	return nil
}

func (wo *workerOffer) resetWorkerSlot(host *dcProtocol.Host) error {
	wo.workerLock.Lock()
	defer wo.workerLock.Unlock()
	for _, w := range wo.worker {
		if host.Equal(w.host) {
			w.resetSlot("idle slot too long")
			blog.Infof("worker offer slot: set worker %s priority %d to Init",
				w.host.Server, w.priority)
			break
		}
	}
	return nil
}
