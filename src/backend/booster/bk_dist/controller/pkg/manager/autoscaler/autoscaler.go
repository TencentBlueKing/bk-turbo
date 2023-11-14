// Package autoscaler is ..
package autoscaler

import (
	"context"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/manager/basic"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
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

func (s *scaler) Compute() int {
	// 每2分钟最多执行一次扩缩容策略
	if time.Since(s.lastTime) < time.Minute*2 {
		return 0
	}

	// 数据过少，不能决策
	dateLen := len(s.data)
	if dateLen < 10 {
		return 0
	}

	sum := 0
	for i := dateLen - 10; i < dateLen; i++ {
		sum += s.data[i].RemoteWorkWaiting
	}

	mean := sum / 10
	// 扩容场景
	if s.lastPending == 0 || mean > 2 {
		return 1
	}

	// 缩容场景
	if mean <= 2 {
		totalSlots, _ := s.work.Local().Slots()
		remoteTotalSlots, _ := s.work.Remote().Slots()
		if remoteTotalSlots-totalSlots > 2 {
			return -1
		}
	}

	return 0
}

func (s *scaler) ScaleUp(ctx context.Context) error {
	info, err := s.work.Resource().Apply(nil, true)
	blog.Infof("scale up %s, %s", info, err)
	if err != nil {
		return err
	}
	s.lastTime = time.Now()
	return err
}

func (s *scaler) ScaleDown(ctx context.Context) error {
	err := s.work.Resource().Release(nil)
	if err != nil {
		return err
	}

	blog.Infof("scale up err, %s", err)
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

			if s.Compute() == 0 {
				continue
			}

			if s.Compute() > 0 {
				err := s.ScaleUp(s.ctx)
				if err != nil {
					blog.Errorf("scale up %s, err: %s", s.workID, err)
				} else {
					blog.Info("scale up %s, done", s.workID)
				}
				continue
			}

			if s.Compute() < 0 {
				err := s.ScaleDown(s.ctx)
				if err != nil {
					blog.Errorf("scale down %s, err: %s", s.workID, err)
				} else {
					blog.Info("scale down %s, done", s.workID)
				}
				continue
			}
		}
	}
}
