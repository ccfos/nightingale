package router

import (
	"fmt"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/common/conv"
	"github.com/didi/nightingale/v5/src/server/engine"
	promstat "github.com/didi/nightingale/v5/src/server/stat"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

func pushEventToQueue(c *gin.Context) {
	var event *models.AlertCurEvent
	ginx.BindJSON(c, &event)
	if event.RuleId == 0 {
		ginx.Bomb(200, "event is illegal")
	}

	event.TagsMap = make(map[string]string)
	for i := 0; i < len(event.TagsJSON); i++ {
		pair := strings.TrimSpace(event.TagsJSON[i])
		if pair == "" {
			continue
		}

		arr := strings.Split(pair, "=")
		if len(arr) != 2 {
			continue
		}

		event.TagsMap[arr[0]] = arr[1]
	}

	// isMuted only need TriggerTime RuleName and TagsMap
	if engine.IsMuted(event) {
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

	promstat.CounterAlertsTotal.WithLabelValues(event.Cluster).Inc()

	engine.LogEvent(event, "http_push_queue")
	if !engine.EventQueue.PushFront(event) {
		msg := fmt.Sprintf("event:%+v push_queue err: queue is full", event)
		ginx.Bomb(200, msg)
		logger.Warningf(msg)
	}
	ginx.NewRender(c).Message(nil)
}

type eventForm struct {
	Alert   bool          `json:"alert"`
	Vectors []conv.Vector `json:"vectors"`
	RuleId  int64         `json:"rule_id"`
	Cluster string        `json:"cluster"`
}

func judgeEvent(c *gin.Context) {
	var form eventForm
	ginx.BindJSON(c, &form)
	re, exists := engine.RuleEvalForExternal.Get(form.RuleId, form.Cluster)
	if !exists {
		ginx.Bomb(200, "rule not exists")
	}
	re.Judge(form.Cluster, form.Vectors)
	ginx.NewRender(c).Message(nil)
}

func makeEvent(c *gin.Context) {
	var events []*eventForm
	ginx.BindJSON(c, &events)
	now := time.Now().Unix()
	for i := 0; i < len(events); i++ {
		re, exists := engine.RuleEvalForExternal.Get(events[i].RuleId, events[i].Cluster)
		logger.Debugf("handle event:%+v exists:%v", events[i], exists)
		if !exists {
			ginx.Bomb(200, "rule not exists")
		}

		if events[i].Alert {
			go re.MakeNewEvent("http", now, events[i].Cluster, events[i].Vectors)
		} else {
			for _, vector := range events[i].Vectors {
				hash := str.MD5(fmt.Sprintf("%d_%s_%s", events[i].RuleId, vector.Key, events[i].Cluster))
				now := vector.Timestamp
				go re.RecoverEvent(hash, now, vector.Value)
			}
		}
	}
	ginx.NewRender(c).Message(nil)
}
