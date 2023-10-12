package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/hash"
	"github.com/ccfos/nightingale/v6/pkg/parser"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/tdengine"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type AlertRuleWorker struct {
	datasourceId int64
	quit         chan struct{}
	inhibit      bool
	severity     int

	rule *models.AlertRule

	processor *process.Processor

	promClients     *prom.PromClientMap
	tdengineClients *tdengine.TdengineClientMap
	ctx             *ctx.Context
}

func NewAlertRuleWorker(rule *models.AlertRule, datasourceId int64, processor *process.Processor, promClients *prom.PromClientMap, tdengineClients *tdengine.TdengineClientMap, ctx *ctx.Context) *AlertRuleWorker {
	arw := &AlertRuleWorker{
		datasourceId: datasourceId,
		quit:         make(chan struct{}),
		rule:         rule,
		processor:    processor,

		promClients:     promClients,
		tdengineClients: tdengineClients,
		ctx:             ctx,
	}

	return arw
}

func (arw *AlertRuleWorker) Key() string {
	return common.RuleKey(arw.datasourceId, arw.rule.Id)
}

func (arw *AlertRuleWorker) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%d_%s_%d",
		arw.rule.Id,
		arw.rule.PromEvalInterval,
		arw.rule.RuleConfig,
		arw.datasourceId,
	))
}

func (arw *AlertRuleWorker) Prepare() {
	arw.processor.RecoverAlertCurEventFromDb()
}

func (arw *AlertRuleWorker) Start() {
	logger.Infof("eval:%s started", arw.Key())
	interval := arw.rule.PromEvalInterval
	if interval <= 0 {
		interval = 10
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-arw.quit:
				return
			case <-ticker.C:
				arw.Eval()
			}
		}
	}()
}

func (arw *AlertRuleWorker) Eval() {
	cachedRule := arw.rule
	if cachedRule == nil {
		// logger.Errorf("rule_eval:%s rule not found", arw.Key())
		return
	}
	arw.processor.Stats.CounterRuleEval.WithLabelValues().Inc()

	typ := cachedRule.GetRuleType()
	var anomalyPoints []common.AnomalyPoint
	var recoverPoints []common.AnomalyPoint
	switch typ {
	case models.PROMETHEUS:
		anomalyPoints = arw.GetPromAnomalyPoint(cachedRule.RuleConfig)
	case models.HOST:
		anomalyPoints = arw.GetHostAnomalyPoint(cachedRule.RuleConfig)
	case models.TDENGINE:
		anomalyPoints, recoverPoints = arw.GetTdengineAnomalyPoint(cachedRule, arw.processor.DatasourceId())
	case models.LOKI:
		anomalyPoints = arw.GetPromAnomalyPoint(cachedRule.RuleConfig)
	default:
		return
	}

	if arw.processor == nil {
		logger.Warningf("rule_eval:%s processor is nil", arw.Key())
		return
	}

	arw.processor.Handle(anomalyPoints, "inner", arw.inhibit)
	for _, point := range recoverPoints {
		str := fmt.Sprintf("%v", point.Value)
		arw.processor.RecoverSingle(process.Hash(cachedRule.Id, arw.processor.DatasourceId(), point), point.Timestamp, &str)
	}
}

func (arw *AlertRuleWorker) Stop() {
	logger.Infof("rule_eval %s stopped", arw.Key())
	close(arw.quit)
}

func (arw *AlertRuleWorker) GetPromAnomalyPoint(ruleConfig string) []common.AnomalyPoint {
	var lst []common.AnomalyPoint
	var severity int

	var rule *models.PromRuleConfig
	if err := json.Unmarshal([]byte(ruleConfig), &rule); err != nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:%v", arw.Key(), ruleConfig, err)
		return lst
	}

	if rule == nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:rule is nil", arw.Key(), ruleConfig)
		return lst
	}

	arw.inhibit = rule.Inhibit
	for _, query := range rule.Queries {
		if query.Severity < severity {
			arw.severity = query.Severity
		}

		promql := strings.TrimSpace(query.PromQl)
		if promql == "" {
			logger.Errorf("rule_eval:%s promql is blank", arw.Key())
			continue
		}

		if arw.promClients.IsNil(arw.datasourceId) {
			logger.Errorf("rule_eval:%s error reader client is nil", arw.Key())
			continue
		}

		readerClient := arw.promClients.GetCli(arw.datasourceId)

		var warnings promsdk.Warnings
		value, warnings, err := readerClient.Query(context.Background(), promql, time.Now())
		if err != nil {
			logger.Errorf("rule_eval:%s promql:%s, error:%v", arw.Key(), promql, err)
			arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
			continue
		}

		if len(warnings) > 0 {
			logger.Errorf("rule_eval:%s promql:%s, warnings:%v", arw.Key(), promql, warnings)
			arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
			continue
		}

		logger.Debugf("rule_eval:%s query:%+v, value:%v", arw.Key(), query, value)
		points := common.ConvertAnomalyPoints(value)
		for i := 0; i < len(points); i++ {
			points[i].Severity = query.Severity
			points[i].Query = promql
		}
		lst = append(lst, points...)
	}
	return lst
}

func (arw *AlertRuleWorker) GetTdengineAnomalyPoint(rule *models.AlertRule, dsId int64) ([]common.AnomalyPoint, []common.AnomalyPoint) {
	// 获取查询和规则判断条件
	points := []common.AnomalyPoint{}
	recoverPoints := []common.AnomalyPoint{}
	ruleConfig := strings.TrimSpace(rule.RuleConfig)
	if ruleConfig == "" {
		logger.Warningf("rule_eval:%d promql is blank", rule.Id)
		return points, recoverPoints
	}

	var ruleQuery models.RuleQuery
	err := json.Unmarshal([]byte(ruleConfig), &ruleQuery)
	if err != nil {
		logger.Warningf("rule_eval:%d promql parse error:%s", rule.Id, err.Error())
		return points, recoverPoints
	}

	arw.inhibit = ruleQuery.Inhibit
	if len(ruleQuery.Queries) > 0 {
		seriesStore := make(map[uint64]*models.DataResp)
		seriesTagIndex := make(map[uint64][]uint64)

		for _, query := range ruleQuery.Queries {
			cli := arw.tdengineClients.GetCli(dsId)
			if cli == nil {
				logger.Warningf("rule_eval:%d tdengine client is nil", rule.Id)
				continue
			}

			series, err := cli.Query(query)
			if err != nil {
				logger.Warningf("rule_eval rid:%d query data error: %v", rule.Id, err)
				continue
			}

			//  此条日志很重要，是告警判断的现场值
			logger.Debugf("rule_eval rid:%d req:%+v resp:%+v", rule.Id, query, series)
			for i := 0; i < len(series); i++ {
				serieHash := hash.GetHash(series[i].Metric, series[i].Ref)
				tagHash := hash.GetTagHash(series[i].Metric)
				seriesStore[serieHash] = series[i]

				// 将曲线按照相同的 tag 分组
				if _, exists := seriesTagIndex[tagHash]; !exists {
					seriesTagIndex[tagHash] = make([]uint64, 0)
				}
				seriesTagIndex[tagHash] = append(seriesTagIndex[tagHash], serieHash)
			}
		}

		// 判断
		for _, trigger := range ruleQuery.Triggers {
			for _, seriesHash := range seriesTagIndex {
				m := make(map[string]float64)
				var ts int64
				var sample *models.DataResp
				var value float64
				for _, serieHash := range seriesHash {
					series, exists := seriesStore[serieHash]
					if !exists {
						logger.Warningf("rule_eval rid:%d series:%+v not found", rule.Id, series)
						continue
					}
					t, v, exists := series.Last()
					if !exists {
						logger.Warningf("rule_eval rid:%d series:%+v value not found", rule.Id, series)
						continue
					}

					if !strings.Contains(trigger.Exp, "$"+series.Ref) {
						// 表达式中不包含该变量
						continue
					}

					m["$"+series.Ref] = v
					m["$"+series.Ref+"."+series.MetricName()] = v
					ts = int64(t)
					sample = series
					value = v
				}
				isTriggered := parser.Calc(trigger.Exp, m)
				//  此条日志很重要，是告警判断的现场值
				logger.Debugf("rule_eval rid:%d trigger:%+v exp:%s res:%v m:%v", rule.Id, trigger, trigger.Exp, isTriggered, m)

				point := common.AnomalyPoint{
					Key:       sample.MetricName(),
					Labels:    sample.Metric,
					Timestamp: int64(ts),
					Value:     value,
					Severity:  trigger.Severity,
					Triggered: isTriggered,
				}

				if isTriggered {
					points = append(points, point)
				} else {
					recoverPoints = append(recoverPoints, point)
				}
			}
		}
	}

	return points, recoverPoints
}

func (arw *AlertRuleWorker) GetHostAnomalyPoint(ruleConfig string) []common.AnomalyPoint {
	var lst []common.AnomalyPoint
	var severity int

	var rule *models.HostRuleConfig
	if err := json.Unmarshal([]byte(ruleConfig), &rule); err != nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:%v", arw.Key(), ruleConfig, err)
		return lst
	}

	if rule == nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:rule is nil", arw.Key(), ruleConfig)
		return lst
	}

	arw.inhibit = rule.Inhibit
	now := time.Now().Unix()
	for _, trigger := range rule.Triggers {
		if trigger.Severity < severity {
			arw.severity = trigger.Severity
		}

		query := models.GetHostsQuery(rule.Queries)
		switch trigger.Type {
		case "target_miss":
			t := now - int64(trigger.Duration)
			targets, err := models.MissTargetGetsByFilter(arw.ctx, query, t)
			if err != nil {
				logger.Errorf("rule_eval:%s query:%v, error:%v", arw.Key(), query, err)
				arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
				continue
			}
			for _, target := range targets {
				m := make(map[string]string)
				target.FillTagsMap()
				for k, v := range target.TagsMap {
					m[k] = v
				}
				m["ident"] = target.Ident

				bg := arw.processor.BusiGroupCache.GetByBusiGroupId(target.GroupId)
				if bg != nil && bg.LabelEnable == 1 {
					m["busigroup"] = bg.LabelValue
				}

				lst = append(lst, common.NewAnomalyPoint(trigger.Type, m, now, float64(now-target.UpdateAt), trigger.Severity))
			}
		case "offset":
			targets, err := models.TargetGetsByFilter(arw.ctx, query, 0, 0)
			if err != nil {
				logger.Errorf("rule_eval:%s query:%v, error:%v", arw.Key(), query, err)
				arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
				continue
			}
			var targetMap = make(map[string]*models.Target)
			for _, target := range targets {
				targetMap[target.Ident] = target
			}

			hostOffsetMap := arw.processor.TargetCache.GetOffsetHost(targets, now, int64(trigger.Duration))
			for host, offset := range hostOffsetMap {
				m := make(map[string]string)
				target, exists := targetMap[host]
				if exists {
					target.FillTagsMap()
					for k, v := range target.TagsMap {
						m[k] = v
					}
				}
				m["ident"] = host

				bg := arw.processor.BusiGroupCache.GetByBusiGroupId(target.GroupId)
				if bg != nil && bg.LabelEnable == 1 {
					m["busigroup"] = bg.LabelValue
				}

				lst = append(lst, common.NewAnomalyPoint(trigger.Type, m, now, float64(offset), trigger.Severity))
			}
		case "pct_target_miss":
			t := now - int64(trigger.Duration)
			count, err := models.MissTargetCountByFilter(arw.ctx, query, t)
			if err != nil {
				logger.Errorf("rule_eval:%s query:%v, error:%v", arw.Key(), query, err)
				arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
				continue
			}

			total, err := models.TargetCountByFilter(arw.ctx, query)
			if err != nil {
				logger.Errorf("rule_eval:%s query:%v, error:%v", arw.Key(), query, err)
				arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
				continue
			}
			pct := float64(count) / float64(total) * 100
			if pct >= float64(trigger.Percent) {
				lst = append(lst, common.NewAnomalyPoint(trigger.Type, nil, now, pct, trigger.Severity))
			}
		}
	}
	return lst
}
