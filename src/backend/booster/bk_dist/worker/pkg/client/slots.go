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
	"container/list"
	"context"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/shirou/gopsutil/mem"
)

const (
	largeRequest  = 100 * 1024 * 1024
	largeLocked   = 1024 * 1024 * 1024
	leastFree     = 2 * 1024 * 1024 * 1024
	maxMemPercent = 85.0
)

// by tming to limit send file memory
func newSlot(ctx context.Context, setSlots int64) *slot {

	// 允许通过参数禁止该功能
	if setSlots < 0 {
		return nil
	}

	minslots := int64(1 * 1024 * 1024 * 1024)
	defaultslots := int64(4 * 1024 * 1024 * 1024)
	maxslots := int64(16 * 1024 * 1024 * 1024)

	// 限制最大内存不超过1半
	v, err := mem.VirtualMemory()
	if err == nil {
		halfmem := int64(v.Total) / 2
		if maxslots > halfmem {
			maxslots = halfmem
		}
	}

	blog.Infof("send slot: maxSlots:%d", setSlots)
	if setSlots <= 0 {
		setSlots = defaultslots
		blog.Infof("send slot: maxSlots:%d", setSlots)
	}

	if setSlots < minslots {
		setSlots = minslots
	}

	if setSlots > maxslots {
		setSlots = maxslots
	}

	blog.Infof("send slot: set max slot:%d", setSlots)

	waitingList := list.New()

	ret := &slot{
		totalSlots:    setSlots,
		occupiedSlots: 0,

		lockChan:   make(chanChanPair, 1000),
		unlockChan: make(chanChanPair, 1000),

		waitingList: waitingList,
	}

	ret.Handle(ctx)

	return ret
}

type chanResult chan struct{}

type chanPair struct {
	result chanResult
	weight int64
}

type chanChanPair chan chanPair

type slot struct {
	ctx context.Context

	totalSlots    int64
	occupiedSlots int64

	lockChan   chanChanPair
	unlockChan chanChanPair

	handling bool

	waitingList *list.List
}

// brings handler up and begin to handle requests
func (lr *slot) Handle(ctx context.Context) {
	if lr.handling {
		return
	}

	lr.handling = true

	go lr.handleLock(ctx)
}

// GetStatus get current slots status
func (lr *slot) GetStatus() (int64, int64) {
	return lr.totalSlots, lr.occupiedSlots
}

// Lock get an usage lock, success with true, failed with false
func (lr *slot) Lock(weight int64) bool {
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
func (lr *slot) Unlock(weight int64) {
	if !lr.handling {
		return
	}

	msg := chanPair{weight: weight, result: nil}
	lr.unlockChan <- msg
}

func (lr *slot) handleLock(ctx context.Context) {
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

func (lr *slot) hasEnoughtSlots(pairChan chanPair) bool {
	// 如果已经锁定的内存超过了largeLocked，再次申请内存时，需要关注当前系统内存情况
	if lr.occupiedSlots > largeLocked {
		v, err := mem.VirtualMemory()
		if err == nil {
			if v.Available < leastFree ||
				v.Available < uint64(pairChan.weight) ||
				v.UsedPercent > maxMemPercent {
				blog.Infof("send slot: request size:%d,locked size:%d,Available:%d,UsedPercent:%f",
					pairChan.weight,
					lr.occupiedSlots,
					v.Available,
					v.UsedPercent)
				return false
			}
		}
	}

	if lr.occupiedSlots+pairChan.weight < lr.totalSlots {
		return true
	}

	if pairChan.weight < lr.totalSlots && lr.occupiedSlots+pairChan.weight > lr.totalSlots {
		return false
	}

	return lr.occupiedSlots < lr.totalSlots
}

func (lr *slot) getSlot(pairChan chanPair) {
	// blog.Infof("send slot: before get slot occpy size:%d,total size:%d,wait length:%d", lr.occupiedSlots, lr.totalSlots, lr.waitingList.Len())

	if lr.hasEnoughtSlots(pairChan) {
		lr.occupiedSlots += pairChan.weight
		pairChan.result <- struct{}{}
	} else {
		lr.waitingList.PushBack(pairChan)
	}

	blog.Debugf("send slot: after get slot occpy size:%d,total size:%d,wait length:%d", lr.occupiedSlots, lr.totalSlots, lr.waitingList.Len())
}

func (lr *slot) putSlot(pairChan chanPair) {
	// blog.Infof("send slot: before put slot occpy size:%d,total size:%d,wait length:%d", lr.occupiedSlots, lr.totalSlots, lr.waitingList.Len())

	lr.occupiedSlots -= pairChan.weight
	if lr.occupiedSlots < 0 {
		lr.occupiedSlots = 0
	}

	// for lr.hasEnoughtSlots(pairChan) && lr.waitingList.Len() > 0 {
	for lr.waitingList.Len() > 0 {
		e := lr.waitingList.Front()
		if e != nil {
			tempchan := e.Value.(chanPair)
			if lr.hasEnoughtSlots(tempchan) {
				lr.occupiedSlots += tempchan.weight

				// awake this task
				tempchan.result <- struct{}{}

				// delete this element
				lr.waitingList.Remove(e)
			} else {
				break
			}
		} else {
			blog.Errorf("send slot: found nil request!!")
			// delete this element
			lr.waitingList.Remove(e)
		}
	}

	blog.Debugf("send slot: after put slot occpy size:%d,total size:%d,wait length:%d", lr.occupiedSlots, lr.totalSlots, lr.waitingList.Len())
}
