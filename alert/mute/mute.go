package mute

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

// IsMuted 返回该事件是否被屏蔽、屏蔽原因、命中的屏蔽规则 id，以及屏蔽方式（models.MuteType*）。
// 规则自身失效等非用户屏蔽规则导致的屏蔽，屏蔽方式恒为 MuteTypeAll（事件与通知都屏蔽）。
func IsMuted(rule *models.AlertRule, event *models.AlertCurEvent, targetCache *memsto.TargetCacheType, alertMuteCache *memsto.AlertMuteCacheType) (bool, string, int64, int) {
	if rule.Disabled == 1 {
		return true, "rule disabled", 0, models.MuteTypeAll
	}

	if TimeSpanMuteStrategy(rule, event) {
		return true, "rule is not effective for period of time, was muted", 0, models.MuteTypeAll
	}

	if IdentNotExistsMuteStrategy(rule, event, targetCache) {
		return true, "ident not exists, was muted", 0, models.MuteTypeAll
	}

	if BgNotMatchMuteStrategy(rule, event, targetCache) {
		return true, "ident not match busigroup, was muted", 0, models.MuteTypeAll
	}

	hit, muteId, muteType := EventMuteStrategy(event, alertMuteCache)
	if hit {
		return true, "match mute rule", muteId, muteType
	}

	return false, "", 0, models.MuteTypeAll
}

// TimeSpanMuteStrategy 根据规则配置的告警生效时间段过滤,如果产生的告警不在规则配置的告警生效时间段内,则不告警,即被mute
// 时间范围，左闭右开，默认范围：00:00-24:00
// 如果规则配置了时区，则在该时区下进行时间判断；如果时区为空，则使用系统时区
func TimeSpanMuteStrategy(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	// 确定使用的时区
	var targetLoc *time.Location
	var err error

	timezone := rule.TimeZone
	if timezone == "" {
		// 如果时区为空，使用系统时区（保持原有逻辑）
		targetLoc = time.Local
	} else {
		// 加载规则配置的时区
		targetLoc, err = time.LoadLocation(timezone)
		if err != nil {
			// 如果时区加载失败，记录错误并使用系统时区
			logger.Warningf("Failed to load timezone %s for rule %d, using system timezone: %v", timezone, rule.Id, err)
			targetLoc = time.Local
		}
	}

	// 将触发时间转换到目标时区
	tm := time.Unix(event.TriggerTime, 0).In(targetLoc)
	triggerTime := tm.Format("15:04")
	triggerWeek := strconv.Itoa(int(tm.Weekday()))

	if rule.EnableDaysOfWeek == "" {
		// 如果规则没有配置生效时间，则默认全天生效

		return false
	}

	enableStime := strings.Fields(rule.EnableStime)
	enableEtime := strings.Fields(rule.EnableEtime)
	enableDaysOfWeek := strings.Split(rule.EnableDaysOfWeek, ";")
	length := len(enableDaysOfWeek)
	// 正常情况下 enableStime、enableEtime、enableDaysOfWeek 段数相同；
	// 但历史脏数据或异常写入可能不一致，循环内已对越界做兜底跳过（见下），避免 panic
	for i := 0; i < length; i++ {
		if i >= len(enableStime) || i >= len(enableEtime) {
			continue
		}
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
		logger.Debugf("alert_eval_%d [IdentNotExistsMuteStrategy] mute: cluster:%s ident:%s", rule.Id, event.Cluster, ident)
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
	if exists && !target.MatchGroupId(rule.GroupId) {
		logger.Debugf("alert_eval_%d [BgNotMatchMuteStrategy] mute: cluster:%s", rule.Id, event.Cluster)
		return true
	}
	return false
}

// EventMuteStrategy 判断事件是否命中业务组屏蔽规则，返回是否命中、命中规则 id、以及生效的屏蔽方式（models.MuteType*）。
// 当事件同时命中多条规则时，更强的屏蔽方式优先：只要存在任一「屏蔽事件与通知」命中即返回 MuteTypeAll，
// 仅当所有命中规则都是「只屏蔽通知」时才返回 MuteTypeNotifyOnly，避免结果受规则在缓存中的先后顺序影响。
func EventMuteStrategy(event *models.AlertCurEvent, alertMuteCache *memsto.AlertMuteCacheType) (bool, int64, int) {
	mutes, has := alertMuteCache.Gets(event.GroupId)
	if !has || len(mutes) == 0 {
		return false, 0, models.MuteTypeAll
	}

	var (
		notifyOnlyHit    bool
		notifyOnlyMuteId int64
	)
	for i := 0; i < len(mutes); i++ {
		matched, _ := MatchMute(event, mutes[i])
		if !matched {
			continue
		}
		if mutes[i].MuteType != models.MuteTypeNotifyOnly {
			// 命中完全屏蔽规则，直接以最强屏蔽方式返回
			return true, mutes[i].Id, models.MuteTypeAll
		}
		if !notifyOnlyHit {
			notifyOnlyHit = true
			notifyOnlyMuteId = mutes[i].Id
		}
	}

	if notifyOnlyHit {
		return true, notifyOnlyMuteId, models.MuteTypeNotifyOnly
	}
	return false, 0, models.MuteTypeAll
}

// MatchMute 如果传入了clock这个可选参数，就表示使用这个clock表示的时间，否则就从event的字段中取TriggerTime
func MatchMute(event *models.AlertCurEvent, mute *models.AlertMute, clock ...int64) (bool, error) {
	if mute.Disabled == 1 {
		return false, errors.New("mute is disabled")
	}

	// 如果不是全局的，判断 匹配的 datasource id
	if len(mute.DatasourceIdsJson) != 0 && mute.DatasourceIdsJson[0] != 0 && event.DatasourceId != 0 {
		if !slices.Contains(mute.DatasourceIdsJson, event.DatasourceId) {
			return false, errors.New("datasource id not match")
		}
	}

	if mute.MuteTimeType == models.TimeRange {
		if !mute.IsWithinTimeRange(event.TriggerTime) {
			return false, errors.New("event trigger time not within mute time range")
		}
	} else if mute.MuteTimeType == models.Periodic {
		ts := event.TriggerTime
		if len(clock) > 0 {
			ts = clock[0]
		}

		if !mute.IsWithinPeriodicMute(ts) {
			return false, errors.New("event trigger time not within periodic mute range")
		}
	} else {
		logger.Warningf("mute time type invalid, %d", mute.MuteTimeType)
		return false, errors.New("mute time type invalid")
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
		return false, errors.New("event severity not match mute severity")
	}

	if len(mute.ITags) == 0 {
		return true, nil
	}
	if !common.MatchTags(event.TagsMap, mute.ITags) {
		return false, errors.New("event tags not match mute tags")
	}
	return true, nil
}
