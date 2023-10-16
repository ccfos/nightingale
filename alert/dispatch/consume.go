package dispatch

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"
)

type Consumer struct {
	alerting aconf.Alerting
	ctx      *ctx.Context

	dispatch *Dispatch
}

// 创建一个 Consumer 实例
func NewConsumer(alerting aconf.Alerting, ctx *ctx.Context, dispatch *Dispatch) *Consumer {
	return &Consumer{
		alerting: alerting,
		ctx:      ctx,
		dispatch: dispatch,
	}
}

func (e *Consumer) LoopConsume() {
	sema := semaphore.NewSemaphore(e.alerting.NotifyConcurrency)
	duration := time.Duration(100) * time.Millisecond
	for {
		events := queue.EventQueue.PopBackBy(100)
		if len(events) == 0 {
			time.Sleep(duration)
			continue
		}
		e.consume(events, sema)
	}
}

func (e *Consumer) consume(events []interface{}, sema *semaphore.Semaphore) {
	for i := range events {
		if events[i] == nil {
			continue
		}

		event := events[i].(*models.AlertCurEvent)
		sema.Acquire()
		go func(event *models.AlertCurEvent) {
			defer sema.Release()
			e.consumeOne(event)
		}(event)
	}
}

func (e *Consumer) consumeOne(event *models.AlertCurEvent) {
	LogEvent(event, "consume")

	eventType := "alert"
	if event.IsRecovered {
		eventType = "recovery"
	}

	e.dispatch.astats.CounterAlertsTotal.WithLabelValues(event.Cluster, eventType, event.GroupName).Inc()

	if err := event.ParseRule("rule_name"); err != nil {
		event.RuleName = fmt.Sprintf("failed to parse rule name: %v", err)
	}

	if err := event.ParseRule("rule_note"); err != nil {
		event.RuleNote = fmt.Sprintf("failed to parse rule note: %v", err)
	}

	if err := event.ParseRule("annotations"); err != nil {
		event.Annotations = fmt.Sprintf("failed to parse rule note: %v", err)
	}

	e.persist(event)

	if event.IsRecovered && event.NotifyRecovered == 0 {
		return
	}

	e.dispatch.HandleEventNotify(event, false)
}

func (e *Consumer) persist(event *models.AlertCurEvent) {
	if event.Status != 0 {
		return
	}

	if !e.ctx.IsCenter {
		event.DB2FE()
		var err error
		event.Id, err = poster.PostByUrlsWithResp[int64](e.ctx, "/v1/n9e/event-persist", event)
		if err != nil {
			logger.Errorf("event:%+v persist err:%v", event, err)
		}
		return
	}

	err := models.EventPersist(e.ctx, event)
	if err != nil {
		logger.Errorf("event%+v persist err:%v", event, err)
	}
}
