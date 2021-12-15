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

		// 重复通知的告警，应该用新的时间来判断是否生效和是否屏蔽，
		// 不能使用TriggerTime，因为TriggerTime是触发时的时间，是一个比较老的时间
		// 先发了告警，又做了屏蔽，本质是不想发了，如果继续用TriggerTime判断，就还是会发，不符合预期
		if isNoneffective(event.NotifyRepeatNext, rule) {
			continue
		}

		if isMuted(event, event.NotifyRepeatNext) {
			continue
		}

		fillUsers(event)
		notify(event)

		if err = event.IncRepeatStep(int64(rule.NotifyRepeatStep * 60)); err != nil {
			logger.Errorf("repeat: IncRepeatStep: %v", err)
		}
	}
}
