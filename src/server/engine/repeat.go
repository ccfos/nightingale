package engine

import (
	"context"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/toolkits/pkg/logger"
)

func loopRepeat(ctx context.Context) {
	duration := time.Duration(9000) * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(duration):
			repeat()
		}
	}
}

// 拉取未恢复的告警表中需要重复通知的数据
func repeat() {
	events, err := models.AlertCurEventNeedRepeat(config.C.ClusterName)
	if err != nil {
		logger.Errorf("repeat: AlertCurEventNeedRepeat: %v", err)
		return
	}

	if len(events) == 0 {
		return
	}

	for i := 0; i < len(events); i++ {
		event := events[i]
		rule := memsto.AlertRuleCache.Get(event.RuleId)
		if rule == nil {
			continue
		}

		if rule.NotifyRepeatStep == 0 {
			// 用户后来调整了这个字段，不让继续发送了
			continue
		}

		event.DB2Mem()

		if isNoneffective(event.TriggerTime, rule) {
			continue
		}

		if isMuted(event) {
			continue
		}

		fillUsers(event)
		notify(event)

		if err = event.IncRepeatStep(int64(rule.NotifyRepeatStep * 60)); err != nil {
			logger.Errorf("repeat: IncRepeatStep: %v", err)
		}
	}
}
