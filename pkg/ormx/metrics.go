package ormx

import (
	"database/sql"
	"errors"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

const metricNamespace = "n9e"

// The "operation" label tracks which GORM callback chain a query went through.
// The mapping below is based on GORM v1.25.10 source (finisher_api.go) and is
// non-obvious in two places — read this before filtering by op in dashboards:
//
//	op="create"  ← db.Create / Save / CreateInBatches
//	op="query"   ← db.First / Take / Last / Find / Pluck / Count
//	               AND db.Raw("SELECT...").Find(&x)  (Find always uses Query chain)
//	op="update"  ← db.Update / Updates / UpdateColumn / UpdateColumns
//	op="delete"  ← db.Delete
//	op="row"     ← db.Row() / db.Rows()
//	               AND db.Raw("SELECT...").Row()/.Rows()
//	               AND db.Raw("SELECT...").Scan(&x)   ← !! Scan internally calls Rows()
//	op="raw"     ← db.Exec(...) ONLY  (and migrator AutoMigrate, which is Exec underneath)
//
// In particular: ad-hoc raw SELECTs scanned into a struct land in op="row",
// not op="raw"; op="raw" is essentially "INSERT/UPDATE/DDL via db.Exec".
//
// table label: db.Statement.Table; "unknown" when GORM can't infer it
// (typical for db.Raw / db.Exec without a Model). Bounded cardinality.
//
// status label: "success" or "fail"; gorm.ErrRecordNotFound is treated as success
// because it's a legitimate "no rows" outcome, not a query error.
var (
	DBOperationLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Subsystem: "db",
			Name:      "operation_latency_seconds",
			Help:      "Histogram of latencies for DB operations",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"operation", "table", "status"},
	)

	// DBOperationTotal counts DB operations with the same labels as
	// DBOperationLatency. Useful for QPS and error-rate calculations.
	DBOperationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Subsystem: "db",
			Name:      "operation_total",
			Help:      "Total number of DB operations",
		},
		[]string{"operation", "table", "status"},
	)

	poolCollector = newDBPoolCollector()
)

func init() {
	prometheus.MustRegister(DBOperationLatency, DBOperationTotal, poolCollector)
}

// RegisterDBMetrics installs GORM callbacks that record per-query latency and
// counter metrics on db, and binds the underlying *sql.DB to the connection
// pool collector. It is safe to call multiple times for different *gorm.DB
// instances; the latest one wins for pool stats.
func RegisterDBMetrics(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	poolCollector.sqlDB.Store(sqlDB)

	cb := db.Callback()

	if err := cb.Create().Before("gorm:create").Register("n9e:metrics:before_create", beforeHook); err != nil {
		return err
	}
	if err := cb.Create().After("gorm:create").Register("n9e:metrics:after_create", afterHookFor("create")); err != nil {
		return err
	}

	if err := cb.Query().Before("gorm:query").Register("n9e:metrics:before_query", beforeHook); err != nil {
		return err
	}
	if err := cb.Query().After("gorm:query").Register("n9e:metrics:after_query", afterHookFor("query")); err != nil {
		return err
	}

	if err := cb.Update().Before("gorm:update").Register("n9e:metrics:before_update", beforeHook); err != nil {
		return err
	}
	if err := cb.Update().After("gorm:update").Register("n9e:metrics:after_update", afterHookFor("update")); err != nil {
		return err
	}

	if err := cb.Delete().Before("gorm:delete").Register("n9e:metrics:before_delete", beforeHook); err != nil {
		return err
	}
	if err := cb.Delete().After("gorm:delete").Register("n9e:metrics:after_delete", afterHookFor("delete")); err != nil {
		return err
	}

	if err := cb.Row().Before("gorm:row").Register("n9e:metrics:before_row", beforeHook); err != nil {
		return err
	}
	if err := cb.Row().After("gorm:row").Register("n9e:metrics:after_row", afterHookFor("row")); err != nil {
		return err
	}

	if err := cb.Raw().Before("gorm:raw").Register("n9e:metrics:before_raw", beforeHook); err != nil {
		return err
	}
	if err := cb.Raw().After("gorm:raw").Register("n9e:metrics:after_raw", afterHookFor("raw")); err != nil {
		return err
	}

	return nil
}

const startTimeKey = "n9e:db:start_time"

func beforeHook(db *gorm.DB) {
	db.InstanceSet(startTimeKey, time.Now())
}

func afterHookFor(op string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		v, ok := db.InstanceGet(startTimeKey)
		if !ok {
			return
		}
		start, ok := v.(time.Time)
		if !ok {
			return
		}

		status := "success"
		if db.Error != nil && !errors.Is(db.Error, gorm.ErrRecordNotFound) {
			status = "fail"
		}

		table := db.Statement.Table
		if table == "" {
			table = "unknown"
		}
		elapsed := time.Since(start).Seconds()

		DBOperationLatency.WithLabelValues(op, table, status).Observe(elapsed)
		DBOperationTotal.WithLabelValues(op, table, status).Inc()
	}
}

// dbPoolCollector implements prometheus.Collector to expose database/sql pool
// stats lazily on each scrape, so we never block on a metrics HTTP request.
type dbPoolCollector struct {
	sqlDB atomic.Pointer[sql.DB]

	maxOpenConns      *prometheus.Desc
	openConns         *prometheus.Desc
	inUseConns        *prometheus.Desc
	idleConns         *prometheus.Desc
	waitCount         *prometheus.Desc
	waitDuration      *prometheus.Desc
	maxIdleClosed     *prometheus.Desc
	maxIdleTimeClosed *prometheus.Desc
	maxLifetimeClosed *prometheus.Desc
}

func newDBPoolCollector() *dbPoolCollector {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(
			prometheus.BuildFQName(metricNamespace, "db_pool", name),
			help, nil, nil,
		)
	}
	return &dbPoolCollector{
		maxOpenConns:      desc("max_open_connections", "Maximum number of open connections to the database"),
		openConns:         desc("open_connections", "The number of established connections both in use and idle"),
		inUseConns:        desc("in_use_connections", "The number of connections currently in use"),
		idleConns:         desc("idle_connections", "The number of idle connections"),
		waitCount:         desc("wait_count_total", "The total number of connections waited for"),
		waitDuration:      desc("wait_duration_seconds_total", "The total time blocked waiting for a new connection"),
		maxIdleClosed:     desc("max_idle_closed_total", "The total number of connections closed due to SetMaxIdleConns"),
		maxIdleTimeClosed: desc("max_idle_time_closed_total", "The total number of connections closed due to SetConnMaxIdleTime"),
		maxLifetimeClosed: desc("max_lifetime_closed_total", "The total number of connections closed due to SetConnMaxLifetime"),
	}
}

func (c *dbPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.maxOpenConns
	ch <- c.openConns
	ch <- c.inUseConns
	ch <- c.idleConns
	ch <- c.waitCount
	ch <- c.waitDuration
	ch <- c.maxIdleClosed
	ch <- c.maxIdleTimeClosed
	ch <- c.maxLifetimeClosed
}

func (c *dbPoolCollector) Collect(ch chan<- prometheus.Metric) {
	db := c.sqlDB.Load()
	if db == nil {
		return
	}
	s := db.Stats()
	ch <- prometheus.MustNewConstMetric(c.maxOpenConns, prometheus.GaugeValue, float64(s.MaxOpenConnections))
	ch <- prometheus.MustNewConstMetric(c.openConns, prometheus.GaugeValue, float64(s.OpenConnections))
	ch <- prometheus.MustNewConstMetric(c.inUseConns, prometheus.GaugeValue, float64(s.InUse))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(s.Idle))
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(s.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.waitDuration, prometheus.CounterValue, s.WaitDuration.Seconds())
	ch <- prometheus.MustNewConstMetric(c.maxIdleClosed, prometheus.CounterValue, float64(s.MaxIdleClosed))
	ch <- prometheus.MustNewConstMetric(c.maxIdleTimeClosed, prometheus.CounterValue, float64(s.MaxIdleTimeClosed))
	ch <- prometheus.MustNewConstMetric(c.maxLifetimeClosed, prometheus.CounterValue, float64(s.MaxLifetimeClosed))
}
