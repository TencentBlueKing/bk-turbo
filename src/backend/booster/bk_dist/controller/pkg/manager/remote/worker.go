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
	"net"

	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// Status save worker status
type Status int

// define file send status
const (
	Init Status = iota
	Detecting
	DetectSucceed
	DetectFailed
	Refused
	InService
	Unknown = 99
)

var (
	statusMap = map[Status]string{
		Init:          "init",
		Detecting:     "detecting",
		DetectSucceed: "detectsucceed",
		DetectFailed:  "detectfailed",
		Refused:       "refused",
		InService:     "inservice",
		Unknown:       "unknown",
	}
)

// String return the string of FileSendStatus
func (f Status) String() string {
	if v, ok := statusMap[f]; ok {
		return v
	}

	return "unknown"
}

// worker describe the worker information includes the host details and the slots status
// and it is the caller's responsibility to ensure the lock.
type worker struct {
	disabled      bool
	host          *dcProtocol.Host
	totalSlots    int
	occupiedSlots int
	// > 100MB
	largefiles          []string
	largefiletotalsize  uint64
	continuousNetErrors int
	dead                bool

	// for new feature
	conn     *net.TCPConn
	status   Status
	lasttime int64
	priority int
}

func (wr *worker) occupySlot() error {
	wr.occupiedSlots++
	return nil
}

func (wr *worker) freeSlot() error {
	wr.occupiedSlots--
	return nil
}

func (wr *worker) hasFile(f string) bool {
	if len(wr.largefiles) > 0 {
		for _, v := range wr.largefiles {
			if v == f {
				return true
			}
		}
	}

	return false
}

func (wr *worker) copy() *worker {
	return &worker{
		disabled:            wr.disabled,
		host:                wr.host,
		totalSlots:          wr.totalSlots,
		occupiedSlots:       wr.occupiedSlots,
		largefiles:          wr.largefiles,
		largefiletotalsize:  wr.largefiletotalsize,
		continuousNetErrors: wr.continuousNetErrors,
		dead:                wr.dead,
	}
}

func (wr *worker) invalid() bool {
	return wr.disabled || wr.dead
}

func (wr *worker) close() error {
	if wr.conn != nil {
		blog.Infof("worker: ready close worker %s now", wr.conn.RemoteAddr().String())
		err := wr.conn.Close()
		wr.conn = nil
		return err
	}

	return nil
}

func (wr *worker) resetSlot(reason string) error {
	blog.Infof("worker: ready reset worker %s for [%s]", wr.host.Server, reason)

	wr.close()
	wr.status = Init
	wr.priority = dcSDK.PriorityUnKnown
	wr.lasttime = 0

	return nil
}
