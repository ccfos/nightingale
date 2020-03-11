package cron

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/mcache"
	"github.com/didi/nightingale/src/modules/monapi/notify"
	"github.com/didi/nightingale/src/modules/monapi/redisc"
)

const (
	PrefixRecoveryTime = "/n9e/recovery/time/"
	PrefixAlertTime    = "/n9e/alert/time/"
	PrefixAlertUpgrade = "/n9e/alert/upgrade/"
)

func consume(event *model.Event) {
	if event == nil {
		return
	}

	// 这个监控指标已经被屏蔽了，设置状态为"已屏蔽"，其他啥都不用干了
	if IsMaskEvent(event) {
		SetEventStatus(event, model.STATUS_MASK)
		return
	}

	if event.NeedUpgrade == 1 {
		needUpgrade, needNotify := isAlertUpgrade(event)
		if needUpgrade {
			var alertUpgrade model.EventAlertUpgrade
			if err := json.Unmarshal([]byte(event.AlertUpgrade), &alertUpgrade); err != nil {
				logger.Errorf("AlertUpgrade unmarshal failed, event: %+v, err: %v", event, err)
				return
			}

			if event.EventType == config.ALERT {
				err := model.UpdateEventCurPriority(event.HashId, alertUpgrade.Level)
				if err != nil {
					logger.Errorf("UpdateEventCurPriority failed, err: %v, event: %+v", err, event)
					return
				}
			}
			err := model.UpdateEventPriority(event.Id, alertUpgrade.Level)
			if err != nil {
				logger.Errorf("UpdateEventPriority failed, err: %v, event: %+v", err, event)
				return
			}
			event.Priority = alertUpgrade.Level

			SetEventStatus(event, model.STATUS_UPGRADE)

			if needNotify {
				if event.EventType == config.ALERT && NeedCallback(event.Sid) {
					if err := PushCallbackEvent(event); err != nil {
						logger.Errorf("push event to callback queue failed, callbackEvent: %+v", event)
					}
					logger.Infof("push event to callback queue succ, event hashid: %v", event.HashId)

					SetEventStatus(event, model.STATUS_CALLBACK)
				}

				go notify.DoNotify(true, event)
				SetEventStatus(event, model.STATUS_SEND)
				return
			}

			SetEventStatus(event, model.STATUS_CONVERGE)
			return
		}
	}

	if isInConverge(event, false) {
		SetEventStatus(event, model.STATUS_CONVERGE)
		return
	}

	if event.EventType == config.ALERT && NeedCallback(event.Sid) {
		if err := PushCallbackEvent(event); err != nil {
			logger.Errorf("push event to callback queue failed, callbackEvent: %+v", event)
		}
		logger.Infof("push event to callback queue succ, event hashid: %v", event.HashId)

		SetEventStatus(event, model.STATUS_CALLBACK)
	}

	// 没有配置报警接收人，修改event状态为无接收人
	if strings.TrimSpace(event.Users) == "[]" && strings.TrimSpace(event.Groups) == "[]" {
		SetEventStatus(event, model.STATUS_NONEUSER)
		return
	}

	go notify.DoNotify(false, event)
	SetEventStatus(event, model.STATUS_SEND)
}

// isInConverge 包含2种情况
// 1. 用户配置了N秒之内只报警M次
// 2. 用户配置了不发送recovery通知
func isInConverge(event *model.Event, isUpgrade bool) bool {
	stra, exists := mcache.StraCache.GetById(event.Sid)
	if !exists {
		logger.Errorf("sid not found, event: %+v", event)
		return false
	}

	eventString := PrefixRecoveryTime + fmt.Sprint(event.HashId)

	now := time.Now().Unix()

	if event.EventType == config.RECOVERY {
		redisc.SetWithTTL(eventString, now, 30*24*3600)
		if stra.RecoveryNotify == 0 {
			// 不发送recovery通知
			return true
		}

		return false
	}

	convergeInSeconds := int64(stra.Converge[0])
	convergeMaxCounts := int64(stra.Converge[1])

	// 最多报0次，相当于不报警，收敛该报警
	if convergeMaxCounts == 0 {
		return true
	}

	// 相当于没有配置收敛策略，不收敛
	if convergeInSeconds == 0 {
		return false
	}

	// 上次恢复的时间，第一次的话自然找不到上次恢复时间，就是0
	var recoveryTs int64
	if redisc.HasKey(eventString) {
		recoveryTs = redisc.GET(eventString)
	}

	// 举例，一个小时以内最多报1条，convergeInSeconds就是3600
	startTs := now - convergeInSeconds
	if startTs < recoveryTs {
		startTs = recoveryTs
	}

	cnt, err := model.EventCnt(event.HashId, model.ParseEtime(startTs), model.ParseEtime(now), isUpgrade)
	if err != nil {
		logger.Errorf("get event count failed, err: %v", err)
		return false
	}

	if cnt >= convergeMaxCounts {
		logger.Infof("converge max counts: %c reached, current: %v, event hashid: %v", convergeMaxCounts, cnt, event.HashId)
		return true
	}

	return false
}

// 三种情况，不需要升级报警
// 1，认领的报警不需要升级
// 2，忽略的报警不需要升级
// 3，屏蔽的报警不需要升级
func isAlertUpgrade(event *model.Event) (needUpgrade, needNotify bool) {
	alertUpgradeKey := PrefixAlertUpgrade + fmt.Sprint(event.HashId)
	eventAlertKey := PrefixAlertTime + fmt.Sprint(event.HashId)

	if event.EventType == config.RECOVERY {
		// 之前如果残留了upgrade的redis记录，现在恢复了，相当于一个新的周期要开始了，自然要删除老旧记录
		if redisc.HasKey(alertUpgradeKey) {
			err := redisc.DelKey(eventAlertKey)
			if err != nil {
				logger.Errorf("redis del eventAlertkey failed, key: %v, err: %v", eventAlertKey, err)
			}

			err = redisc.DelKey(alertUpgradeKey)
			if err != nil {
				logger.Errorf("redis del alertUpgradeKey failed, key: %v, err: %v", alertUpgradeKey, err)
			}

			// 之前升级过，即老板已经知道了，那现在恢复了，就需要把恢复通知发给老板
			// 如果配置了静默恢复呢？配置了升级的告警，显然是重要的告警，并且此时老板已经知道了，哪能静默恢复呢...
			// 老板收到升级告警了，但是恢复了之后，就一定要让他收到告警恢复的通知，忽略用户的"静默恢复"的配置项
			return true, true
		}

		// 之前没有升级过，老板压根不知道这个事，现在恢复了，自然也不需要知道
		return false, false
	}

	// 这是一个alert，not recovery，但是告警事件都找不到了，还升级通知个毛线
	eventCur, err := model.EventCurGet("hashid", event.HashId)
	if err != nil {
		logger.Errorf("AlertUpgrade failed:get event_cur failed, event: %+v, err: %v", event, err)
		return false, false
	}

	// 告警事件都找不到了，还升级通知个毛线
	if eventCur == nil {
		logger.Infof("AlertUpgrade failed:get event_cur is nil, event hashid: %v", event.HashId)
		return false, false
	}

	now := time.Now().Unix()

	// 升级配置解析失败...自然没法升级了
	var alertUpgrade model.EventAlertUpgrade
	if err = json.Unmarshal([]byte(event.AlertUpgrade), &alertUpgrade); err != nil {
		logger.Errorf("AlertUpgrade unmarshal failed, event: %+v, err: %v", event, err)
		return false, false
	}

	upgradeDuration := int64(alertUpgrade.Duration)

	// 说明告警已经被认领
	claimants := strings.TrimSpace(eventCur.Claimants)
	if claimants != "[]" && claimants != "" {
		return false, false
	}

	// 告警已经忽略了
	if eventCur.IgnoreAlert == 1 {
		return false, false
	}

	// 告警之后，比如30分钟没有处理，就需要升级，那首先得知道首次告警时间
	if !redisc.HasKey(eventAlertKey) {
		err := redisc.SetWithTTL(eventAlertKey, now, 30*24*3600)
		if err != nil {
			logger.Errorf("set eventAlertKey failed, eventAlertKey: %v, err: %v", eventAlertKey, err)
			return false, false
		}
	}

	// 比如：没到30分钟呢，不用升级
	firstAlertTime := redisc.GET(eventAlertKey)
	if now-firstAlertTime < upgradeDuration {
		return false, false
	}

	err = redisc.SetWithTTL(alertUpgradeKey, 1, 30*24*3600)
	if err != nil {
		logger.Errorf("set alertUpgradeKey failed, alertUpgradeKey: %v, err: %v", alertUpgradeKey, err)
		return false, false
	}

	// 还没有升级之前可能已经发过多次告警，并且已经触发了收敛，这时触发升级的告警，可千万不能被收敛
	// 比如1h内最多报1一次，在1分钟的时候触发告警并发送，6分钟、11分钟、16分钟的时候又触发但被收敛
	// 要求20分钟未处理则升级，虽然此时仍然在1h时间内，但是升级的情况需要单独来看之前是否有"已升级并且已发送"的事件
	// 显然，在这个场景下，前面只有"已发送"和"已收敛"的事件，没有"已升级并且已发送"的事件
	// 所以在21分钟的时候，应该触发升级并发送，在26分钟、31分钟的时候，都是"已升级并且已收敛"
	if isInConverge(event, true) {
		return true, false
	}

	return true, true
}

func SetEventStatus(event *model.Event, status string) {
	if err := model.SaveEventStatus(event.Id, status); err != nil {
		logger.Errorf("set event status fail, event: %+v, status: %v, err:%v", event, status, err)
	} else {
		logger.Infof("set event status succ, event hashid: %v, status: %v", event.HashId, status)
	}

	if event.EventType == config.ALERT {
		if err := model.SaveEventCurStatus(event.HashId, status); err != nil {
			logger.Errorf("set event_cur status fail, event: %+v, status: %v, err:%v", event, status, err)
		} else {
			logger.Infof("set event_cur status succ, event hashid: %v, status: %v", event.HashId, status)
		}
	}
}
