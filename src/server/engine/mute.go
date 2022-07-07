package engine

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

// 如果传入了clock这个可选参数，就表示使用这个clock表示的时间，否则就从event的字段中取TriggerTime
func isMuted(event *models.AlertCurEvent, clock ...int64) bool {
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
	ts := event.TriggerTime
	if len(clock) > 0 {
		ts = clock[0]
	}

	if ts < mute.Btime || ts > mute.Etime {
		return false
	}
	tg := event.TagsMap
	tg["rulename"] = event.RuleName
	return matchTags(tg, mute.ITags)
}

func matchTags(eventTagsMap map[string]string, itags []models.TagFilter) bool {
	for i := 0; i < len(itags); i++ {
		filter := itags[i]
		value, exists := eventTagsMap[filter.Key]
		if !exists {
			return false
		}

		if filter.Func == "==" {
			// ==
			if filter.Value != value {
				return false
			}
		} else if filter.Func == "in" {
			// in
			if _, has := filter.Vset[value]; !has {
				return false
			}
		} else {
			// =~
			if !filter.Regexp.MatchString(value) {
				return false
			}
		}
	}

	return true
}
