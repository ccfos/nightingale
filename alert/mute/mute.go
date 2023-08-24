package mute

import (
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
)

func IsMuted(rule *models.AlertRule, event *models.AlertCurEvent, targetCache *memsto.TargetCacheType, alertMuteCache *memsto.AlertMuteCacheType) bool {
	if rule.Disabled == 1 {
		return true
	}

	if TimeSpanMuteStrategy(rule, event) {
		return true
	}

	if IdentNotExistsMuteStrategy(rule, event, targetCache) {
		return true
	}

	if BgNotMatchMuteStrategy(rule, event, targetCache) {
		return true
	}

	if EventMuteStrategy(event, alertMuteCache) {
		return true
	}

	return false
}

// TimeSpanMuteStrategy 根据规则配置的告警生效时间段过滤,如果产生的告警不在规则配置的告警生效时间段内,则不告警,即被mute
// 时间范围，左闭右开，默认范围：00:00-24:00
func TimeSpanMuteStrategy(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	tm := time.Unix(event.TriggerTime, 0)
	triggerTime := tm.Format("15:04")
	triggerWeek := strconv.Itoa(int(tm.Weekday()))

	enableStime := strings.Fields(rule.EnableStime)
	enableEtime := strings.Fields(rule.EnableEtime)
	enableDaysOfWeek := strings.Split(rule.EnableDaysOfWeek, ";")
	length := len(enableDaysOfWeek)
	// enableStime,enableEtime,enableDaysOfWeek三者长度肯定相同，这里循环一个即可
	for i := 0; i < length; i++ {
		enableDaysOfWeek[i] = strings.Replace(enableDaysOfWeek[i], "7", "0", 1)
		if !strings.Contains(enableDaysOfWeek[i], triggerWeek) {
			continue
		}

		if enableStime[i] < enableEtime[i] {
			if enableEtime[i] == "23:59" {
				// 02:00-23:59，这种情况做个特殊处理，相当于左闭右闭区间了
				if triggerTime < enableStime[i] {
					// mute, 即没生效
					continue
				}
			} else {
				// 02:00-04:00 或者 02:00-24:00
				if triggerTime < enableStime[i] || triggerTime >= enableEtime[i] {
					// mute, 即没生效
					continue
				}
			}
		} else if enableStime[i] > enableEtime[i] {
			// 21:00-09:00
			if triggerTime < enableStime[i] && triggerTime >= enableEtime[i] {
				// mute, 即没生效
				continue
			}
		}

		// 到这里说明当前时刻在告警规则的某组生效时间范围内，即没有 mute，直接返回 false
		return false
	}

	return true
}

// IdentNotExistsMuteStrategy 根据ident是否存在过滤,如果ident不存在,则target_up的告警直接过滤掉
func IdentNotExistsMuteStrategy(rule *models.AlertRule, event *models.AlertCurEvent, targetCache *memsto.TargetCacheType) bool {
	ident, has := event.TagsMap["ident"]
	if !has {
		return false
	}
	_, exists := targetCache.Get(ident)
	// 如果是target_up的告警,且ident已经不存在了,直接过滤掉
	// 这里的判断有点太粗暴了,但是目前没有更好的办法
	if !exists && strings.Contains(rule.PromQl, "target_up") {
		logger.Debugf("[%s] mute: rule_eval:%d cluster:%s ident:%s", "IdentNotExistsMuteStrategy", rule.Id, event.Cluster, ident)
		return true
	}
	return false
}

// BgNotMatchMuteStrategy 当规则开启只在bg内部告警时,对于非bg内部的机器过滤
func BgNotMatchMuteStrategy(rule *models.AlertRule, event *models.AlertCurEvent, targetCache *memsto.TargetCacheType) bool {
	// 没有开启BG内部告警,直接不过滤
	if rule.EnableInBG == 0 {
		return false
	}

	ident, has := event.TagsMap["ident"]
	if !has {
		return false
	}

	target, exists := targetCache.Get(ident)
	// 对于包含ident的告警事件，check一下ident所属bg和rule所属bg是否相同
	// 如果告警规则选择了只在本BG生效，那其他BG的机器就不能因此规则产生告警
	if exists && target.GroupId != rule.GroupId {
		logger.Debugf("[%s] mute: rule_eval:%d cluster:%s", "BgNotMatchMuteStrategy", rule.Id, event.Cluster)
		return true
	}
	return false
}

func EventMuteStrategy(event *models.AlertCurEvent, alertMuteCache *memsto.AlertMuteCacheType) bool {
	mutes, has := alertMuteCache.Gets(event.GroupId)
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

	// 如果不是全局的，判断 匹配的 datasource id
	if !(len(mute.DatasourceIdsJson) != 0 && mute.DatasourceIdsJson[0] == 0) && event.DatasourceId != 0 {
		idm := make(map[int64]struct{}, len(mute.DatasourceIdsJson))
		for i := 0; i < len(mute.DatasourceIdsJson); i++ {
			idm[mute.DatasourceIdsJson[i]] = struct{}{}
		}

		// 判断 event.datasourceId 是否包含在 idm 中
		if _, has := idm[event.DatasourceId]; !has {
			return false
		}
	}

	var matchTime bool
	if mute.MuteTimeType == models.TimeRange {
		if ts < mute.Btime || ts > mute.Etime {
			return false
		}
		matchTime = true
	} else if mute.MuteTimeType == models.Periodic {
		tm := time.Unix(event.TriggerTime, 0)
		triggerTime := tm.Format("15:04")
		triggerWeek := strconv.Itoa(int(tm.Weekday()))

		for i := 0; i < len(mute.PeriodicMutesJson); i++ {
			if strings.Contains(mute.PeriodicMutesJson[i].EnableDaysOfWeek, triggerWeek) {
				if mute.PeriodicMutesJson[i].EnableStime == mute.PeriodicMutesJson[i].EnableEtime {
					matchTime = true
					break
				} else if mute.PeriodicMutesJson[i].EnableStime < mute.PeriodicMutesJson[i].EnableEtime {
					if triggerTime >= mute.PeriodicMutesJson[i].EnableStime && triggerTime < mute.PeriodicMutesJson[i].EnableEtime {
						matchTime = true
						break
					}
				} else {
					if triggerTime >= mute.PeriodicMutesJson[i].EnableStime || triggerTime < mute.PeriodicMutesJson[i].EnableEtime {
						matchTime = true
						break
					}
				}
			}
		}
	}
	if !matchTime {
		return false
	}

	var matchSeverity bool
	if len(mute.SeveritiesJson) > 0 {
		for _, s := range mute.SeveritiesJson {
			if event.Severity == s || s == 0 {
				matchSeverity = true
				break
			}
		}
	} else {
		matchSeverity = true
	}

	if !matchSeverity {
		return false
	}

	return common.MatchTags(event.TagsMap, mute.ITags)
}
