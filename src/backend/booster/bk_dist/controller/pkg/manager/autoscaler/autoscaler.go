// Package autoscaler is ..
package autoscaler

import (
	"context"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/manager/basic"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	v2 "github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/pkg/api/v2"
)

type scaler struct {
	ctx         context.Context
	workID      string
	work        *types.Work
	lastPending int
	lastTime    time.Time
	data        []*types.RuntimeState
}

func NewMgr(ctx context.Context, work *types.Work) types.AutoscalerMgr {
	return &scaler{
		ctx:      ctx,
		workID:   work.ID(),
		work:     work,
		data:     []*types.RuntimeState{},
		lastTime: time.Now(),
	}
}

func (s *scaler) ScaleUp(ctx context.Context, cores int) error {
	req := &v2.ParamApply{
		RequestCPU:    cores,
		RequestMemory: cores * 1024 * 2, // 内存是CPU的2倍
	}
	resp, err := s.work.Resource().Apply(req, true)
	blog.Infof("scale up core=%d, %s, %s", cores, resp, err)
	if err != nil {
		return err
	}
	s.lastTime = time.Now()
	return nil
}

func (s *scaler) ScaleDown(ctx context.Context, cores int) error {
	err := s.work.Resource().Release(nil)
	if err != nil {
		return err
	}

	blog.Infof("scale down core=%d, err, %s", cores, err)
	s.lastTime = time.Now()
	return nil
}

func (s *scaler) Run() {
	blog.Infof("do %s autoscaling", s.workID)
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			blog.Infof("close work autoscaling: %s", s.workID)
			return
		case <-ticker.C:
			totalSlots, occupiedSlots := s.work.Local().Slots()
			basic := s.work.Basic().(*basic.Mgr)
			remoteTotalSlots, remoteOccupiedSlots := s.work.Remote().Slots()
			blog.Infof("do %s autoscaling, hosts=%s, local_totalSlots=%d, local_occupiedSlots=%d, alive=%d, remoteTotalSlots=%d, remoteOccupiedSlots=%d, state=%s",
				s.workID,
				s.work.Resource().GetHosts(),
				basic.Alive(),
				totalSlots,
				occupiedSlots,
				remoteTotalSlots,
				remoteOccupiedSlots,
				s.work.Basic().GetDetails(0).GetRuntimeState(),
			)
			s.data = append(s.data, s.work.Basic().GetDetails(0).GetRuntimeState())

			cores := s.Compute()
			if cores == 0 {
				blog.Errorf("cores == 0, ignore scale up")
				continue
			}

			if cores > 0 {
				err := s.ScaleUp(s.ctx, cores)
				if err != nil {
					blog.Errorf("scale up %s, core=%d, err: %s", s.workID, cores, err)
				} else {
					blog.Info("scale up %s, core=%d, done", s.workID, cores)
				}
				continue
			}

			if cores < 0 {
				err := s.ScaleDown(s.ctx, cores)
				if err != nil {
					blog.Errorf("scale down %s, core=%d, err: %s", s.workID, cores, err)
				} else {
					blog.Info("scale down %s, core=%d, done", s.workID, cores)
				}
				continue
			}
		}
	}
}
