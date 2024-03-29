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
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/pkg/engine"
)

// Cleaner checks all the terminated status(finish or failed), find out the no-released-yet tasks, collect the
// stats information and release the backend servers.
type Cleaner interface {
	Run(pCtx context.Context) error
	OnTaskStatus(tb *engine.TaskBasic, curstatus engine.TaskStatusType) error
}

// NewCleaner get a new cleaner with given layer.
func NewCleaner(layer TaskBasicLayer) Cleaner {
	return &cleaner{
		layer: layer,
		tasks: map[string]bool{},
	}
}

type cleaner struct {
	ctx   context.Context
	layer TaskBasicLayer

	tasks     map[string]bool
	tasksLock sync.RWMutex

	c chan *engine.TaskBasic
}

// Run the cleaner handler with context.
func (c *cleaner) Run(ctx context.Context) error {
	c.ctx = ctx
	c.c = make(chan *engine.TaskBasic, 100)
	go c.start()
	return nil
}

func (c *cleaner) start() {
	blog.Infof("cleaner start")
	timeTicker := time.NewTicker(cleanerReleaseCheckGapTime)
	defer timeTicker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			blog.Warnf("cleaner shutdown")
			return
		case <-timeTicker.C:
			c.check()
		case tb := <-c.c:
			blog.Infof("cleaner: received notify")
			go c.onCleanNotify(tb)
		}
	}
}

func (c *cleaner) check() {
	terminatedTaskList, err := c.layer.ListTaskBasic(false, engine.TaskStatusFinish, engine.TaskStatusFailed)
	if err != nil {
		blog.Errorf("cleaner: doing check, list terminated task basic failed: %v", err)
		return
	}

	// var wg sync.WaitGroup
	for _, tb := range terminatedTaskList {
		if tb.Status.Released {
			continue
		}
		blog.Infof("cleaner: check and find task(%s) is unreleased, prepare to collect data and release", tb.ID)

		egn, err := c.layer.GetEngineByTypeName(tb.Client.EngineName)
		if err != nil {
			blog.Errorf("cleaner: try get task(%s) engine failed: %v", tb.ID, err)
			continue
		}

		// wg.Add(1)
		// go c.clean(tb.ID, egn, &wg)
		go c.clean(tb.ID, egn)
	}
	// wg.Wait()
}

// 确保一个taskid只有一个协程处理，避免拉起太多协程
func (c *cleaner) setFlag(taskID string) bool {
	c.tasksLock.Lock()
	defer c.tasksLock.Unlock()

	if _, ok := c.tasks[taskID]; !ok {
		c.tasks[taskID] = true
		return true
	} else {
		return false
	}
}

func (c *cleaner) unsetFlag(taskID string) {
	c.tasksLock.Lock()
	defer c.tasksLock.Unlock()

	delete(c.tasks, taskID)
}

// func (c *cleaner) clean(taskID string, egn engine.Engine, wg *sync.WaitGroup) {
func (c *cleaner) clean(taskID string, egn engine.Engine) {
	// defer wg.Done()

	blog.Infof("cleaner: start clean task(%s)", taskID)

	if c.setFlag(taskID) {
		defer c.unsetFlag(taskID)
	} else {
		blog.Infof("cleaner: task (%s) is cleaning by others, do nothing", taskID)
		return
	}

	c.layer.LockTask(taskID, "clean_of_cleaner")
	defer c.layer.UnLockTask(taskID)

	blog.Infof("cleaner: start get basic info for task(%s)", taskID)

	tb, err := c.layer.GetTaskBasic(taskID)
	if err != nil {
		blog.Errorf("cleaner: get task(%s) failed: %v", taskID, err)
		return
	}

	if tb.Status.Released {
		blog.Infof("cleaner: task (%s) is already released,do nothing", tb.ID)
		return
	}

	// StatusCode records from which status the task changed and if the server is probably alive, then collect
	// the stats info from servers.
	if tb.Status.StatusCode.ServerAlive() {
		// try collecting data via engine, no matter successful or not, this will not be retried
		blog.Infof("cleaner: try collecting task(%s) data before released", taskID)
		if err = egn.CollectTaskData(tb); err != nil {
			blog.Errorf("cleaner: try collecting task(%s) data failed: %v", taskID, err)
		}
	}

	projectInfoDelta := engine.DeltaProjectInfoBasic{}
	switch tb.Status.Status {
	case engine.TaskStatusFinish:
		projectInfoDelta.CompileFinishTimes = 1
	case engine.TaskStatusFailed:
		projectInfoDelta.CompileFailedTimes = 1
	}
	if err = engine.UpdateProjectInfoBasic(egn, tb.Client.ProjectID, projectInfoDelta); err != nil {
		blog.Errorf("cleaner: try update project(%s) info basic according task(%s) with delta(%+v) failed: %v",
			tb.Client.ProjectID, taskID, projectInfoDelta, err)
	}

	blog.Infof("cleaner: try releasing task(%s)", taskID)
	if err = egn.ReleaseTask(taskID); err != nil {
		blog.Errorf("cleaner: try releasing task(%s) failed: %v", taskID, err)
		return
	}

	blog.Infof("cleaner: success to release task(%s)", taskID)
	tb.Status.ShutDown()
	if err = c.layer.UpdateTaskBasic(tb); err != nil {
		blog.Errorf("cleaner: update task basic(%s) failed: %v", taskID, err)
		return
	}
	blog.Infof("cleaner: success to release and update task basic(%s)", taskID)
}

func (c *cleaner) onCleanNotify(tb *engine.TaskBasic) error {
	blog.Infof("cleaner: on notify ready clean task (%s)", tb.ID)
	if tb.Status.Released {
		blog.Infof("cleaner: on notify task (%s) is already released,do nothing", tb.ID)
		return nil
	}

	blog.Infof("cleaner: on notify check and find task(%s) is unreleased, prepare to collect data and release", tb.ID)
	egn, err := c.layer.GetEngineByTypeName(tb.Client.EngineName)
	if err != nil {
		blog.Errorf("cleaner: on notify try get task(%s) engine failed: %v", tb.ID, err)
		return err
	}

	// wg 不需要，只是为了保持clean的调用方式
	// var wg sync.WaitGroup
	// wg.Add(1)
	// c.clean(tb.ID, egn, &wg)
	c.clean(tb.ID, egn)
	// wg.Wait()

	return nil
}

func (c *cleaner) OnTaskStatus(tb *engine.TaskBasic, curstatus engine.TaskStatusType) error {
	blog.Infof("cleaner: ready notify to task(%s) engine(%s) queue(%s)", tb.ID, tb.Client.EngineName, tb.Client.QueueName)
	c.c <- tb
	return nil
}
