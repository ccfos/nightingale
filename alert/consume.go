package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/judge"
	"github.com/didi/nightingale/v5/models"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/sys"
)

func popEvent() {
	sema := semaphore.NewSemaphore(config.Config.Alert.NotifyScriptConcurrency)
	duration := time.Duration(100) * time.Millisecond
	for {
		events := judge.EventQueue.PopBackBy(200)
		if len(events) < 1 {
			time.Sleep(duration)
			continue
		}
		consume(events, sema)
	}
}

func consume(events []interface{}, sema *semaphore.Semaphore) {
	for i := range events {
		if events[i] == nil {
			continue
		}

		event := events[i].(*models.AlertEvent)

		alertRule, exists := cache.AlertRules.Get(event.RuleId)
		if !exists {
			logger.Errorf("event_consume: alert rule not found, event:%+v", event)
			continue
		}
		logger.Debugf("[event_consume_success][type:%v][event:%+v]", event.IsPromePull, event)
		if isNoneffective(event, alertRule) {
			// 告警规则非生效时段
			continue
		}

		event.RuleName = alertRule.Name
		event.RuleNote = alertRule.Note
		event.NotifyChannels = alertRule.NotifyChannels
		classpaths := cache.ResClasspath.GetValues(event.ResIdent)
		event.ResClasspaths = strings.Join(classpaths, " ")
		enrichTag(event, alertRule)

		if isEventMute(event) && event.IsAlert() {
			// 被屏蔽的事件
			event.MarkMuted()

			if config.Config.Alert.MutedAlertPersist {
				err := event.Add()
				if err != nil {
					logger.Warningf("event_consume: insert muted event err:%v, event:%+v", err, event)
				}
			}

			continue
		}

		// 操作数据库
		persist(event)

		// 不管是告警还是恢复，都触发回调，接收端自己处理
		if alertRule.Callbacks != "" {
			go callback(event, alertRule)
		}

		uids := genNotifyUserIDs(alertRule)
		if len(uids) == 0 {
			logger.Warningf("event_consume: notify users not found, event:%+v", event)
			continue
		}

		users := cache.UserCache.GetByIds(uids)
		if len(users) == 0 {
			logger.Warningf("event_consume: notify users not found, event:%+v", event)
			continue
		}

		alertMsg := AlertMsg{
			Event: event,
			Rule:  alertRule,
			Users: users,
		}

		logger.Infof("event_consume: notify alert:%+v", alertMsg)

		sema.Acquire()
		go func(alertMsg AlertMsg) {
			defer sema.Release()
			notify(alertMsg)
		}(alertMsg)
	}
}

func genNotifyUserIDs(alertRule *models.AlertRule) []int64 {
	uidMap := make(map[int64]struct{})

	groupIds := strings.Fields(alertRule.NotifyGroups)
	for _, groupId := range groupIds {
		gid, err := strconv.ParseInt(groupId, 10, 64)
		if err != nil {
			logger.Warningf("event_consume: strconv groupid(%s) fail: %v", groupId, err)
			continue
		}

		um, exists := cache.UserGroupMember.Get(gid)
		if !exists {
			continue
		}

		for uid := range um {
			uidMap[uid] = struct{}{}
		}
	}

	userIds := strings.Fields(alertRule.NotifyUsers)
	for _, userId := range userIds {
		uid, err := strconv.ParseInt(userId, 10, 64)
		if err != nil {
			logger.Warningf("event_consume: strconv userid(%s) fail: %v", userId, err)
			continue
		}

		uidMap[uid] = struct{}{}
	}

	uids := make([]int64, 0, len(uidMap))
	for uid := range uidMap {
		uids = append(uids, uid)
	}

	return uids
}

// 如果是告警，就存库，如果是恢复，就从未恢复的告警表里删除
func persist(event *models.AlertEvent) {
	if event.IsRecov() {
		err := event.DelByHashId()
		if err != nil {
			logger.Warningf("event_consume: delete recovery event err:%v, event:%+v", err, event)
		}
	} else {
		err := event.Add()
		if err != nil {
			logger.Warningf("event_consume: insert alert event err:%v, event:%+v", err, event)
		}
	}
}

type AlertMsg struct {
	Event *models.AlertEvent `json:"event"`
	Rule  *models.AlertRule  `json:"rule"`
	Users []*models.User     `json:"users"`
}

func notify(alertMsg AlertMsg) {
	//增加并发控制
	bs, err := json.Marshal(alertMsg)
	if err != nil {
		logger.Errorf("notify: marshal alert %+v err:%v", alertMsg, err)
	}

	fpath := config.Config.Alert.NotifyScriptPath
	cmd := exec.Command(fpath)
	cmd.Stdin = bytes.NewReader(bs)

	// combine stdout and stderr
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err = cmd.Start()
	if err != nil {
		logger.Errorf("notify: run cmd err:%v", err)
		return
	}

	err, isTimeout := sys.WrapTimeout(cmd, time.Duration(10)*time.Second)

	if isTimeout {
		if err == nil {
			logger.Errorf("notify: timeout and killed process %s", fpath)
		}

		if err != nil {
			logger.Errorf("notify: kill process %s occur error %v", fpath, err)
		}

		return
	}

	if err != nil {
		logger.Errorf("notify: exec script %s occur error: %v", fpath, err)
		return
	}

	logger.Infof("notify: exec %s output: %s", fpath, buf.String())
}

func callback(event *models.AlertEvent, alertRule *models.AlertRule) {
	urls := strings.Fields(alertRule.Callbacks)
	for _, url := range urls {
		if url == "" {
			continue
		}

		if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
			url = "http://" + url
		}

		resp, code, err := httplib.PostJSON(url, 5*time.Second, event, map[string]string{})
		if err != nil {
			logger.Errorf("callback[%s] fail, callback content: %+v, resp: %s, err: %v, code:%d", url, event, string(resp), err, code)
		} else {
			logger.Infof("callback[%s] succ, callback content: %+v, resp: %s, code:%d", url, event, string(resp), code)
		}
	}
}

func isNoneffective(event *models.AlertEvent, alertRule *models.AlertRule) bool {
	// 生效时间过滤
	if alertRule.Status == models.ALERT_RULE_DISABLED {
		logger.Debugf("event:%+v alert rule:%+v disable", event, alertRule)
		return true
	}

	tm := time.Unix(event.TriggerTime, 0)
	triggerTime := tm.Format("15:04")
	triggerWeek := strconv.Itoa(int(tm.Weekday()))

	if alertRule.EnableStime <= alertRule.EnableEtime {
		if triggerTime < alertRule.EnableStime || triggerTime > alertRule.EnableEtime {
			logger.Debugf("event:%+v alert rule:%+v triggerTime Noneffective", event, alertRule)
			return true
		}
	} else {
		if triggerTime < alertRule.EnableStime && triggerTime > alertRule.EnableEtime {
			logger.Debugf("event:%+v alert rule:%+v triggerTime Noneffective", event, alertRule)
			return true
		}
	}

	if !strings.Contains(alertRule.EnableDaysOfWeek, triggerWeek) {
		logger.Debugf("event:%+v alert rule:%+v triggerWeek Noneffective", event, alertRule)
		return true
	}

	return false
}

// 事件的tags有多种tags组成：ident作为一个tag，数据本身的tags(前期已经把res的tags也附到数据tags里了)、规则的tags
func enrichTag(event *models.AlertEvent, alertRule *models.AlertRule) {
	if event.ResIdent != "" {
		event.TagMap["ident"] = event.ResIdent
	}

	if alertRule.AppendTags != "" {
		appendTags := strings.Fields(alertRule.AppendTags)
		for _, tag := range appendTags {
			arr := strings.Split(tag, "=")
			if len(arr) != 2 {
				logger.Warningf("alertRule AppendTags:%+v illagel", alertRule.AppendTags)
				continue
			}
			event.TagMap[arr[0]] = arr[1]
		}
	}

	var tagList []string
	for key, value := range event.TagMap {
		tagList = append(tagList, fmt.Sprintf("%s=%s", key, value))
	}
	sort.Strings(tagList)
	event.Tags = strings.Join(tagList, " ")
}
