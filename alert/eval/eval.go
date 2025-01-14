package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/hash"
	"github.com/ccfos/nightingale/v6/pkg/parser"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"
	promql2 "github.com/ccfos/nightingale/v6/pkg/promql"
	"github.com/ccfos/nightingale/v6/pkg/unit"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/prometheus/common/model"

	"github.com/robfig/cron/v3"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type AlertRuleWorker struct {
	DatasourceId int64
	Quit         chan struct{}
	Inhibit      bool
	Severity     int

	Rule *models.AlertRule

	Processor *process.Processor

	PromClients *prom.PromClientMap
	Ctx         *ctx.Context

	Scheduler *cron.Cron

	HostAndDeviceIdentCache sync.Map

	DeviceIdentHook func(arw *AlertRuleWorker, paramQuery models.ParamQuery) ([]string, error)
}

const (
	GET_RULE_CONFIG = "get_rule_config"
	GET_Processor   = "get_Processor"
	CHECK_QUERY     = "check_query_config"
	GET_CLIENT      = "get_client"
	QUERY_DATA      = "query_data"
)

const (
	JoinMark = "@@"
)

type JoinType string

const (
	Left  JoinType = "left"
	Right JoinType = "right"
	Inner JoinType = "inner"
)

func NewAlertRuleWorker(rule *models.AlertRule, datasourceId int64, Processor *process.Processor, promClients *prom.PromClientMap, ctx *ctx.Context) *AlertRuleWorker {
	arw := &AlertRuleWorker{
		DatasourceId: datasourceId,
		Quit:         make(chan struct{}),
		Rule:         rule,
		Processor:    Processor,

		PromClients:             promClients,
		Ctx:                     ctx,
		HostAndDeviceIdentCache: sync.Map{},
		DeviceIdentHook: func(arw *AlertRuleWorker, paramQuery models.ParamQuery) ([]string, error) {
			return nil, nil
		},
	}

	interval := rule.PromEvalInterval
	if interval <= 0 {
		interval = 10
	}

	if rule.CronPattern == "" {
		rule.CronPattern = fmt.Sprintf("@every %ds", interval)
	}

	arw.Scheduler = cron.New(cron.WithSeconds())

	entryID, err := arw.Scheduler.AddFunc(rule.CronPattern, func() {
		arw.Eval()
	})

	if err != nil {
		logger.Errorf("alert rule %s add cron pattern error: %v", arw.Key(), err)
	}

	Processor.ScheduleEntry = arw.Scheduler.Entry(entryID)

	Processor.PromEvalInterval = getPromEvalInterval(Processor.ScheduleEntry.Schedule)
	return arw
}

func getPromEvalInterval(schedule cron.Schedule) int {
	now := time.Now()
	next1 := schedule.Next(now)
	next2 := schedule.Next(next1)
	return int(next2.Sub(next1).Seconds())
}

func (arw *AlertRuleWorker) Key() string {
	return common.RuleKey(arw.DatasourceId, arw.Rule.Id)
}

func (arw *AlertRuleWorker) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%s_%s_%d",
		arw.Rule.Id,
		arw.Rule.CronPattern,
		arw.Rule.RuleConfig,
		arw.DatasourceId,
	))
}

func (arw *AlertRuleWorker) Prepare() {
	arw.Processor.RecoverAlertCurEventFromDb()
}

func (arw *AlertRuleWorker) Start() {
	arw.Scheduler.Start()
}

func (arw *AlertRuleWorker) Eval() {
	logger.Infof("eval:%s started", arw.Key())
	if arw.Processor.PromEvalInterval == 0 {
		arw.Processor.PromEvalInterval = getPromEvalInterval(arw.Processor.ScheduleEntry.Schedule)
	}

	cachedRule := arw.Rule
	if cachedRule == nil {
		// logger.Errorf("rule_eval:%s Rule not found", arw.Key())
		return
	}
	arw.Processor.Stats.CounterRuleEval.WithLabelValues().Inc()
	arw.HostAndDeviceIdentCache = sync.Map{}

	typ := cachedRule.GetRuleType()
	var (
		anomalyPoints []models.AnomalyPoint
		recoverPoints []models.AnomalyPoint
		err           error
	)

	switch typ {
	case models.PROMETHEUS:
		anomalyPoints, err = arw.GetPromAnomalyPoint(cachedRule.RuleConfig)
	case models.HOST:
		anomalyPoints, err = arw.GetHostAnomalyPoint(cachedRule.RuleConfig)
	case models.LOKI:
		anomalyPoints, err = arw.GetPromAnomalyPoint(cachedRule.RuleConfig)
	default:
		anomalyPoints, recoverPoints = arw.GetAnomalyPoint(cachedRule, arw.Processor.DatasourceId())
	}

	if err != nil {
		logger.Errorf("rule_eval:%s get anomaly point err:%s", arw.Key(), err.Error())
		return
	}

	if arw.Processor == nil {
		logger.Warningf("rule_eval:%s Processor is nil", arw.Key())
		return
	}

	if arw.Inhibit {
		pointsMap := make(map[string]models.AnomalyPoint)
		for _, point := range recoverPoints {
			// 对于恢复的事件，合并处理
			tagHash := process.TagHash(point)

			p, exists := pointsMap[tagHash]
			if !exists {
				pointsMap[tagHash] = point
				continue
			}

			if p.Severity > point.Severity {
				hash := process.Hash(cachedRule.Id, arw.Processor.DatasourceId(), p)
				arw.Processor.DeleteProcessEvent(hash)
				models.AlertCurEventDelByHash(arw.Ctx, hash)

				pointsMap[tagHash] = point
			}
		}

		now := time.Now().Unix()
		for _, point := range pointsMap {
			str := fmt.Sprintf("%v", point.Value)
			arw.Processor.RecoverSingle(true, process.Hash(cachedRule.Id, arw.Processor.DatasourceId(), point), now, &str)
		}
	} else {
		now := time.Now().Unix()
		for _, point := range recoverPoints {
			str := fmt.Sprintf("%v", point.Value)
			arw.Processor.RecoverSingle(true, process.Hash(cachedRule.Id, arw.Processor.DatasourceId(), point), now, &str)
		}
	}

	arw.Processor.Handle(anomalyPoints, "inner", arw.Inhibit)
}

func (arw *AlertRuleWorker) Stop() {
	logger.Infof("rule_eval %s stopped", arw.Key())
	close(arw.Quit)
	c := arw.Scheduler.Stop()
	<-c.Done()

}

func (arw *AlertRuleWorker) GetPromAnomalyPoint(ruleConfig string) ([]models.AnomalyPoint, error) {
	var lst []models.AnomalyPoint
	var severity int

	var rule *models.PromRuleConfig
	if err := json.Unmarshal([]byte(ruleConfig), &rule); err != nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:%v", arw.Key(), ruleConfig, err)
		arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_RULE_CONFIG, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
		return lst, err
	}

	if rule == nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:rule is nil", arw.Key(), ruleConfig)
		arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_RULE_CONFIG, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
		return lst, errors.New("rule is nil")
	}

	arw.Inhibit = rule.Inhibit
	for _, query := range rule.Queries {
		if query.Severity < severity {
			arw.Severity = query.Severity
		}

		readerClient := arw.PromClients.GetCli(arw.DatasourceId)

		if query.VarEnabled {
			var anomalyPoints []models.AnomalyPoint
			if hasLabelLossAggregator(query) || notExactMatch(query) {
				// 若有聚合函数或非精确匹配则需要先填充变量然后查询，这个方式效率较低
				anomalyPoints = arw.VarFillingBeforeQuery(query, readerClient)
			} else {
				// 先查询再过滤变量，效率较高，但无法处理有聚合函数的情况
				anomalyPoints = arw.VarFillingAfterQuery(query, readerClient)
			}
			lst = append(lst, anomalyPoints...)
		} else {
			// 无变量
			promql := strings.TrimSpace(query.PromQl)
			if promql == "" {
				logger.Warningf("rule_eval:%s promql is blank", arw.Key())
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), CHECK_QUERY, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
				continue
			}

			if arw.PromClients.IsNil(arw.DatasourceId) {
				logger.Warningf("rule_eval:%s error reader client is nil", arw.Key())
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_CLIENT, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
				continue
			}

			var warnings promsdk.Warnings
			arw.Processor.Stats.CounterQueryDataTotal.WithLabelValues(fmt.Sprintf("%d", arw.DatasourceId)).Inc()
			value, warnings, err := readerClient.Query(context.Background(), promql, time.Now())
			if err != nil {
				logger.Errorf("rule_eval:%s promql:%s, error:%v", arw.Key(), promql, err)
				arw.Processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.DatasourceId)).Inc()
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), QUERY_DATA, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
				return lst, err
			}

			if len(warnings) > 0 {
				logger.Errorf("rule_eval:%s promql:%s, warnings:%v", arw.Key(), promql, warnings)
				arw.Processor.Stats.CounterQueryDataErrorTotal.WithLabelValues(fmt.Sprintf("%d", arw.DatasourceId)).Inc()
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), QUERY_DATA, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
			}

			logger.Debugf("rule_eval:%s query:%+v, value:%v", arw.Key(), query, value)
			points := models.ConvertAnomalyPoints(value)
			for i := 0; i < len(points); i++ {
				points[i].Severity = query.Severity
				points[i].Query = promql
				points[i].ValuesUnit = map[string]unit.FormattedValue{
					"v": unit.ValueFormatter(query.Unit, 2, points[i].Value),
				}
			}

			lst = append(lst, points...)
		}
	}
	return lst, nil
}

type sample struct {
	Metric    model.Metric      `json:"metric"`
	Value     model.SampleValue `json:"value"`
	Timestamp model.Time
}

// VarFillingAfterQuery 填充变量，先查询再填充变量
// 公式: mem_used_percent{host="$host"} > $val 其中 $host 为参数变量，$val 为值变量
// 实现步骤:
// 依次遍历参数配置节点，保证同一参数变量的子筛选可以覆盖上一层筛选
// 每个节点先查询无参数的 query, 即 mem_used_percent{} > curVal, 得到满足值变量的所有结果
// 结果中有满足本节点参数变量的值，加入异常点列表
// 参数变量的值不满足的组合，需要覆盖上层筛选中产生的异常点
func (arw *AlertRuleWorker) VarFillingAfterQuery(query models.PromQuery, readerClient promsdk.API) []models.AnomalyPoint {
	varToLabel := ExtractVarMapping(query.PromQl)
	fullQuery := removeVal(query.PromQl)
	// 存储所有的异常点，key 为参数变量的组合，可以实现子筛选对上一层筛选的覆盖
	anomalyPointsMap := make(map[string]models.AnomalyPoint)
	// 统一变量配置格式
	VarConfigForCalc := &models.ChildVarConfig{
		ParamVal:        make([]map[string]models.ParamQuery, 1),
		ChildVarConfigs: query.VarConfig.ChildVarConfigs,
	}
	VarConfigForCalc.ParamVal[0] = make(map[string]models.ParamQuery)
	for _, p := range query.VarConfig.ParamVal {
		VarConfigForCalc.ParamVal[0][p.Name] = models.ParamQuery{
			ParamType: p.ParamType,
			Query:     p.Query,
		}
	}
	// 使用一个统一的参数变量顺序
	var ParamKeys []string
	for val, valQuery := range VarConfigForCalc.ParamVal[0] {
		if valQuery.ParamType == "threshold" {
			continue
		}
		ParamKeys = append(ParamKeys, val)
	}
	sort.Slice(ParamKeys, func(i, j int) bool {
		return ParamKeys[i] < ParamKeys[j]
	})
	// 遍历变量配置链表
	curNode := VarConfigForCalc
	for curNode != nil {
		for _, param := range curNode.ParamVal {
			// curQuery 当前节点的无参数 query，用于时序库查询
			curQuery := fullQuery
			// realQuery 当前节点产生异常点的 query，用于告警展示
			realQuery := query.PromQl
			// 取出阈值变量
			valMap := make(map[string]string)
			for val, valQuery := range param {
				if valQuery.ParamType == "threshold" {
					valMap[val] = getString(valQuery.Query)
				}
			}
			// 替换值变量
			for key, val := range valMap {
				curQuery = strings.Replace(curQuery, fmt.Sprintf("$%s", key), val, -1)
				realQuery = strings.Replace(realQuery, fmt.Sprintf("$%s", key), val, -1)
			}
			// 得到满足值变量的所有结果
			value, _, err := readerClient.Query(context.Background(), curQuery, time.Now())
			if err != nil {
				logger.Errorf("rule_eval:%s, promql:%s, error:%v", arw.Key(), curQuery, err)
				continue
			}
			seqVals := getSamples(value)
			// 得到参数变量的所有组合
			paramPermutation, err := arw.getParamPermutation(param, ParamKeys, varToLabel, query.PromQl, readerClient)
			if err != nil {
				logger.Errorf("rule_eval:%s, paramPermutation error:%v", arw.Key(), err)
				continue
			}
			// 判断哪些参数值符合条件
			for i := range seqVals {
				curRealQuery := realQuery
				var cur []string
				for _, paramKey := range ParamKeys {
					val := string(seqVals[i].Metric[model.LabelName(varToLabel[paramKey])])
					cur = append(cur, val)
					curRealQuery = fillVar(curRealQuery, paramKey, val)
				}

				if _, ok := paramPermutation[strings.Join(cur, JoinMark)]; ok {
					anomalyPointsMap[strings.Join(cur, JoinMark)] = models.AnomalyPoint{
						Key:       seqVals[i].Metric.String(),
						Timestamp: seqVals[i].Timestamp.Unix(),
						Value:     float64(seqVals[i].Value),
						Labels:    seqVals[i].Metric,
						Severity:  query.Severity,
						Query:     curRealQuery,
					}
					// 生成异常点后，删除该参数组合
					delete(paramPermutation, strings.Join(cur, JoinMark))
				}
			}

			// 剩余的参数组合为本层筛选不产生异常点的组合，需要覆盖上层筛选中产生的异常点
			for k, _ := range paramPermutation {
				delete(anomalyPointsMap, k)
			}
		}
		curNode = curNode.ChildVarConfigs
	}

	anomalyPoints := make([]models.AnomalyPoint, 0)
	for _, point := range anomalyPointsMap {
		anomalyPoints = append(anomalyPoints, point)
	}
	return anomalyPoints
}

// getSamples 获取查询结果的所有样本，并转化为统一的格式
func getSamples(value model.Value) []sample {
	var seqVals []sample
	switch value.Type() {
	case model.ValVector:
		items, ok := value.(model.Vector)
		if !ok {
			break
		}
		for i := range items {
			seqVals = append(seqVals, sample{
				Metric:    items[i].Metric,
				Value:     items[i].Value,
				Timestamp: items[i].Timestamp,
			})
		}
	case model.ValMatrix:
		items, ok := value.(model.Matrix)
		if !ok {
			break
		}
		for i := range items {
			last := items[i].Values[len(items[i].Values)-1]
			seqVals = append(seqVals, sample{
				Metric:    items[i].Metric,
				Value:     last.Value,
				Timestamp: last.Timestamp,
			})
		}
	default:
	}
	return seqVals
}

// removeVal 去除 promql 中的参数变量
// mem{test1=\"$test1\",test2=\"test2\"} > $val1 and mem{test3=\"test3\",test4=\"$test4\"} > $val2
// ==> mem{test2=\"test2\"} > $val1 and mem{test3=\"test3\"} > $val2
func removeVal(promql string) string {
	sb := strings.Builder{}
	n := len(promql)
	start := false
	lastIdx := 0
	curIdx := 0
	isVar := false
	for curIdx < n {
		if !start {
			if promql[curIdx] == '{' {
				start = true
				lastIdx = curIdx
			}
			sb.WriteRune(rune(promql[curIdx]))
		} else {
			if promql[curIdx] == '$' {
				isVar = true
			}
			if promql[curIdx] == ',' || promql[curIdx] == '}' {
				if !isVar {
					if sb.String()[sb.Len()-1] == '{' {
						lastIdx++
					}
					sb.WriteString(promql[lastIdx:curIdx])
				}
				isVar = false
				if promql[curIdx] == '}' {
					start = false
					sb.WriteRune(rune(promql[curIdx]))
				}
				lastIdx = curIdx
			}
		}
		curIdx++
	}

	return sb.String()
}

// 获取参数变量的所有组合
func (arw *AlertRuleWorker) getParamPermutation(paramVal map[string]models.ParamQuery, paramKeys []string, varToLabel map[string]string, originPromql string, readerClient promsdk.API) (map[string]struct{}, error) {

	// 参数变量查询，得到参数变量值
	paramMap := make(map[string][]string)
	for _, paramKey := range paramKeys {
		var params []string
		paramQuery, ok := paramVal[paramKey]
		if !ok {
			return nil, fmt.Errorf("param key not found: %s", paramKey)
		}
		switch paramQuery.ParamType {
		case "host":
			hostIdents, err := arw.getHostIdents(paramQuery)
			if err != nil {
				logger.Errorf("rule_eval:%s, fail to get host idents, error:%v", arw.Key(), err)
				break
			}
			params = hostIdents
		case "device":
			deviceIdents, err := arw.getDeviceIdents(paramQuery)
			if err != nil {
				logger.Errorf("rule_eval:%s, fail to get device idents, error:%v", arw.Key(), err)
				break
			}
			params = deviceIdents
		case "enum":
			q, _ := json.Marshal(paramQuery.Query)
			var query []string
			err := json.Unmarshal(q, &query)
			if err != nil {
				logger.Errorf("query:%s fail to unmarshalling into string slice, error:%v", paramQuery.Query, err)
			}
			if len(query) == 0 {
				paramsKeyAllLabel, err := getParamKeyAllLabel(varToLabel[paramKey], originPromql, readerClient)
				if err != nil {
					logger.Errorf("rule_eval:%s, fail to getParamKeyAllLabel, error:%v", arw.Key(), paramQuery.Query, err)
				}
				params = paramsKeyAllLabel
			} else {
				params = query
			}
		default:
			return nil, fmt.Errorf("unknown param type: %s", paramQuery.ParamType)
		}

		if len(params) == 0 {
			return nil, fmt.Errorf("param key: %s, params is empty", paramKey)
		}

		logger.Infof("rule_eval:%s paramKey: %s, params: %v", arw.Key(), paramKey, params)
		paramMap[paramKey] = params
	}

	// 得到以 paramKeys 为顺序的所有参数组合
	permutation := mapPermutation(paramKeys, paramMap)

	res := make(map[string]struct{})
	for i := range permutation {
		res[strings.Join(permutation[i], JoinMark)] = struct{}{}
	}

	return res, nil
}

func getParamKeyAllLabel(paramKey string, promql string, client promsdk.API) ([]string, error) {
	labels, metricName, err := promql2.GetLabelsAndMetricNameWithReplace(promql, "$")
	if err != nil {
		return nil, fmt.Errorf("promql:%s, get labels error:%v", promql, err)
	}
	labelstrs := make([]string, 0)
	for _, label := range labels {
		if strings.HasPrefix(label.Value, "$") {
			continue
		}
		labelstrs = append(labelstrs, label.Name+label.Op+label.Value)
	}
	pr := metricName + "{" + strings.Join(labelstrs, ",") + "}"

	value, _, err := client.Query(context.Background(), pr, time.Now())
	if err != nil {
		return nil, fmt.Errorf("promql: %s query error: %v", pr, err)
	}
	labelValuesMap := make(map[string]struct{})

	switch value.Type() {
	case model.ValVector:
		vector := value.(model.Vector)
		for _, sample := range vector {
			for labelName, labelValue := range sample.Metric {
				// 只处理ParamKeys中指定的label
				if string(labelName) == paramKey {
					labelValuesMap[string(labelValue)] = struct{}{}
				}
			}
		}
	case model.ValMatrix:
		matrix := value.(model.Matrix)
		for _, series := range matrix {
			for labelName, labelValue := range series.Metric {
				// 只处理ParamKeys中指定的label
				if string(labelName) == paramKey {
					labelValuesMap[string(labelValue)] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0)
	for labelValue, _ := range labelValuesMap {
		result = append(result, labelValue)
	}

	return result, nil
}

func (arw *AlertRuleWorker) getHostIdents(paramQuery models.ParamQuery) ([]string, error) {
	var params []string
	q, _ := json.Marshal(paramQuery.Query)

	cacheKey := "Host_" + string(q)
	value, hit := arw.HostAndDeviceIdentCache.Load(cacheKey)
	if idents, ok := value.([]string); hit && ok {
		params = idents
		return params, nil
	}

	var queries []models.HostQuery
	err := json.Unmarshal(q, &queries)
	if err != nil {
		return nil, err
	}

	hostsQuery := models.GetHostsQuery(queries)
	session := models.TargetFilterQueryBuild(arw.Ctx, hostsQuery, 0, 0)
	var lst []*models.Target
	err = session.Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for i := range lst {
		params = append(params, lst[i].Ident)
	}
	arw.HostAndDeviceIdentCache.Store(cacheKey, params)
	return params, nil
}

func (arw *AlertRuleWorker) getDeviceIdents(paramQuery models.ParamQuery) ([]string, error) {
	return arw.DeviceIdentHook(arw, paramQuery)
}

// 生成所有排列组合
func mapPermutation(paramKeys []string, paraMap map[string][]string) [][]string {
	var result [][]string
	current := make([]string, len(paramKeys))
	combine(paramKeys, paraMap, 0, current, &result)
	return result
}

// 递归生成所有排列组合
func combine(paramKeys []string, paraMap map[string][]string, index int, current []string, result *[][]string) {
	// 当到达最后一个 key 时，存储当前的组合
	if index == len(paramKeys) {
		combination := make([]string, len(current))
		copy(combination, current)
		*result = append(*result, combination)
		return
	}

	// 获取当前 key 对应的 value 列表
	key := paramKeys[index]
	valueList := paraMap[key]

	// 遍历每个 value，并递归生成下一个 key 的组合
	for _, value := range valueList {
		current[index] = value
		combine(paramKeys, paraMap, index+1, current, result)
	}
}

func (arw *AlertRuleWorker) GetHostAnomalyPoint(ruleConfig string) ([]models.AnomalyPoint, error) {
	var lst []models.AnomalyPoint
	var severity int

	var rule *models.HostRuleConfig
	if err := json.Unmarshal([]byte(ruleConfig), &rule); err != nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:%v", arw.Key(), ruleConfig, err)
		arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_RULE_CONFIG, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
		return lst, err
	}

	if rule == nil {
		logger.Errorf("rule_eval:%s rule_config:%s, error:rule is nil", arw.Key(), ruleConfig)
		arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_RULE_CONFIG, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
		return lst, errors.New("rule is nil")
	}

	arw.Inhibit = rule.Inhibit
	now := time.Now().Unix()
	for _, trigger := range rule.Triggers {
		if trigger.Severity < severity {
			arw.Severity = trigger.Severity
		}

		switch trigger.Type {
		case "target_miss":
			t := now - int64(trigger.Duration)

			var idents, engineIdents, missEngineIdents []string
			var exists bool
			if arw.Ctx.IsCenter {
				// 如果是中心节点, 将不再上报数据的主机 engineName 为空的机器，也加入到 targets 中
				missEngineIdents, exists = arw.Processor.TargetsOfAlertRuleCache.Get("", arw.Rule.Id)
				if !exists {
					logger.Debugf("rule_eval:%s targets not found engineName:%s", arw.Key(), arw.Processor.EngineName)
					arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), QUERY_DATA, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
				}
			}
			idents = append(idents, missEngineIdents...)

			engineIdents, exists = arw.Processor.TargetsOfAlertRuleCache.Get(arw.Processor.EngineName, arw.Rule.Id)
			if !exists {
				logger.Warningf("rule_eval:%s targets not found engineName:%s", arw.Key(), arw.Processor.EngineName)
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), QUERY_DATA, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
			}
			idents = append(idents, engineIdents...)

			if len(idents) == 0 {
				continue
			}

			var missTargets []string
			targetUpdateTimeMap := arw.Processor.TargetCache.GetHostUpdateTime(idents)
			for ident, updateTime := range targetUpdateTimeMap {
				if updateTime < t {
					missTargets = append(missTargets, ident)
				}
			}
			logger.Debugf("rule_eval:%s missTargets:%v", arw.Key(), missTargets)
			targets := arw.Processor.TargetCache.Gets(missTargets)
			for _, target := range targets {
				m := make(map[string]string)
				for k, v := range target.TagsMap {
					m[k] = v
				}
				m["ident"] = target.Ident

				lst = append(lst, models.NewAnomalyPoint(trigger.Type, m, now, float64(now-target.UpdateAt), trigger.Severity))
			}
		case "offset":
			idents, exists := arw.Processor.TargetsOfAlertRuleCache.Get(arw.Processor.EngineName, arw.Rule.Id)
			if !exists {
				logger.Warningf("rule_eval:%s targets not found", arw.Key())
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), QUERY_DATA, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
				continue
			}

			targets := arw.Processor.TargetCache.Gets(idents)
			targetMap := make(map[string]*models.Target)
			for _, target := range targets {
				targetMap[target.Ident] = target
			}

			offsetIdents := make(map[string]int64)
			targetsMeta := arw.Processor.TargetCache.GetHostMetas(targets)
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
				target, exists := arw.Processor.TargetCache.Get(host)
				if exists {
					for k, v := range target.TagsMap {
						m[k] = v
					}
				}
				m["ident"] = host

				lst = append(lst, models.NewAnomalyPoint(trigger.Type, m, now, float64(offset), trigger.Severity))
			}
		case "pct_target_miss":
			t := now - int64(trigger.Duration)
			idents, exists := arw.Processor.TargetsOfAlertRuleCache.Get(arw.Processor.EngineName, arw.Rule.Id)
			if !exists {
				logger.Warningf("rule_eval:%s targets not found", arw.Key())
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), QUERY_DATA, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
				continue
			}

			var missTargets []string
			targetUpdateTimeMap := arw.Processor.TargetCache.GetHostUpdateTime(idents)
			for ident, updateTime := range targetUpdateTimeMap {
				if updateTime < t {
					missTargets = append(missTargets, ident)
				}
			}
			logger.Debugf("rule_eval:%s missTargets:%v", arw.Key(), missTargets)
			pct := float64(len(missTargets)) / float64(len(idents)) * 100
			if pct >= float64(trigger.Percent) {
				lst = append(lst, models.NewAnomalyPoint(trigger.Type, nil, now, pct, trigger.Severity))
			}
		}
	}
	return lst, nil
}

func GetAnomalyPoint(ruleId int64, ruleQuery models.RuleQuery, seriesTagIndexes map[string]map[uint64][]uint64, seriesStore map[uint64]models.DataResp) ([]models.AnomalyPoint, []models.AnomalyPoint) {
	points := []models.AnomalyPoint{}
	recoverPoints := []models.AnomalyPoint{}

	if len(ruleQuery.Triggers) == 0 {
		return points, recoverPoints
	}

	if len(seriesTagIndexes) == 0 {
		return points, recoverPoints
	}

	unitMap := make(map[string]string)
	for _, query := range ruleQuery.Queries {
		ref, unit, err := GetQueryRefAndUnit(query)
		if err != nil {
			continue
		}
		unitMap[ref] = unit
	}

	for _, trigger := range ruleQuery.Triggers {
		// seriesTagIndex 的 key 仅做分组使用，value 为每组 series 的 hash
		seriesTagIndex := ProcessJoins(ruleId, trigger, seriesTagIndexes, seriesStore)

		for _, seriesHash := range seriesTagIndex {
			valuesUnitMap := make(map[string]unit.FormattedValue)

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

				if u, exists := unitMap[series.Ref]; exists {
					valuesUnitMap[series.Ref] = unit.ValueFormatter(u, 2, v)
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

			point := models.AnomalyPoint{
				Key:           sample.MetricName(),
				Labels:        sample.Metric,
				Timestamp:     int64(ts),
				Value:         value,
				Values:        values,
				Severity:      trigger.Severity,
				Triggered:     isTriggered,
				Query:         fmt.Sprintf("query:%+v trigger:%+v", ruleQuery.Queries, trigger),
				RecoverConfig: trigger.RecoverConfig,
				ValuesUnit:    valuesUnitMap,
			}

			if sample.Query != "" {
				point.Query = sample.Query
			}
			// 恢复条件判断经过讨论是只在表达式模式下支持，表达式模式会通过 isTriggered 判断是告警点还是恢复点
			// 1. 不设置恢复判断，满足恢复条件产生 recoverPoint 恢复，无数据不产生 anomalyPoint 恢复
			// 2. 设置满足条件才恢复，仅可通过产生 recoverPoint 恢复，不能通过不产生 anomalyPoint 恢复
			// 3. 设置无数据不恢复，仅可通过产生 recoverPoint 恢复，不产生 anomalyPoint 恢复
			if isTriggered {
				points = append(points, point)
			} else {
				switch trigger.RecoverConfig.JudgeType {
				case models.Origin:
					// 对齐原实现 do nothing
				case models.RecoverOnCondition:
					// 额外判断恢复条件，满足才恢复
					fulfill := parser.Calc(trigger.RecoverConfig.RecoverExp, m)
					if !fulfill {
						continue
					}
				}
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
	for rehash := range reHashTagIndex1 {
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

func ProcessJoins(ruleId int64, trigger models.Trigger, seriesTagIndexes map[string]map[uint64][]uint64, seriesStore map[uint64]models.DataResp) map[uint64][]uint64 {
	last := make(map[uint64][]uint64)
	if len(seriesTagIndexes) == 0 {
		return last
	}

	if len(trigger.Joins) == 0 {
		idx := 0
		for _, seriesTagIndex := range seriesTagIndexes {
			if idx == 0 {
				last = seriesTagIndex
			} else {
				last = originalJoin(last, seriesTagIndex)
			}
			idx++
		}
		return last
	}

	// 有 join 条件，按条件依次合并
	if len(seriesTagIndexes) < len(trigger.Joins)+1 {
		logger.Errorf("rule_eval rid:%d queries' count: %d not match join condition's count: %d", ruleId, len(seriesTagIndexes), len(trigger.Joins))
		return nil
	}

	last = seriesTagIndexes[trigger.JoinRef]
	lastRehashed := rehashSet(last, seriesStore, trigger.Joins[0].On)
	for i := range trigger.Joins {
		cur := seriesTagIndexes[trigger.Joins[i].Ref]
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
	return last
}

func GetQueryRef(query interface{}) (string, error) {
	// 首先检查是否为 map
	if m, ok := query.(map[string]interface{}); ok {
		if ref, exists := m["ref"]; exists {
			if refStr, ok := ref.(string); ok {
				return refStr, nil
			}
			return "", fmt.Errorf("ref 字段不是字符串类型")
		}
		return "", fmt.Errorf("query 中没有找到 ref 字段")
	}

	// 如果不是 map，则按原来的方式处理结构体
	v := reflect.ValueOf(query)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return "", fmt.Errorf("query not a struct or map")
	}

	refField := v.FieldByName("Ref")
	if !refField.IsValid() {
		return "", fmt.Errorf("not find ref field")
	}

	if refField.Kind() != reflect.String {
		return "", fmt.Errorf("ref not a string")
	}

	return refField.String(), nil
}

// query 可能是 string 或是 int int64 float64 等数字，全部转为 string
func getString(query interface{}) string {
	switch query.(type) {
	case string:
		return query.(string)
	case float64:
		return strconv.FormatFloat(query.(float64), 'f', -1, 64)
	default:
		return ""
	}
}

func GetQueryRefAndUnit(query interface{}) (string, string, error) {
	type Query struct {
		Ref  string `json:"ref"`
		Unit string `json:"unit"`
	}

	queryMap := Query{}
	queryBytes, err := json.Marshal(query)
	if err != nil {
		return "", "", err
	}
	json.Unmarshal(queryBytes, &queryMap)
	return queryMap.Ref, queryMap.Unit, nil
}

// VarFillingBeforeQuery 填充变量，先填充变量再查询，针对有聚合函数的情况
// 公式: avg(mem_used_percent{host="$host"}) > $val 其中 $host 为参数变量，$val 为值变量
// 实现步骤:
// 依次遍历参数配置节点，保证同一参数变量的子筛选可以覆盖上一层筛选
// 每个节点先填充参数再进行查询, 即先得到完整的 promql avg(mem_used_percent{host="127.0.0.1"}) > 5
// 再查询得到满足值变量的所有结果加入异常点列表
// 参数变量的值不满足的组合，需要覆盖上层筛选中产生的异常点
func (arw *AlertRuleWorker) VarFillingBeforeQuery(query models.PromQuery, readerClient promsdk.API) []models.AnomalyPoint {
	varToLabel := ExtractVarMapping(query.PromQl)
	// 存储异常点的 map，key 为参数变量的组合，可以实现子筛选对上一层筛选的覆盖
	anomalyPointsMap := sync.Map{}
	// 统一变量配置格式
	VarConfigForCalc := &models.ChildVarConfig{
		ParamVal:        make([]map[string]models.ParamQuery, 1),
		ChildVarConfigs: query.VarConfig.ChildVarConfigs,
	}
	VarConfigForCalc.ParamVal[0] = make(map[string]models.ParamQuery)
	for _, p := range query.VarConfig.ParamVal {
		VarConfigForCalc.ParamVal[0][p.Name] = models.ParamQuery{
			ParamType: p.ParamType,
			Query:     p.Query,
		}
	}
	// 使用一个统一的参数变量顺序
	var ParamKeys []string
	for val, valQuery := range VarConfigForCalc.ParamVal[0] {
		if valQuery.ParamType == "threshold" {
			continue
		}
		ParamKeys = append(ParamKeys, val)
	}
	sort.Slice(ParamKeys, func(i, j int) bool {
		return ParamKeys[i] < ParamKeys[j]
	})
	// 遍历变量配置链表
	curNode := VarConfigForCalc
	for curNode != nil {
		for _, param := range curNode.ParamVal {
			curPromql := query.PromQl
			// 取出阈值变量
			valMap := make(map[string]string)
			for val, valQuery := range param {
				if valQuery.ParamType == "threshold" {
					valMap[val] = getString(valQuery.Query)
				}
			}
			// 替换阈值变量
			for key, val := range valMap {
				curPromql = strings.Replace(curPromql, fmt.Sprintf("$%s", key), val, -1)
			}
			// 得到参数变量的所有组合
			paramPermutation, err := arw.getParamPermutation(param, ParamKeys, varToLabel, query.PromQl, readerClient)
			if err != nil {
				logger.Errorf("rule_eval:%s, paramPermutation error:%v", arw.Key(), err)
				continue
			}

			keyToPromql := make(map[string]string)
			for paramPermutationKeys, _ := range paramPermutation {
				realPromql := curPromql
				split := strings.Split(paramPermutationKeys, JoinMark)
				for j := range ParamKeys {
					realPromql = fillVar(realPromql, ParamKeys[j], split[j])
				}
				keyToPromql[paramPermutationKeys] = realPromql
			}

			// 并发查询
			wg := sync.WaitGroup{}
			semaphore := make(chan struct{}, 200)
			for key, promql := range keyToPromql {
				wg.Add(1)
				semaphore <- struct{}{}
				go func(key, promql string) {
					defer func() {
						<-semaphore
						wg.Done()
					}()
					value, _, err := readerClient.Query(context.Background(), promql, time.Now())
					if err != nil {
						logger.Errorf("rule_eval:%s, promql:%s, error:%v", arw.Key(), promql, err)
						return
					}
					logger.Infof("rule_eval:%s, promql:%s, value:%+v", arw.Key(), promql, value)

					points := models.ConvertAnomalyPoints(value)
					if len(points) == 0 {
						anomalyPointsMap.Delete(key)
						return
					}
					for i := 0; i < len(points); i++ {
						points[i].Severity = query.Severity
						points[i].Query = promql
						points[i].ValuesUnit = map[string]unit.FormattedValue{
							"v": unit.ValueFormatter(query.Unit, 2, points[i].Value),
						}
					}
					anomalyPointsMap.Store(key, points)
				}(key, promql)
			}
			wg.Wait()
		}
		curNode = curNode.ChildVarConfigs
	}
	anomalyPoints := make([]models.AnomalyPoint, 0)
	anomalyPointsMap.Range(func(key, value any) bool {
		if points, ok := value.([]models.AnomalyPoint); ok {
			anomalyPoints = append(anomalyPoints, points...)
		}
		return true
	})
	return anomalyPoints
}

// 判断 query 中是否有会导致标签丢失的聚合函数
func hasLabelLossAggregator(query models.PromQuery) bool {
	noLabelAggregators := []string{
		"sum", "min", "max", "avg",
		"stddev", "stdvar",
		"count", "quantile",
		"group",
	}
	promql := strings.ToLower(query.PromQl)

	for _, fn := range noLabelAggregators {
		// 检查是否包含这些聚合函数，需要确保函数名后面跟着左括号
		if strings.Contains(promql, fn+"(") {
			return true
		}
	}

	return false
}

// 判断 query 中是否有 != =~ !~
func notExactMatch(query models.PromQuery) bool {
	promql := strings.ToLower(query.PromQl)
	if strings.Contains(promql, "!=") || strings.Contains(promql, "=~") || strings.Contains(promql, "!~") {
		return true
	}
	return false
}

// ExtractVarMapping 从 promql 中提取变量映射关系，为了在 query 之后可以将标签正确的放回 promql
// 输入: sum(rate(mem_used_percent{host="$my_host"})) by (instance) + avg(node_load1{region="$region"}) > $val
// 输出: map[string]string{"my_host":"host", "region":"region"}
func ExtractVarMapping(promql string) map[string]string {
	varMapping := make(map[string]string)

	// 遍历所有花括号对
	for {
		start := strings.Index(promql, "{")
		if start == -1 {
			break
		}

		end := strings.Index(promql, "}")
		if end == -1 {
			break
		}

		// 提取标签键值对
		labels := promql[start+1 : end]
		pairs := strings.Split(labels, ",")

		for _, pair := range pairs {
			// 分割键值对
			kv := strings.Split(pair, "=")
			if len(kv) != 2 {
				continue
			}

			key := strings.TrimSpace(kv[0])
			value := strings.Trim(strings.TrimSpace(kv[1]), "\"")
			value = strings.Trim(value, "'")

			// 检查值是否为变量(以$开头)
			if strings.HasPrefix(value, "$") {
				varName := value[1:] // 去掉$前缀
				varMapping[varName] = key
			}
		}

		// 继续处理剩余部分
		promql = promql[end+1:]
	}

	return varMapping
}

func fillVar(curRealQuery string, paramKey string, val string) string {
	curRealQuery = strings.Replace(curRealQuery, fmt.Sprintf("'$%s'", paramKey), fmt.Sprintf("'%s'", val), -1)
	curRealQuery = strings.Replace(curRealQuery, fmt.Sprintf("\"$%s\"", paramKey), fmt.Sprintf("\"%s\"", val), -1)
	return curRealQuery
}

func (arw *AlertRuleWorker) GetAnomalyPoint(rule *models.AlertRule, dsId int64) ([]models.AnomalyPoint, []models.AnomalyPoint) {
	// 获取查询和规则判断条件
	points := []models.AnomalyPoint{}
	recoverPoints := []models.AnomalyPoint{}
	ruleConfig := strings.TrimSpace(rule.RuleConfig)
	if ruleConfig == "" {
		logger.Warningf("rule_eval:%d promql is blank", rule.Id)
		arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_RULE_CONFIG, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
		return points, recoverPoints
	}

	var ruleQuery models.RuleQuery
	err := json.Unmarshal([]byte(ruleConfig), &ruleQuery)
	if err != nil {
		logger.Warningf("rule_eval:%d promql parse error:%s", rule.Id, err.Error())
		arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_RULE_CONFIG, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
		return points, recoverPoints
	}

	arw.Inhibit = ruleQuery.Inhibit
	if len(ruleQuery.Queries) > 0 {
		seriesStore := make(map[uint64]models.DataResp)
		seriesTagIndexes := make(map[string]map[uint64][]uint64, 0)
		for _, query := range ruleQuery.Queries {
			seriesTagIndex := make(map[uint64][]uint64)

			plug, exists := dscache.DsCache.Get(rule.Cate, dsId)
			if !exists {
				logger.Warningf("rule_eval rid:%d datasource:%d not exists", rule.Id, dsId)
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_CLIENT, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
				continue
			}

			series, err := plug.QueryData(context.Background(), query)
			arw.Processor.Stats.CounterQueryDataTotal.WithLabelValues(fmt.Sprintf("%d", arw.DatasourceId)).Inc()
			if err != nil {
				logger.Warningf("rule_eval rid:%d query data error: %v", rule.Id, err)
				arw.Processor.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", arw.Processor.DatasourceId()), GET_CLIENT, arw.Processor.BusiGroupCache.GetNameByBusiGroupId(arw.Rule.GroupId), fmt.Sprintf("%v", arw.Rule.Id)).Inc()
				continue
			}

			//  此条日志很重要，是告警判断的现场值
			logger.Infof("rule_eval rid:%d req:%+v resp:%v", rule.Id, query, series)
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
			ref, err := GetQueryRef(query)
			if err != nil {
				logger.Warningf("rule_eval rid:%d query:%+v get ref error:%s", rule.Id, query, err.Error())
				continue
			}
			seriesTagIndexes[ref] = seriesTagIndex
		}

		unitMap := make(map[string]string)
		for _, query := range ruleQuery.Queries {
			ref, unit, err := GetQueryRefAndUnit(query)
			if err != nil {
				continue
			}
			unitMap[ref] = unit
		}

		// 判断
		for _, trigger := range ruleQuery.Triggers {
			seriesTagIndex := ProcessJoins(rule.Id, trigger, seriesTagIndexes, seriesStore)
			for _, seriesHash := range seriesTagIndex {
				valuesUnitMap := make(map[string]unit.FormattedValue)

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
					for k, v := range series.Metric {
						if k == "__name__" {
							continue
						}

						if !strings.Contains(trigger.Exp, "$"+series.Ref+"."+string(k)) {
							// 过滤掉表达式中不包含的标签
							continue
						}

						m["$"+series.Ref+"."+string(k)] = string(v)
					}

					if u, exists := unitMap[series.Ref]; exists {
						valuesUnitMap["$"+series.Ref+"."+series.MetricName()] = unit.ValueFormatter(u, 2, v)
					}

					ts = int64(t)
					sample = series
					value = v
					logger.Infof("rule_eval rid:%d origin series labels:%+v", rule.Id, series.Metric)
				}

				isTriggered := parser.CalcWithRid(trigger.Exp, m, rule.Id)
				//  此条日志很重要，是告警判断的现场值
				logger.Infof("rule_eval rid:%d trigger:%+v exp:%s res:%v m:%v", rule.Id, trigger, trigger.Exp, isTriggered, m)

				var values string
				for k, v := range m {
					if !strings.Contains(k, ".") {
						continue
					}

					switch v.(type) {
					case float64:
						values += fmt.Sprintf("%s:%.3f ", k, v)
					case string:
						values += fmt.Sprintf("%s:%s ", k, v)
					}
				}

				point := models.AnomalyPoint{
					Key:           sample.MetricName(),
					Labels:        sample.Metric,
					Timestamp:     int64(ts),
					Value:         value,
					Values:        values,
					Severity:      trigger.Severity,
					Triggered:     isTriggered,
					Query:         fmt.Sprintf("query:%+v trigger:%+v", ruleQuery.Queries, trigger),
					RecoverConfig: trigger.RecoverConfig,
					ValuesUnit:    valuesUnitMap,
				}

				if isTriggered {
					points = append(points, point)
				} else {
					switch trigger.RecoverConfig.JudgeType {
					case models.Origin:
						// do nothing
					case models.RecoverOnCondition:
						fulfill := parser.CalcWithRid(trigger.RecoverConfig.RecoverExp, m, rule.Id)
						if !fulfill {
							continue
						}
					}
					recoverPoints = append(recoverPoints, point)
				}
			}
		}
	}

	return points, recoverPoints
}
