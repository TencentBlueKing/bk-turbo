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
	"runtime"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

// define const strings
const (
	updateResourceIntervalTime = 50 * time.Millisecond
)

var (
	totalCPU    float64 = 0
	totalMemory float64 = 0
	totalDisk   float64 = 0

	// for execute limit
	maxExecuteCPUPercent = 0

	totalExecuteCPU    float64 = 0
	totalExecuteMemory float64 = 0
	totalExecuteDisk   float64 = 0

	currentAvailableExecuteCPU    float64 = 0
	currentAvailableExecuteMemory float64 = 0
	currentAvailableExecuteDisk   float64 = 0

	// for slot limit
	maxSlotCPUPercent = 0

	totalSlotCPU    float64 = 0
	totalSlotMemory float64 = 0
	totalSlotDisk   float64 = 0

	currentAvailableSlotCPU    float64 = 0
	currentAvailableSlotMemory float64 = 0
	currentAvailableSlotDisk   float64 = 0
)

func (o *tcpManager) initResourceConf() error {
	blog.Infof("[resource] init resource conf with:%+v", *o.conf)

	if o.conf.MaxExecuteCPUPercent == 0 || o.conf.MaxExecuteCPUPercent > 100 {
		return fmt.Errorf("resource MaxExecuteCPUPercent %d can't be over 100 or equal 0", o.conf.MaxExecuteCPUPercent)
	}

	if o.conf.MaxExecuteMemoryPercent == 0 || o.conf.MaxExecuteMemoryPercent > 100 {
		return fmt.Errorf("resource MaxExecuteMemoryPercent %d can't be over 100 or equal 0", o.conf.MaxExecuteMemoryPercent)
	}

	if o.conf.MaxSlotCPUPercent == 0 || o.conf.MaxSlotCPUPercent > 100 {
		return fmt.Errorf("resource MaxSlotCPUPercent %d can't be over 100 or equal 0", o.conf.MaxSlotCPUPercent)
	}

	if o.conf.MaxSlotMemoryPercent == 0 || o.conf.MaxSlotMemoryPercent > 100 {
		return fmt.Errorf("resource MaxSlotMemoryPercent %d can't be over 100 or equal 0", o.conf.MaxSlotMemoryPercent)
	}

	totalCPUNum := runtime.NumCPU()
	totalCPU = float64(totalCPUNum)

	if o.conf.MaxExecuteCPUPercent < 0 {
		totalExecuteCPU = float64(totalCPUNum + o.conf.MaxExecuteCPUPercent)
		if totalExecuteCPU <= 0 {
			return fmt.Errorf("MaxExecuteCPUPercent %d too less to get available cpu", o.conf.MaxExecuteCPUPercent)
		}

		maxExecuteCPUPercent = int(totalExecuteCPU) * 100 / totalCPUNum
	} else {
		totalExecuteCPU = float64(totalCPUNum * o.conf.MaxExecuteCPUPercent / 100)
		maxExecuteCPUPercent = o.conf.MaxExecuteCPUPercent
	}

	if o.conf.MaxSlotCPUPercent < 0 {
		totalSlotCPU = float64(totalCPUNum + o.conf.MaxSlotCPUPercent)
		if totalSlotCPU <= 0 {
			return fmt.Errorf("MaxSlotCPUPercent %d too less to get available cpu", o.conf.MaxSlotCPUPercent)
		}

		maxSlotCPUPercent = int(totalSlotCPU) * 100 / totalCPUNum
	} else {
		totalSlotCPU = float64(totalCPUNum * o.conf.MaxSlotCPUPercent / 100)
		maxSlotCPUPercent = o.conf.MaxSlotCPUPercent
	}

	v, err := mem.VirtualMemory()
	if err != nil {
		blog.Warnf("[resource] get virtual memory failed with error:%v", err)
		return err
	}
	totalMemory = float64(v.Total / 1024 / 1024)

	totalExecuteMemory = float64(v.Total / 1024 / 1024 * uint64(o.conf.MaxExecuteMemoryPercent) / 100)
	totalSlotMemory = float64(v.Total / 1024 / 1024 * uint64(o.conf.MaxSlotMemoryPercent) / 100)

	blog.Infof("[resource] got toal execute cpu:%f memory %f, total slot cpu:%f memory %f",
		totalExecuteCPU, totalExecuteMemory, totalSlotCPU, totalSlotMemory)
	blog.Infof("[resource] got max execute cpu percent:%d max slot cpu percent:%d",
		maxExecuteCPUPercent, maxSlotCPUPercent)

	return nil
}

func (o *tcpManager) resourceTimer() {
	blog.Infof("[resource] start resource update timer")
	tick := time.NewTicker(updateResourceIntervalTime)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			o.updateAvailable()
		}
	}
}

func (o *tcpManager) updateAvailable() error {
	haserror := false
	var err error
	v, err1 := mem.VirtualMemory()
	if err1 != nil {
		blog.Warnf("[resource] get virtual memory failed with error:%v", err1)
		haserror = true
		err = err1
	}
	usedMemory := float64(v.Used / 1024 / 1024)
	currentAvailableExecuteMemory = totalExecuteMemory - usedMemory
	if currentAvailableExecuteMemory < 0 {
		currentAvailableExecuteMemory = 0
	}

	currentAvailableSlotMemory = totalSlotMemory - usedMemory
	if currentAvailableSlotMemory < 0 {
		currentAvailableSlotMemory = 0
	}

	usedCPUPercent, err1 := cpu.Percent(0, false)
	if err1 != nil {
		blog.Warnf("[resource] get cpu info failed with error:%v", err1)
		haserror = true
		err = err1
	}

	if int(usedCPUPercent[0]) < maxExecuteCPUPercent {
		currentAvailableExecuteCPU = totalCPU * (float64(maxExecuteCPUPercent) - usedCPUPercent[0]) / 100
	} else {
		currentAvailableExecuteCPU = 0
	}

	if int(usedCPUPercent[0]) < maxSlotCPUPercent {
		currentAvailableSlotCPU = totalCPU * (float64(maxSlotCPUPercent) - usedCPUPercent[0]) / 100
	} else {
		currentAvailableSlotCPU = 0
	}

	if haserror {
		currentAvailableExecuteCPU = 0
		currentAvailableExecuteMemory = 0
		currentAvailableSlotCPU = 0
		currentAvailableSlotMemory = 0
	}

	return err
}

func (o *tcpManager) resourceAvailable() bool {
	return currentAvailableExecuteCPU > 0 && currentAvailableExecuteMemory > 0
}
