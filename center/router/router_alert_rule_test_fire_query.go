package router

// 告警规则「模拟触发」的 query_check 阶段：真实执行规则里的查询条件，
// 并对与查询分离的判断条件（triggers[].exp）做 引用 / 语法 / 实算 三层检测。
// 查询是只读动作且引擎本来就会周期执行同款查询，因此干跑（skip_send）时也照常检测；
// 检测失败只在报告中标红，不阻断后续阶段（数据源不可达时用户仍可测通知链路）。
// 各 cate 的检测路径与 alert/eval/eval.go Eval() 的分发保持一致：
// prometheus/loki -> PromClients，host -> 主机过滤器，其余 -> dscache 插件。

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/eval"
	"github.com/ccfos/nightingale/v6/dscache"
	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/hash"
	"github.com/ccfos/nightingale/v6/pkg/parser"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	promqlparser "github.com/prometheus/prometheus/promql/parser"
	"github.com/toolkits/pkg/logger"
)

const (
	// 单次检测的查询/判断条件数量上限：rule_config 完全来自请求体，
	// 防止超大配置把接口拖死（每条查询最多占用 testFireQueryTimeout）
	testFireMaxCheckQueries  = 10
	testFireMaxCheckTriggers = 10
	// 单条查询的执行超时
	testFireQueryTimeout = 10 * time.Second
	// 每个判断条件参与实算的曲线分组上限，超出只计数不再计算
	testFireMaxEvalGroups = 500
)

// testFireAutoSample 是 query_check 从真实查询结果里取出的样本，供合成事件使用。
// 仅非 prometheus 数据源使用：prometheus 的真实样本由前端选定（用户可显式选 mock），
// 其余 cate 前端没有采样交互，由后端自动取第一条命中序列。
type testFireAutoSample struct {
	labels     map[string]string
	value      float64
	valuesText string // 非空时作为事件 TriggerValues，与真实链路的 $A.metric:81.500 格式一致
}

// resolveTestFireDatasourceId 与引擎真实链路完全一致的数据源解析：只认 DatasourceQueries
//（引擎 syncAlertRules 只用它）。datasource_ids 是 Deprecated 的展示字段（DB2FE 反填，
// 编辑表单会原样带回，切换 cate 后残留旧数据源 id），这里明确不使用。
// DatasourceCache 单测环境下为空，做 nil 保护。
func (rt *Router) resolveTestFireDatasourceId(cfg *models.AlertRule) int64 {
	if len(cfg.DatasourceQueries) == 0 || rt.DatasourceCache == nil {
		return 0
	}
	if ids := rt.DatasourceCache.GetIDsByDsCateAndQueries(cfg.Cate, cfg.DatasourceQueries); len(ids) > 0 {
		return ids[0]
	}
	return 0
}

func (rt *Router) runTestFireQueryCheck(c *gin.Context, cfg *models.AlertRule, dsId int64) (testFireStage, *testFireAutoSample) {
	// 诊断日志：数据源解析是排障高频点，筛选值形态不对（精确匹配里存了字符串）、
	// 缓存未同步、cate 不匹配都会落到 dsId=0，这里把解析入参和结果留痕
	logger.Infof("test-fire query_check: cate=%s prod=%s resolved_ds_id=%d datasource_ids=%v datasource_queries=%+v",
		cfg.Cate, cfg.Prod, dsId, cfg.DatasourceIdsJson, cfg.DatasourceQueries)

	var stage testFireStage
	var sample *testFireAutoSample
	switch cfg.GetRuleType() {
	case models.HOST:
		stage, sample = rt.checkTestFireHostQueries(cfg)
	case models.PROMETHEUS:
		// v2（高级模式）：查询在 queries[].query、判断条件在 triggers[]（与 plus 引擎
		// GetPromV2AnomalyPoint 的形态一致），v1 的 PromRuleConfig 解析不适用
		if isPromV2RuleConfig(cfg.RuleConfig) {
			stage = rt.checkTestFirePromV2Queries(c, cfg, dsId)
		} else {
			stage, _ = rt.checkTestFirePromQueries(c, cfg, dsId, true)
		}
		// prometheus 的真实样本由前端选定（用户可能显式选择 mock），不自动取样
		sample = nil
	case models.LOKI:
		// loki 走 prom 兼容查询接口（与引擎 Eval 分发一致），但 LogQL 不是 PromQL，跳过语法检查
		stage, sample = rt.checkTestFirePromQueries(c, cfg, dsId, false)
	default:
		stage, sample = rt.checkTestFireGenericQueries(c, cfg, dsId)
	}

	// 引擎对筛选匹配到的每个数据源都会各自评估告警；测试只对第一个数据源发起检测，
	// 匹配多于一个时在报告中说明，避免读作「所有数据源都验证过了」
	if rt.DatasourceCache != nil && len(cfg.DatasourceQueries) > 0 && stage.Data != nil {
		if all := rt.DatasourceCache.GetIDsByDsCateAndQueries(cfg.Cate, cfg.DatasourceQueries); len(all) > 1 {
			stage.Data["matched_datasource_ids"] = all
			stage.Data["only_first_checked"] = true
		}
	}
	return stage, sample
}

// testFireDatasourceError 区分两类数据源不可用：筛选没匹配到实例（dsId=0，
// 常见于 datasource_queries 值形态不对或 cate 不匹配）vs 实例存在但 client 未初始化
func testFireDatasourceError(cate string, dsId int64) string {
	if dsId == 0 {
		return "no datasource matched by datasource filter, check datasource_queries"
	}
	return fmt.Sprintf("datasource client not initialized: cate=%s id=%d", cate, dsId)
}

// 数据源权限不通过时的单条报错。与 notify/pipeline 的越权 403 不同，这里不中断请求：
// 查询不发起本身就是完备防护（零数据外泄），标红后其余阶段照常，用户仍可测通知链路。
// plus 的 CheckDsPerm 对 es/sls/cls/tls/lts/doris 按数据源+index/topic 细粒度判权，
// 403 中断会把「筛选没匹配到数据源」等友好提示一并吞掉（dsId=0 时 plus 权限缓存必 miss）。
const testFireDsPermDenied = "forbidden: no permission to query this datasource"

// mergeTestFireStatus 阶段状态只升不降：pass < warn < fail
func mergeTestFireStatus(cur, next string) string {
	if cur == testFireStageFail || next == testFireStageFail {
		return testFireStageFail
	}
	if cur == testFireStageWarn || next == testFireStageWarn {
		return testFireStageWarn
	}
	return next
}

// checkTestFirePromQueries 检测 prometheus/loki 规则的查询条件。
// 与引擎 GetPromAnomalyPoint 同路径：PromClients.GetCli(dsId) 发起即时查询。
func (rt *Router) checkTestFirePromQueries(c *gin.Context, cfg *models.AlertRule, dsId int64, syntaxCheck bool) (testFireStage, *testFireAutoSample) {
	var rc models.PromRuleConfig
	if err := json.Unmarshal([]byte(cfg.RuleConfig), &rc); err != nil {
		return testFireStage{Stage: "query_check", Status: testFireStageFail, Data: gin.H{
			"datasource_id": dsId, "error": "rule_config parse error: " + err.Error(),
		}}, nil
	}
	if len(rc.Queries) == 0 {
		return testFireStage{Stage: "query_check", Status: testFireStageWarn, Data: gin.H{
			"datasource_id": dsId, "queries": []gin.H{}, "error": "no queries",
		}}, nil
	}

	status := testFireStagePass
	results := make([]gin.H, 0, len(rc.Queries))
	var sample *testFireAutoSample

	for i, q := range rc.Queries {
		if i >= testFireMaxCheckQueries {
			results = append(results, gin.H{"skipped": true, "reason": "too_many_queries"})
			status = mergeTestFireStatus(status, testFireStageWarn)
			break
		}

		promql := strings.TrimSpace(q.PromQl)
		item := gin.H{"promql": promql}
		if promql == "" {
			item["ok"] = false
			item["error"] = "promql is blank"
			status = testFireStageFail
			results = append(results, item)
			continue
		}

		if q.VarEnabled && strings.Contains(promql, "$") {
			// 变量查询：$xx 占位符不是合法 promql，且 VarFilling 依赖引擎上下文，跳过检测
			item["ok"] = true
			item["skipped_var"] = true
			status = mergeTestFireStatus(status, testFireStageWarn)
			results = append(results, item)
			continue
		}

		if syntaxCheck {
			if _, err := promqlparser.ParseExpr(promql); err != nil {
				item["ok"] = false
				item["syntax_error"] = err.Error()
				status = testFireStageFail
				results = append(results, item)
				continue
			}
		}

		// 顺序不可变：先报「筛选没匹配到数据源」，再做权限门（只标红不中断），最后 client 存在性
		if dsId == 0 {
			item["ok"] = false
			item["error"] = testFireDatasourceError(cfg.Cate, dsId)
			status = testFireStageFail
			results = append(results, item)
			continue
		}
		if !CheckDsPerm(c, dsId, cfg.Cate, q) {
			item["ok"] = false
			item["error"] = testFireDsPermDenied
			status = testFireStageFail
			results = append(results, item)
			continue
		}

		var cli promsdk.API
		if rt.PromClients != nil {
			cli = rt.PromClients.GetCli(dsId)
		}
		if cli == nil {
			item["ok"] = false
			item["error"] = testFireDatasourceError(cfg.Cate, dsId)
			status = testFireStageFail
			results = append(results, item)
			continue
		}

		qctx, cancel := context.WithTimeout(c.Request.Context(), testFireQueryTimeout)
		value, _, err := cli.Query(qctx, promql, time.Now())
		cancel()
		if err != nil {
			item["ok"] = false
			item["error"] = redactSensitive(err.Error())
			status = testFireStageFail
			results = append(results, item)
			continue
		}

		item["ok"] = true
		n, first := promSeriesCountAndFirst(value)
		item["series_count"] = n
		if n == 0 {
			// 查询有效但当前无数据：无法判断会不会触发，警示而非失败
			status = mergeTestFireStatus(status, testFireStageWarn)
		} else if first != nil {
			item["latest_value"] = first.value
			if sample == nil {
				sample = first
			}
		}
		results = append(results, item)
	}

	return testFireStage{Stage: "query_check", Status: status, Data: gin.H{
		"datasource_id": dsId,
		"queries":       results,
	}}, sample
}

// isPromV2RuleConfig 判定 prometheus 高级模式规则（与 plus 引擎 IsPromV2 同判定）：
// rule_config.version == "v2"，查询与判断条件分离
func isPromV2RuleConfig(ruleConfig string) bool {
	var ruleQuery models.RuleQuery
	if err := json.Unmarshal([]byte(ruleConfig), &ruleQuery); err != nil {
		return false
	}
	return ruleQuery.Version == "v2"
}

// checkTestFirePromV2Queries 检测 prometheus 高级模式规则：
// 逐条 queries[].query 做 PromQL 语法检查 + PromClients 实查，查询结果按引擎同款
// 结构建立索引（对齐 plus GetPromV2AnomalyPoint），判断条件复用三层检测。
func (rt *Router) checkTestFirePromV2Queries(c *gin.Context, cfg *models.AlertRule, dsId int64) testFireStage {
	var ruleQuery models.RuleQuery
	if err := json.Unmarshal([]byte(cfg.RuleConfig), &ruleQuery); err != nil {
		return testFireStage{Stage: "query_check", Status: testFireStageFail, Data: gin.H{
			"datasource_id": dsId, "error": "rule_config parse error: " + err.Error(),
		}}
	}
	if len(ruleQuery.Queries) == 0 {
		return testFireStage{Stage: "query_check", Status: testFireStageWarn, Data: gin.H{
			"datasource_id": dsId, "queries": []gin.H{}, "error": "no queries",
		}}
	}

	status := testFireStagePass
	queryResults := make([]gin.H, 0, len(ruleQuery.Queries))
	refs := make(map[string]struct{})
	seriesStore := make(map[uint64]models.DataResp)
	seriesTagIndexes := make(map[string]map[uint64][]uint64)

	var cli promsdk.API
	if rt.PromClients != nil {
		cli = rt.PromClients.GetCli(dsId)
	}

	for i, query := range ruleQuery.Queries {
		if i >= testFireMaxCheckQueries {
			queryResults = append(queryResults, gin.H{"skipped": true, "reason": "too_many_queries"})
			status = mergeTestFireStatus(status, testFireStageWarn)
			break
		}

		// v2 query 是 {ref, query, unit, ...} 形态的 map，round-trip 到结构体取字段
		var q struct {
			Ref   string `json:"ref"`
			Query string `json:"query"`
		}
		if bs, err := json.Marshal(query); err == nil {
			json.Unmarshal(bs, &q)
		}
		if q.Ref != "" {
			refs[q.Ref] = struct{}{}
		}
		promql := strings.TrimSpace(q.Query)
		item := gin.H{"ref": q.Ref, "promql": promql}
		if promql == "" {
			item["ok"] = false
			item["error"] = "query is blank"
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		if _, err := promqlparser.ParseExpr(promql); err != nil {
			item["ok"] = false
			item["syntax_error"] = err.Error()
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		// 顺序不可变：先报「筛选没匹配到数据源」，再做权限门（只标红不中断），最后 client 存在性
		if dsId == 0 {
			item["ok"] = false
			item["error"] = testFireDatasourceError(cfg.Cate, dsId)
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}
		if !CheckDsPerm(c, dsId, cfg.Cate, query) {
			item["ok"] = false
			item["error"] = testFireDsPermDenied
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		if cli == nil {
			item["ok"] = false
			item["error"] = testFireDatasourceError(cfg.Cate, dsId)
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		qctx, cancel := context.WithTimeout(c.Request.Context(), testFireQueryTimeout)
		value, _, err := cli.Query(qctx, promql, time.Now())
		cancel()
		if err != nil {
			item["ok"] = false
			item["error"] = redactSensitive(err.Error())
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		series := promValueToDataResps(value, q.Ref, promql)
		item["ok"] = true
		item["series_count"] = len(series)
		if len(series) == 0 {
			status = mergeTestFireStatus(status, testFireStageWarn)
		} else if _, v, ok := series[0].Last(); ok {
			item["latest_value"] = v
		}
		queryResults = append(queryResults, item)

		// 按相同 tag 分组建立索引（plus GetPromV2AnomalyPoint 同款）
		seriesTagIndex := make(map[uint64][]uint64)
		for j := 0; j < len(series); j++ {
			seriesHash := hash.GetHash(series[j].Metric, series[j].Ref)
			tagHash := hash.GetTagHash(series[j].Metric)
			seriesStore[seriesHash] = series[j]
			seriesTagIndex[tagHash] = append(seriesTagIndex[tagHash], seriesHash)
		}
		if q.Ref != "" {
			seriesTagIndexes[q.Ref] = seriesTagIndex
		}
	}

	triggerResults, triggerStatus, _ := checkTestFireTriggersLoop(ruleQuery, refs, seriesTagIndexes, seriesStore)
	status = mergeTestFireStatus(status, triggerStatus)

	return testFireStage{Stage: "query_check", Status: status, Data: gin.H{
		"datasource_id": dsId,
		"queries":       queryResults,
		"triggers":      triggerResults,
	}}
}

// promValueToDataResps 把即时查询结果转成引擎同款 DataResp 列表
//（对齐 plus obj.ConvertToTimeSeries：NaN/Inf 过滤，vector 每个样本一条）
func promValueToDataResps(value model.Value, ref, query string) []models.DataResp {
	lst := make([]models.DataResp, 0)
	vector, ok := value.(model.Vector)
	if !ok {
		return lst
	}
	for _, item := range vector {
		v := float64(item.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		lst = append(lst, models.DataResp{
			Ref:    ref,
			Query:  query,
			Metric: item.Metric,
			Values: [][]float64{{float64(item.Timestamp.Unix()), v}},
		})
	}
	return lst
}

// promSeriesCountAndFirst 统计即时查询结果的序列数并取第一条作为候选样本
func promSeriesCountAndFirst(value model.Value) (int, *testFireAutoSample) {
	switch v := value.(type) {
	case model.Vector:
		if len(v) == 0 {
			return 0, nil
		}
		labels := make(map[string]string, len(v[0].Metric))
		for k, lv := range v[0].Metric {
			labels[string(k)] = string(lv)
		}
		return len(v), &testFireAutoSample{labels: labels, value: float64(v[0].Value)}
	case model.Matrix:
		return len(v), nil
	case *model.Scalar:
		if v == nil {
			return 0, nil
		}
		return 1, &testFireAutoSample{labels: map[string]string{}, value: float64(v.Value)}
	default:
		return 0, nil
	}
}

// checkTestFireHostQueries 检测 host 规则：主机过滤器解析后统计命中主机数。
// host 的 triggers 是 target_miss 等枚举类型，无表达式可验。
func (rt *Router) checkTestFireHostQueries(cfg *models.AlertRule) (testFireStage, *testFireAutoSample) {
	var rc models.HostRuleConfig
	if err := json.Unmarshal([]byte(cfg.RuleConfig), &rc); err != nil {
		return testFireStage{Stage: "query_check", Status: testFireStageFail, Data: gin.H{
			"error": "rule_config parse error: " + err.Error(),
		}}, nil
	}
	if len(rc.Queries) == 0 {
		return testFireStage{Stage: "query_check", Status: testFireStageWarn, Data: gin.H{
			"host_count": 0, "error": "no queries",
		}}, nil
	}

	hostsQuery := models.GetHostsQuery(rc.Queries)
	var cnt int64
	if err := models.TargetFilterQueryBuild(rt.Ctx, hostsQuery, 0, 0).Count(&cnt).Error; err != nil {
		return testFireStage{Stage: "query_check", Status: testFireStageFail, Data: gin.H{
			"error": redactSensitive(err.Error()),
		}}, nil
	}

	status := testFireStagePass
	if cnt == 0 {
		status = testFireStageWarn
	}

	// 取一台真实命中的主机作为样本：通知模板常引用 $labels.ident
	var sample *testFireAutoSample
	if cnt > 0 {
		var idents []string
		if err := models.TargetFilterQueryBuild(rt.Ctx, hostsQuery, 1, 0).Pluck("ident", &idents).Error; err == nil && len(idents) > 0 {
			sample = &testFireAutoSample{labels: map[string]string{"ident": idents[0]}}
		}
	}

	return testFireStage{Stage: "query_check", Status: status, Data: gin.H{
		"host_count": cnt,
	}}, sample
}

// checkTestFireGenericQueries 检测通用 cate（ES/CK/MySQL/...）规则：
// 查询走引擎同款三步（ExecuteQueryTemplate -> DsCache -> QueryData），
// 判断条件做 引用/语法/实算 三层检测。逻辑锚定 eval.go GetAnomalyPoint。
func (rt *Router) checkTestFireGenericQueries(c *gin.Context, cfg *models.AlertRule, dsId int64) (testFireStage, *testFireAutoSample) {
	ruleConfig := strings.TrimSpace(cfg.RuleConfig)
	if ruleConfig == "" {
		return testFireStage{Stage: "query_check", Status: testFireStageWarn, Data: gin.H{
			"datasource_id": dsId, "error": "rule_config is blank",
		}}, nil
	}
	var ruleQuery models.RuleQuery
	if err := json.Unmarshal([]byte(ruleConfig), &ruleQuery); err != nil {
		return testFireStage{Stage: "query_check", Status: testFireStageFail, Data: gin.H{
			"datasource_id": dsId, "error": "rule_config parse error: " + err.Error(),
		}}, nil
	}
	if len(ruleQuery.Queries) == 0 {
		return testFireStage{Stage: "query_check", Status: testFireStageWarn, Data: gin.H{
			"datasource_id": dsId, "queries": []gin.H{}, "error": "no queries",
		}}, nil
	}

	status := testFireStagePass
	queryResults := make([]gin.H, 0, len(ruleQuery.Queries))
	refs := make(map[string]struct{})
	// 与引擎一致的索引结构，供判断条件实算（eval.go GetAnomalyPoint 同款）
	seriesStore := make(map[uint64]models.DataResp)
	seriesTagIndexes := make(map[string]map[uint64][]uint64)

	for i, query := range ruleQuery.Queries {
		if i >= testFireMaxCheckQueries {
			queryResults = append(queryResults, gin.H{"skipped": true, "reason": "too_many_queries"})
			status = mergeTestFireStatus(status, testFireStageWarn)
			break
		}

		ref, _ := eval.GetQueryRef(query)
		if ref != "" {
			refs[ref] = struct{}{}
		}
		item := gin.H{"ref": ref}

		// 顺序不可变：先报「筛选没匹配到数据源」（dsId=0 时 plus 的细粒度权限判定必失败，
		// 会把这个排障提示吞掉），再做权限门，最后才是 client 存在性
		if dsId == 0 {
			item["ok"] = false
			item["error"] = testFireDatasourceError(cfg.Cate, dsId)
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		// 与 /ds-query 相同的数据源权限门，不通过只标红该条、不发起查询、不中断报告
		if !CheckDsPerm(c, dsId, cfg.Cate, query) {
			item["ok"] = false
			item["error"] = testFireDsPermDenied
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		plug, exists := dscache.DsCache.Get(cfg.Cate, dsId)
		if !exists {
			item["ok"] = false
			item["error"] = testFireDatasourceError(cfg.Cate, dsId)
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		if err := eval.ExecuteQueryTemplate(cfg.Cate, query, nil); err != nil {
			item["ok"] = false
			item["error"] = redactSensitive(err.Error())
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		qctx, cancel := context.WithTimeout(c.Request.Context(), testFireQueryTimeout)
		qctx = context.WithValue(qctx, "delay", int64(cfg.Delay))
		qctx = dskittypes.WithCallContext(qctx, dskittypes.CallContext{
			DatasourceID: dsId,
			Operator:     "alert_rule_test_fire",
			RuleID:       cfg.Id,
		})
		series, err := plug.QueryData(qctx, query)
		cancel()
		if err != nil {
			item["ok"] = false
			item["error"] = redactSensitive(err.Error())
			status = testFireStageFail
			queryResults = append(queryResults, item)
			continue
		}

		item["ok"] = true
		item["series_count"] = len(series)
		if len(series) == 0 {
			// 查询有效但当前无数据：无法实算判断条件，警示而非失败
			status = mergeTestFireStatus(status, testFireStageWarn)
		} else if _, v, ok := series[0].Last(); ok {
			item["latest_value"] = v
		}
		queryResults = append(queryResults, item)

		// 按相同 tag 分组建立索引（eval.go GetAnomalyPoint 同款）
		seriesTagIndex := make(map[uint64][]uint64)
		for j := 0; j < len(series); j++ {
			seriesHash := hash.GetHash(series[j].Metric, series[j].Ref)
			tagHash := hash.GetTagHash(series[j].Metric)
			seriesStore[seriesHash] = series[j]
			seriesTagIndex[tagHash] = append(seriesTagIndex[tagHash], seriesHash)
		}
		if ref != "" {
			seriesTagIndexes[ref] = seriesTagIndex
		}
	}

	triggerResults, triggerStatus, sample := checkTestFireTriggersLoop(ruleQuery, refs, seriesTagIndexes, seriesStore)
	status = mergeTestFireStatus(status, triggerStatus)

	return testFireStage{Stage: "query_check", Status: status, Data: gin.H{
		"datasource_id": dsId,
		"queries":       queryResults,
		"triggers":      triggerResults,
	}}, sample
}

// checkTestFireTriggersLoop 逐条检测判断条件（带数量上限），返回报告项、聚合状态与候选样本。
// ExpTriggerDisable 时引擎不执行表达式判断（如 nodata-only 规则），返回空列表。
func checkTestFireTriggersLoop(ruleQuery models.RuleQuery, refs map[string]struct{},
	seriesTagIndexes map[string]map[uint64][]uint64, seriesStore map[uint64]models.DataResp) ([]gin.H, string, *testFireAutoSample) {

	triggerResults := make([]gin.H, 0, len(ruleQuery.Triggers))
	status := testFireStagePass
	var sample *testFireAutoSample
	sampleFromFired := false
	if !ruleQuery.ExpTriggerDisable {
		for i, trigger := range ruleQuery.Triggers {
			if i >= testFireMaxCheckTriggers {
				triggerResults = append(triggerResults, gin.H{"skipped": true, "reason": "too_many_triggers"})
				status = mergeTestFireStatus(status, testFireStageWarn)
				break
			}
			item, itemStatus, trSample, trFired := checkTestFireTrigger(trigger, refs, seriesTagIndexes, seriesStore)
			status = mergeTestFireStatus(status, itemStatus)
			triggerResults = append(triggerResults, item)
			// 优先用命中判断条件的分组做样本：其标签最接近真实告警事件
			if trSample != nil && (sample == nil || (trFired && !sampleFromFired)) {
				sample = trSample
				sampleFromFired = trFired
			}
		}
	}
	return triggerResults, status, sample
}

// testFireExpRefPattern 提取表达式中 $A / $A.label 形式引用的查询 ref（取 $ 后到点号前的标识符）
var testFireExpRefPattern = regexp.MustCompile(`\$([A-Za-z][A-Za-z0-9_]*)`)

// missingExpRefs 返回表达式引用了、但查询里不存在的 ref
func missingExpRefs(exp string, refs map[string]struct{}) []string {
	missing := []string{}
	seen := make(map[string]struct{})
	for _, mt := range testFireExpRefPattern.FindAllStringSubmatch(exp, -1) {
		ref := mt[1]
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		if _, ok := refs[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	sort.Strings(missing)
	return missing
}

// checkTestFireTrigger 对单个判断条件做三层检测：
//  1. 引用检查：exp 与 joins 引用的 ref 必须存在于查询的 ref 集合
//  2. 语法检查：expr 编译（parser.ValidateExp），能抓语法错误
//  3. 实算：用真实查询结果按引擎同款分组逐组计算，报告是否命中与现场变量值
//
// 返回 报告项、该项状态、候选样本、样本是否来自命中分组。
func checkTestFireTrigger(trigger models.Trigger, refs map[string]struct{},
	seriesTagIndexes map[string]map[uint64][]uint64, seriesStore map[uint64]models.DataResp) (gin.H, string, *testFireAutoSample, bool) {

	exp := strings.TrimSpace(trigger.Exp)
	item := gin.H{"exp": exp}

	if exp == "" {
		item["ok"] = false
		item["error"] = "exp is blank"
		return item, testFireStageFail, nil, false
	}

	// 1. 引用检查（查询里一个 ref 都解析不出来时跳过，避免误报）
	if len(refs) > 0 {
		if missing := missingExpRefs(exp, refs); len(missing) > 0 {
			item["ok"] = false
			item["missing_refs"] = missing
			return item, testFireStageFail, nil, false
		}
		if len(trigger.Joins) > 0 {
			joinMissing := []string{}
			if trigger.JoinRef != "" {
				if _, ok := refs[trigger.JoinRef]; !ok {
					joinMissing = append(joinMissing, trigger.JoinRef)
				}
			}
			for _, j := range trigger.Joins {
				if j.Ref != "" {
					if _, ok := refs[j.Ref]; !ok {
						joinMissing = append(joinMissing, j.Ref)
					}
				}
			}
			if len(joinMissing) > 0 {
				item["ok"] = false
				item["missing_join_refs"] = joinMissing
				return item, testFireStageFail, nil, false
			}
		}
	}

	// 2. 语法检查
	if err := parser.ValidateExp(exp); err != nil {
		item["ok"] = false
		item["syntax_error"] = err.Error()
		return item, testFireStageFail, nil, false
	}
	if trigger.RecoverConfig.JudgeType == models.RecoverOnCondition {
		if recoverExp := strings.TrimSpace(trigger.RecoverConfig.RecoverExp); recoverExp != "" {
			if err := parser.ValidateExp(recoverExp); err != nil {
				item["ok"] = false
				item["recover_exp_error"] = err.Error()
				return item, testFireStageFail, nil, false
			}
		}
	}

	// 3. 实算：分组与变量注入逻辑锚定 eval.go GetAnomalyPoint（ProcessJoins + m 构建）
	groups := eval.ProcessJoins(0, trigger, seriesTagIndexes, seriesStore)
	item["ok"] = true
	item["groups"] = len(groups)
	if len(groups) == 0 {
		// 语法/引用都过了但没有数据可实算
		item["no_data"] = true
		return item, testFireStageWarn, nil, false
	}

	// map 遍历无序，按 key 排序保证报告与样本选取稳定
	groupKeys := make([]uint64, 0, len(groups))
	for k := range groups {
		groupKeys = append(groupKeys, k)
	}
	sort.Slice(groupKeys, func(i, j int) bool { return groupKeys[i] < groupKeys[j] })

	evaluated, fired, truncated := 0, 0, false
	var evalErr string
	var sample *testFireAutoSample
	var sampleVars map[string]interface{}
	sampleFromFired := false
	for _, gk := range groupKeys {
		if evaluated >= testFireMaxEvalGroups {
			truncated = true
			break
		}
		evaluated++
		m, lastSeries := buildTestFireTriggerVars(exp, groups[gk], seriesStore)
		v, err := parser.MathCalc(exp, m)
		if err != nil {
			// 真实链路里该分组会静默判为 false（仅记日志），这里把首个错误暴露出来：
			// 常见于类型不匹配（标签值是字符串却与数字比较）或分组缺某个 ref 的数据
			if evalErr == "" {
				evalErr = err.Error()
			}
			continue
		}
		isFired := v > 0
		if isFired {
			fired++
		}
		if lastSeries != nil && (sample == nil || (isFired && !sampleFromFired)) {
			sample = makeTestFireSampleFromSeries(lastSeries, m)
			sampleVars = m
			sampleFromFired = isFired
		}
	}
	item["fired_groups"] = fired
	if truncated {
		item["groups_truncated"] = true
	}
	if sampleVars != nil {
		item["sample_vars"] = sampleVars
	}
	if evalErr != "" {
		item["eval_error"] = evalErr
		return item, testFireStageWarn, sample, sampleFromFired
	}
	return item, testFireStagePass, sample, sampleFromFired
}

// buildTestFireTriggerVars 按引擎同款逻辑（eval.go GetAnomalyPoint 的 m 构建）
// 把分组内每条曲线的最新值与标签注入表达式变量，返回变量表和最后一条参与注入的曲线。
func buildTestFireTriggerVars(exp string, group []uint64, seriesStore map[uint64]models.DataResp) (map[string]interface{}, *models.DataResp) {
	sorted := append([]uint64(nil), group...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	m := make(map[string]interface{})
	var last *models.DataResp
	for _, h := range sorted {
		series, exists := seriesStore[h]
		if !exists {
			continue
		}
		_, v, exists := series.Last()
		if !exists {
			continue
		}
		if !strings.Contains(exp, "$"+series.Ref) {
			// 表达式中不包含该变量
			continue
		}
		m["$"+series.Ref] = v
		m["$"+series.Ref+"."+series.MetricName()] = v
		for k, lv := range series.Metric {
			if k == "__name__" {
				continue
			}
			if !strings.Contains(exp, "$"+series.Ref+"."+string(k)) {
				continue
			}
			m["$"+series.Ref+"."+string(k)] = string(lv)
		}
		s := series
		last = &s
	}
	return m, last
}

// makeTestFireSampleFromSeries 从曲线和变量表构造合成事件用的样本。
// valuesText 与引擎生成 AnomalyPoint.Values 的格式一致（$A.metric:81.500 ），
// 差异：引擎在配置了 unit 时会做单位格式化，这里统一按原始数值输出。
func makeTestFireSampleFromSeries(series *models.DataResp, m map[string]interface{}) *testFireAutoSample {
	labels := make(map[string]string, len(series.Metric))
	for k, v := range series.Metric {
		labels[string(k)] = string(v)
	}
	_, value, _ := series.Last()

	var values string
	for k, v := range m {
		if !strings.Contains(k, ".") {
			continue
		}
		switch vv := v.(type) {
		case float64:
			values += fmt.Sprintf("%s:%.3f ", k, vv)
		case string:
			values += fmt.Sprintf("%s:%s ", k, vv)
		}
	}

	return &testFireAutoSample{
		labels:     labels,
		value:      value,
		valuesText: strings.TrimSpace(values),
	}
}
