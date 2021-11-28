package engine

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

func isMuted(event *models.AlertCurEvent) bool {
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

func matchMute(event *models.AlertCurEvent, mute *models.AlertMute) bool {
	if event.TriggerTime < mute.Btime || event.TriggerTime > mute.Etime {
		return false
	}

	return matchTags(event.TagsMap, mute.ITags)
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
