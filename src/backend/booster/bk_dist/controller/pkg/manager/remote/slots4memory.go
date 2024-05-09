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

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// by tming to limit local memory usage
func newMemorySlot(maxSlots int64) *memorySlot {
	minmemroy := int64(100 * 1024 * 1024)          // 100MB
	defaultmemroy := int64(1 * 1024 * 1024 * 1024) // 1GB
	maxmemroy := int64(8 * 1024 * 1024 * 1024)     // 8GB

	blog.Infof("memory slot: maxSlots:%d", maxSlots)
	if maxSlots <= 0 {
		// v, err := mem.VirtualMemory()
		// if err != nil {
		// 	blog.Infof("memory slot: failed to get virtaul memory with err:%v", err)
		// 	maxSlots = int64((runtime.NumCPU() - 2)) * 1024 * 1024 * 1024
		// } else {
		// 	// maxSlots = int64(v.Total) - minmemroy
		// 	maxSlots = int64(v.Total) / 8
		// }
		maxSlots = defaultmemroy
		blog.Infof("memory slot: maxSlots:%d", maxSlots)
	}

	if maxSlots < minmemroy {
		maxSlots = minmemroy
	}

	if maxSlots > maxmemroy {
		maxSlots = maxmemroy
	}

	blog.Infof("memory slot: set max local memory:%d", maxSlots)

	waitingList := list.New()

	return &memorySlot{
		totalSlots:    maxSlots,
		occupiedSlots: 0,

		lockChan:   make(chanChanPair, 1000),
		unlockChan: make(chanChanPair, 1000),

		waitingList: waitingList,
	}
}

type chanResult chan struct{}

type chanPair struct {
	result chanResult
	weight int64
}

type chanChanPair chan chanPair

type memorySlot struct {
	ctx context.Context

	totalSlots    int64
	occupiedSlots int64

	lockChan   chanChanPair
	unlockChan chanChanPair

	handling bool

	waitingList *list.List
}

// brings handler up and begin to handle requests
func (lr *memorySlot) Handle(ctx context.Context) {
	if lr.handling {
		return
	}

	lr.handling = true

	go lr.handleLock(ctx)
}

// GetStatus get current slots status
func (lr *memorySlot) GetStatus() (int64, int64) {
	return lr.totalSlots, lr.occupiedSlots
}

// Lock get an usage lock, success with true, failed with false
func (lr *memorySlot) Lock(weight int64) bool {
	if !lr.handling {
		return false
	}

	msg := chanPair{weight: weight, result: make(chanResult, 1)}
	lr.lockChan <- msg

	select {
	case <-lr.ctx.Done():
		return false

	// wait result
	case <-msg.result:
		return true
	}
}

// Unlock release an usage lock
func (lr *memorySlot) Unlock(weight int64) {
	if !lr.handling {
		return
	}

	msg := chanPair{weight: weight, result: nil}
	lr.unlockChan <- msg
}

func (lr *memorySlot) handleLock(ctx context.Context) {
	lr.ctx = ctx

	for {
		select {
		case <-ctx.Done():
			lr.handling = false
			return
		case pairChan := <-lr.unlockChan:
			lr.putSlot(pairChan)
		case pairChan := <-lr.lockChan:
			lr.getSlot(pairChan)
		}
	}
}

func (lr *memorySlot) getSlot(pairChan chanPair) {
	if lr.occupiedSlots < lr.totalSlots {
		lr.occupiedSlots += pairChan.weight
		pairChan.result <- struct{}{}
	} else {
		lr.waitingList.PushBack(pairChan)
	}
}

func (lr *memorySlot) putSlot(pairChan chanPair) {
	lr.occupiedSlots -= pairChan.weight
	if lr.occupiedSlots < 0 {
		lr.occupiedSlots = 0
	}

	for lr.occupiedSlots < lr.totalSlots && lr.waitingList.Len() > 0 {
		e := lr.waitingList.Front()
		if e != nil {
			tempchan := e.Value.(chanPair)
			lr.occupiedSlots += tempchan.weight

			// awake this task
			tempchan.result <- struct{}{}

			// delete this element
			lr.waitingList.Remove(e)
		}
	}
}
