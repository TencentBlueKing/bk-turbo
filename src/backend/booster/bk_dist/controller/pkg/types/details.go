/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package types

import (
	"time"

	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
)

// WorkStatsDetail describe the work stats details
type WorkStatsDetail struct {
	CurrentTime      int64                       `json:"current_time"`
	WorkID           string                      `json:"work_id"`
	TaskID           string                      `json:"task_id"`
	Scene            string                      `json:"scene"`
	Status           string                      `json:"status"`
	Success          bool                        `json:"success"`
	RegisteredTime   int64                       `json:"registered_time"`
	UnregisteredTime int64                       `json:"unregistered_time"`
	StartTime        int64                       `json:"start_time"`
	EndTime          int64                       `json:"end_time"`
	JobRemoteOK      int                         `json:"job_remote_ok"`
	JobRemoteError   int                         `json:"job_remote_error"`
	JobLocalOK       int                         `json:"job_local_ok"`
	JobLocalError    int                         `json:"job_local_error"`
	Jobs             []*dcSDK.ControllerJobStats `json:"jobs"`
}

// 运行时信息
type RuntimeState struct {
	// 远程处理等待中
	RemoteWorkWaiting int
	// 远程处理持锁中
	RemoteWorkHolding int
	// 远程处理累计持锁
	RemoteWorkHeld int
	// remote_work_released
	// pre_work_waiting
	// pre_work_holding
	// pre_work_held
	// pre_work_released
	// local_work_waiting
	// local_work_holding
	// local_work_held
	// local_work_released
	// post_work_waiting
	// post_work_holding
	// post_work_held
	// post_work_released
	// dependent_waiting
	// sending
	// receiving
}

// GetRuntimeState get active jobs
func (wsd *WorkStatsDetail) GetRuntimeState() *RuntimeState {
	jobLeaveEndTime := time.Now().UnixNano()

	state := &RuntimeState{}

	for _, job := range wsd.Jobs {
		// zero 还在执行中 // 完成时间
		if leaveTime := job.LeaveTime.UnixNano(); leaveTime > 0 && leaveTime < jobLeaveEndTime {
			continue
		}

		if job.RemoteWorkEnterTime.UnixNano() > 0 && job.RemoteWorkLockTime.UnixNano() <= 0 {
			state.RemoteWorkWaiting++
		}

		if job.RemoteWorkLockTime.UnixNano() > 0 && job.RemoteWorkUnlockTime.UnixNano() <= 0 {
			state.RemoteWorkHolding++
		}

		if job.RemoteWorkLockTime.UnixNano() > 0 {
			state.RemoteWorkHeld++
		}
	}

	return state

}

type WorkStatsDetailList []*WorkStatsDetail

// Len return the length
func (wsl WorkStatsDetailList) Len() int {
	return len(wsl)
}

// Less do the comparing
func (wsl WorkStatsDetailList) Less(i, j int) bool {
	return wsl[i].RegisteredTime > wsl[j].RegisteredTime
}

// Swap do the data swap
func (wsl WorkStatsDetailList) Swap(i, j int) {
	wsl[i], wsl[j] = wsl[j], wsl[i]
}
