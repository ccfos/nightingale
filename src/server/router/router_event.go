package router

import (
	"fmt"
	"strings"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/engine"
	promstat "github.com/didi/nightingale/v5/src/server/stat"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
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

	if err := event.ParseRuleNote(); err != nil {
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

	promstat.CounterAlertsTotal.WithLabelValues(config.C.ClusterName).Inc()
	engine.LogEvent(event, "http_push_queue")
	if !engine.EventQueue.PushFront(event) {
		msg := fmt.Sprintf("event:%+v push_queue err: queue is full", event)
		ginx.Bomb(200, msg)
		logger.Warningf(msg)
	}
	ginx.NewRender(c).Message(nil)
}
