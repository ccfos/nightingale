package router

import (
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/hash"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
)

// 数据源解析只认 datasource_queries（与引擎一致）；datasource_ids 是 Deprecated 的
// 展示字段（编辑表单会携带旧数据源 id 残留），必须被忽略
func TestResolveTestFireDatasourceId_IgnoresDeprecatedIds(t *testing.T) {
	rt := &Router{} // DatasourceCache 为空

	// 只有 datasource_ids：不使用，解析为空
	cfg := &models.AlertRule{Cate: "mysql", DatasourceIdsJson: []int64{3}}
	if got := rt.resolveTestFireDatasourceIds(cfg); len(got) != 0 {
		t.Fatalf("datasource_ids alone should be ignored, got %v", got)
	}

	// datasource_ids 残留 + datasource_queries 并存：ids 同样不参与
	//（单测环境无 DatasourceCache，queries 解析不出 → 空，绝不能回退到 ids 的 3）
	cfg = &models.AlertRule{
		Cate:              "mysql",
		DatasourceIdsJson: []int64{3},
		DatasourceQueries: []models.DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{float64(4)}}},
	}
	if got := rt.resolveTestFireDatasourceIds(cfg); len(got) != 0 {
		t.Fatalf("stale datasource_ids must never override queries resolution, got %v", got)
	}
}

// 多数据源规则的解析结果必须稳定有序：底层是 map 收集，不排序会让每次模拟触发
// 检测到不同的数据源，报告里的「只检测了第一个」也会指向另一个数据源
func TestResolveTestFireDatasourceIds_StableOrder(t *testing.T) {
	rt := &Router{DatasourceCache: &memsto.DatasourceCacheType{
		CateToIDs: map[string]map[int64]*models.Datasource{
			"mysql": {7: {}, 3: {}, 12: {}, 5: {}},
		},
		CateToNames: map[string]map[string]int64{
			"mysql": {"db-a": 3, "db-b": 5, "db-c": 7, "db-d": 12},
		},
	}}
	cfg := &models.AlertRule{
		Cate:              "mysql",
		DatasourceQueries: []models.DatasourceQuery{models.DataSourceQueryAll},
	}

	want := []int64{3, 5, 7, 12}
	// map 遍历顺序每次不同，多跑几轮才能暴露未排序的实现
	for i := 0; i < 20; i++ {
		got := rt.resolveTestFireDatasourceIds(cfg)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round %d: got %v, want %v", i, got, want)
		}
		if firstTestFireDatasourceId(got) != want[0] {
			t.Fatalf("round %d: first id got %d, want %d", i, firstTestFireDatasourceId(got), want[0])
		}
	}

	if got := firstTestFireDatasourceId(nil); got != 0 {
		t.Fatalf("no matched datasource should resolve to 0, got %d", got)
	}
}

func TestMissingExpRefs(t *testing.T) {
	refs := map[string]struct{}{"A": {}, "B": {}}
	cases := []struct {
		exp  string
		want []string
	}{
		{"$A > 80", nil},
		{"$A > 80 && $B < 10", nil},
		{"$A.ident == 'web-01'", nil},
		{"$C > 0", []string{"C"}},
		{"$A > 0 || $C > 0 || $D > 0", []string{"C", "D"}},
		{"between($A, [1,2])", nil},
	}
	for _, c := range cases {
		got := missingExpRefs(c.exp, refs)
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", append([]string{}, c.want...)) {
			t.Fatalf("missingExpRefs(%q): got %v, want %v", c.exp, got, c.want)
		}
	}
}

// fabricateSeries 构造单条曲线的引擎同款索引结构
func fabricateSeries(ref string, metric model.Metric, value float64) (map[string]map[uint64][]uint64, map[uint64]models.DataResp) {
	s := models.DataResp{Ref: ref, Metric: metric, Values: [][]float64{{1000, value}}}
	sHash := hash.GetHash(s.Metric, ref)
	tagHash := hash.GetTagHash(s.Metric)
	return map[string]map[uint64][]uint64{ref: {tagHash: {sHash}}},
		map[uint64]models.DataResp{sHash: s}
}

// 判断条件三层检测：引用缺失 / 语法错误 / 恢复条件错误 都应 fail
func TestCheckTestFireTrigger_StaticChecks(t *testing.T) {
	refs := map[string]struct{}{"A": {}}
	empty := map[string]map[uint64][]uint64{}
	emptyStore := map[uint64]models.DataResp{}

	// 引用了不存在的查询 ref
	item, status, _, _ := checkTestFireTrigger(models.Trigger{Exp: "$B > 0"}, refs, empty, emptyStore)
	if status != testFireStageFail || fmt.Sprintf("%v", item["missing_refs"]) != "[B]" {
		t.Fatalf("missing ref should fail, got status=%s item=%+v", status, item)
	}

	// join 引用了不存在的 ref
	item, status, _, _ = checkTestFireTrigger(models.Trigger{
		Exp: "$A > 0", JoinRef: "A",
		Joins: []models.Join{{JoinType: "inner_join", Ref: "X", On: []string{"ident"}}},
	}, refs, empty, emptyStore)
	if status != testFireStageFail || item["missing_join_refs"] == nil {
		t.Fatalf("missing join ref should fail, got status=%s item=%+v", status, item)
	}

	// 语法错误
	item, status, _, _ = checkTestFireTrigger(models.Trigger{Exp: "$A > "}, refs, empty, emptyStore)
	if status != testFireStageFail || item["syntax_error"] == nil {
		t.Fatalf("syntax error should fail, got status=%s item=%+v", status, item)
	}

	// 空表达式
	_, status, _, _ = checkTestFireTrigger(models.Trigger{Exp: "  "}, refs, empty, emptyStore)
	if status != testFireStageFail {
		t.Fatalf("blank exp should fail, got %s", status)
	}

	// 恢复条件语法错误
	item, status, _, _ = checkTestFireTrigger(models.Trigger{
		Exp:           "$A > 0",
		RecoverConfig: models.RecoverConfig{JudgeType: models.RecoverOnCondition, RecoverExp: "$A <"},
	}, refs, empty, emptyStore)
	if status != testFireStageFail || item["recover_exp_error"] == nil {
		t.Fatalf("recover exp error should fail, got status=%s item=%+v", status, item)
	}

	// 语法/引用都对但无数据可实算：warn + no_data
	item, status, _, _ = checkTestFireTrigger(models.Trigger{Exp: "$A > 0"}, refs, empty, emptyStore)
	if status != testFireStageWarn || item["no_data"] != true {
		t.Fatalf("no data should warn, got status=%s item=%+v", status, item)
	}
}

// 判断条件实算：用真实查询结果逐组计算，报告命中数与现场变量，并产出样本
func TestCheckTestFireTrigger_RealEval(t *testing.T) {
	refs := map[string]struct{}{"A": {}}
	indexes, store := fabricateSeries("A", model.Metric{"__name__": "mem_used", "ident": "web-01"}, 85)

	// 命中：85 > 80
	item, status, sample, fired := checkTestFireTrigger(models.Trigger{Exp: "$A > 80"}, refs, indexes, store)
	if status != testFireStagePass || item["fired_groups"] != 1 || !fired {
		t.Fatalf("should fire, got status=%s item=%+v fired=%v", status, item, fired)
	}
	if sample == nil || sample.labels["ident"] != "web-01" || sample.value != 85 {
		t.Fatalf("sample from fired group: got %+v", sample)
	}
	if !strings.Contains(sample.valuesText, "$A.mem_used:85.000") {
		t.Fatalf("valuesText should carry $A.mem_used, got %q", sample.valuesText)
	}

	// 不命中：仍返回样本供合成事件，fired=false
	item, status, sample, fired = checkTestFireTrigger(models.Trigger{Exp: "$A > 90"}, refs, indexes, store)
	if status != testFireStagePass || item["fired_groups"] != 0 || fired {
		t.Fatalf("should not fire, got status=%s item=%+v fired=%v", status, item, fired)
	}
	if sample == nil || sample.labels["ident"] != "web-01" {
		t.Fatalf("non-fired sample: got %+v", sample)
	}

	// 类型不匹配（标签值是字符串却与数字比较）：真实链路会静默判 false，这里应 warn 并暴露错误
	item, status, _, _ = checkTestFireTrigger(models.Trigger{Exp: "$A.ident > 10"}, refs, indexes, store)
	if status != testFireStageWarn || item["eval_error"] == nil {
		t.Fatalf("type mismatch should warn with eval_error, got status=%s item=%+v", status, item)
	}

	// 标签比较：字符串等值判断正常实算
	item, status, _, fired = checkTestFireTrigger(models.Trigger{Exp: "$A > 80 && $A.ident == 'web-01'"}, refs, indexes, store)
	if status != testFireStagePass || !fired {
		t.Fatalf("label comparison should fire, got status=%s item=%+v", status, item)
	}
}

// 通用 cate 端点级：数据源不存在时查询 fail、判断条件引用检查仍生效、链路继续走完
func TestAlertRuleTestFire_GenericQueryCheck(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:          "es query",
			Prod:          "metric",
			Cate:          "elasticsearch",
			NotifyVersion: 1,
			RuleConfigJson: map[string]interface{}{
				"queries": []interface{}{
					map[string]interface{}{"ref": "A", "index": "app-*"},
				},
				"triggers": []interface{}{
					map[string]interface{}{"exp": "$B > 0", "mode": 1, "severity": 2},
					map[string]interface{}{"exp": "$A > 0", "mode": 1, "severity": 2},
				},
			},
		},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	qc := stageByName(t, resp, "query_check")
	if qc.Status != "fail" {
		t.Fatalf("query_check should fail without datasource, got %+v", qc)
	}
	queries, _ := qc.Data["queries"].([]interface{})
	if len(queries) != 1 {
		t.Fatalf("queries: got %v", qc.Data["queries"])
	}
	q0 := queries[0].(map[string]interface{})
	if q0["ok"] != false || q0["error"] != "no datasource matched by datasource filter, check datasource_queries" {
		t.Fatalf("query item: got %+v", q0)
	}

	triggers, _ := qc.Data["triggers"].([]interface{})
	if len(triggers) != 2 {
		t.Fatalf("triggers: got %v", qc.Data["triggers"])
	}
	// $B 引用不存在：即使查询失败也能静态检出
	t0 := triggers[0].(map[string]interface{})
	if t0["ok"] != false || fmt.Sprintf("%v", t0["missing_refs"]) != "[B]" {
		t.Fatalf("trigger[0] should report missing ref B, got %+v", t0)
	}
	// $A 引用正确但无数据可实算
	t1 := triggers[1].(map[string]interface{})
	if t1["ok"] != true || t1["no_data"] != true {
		t.Fatalf("trigger[1] should report no_data, got %+v", t1)
	}

	// 检测失败不阻断：链路继续走完六段
	if len(resp.Dat.Stages) != 6 {
		t.Fatalf("chain should continue after query_check fail, stages: %d", len(resp.Dat.Stages))
	}
}

// prometheus v2（高级模式）端点级：查询在 queries[].query、判断条件在 triggers[]，
// 不能按 v1 的 prom_ql 解析（否则恒报 promql is blank）；判断条件三层检测照常生效
func TestAlertRuleTestFire_PromV2QueryCheck(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:          "cpu high v2",
			Prod:          "metric",
			Cate:          "prometheus",
			NotifyVersion: 1,
			RuleConfigJson: map[string]interface{}{
				"version": "v2",
				"queries": []interface{}{
					map[string]interface{}{"ref": "A", "query": "cpu_usage_active"},
					map[string]interface{}{"ref": "B", "query": "mem_used >"}, // 语法错误
					map[string]interface{}{"ref": "C", "query": ""},           // 空查询
				},
				"triggers": []interface{}{
					map[string]interface{}{"exp": "$A > 80", "mode": 1, "severity": 2},
					map[string]interface{}{"exp": "$X > 0", "mode": 1, "severity": 2}, // 引用不存在
				},
			},
		},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	qc := stageByName(t, resp, "query_check")
	if qc.Status != "fail" {
		t.Fatalf("query_check should fail, got %+v", qc)
	}
	queries, _ := qc.Data["queries"].([]interface{})
	if len(queries) != 3 {
		t.Fatalf("queries: got %v", qc.Data["queries"])
	}
	// A：语法正确，但单测环境无 PromClients 且数据源筛选为空
	q0 := queries[0].(map[string]interface{})
	if q0["ref"] != "A" || q0["ok"] != false || q0["error"] != "no datasource matched by datasource filter, check datasource_queries" {
		t.Fatalf("query[0]: got %+v", q0)
	}
	// B：v2 的 query 也做 PromQL 语法检查
	q1 := queries[1].(map[string]interface{})
	if q1["ok"] != false || q1["syntax_error"] == nil {
		t.Fatalf("query[1] should report syntax error, got %+v", q1)
	}
	// C：空查询明确报错，而不是 v1 式的 promql is blank 误报
	q2 := queries[2].(map[string]interface{})
	if q2["ok"] != false || q2["error"] != "query is blank" {
		t.Fatalf("query[2]: got %+v", q2)
	}

	triggers, _ := qc.Data["triggers"].([]interface{})
	if len(triggers) != 2 {
		t.Fatalf("triggers: got %v", qc.Data["triggers"])
	}
	t0 := triggers[0].(map[string]interface{})
	if t0["ok"] != true || t0["no_data"] != true {
		t.Fatalf("trigger[0] should be no_data warn, got %+v", t0)
	}
	t1 := triggers[1].(map[string]interface{})
	if fmt.Sprintf("%v", t1["missing_refs"]) != "[X]" {
		t.Fatalf("trigger[1] should report missing ref X, got %+v", t1)
	}
}

// prometheus 端点级：promql 语法错误在无数据源客户端时也能检出
func TestAlertRuleTestFire_PromQueryCheck(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:          "cpu high",
			Prod:          "metric",
			Cate:          "prometheus",
			NotifyVersion: 1,
			RuleConfigJson: map[string]interface{}{
				"queries": []interface{}{
					map[string]interface{}{"prom_ql": "cpu_usage_active >", "severity": 2}, // 语法错误
					map[string]interface{}{"prom_ql": "cpu_usage_active > 80", "severity": 2},
					map[string]interface{}{"prom_ql": "", "severity": 2}, // 空 promql
				},
			},
		},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	qc := stageByName(t, resp, "query_check")
	if qc.Status != "fail" {
		t.Fatalf("query_check should fail, got %+v", qc)
	}
	queries, _ := qc.Data["queries"].([]interface{})
	if len(queries) != 3 {
		t.Fatalf("queries: got %v", qc.Data["queries"])
	}
	q0 := queries[0].(map[string]interface{})
	if q0["ok"] != false || q0["syntax_error"] == nil {
		t.Fatalf("query[0] should report syntax error, got %+v", q0)
	}
	// 语法正确但单测环境没有 PromClients（数据源筛选也为空 → dsId=0）
	q1 := queries[1].(map[string]interface{})
	if q1["ok"] != false || q1["error"] != "no datasource matched by datasource filter, check datasource_queries" {
		t.Fatalf("query[1] should report datasource not matched, got %+v", q1)
	}
	q2 := queries[2].(map[string]interface{})
	if q2["ok"] != false || q2["error"] != "promql is blank" {
		t.Fatalf("query[2] should report blank promql, got %+v", q2)
	}
}

// host 端点级：统计命中主机数，并自动取一台真实主机 ident 作为样本
func TestAlertRuleTestFire_HostQueryCheck(t *testing.T) {
	rt, c, bgid := setupTestFire(t)
	if err := c.DB.AutoMigrate(&models.Target{}, &models.TargetBusiGroup{}); err != nil {
		t.Fatalf("migrate target: %v", err)
	}
	if err := c.DB.Create(&models.Target{Ident: "web-01"}).Error; err != nil {
		t.Fatalf("seed target: %v", err)
	}

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:          "host down",
			Prod:          "host",
			Cate:          "host",
			NotifyVersion: 1,
			RuleConfigJson: map[string]interface{}{
				"queries": []interface{}{
					map[string]interface{}{"key": "hosts", "op": "==", "values": []interface{}{"web-01", "web-02"}},
				},
				"triggers": []interface{}{
					map[string]interface{}{"type": "target_miss", "severity": 2, "duration": 60},
				},
			},
		},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	qc := stageByName(t, resp, "query_check")
	if qc.Status != "pass" || qc.Data["host_count"] != float64(1) {
		t.Fatalf("host query_check: got %+v", qc)
	}

	// 命中主机被自动取样进合成事件
	if got := stageByName(t, resp, "synthesize").Data["sample_source"]; got != "real_auto" {
		t.Fatalf("sample_source: got %v, want real_auto", got)
	}
	tags := fmt.Sprintf("%v", resp.Dat.Event["tags"])
	if !strings.Contains(tags, "ident=web-01") {
		t.Fatalf("tags should carry real ident, got %v", tags)
	}
}

// host 无命中主机：warn
func TestAlertRuleTestFire_HostQueryCheckNoHost(t *testing.T) {
	rt, c, bgid := setupTestFire(t)
	if err := c.DB.AutoMigrate(&models.Target{}, &models.TargetBusiGroup{}); err != nil {
		t.Fatalf("migrate target: %v", err)
	}

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:          "host down",
			Prod:          "host",
			Cate:          "host",
			NotifyVersion: 1,
			RuleConfigJson: map[string]interface{}{
				"queries": []interface{}{
					map[string]interface{}{"key": "hosts", "op": "==", "values": []interface{}{"nonexistent"}},
				},
			},
		},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	qc := stageByName(t, resp, "query_check")
	if qc.Status != "warn" || qc.Data["host_count"] != float64(0) {
		t.Fatalf("empty host match should warn, got %+v", qc)
	}
	if got := stageByName(t, resp, "synthesize").Data["sample_source"]; got != "mock" {
		t.Fatalf("sample_source should fall back to mock, got %v", got)
	}
}

// 通知阶段遍历事件上的 NotifyRuleIds（与引擎一致，pipeline 可修改路由）：
// pipeline 动态加入、未经归属校验的 id 不代发，报告 added_by_pipeline
func TestRunTestFireNotify_PipelineAddedIdsNotSent(t *testing.T) {
	rt, _, _ := setupTestFire(t)

	cfg := &models.AlertRule{NotifyVersion: 1, NotifyRuleIds: []int64{1}}
	event := &models.AlertCurEvent{NotifyRuleIds: []int64{1, 99}} // 99 相当于 pipeline 动态加入
	notifyRuleMap := map[int64]*models.NotifyRule{1: {ID: 1, Name: "nr1", Enable: true}}

	stage := rt.runTestFireNotify(cfg, event, true, "tester", notifyRuleMap)
	results, _ := stage.Data["results"].([]gin.H)

	var found bool
	for _, item := range results {
		if item["notify_rule_id"] == int64(99) {
			found = true
			if item["added_by_pipeline"] != true {
				t.Fatalf("id 99 should be reported as added_by_pipeline, got %+v", item)
			}
			if item["sent"] == true {
				t.Fatalf("unauthorized pipeline-added id must not be sent, got %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("pipeline-added id 99 missing from results: %+v", results)
	}
	// 授权集合外的 id 不算失败，阶段不应 fail
	if stage.Status == testFireStageFail {
		t.Fatalf("stage should not fail, got %s", stage.Status)
	}
}

// 屏蔽匹配报告 mute_type：完全屏蔽置 full_mute_hit，仅屏蔽通知不置
func TestAlertRuleTestFire_MuteTypeReported(t *testing.T) {
	rt, c, bgid := setupTestFire(t)

	now := time.Now().Unix()
	seed := func(muteType int, note string) {
		m := &models.AlertMute{
			GroupId:      bgid,
			Note:         note,
			Btime:        now - 3600,
			Etime:        now + 3600,
			MuteTimeType: models.TimeRange,
			Tags:         []byte("[]"), // 空过滤条件匹配所有事件
			MuteType:     muteType,
		}
		if err := c.DB.Create(m).Error; err != nil {
			t.Fatalf("seed mute: %v", err)
		}
	}
	seed(models.MuteTypeNotifyOnly, "notify-only")
	seed(models.MuteTypeAll, "full")

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config:   models.AlertRule{Name: "cpu high", NotifyVersion: 1},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	muteStage := stageByName(t, resp, "mute")
	if muteStage.Status != "warn" {
		t.Fatalf("mute stage should warn, got %+v", muteStage)
	}
	if muteStage.Data["full_mute_hit"] != true {
		t.Fatalf("full_mute_hit should be true, got %+v", muteStage.Data)
	}
	matched, _ := muteStage.Data["matched_mutes"].([]interface{})
	if len(matched) != 2 {
		t.Fatalf("both mutes should match, got %+v", muteStage.Data)
	}
	types := map[float64]bool{}
	for _, m := range matched {
		mm := m.(map[string]interface{})
		types[mm["mute_type"].(float64)] = true
	}
	if !types[0] || !types[1] {
		t.Fatalf("matched_mutes should carry both mute types, got %+v", matched)
	}
}

// 数据源权限门（plus 会注入细粒度实现）：不通过只标红该条、不中断报告。
// 开源默认 CheckDsPerm 恒 true 覆盖不到，这里注入假实现模拟 plus 拒绝。
func TestAlertRuleTestFire_DsPermDeniedMarksFailNotAbort(t *testing.T) {
	rt, _, bgid := setupTestFire(t)
	orig := CheckDsPerm
	CheckDsPerm = func(c *gin.Context, dsId int64, cate string, q interface{}) bool { return false }
	defer func() { CheckDsPerm = orig }()

	// 场景 1（端点级）：dsId=0 时「筛选没匹配到数据源」的提示必须先于权限门可达
	//（plus 的权限判定在 dsId=0 时必失败，若顺序颠倒该提示会被 403 吞掉），且六段链路完整
	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:          "es perm",
			Prod:          "metric",
			Cate:          "elasticsearch",
			NotifyVersion: 1,
			RuleConfigJson: map[string]interface{}{
				"queries":  []interface{}{map[string]interface{}{"ref": "A", "index": "app-*"}},
				"triggers": []interface{}{map[string]interface{}{"exp": "$A > 0", "mode": 1, "severity": 2}},
			},
		},
	})
	if panicked {
		t.Fatal("perm denial must not abort the whole request")
	}
	qc := stageByName(t, resp, "query_check")
	queries, _ := qc.Data["queries"].([]interface{})
	q0 := queries[0].(map[string]interface{})
	if q0["error"] != "no datasource matched by datasource filter, check datasource_queries" {
		t.Fatalf("dsId=0 friendly error should win over perm check, got %+v", q0)
	}
	if len(resp.Dat.Stages) != 6 {
		t.Fatalf("chain should continue, stages: %d", len(resp.Dat.Stages))
	}

	// 场景 2（单元级）：dsId>0 且权限不通过 → 该条标红 forbidden，不 panic
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	cfg := &models.AlertRule{
		Cate:       "elasticsearch",
		RuleConfig: `{"queries":[{"ref":"A","index":"app-*"}],"triggers":[{"exp":"$A > 0","mode":1,"severity":2}]}`,
	}
	stage, _ := rt.checkTestFireGenericQueries(c, cfg, 5)
	if stage.Status != testFireStageFail {
		t.Fatalf("stage should fail, got %s", stage.Status)
	}
	items, _ := stage.Data["queries"].([]gin.H)
	if len(items) != 1 || items[0]["error"] != testFireDsPermDenied {
		t.Fatalf("query item should be marked forbidden, got %+v", stage.Data["queries"])
	}
	// 判断条件的静态检测不受权限影响，照常给出 no_data 报告
	trs, _ := stage.Data["triggers"].([]gin.H)
	if len(trs) != 1 || trs[0]["no_data"] != true {
		t.Fatalf("trigger static check should still run, got %+v", stage.Data["triggers"])
	}
}

// 判断条件数量超限：只检测前 N 条并警示
func TestAlertRuleTestFire_TooManyTriggers(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	triggers := make([]interface{}, 0, testFireMaxCheckTriggers+2)
	for i := 0; i < testFireMaxCheckTriggers+2; i++ {
		triggers = append(triggers, map[string]interface{}{"exp": "$A > 0", "mode": 1, "severity": 2})
	}
	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:          "r",
			Prod:          "metric",
			Cate:          "elasticsearch",
			NotifyVersion: 1,
			RuleConfigJson: map[string]interface{}{
				"queries":  []interface{}{map[string]interface{}{"ref": "A", "index": "app-*"}},
				"triggers": triggers,
			},
		},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	qc := stageByName(t, resp, "query_check")
	trs, _ := qc.Data["triggers"].([]interface{})
	if len(trs) != testFireMaxCheckTriggers+1 {
		t.Fatalf("triggers should be capped at %d + 1 skip marker, got %d", testFireMaxCheckTriggers, len(trs))
	}
	last := trs[len(trs)-1].(map[string]interface{})
	if last["skipped"] != true {
		t.Fatalf("last trigger item should be the skip marker, got %+v", last)
	}
}
