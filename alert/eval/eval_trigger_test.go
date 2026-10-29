package eval

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/prometheus/common/model"
)

const fakeTriggerCate = "fake-trigger-test"

var (
	testStatsOnce sync.Once
	testStats     *astats.Stats
)

// NewSyncStats registers collectors globally, so it can only run once per test binary
func getTestStats() *astats.Stats {
	testStatsOnce.Do(func() {
		testStats = astats.NewSyncStats()
	})
	return testStats
}

// fakeTriggerDatasource returns the preset series of each query by its ref
type fakeTriggerDatasource struct {
	datasource.Datasource
	series map[string][]models.DataResp
}

func (f *fakeTriggerDatasource) InitClient() error { return nil }

func (f *fakeTriggerDatasource) Equal(p datasource.Datasource) bool { return false }

func (f *fakeTriggerDatasource) QueryData(ctx context.Context, query interface{}) ([]models.DataResp, error) {
	ref, err := GetQueryRef(query)
	if err != nil {
		return nil, err
	}
	return f.series[ref], nil
}

func fakeSeries(ref string, labels model.Metric, value float64) models.DataResp {
	return models.DataResp{
		Ref:    ref,
		Metric: labels,
		Values: [][]float64{{1700000000, value}},
	}
}

func pointKeys(points []models.AnomalyPoint) []string {
	keys := make([]string, 0, len(points))
	for _, p := range points {
		keys = append(keys, fmt.Sprintf("%s/%d", p.Labels.String(), p.Severity))
	}
	sort.Strings(keys)
	return keys
}

// Queries whose series carry different labels end up in different groups, so a
// trigger only referencing $A also sees groups holding nothing but B's series.
// Those groups must be skipped instead of being evaluated with an empty variable
// map (which logs "unknown name $A" and yields a label-less recover point).
func TestGetAnomalyPoint_SkipGroupsWithoutReferencedSeries(t *testing.T) {
	search := model.Metric{"api": "search"}
	fetch := model.Metric{"api": "fetch"}

	tests := []struct {
		name        string
		series      map[string][]models.DataResp
		triggers    string
		wantPoints  []string
		wantRecover []string
	}{
		{
			name: "separate triggers on queries with different labels, both triggered",
			series: map[string][]models.DataResp{
				"A": {fakeSeries("A", search, 50)},
				"B": {fakeSeries("B", fetch, 60)},
			},
			triggers: `[
				{"exp": "$A < 90", "severity": 2, "recover_config": {"judge_type": 1}},
				{"exp": "$B < 90", "severity": 2, "recover_config": {"judge_type": 1}}
			]`,
			wantPoints:  []string{`{api="fetch"}/2`, `{api="search"}/2`},
			wantRecover: []string{},
		},
		{
			name: "separate triggers on queries with different labels, none triggered",
			series: map[string][]models.DataResp{
				"A": {fakeSeries("A", search, 95)},
				"B": {fakeSeries("B", fetch, 95)},
			},
			triggers: `[
				{"exp": "$A < 90", "severity": 2, "recover_config": {"judge_type": 0}},
				{"exp": "$B < 90", "severity": 2, "recover_config": {"judge_type": 0}}
			]`,
			wantPoints:  []string{},
			wantRecover: []string{`{api="fetch"}/2`, `{api="search"}/2`},
		},
		{
			name: "one query returns no data, the other trigger is unaffected",
			series: map[string][]models.DataResp{
				"A": {},
				"B": {fakeSeries("B", fetch, 60)},
			},
			triggers: `[
				{"exp": "$A < 90", "severity": 2, "recover_config": {"judge_type": 0}},
				{"exp": "$B < 90", "severity": 1, "recover_config": {"judge_type": 0}}
			]`,
			wantPoints:  []string{`{api="fetch"}/1`},
			wantRecover: []string{},
		},
		{
			name: "queries with the same labels still evaluate together",
			series: map[string][]models.DataResp{
				"A": {fakeSeries("A", search, 50)},
				"B": {fakeSeries("B", search, 95)},
			},
			triggers: `[
				{"exp": "$A < 90", "severity": 2, "recover_config": {"judge_type": 0}},
				{"exp": "$A < 90 && $B < 90", "severity": 1, "recover_config": {"judge_type": 0}}
			]`,
			wantPoints:  []string{`{api="search"}/2`},
			wantRecover: []string{`{api="search"}/1`},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsId := int64(9000 + i)
			dscache.DsCache.Put(fakeTriggerCate, dsId, &fakeTriggerDatasource{series: tt.series})

			rule := &models.AlertRule{
				Id:   int64(100 + i),
				Prod: models.METRIC,
				Cate: fakeTriggerCate,
				RuleConfig: `{
					"queries": [{"ref": "A", "unit": "none"}, {"ref": "B", "unit": "none"}],
					"triggers": ` + tt.triggers + `
				}`,
			}
			arw := &AlertRuleWorker{
				DatasourceId: dsId,
				Rule:         rule,
				Processor:    &process.Processor{Stats: getTestStats()},
			}

			points, recoverPoints, err := arw.GetAnomalyPoint(rule, dsId)
			if err != nil {
				t.Fatalf("GetAnomalyPoint() error = %v", err)
			}
			if got := pointKeys(points); fmt.Sprint(got) != fmt.Sprint(tt.wantPoints) {
				t.Errorf("points = %v, want %v", got, tt.wantPoints)
			}
			if got := pointKeys(recoverPoints); fmt.Sprint(got) != fmt.Sprint(tt.wantRecover) {
				t.Errorf("recoverPoints = %v, want %v", got, tt.wantRecover)
			}
		})
	}
}
