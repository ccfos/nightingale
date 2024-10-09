package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
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

const (
	GET_RULE_CONFIG = "get_rule_config"
	GET_PROCESSOR   = "get_processor"
	CHECK_QUERY     = "check_query_config"
	GET_CLIENT      = "get_client"
	QUERY_DATA      = "query_data"
)

type JoinType string

const (
	Left  JoinType = "left"
	Right JoinType = "right"
	Inner JoinType = "inner"
)

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

	if arw.inhibit {
		pointsMap := make(map[string]common.AnomalyPoint)
		for _, point := range recoverPoints {
			// 对于恢复的事件，合并处理
			tagHash := process.TagHash(point)

			p, exists := pointsMap[tagHash]
			if !exists {
				pointsMap[tagHash] = point
				continue
			}

			if p.Severity > point.Severity {
				hash := process.Hash(cachedRule.Id, arw.processor.DatasourceId(), p)
				arw.processor.DeleteProcessEvent(hash)
				models.AlertCurEventDelByHash(arw.ctx, hash)

				pointsMap[tagHash] = point
			}
		}

		now := time.Now().Unix()
		for _, point := range pointsMap {
			str := fmt.Sprintf("%v", point.Value)
			arw.processor.RecoverSingle(process.Hash(cachedRule.Id, arw.processor.DatasourceId(), point), now, &str)
		}
	} else {
		now := time.Now().Unix()
		for _, point := range recoverPoints {
			str := fmt.Sprintf("%v", point.Value)
			arw.processor.RecoverSingle(process.Hash(cachedRule.Id, arw.processor.DatasourceId(), point), now, &str)
		}
	}

	arw.processor.Handle(anomalyPoints, "inner", arw.inhibit)
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
		arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), GET_RULE_CONFIG).Inc()
		return lst
	}

	if rule == nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:rule is nil", arw.Key(), ruleConfig)
		arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), GET_RULE_CONFIG).Inc()
		return lst
	}

	arw.inhibit = rule.Inhibit
	for _, query := range rule.Queries {
		if query.Severity < severity {
			arw.severity = query.Severity
		}

		promql := strings.TrimSpace(query.PromQl)
		if promql == "" {
			logger.Warningf("rule_eval:%s promql is blank", arw.Key())
			arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), CHECK_QUERY).Inc()
			continue
		}

		if arw.promClients.IsNil(arw.datasourceId) {
			logger.Warningf("rule_eval:%s error reader client is nil", arw.Key())
			arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), GET_CLIENT).Inc()
			continue
		}

		readerClient := arw.promClients.GetCli(arw.datasourceId)

		var warnings promsdk.Warnings
		arw.processor.Stats.CounterQueryDataTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
		value, warnings, err := readerClient.Query(context.Background(), promql, time.Now())
		if err != nil {
			logger.Errorf("rule_eval:%s promql:%s, error:%v", arw.Key(), promql, err)
			arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
			arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), QUERY_DATA).Inc()
			continue
		}

		if len(warnings) > 0 {
			logger.Errorf("rule_eval:%s promql:%s, warnings:%v", arw.Key(), promql, warnings)
			arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
			arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), QUERY_DATA).Inc()
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
		arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), GET_RULE_CONFIG).Inc()
		return points, recoverPoints
	}

	var ruleQuery models.RuleQuery
	err := json.Unmarshal([]byte(ruleConfig), &ruleQuery)
	if err != nil {
		logger.Warningf("rule_eval:%d promql parse error:%s", rule.Id, err.Error())
		arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId())).Inc()
		arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), GET_RULE_CONFIG).Inc()
		return points, recoverPoints
	}

	arw.inhibit = ruleQuery.Inhibit
	if len(ruleQuery.Queries) > 0 {
		seriesStore := make(map[uint64]models.DataResp)
		// 将不同查询的 hash 索引分组存放
		seriesTagIndexes := make([]map[uint64][]uint64, 0)

		for _, query := range ruleQuery.Queries {
			seriesTagIndex := make(map[uint64][]uint64)

			arw.processor.Stats.CounterQueryDataTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
			cli := arw.tdengineClients.GetCli(dsId)
			if cli == nil {
				logger.Warningf("rule_eval:%d tdengine client is nil", rule.Id)
				arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
				arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), GET_CLIENT).Inc()
				continue
			}

			series, err := cli.Query(query)
			arw.processor.Stats.CounterQueryDataTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
			if err != nil {
				logger.Warningf("rule_eval rid:%d query data error: %v", rule.Id, err)
				arw.processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.datasourceId)).Inc()
				arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), QUERY_DATA).Inc()
				continue
			}
			//  此条日志很重要，是告警判断的现场值
			logger.Debugf("rule_eval rid:%d req:%+v resp:%+v", rule.Id, query, series)
			MakeSeriesMap(series, seriesTagIndex, seriesStore)
			seriesTagIndexes = append(seriesTagIndexes, seriesTagIndex)
		}

		points, recoverPoints = GetAnomalyPoint(rule.Id, ruleQuery, seriesTagIndexes, seriesStore)
	}

	return points, recoverPoints
}

func (arw *AlertRuleWorker) GetHostAnomalyPoint(ruleConfig string) []common.AnomalyPoint {
	var lst []common.AnomalyPoint
	var severity int

	var rule *models.HostRuleConfig
	if err := json.Unmarshal([]byte(ruleConfig), &rule); err != nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:%v", arw.Key(), ruleConfig, err)
		arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), GET_RULE_CONFIG).Inc()
		return lst
	}

	if rule == nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:rule is nil", arw.Key(), ruleConfig)
		arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), GET_RULE_CONFIG).Inc()
		return lst
	}

	arw.inhibit = rule.Inhibit
	now := time.Now().Unix()
	for _, trigger := range rule.Triggers {
		if trigger.Severity < severity {
			arw.severity = trigger.Severity
		}

		switch trigger.Type {
		case "target_miss":
			t := now - int64(trigger.Duration)

			var idents, engineIdents, missEngineIdents []string
			var exists bool
			if arw.ctx.IsCenter {
				// 如果是中心节点, 将不再上报数据的主机 engineName 为空的机器，也加入到 targets 中
				missEngineIdents, exists = arw.processor.TargetsOfAlertRuleCache.Get("", arw.rule.Id)
				if !exists {
					logger.Debugf("rule_eval:%s targets not found engineName:%s", arw.Key(), arw.processor.EngineName)
					arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), QUERY_DATA).Inc()
				}
			}
			idents = append(idents, missEngineIdents...)

			engineIdents, exists = arw.processor.TargetsOfAlertRuleCache.Get(arw.processor.EngineName, arw.rule.Id)
			if !exists {
				logger.Warningf("rule_eval:%s targets not found engineName:%s", arw.Key(), arw.processor.EngineName)
				arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), QUERY_DATA).Inc()
			}
			idents = append(idents, engineIdents...)

			if len(idents) == 0 {
				continue
			}

			var missTargets []string
			targetUpdateTimeMap := arw.processor.TargetCache.GetHostUpdateTime(idents)
			for ident, updateTime := range targetUpdateTimeMap {
				if updateTime < t {
					missTargets = append(missTargets, ident)
				}
			}
			logger.Debugf("rule_eval:%s missTargets:%v", arw.Key(), missTargets)
			targets := arw.processor.TargetCache.Gets(missTargets)
			for _, target := range targets {
				m := make(map[string]string)
				target.FillTagsMap()
				for k, v := range target.TagsMap {
					m[k] = v
				}
				m["ident"] = target.Ident

				lst = append(lst, common.NewAnomalyPoint(trigger.Type, m, now, float64(now-target.UpdateAt), trigger.Severity))
			}
		case "offset":
			idents, exists := arw.processor.TargetsOfAlertRuleCache.Get(arw.processor.EngineName, arw.rule.Id)
			if !exists {
				logger.Warningf("rule_eval:%s targets not found", arw.Key())
				arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), QUERY_DATA).Inc()
				continue
			}

			targets := arw.processor.TargetCache.Gets(idents)
			targetMap := make(map[string]*models.Target)
			for _, target := range targets {
				targetMap[target.Ident] = target
			}

			offsetIdents := make(map[string]int64)
			targetsMeta := arw.processor.TargetCache.GetHostMetas(targets)
			for ident, meta := range targetsMeta {
				if meta.CpuNum <= 0 {
					// means this target is not collect by categraf, do not check offset
					continue
				}
				if target, exists := targetMap[ident]; exists {
					if now-target.UpdateAt > 120 {
						// means this target is not a active host, do not check offset
						continue
					}
				}

				offset := meta.Offset
				if math.Abs(float64(offset)) > float64(trigger.Duration) {
					offsetIdents[ident] = offset
				}
			}

			logger.Debugf("rule_eval:%s offsetIdents:%v", arw.Key(), offsetIdents)
			for host, offset := range offsetIdents {
				m := make(map[string]string)
				target, exists := arw.processor.TargetCache.Get(host)
				if exists {
					target.FillTagsMap()
					for k, v := range target.TagsMap {
						m[k] = v
					}
				}
				m["ident"] = host

				lst = append(lst, common.NewAnomalyPoint(trigger.Type, m, now, float64(offset), trigger.Severity))
			}
		case "pct_target_miss":
			t := now - int64(trigger.Duration)
			idents, exists := arw.processor.TargetsOfAlertRuleCache.Get(arw.processor.EngineName, arw.rule.Id)
			if !exists {
				logger.Warningf("rule_eval:%s targets not found", arw.Key())
				arw.processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.processor.DatasourceId()), QUERY_DATA).Inc()
				continue
			}

			var missTargets []string
			targetUpdateTimeMap := arw.processor.TargetCache.GetHostUpdateTime(idents)
			for ident, updateTime := range targetUpdateTimeMap {
				if updateTime < t {
					missTargets = append(missTargets, ident)
				}
			}
			logger.Debugf("rule_eval:%s missTargets:%v", arw.Key(), missTargets)
			pct := float64(len(missTargets)) / float64(len(idents)) * 100
			if pct >= float64(trigger.Percent) {
				lst = append(lst, common.NewAnomalyPoint(trigger.Type, nil, now, pct, trigger.Severity))
			}
		}
	}
	return lst
}

func GetAnomalyPoint(ruleId int64, ruleQuery models.RuleQuery, seriesTagIndexes []map[uint64][]uint64, seriesStore map[uint64]models.DataResp) ([]common.AnomalyPoint, []common.AnomalyPoint) {
	points := []common.AnomalyPoint{}
	recoverPoints := []common.AnomalyPoint{}

	if len(ruleQuery.Triggers) == 0 {
		return points, recoverPoints
	}

	if len(seriesTagIndexes) == 0 {
		return points, recoverPoints
	}

	for _, trigger := range ruleQuery.Triggers {
		// seriesTagIndex 的 key 仅做分组使用，value 为每组 series 的 hash
		seriesTagIndex := make(map[uint64][]uint64)

		if len(trigger.Joins) == 0 {
			// 没有 join 条件，走原逻辑
			last := seriesTagIndexes[0]
			for i := 1; i < len(seriesTagIndexes); i++ {
				last = originalJoin(last, seriesTagIndexes[i])
			}
			seriesTagIndex = last
		} else {
			// 有 join 条件，按条件依次合并
			if len(seriesTagIndexes) != len(trigger.Joins)+1 {
				logger.Errorf("rule_eval rid:%d queries' count: %d not match join condition's count: %d", ruleId, len(seriesTagIndexes), len(trigger.Joins))
				continue
			}

			last := seriesTagIndexes[0]
			lastRehashed := rehashSet(last, seriesStore, trigger.Joins[0].On)
			for i := range trigger.Joins {
				cur := seriesTagIndexes[i+1]
				switch trigger.Joins[i].JoinType {
				case "original":
					last = originalJoin(last, cur)
				case "none":
					last = noneJoin(last, cur)
				case "cartesian":
					last = cartesianJoin(last, cur)
				case "inner_join":
					curRehashed := rehashSet(cur, seriesStore, trigger.Joins[i].On)
					lastRehashed = onJoin(lastRehashed, curRehashed, Inner)
					last = flatten(lastRehashed)
				case "left_join":
					curRehashed := rehashSet(cur, seriesStore, trigger.Joins[i].On)
					lastRehashed = onJoin(lastRehashed, curRehashed, Left)
					last = flatten(lastRehashed)
				case "right_join":
					curRehashed := rehashSet(cur, seriesStore, trigger.Joins[i].On)
					lastRehashed = onJoin(curRehashed, lastRehashed, Right)
					last = flatten(lastRehashed)
				case "left_exclude":
					curRehashed := rehashSet(cur, seriesStore, trigger.Joins[i].On)
					lastRehashed = exclude(lastRehashed, curRehashed)
					last = flatten(lastRehashed)
				case "right_exclude":
					curRehashed := rehashSet(cur, seriesStore, trigger.Joins[i].On)
					lastRehashed = exclude(curRehashed, lastRehashed)
					last = flatten(lastRehashed)
				default:
					logger.Warningf("rule_eval rid:%d join type:%s not support", ruleId, trigger.Joins[i].JoinType)
				}
			}
			seriesTagIndex = last
		}

		for _, seriesHash := range seriesTagIndex {
			sort.Slice(seriesHash, func(i, j int) bool {
				return seriesHash[i] < seriesHash[j]
			})

			m := make(map[string]interface{})
			var ts int64
			var sample models.DataResp
			var value float64
			for _, serieHash := range seriesHash {
				series, exists := seriesStore[serieHash]
				if !exists {
					logger.Warningf("rule_eval rid:%d series:%+v not found", ruleId, series)
					continue
				}
				t, v, exists := series.Last()
				if !exists {
					logger.Warningf("rule_eval rid:%d series:%+v value not found", ruleId, series)
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
			logger.Infof("rule_eval rid:%d trigger:%+v exp:%s res:%v m:%v", ruleId, trigger, trigger.Exp, isTriggered, m)

			var values string
			for k, v := range m {
				if !strings.Contains(k, ".") {
					continue
				}
				values += fmt.Sprintf("%s:%v ", k, v)
			}

			point := common.AnomalyPoint{
				Key:       sample.MetricName(),
				Labels:    sample.Metric,
				Timestamp: int64(ts),
				Value:     value,
				Values:    values,
				Severity:  trigger.Severity,
				Triggered: isTriggered,
				Query:     fmt.Sprintf("query:%+v trigger:%+v", ruleQuery.Queries, trigger),
			}

			if sample.Query != "" {
				point.Query = sample.Query
			}

			if isTriggered {
				points = append(points, point)
			} else {
				recoverPoints = append(recoverPoints, point)
			}
		}
	}

	return points, recoverPoints
}

func flatten(rehashed map[uint64][][]uint64) map[uint64][]uint64 {
	seriesTagIndex := make(map[uint64][]uint64)
	var i uint64
	for _, HashTagIndex := range rehashed {
		for u := range HashTagIndex {
			seriesTagIndex[i] = HashTagIndex[u]
			i++
		}
	}
	return seriesTagIndex
}

// onJoin 组合两个经过 rehash 之后的集合
// 如查询 A，经过 on data_base rehash 分组后
// [[A1{data_base=1, table=alert}，A2{data_base=1, table=alert}]，[A5{data_base=1, table=board}]]
// [[A3{data_base=2, table=board}]，[A4{data_base=2, table=alert}]]
// 查询 B，经过 on data_base rehash 分组后
// [[B1{data_base=1, table=alert}]]
// [[B2{data_base=2, table=alert}]]
// 内联得到
// [[A1{data_base=1, table=alert}，A2{data_base=1, table=alert}，B1{data_base=1, table=alert}]，[A5{data_base=1, table=board}，[B1{data_base=1, table=alert}]]
// [[A3{data_base=2, table=board}，B2{data_base=2, table=alert}]，[A4{data_base=2, table=alert}，B2{data_base=2, table=alert}]]
func onJoin(reHashTagIndex1 map[uint64][][]uint64, reHashTagIndex2 map[uint64][][]uint64, joinType JoinType) map[uint64][][]uint64 {
	reHashTagIndex := make(map[uint64][][]uint64)
	for rehash, _ := range reHashTagIndex1 {
		if _, ok := reHashTagIndex2[rehash]; ok {
			// 若有 rehash 相同的记录，两两合并
			for i1 := range reHashTagIndex1[rehash] {
				for i2 := range reHashTagIndex2[rehash] {
					reHashTagIndex[rehash] = append(reHashTagIndex[rehash], mergeNewArray(reHashTagIndex1[rehash][i1], reHashTagIndex2[rehash][i2]))
				}
			}
		} else {
			// 合并方式不为 inner 时，需要保留 reHashTagIndex1 中未匹配的记录
			if joinType != Inner {
				reHashTagIndex[rehash] = reHashTagIndex1[rehash]
			}
		}
	}
	return reHashTagIndex
}

// rehashSet 重新 hash 分组
// 如当前查询 A 有五条记录
// A1{data_base=1, table=alert}
// A2{data_base=1, table=alert}
// A3{data_base=2, table=board}
// A4{data_base=2, table=alert}
// A5{data_base=1, table=board}
// 经过预处理（按曲线分组，此步已在进入 GetAnomalyPoint 函数前完成）后，分为 4 组，
// [A1{data_base=1, table=alert}，A2{data_base=1, table=alert}]
// [A3{data_base=2, table=board}]
// [A4{data_base=2, table=alert}]
// [A5{data_base=1, table=board}]
// 若 rehashSet 按 data_base 重新分组，此时会得到按 rehash 值分的二维数组，即不会将 rehash 值相同的记录完全合并
// [[A1{data_base=1, table=alert}，A2{data_base=1, table=alert}]，[A5{data_base=1, table=board}]]
// [[A3{data_base=2, table=board}]，[A4{data_base=2, table=alert}]]
func rehashSet(seriesTagIndex1 map[uint64][]uint64, seriesStore map[uint64]models.DataResp, on []string) map[uint64][][]uint64 {
	reHashTagIndex := make(map[uint64][][]uint64)
	for _, seriesHashes := range seriesTagIndex1 {
		if len(seriesHashes) == 0 {
			continue
		}
		series, exists := seriesStore[seriesHashes[0]]
		if !exists {
			continue
		}
		rehash := hash.GetTargetTagHash(series.Metric, on)
		if _, ok := reHashTagIndex[rehash]; !ok {
			reHashTagIndex[rehash] = make([][]uint64, 0)
		}
		reHashTagIndex[rehash] = append(reHashTagIndex[rehash], seriesHashes)
	}
	return reHashTagIndex
}

// 笛卡尔积，查询的结果两两合并
func cartesianJoin(seriesTagIndex1 map[uint64][]uint64, seriesTagIndex2 map[uint64][]uint64) map[uint64][]uint64 {
	var index uint64
	seriesTagIndex := make(map[uint64][]uint64)
	for _, seriesHashes1 := range seriesTagIndex1 {
		for _, seriesHashes2 := range seriesTagIndex2 {
			seriesTagIndex[index] = mergeNewArray(seriesHashes1, seriesHashes2)
			index++
		}
	}
	return seriesTagIndex
}

// noneJoin 直接拼接
func noneJoin(seriesTagIndex1 map[uint64][]uint64, seriesTagIndex2 map[uint64][]uint64) map[uint64][]uint64 {
	seriesTagIndex := make(map[uint64][]uint64)
	var index uint64
	for _, seriesHashes := range seriesTagIndex1 {
		seriesTagIndex[index] = seriesHashes
		index++
	}
	for _, seriesHashes := range seriesTagIndex2 {
		seriesTagIndex[index] = seriesHashes
		index++
	}
	return seriesTagIndex
}

// originalJoin 原始分组方案，key 相同，即标签全部相同分为一组
func originalJoin(seriesTagIndex1 map[uint64][]uint64, seriesTagIndex2 map[uint64][]uint64) map[uint64][]uint64 {
	seriesTagIndex := make(map[uint64][]uint64)
	for tagHash, seriesHashes := range seriesTagIndex1 {
		if _, ok := seriesTagIndex[tagHash]; !ok {
			seriesTagIndex[tagHash] = mergeNewArray(seriesHashes)
		} else {
			seriesTagIndex[tagHash] = append(seriesTagIndex[tagHash], seriesHashes...)
		}
	}

	for tagHash, seriesHashes := range seriesTagIndex2 {
		if _, ok := seriesTagIndex[tagHash]; !ok {
			seriesTagIndex[tagHash] = mergeNewArray(seriesHashes)
		} else {
			seriesTagIndex[tagHash] = append(seriesTagIndex[tagHash], seriesHashes...)
		}
	}

	return seriesTagIndex
}

// exclude 左斥，留下在 reHashTagIndex1 中，但不在 reHashTagIndex2 中的记录
func exclude(reHashTagIndex1 map[uint64][][]uint64, reHashTagIndex2 map[uint64][][]uint64) map[uint64][][]uint64 {
	reHashTagIndex := make(map[uint64][][]uint64)
	for rehash, _ := range reHashTagIndex1 {
		if _, ok := reHashTagIndex2[rehash]; !ok {
			reHashTagIndex[rehash] = reHashTagIndex1[rehash]
		}
	}
	return reHashTagIndex
}

func MakeSeriesMap(series []models.DataResp, seriesTagIndex map[uint64][]uint64, seriesStore map[uint64]models.DataResp) {
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

func mergeNewArray(arg ...[]uint64) []uint64 {
	res := make([]uint64, 0)
	for _, a := range arg {
		res = append(res, a...)
	}
	return res
}
