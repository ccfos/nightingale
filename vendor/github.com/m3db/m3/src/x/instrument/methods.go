// Copyright (c) 2016 Uber Technologies, Inc.
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

package instrument

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/uber-go/tally"
)

var (
	nullStopWatchStart tally.Stopwatch
)

// TimerType is a type of timer, standard or histogram timer.
type TimerType uint

const (
	// StandardTimerType is a standard timer type back by a regular timer.
	StandardTimerType TimerType = iota
	// HistogramTimerType is a histogram timer backed by a histogram.
	HistogramTimerType
)

// TimerOptions is a set of timer options when creating a timer.
type TimerOptions struct {
	Type               TimerType
	StandardSampleRate float64
	HistogramBuckets   tally.Buckets
}

// NewTimer creates a new timer based on the timer options.
func (o TimerOptions) NewTimer(scope tally.Scope, name string) tally.Timer {
	return NewTimer(scope, name, o)
}

// DefaultHistogramTimerHistogramBuckets returns a set of default
// histogram timer histogram buckets, from 2ms up to 1hr.
func DefaultHistogramTimerHistogramBuckets() tally.Buckets {
	return tally.ValueBuckets{
		0.002,
		0.004,
		0.006,
		0.008,
		0.01,
		0.02,
		0.04,
		0.06,
		0.08,
		0.1,
		0.2,
		0.4,
		0.6,
		0.8,
		1,
		1.5,
		2,
		2.5,
		3,
		3.5,
		4,
		4.5,
		5,
		5.5,
		6,
		6.5,
		7,
		7.5,
		8,
		8.5,
		9,
		9.5,
		10,
		15,
		20,
		25,
		30,
		35,
		40,
		45,
		50,
		55,
		60,
		150,
		300,
		450,
		600,
		900,
		1200,
		1500,
		1800,
		2100,
		2400,
		2700,
		3000,
		3300,
		3600,
	}
}

// DefaultSummaryQuantileObjectives is a set of default summary
// quantile objectives and allowed error thresholds.
func DefaultSummaryQuantileObjectives() map[float64]float64 {
	return map[float64]float64{
		0.5:   0.01,
		0.75:  0.001,
		0.95:  0.001,
		0.99:  0.001,
		0.999: 0.0001,
	}
}

// NewStandardTimerOptions returns a set of standard timer options for
// standard timer types.
func NewStandardTimerOptions() TimerOptions {
	return TimerOptions{Type: StandardTimerType}
}

// HistogramTimerOptions is a set of histogram timer options.
type HistogramTimerOptions struct {
	HistogramBuckets tally.Buckets
}

// NewHistogramTimerOptions returns a set of histogram timer options
// and if no histogram buckets are set it will use the default
// histogram buckets defined.
func NewHistogramTimerOptions(opts HistogramTimerOptions) TimerOptions {
	result := TimerOptions{Type: HistogramTimerType}
	if opts.HistogramBuckets != nil && opts.HistogramBuckets.Len() > 0 {
		result.HistogramBuckets = opts.HistogramBuckets
	} else {
		result.HistogramBuckets = DefaultHistogramTimerHistogramBuckets()
	}
	return result
}

var _ tally.Timer = (*timer)(nil)

// timer is a timer that can be backed by a timer or a histogram
// depending on TimerOptions.
type timer struct {
	TimerOptions
	timer     tally.Timer
	histogram tally.Histogram
}

// NewTimer returns a new timer that is backed by a timer or a histogram
// based on the timer options.
func NewTimer(scope tally.Scope, name string, opts TimerOptions) tally.Timer {
	t := &timer{TimerOptions: opts}
	switch t.Type {
	case HistogramTimerType:
		t.histogram = scope.Histogram(name, opts.HistogramBuckets)
	default:
		t.timer = scope.Timer(name)
		if rate := opts.StandardSampleRate; validRate(rate) {
			t.timer = newSampledTimer(t.timer, rate)
		}
	}
	return t
}

func (t *timer) Record(v time.Duration) {
	switch t.Type {
	case HistogramTimerType:
		t.histogram.RecordDuration(v)
	default:
		t.timer.Record(v)
	}
}

func (t *timer) Start() tally.Stopwatch {
	return tally.NewStopwatch(time.Now(), t)
}

func (t *timer) RecordStopwatch(stopwatchStart time.Time) {
	t.Record(time.Since(stopwatchStart))
}

// sampledTimer is a sampled timer that implements the tally timer interface.
// NB(xichen): the sampling logic should eventually be implemented in tally.
type sampledTimer struct {
	tally.Timer

	cnt  uint64
	rate uint64
}

// NewSampledTimer creates a new sampled timer.
func NewSampledTimer(base tally.Timer, rate float64) (tally.Timer, error) {
	if !validRate(rate) {
		return nil, fmt.Errorf("sampling rate %f must be between 0.0 and 1.0", rate)
	}
	return newSampledTimer(base, rate), nil
}

func validRate(rate float64) bool {
	return rate > 0.0 && rate <= 1.0
}

func newSampledTimer(base tally.Timer, rate float64) tally.Timer {
	if rate == 1.0 {
		// Avoid the overhead of working out if should sample each time.
		return base
	}
	return &sampledTimer{
		Timer: base,
		rate:  uint64(1.0 / rate),
	}
}

// MustCreateSampledTimer creates a new sampled timer, and panics if an error
// is encountered.
func MustCreateSampledTimer(base tally.Timer, rate float64) tally.Timer {
	t, err := NewSampledTimer(base, rate)
	if err != nil {
		panic(err)
	}
	return t
}

func (t *sampledTimer) shouldSample() bool {
	return atomic.AddUint64(&t.cnt, 1)%t.rate == 0
}

func (t *sampledTimer) Start() tally.Stopwatch {
	if !t.shouldSample() {
		return nullStopWatchStart
	}
	return t.Timer.Start()
}

func (t *sampledTimer) Stop(startTime tally.Stopwatch) {
	if startTime == nullStopWatchStart { // nolint: badtime
		// If startTime is nullStopWatchStart, do nothing.
		return
	}
	startTime.Stop()
}

func (t *sampledTimer) Record(d time.Duration) {
	if !t.shouldSample() {
		return
	}
	t.Timer.Record(d)
}

// MethodMetrics is a bundle of common metrics with a uniform naming scheme.
type MethodMetrics struct {
	Errors         tally.Counter
	Success        tally.Counter
	ErrorsLatency  tally.Timer
	SuccessLatency tally.Timer
}

// ReportSuccess reports a success.
func (m *MethodMetrics) ReportSuccess(d time.Duration) {
	m.Success.Inc(1)
	m.SuccessLatency.Record(d)
}

// ReportError reports an error.
func (m *MethodMetrics) ReportError(d time.Duration) {
	m.Errors.Inc(1)
	m.ErrorsLatency.Record(d)
}

// ReportSuccessOrError increments Error/Success counter dependending on the error.
func (m *MethodMetrics) ReportSuccessOrError(e error, d time.Duration) {
	if e != nil {
		m.ReportError(d)
	} else {
		m.ReportSuccess(d)
	}
}

// NewMethodMetrics returns a new Method metrics for the given method name.
func NewMethodMetrics(scope tally.Scope, methodName string, opts TimerOptions) MethodMetrics {
	return MethodMetrics{
		Errors:         scope.Counter(methodName + ".errors"),
		Success:        scope.Counter(methodName + ".success"),
		ErrorsLatency:  NewTimer(scope, methodName+".errors-latency", opts),
		SuccessLatency: NewTimer(scope, methodName+".success-latency", opts),
	}
}

// BatchMethodMetrics is a bundle of common metrics for methods with batch semantics.
type BatchMethodMetrics struct {
	RetryableErrors    tally.Counter
	NonRetryableErrors tally.Counter
	Errors             tally.Counter
	Success            tally.Counter
	Latency            tally.Timer
}

// NewBatchMethodMetrics creates new batch method metrics.
func NewBatchMethodMetrics(
	scope tally.Scope,
	methodName string,
	opts TimerOptions,
) BatchMethodMetrics {
	return BatchMethodMetrics{
		RetryableErrors:    scope.Counter(methodName + ".retryable-errors"),
		NonRetryableErrors: scope.Counter(methodName + ".non-retryable-errors"),
		Errors:             scope.Counter(methodName + ".errors"),
		Success:            scope.Counter(methodName + ".success"),
		Latency:            NewTimer(scope, methodName+".latency", opts),
	}
}

// ReportSuccess reports successess.
func (m *BatchMethodMetrics) ReportSuccess(n int) {
	m.Success.Inc(int64(n))
}

// ReportRetryableErrors reports retryable errors.
func (m *BatchMethodMetrics) ReportRetryableErrors(n int) {
	m.RetryableErrors.Inc(int64(n))
	m.Errors.Inc(int64(n))
}

// ReportNonRetryableErrors reports non-retryable errors.
func (m *BatchMethodMetrics) ReportNonRetryableErrors(n int) {
	m.NonRetryableErrors.Inc(int64(n))
	m.Errors.Inc(int64(n))
}

// ReportLatency reports latency.
func (m *BatchMethodMetrics) ReportLatency(d time.Duration) {
	m.Latency.Record(d)
}
