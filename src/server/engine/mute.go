package engine

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

// 如果传入了clock这个可选参数，就表示使用这个clock表示的时间，否则就从event的字段中取TriggerTime
func IsMuted(event *models.AlertCurEvent, clock ...int64) bool {
	mutes, has := memsto.AlertMuteCache.Gets(event.GroupId)
	if !has || len(mutes) == 0 {
		return false
	}

	for i := 0; i < len(mutes); i++ {
		if matchMute(event, mutes[i], clock...) {
			return true
		}
	}

	return false
}

func matchMute(event *models.AlertCurEvent, mute *models.AlertMute, clock ...int64) bool {
	if mute.Disabled == 1 {
		return false
	}

	ts := event.TriggerTime
	if len(clock) > 0 {
		ts = clock[0]
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
