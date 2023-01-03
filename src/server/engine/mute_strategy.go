package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

var AlertMuteStrategies = AlertMuteStrategiesType{&TimeEffectiveMuteStrategy{}, &BgNotMatchMuteStrategy{}, &MuteStrategy{}}

type AlertMuteStrategiesType []AlertMuteStrategy

func (ss AlertMuteStrategiesType) IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	for _, s := range ss {
		if s.IsMuted(rule, event) {
			return true
		}
	}
	return false
}

// AlertMuteStrategy 是过滤event的抽象,当返回true时,表示该告警时间由于某些原因不需要告警
type AlertMuteStrategy interface {
	IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool
}

// TimeEffectiveMuteStrategy 根据规则配置的告警时间过滤,如果产生的告警不在规则配置的告警时间内,则不告警
type TimeEffectiveMuteStrategy struct{}

func (s *TimeEffectiveMuteStrategy) IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	if rule.Disabled == 1 {
		return true
	}

	tm := time.Unix(event.TriggerTime, 0)
	triggerTime := tm.Format("15:04")
	triggerWeek := strconv.Itoa(int(tm.Weekday()))

	if rule.EnableStime <= rule.EnableEtime {
		if triggerTime < rule.EnableStime || triggerTime > rule.EnableEtime {
			return true
		}
	} else {
		if triggerTime < rule.EnableStime && triggerTime > rule.EnableEtime {
			return true
		}
	}

	rule.EnableDaysOfWeek = strings.Replace(rule.EnableDaysOfWeek, "7", "0", 1)
	return !strings.Contains(rule.EnableDaysOfWeek, triggerWeek)
}

// BgNotMatchMuteStrategy 当规则开启只在bg内部告警时,对于非bg内部的机器过滤
type BgNotMatchMuteStrategy struct{}

func (s *BgNotMatchMuteStrategy) IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	ident, has := event.TagsMap["ident"]
	if !has {
		return false
	}

	if rule.EnableInBG == 0 {
		return false
	}

	target, exists := memsto.TargetCache.Get(ident)
	if exists {
		// 对于包含ident的告警事件，check一下ident所属bg和rule所属bg是否相同
		// 如果告警规则选择了只在本BG生效，那其他BG的机器就不能因此规则产生告警
		if target.GroupId != rule.GroupId {
			logger.Debugf("event_enable_in_bg: rule_eval:%d cluster:%s", rule.Id, event.Cluster)
			return true
		}
	} else {
		if strings.Contains(rule.PromQl, "target_up") {
			// target 已经不存在了，可能是被删除了
			return true
		}
	}
	return false
}

type MuteStrategy struct{}

func (s *MuteStrategy) IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	return IsMuted(event)
}
