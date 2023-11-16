package autoscaler

import (
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

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

	blog.Infof("compute core=%d", mean)
	// 扩容场景
	if mean > 2 {
		return mean
	}

	// 缩容场景
	// 1. 资源计算
	// 2. 按资源排空任务
	// 3. 缩容
	if mean <= 2 {
		totalSlots, _ := s.work.Local().Slots()
		remoteTotalSlots, _ := s.work.Remote().Slots()
		if remoteTotalSlots-totalSlots > 2 {
			return remoteTotalSlots - totalSlots - 2
		}
	}

	return 0
}
