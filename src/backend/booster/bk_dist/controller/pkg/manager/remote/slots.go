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
	"sync"
	"time"

	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

type RemoteSlotMgr interface {
	Handle(ctx context.Context)
	Reset(hl []*dcProtocol.Host) ([]*dcProtocol.Host, error)
	DisableAllWorker()
	GetDeadWorkers() []*worker
	RecoverDeadWorker(w *worker)
	DisableWorker(host *dcProtocol.Host)
	EnableWorker(host *dcProtocol.Host)
	SetWorkerStatus(host *dcProtocol.Host, status Status)
	Lock(usage dcSDK.JobUsage, f string, banWorkerList []*dcProtocol.Host) *dcProtocol.Host
	Unlock(usage dcSDK.JobUsage, host *dcProtocol.Host)
	TotalSlots() int
	GetWorkers() []*worker
	CountWorkerError(w *worker)
	IsWorkerDead(w *worker, netErrorLimit int) bool
	WorkerDead(w *worker)
}

type lockWorkerMessage struct {
	jobUsage      dcSDK.JobUsage
	toward        *dcProtocol.Host
	result        chan *dcProtocol.Host
	largeFile     string
	banWorkerList []*dcProtocol.Host
	startWait     time.Time
}
type lockWorkerChan chan lockWorkerMessage

type usageWorkerSet struct {
	limit    int
	occupied int
}

// func newResource(hl []*dcProtocol.Host, usageLimit map[dcSDK.JobUsage]int) *resource {
func newResource(hl []*dcProtocol.Host) *resource {
	wl := make([]*worker, 0, len(hl))
	total := 0
	for _, h := range hl {
		if h.Jobs <= 0 {
			continue
		}

		wl = append(wl, &worker{
			host:                h,
			totalSlots:          h.Jobs,
			occupiedSlots:       0,
			continuousNetErrors: 0,
			dead:                false,
		})
		total += h.Jobs
	}

	usageMap := make(map[dcSDK.JobUsage]*usageWorkerSet, 10)
	// do not use usageLimit, we only need JobUsageRemoteExe, and it is always 0 by now
	usageMap[dcSDK.JobUsageRemoteExe] = &usageWorkerSet{
		limit:    total,
		occupied: 0,
	}
	usageMap[dcSDK.JobUsageDefault] = &usageWorkerSet{
		limit:    total,
		occupied: 0,
	}

	for _, v := range usageMap {
		blog.Infof("remote slot: usage map:%v after new resource", *v)
	}
	blog.Infof("remote slot: total slots:%d after new resource", total)

	return &resource{
		totalSlots:    total,
		occupiedSlots: 0,
		usageMap:      usageMap,
		lockChan:      make(lockWorkerChan, 1000),
		unlockChan:    make(lockWorkerChan, 1000),
		emptyChan:     make(chan bool, 1000),
		worker:        wl,

		waitingList: list.New(),
	}
}

type resource struct {
	ctx context.Context

	workerLock sync.RWMutex
	worker     []*worker

	totalSlots    int
	occupiedSlots int

	usageMap map[dcSDK.JobUsage]*usageWorkerSet

	lockChan   lockWorkerChan
	unlockChan lockWorkerChan

	// trigger when worker change to empty
	emptyChan chan bool

	handling bool

	// to save waiting requests
	waitingList *list.List
}

// reset with []*dcProtocol.Host
// add new hosts and disable released hosts
func (wr *resource) Reset(hl []*dcProtocol.Host) ([]*dcProtocol.Host, error) {
	blog.Infof("remote slot: ready reset with %d host", len(hl))

	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	wl := make([]*worker, 0, len(hl))
	total := 0
	for _, h := range hl {
		if h.Jobs <= 0 {
			continue
		}

		wl = append(wl, &worker{
			host:                h,
			totalSlots:          h.Jobs,
			occupiedSlots:       0,
			continuousNetErrors: 0,
			dead:                false,
		})
		total += h.Jobs
	}

	usageMap := make(map[dcSDK.JobUsage]*usageWorkerSet, 10)
	// do not use usageLimit, we only need JobUsageRemoteExe, and it is always 0 by now
	usageMap[dcSDK.JobUsageRemoteExe] = &usageWorkerSet{
		limit:    total,
		occupied: 0,
	}
	usageMap[dcSDK.JobUsageDefault] = &usageWorkerSet{
		limit:    total,
		occupied: 0,
	}

	for _, v := range usageMap {
		blog.Infof("remote slot: usage map:%v after reset with new resource", *v)
	}
	blog.Infof("remote slot: total slots:%d after reset with new resource", total)

	wr.totalSlots = total
	wr.occupiedSlots = 0
	wr.usageMap = usageMap
	wr.worker = wl

	return hl, nil
}

// brings handler up and begin to handle requests
func (wr *resource) Handle(ctx context.Context) {
	if wr.handling {
		return
	}

	wr.handling = true

	go wr.handleLock(ctx)
}

// Lock get an usage lock, success with true, failed with false
func (wr *resource) Lock(usage dcSDK.JobUsage, f string, banWorkerList []*dcProtocol.Host) *dcProtocol.Host {
	if !wr.handling {
		return nil
	}

	msg := lockWorkerMessage{
		jobUsage:      usage,
		toward:        nil,
		result:        make(chan *dcProtocol.Host, 1),
		largeFile:     f,
		banWorkerList: banWorkerList,
	}

	// send a new lock request
	wr.lockChan <- msg

	select {
	case <-wr.ctx.Done():
		return &dcProtocol.Host{}

	// wait result
	case h := <-msg.result:
		return h
	}
}

// Unlock release an usage lock
func (wr *resource) Unlock(usage dcSDK.JobUsage, host *dcProtocol.Host) {
	if !wr.handling {
		return
	}

	wr.unlockChan <- lockWorkerMessage{
		jobUsage: usage,
		toward:   host,
		result:   nil,
	}
}

func (wr *resource) TotalSlots() int {
	return wr.totalSlots
}

func (wr *resource) DisableWorker(host *dcProtocol.Host) {
	if host == nil {
		return
	}

	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	invalidjobs := 0
	for _, w := range wr.worker {
		if !host.Equal(w.host) {
			continue
		}

		if w.disabled {
			blog.Infof("remote slot: host:%v disabled before,do nothing now", *host)
			break
		}

		w.disabled = true
		invalidjobs = w.totalSlots
		break
	}

	// !!! wr.totalSlots and v.limit may be <= 0 !!!
	if invalidjobs > 0 {
		wr.totalSlots -= invalidjobs
		for _, v := range wr.usageMap {
			v.limit = wr.totalSlots
			blog.Infof("remote slot: usage map:%v after disable host:%v", *v, *host)
		}
	}

	if wr.totalSlots <= 0 {
		wr.emptyChan <- true
	}

	blog.Infof("remote slot: total slot:%d after disable host:%v", wr.totalSlots, *host)
	return
}

func (wr *resource) EnableWorker(host *dcProtocol.Host) {
	if host == nil {
		return
	}
	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	for _, w := range wr.worker {
		if !host.Equal(w.host) {
			continue
		}
		if !w.disabled {
			blog.Infof("remote slot: host:%v enabled before, do nothing now", *host)
			return // already enabled
		}

		w.disabled = false
		wr.totalSlots += w.totalSlots
		w.status = RetrySucceed
		break
	}

	for _, v := range wr.usageMap {
		v.limit = wr.totalSlots
		blog.Infof("remote slot: usage map:%v after enable host:%v", *v, *host)
	}
	blog.Infof("remote slot: total slot:%d after enable host:%v", wr.totalSlots, *host)
}

func (wr *resource) SetWorkerStatus(host *dcProtocol.Host, s Status) {
	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	for _, w := range wr.worker {
		if !w.host.Equal(host) {
			continue
		}

		w.status = s
		break
	}
}

func (wr *resource) WorkerDead(w *worker) {
	if w == nil || w.host == nil {
		return
	}

	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	invalidjobs := 0
	for _, wk := range wr.worker {
		if !wk.host.Equal(w.host) {
			continue
		}

		if wk.dead {
			blog.Infof("remote slot: host:%v is already dead,do nothing now", w.host)
			return
		}

		wk.dead = true
		invalidjobs = w.totalSlots
		break
	}

	// !!! wr.totalSlots and v.limit may be <= 0 !!!
	if invalidjobs > 0 {
		wr.totalSlots -= invalidjobs
		for _, v := range wr.usageMap {
			v.limit = wr.totalSlots
			blog.Infof("remote slot: usage map:%v after host is dead:%v", *v, w.host)
		}
	}

	if wr.totalSlots <= 0 {
		wr.emptyChan <- true
	}

	blog.Infof("remote slot: total slot:%d after host is dead:%v", wr.totalSlots, w.host)
	return
}

func (wr *resource) DisableAllWorker() {
	blog.Infof("remote slot: ready disable all host")

	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	for _, w := range wr.worker {
		if w.disabled {
			continue
		}

		w.disabled = true
	}

	wr.totalSlots = 0
	for _, v := range wr.usageMap {
		v.limit = 0
		blog.Infof("remote slot: usage map:%v after disable all host", *v)
	}

	if wr.totalSlots <= 0 {
		wr.emptyChan <- true
	}

	blog.Infof("remote slot: total slot:%d after disable all host", wr.totalSlots)
	return
}

func (wr *resource) RecoverDeadWorker(w *worker) {
	if w == nil || w.host == nil {
		return
	}
	blog.Infof("remote slot: ready enable host(%s)", w.host.Server)

	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	for _, wk := range wr.worker {
		if !w.host.Equal(wk.host) {
			continue
		}

		if !wk.dead {
			blog.Infof("remote slot: host:%v is not dead, do nothing now", w.host)
			return
		}

		wk.dead = false
		wk.continuousNetErrors = 0
		wr.totalSlots += w.totalSlots
		break
	}

	for _, v := range wr.usageMap {
		v.limit = wr.totalSlots
		blog.Infof("remote slot: usage map:%v after recover host:%v", *v, w.host)
	}

	blog.Infof("remote slot: total slot:%d after recover host:%v", wr.totalSlots, w.host)
	return
}

func (wr *resource) CountWorkerError(w *worker) {
	if w == nil || w.host == nil {
		return
	}
	blog.Infof("remote slot: ready count error from host(%s)", w.host.Server)

	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	for _, wk := range wr.worker {
		if !w.host.Equal(wk.host) {
			continue
		}
		wk.continuousNetErrors++
		break
	}
}

func (wr *resource) GetWorkers() []*worker {
	wr.workerLock.RLock()
	defer wr.workerLock.RUnlock()

	workers := []*worker{}
	for _, w := range wr.worker {
		workers = append(workers, w.copy())
	}
	return workers
}

func (wr *resource) GetDeadWorkers() []*worker {
	wr.workerLock.RLock()
	defer wr.workerLock.RUnlock()

	workers := []*worker{}
	for _, w := range wr.worker {
		if !w.disabled && w.dead {
			workers = append(workers, w.copy())
		}
	}
	return workers
}

func (wr *resource) IsWorkerDead(w *worker, netErrorLimit int) bool {
	wr.workerLock.RLock()
	defer wr.workerLock.RUnlock()

	for _, wk := range wr.worker {
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

func (wr *resource) addWorker(host *dcProtocol.Host) {
	if host == nil || host.Jobs <= 0 {
		return
	}

	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	for _, w := range wr.worker {
		if host.Equal(w.host) {
			blog.Infof("remote slot: host(%s) existed when add", w.host.Server)
			return
		}
	}

	wr.worker = append(wr.worker, &worker{
		host:                host,
		totalSlots:          host.Jobs,
		occupiedSlots:       0,
		continuousNetErrors: 0,
		dead:                false,
	})
	wr.totalSlots += host.Jobs

	for _, v := range wr.usageMap {
		v.limit = wr.totalSlots
		blog.Infof("remote slot: usage map:%v after add host:%v", *v, *host)
	}

	blog.Infof("remote slot: total slot:%d after add host:%v", wr.totalSlots, *host)
	return
}

func (wr *resource) getWorkerWithMostFreeSlots(banWorkerList []*dcProtocol.Host) (*worker, bool) {
	var w *worker
	max := 0
	hasAvailableWorker := false
	for _, worker := range wr.worker {
		if worker.disabled || worker.dead {
			continue
		}
		matched := false
		for _, host := range banWorkerList {
			if worker.host.Equal(host) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		hasAvailableWorker = true
		free := worker.totalSlots - worker.occupiedSlots
		if free >= max {
			max = free
			w = worker
		}
	}

	// if w == nil {
	// 	w = wr.worker[0]
	// }

	return w, hasAvailableWorker
}

// 大文件优先
func (wr *resource) getWorkerLargeFileFirst(f string, banWorkerList []*dcProtocol.Host) (*worker, bool) {
	var w *worker
	max := 0
	inlargequeue := false
	hasAvailableWorker := false
	for _, worker := range wr.worker {
		if worker.disabled || worker.dead {
			continue
		}
		matched := false
		for _, host := range banWorkerList {
			if worker.host.Equal(host) {
				matched = true
				break
			}
		}

		if matched {
			continue
		}

		hasAvailableWorker = true
		free := worker.totalSlots - worker.occupiedSlots

		// 在资源空闲时，大文件优先
		if free > worker.totalSlots/2 && worker.hasFile(f) {
			if !inlargequeue { // first in large queue
				inlargequeue = true
				max = free
				w = worker
			} else {
				if free >= max {
					max = free
					w = worker
				}
			}
			continue
		}

		if free >= max && !inlargequeue {
			max = free
			w = worker
		}
	}

	if w == nil {
		// w = wr.worker[0]
		return w, hasAvailableWorker
	}

	if f != "" && !w.hasFile(f) {
		w.largefiles = append(w.largefiles, f)
	}

	return w, hasAvailableWorker
}

func (wr *resource) occupyWorkerSlots(f string, banWorkerList []*dcProtocol.Host) (*dcProtocol.Host, bool) {
	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	var w *worker
	var hasWorkerAvailable bool
	if f == "" {
		w, hasWorkerAvailable = wr.getWorkerWithMostFreeSlots(banWorkerList)
	} else {
		w, hasWorkerAvailable = wr.getWorkerLargeFileFirst(f, banWorkerList)
	}

	if w == nil {
		return nil, hasWorkerAvailable
	}

	_ = w.occupySlot()
	return w.host, hasWorkerAvailable
}

func (wr *resource) freeWorkerSlots(host *dcProtocol.Host) {
	wr.workerLock.Lock()
	defer wr.workerLock.Unlock()

	for _, w := range wr.worker {
		if !host.Equal(w.host) {
			continue
		}

		_ = w.freeSlot()
		return
	}
}

func (wr *resource) handleLock(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 20)

	defer ticker.Stop()
	wr.ctx = ctx

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-wr.unlockChan:
			wr.putSlot(msg)
		case msg := <-wr.lockChan:
			wr.getSlot(msg)
		case <-wr.emptyChan:
			wr.onSlotEmpty()
		case <-ticker.C:
			wr.occupyWaitList()
		}
	}
}

func (wr *resource) getUsageSet(usage dcSDK.JobUsage) *usageWorkerSet {
	set, ok := wr.usageMap[usage]
	if !ok {
		// unknown usage, get default usage
		set = wr.usageMap[dcSDK.JobUsageDefault]
	}

	return set
}

func (wr *resource) isIdle(set *usageWorkerSet) bool {
	if set == nil {
		return false
	}

	if set.occupied < set.limit || set.limit <= 0 {
		return true
	}

	return false
}

func (wr *resource) getSlot(msg lockWorkerMessage) {
	if wr.totalSlots <= 0 {
		msg.result <- nil
		return
	}

	satisfied := false
	usage := msg.jobUsage
	if wr.occupiedSlots < wr.totalSlots || wr.totalSlots <= 0 {
		set := wr.getUsageSet(usage)
		if wr.isIdle(set) {
			if h, _ := wr.occupyWorkerSlots(msg.largeFile, msg.banWorkerList); h != nil {
				set.occupied++
				wr.occupiedSlots++
				blog.Infof("remote slot: total slots:%d occupied slots:%d, remote slot available",
					wr.totalSlots, wr.occupiedSlots)
				msg.result <- h
				satisfied = true
			}
		}
	}

	if !satisfied {
		blog.Infof("remote slot: total slots:%d occupied slots:%d, remote slot not available",
			wr.totalSlots, wr.occupiedSlots)
		// wr.waitingList.PushBack(usage)
		// wr.waitingList.PushBack(msg.result)
		wr.waitingList.PushBack(&msg)
	}
}

func (wr *resource) putSlot(msg lockWorkerMessage) {
	wr.freeWorkerSlots(msg.toward)
	wr.occupiedSlots--
	blog.Debugf("remote slot: free slot for worker %v, %v", wr.occupiedSlots, wr.totalSlots)
	usage := msg.jobUsage
	set := wr.getUsageSet(usage)
	set.occupied--

	// check whether other waiting is satisfied now
	if wr.waitingList.Len() > 0 {
		blog.Debugf("remote slot: free slot for worker %v, %v", wr.occupiedSlots, wr.waitingList.Len())
		for e := wr.waitingList.Front(); e != nil; e = e.Next() {
			msg := e.Value.(*lockWorkerMessage)
			set := wr.getUsageSet(msg.jobUsage)
			if wr.isIdle(set) {
				if h, hasAvailableWorker := wr.occupyWorkerSlots(msg.largeFile, msg.banWorkerList); h != nil {
					set.occupied++
					wr.occupiedSlots++
					msg.result <- h
					wr.waitingList.Remove(e)
					break
				} else if !hasAvailableWorker {
					msg.result <- nil
					wr.waitingList.Remove(e)
					blog.Infof("remote slot: occupy waiting list, but no slot available for ban worker list %v, just turn it local", msg.banWorkerList)
				} else {
					blog.Debugf("remote slot: occupy waiting list, but no slot available %v", msg.banWorkerList)
				}
			}
		}
	}
}

func (wr *resource) onSlotEmpty() {
	blog.Infof("remote slot: on slot empty: occupy:%d,total:%d,waiting:%d", wr.occupiedSlots, wr.totalSlots, wr.waitingList.Len())

	for wr.waitingList.Len() > 0 {
		e := wr.waitingList.Front()
		msg := e.Value.(*lockWorkerMessage)
		msg.result <- nil
		wr.waitingList.Remove(e)
	}
}

func (wr *resource) occupyWaitList() {
	if wr.waitingList.Len() > 0 {
		for e := wr.waitingList.Front(); e != nil; e = e.Next() {
			msg := e.Value.(*lockWorkerMessage)
			set := wr.getUsageSet(msg.jobUsage)
			if wr.isIdle(set) {
				h, hasAvailableWorker := wr.occupyWorkerSlots(msg.largeFile, msg.banWorkerList)
				if h != nil {
					set.occupied++
					wr.occupiedSlots++
					msg.result <- h
					wr.waitingList.Remove(e)
					blog.Debugf("remote slot: occupy waiting list")
				} else if !hasAvailableWorker { // no slot available for ban worker list, turn it local
					blog.Infof("remote slot: occupy waiting list, but no slot available for ban worker list %v, just turn it local", msg.banWorkerList)
					msg.result <- nil
					wr.waitingList.Remove(e)
				} else {
					blog.Debugf("remote slot: occupy waiting list, but no slot available %v", msg.banWorkerList)
				}
			}
		}
	}
}
