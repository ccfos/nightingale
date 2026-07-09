package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/alert/sender"
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

// NotifyMutedEventHook 在事件命中「只屏蔽通知」、跳过正常通知流程时调用，
// 让下游（如 n9e-plus 升级）在不发送通知的前提下完成必要的状态清理
// （例如恢复事件清理升级用的 Redis 记录，避免屏蔽结束后继续升级已恢复的告警）。
var NotifyMutedEventHook = func(event *models.AlertCurEvent) {}

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

	if event.NotifyMuted == 1 {
		// 命中「只屏蔽通知」规则：事件已产生并记录，此处跳过全部通知渠道，
		// 并写一条通知记录说明被哪条屏蔽规则拦截，供事件详情「通知记录」排查（含恢复事件）。
		LogEvent(event, "notify_muted")
		e.recordNotifyMuted(event)
		// 跳过发送，但仍让下游完成必要的状态清理（如恢复事件清理升级用的 Redis 记录）
		NotifyMutedEventHook(event)
		return
	}

	e.dispatch.HandleEventNotify(event, false)
}

// recordNotifyMuted 为「只屏蔽通知」而未发送的事件（含恢复事件）写一条通知记录，
// 说明命中的屏蔽规则，替代事件表上的持久化标记。
func (e *Consumer) recordNotifyMuted(event *models.AlertCurEvent) {
	if event.Id == 0 {
		return
	}

	muteName := e.muteRuleName(event.GroupId, event.MuteId)
	detail := fmt.Sprintf("命中「只屏蔽通知」屏蔽规则（%s），事件已产生并记录，通知被抑制", muteName)
	if event.IsRecovered {
		detail = fmt.Sprintf("命中「只屏蔽通知」屏蔽规则（%s），恢复事件已记录，恢复通知被抑制", muteName)
	}

	record := &models.NotificationRecord{
		EventId:   event.Id,
		SubId:     event.SubRuleId,
		Channel:   "屏蔽规则",
		Status:    models.NotiStatusMuted,
		Target:    muteName,
		Details:   detail,
		CreatedAt: time.Now().Unix(),
	}
	sender.RecordNotifications(e.ctx, []*models.NotificationRecord{record})
}

// muteRuleName 尽力从缓存解析屏蔽规则名（note），解析不到则回退为 id。
func (e *Consumer) muteRuleName(groupId, muteId int64) string {
	if muteId == 0 {
		return "-"
	}

	if mutes, has := e.alertMuteCache.Gets(groupId); has {
		for _, m := range mutes {
			if m.Id == muteId {
				if m.Note != "" {
					return fmt.Sprintf("%s(id=%d)", m.Note, muteId)
				}
				break
			}
		}
	}

	return fmt.Sprintf("id=%d", muteId)
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
