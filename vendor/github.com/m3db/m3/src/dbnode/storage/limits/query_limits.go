// Copyright (c) 2020 Uber Technologies, Inc.
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

package limits

import (
	"fmt"
	"time"

	"github.com/m3db/m3/src/x/instrument"

	"github.com/uber-go/tally"
	"go.uber.org/atomic"
)

const defaultLookback = time.Second * 15

type queryLimits struct {
	docsLimit      *lookbackLimit
	bytesReadLimit *lookbackLimit
}

type lookbackLimit struct {
	name    string
	options LookbackLimitOptions
	metrics lookbackLimitMetrics
	recent  *atomic.Int64
	stopCh  chan struct{}
}

type lookbackLimitMetrics struct {
	recentCount tally.Gauge
	recentMax   tally.Gauge
	total       tally.Counter
	exceeded    tally.Counter
}

var (
	_ QueryLimits   = (*queryLimits)(nil)
	_ LookbackLimit = (*lookbackLimit)(nil)
)

// DefaultLookbackLimitOptions returns a new query limits manager.
func DefaultLookbackLimitOptions() LookbackLimitOptions {
	return LookbackLimitOptions{
		// Default to no limit.
		Limit:    0,
		Lookback: defaultLookback,
	}
}

// NewQueryLimits returns a new query limits manager.
func NewQueryLimits(
	docsLimitOpts LookbackLimitOptions,
	bytesReadLimitOpts LookbackLimitOptions,
	instrumentOpts instrument.Options,
) (QueryLimits, error) {
	if err := docsLimitOpts.validate(); err != nil {
		return nil, err
	}
	if err := bytesReadLimitOpts.validate(); err != nil {
		return nil, err
	}
	docsLimit := newLookbackLimit(instrumentOpts, docsLimitOpts, "docs-matched")
	bytesReadLimit := newLookbackLimit(instrumentOpts, bytesReadLimitOpts, "disk-bytes-read")
	return &queryLimits{
		docsLimit:      docsLimit,
		bytesReadLimit: bytesReadLimit,
	}, nil
}

func newLookbackLimit(
	instrumentOpts instrument.Options,
	opts LookbackLimitOptions,
	name string,
) *lookbackLimit {
	return &lookbackLimit{
		name:    name,
		options: opts,
		metrics: newLookbackLimitMetrics(instrumentOpts, name),
		recent:  atomic.NewInt64(0),
		stopCh:  make(chan struct{}),
	}
}

func newLookbackLimitMetrics(instrumentOpts instrument.Options, name string) lookbackLimitMetrics {
	scope := instrumentOpts.
		MetricsScope().
		SubScope("query-limit")
	return lookbackLimitMetrics{
		recentCount: scope.Gauge(fmt.Sprintf("recent-count-%s", name)),
		recentMax:   scope.Gauge(fmt.Sprintf("recent-max-%s", name)),
		total:       scope.Counter(fmt.Sprintf("total-%s", name)),
		exceeded:    scope.Tagged(map[string]string{"limit": name}).Counter("exceeded"),
	}
}

func (q *queryLimits) DocsLimit() LookbackLimit {
	return q.docsLimit
}

func (q *queryLimits) BytesReadLimit() LookbackLimit {
	return q.bytesReadLimit
}

func (q *queryLimits) Start() {
	q.docsLimit.start()
	q.bytesReadLimit.start()
}

func (q *queryLimits) Stop() {
	q.docsLimit.stop()
	q.bytesReadLimit.stop()
}

func (q *queryLimits) AnyExceeded() error {
	if err := q.docsLimit.exceeded(); err != nil {
		return err
	}
	return q.bytesReadLimit.exceeded()
}

// Inc increments the current value and returns an error if above the limit.
func (q *lookbackLimit) Inc(val int) error {
	if val < 0 {
		return fmt.Errorf("invalid negative query limit inc %d", val)
	}
	if val == 0 {
		return q.exceeded()
	}

	// Add the new stats to the global state.
	valI64 := int64(val)
	recent := q.recent.Add(valI64)

	// Update metrics.
	q.metrics.recentCount.Update(float64(recent))
	q.metrics.total.Inc(valI64)

	// Enforce limit (if specified).
	return q.checkLimit(recent)
}

func (q *lookbackLimit) exceeded() error {
	return q.checkLimit(q.recent.Load())
}

func (q *lookbackLimit) checkLimit(recent int64) error {
	if q.options.Limit > 0 && recent > q.options.Limit {
		q.metrics.exceeded.Inc(1)
		return fmt.Errorf(
			"query aborted due to limit: name=%s, limit=%d, current=%d, within=%s",
			q.name, q.options.Limit, recent, q.options.Lookback)
	}
	return nil
}

func (q *lookbackLimit) start() {
	ticker := time.NewTicker(q.options.Lookback)
	go func() {
		for {
			select {
			case <-ticker.C:
				q.reset()
			case <-q.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (q *lookbackLimit) stop() {
	close(q.stopCh)
}

func (q *lookbackLimit) current() int64 {
	return q.recent.Load()
}

func (q *lookbackLimit) reset() {
	// Update peak gauge only on resets so it only tracks
	// the peak values for each lookback period.
	recent := q.recent.Load()
	q.metrics.recentMax.Update(float64(recent))

	// Update the standard recent gauge to reflect drop back to zero.
	q.metrics.recentCount.Update(0)

	q.recent.Store(0)
}

func (opts LookbackLimitOptions) validate() error {
	if opts.Limit < 0 {
		return fmt.Errorf("query limit requires limit >= 0 (%d)", opts.Limit)
	}
	if opts.Lookback <= 0 {
		return fmt.Errorf("query limit requires lookback > 0 (%d)", opts.Lookback)
	}
	return nil
}
