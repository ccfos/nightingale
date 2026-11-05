package eval

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/prometheus/common/model"
	promparser "github.com/prometheus/prometheus/promql/parser"
)

// fakePromAPI 返回固定的 cpu 序列；查询里已填好 ident 时只返回该主机的序列
type fakePromAPI struct {
	promsdk.API
	idents []string
	err    error
}

func (f *fakePromAPI) Query(_ context.Context, query string, ts time.Time) (model.Value, promsdk.Warnings, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	var vec model.Vector
	for _, ident := range f.idents {
		if strings.Contains(query, `ident="`) && !strings.Contains(query, `ident="`+ident+`"`) {
			continue
		}
		vec = append(vec, &model.Sample{
			Metric:    model.Metric{"__name__": "cpu_usage", "ident": model.LabelValue(ident)},
			Value:     5,
			Timestamp: model.TimeFromUnix(ts.Unix()),
		})
	}
	return vec, nil, nil
}

func newEdgeHostVarWorker(deviceErr error) *AlertRuleWorker {
	tc := &memsto.TargetCacheType{}
	tc.Set(map[string]*models.Target{
		"web-01": {Ident: "web-01", GroupIds: []int64{1}},
		"web-02": {Ident: "web-02", GroupIds: []int64{2}},
	}, 2, 1)

	return &AlertRuleWorker{
		DatasourceId: 1,
		Rule:         &models.AlertRule{Id: 1},
		// 不配置中心地址：若仍走 /v1/n9e/targets-of-host-query 会直接报错
		Ctx:       &ctx.Context{IsCenter: false},
		Processor: &process.Processor{Stats: getTestStats(), TargetCache: tc},
		DeviceIdentHook: func(*AlertRuleWorker, models.ParamQuery) ([]string, error) {
			return nil, deviceErr
		},
	}
}

func hostVarQuery(groupId float64, child *models.ChildVarConfig) models.PromQuery {
	return models.PromQuery{
		PromQl:     `cpu_usage{ident="$ident"} > 1`,
		VarEnabled: true,
		VarConfig: models.VarConfig{
			ParamVal: []models.ParamQueryForFirst{{
				Name:      "ident",
				ParamType: "host",
				Query:     []interface{}{map[string]interface{}{"key": "group_ids", "op": "==", "values": []interface{}{groupId}}},
			}},
			ChildVarConfigs: child,
		},
	}
}

func pointIdents(points []models.AnomalyPoint) []string {
	var res []string
	for _, p := range points {
		res = append(res, string(p.Labels["ident"]))
	}
	sort.Strings(res)
	return res
}

func TestEdgeHostVarUsesLocalTargetCache(t *testing.T) {
	arw := newEdgeHostVarWorker(nil)
	reader := &fakePromAPI{idents: []string{"web-01", "web-02", "db-01"}}

	for name, fill := range map[string]func(models.PromQuery, promsdk.API) ([]models.AnomalyPoint, error){
		"after":  arw.VarFillingAfterQuery,
		"before": arw.VarFillingBeforeQuery,
	} {
		t.Run(name, func(t *testing.T) {
			points, err := fill(hostVarQuery(1, nil), reader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := pointIdents(points); len(got) != 1 || got[0] != "web-01" {
				t.Fatalf("points = %v, want [web-01]", got)
			}
		})
	}
}

// 子筛选取值失败时必须中止本轮，不能留下上层筛选的异常点
func TestVarFillingAbortsOnParamFetchError(t *testing.T) {
	arw := newEdgeHostVarWorker(errors.New("center unreachable"))
	reader := &fakePromAPI{idents: []string{"web-01", "web-02"}}
	child := &models.ChildVarConfig{
		ParamVal: []map[string]models.ParamQuery{{
			"ident": {ParamType: "device", Query: []interface{}{}},
		}},
	}

	for name, fill := range map[string]func(models.PromQuery, promsdk.API) ([]models.AnomalyPoint, error){
		"after":  arw.VarFillingAfterQuery,
		"before": arw.VarFillingBeforeQuery,
	} {
		t.Run(name, func(t *testing.T) {
			points, err := fill(hostVarQuery(1, child), reader)
			if !errors.Is(err, errParamValueFetch) {
				t.Fatalf("err = %v, want errParamValueFetch", err)
			}
			if len(points) != 0 {
				t.Fatalf("points = %v, want none", pointIdents(points))
			}
		})
	}
}

// 筛选条件确实没有命中主机时保持原行为：不报错、无异常点，已有告警可以正常恢复
func TestVarFillingEmptyHostsIsNotError(t *testing.T) {
	arw := newEdgeHostVarWorker(nil)
	reader := &fakePromAPI{idents: []string{"web-01"}}

	points, err := arw.VarFillingAfterQuery(hostVarQuery(99, nil), reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 0 {
		t.Fatalf("points = %v, want none", pointIdents(points))
	}
}

// 数据源查询失败时必须中止本轮，不能当成"无数据"而误发恢复
func TestVarFillingAbortsOnDatasourceQueryError(t *testing.T) {
	arw := newEdgeHostVarWorker(nil)
	reader := &fakePromAPI{err: errors.New("datasource timeout")}

	for name, fill := range map[string]func(models.PromQuery, promsdk.API) ([]models.AnomalyPoint, error){
		"after":  arw.VarFillingAfterQuery,
		"before": arw.VarFillingBeforeQuery,
	} {
		t.Run(name, func(t *testing.T) {
			points, err := fill(hostVarQuery(1, nil), reader)
			if err == nil {
				t.Fatalf("expected error, got points %v", pointIdents(points))
			}
			if len(points) != 0 {
				t.Fatalf("points = %v, want none", pointIdents(points))
			}
		})
	}
}

// 枚举变量未填值时会查数据源取全部标签值，这次查询失败同样要中止
func TestEnumAllLabelQueryErrorIsFetchError(t *testing.T) {
	arw := newEdgeHostVarWorker(nil)
	reader := &fakePromAPI{err: errors.New("datasource timeout")}
	_, err := arw.getParamPermutation(
		map[string]models.ParamQuery{"ident": {ParamType: "enum", Query: []interface{}{}}},
		[]string{"ident"}, map[string]string{"ident": "ident"}, `cpu_usage{ident="$ident"} > 1`, reader)
	if !errors.Is(err, errParamValueFetch) {
		t.Fatalf("err = %v, want errParamValueFetch", err)
	}
}

// rejectingPromAPI 对查询指定主机的子查询返回 errFor 里的错误，其余行为同 fakePromAPI
type rejectingPromAPI struct {
	fakePromAPI
	errFor map[string]error
}

func (f *rejectingPromAPI) Query(ctx context.Context, query string, ts time.Time) (model.Value, promsdk.Warnings, error) {
	for ident, err := range f.errFor {
		if strings.Contains(query, `ident="`+ident+`"`) {
			return nil, nil, err
		}
	}
	return f.fakePromAPI.Query(ctx, query, ts)
}

func webHostsQuery() models.PromQuery {
	return models.PromQuery{
		PromQl:     `cpu_usage{ident="$ident"} > 1`,
		VarEnabled: true,
		VarConfig: models.VarConfig{
			ParamVal: []models.ParamQueryForFirst{{
				Name:      "ident",
				ParamType: "host",
				Query:     []interface{}{map[string]interface{}{"key": "hosts", "op": "=~", "values": []interface{}{"web*"}}},
			}},
		},
	}
}

var errBadData = &promsdk.Error{Type: promsdk.ErrBadData, Msg: "parse error"}

func TestVarFillingBeforeQueryInvalidSubQueries(t *testing.T) {
	cases := []struct {
		name      string
		errFor    map[string]error
		wantErr   bool
		wantHosts []string
		wantWarns int
	}{
		// 只有一个组合的查询不合法：跳过它并写入 eval log 警告，其他主机照常出点
		{"one invalid", map[string]error{"web-02": errBadData}, false, []string{"web-01"}, 1},
		// 所有组合都不合法：promql 本身有问题，中止本轮
		{"all invalid", map[string]error{"web-01": errBadData, "web-02": errBadData}, true, nil, 0},
		// 超时等瞬时错误仍然中止本轮
		{"one timeout", map[string]error{"web-02": &promsdk.Error{Type: promsdk.ErrTimeout, Msg: "timeout"}}, true, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arw := newEdgeHostVarWorker(nil)
			reader := &rejectingPromAPI{fakePromAPI: fakePromAPI{idents: []string{"web-01", "web-02"}}, errFor: tc.errFor}
			points, warns, err := arw.varFillingBeforeQuery(webHostsQuery(), reader)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got := pointIdents(points); !reflect.DeepEqual(got, tc.wantHosts) {
				t.Fatalf("points = %v, want %v", got, tc.wantHosts)
			}
			if len(warns) != tc.wantWarns {
				t.Fatalf("warns = %v, want %d", warns, tc.wantWarns)
			}
		})
	}
}

// 填进 promql 的值带引号或反斜杠时，发给数据源的查询仍要能解析
func TestFillVarEscapedValues(t *testing.T) {
	// 展示用的 fillVar 保持原样：其结果参与告警事件 hash
	if got := fillVar(`cpu{ident="$ident"}`, "ident", `a"b`); got != `cpu{ident="a"b"}` {
		t.Fatalf("fillVar must not escape, got %s", got)
	}
	for _, val := range []string{`HOST\SQLEXPRESS`, `say "hi"`, `it's`, `\d+`} {
		for _, ql := range []string{`cpu{ident="$ident"}`, `cpu{ident='$ident'}`} {
			got := fillVarEscaped(ql, "ident", val)
			expr, err := promparser.ParseExpr(got)
			if err != nil {
				t.Fatalf("fillVarEscaped(%q, %q) = %q, parse error: %v", ql, val, got, err)
			}
			sel, ok := expr.(*promparser.VectorSelector)
			if !ok {
				t.Fatalf("unexpected expr %T", expr)
			}
			for _, m := range sel.LabelMatchers {
				if m.Name == "ident" && m.Value != val {
					t.Fatalf("ident = %q, want %q", m.Value, val)
				}
			}
		}
	}
}

// 主机筛选值无法编译成正则时，按取值失败中止本轮，不能 panic
func TestEdgeHostVarInvalidFilterIsFetchError(t *testing.T) {
	arw := newEdgeHostVarWorker(nil)
	query := webHostsQuery()
	query.VarConfig.ParamVal[0].Query = []interface{}{map[string]interface{}{
		"key": "hosts", "op": "=~", "values": []interface{}{strings.Repeat("a_", 2_000_000)},
	}}
	_, err := arw.VarFillingBeforeQuery(query, &fakePromAPI{idents: []string{"web-01"}})
	if !errors.Is(err, errParamValueFetch) {
		t.Fatalf("err = %v, want errParamValueFetch", err)
	}
}
