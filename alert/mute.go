package alert

import (
	"strings"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"
	"github.com/toolkits/pkg/logger"
)

func isEventMute(event *models.AlertEvent) bool {
	historyPoints, err := event.GetHistoryPoints()
	if err != nil {
		logger.Errorf("get event HistoryPoints:%+v failed, err: %v", event.HistoryPoints, err)
		return false
	}

	// 先去匹配一下metric为空的mute
	if matchMute("", event.ResIdent, event.TagMap, event.ResClasspaths) {
		return true
	}

	// 如果是与条件，就会有多个metric，任一个匹配了屏蔽规则都算被屏蔽
	for i := 0; i < len(historyPoints); i++ {
		if matchMute(historyPoints[i].Metric, event.ResIdent, event.TagMap, event.ResClasspaths) {
			return true
		}
	}

	resAndTags, exists := cache.ResTags.Get(event.ResIdent)
	if exists {
		if event.TriggerTime > resAndTags.Resource.MuteBtime && event.TriggerTime < resAndTags.Resource.MuteEtime {
			return true
		}
	}

	return false
}

func matchMute(metric, ident string, tags map[string]string, classpaths string) bool {
	filters, exists := cache.AlertMute.GetByKey(metric)
	if !exists {
		// 没有屏蔽规则跟这个事件相关
		return false
	}

	// 只要有一个屏蔽规则命中，那这个事件就是被屏蔽了
	for _, filter := range filters {
		if matchMuteOnce(filter, ident, tags, classpaths) {
			return true
		}
	}

	return false
}

func matchMuteOnce(filter cache.Filter, ident string, tags map[string]string, classpaths string) bool {
	if len(filter.ClasspathPrefix) != 0 && !strings.HasPrefix(classpaths, filter.ClasspathPrefix) && !strings.Contains(classpaths, " "+filter.ClasspathPrefix) {
		return false
	}

	if filter.ResReg != nil && !filter.ResReg.MatchString(ident) {
		// 比如屏蔽规则配置的是：c3-ceph.*
		// 当前事件的资源标识是：c4-ceph01.bj
		// 只要有任一点不满足，那这个屏蔽规则也没有继续验证下去的必要
		return false
	}

	// 每个mute中的tags都得出现在event.tags，否则就是不匹配
	return mapContains(tags, filter.TagsMap)
}

func mapContains(big, small map[string]string) bool {
	for tagk, tagv := range small {
		val, exists := big[tagk]
		if !exists {
			return false
		}

		if val != tagv {
			return false
		}
	}
	return true
}
