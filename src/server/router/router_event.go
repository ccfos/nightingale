package router

import (
	"fmt"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/engine"
	promstat "github.com/didi/nightingale/v5/src/server/stat"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

func pushEventToQueue(c *gin.Context) {
	var event models.AlertCurEvent
	ginx.BindJSON(c, &event)
	if event.RuleId == 0 {
		ginx.Bomb(200, "event is illegal")
	}

	promstat.CounterAlertsTotal.WithLabelValues(config.C.ClusterName).Inc()
	engine.LogEvent(&event, "http_push_queue")
	if !engine.EventQueue.PushFront(event) {
		msg := fmt.Sprintf("event:%+v push_queue err: queue is full", event)
		ginx.Bomb(200, msg)
		logger.Warningf(msg)
	}
	ginx.NewRender(c).Message(nil)
}
