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
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"

	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/protocol"
)

const (
	estimateSlotIntervalTime = 500 * time.Millisecond
	clientCheckIntervalTime  = 60 * time.Second
	slotCheckIntervalTime    = 10 * time.Second

	maxClient = 3

	maxWaitSlotTime = 120 * time.Second

	clientCacheNum = 50
	slotCacheNum   = 50
)

var (
	// 优先级为高的客户端只保留一个（互斥）
	clientCache []*client = make([]*client, 0, clientCacheNum)
	clientLock  sync.RWMutex

	// 预订slot的细节
	slotCache []*slot = make([]*slot, 0, slotCacheNum)
	slotLock  sync.RWMutex

	// 预订slot的总数，这个数值有两个场景可以减少，1. 预期的任务来了  2. 超时取消
	// 但是这个数值可能不准确，比如给用户一个slot，用户发了不止一个任务过来；
	// 或者worker重启了，但是用户还是发了任务过来
	// 怎么彻底规避这类问题 ?
	// 似乎没办法，因为这儿取任务不是一个原子操作；比如每次重启，worker都可能发送邀约，
	// 如果不停的重启，那发出去的邀约要大于本地可用资源
	// 先假设worker和客户端是稳定的，即使任务大于预期，也只会排队，不会导致worker运行异常
	bookedSlotNum int

	slotChan chan bool = make(chan bool, 100)

	errorTooManyClient        = fmt.Errorf("too many client")
	errorHightPriorityExisted = fmt.Errorf("hight priority client existed")
	errorClientExisted        = fmt.Errorf("client has existed")
)

type client struct {
	tcpclient *protocol.TCPClient
	ip        string
	priority  int
	// 客户端请求的任务类型，先留着，后续看能否用上
	tasktype string
	valid    bool
}

type slot struct {
	c       *client
	num     int
	time    time.Time
	timeout bool
}

func notifySlot() {
	slotChan <- true
}

// 触发条件： 1. 定时 2. 有任务结束  3. 有新的客户端查询进来  4. 已分配的slot超时未使用清理
// 需要考虑各种异常情况：
// 1. 发送通知失败，则该可用槽需要回收（计数器回退），同时将该客户端从等待的池子中删除
// 2. 发送成功，但客户端这时没有任务了，则占了一个槽但没有用到，这种情况下根据超时来清理，比如分配了1分钟还没有任务过来，则清理掉,
//    同时降低该客户端的优先级
// 3. 客户端任务的端口是变化的，根据客户端的ip来对比（如果没有对比上，是否考虑直接丢弃？能否同时支持p2p和非p2p的任务？）
func (o *tcpManager) slotTimer() {
	blog.Infof("[slotmgr] start estimate slot timer")
	tick := time.NewTicker(estimateSlotIntervalTime)
	defer tick.Stop()

	tick1 := time.NewTicker(clientCheckIntervalTime)
	defer tick1.Stop()

	tick2 := time.NewTicker(slotCheckIntervalTime)
	defer tick2.Stop()

	for {
		select {
		case <-tick.C:
			o.estimateSlot()
		case <-slotChan:
			o.estimateSlot()
		case <-tick1.C:
			go o.checkClient()
		case <-tick2.C:
			go o.checkSlot()
		}
	}
}

// 如果满足条件，将该客户端放到等待池子里，并触发可用槽的计算，否则直接拒绝
func (o *tcpManager) dealQuerySlotCmd(tcpclient *protocol.TCPClient, head *dcProtocol.PBHead) error {
	req, err := protocol.ReceiveBKQuerySlot(tcpclient, head)
	if err != nil {
		blog.Warnf("[slotmgr] receive query slot request failed with error:%v", err)
		tcpclient.Close()
		return err
	}

	newclient := client{
		tcpclient: tcpclient,
		ip:        tcpclient.RemoteIP(),
		priority:  int(*req.Priority),
		tasktype:  *req.Tasktype,
		valid:     true,
	}

	blog.Infof("[slotmgr] received new query slot request:%v", newclient)

	newIP := tcpclient.RemoteIP()
	clientLock.RLock()
	curclientnum := len(clientCache)
	if *req.Priority == sdk.PriorityHight {
		for _, v := range clientCache {
			if v.priority == sdk.PriorityHight {
				clientLock.RUnlock()
				err = errorHightPriorityExisted
				// 高优先级的用户只允许一个
				goto onerror
			}
		}
	} else {
		for _, v := range clientCache {
			if v.ip == newIP {
				// 如果调高了优先级，需要更新
				if int(*req.Priority) > v.priority {
					v.priority = int(*req.Priority)
					blog.Infof("[slotmgr] received existed query slot request:%v Priority from %d to %d",
						newclient, v.priority, int(*req.Priority))
				}
				clientLock.RUnlock()
				err = errorClientExisted
				// 该用户已存在
				goto onerror
			}
		}
	}
	clientLock.RUnlock()

	// 普通优先级，则总用户数不超过最大值
	if curclientnum >= maxClient && *req.Priority < sdk.PriorityHight {
		err = errorTooManyClient
		goto onerror
	}

	clientLock.Lock()
	clientCache = append(clientCache, &newclient)
	clientLock.Unlock()

	blog.Infof("[slotmgr] append new query slot request:%v", newclient)
	notifySlot()
	return nil

onerror:
	o.sendSlotResponse(tcpclient, -1, 1, err.Error())
	time.Sleep(60 * time.Second)
	tcpclient.Close()
	blog.Warnf("[slotmgr] deal query slot request failed with error:%v", err)
	return nil
}

// 预估可用slot
func (o *tcpManager) estimateSlot() error {
	blog.Infof("[slotmgr] estimateSlot")
	o.updateAvailable()
	availableslotnum := int(currentAvailableCPU) - o.curjobs - bookedSlotNum

	if availableslotnum > 0 {
		c := o.selectClient()
		if c != nil {
			// save to slot cache
			slotLock.Lock()
			slotCache = append(slotCache, &slot{
				c:       c,
				num:     availableslotnum,
				time:    time.Now(),
				timeout: false,
			})
			bookedSlotNum += availableslotnum
			slotLock.Unlock()

			refused := 0
			message := ""
			o.sendSlotResponse(c.tcpclient, int32(availableslotnum), int32(refused), message)
		}
	}

	return nil
}

// 选择第一个优先级最高的客户端
func (o *tcpManager) selectClient() *client {
	clientLock.RLock()
	defer clientLock.RUnlock()

	var c *client = nil
	priority := -1
	for _, v := range clientCache {
		if v.priority > priority {
			c = v
			priority = v.priority
		}
	}

	return c
}

// send query slot response
func (o *tcpManager) sendSlotResponse(
	client *protocol.TCPClient,
	availableslotnum int32,
	refused int32,
	message string) error {
	blog.Infof("[slotmgr] send slot response to client[%s] with %d slots,refused:%d,message:[%s]",
		client.RemoteAddr(), availableslotnum, refused, message)

	// encode response to messages
	messages, err := protocol.EncodeBKQuerySlotRsp(availableslotnum, refused, message)
	if err != nil {
		blog.Errorf("[slotmgr] failed to encode rsp to messages for error:%v", err)
	}

	// send response
	err = protocol.SendMessages(client, &messages)
	if err != nil {
		blog.Errorf("[slotmgr] failed to send messages to client[%s] for error:%v", client.RemoteAddr(), err)
	}

	return nil
}

// 检查客户端连接是否正常，如果异常了，则清理掉
func (o *tcpManager) checkClient() {
	blog.Infof("[slotmgr] check all client now")
	clientLock.Lock()
	defer clientLock.Unlock()

	needclean := false
	for i := range clientCache {
		if clientCache[i].tcpclient.Closed() {
			needclean = true
			clientCache[i].valid = false
			clientCache[i].tcpclient.Close()
		}
	}

	if needclean {
		newlen := clientCacheNum
		if len(clientCache) > newlen {
			newlen = len(clientCache)
		}
		tmpCache := make([]*client, 0, newlen)
		for i := range clientCache {
			if clientCache[i].valid {
				tmpCache = append(tmpCache, clientCache[i])
			}
		}
		clientCache = tmpCache
	}
}

// 检查分配的slot是否超时，需要考虑客户端发送工具链的额外时间，先将超时时间设置稍微大点
// 超时的slot意味着什么？ 后续是否继续为该客户端分配slot？ worker端先不管，由客户端来检查和释放连接
func (o *tcpManager) checkSlot() {
	blog.Infof("[slotmgr] check slot now")

	slotLock.Lock()
	defer slotLock.Unlock()

	needclean := false
	for i := range slotCache {
		if slotCache[i].time.Add(maxWaitSlotTime).Before(time.Now()) {
			needclean = true
			slotCache[i].timeout = true
			if slotCache[i].num > 0 {
				bookedSlotNum -= slotCache[i].num
			}
		}
	}

	if needclean {
		if bookedSlotNum < 0 {
			bookedSlotNum = 0
		}

		newlen := slotCacheNum
		if len(slotCache) > newlen {
			newlen = len(slotCache)
		}
		tmpCache := make([]*slot, 0, newlen)
		for i := range slotCache {
			if !slotCache[i].timeout {
				tmpCache = append(tmpCache, slotCache[i])
			}
		}

		slotCache = tmpCache
	}
}

// 收到了客户端的任务，去掉相应的预扣
func (o *tcpManager) onTaskReceived(ip string) {
	blog.Infof("[slotmgr] on task received from client:%s", ip)

	slotLock.Lock()
	defer slotLock.Unlock()

	found := false
	for i := range slotCache {
		if slotCache[i].c.ip == ip {
			if slotCache[i].num > 0 {
				found = true
				blog.Infof("[slotmgr] found allocated record for client:%s", ip)

				slotCache[i].num -= 1
				// 删除该记录
				if slotCache[i].num <= 0 {
					slotCache = append(slotCache[:i], slotCache[i+1:]...)
				}

				bookedSlotNum -= 1
				if bookedSlotNum < 0 {
					bookedSlotNum = 0
				}

				break
			}
		}
	}

	if !found {
		blog.Infof("[slotmgr] not found allocated record for client:%s, it's unexpected task", ip)
	}
}

// 如果相应的客户端有发送文件过来，也可以认为该slot是激活的，超时时间可以往后推
//        因为现在任务执行前需要发送依赖文件
//        只激活超时时间的起始时间，不去掉预扣的slot
func (o *tcpManager) onFileReceived(ip string) {
	blog.Infof("[slotmgr] on file received from client:%s", ip)

	slotLock.Lock()
	defer slotLock.Unlock()

	for i := range slotCache {
		if slotCache[i].c.ip == ip {
			if slotCache[i].num > 0 {
				blog.Infof("[slotmgr] received file for client:%s", ip)
				slotCache[i].time = time.Now()
				break
			}
		}
	}
}
