// Package tsdb embeds the prometheus tsdb storage engine and promql engine
// in process, so the all-in-one n9e binary can store and query metrics
// without an external TSDB.
package tsdb

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/tsdb/tconf"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/promql"
	promtsdb "github.com/prometheus/prometheus/tsdb"
	"github.com/toolkits/pkg/logger"
)

// registerOnce: tsdb/promql register their collectors on the registerer at
// construction time and would panic on re-registration; only the first Open
// in the process (the only one outside tests) exports metrics.
var registerOnce sync.Once

type Instance struct {
	DB     *promtsdb.DB
	Engine *promql.Engine
	Cfg    tconf.EmbeddedTSDB
}

func Open(cfg tconf.EmbeddedTSDB) (*Instance, error) {
	kl := kitLogger{}

	var reg prometheus.Registerer
	registerOnce.Do(func() { reg = prometheus.DefaultRegisterer })

	opts := promtsdb.DefaultOptions()
	opts.RetentionDuration = cfg.RetentionDurationValue.Milliseconds()
	opts.MaxBytes = cfg.MaxBytesValue
	opts.OutOfOrderTimeWindow = cfg.OutOfOrderTimeWindowValue.Milliseconds()

	// same algorithm as cmd/prometheus: allow compacting up to
	// min(retention/10, 31d) so old 2h blocks get merged instead of piling up
	maxBlock := opts.RetentionDuration / 10
	if limit := (31 * 24 * time.Hour).Milliseconds(); maxBlock > limit {
		maxBlock = limit
	}
	if maxBlock > opts.MaxBlockDuration {
		opts.MaxBlockDuration = maxBlock
	}

	db, err := promtsdb.Open(cfg.Dir, kl, reg, opts, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open embedded tsdb dir: %s error: %v", cfg.Dir, err)
	}

	engine := promql.NewEngine(promql.EngineOpts{
		Logger:                   kl,
		Reg:                      reg,
		MaxSamples:               cfg.QueryMaxSamples,
		Timeout:                  cfg.QueryTimeoutValue,
		LookbackDelta:            cfg.LookbackDeltaValue,
		NoStepSubqueryIntervalFn: func(int64) int64 { return time.Minute.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
		// bounds concurrent queries (queue when exceeded), otherwise a burst
		// of heavy queries can OOM the whole process
		ActiveQueryTracker: promql.NewActiveQueryTracker(cfg.Dir, cfg.QueryMaxConcurrency, kl),
	})

	logger.Infof("embedded tsdb opened, dir: %s retention: %s maxBytes: %d", cfg.Dir, cfg.RetentionDuration, cfg.MaxBytesValue)

	return &Instance{DB: db, Engine: engine, Cfg: cfg}, nil
}

func (in *Instance) Close() error {
	logger.Info("embedded tsdb closing...")
	return in.DB.Close()
}

// kitLogger adapts the go-kit logger required by prometheus tsdb/promql to
// the toolkits logger used across n9e.
type kitLogger struct{}

func (kitLogger) Log(keyvals ...interface{}) error {
	var sb strings.Builder
	level := "info"

	for i := 0; i+1 < len(keyvals); i += 2 {
		k := fmt.Sprint(keyvals[i])
		v := fmt.Sprint(keyvals[i+1])
		if k == "level" {
			level = v
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(v)
	}

	msg := "embedded-tsdb: " + sb.String()
	switch level {
	case "error":
		logger.Error(msg)
	case "warn":
		logger.Warning(msg)
	case "debug":
		logger.Debug(msg)
	default:
		logger.Info(msg)
	}

	return nil
}
