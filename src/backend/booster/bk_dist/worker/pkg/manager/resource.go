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

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

// define const strings
const ()

var (
	totalCPU    float64 = 0
	totalMemory float64 = 0
	totalDisk   float64 = 0

	maxCPUPercent = 0

	totalSpecifiedCPU    float64 = 0
	totalSpecifiedMemory float64 = 0
	totalSpecifiedDisk   float64 = 0

	currentAvailableCPU    float64 = 0
	currentAvailableMemory float64 = 0
	currentAvailableDisk   float64 = 0
)

func (o *tcpManager) initResourceConf() error {
	blog.Infof("[resource] init resource conf with:%+v", *o.conf)

	if o.conf.MaxCPUPercent == 0 || o.conf.MaxCPUPercent > 100 {
		return fmt.Errorf("resource MaxCPUPercent %d can't be over 100 or equal 0", o.conf.MaxCPUPercent)
	}

	if o.conf.MaxMemoryPercent == 0 || o.conf.MaxMemoryPercent > 100 {
		return fmt.Errorf("resource MaxMemoryPercent %d can't be over 100 or equal 0", o.conf.MaxMemoryPercent)
	}

	totalCPUNum := runtime.NumCPU()
	if o.conf.MaxCPUPercent < 0 {
		totalSpecifiedCPU = float64(totalCPUNum + o.conf.MaxCPUPercent)
		if totalSpecifiedCPU <= 0 {
			return fmt.Errorf("MaxCPUPercent %d too less to get available cpu", o.conf.MaxCPUPercent)
		}

		maxCPUPercent = int(totalSpecifiedCPU) * 100 / totalCPUNum

	} else {
		totalSpecifiedCPU = float64(totalCPUNum * o.conf.MaxCPUPercent / 100)
		maxCPUPercent = o.conf.MaxCPUPercent
	}
	totalCPU = float64(totalCPUNum)

	v, err := mem.VirtualMemory()
	if err != nil {
		blog.Warnf("[resource] get virtual memory failed with error:%v", err)
		return err
	}
	totalSpecifiedMemory = float64(v.Total / 1024 / 1024 * uint64(o.conf.MaxMemoryPercent) / 100)
	totalMemory = float64(v.Total / 1024 / 1024)

	blog.Infof("[resource] got toal specified cpu:%f memroy %f", totalSpecifiedCPU, totalSpecifiedMemory)

	return nil
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
	currentAvailableMemory = totalSpecifiedMemory - usedMemory
	if currentAvailableMemory < 0 {
		currentAvailableMemory = 0
	}

	usedCPUPercent, err1 := cpu.Percent(0, false)
	if err1 != nil {
		blog.Warnf("[resource] get cpu info failed with error:%v", err1)
		haserror = true
		err = err1
	}

	if int(usedCPUPercent[0]) < maxCPUPercent {
		currentAvailableCPU = totalCPU * (float64(maxCPUPercent) - usedCPUPercent[0]) / 100
	}

	if haserror {
		currentAvailableCPU = 0
		currentAvailableMemory = 0
	}

	return err
}

func (o *tcpManager) resourceAvailable() bool {
	o.updateAvailable()

	return currentAvailableCPU > 0 && currentAvailableMemory > 0
}
