package router

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/dispatch"
	"github.com/ccfos/nightingale/v6/alert/mute"
	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

func (rt *Router) pushEventToQueue(c *gin.Context) {
	var event *models.AlertCurEvent
	ginx.BindJSON(c, &event)
	if event.RuleId == 0 {
		ginx.Bomb(200, "event is illegal")
	}
	event.FE2DB()

	event.TagsMap = make(map[string]string)
	for i := 0; i < len(event.TagsJSON); i++ {
		pair := strings.TrimSpace(event.TagsJSON[i])
		if pair == "" {
			continue
		}

		arr := strings.SplitN(pair, "=", 2)
		if len(arr) != 2 {
			continue
		}

		event.TagsMap[arr[0]] = arr[1]
	}
	// 触发事件按触发时刻判定屏蔽；恢复事件按恢复时刻（当前时间）重判，
	// 避免 TriggerTime 早于「只屏蔽通知」窗口而漏判、导致窗口内的恢复通知被误发。
	origTriggerTime := event.TriggerTime
	if event.IsRecovered {
		event.TriggerTime = time.Now().Unix()
	}
	hit, muteId, muteType := mute.EventMuteStrategy(event, rt.AlertMuteCache)
	event.TriggerTime = origTriggerTime

	if hit && muteType == models.MuteTypeNotifyOnly {
		// 只屏蔽通知：事件照常入队产生/记录，仅打标，通知阶段据此跳过发送并写通知记录（含恢复事件）
		event.NotifyMuted = 1
		event.MuteId = muteId
	} else if hit && !event.IsRecovered {
		// 完全屏蔽：仅丢弃触发事件；恢复事件照常放行以闭环
		logger.Infof("event_muted: rule_id=%d %s", event.RuleId, event.Hash)
		ginx.NewRender(c).Message(nil)
		return
	}

	if err := event.ParseRule("rule_name"); err != nil {
		event.RuleName = fmt.Sprintf("failed to parse rule name: %v", err)
	}

	if err := event.ParseRule("rule_note"); err != nil {
		event.RuleNote = fmt.Sprintf("failed to parse rule note: %v", err)
	}

	if err := event.ParseRule("annotations"); err != nil {
		event.RuleNote = fmt.Sprintf("failed to parse rule note: %v", err)
	}

	// 如果 rule_note 中有 ; 前缀，则使用 rule_note 替换 tags 中的内容
	if strings.HasPrefix(event.RuleNote, ";") {
		event.RuleNote = strings.TrimPrefix(event.RuleNote, ";")
		event.Tags = strings.ReplaceAll(event.RuleNote, " ", ",,")
		event.TagsJSON = strings.Split(event.Tags, ",,")
	} else {
		event.Tags = strings.Join(event.TagsJSON, ",,")
	}

	event.Callbacks = strings.Join(event.CallbacksJSON, " ")
	event.NotifyChannels = strings.Join(event.NotifyChannelsJSON, " ")
	event.NotifyGroups = strings.Join(event.NotifyGroupsJSON, " ")

	dispatch.LogEvent(event, "http_push_queue")
	if !queue.EventQueue.PushFront(event) {
		msg := fmt.Sprintf("event:%s push_queue err: queue is full", event.Hash)
		ginx.Bomb(200, "%s", msg)
		logger.Warning(msg)
	}
	ginx.NewRender(c).Message(nil)
}

func (rt *Router) eventPersist(c *gin.Context) {
	var event *models.AlertCurEvent
	ginx.BindJSON(c, &event)
	event.FE2DB()
	err := models.EventPersist(rt.Ctx, event)
	ginx.NewRender(c).Data(event.Id, err)
}

type eventForm struct {
	Alert         bool                  `json:"alert"`
	AnomalyPoints []models.AnomalyPoint `json:"vectors"`
	RuleId        int64                 `json:"rule_id"`
	DatasourceId  int64                 `json:"datasource_id"`
	Inhibit       bool                  `json:"inhibit"`
}

func (rt *Router) makeEvent(c *gin.Context) {
	var events []*eventForm
	ginx.BindJSON(c, &events)
	//now := time.Now().Unix()
	for i := 0; i < len(events); i++ {
		node, err := naming.DatasourceHashRing.GetNode(strconv.FormatInt(events[i].DatasourceId, 10), fmt.Sprintf("%d", events[i].RuleId))
		if err != nil {
			logger.Warningf("event(rule_id=%d ds_id=%d) get node err:%v", events[i].RuleId, events[i].DatasourceId, err)
			ginx.Bomb(200, "event node not exists")
		}

		if node != rt.Alert.Heartbeat.Endpoint {
			err := forwardEvent(events[i], node)
			if err != nil {
				logger.Warningf("event(rule_id=%d ds_id=%d) forward err:%v", events[i].RuleId, events[i].DatasourceId, err)
				ginx.Bomb(200, "event forward error")
			}
			continue
		}

		ruleWorker, exists := rt.ExternalProcessors.GetExternalAlertRule(events[i].DatasourceId, events[i].RuleId)
		logger.Debugf("handle event(rule_id=%d ds_id=%d) exists:%v", events[i].RuleId, events[i].DatasourceId, exists)
		if !exists {
			ginx.Bomb(200, "rule not exists")
		}

		if events[i].Alert {
			go ruleWorker.Handle(events[i].AnomalyPoints, "http", events[i].Inhibit)
		} else {
			for _, vector := range events[i].AnomalyPoints {
				readableString := vector.ReadableValue()
				go ruleWorker.RecoverSingle(false, process.Hash(events[i].RuleId, events[i].DatasourceId, vector), vector.Timestamp, &readableString)
			}
		}
	}
	ginx.NewRender(c).Message(nil)
}

// event 不归本实例处理，转发给对应的实例
func forwardEvent(event *eventForm, instance string) error {
	ur := fmt.Sprintf("http://%s/v1/n9e/make-event", instance)
	res, code, err := poster.PostJSON(ur, time.Second*5, []*eventForm{event}, 3)
	if err != nil {
		return err
	}
	logger.Infof("forward event: result=succ url=%s code=%d rule_id=%d response=%s", ur, code, event.RuleId, string(res))
	return nil
}
