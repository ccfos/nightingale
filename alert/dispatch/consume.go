package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/ccfos/nightingale/v6/prom"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"
)

type Consumer struct {
	alerting aconf.Alerting
	ctx      *ctx.Context

	dispatch       *Dispatch
	promClients    *prom.PromClientMap
	alertMuteCache *memsto.AlertMuteCacheType
}

type EventMuteHookFunc func(event *models.AlertCurEvent) bool

var EventMuteHook EventMuteHookFunc = func(event *models.AlertCurEvent) bool { return false }

func InitRegisterQueryFunc(promClients *prom.PromClientMap) {
	tplx.RegisterQueryFunc(func(datasourceID int64, promql string) model.Value {
		if promClients.IsNil(datasourceID) {
			return nil
		}

		readerClient := promClients.GetCli(datasourceID)
		value, _, _ := readerClient.Query(context.Background(), promql, time.Now())
		return value
	})
}

// 创建一个 Consumer 实例
func NewConsumer(alerting aconf.Alerting, ctx *ctx.Context, dispatch *Dispatch, promClients *prom.PromClientMap, alertMuteCache *memsto.AlertMuteCacheType) *Consumer {
	return &Consumer{
		alerting:    alerting,
		ctx:         ctx,
		dispatch:    dispatch,
		promClients: promClients,

		alertMuteCache: alertMuteCache,
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

	e.dispatch.Astats.CounterAlertsTotal.WithLabelValues(event.Cluster, eventType, event.GroupName).Inc()

	if err := event.ParseRule("rule_name"); err != nil {
		logger.Warningf("alert_eval_%d datasource_%d failed to parse rule name: %v", event.RuleId, event.DatasourceId, err)
		event.RuleName = fmt.Sprintf("failed to parse rule name: %v", err)
	}

	if err := event.ParseRule("annotations"); err != nil {
		logger.Warningf("alert_eval_%d datasource_%d failed to parse annotations: %v", event.RuleId, event.DatasourceId, err)
		event.Annotations = fmt.Sprintf("failed to parse annotations: %v", err)
		event.AnnotationsJSON["error"] = event.Annotations
	}

	e.queryRecoveryVal(event)

	if err := event.ParseRule("rule_note"); err != nil {
		logger.Warningf("alert_eval_%d datasource_%d failed to parse rule note: %v", event.RuleId, event.DatasourceId, err)
		event.RuleNote = fmt.Sprintf("failed to parse rule note: %v", err)
	}

	e.persist(event)

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
			logger.Errorf("event:%s persist err:%v", event.Hash, err)
			e.dispatch.Astats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", event.DatasourceId), "persist_event", event.GroupName, fmt.Sprintf("%v", event.RuleId)).Inc()
		}
		return
	}

	err := models.EventPersist(e.ctx, event)
	if err != nil {
		logger.Errorf("event:%s persist err:%v", event.Hash, err)
		e.dispatch.Astats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", event.DatasourceId), "persist_event", event.GroupName, fmt.Sprintf("%v", event.RuleId)).Inc()
	}
}

func (e *Consumer) queryRecoveryVal(event *models.AlertCurEvent) {
	if !event.IsRecovered {
		return
	}

	// If the event is a recovery event, execute the recovery_promql query
	promql, ok := event.AnnotationsJSON["recovery_promql"]
	if !ok {
		return
	}

	promql = strings.TrimSpace(promql)
	if promql == "" {
		logger.Warningf("alert_eval_%d datasource_%d promql is blank", event.RuleId, event.DatasourceId)
		return
	}

	if e.promClients.IsNil(event.DatasourceId) {
		logger.Warningf("alert_eval_%d datasource_%d error reader client is nil", event.RuleId, event.DatasourceId)
		return
	}

	readerClient := e.promClients.GetCli(event.DatasourceId)

	var warnings promsdk.Warnings
	value, warnings, err := readerClient.Query(e.ctx.Ctx, promql, time.Now())
	if err != nil {
		logger.Errorf("alert_eval_%d datasource_%d promql:%s, error:%v", event.RuleId, event.DatasourceId, promql, err)
		event.AnnotationsJSON["recovery_promql_error"] = fmt.Sprintf("promql:%s error:%v", promql, err)

		b, err := json.Marshal(event.AnnotationsJSON)
		if err != nil {
			event.AnnotationsJSON = make(map[string]string)
			event.AnnotationsJSON["error"] = fmt.Sprintf("failed to parse annotations: %v", err)
		} else {
			event.Annotations = string(b)
		}
		return
	}

	if len(warnings) > 0 {
		logger.Errorf("alert_eval_%d datasource_%d promql:%s, warnings:%v", event.RuleId, event.DatasourceId, promql, warnings)
	}

	anomalyPoints := models.ConvertAnomalyPoints(value)
	if len(anomalyPoints) == 0 {
		logger.Warningf("alert_eval_%d datasource_%d promql:%s, result is empty", event.RuleId, event.DatasourceId, promql)
		event.AnnotationsJSON["recovery_promql_error"] = fmt.Sprintf("promql:%s error:%s", promql, "result is empty")
	} else {
		event.AnnotationsJSON["recovery_value"] = fmt.Sprintf("%v", anomalyPoints[0].Value)
	}

	b, err := json.Marshal(event.AnnotationsJSON)
	if err != nil {
		event.AnnotationsJSON = make(map[string]string)
		event.AnnotationsJSON["error"] = fmt.Sprintf("failed to parse annotations: %v", err)
	} else {
		event.Annotations = string(b)
	}
}

