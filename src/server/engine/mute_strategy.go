package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

var AlertMuteStrategies = AlertMuteStrategiesType{
	&TimeNonEffectiveMuteStrategy{},
	&IdentNotExistsMuteStrategy{},
	&BgNotMatchMuteStrategy{},
	&EventMuteStrategy{},
}

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

// TimeNonEffectiveMuteStrategy 根据规则配置的告警时间过滤,如果产生的告警不在规则配置的告警时间内,则不告警
type TimeNonEffectiveMuteStrategy struct{}

func (s *TimeNonEffectiveMuteStrategy) IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool {
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

// IdentNotExistsMuteStrategy 根据ident是否存在过滤,如果ident不存在,则target_up的告警直接过滤掉
type IdentNotExistsMuteStrategy struct{}

func (s *IdentNotExistsMuteStrategy) IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	ident, has := event.TagsMap["ident"]
	if !has {
		return false
	}
	_, exists := memsto.TargetCache.Get(ident)
	// 如果是target_up的告警,且ident已经不存在了,直接过滤掉
	// 这里的判断有点太粗暴了,但是目前没有更好的办法
	if !exists && strings.Contains(rule.PromQl, "target_up") {
		logger.Debugf("[%T] mute: rule_eval:%d cluster:%s ident:%s", s, rule.Id, event.Cluster, ident)
		return true
	}
	return false
}

// BgNotMatchMuteStrategy 当规则开启只在bg内部告警时,对于非bg内部的机器过滤
type BgNotMatchMuteStrategy struct{}

func (s *BgNotMatchMuteStrategy) IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	// 没有开启BG内部告警,直接不过滤
	if rule.EnableInBG == 0 {
		return false
	}

	ident, has := event.TagsMap["ident"]
	if !has {
		return false
	}

	target, exists := memsto.TargetCache.Get(ident)
	// 对于包含ident的告警事件，check一下ident所属bg和rule所属bg是否相同
	// 如果告警规则选择了只在本BG生效，那其他BG的机器就不能因此规则产生告警
	if exists && target.GroupId != rule.GroupId {
		logger.Debugf("[%T] mute: rule_eval:%d cluster:%s", s, rule.Id, event.Cluster)
		return true
	}
	return false
}

type EventMuteStrategy struct{}

func (s *EventMuteStrategy) IsMuted(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	mutes, has := memsto.AlertMuteCache.Gets(event.GroupId)
	if !has || len(mutes) == 0 {
		return false
	}

	for i := 0; i < len(mutes); i++ {
		if matchMute(event, mutes[i]) {
			return true
		}
	}

	return false
}

// matchMute 如果传入了clock这个可选参数，就表示使用这个clock表示的时间，否则就从event的字段中取TriggerTime
func matchMute(event *models.AlertCurEvent, mute *models.AlertMute, clock ...int64) bool {
	if mute.Disabled == 1 {
		return false
	}

	ts := event.TriggerTime
	if len(clock) > 0 {
		ts = clock[0]
	}

	// 如果不是全局的，判断 cluster
	if mute.Cluster != models.ClusterAll {
		// mute.Cluster 是一个字符串，可能是多个cluster的组合，比如"cluster1 cluster2"
		clusters := strings.Fields(mute.Cluster)
		cm := make(map[string]struct{}, len(clusters))
		for i := 0; i < len(clusters); i++ {
			cm[clusters[i]] = struct{}{}
		}

		// 判断event.Cluster是否包含在cm中
		if _, has := cm[event.Cluster]; !has {
			return false
		}
	}

	if ts < mute.Btime || ts > mute.Etime {
		return false
	}

	return matchTags(event.TagsMap, mute.ITags)
}

func matchTag(value string, filter models.TagFilter) bool {
	switch filter.Func {
	case "==":
		return filter.Value == value
	case "!=":
		return filter.Value != value
	case "in":
		_, has := filter.Vset[value]
		return has
	case "not in":
		_, has := filter.Vset[value]
		return !has
	case "=~":
		return filter.Regexp.MatchString(value)
	case "!~":
		return !filter.Regexp.MatchString(value)
	}
	// unexpect func
	return false
}

func matchTags(eventTagsMap map[string]string, itags []models.TagFilter) bool {
	for _, filter := range itags {
		value, has := eventTagsMap[filter.Key]
		if !has {
			return false
		}
		if !matchTag(value, filter) {
			return false
		}
	}
	return true
}
