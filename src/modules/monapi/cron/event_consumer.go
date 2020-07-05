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

	// 配置了升级策略，但不代表每个事件都要升级，比如判断时间是否到了升级条件
	if event.NeedUpgrade == 1 {
		event.RealUpgrade = needUpgrade(event)
	}

	if event.RealUpgrade {
		// 确实需要升级的话，事件级别要改成升级之后的级别
		if err := updatePriority(event); err != nil {
			return
		}
		SetEventStatus(event, model.STATUS_UPGRADE)
	}

	if isInConverge(event) {
		SetEventStatus(event, model.STATUS_CONVERGE)
		return
	}

	if NeedCallback(event.Sid) {
		if err := PushCallbackEvent(event); err != nil {
			logger.Errorf("push event to callback queue failed, callbackEvent: %+v", event)
		}
		logger.Infof("push event to callback queue succ, event hashid: %v", event.HashId)

		SetEventStatus(event, model.STATUS_CALLBACK)
	}

	// 如果需要升级，需要在这个方法里把升级策略里配置的升级人员也解析出来
	if err := fillRecvs(event); err != nil {
		return
	}

	// 没有配置报警接收人，修改event状态为无接收人
	if len(event.RecvUserObjs) == 0 {
		SetEventStatus(event, model.STATUS_NONEUSER)
		return
	}

	go notify.DoNotify(event)
	SetEventStatus(event, model.STATUS_SEND)
}

func updatePriority(event *model.Event) error {
	var alertUpgrade model.EventAlertUpgrade
	if err := json.Unmarshal([]byte(event.AlertUpgrade), &alertUpgrade); err != nil {
		logger.Errorf("AlertUpgrade unmarshal failed, event: %+v, err: %v", event, err)
		return err
	}

	if event.EventType == config.ALERT {
		err := model.UpdateEventCurPriority(event.HashId, alertUpgrade.Level)
		if err != nil {
			logger.Errorf("UpdateEventCurPriority failed, err: %v, event: %+v", err, event)
			return err
		}
	}
	err := model.UpdateEventPriority(event.Id, alertUpgrade.Level)
	if err != nil {
		logger.Errorf("UpdateEventPriority failed, err: %v, event: %+v", err, event)
		return err
	}
	event.Priority = alertUpgrade.Level
	return nil
}

func fillRecvs(event *model.Event) error {
	userIds, err := getUserIds(event.Users, event.Groups)
	if err != nil {
		logger.Errorf("notify failed, get users id failed, event: %+v, err: %v", event, err)
		return err
	}

	if event.RealUpgrade {
		// 这里即使报错也不返回，只是打印个日志，无非就是拿不到升级配置里边的users信息，没所谓
		var alertUpgrade model.EventAlertUpgrade
		if err := json.Unmarshal([]byte(event.AlertUpgrade), &alertUpgrade); err != nil {
			logger.Errorf("unmarshal EventAlertUpgrade fail: %v", err)
		}

		upgradeUserIds, err := getUserIds(alertUpgrade.Users, alertUpgrade.Groups)
		if err != nil {
			logger.Errorf("upgrade notify failed, get upgrade users id failed, event: %+v, err: %v", event, err)
		}

		if upgradeUserIds != nil {
			userIds = append(userIds, upgradeUserIds...)
		}
	}

	event.RecvUserIDs = userIds
	userObjs, err := model.UserGetByIds(userIds)
	if err != nil {
		logger.Errorf("notify failed, get user by id failed, event: %+v, err: %v", event, err)
		return err
	}

	event.RecvUserObjs = userObjs
	return nil
}

func getUserIds(users, groups string) ([]int64, error) {
	var userIds []int64

	if err := json.Unmarshal([]byte(users), &userIds); err != nil {
		logger.Errorf("unmarshal users failed, users: %s, err: %v", users, err)
		return nil, nil
	}

	var groupIds []int64
	if err := json.Unmarshal([]byte(groups), &groupIds); err != nil {
		logger.Errorf("unmarshal groups failed, groups: %s, err: %v", groups, err)
		return nil, nil
	}

	teamUsers, err := model.UserIdGetByTeamIds(groupIds)
	if err != nil {
		logger.Errorf("get user id by team id failed, err: %v", err)
		return nil, err
	}

	userIds = append(userIds, teamUsers...)

	return userIds, nil
}

// isInConverge 包含2种情况
// 1. 用户配置了N秒之内只报警M次
// 2. 用户配置了不发送recovery通知
func isInConverge(event *model.Event) bool {
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

	cnt, err := model.EventCnt(event.HashId, model.ParseEtime(startTs), model.ParseEtime(now), event.RealUpgrade)
	if err != nil {
		logger.Errorf("get event count failed, err: %v", err)
		return false
	}

	if cnt >= convergeMaxCounts {
		logger.Infof("converge max counts: %d reached, current: %v, event hashid: %v", convergeMaxCounts, cnt, event.HashId)
		return true
	}

	return false
}

// 两种情况，不需要升级报警
// 1，认领的报警不需要升级
// 2，屏蔽的报警不需要升级，屏蔽判断在前面已经有了处理，这个方法不用关注
func needUpgrade(event *model.Event) bool {
	alertUpgradeKey := PrefixAlertUpgrade + fmt.Sprint(event.HashId)
	eventAlertKey := PrefixAlertTime + fmt.Sprint(event.HashId)

	// 如果告警恢复了
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
			return true
		}

		// 之前没有升级过，老板压根不知道这个事，现在恢复了，自然也不需要知道
		return false
	}

	// 这是一个alert，not recovery，但是告警事件都找不到了，还升级通知个毛线
	eventCur, err := model.EventCurGet("hashid", event.HashId)
	if err != nil {
		logger.Errorf("AlertUpgrade failed:get event_cur failed, event: %+v, err: %v", event, err)
		return false
	}

	// 告警事件都找不到了，还升级通知个毛线
	if eventCur == nil {
		logger.Infof("AlertUpgrade failed:get event_cur is nil, event hashid: %v", event.HashId)
		return false
	}

	now := time.Now().Unix()

	// 升级配置解析失败...自然没法升级了
	var alertUpgrade model.EventAlertUpgrade
	if err = json.Unmarshal([]byte(event.AlertUpgrade), &alertUpgrade); err != nil {
		logger.Errorf("AlertUpgrade unmarshal failed, event: %+v, err: %v", event, err)
		return false
	}

	upgradeDuration := int64(alertUpgrade.Duration)

	// 说明告警已经被认领
	claimants := strings.TrimSpace(eventCur.Claimants)
	if claimants != "[]" && claimants != "" {
		return false
	}

	// 告警之后，比如30分钟没有处理，就需要升级，那首先得知道首次告警时间
	if !redisc.HasKey(eventAlertKey) {
		err := redisc.SetWithTTL(eventAlertKey, now, 30*24*3600)
		if err != nil {
			logger.Errorf("set eventAlertKey failed, eventAlertKey: %v, err: %v", eventAlertKey, err)
		}

		// 之前没有eventAlertKey，说明是第一次报警，不需要升级
		return false
	}

	// 比如：没到30分钟呢，不用升级
	firstAlertTime := redisc.GET(eventAlertKey)
	if now-firstAlertTime < upgradeDuration {
		return false
	}

	err = redisc.SetWithTTL(alertUpgradeKey, 1, 30*24*3600)
	if err != nil {
		logger.Errorf("set alertUpgradeKey failed, alertUpgradeKey: %v, err: %v", alertUpgradeKey, err)
		return false
	}

	return true
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
