/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package normal

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/pkg/engine"
)

const (
	queueNameSep = "<|>"
)

// Selector pay attention to the tasks in queue, pick the top-ranking task and launch it.
// And Selector change the task status staging to status starting.
type Selector interface {
	Run(pCtx context.Context) error
	OnTaskStatus(tb *engine.TaskBasic, curstatus engine.TaskStatusType) error
}

// NewSelector get a new selector with given layer and a list of queue brief info.
// queue brief info contains queue-engine pair, each one decide a picker to pick task from the queue of this engine.
func NewSelector(layer TaskBasicLayer, mgr *manager, queueInfoList ...engine.QueueBriefInfo) Selector {
	return &selector{
		layer:         layer,
		mgr:           mgr,
		queueInfoList: queueInfoList,
		queueChanMap:  make(map[engine.QueueBriefInfo]chan bool),
	}
}

type selector struct {
	ctx           context.Context
	layer         TaskBasicLayer
	queueInfoList []engine.QueueBriefInfo
	queueChanMap  map[engine.QueueBriefInfo]chan bool
	queueLock     sync.RWMutex
	mgr           *manager
}

// Run the selector handler with context.
func (s *selector) Run(ctx context.Context) error {
	s.ctx = ctx
	go s.start()
	return nil
}

func (s *selector) start() {
	blog.Infof("selector start")

	// init queueChanMap
	for _, info := range s.queueInfoList {
		c := make(chan bool, 100)
		s.queueChanMap[info] = c
	}

	s.queueLock.Lock()
	// start pickers
	for _, info := range s.queueInfoList {
		for k, v := range s.queueChanMap {
			if k.QueueName == info.QueueName && k.EngineName == info.EngineName {
				go s.picker(info, v)
				break
			}
		}
	}
	s.queueLock.Unlock()

	ticker := time.NewTicker(selectorCheckNewQueueTime)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			blog.Warnf("select shutdown")
			return
		case <-ticker.C:
			s.checkNewQueue()
		}
	}
}

func (s *selector) OnTaskStatus(tb *engine.TaskBasic, curstatus engine.TaskStatusType) error {
	blog.Infof("selector: ready notify to task(%s) engine(%s) queue(%s)", tb.ID, tb.Client.EngineName, tb.Client.QueueName)

	isNewQueue := true
	for k, v := range s.queueChanMap {
		if k.QueueName == tb.Client.QueueName && k.EngineName == tb.Client.EngineName {
			blog.Infof("selector: send notify to task(%s) engine(%s) queue(%s)", tb.ID, tb.Client.EngineName, tb.Client.QueueName)
			v <- true
			isNewQueue = false
			break
		}
	}

	if isNewQueue {
		s.onNewQueue(tb.Client.QueueName, tb.Client.EngineName)
	}

	return nil
}

func (s *selector) onNewQueue(queueName string, engineName engine.TypeName) error {
	// add to queue with lock
	info := engine.QueueBriefInfo{
		QueueName:  queueName,
		EngineName: engineName,
	}
	c := make(chan bool, 100)

	s.queueLock.Lock()
	s.queueChanMap[info] = c
	s.queueLock.Unlock()

	// start this go routine
	blog.Infof("selector: ready start picker for new engine(%s) queue(%s)", engineName, queueName)
	go s.picker(info, c)

	// sleep and notify
	// it may be fail to notify here, but the picker timer will recover this
	time.Sleep(1 * time.Second)
	c <- true

	return nil
}

func (s *selector) picker(info engine.QueueBriefInfo, c chan bool) {
	blog.Infof("selector: start engine(%s) queue(%s) picker", info.EngineName, info.QueueName)
	ticker := time.NewTicker(selectorLogQueueStatGapTime)
	defer ticker.Stop()

	egn, err := s.layer.GetEngineByTypeName(info.EngineName)
	if err != nil {
		blog.Errorf("selector: get engine(%s) failed: %v, exit picker", info.EngineName, err)
		return
	}

	tqg, err := s.layer.GetTaskQueueGroup(info.EngineName)
	if err != nil {
		blog.Errorf("selector: get engine(%s) task queue group failed: %v, exit picker", info.EngineName, err)
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.logStats(info, tqg)
		case <-c:
			blog.Infof("selector: received notify of engine(%s) queue(%s)", info.EngineName, info.QueueName)
			s.pick(egn, tqg, info.QueueName)
		default:
			s.pick(egn, tqg, info.QueueName)
			time.Sleep(selectorSelectSleepTime)
		}
	}
}

func (s *selector) logStats(info engine.QueueBriefInfo, tqg *engine.TaskQueueGroup) {
	tbl := tqg.GetQueue(info.QueueName).All()
	l := make([]string, 0, 100)
	for _, tb := range tbl {
		l = append(l, tb.ID)
	}
	blog.Infof("selector: engine(%s) queue(%s) stats: %v", info.EngineName, info.QueueName, l)
}

func (s *selector) pick(egn engine.Engine, tqg *engine.TaskQueueGroup, queueName string) {
	tb, err := egn.SelectFirstTaskBasic(tqg, queueName)
	if err != nil {
		if err != engine.ErrorNoTaskInQueue {
			blog.Errorf("selector: pick task from engine(%s) queue(%s) failed: %v", egn.Name(), queueName, err)
		}
		return
	}

	s.layer.LockTask(tb.ID, "pick_of_selector")
	defer s.layer.UnLockTask(tb.ID)

	// for debug
	blog.Infof("selector: ready launch task(%s) from engine(%s) queue(%s)", tb.ID, egn.Name(), queueName)

	tb, err = s.layer.GetTaskBasic(tb.ID)
	if err != nil {
		blog.Errorf("selector: get task basic(%s) from engine(%s) failed", tb.ID, egn.Name())
		return
	}

	if tb.Status.Status != engine.TaskStatusStaging {
		blog.Warnf("selector: task basic(%s) from engine(%s) queue(%s) not in staging, but in status(%s), skip",
			tb.ID, egn.Name(), queueName, tb.Status.Status)
		return
	}

	// 支持 queueName 为复合内容，比如 WIN://p2p_shenzhen_buildbooster<|>K8S_WIN://shenzhen
	// 表示支持  WIN://p2p_shenzhen_buildbooster 和 K8S_WIN://shenzhen 两种类型的资源的获取
	realqueuenames := strings.Split(queueName, queueNameSep)
	for _, q := range realqueuenames {
		if err = egn.LaunchTask(tb, q); err != nil {
			if err != engine.ErrorNoEnoughResources {
				blog.Infof("selector: launch task(%s) from engine(%s) queue(%s) failed: %v", tb.ID, egn.Name(), q, err)
			}
			blog.Infof("selector: launch task(%s) from engine(%s) queue(%s) failed: %v", tb.ID, egn.Name(), q, err)
			continue
		} else {
			queueName = q
			break
		}
	}

	if err != nil {
		return
	}

	tb.Client.QueueName = queueName
	tb.Status.Launch()
	tb.Status.Message = messageTaskStarting
	if err = s.layer.UpdateTaskBasic(tb); err != nil {
		blog.Errorf("selector: update task basic(%s) failed: %v", tb.ID, err)
		_ = egn.ReleaseTask(tb.ID)
		return
	}

	blog.Infof("selector: success to launch task(%s) from engine(%s) queue(%s)", tb.ID, egn.Name(), queueName)

	// notify next step immediately
	s.mgr.onTaskStatus(tb, engine.TaskStatusStarting)
}

// 用于定时检查新的queue的任务，主要是server重启或者主从切换时
// 如果是server运行期间的新的queue任务，会通过OnTaskStatus直接触发
func (s *selector) checkNewQueue() {
	blog.Debugf("selector: do check for checking new queue task")
	taskList, err := s.layer.ListTaskBasic(false, engine.TaskStatusStaging)
	if err != nil {
		blog.Errorf("selector: doing check, list task failed: %v", err)
		return
	}

	for _, tb := range taskList {
		isNewQueue := true
		for k := range s.queueChanMap {
			if k.QueueName == tb.Client.QueueName && k.EngineName == tb.Client.EngineName {
				isNewQueue = false
				break
			}
		}

		if isNewQueue {
			s.onNewQueue(tb.Client.QueueName, tb.Client.EngineName)
		}
	}

	return
}
