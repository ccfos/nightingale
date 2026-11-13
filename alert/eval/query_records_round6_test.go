package eval

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/evallog"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetPromAnomalyPoint_RecordsQueryWarningsAndReturnedSeries(t *testing.T) {
	round4EvalLog(t)
	worker := newAnomalyTestWorker()
	worker.EvalRec = &evallog.EvalRecord{}
	api := &fixedAnomalyPromAPI{
		value:    model.Vector{{Metric: model.Metric{"host": "node"}, Timestamp: 1700000000000, Value: 95}},
		warnings: promsdk.Warnings{"partial response"},
	}
	attachAnomalyPromAPI(worker, api)

	points, err := worker.GetPromAnomalyPoint(`{"queries":[{"prom_ql":" cpu > 90 ","severity":2}]}`)

	require.NoError(t, err)
	require.Len(t, points, 1)
	require.Equal(t, "cpu > 90", points[0].Query)
	require.Len(t, worker.EvalRec.Queries, 1)
	record := worker.EvalRec.Queries[0]
	require.Equal(t, "0", record.Ref)
	require.Equal(t, "cpu > 90", record.Query)
	require.Equal(t, []string{"partial response"}, record.Warnings)
	require.Equal(t, 1, record.SeriesTotal)
	require.Equal(t, []evallog.SeriesSample{{
		Labels: map[string]string{"host": "node"}, Points: [][2]float64{{1700000000, 95}},
	}}, record.Series)
}

func TestGetPromAnomalyPoint_RecordsFailedQueryWithoutSeries(t *testing.T) {
	round4EvalLog(t)
	worker := newAnomalyTestWorker()
	worker.EvalRec = &evallog.EvalRecord{}
	queryErr := errors.New("query rejected")
	api := &fixedAnomalyPromAPI{err: queryErr}
	attachAnomalyPromAPI(worker, api)

	points, err := worker.GetPromAnomalyPoint(`{"queries":[{"prom_ql":"cpu > 90"}]}`)

	require.ErrorIs(t, err, queryErr)
	require.Empty(t, points)
	require.Len(t, worker.EvalRec.Queries, 1)
	require.Equal(t, "query rejected", worker.EvalRec.Queries[0].Error)
	require.Equal(t, "cpu > 90", worker.EvalRec.Queries[0].Query)
	require.Empty(t, worker.EvalRec.Queries[0].Series)
}

func TestGetPromAnomalyPoint_RecordsExpandedVariableQueryResults(t *testing.T) {
	for _, query := range []string{`cpu{ident="$ident"} > 1`, `sum(cpu{ident="$ident"}) > 1`} {
		t.Run(query, func(t *testing.T) {
			round4EvalLog(t)
			worker := newAnomalyTestWorker()
			worker.Processor.Stats.CounterVarFillingQuery = prometheus.NewCounterVec(
				prometheus.CounterOpts{Name: "round6_variable_queries"}, []string{"rule", "datasource", "ref", "strategy"})
			worker.EvalRec = &evallog.EvalRecord{}
			api := &fixedAnomalyPromAPI{value: model.Vector{{
				Metric: model.Metric{"ident": "node"}, Timestamp: 1700000000000, Value: 5,
			}}}
			attachAnomalyPromAPI(worker, api)
			promQuery := models.PromRuleConfig{Queries: []models.PromQuery{{
				PromQl: query, VarEnabled: true, Severity: 2,
				VarConfig: models.VarConfig{ParamVal: []models.ParamQueryForFirst{{
					Name: "ident", ParamType: "enum", Query: []string{"node"},
				}}},
			}}}
			encoded := round6PromConfig(t, promQuery)

			points, err := worker.GetPromAnomalyPoint(encoded)

			require.NoError(t, err)
			require.Len(t, points, 1)
			require.Len(t, worker.EvalRec.Queries, 1)
			record := worker.EvalRec.Queries[0]
			require.True(t, record.VarQuery)
			require.Equal(t, query, record.Query)
			require.Equal(t, 1, record.SeriesTotal)
			require.Equal(t, []evallog.SeriesSample{{
				Labels: map[string]string{"ident": "node"}, Points: [][2]float64{{1700000000, 5}},
			}}, record.Series)
		})
	}
}

func TestGetPromAnomalyPoint_RecordsVariableQueryFailure(t *testing.T) {
	round4EvalLog(t)
	worker := newAnomalyTestWorker()
	worker.Processor.Stats.CounterVarFillingQuery = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "round6_variable_queries"}, []string{"rule", "datasource", "ref", "strategy"})
	worker.EvalRec = &evallog.EvalRecord{}
	api := &fixedAnomalyPromAPI{err: errors.New("query rejected")}
	attachAnomalyPromAPI(worker, api)
	config := round6PromConfig(t, models.PromRuleConfig{Queries: []models.PromQuery{{
		PromQl: `cpu{ident="$ident"} > 1`, VarEnabled: true,
		VarConfig: models.VarConfig{ParamVal: []models.ParamQueryForFirst{{
			Name: "ident", ParamType: "enum", Query: []string{"node"},
		}}},
	}}})

	points, err := worker.GetPromAnomalyPoint(config)

	require.ErrorContains(t, err, "query rejected")
	require.Empty(t, points)
	require.Len(t, worker.EvalRec.Queries, 1)
	require.True(t, worker.EvalRec.Queries[0].VarQuery)
	require.Contains(t, worker.EvalRec.Queries[0].Error, "query rejected")
}

func TestGetPromAnomalyPoint_MissingClientMarksQueryUnavailable(t *testing.T) {
	worker := newAnomalyTestWorker()
	worker.PromClients = &prom.PromClientMap{}

	points, err := worker.GetPromAnomalyPoint(`{"queries":[{"prom_ql":"up == 0"}]}`)

	require.NoError(t, err)
	require.Empty(t, points)
	gauge := worker.Processor.Stats.GaugeQuerySeriesCount.WithLabelValues("7302", "0", "0")
	require.Equal(t, -2.0, testutil.ToFloat64(gauge))
}

func TestGetHostIdents_CenterFiltersTargetsAndCachesSelection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.Target{}, &models.TargetBusiGroup{}))
	require.NoError(t, db.Create(&[]models.Target{{Ident: "web-1"}, {Ident: "db-1"}}).Error)
	worker := newAnomalyTestWorker()
	worker.Ctx = ctx.NewContext(context.Background(), db, true)
	query := models.ParamQuery{Query: []models.HostQuery{{Key: "hosts", Op: "==", Values: []interface{}{"web-1"}}}}

	idents, err := worker.getHostIdents(query)

	require.NoError(t, err)
	require.Equal(t, []string{"web-1"}, idents)
	// A second call uses the cached selection even if the underlying target disappears.
	require.NoError(t, db.Where("ident = ?", "web-1").Delete(&models.Target{}).Error)
	idents, err = worker.getHostIdents(query)
	require.NoError(t, err)
	require.Equal(t, []string{"web-1"}, idents)
}

func TestGetHostIdents_CenterDatabaseFailurePreservesErrorIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	worker := newAnomalyTestWorker()
	worker.Ctx = ctx.NewContext(context.Background(), db, true)

	idents, err := worker.getHostIdents(models.ParamQuery{})

	require.ErrorIs(t, err, errParamValueFetch)
	require.Empty(t, idents)
}

func round6PromConfig(t *testing.T, config models.PromRuleConfig) string {
	t.Helper()
	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	return string(encoded)
}
