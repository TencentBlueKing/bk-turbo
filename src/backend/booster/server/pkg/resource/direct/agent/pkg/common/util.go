/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package common

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/util"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

const (
	LocalIPKKey = "BK_DISTCC_LOCAL_IP"
)

// get local ip from env first, if not exists or empty, then get from net
func GetLocalIP() (string, error) {
	ip := os.Getenv(LocalIPKKey)
	if ip != "" {
		return ip, nil
	}

	ips := util.GetIPAddress()
	if len(ips) == 0 {
		return "", fmt.Errorf("get local IP failed, the client will exit")
	}

	return ips[0], nil
}

// ListApplicationByNameWindows
func ListApplicationByNameWindows(processName string) (string, error) {
	blog.Infof("ListApplicationByNameWindows with process[%s]", processName)

	condition := fmt.Sprintf("imagename eq %s", processName)
	cmd := exec.Command("tasklist", "/fi", condition)
	//buf := bytes.NewBuffer(make([]byte, 10240))
	//cmd.Stdout = buf
	data, err := cmd.Output()
	if err != nil {
		blog.Infof("failed to tasklist /fi %s for err[%v]", condition, err)
		return "", err
	} else {
		return (string)(data), nil
	}
}

// ListApplicationByNameUnix
func ListApplicationByNameUnix(processName string) (string, error) {
	blog.Infof("ListApplicationByNameUnix with process[%s]", processName)

	command := fmt.Sprintf("ps aux|grep -F \"%s\"", processName)
	cmd := exec.Command("/bin/bash", "-c", command)
	data, err := cmd.Output()
	if err != nil {
		blog.Infof("failed to execute %s for err[%v]", command, err)
		return "", err
	}
	return (string)(data), nil
}

// ListApplicationByNameAndPidWindows
func ListApplicationByNameAndPidWindows(processName string, pid string) (string, error) {
	blog.Infof("ListApplicationByNameAndPidWindows with process[%s] pid[%s]", processName, pid)

	condition1 := fmt.Sprintf("imagename eq %s", processName)
	condition2 := fmt.Sprintf("pid eq %s", pid)
	cmd := exec.Command("tasklist", "/fi", condition1, "/fi", condition2)

	data, err := cmd.Output()
	if err != nil {
		blog.Infof("failed to tasklist /fi %s /fi %s for err[%v]", condition1, condition2, err)
		return "", err
	} else {
		return (string)(data), nil
	}
}

// ListApplicationByNameAndPidUnix
func ListApplicationByNameAndPidUnix(processName string, pid string) (string, error) {
	blog.Infof("ListApplicationByNameAndPidUnix with process[%s] pid[%s]", processName, pid)

	command := fmt.Sprintf("ps aux|grep -F \"%s\"|grep -F \"%s\"|grep -v grep", processName, pid)
	cmd := exec.Command("/bin/bash", "-c", command)
	data, err := cmd.Output()
	if err != nil {
		blog.Infof("failed to execute %s for err[%v]", command, err)
		return "", err
	}
	return (string)(data), nil
}

// KillApplicationByNameWindows
func KillApplicationByNameWindows(processName string) error {
	blog.Infof("KillApplicationByNameWindows with process[%s]", processName)

	condition := fmt.Sprintf("imagename eq %s", processName)
	cmd := exec.Command("taskkill", "/f", "/fi", condition)
	err := cmd.Run()
	if err != nil {
		blog.Infof("failed to taskkill /f /fi %s for err[%v]", condition, err)
		return err
	} else {
		blog.Infof("succeed to taskkill /f /fi %s", condition)
		return nil
	}
}

// KillApplicationByNameUnix
func KillApplicationByNameUnix(processName string) error {
	blog.Infof("KillApplicationByNameUnix with process[%s]", processName)

	command := fmt.Sprintf("pkill \"%s\"", processName)
	cmd := exec.Command("/bin/bash", "-c", command)
	err := cmd.Run()
	if err != nil {
		blog.Infof("failed to pkill %s for err[%v]", processName, err)
		return err
	} else {
		blog.Infof("succeed to pkill %s for err[%v]", processName, err)
		return nil
	}
}

// GetTotalMemory get total memory(KB) for windows
func GetTotalMemory() (uint64, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	} else {
		return v.Total / 1024, nil
	}
}

// between 0 ~ 100
func GetTotalCPUUsage() (float64, error) {
	per, err := cpu.Percent(0, false)
	if err != nil {
		return 0.0, err
	}

	return per[0], err
}

// Exist check file exist
func Exist(filename string) (bool, error) {
	_, err := os.Stat(filename)
	if err == nil || os.IsExist(err) {
		return true, nil
	}

	return false, err
}
