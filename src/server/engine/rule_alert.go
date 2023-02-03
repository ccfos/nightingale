package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/prom"
	"github.com/didi/nightingale/v5/src/server/common/conv"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

type AlertRuleContext struct {
	cluster string
	quit    chan struct{}

	rule     *models.AlertRule
	fires    *AlertCurEventMap
	pendings *AlertCurEventMap
}

func NewAlertRuleContext(rule *models.AlertRule, cluster string) *AlertRuleContext {
	return &AlertRuleContext{
		cluster: cluster,
		quit:    make(chan struct{}),
		rule:    rule,
	}
}

func (arc *AlertRuleContext) RuleFromCache() *models.AlertRule {
	return memsto.AlertRuleCache.Get(arc.rule.Id)
}

func (arc *AlertRuleContext) Key() string {
	return ruleKey(arc.cluster, arc.rule.Id)
}

func (arc *AlertRuleContext) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%d_%s_%s",
		arc.rule.Id,
		arc.rule.PromEvalInterval,
		arc.rule.PromQl,
		arc.cluster,
	))
}

func (arc *AlertRuleContext) Prepare() {
	arc.recoverAlertCurEventFromDb()
}

func (arc *AlertRuleContext) Start() {
	logger.Infof("eval:%s started", arc.Key())
	interval := arc.rule.PromEvalInterval
	if interval <= 0 {
		interval = 10
	}
	go func() {
		for {
			select {
			case <-arc.quit:
				return
			default:
				arc.Eval()
				time.Sleep(time.Duration(interval) * time.Second)
			}
		}
	}()
}

func (arc *AlertRuleContext) Eval() {
	promql := strings.TrimSpace(arc.rule.PromQl)
	if promql == "" {
		logger.Errorf("rule_eval:%s promql is blank", arc.Key())
		return
	}

	if config.ReaderClients.IsNil(arc.cluster) {
		logger.Errorf("rule_eval:%s error reader client is nil", arc.Key())
		return
	}

	readerClient := config.ReaderClients.GetCli(arc.cluster)

	var value model.Value
	var err error

	cachedRule := arc.RuleFromCache()
	if cachedRule == nil {
		logger.Errorf("rule_eval:%s rule not found", arc.Key())
		return
	}

	// 如果是单个goroutine执行, 完全可以考虑把cachedRule赋值给arc.rule, 不会有问题
	// 但是在externalRule的场景中, 会调用HandleVectors/RecoverSingle;就行不通了,还是在需要的时候从cache中拿rule吧
	// arc.rule = cachedRule

	// 如果cache中的规则由prometheus规则改为其他类型，也没必要再去prometheus查询了
	if cachedRule.IsPrometheusRule() {
		var warnings prom.Warnings
		value, warnings, err = readerClient.Query(context.Background(), promql, time.Now())
		if err != nil {
			logger.Errorf("rule_eval:%s promql:%s, error:%v", arc.Key(), promql, err)
			//notifyToMaintainer(err, "failed to query prometheus")
			Report(QueryPrometheusError)
			return
		}

		if len(warnings) > 0 {
			logger.Errorf("rule_eval:%s promql:%s, warnings:%v", arc.Key(), promql, warnings)
			return
		}
		logger.Debugf("rule_eval:%s promql:%s, value:%v", arc.Key(), promql, value)
	}
	arc.HandleVectors(conv.ConvertVectors(value), "inner")
}

func (arc *AlertRuleContext) HandleVectors(vectors []conv.Vector, from string) {
	// 有可能rule的一些配置已经发生变化，比如告警接收人、callbacks等
	// 这些信息的修改是不会引起worker restart的，但是确实会影响告警处理逻辑
	// 所以，这里直接从memsto.AlertRuleCache中获取并覆盖
	cachedRule := arc.RuleFromCache()
	if cachedRule == nil {
		logger.Errorf("rule_eval:%s rule not found", arc.Key())
		return
	}
	now := time.Now().Unix()
	alertingKeys := map[string]struct{}{}
	for _, vector := range vectors {
		alertVector := NewAlertVector(arc, cachedRule, vector, from)
		event := alertVector.BuildEvent(now)
		// 如果event被mute了,本质也是fire的状态,这里无论如何都添加到alertingKeys中,防止fire的事件自动恢复了
		alertingKeys[alertVector.Hash()] = struct{}{}
		if IsMuted(cachedRule, event) {
			continue
		}
		arc.handleEvent(event)
	}

	arc.HandleRecover(alertingKeys, now)
}

func (arc *AlertRuleContext) HandleRecover(alertingKeys map[string]struct{}, now int64) {
	for _, hash := range arc.pendings.Keys() {
		if _, has := alertingKeys[hash]; has {
			continue
		}
		arc.pendings.Delete(hash)
	}

	for hash := range arc.fires.GetAll() {
		if _, has := alertingKeys[hash]; has {
			continue
		}
		arc.RecoverSingle(hash, now, nil)
	}
}

func (arc *AlertRuleContext) RecoverSingle(hash string, now int64, value *string) {
	cachedRule := arc.RuleFromCache()
	if cachedRule == nil {
		return
	}
	event, has := arc.fires.Get(hash)
	if !has {
		return
	}
	// 如果配置了留观时长，就不能立马恢复了
	if cachedRule.RecoverDuration > 0 && now-event.LastEvalTime < cachedRule.RecoverDuration {
		return
	}
	if value != nil {
		event.TriggerValue = *value
	}

	// 没查到触发阈值的vector，姑且就认为这个vector的值恢复了
	// 我确实无法分辨，是prom中有值但是未满足阈值所以没返回，还是prom中确实丢了一些点导致没有数据可以返回，尴尬
	arc.fires.Delete(hash)
	arc.pendings.Delete(hash)

	// 可能是因为调整了promql才恢复的，所以事件里边要体现最新的promql，否则用户会比较困惑
	// 当然，其实rule的各个字段都可能发生变化了，都更新一下吧
	cachedRule.UpdateEvent(event)
	event.IsRecovered = true
	event.LastEvalTime = now
	arc.pushEventToQueue(event)
}

func (arc *AlertRuleContext) handleEvent(event *models.AlertCurEvent) {
	if event == nil {
		return
	}
	if event.PromForDuration == 0 {
		arc.fireEvent(event)
		return
	}

	var preTriggerTime int64
	preEvent, has := arc.pendings.Get(event.Hash)
	if has {
		arc.pendings.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
		preTriggerTime = preEvent.TriggerTime
	} else {
		arc.pendings.Set(event.Hash, event)
		preTriggerTime = event.TriggerTime
	}

	if event.LastEvalTime-preTriggerTime+int64(event.PromEvalInterval) >= int64(event.PromForDuration) {
		arc.fireEvent(event)
	}
}

func (arc *AlertRuleContext) fireEvent(event *models.AlertCurEvent) {
	// As arc.rule maybe outdated, use rule from cache
	cachedRule := arc.RuleFromCache()
	if cachedRule == nil {
		return
	}
	if fired, has := arc.fires.Get(event.Hash); has {
		arc.fires.UpdateLastEvalTime(event.Hash, event.LastEvalTime)

		if cachedRule.NotifyRepeatStep == 0 {
			// 说明不想重复通知，那就直接返回了，nothing to do
			return
		}

		// 之前发送过告警了，这次是否要继续发送，要看是否过了通道静默时间
		if event.LastEvalTime > fired.LastSentTime+int64(cachedRule.NotifyRepeatStep)*60 {
			if cachedRule.NotifyMaxNumber == 0 {
				// 最大可以发送次数如果是0，表示不想限制最大发送次数，一直发即可
				event.NotifyCurNumber = fired.NotifyCurNumber + 1
				event.FirstTriggerTime = fired.FirstTriggerTime
				arc.pushEventToQueue(event)
			} else {
				// 有最大发送次数的限制，就要看已经发了几次了，是否达到了最大发送次数
				if fired.NotifyCurNumber >= cachedRule.NotifyMaxNumber {
					return
				} else {
					event.NotifyCurNumber = fired.NotifyCurNumber + 1
					event.FirstTriggerTime = fired.FirstTriggerTime
					arc.pushEventToQueue(event)
				}
			}
		}
	} else {
		event.NotifyCurNumber = 1
		event.FirstTriggerTime = event.TriggerTime
		arc.pushEventToQueue(event)
	}
}

func (arc *AlertRuleContext) pushEventToQueue(event *models.AlertCurEvent) {
	if !event.IsRecovered {
		event.LastSentTime = event.LastEvalTime
		arc.fires.Set(event.Hash, event)
	}

	promstat.CounterAlertsTotal.WithLabelValues(event.Cluster).Inc()
	LogEvent(event, "push_queue")
	if !EventQueue.PushFront(event) {
		logger.Warningf("event_push_queue: queue is full, event:%+v", event)
	}
}

func (arc *AlertRuleContext) Stop() {
	logger.Infof("%s stopped", arc.Key())
	close(arc.quit)
}

func (arc *AlertRuleContext) recoverAlertCurEventFromDb() {
	arc.pendings = NewAlertCurEventMap(nil)

	curEvents, err := models.AlertCurEventGetByRuleIdAndCluster(arc.rule.Id, arc.cluster)
	if err != nil {
		logger.Errorf("recover event from db for rule:%s failed, err:%s", arc.Key(), err)
		arc.fires = NewAlertCurEventMap(nil)
		return
	}

	fireMap := make(map[string]*models.AlertCurEvent)
	for _, event := range curEvents {
		event.DB2Mem()
		fireMap[event.Hash] = event
	}

	arc.fires = NewAlertCurEventMap(fireMap)
}
