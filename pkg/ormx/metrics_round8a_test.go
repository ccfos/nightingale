package ormx

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Isolate metrics so tests neither retain closed pools nor alter other tests' counters.
func r8aIsolateMetrics(t *testing.T) {
	t.Helper()
	oldTotal, oldLatency, oldCollector := DBOperationTotal, DBOperationLatency, poolCollector
	t.Cleanup(func() { DBOperationTotal, DBOperationLatency, poolCollector = oldTotal, oldLatency, oldCollector })
	DBOperationTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "r8a_operations"}, []string{"operation", "table", "status"})
	DBOperationLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "r8a_latency"}, []string{"operation", "table", "status"})
	poolCollector = newDBPoolCollector()
}

func TestR8aMetricsCallbacks(t *testing.T) {
	r8aIsolateMetrics(t)
	db := r8aDB(t)
	require.NoError(t, RegisterDBMetrics(db))
	require.NoError(t, db.Exec("CREATE TABLE r8a_metrics (id integer primary key, name text)").Error)
	checks := []struct {
		op, table, status string
		run               func() error
	}{
		{"create", "r8a_metrics", "success", func() error {
			return db.Table("r8a_metrics").Create(map[string]interface{}{"id": 1, "name": "first"}).Error
		}},
		{"query", "r8a_metrics", "success", func() error { var rows []map[string]interface{}; return db.Table("r8a_metrics").Find(&rows).Error }},
		{"update", "r8a_metrics", "success", func() error { return db.Table("r8a_metrics").Where("id = ?", 1).Update("name", "updated").Error }},
		{"row", "unknown", "success", func() error { var n int; return db.Raw("SELECT count(*) FROM r8a_metrics").Row().Scan(&n) }},
		{"raw", "unknown", "success", func() error { return db.Exec("UPDATE r8a_metrics SET name = 'raw'").Error }},
		{"delete", "r8a_metrics", "success", func() error { return db.Table("r8a_metrics").Where("id = ?", 1).Delete(nil).Error }},
	}
	for _, c := range checks {
		t.Run(c.op, func(t *testing.T) {
			counter := DBOperationTotal.WithLabelValues(c.op, c.table, c.status)
			before := testutil.ToFloat64(counter)
			require.NoError(t, c.run())
			require.Equal(t, before+1, testutil.ToFloat64(counter))
		})
	}
	counter := DBOperationTotal.WithLabelValues("raw", "unknown", "fail")
	before := testutil.ToFloat64(counter)
	require.Error(t, db.Exec("INSERT INTO r8a_missing VALUES (1)").Error)
	require.Equal(t, before+1, testutil.ToFloat64(counter))
}

func TestR8aMetricHooksAndPool(t *testing.T) {
	r8aIsolateMetrics(t)
	db := r8aDB(t)
	for _, tc := range []struct {
		name   string
		err    error
		status string
	}{
		{"success", nil, "success"}, {"missing", gorm.ErrRecordNotFound, "success"}, {"failure", errors.New("bad query"), "fail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := db.Session(&gorm.Session{NewDB: true, Initialized: true})
			tx.Statement = &gorm.Statement{DB: tx}
			counter := DBOperationTotal.WithLabelValues("r8a_hook", "unknown", tc.status)
			before := testutil.ToFloat64(counter)
			afterHookFor("r8a_hook")(tx)
			require.Equal(t, before, testutil.ToFloat64(counter))
			tx.InstanceSet(startTimeKey, "wrong type")
			afterHookFor("r8a_hook")(tx)
			require.Equal(t, before, testutil.ToFloat64(counter))
			beforeHook(tx)
			start, ok := tx.InstanceGet(startTimeKey)
			require.True(t, ok)
			require.WithinDuration(t, time.Now(), start.(time.Time), time.Second)
			tx.Error = tc.err
			afterHookFor("r8a_hook")(tx)
			require.Equal(t, before+1, testutil.ToFloat64(counter))
		})
	}
	collector := newDBPoolCollector()
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(collector))
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Empty(t, families)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(7)
	collector.sqlDB.Store(sqlDB)
	families, err = registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 9)
	found := false
	for _, family := range families {
		if family.GetName() == "n9e_db_pool_max_open_connections" {
			found = true
			require.Equal(t, float64(7), family.Metric[0].Gauge.GetValue())
		}
	}
	require.True(t, found)
}
