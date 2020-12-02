// Copyright (c) 2018 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cost

import (
	"fmt"
	"sync"
	"time"

	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/cluster/kv/util"

	"github.com/uber-go/tally"
	"go.uber.org/atomic"
)

type limitManager struct {
	sync.RWMutex

	threshold *atomic.Float64
	enabled   *atomic.Bool

	thresholdWatcher kv.ValueWatch
	enabledWatcher   kv.ValueWatch
	closed           bool
	closeCh          chan struct{}
	reportInterval   time.Duration
	metrics          limitManagerMetrics
}

// NewDynamicLimitManager returns a new LimitWatcher which watches for updates to the cost limit
// of an operation in KV.
func NewDynamicLimitManager(
	store kv.Store,
	kvLimitKey, kvEnabledKey string,
	opts LimitManagerOptions,
) (LimitManager, error) {
	if opts == nil {
		opts = NewLimitManagerOptions()
	}
	iOpts := opts.InstrumentOptions()

	var (
		limit            = opts.DefaultLimit()
		defaultThreshold = float64(limit.Threshold)
		defaultEnabled   = limit.Enabled
		m                = &limitManager{
			threshold:      atomic.NewFloat64(defaultThreshold),
			enabled:        atomic.NewBool(defaultEnabled),
			closeCh:        make(chan struct{}),
			reportInterval: iOpts.ReportInterval(),
			metrics:        newLimitManagerMetrics(iOpts.MetricsScope()),
		}
	)

	watchOpts := util.NewOptions().SetLogger(iOpts.Logger())
	thresholdWatcher, err := util.WatchAndUpdateAtomicFloat64(
		store,
		kvLimitKey,
		m.threshold,
		defaultThreshold,
		watchOpts.SetValidateFn(opts.ValidateLimitFn()),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to watch key '%s': %v", kvLimitKey, err)
	}
	m.thresholdWatcher = thresholdWatcher

	enabledWatcher, err := util.WatchAndUpdateAtomicBool(
		store,
		kvEnabledKey,
		m.enabled,
		defaultEnabled,
		watchOpts,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to watch key '%s': %v", kvEnabledKey, err)
	}
	m.enabledWatcher = enabledWatcher

	return m, nil
}

func (m *limitManager) Limit() Limit {
	return Limit{
		Threshold: Cost(m.threshold.Load()),
		Enabled:   m.enabled.Load(),
	}
}

func (m *limitManager) Close() {
	m.Lock()
	defer m.Unlock()
	if m.closed {
		return
	}
	if m.thresholdWatcher != nil {
		m.thresholdWatcher.Close()
	}
	if m.enabledWatcher != nil {
		m.enabledWatcher.Close()
	}
	close(m.closeCh)
	m.closed = true
}

func (m *limitManager) Report() {
	ticker := time.NewTicker(m.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			limit := m.Limit()
			m.metrics.threshold.Update(float64(limit.Threshold))

			var v float64
			if limit.Enabled {
				v = 1
			}
			m.metrics.enabled.Update(v)

		case <-m.closeCh:
			return
		}
	}
}

// NewStaticLimitManager returns a new LimitManager which always returns the same limit.
func NewStaticLimitManager(opts LimitManagerOptions) LimitManager {
	if opts == nil {
		opts = NewLimitManagerOptions()
	}
	iOpts := opts.InstrumentOptions()

	var (
		l = opts.DefaultLimit()
		m = &limitManager{
			threshold:      atomic.NewFloat64(float64(l.Threshold)),
			enabled:        atomic.NewBool(l.Enabled),
			closeCh:        make(chan struct{}),
			reportInterval: iOpts.ReportInterval(),
			metrics:        newLimitManagerMetrics(iOpts.MetricsScope()),
		}
	)
	return m
}

type limitManagerMetrics struct {
	threshold tally.Gauge
	enabled   tally.Gauge
}

func newLimitManagerMetrics(s tally.Scope) limitManagerMetrics {
	return limitManagerMetrics{
		threshold: s.Gauge("threshold"),
		enabled:   s.Gauge("enabled"),
	}
}
