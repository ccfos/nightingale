package dispatch

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

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
	has, err := models.AlertCurEventExists(e.ctx, "hash=?", event.Hash)
	if err != nil {
		logger.Errorf("event_persist_check_exists_fail: %v rule_id=%d hash=%s", err, event.RuleId, event.Hash)
		return
	}

	his := event.ToHis(e.ctx)

	// 不管是告警还是恢复，全量告警里都要记录
	if err := his.Add(e.ctx); err != nil {
		logger.Errorf(
			"event_persist_his_fail: %v rule_id=%d cluster:%s hash=%s tags=%v timestamp=%d value=%s",
			err,
			event.RuleId,
			event.Cluster,
			event.Hash,
			event.TagsJSON,
			event.TriggerTime,
			event.TriggerValue,
		)
	}

	if has {
		// 活跃告警表中有记录，删之
		err = models.AlertCurEventDelByHash(e.ctx, event.Hash)
		if err != nil {
			logger.Errorf("event_del_cur_fail: %v hash=%s", err, event.Hash)
			return
		}

		if !event.IsRecovered {
			// 恢复事件，从活跃告警列表彻底删掉，告警事件，要重新加进来新的event
			// use his id as cur id
			event.Id = his.Id
			if event.Id > 0 {
				if err := event.Add(e.ctx); err != nil {
					logger.Errorf(
						"event_persist_cur_fail: %v rule_id=%d cluster:%s hash=%s tags=%v timestamp=%d value=%s",
						err,
						event.RuleId,
						event.Cluster,
						event.Hash,
						event.TagsJSON,
						event.TriggerTime,
						event.TriggerValue,
					)
				}
			}
		}

		return
	}

	if event.IsRecovered {
		// alert_cur_event表里没有数据，表示之前没告警，结果现在报了恢复，神奇....理论上不应该出现的
		return
	}

	// use his id as cur id
	event.Id = his.Id
	if event.Id > 0 {
		if err := event.Add(e.ctx); err != nil {
			logger.Errorf(
				"event_persist_cur_fail: %v rule_id=%d cluster:%s hash=%s tags=%v timestamp=%d value=%s",
				err,
				event.RuleId,
				event.Cluster,
				event.Hash,
				event.TagsJSON,
				event.TriggerTime,
				event.TriggerValue,
			)
		}
	}
}
