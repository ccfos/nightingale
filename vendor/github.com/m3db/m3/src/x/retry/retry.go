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

package retry

import (
	"errors"
	"math"
	"time"

	xerrors "github.com/m3db/m3/src/x/errors"

	"github.com/uber-go/tally"
)

var (
	// ErrWhileConditionFalse is returned when the while condition to a while retry
	// method evaluates false.
	ErrWhileConditionFalse = errors.New("retry while condition evaluated to false")
)

type retrier struct {
	opts           Options
	initialBackoff time.Duration
	backoffFactor  float64
	maxBackoff     time.Duration
	maxRetries     int
	forever        bool
	jitter         bool
	rngFn          RngFn
	sleepFn        func(t time.Duration)
	metrics        retrierMetrics
}

type retrierMetrics struct {
	calls              tally.Counter
	attempts           tally.Counter
	success            tally.Counter
	successLatency     tally.Histogram
	errors             tally.Counter
	errorsNotRetryable tally.Counter
	errorsFinal        tally.Counter
	errorsLatency      tally.Histogram
	retries            tally.Counter
}

// NewRetrier creates a new retrier.
func NewRetrier(opts Options) Retrier {
	scope := opts.MetricsScope()
	errorTags := struct {
		retryable    map[string]string
		notRetryable map[string]string
	}{
		map[string]string{
			"type": "retryable",
		},
		map[string]string{
			"type": "not-retryable",
		},
	}

	return &retrier{
		opts:           opts,
		initialBackoff: opts.InitialBackoff(),
		backoffFactor:  opts.BackoffFactor(),
		maxBackoff:     opts.MaxBackoff(),
		maxRetries:     opts.MaxRetries(),
		forever:        opts.Forever(),
		jitter:         opts.Jitter(),
		rngFn:          opts.RngFn(),
		sleepFn:        time.Sleep,
		metrics: retrierMetrics{
			calls:              scope.Counter("calls"),
			attempts:           scope.Counter("attempts"),
			success:            scope.Counter("success"),
			successLatency:     histogramWithDurationBuckets(scope, "success-latency"),
			errors:             scope.Tagged(errorTags.retryable).Counter("errors"),
			errorsNotRetryable: scope.Tagged(errorTags.notRetryable).Counter("errors"),
			errorsFinal:        scope.Counter("errors-final"),
			errorsLatency:      histogramWithDurationBuckets(scope, "errors-latency"),
			retries:            scope.Counter("retries"),
		},
	}
}

func (r *retrier) Options() Options {
	return r.opts
}

func (r *retrier) Attempt(fn Fn) error {
	return r.attempt(nil, fn)
}

func (r *retrier) AttemptWhile(continueFn ContinueFn, fn Fn) error {
	return r.attempt(continueFn, fn)
}

func (r *retrier) attempt(continueFn ContinueFn, fn Fn) error {
	// Always track a call, useful for counting number of total operations.
	r.metrics.calls.Inc(1)

	attempt := 0

	if continueFn != nil && !continueFn(attempt) {
		return ErrWhileConditionFalse
	}

	start := time.Now()
	err := fn()
	duration := time.Since(start)
	r.metrics.attempts.Inc(1)
	attempt++
	if err == nil {
		r.metrics.successLatency.RecordDuration(duration)
		r.metrics.success.Inc(1)
		return nil
	}
	r.metrics.errorsLatency.RecordDuration(duration)
	if xerrors.IsNonRetryableError(err) {
		r.metrics.errorsNotRetryable.Inc(1)
		return err
	}
	r.metrics.errors.Inc(1)

	for i := 1; r.forever || i <= r.maxRetries; i++ {
		r.sleepFn(time.Duration(BackoffNanos(
			i,
			r.jitter,
			r.backoffFactor,
			r.initialBackoff,
			r.maxBackoff,
			r.rngFn,
		)))

		if continueFn != nil && !continueFn(attempt) {
			return ErrWhileConditionFalse
		}

		r.metrics.retries.Inc(1)
		start := time.Now()
		err = fn()
		duration := time.Since(start)
		r.metrics.attempts.Inc(1)
		attempt++
		if err == nil {
			r.metrics.successLatency.RecordDuration(duration)
			r.metrics.success.Inc(1)
			return nil
		}
		r.metrics.errorsLatency.RecordDuration(duration)
		if xerrors.IsNonRetryableError(err) {
			r.metrics.errorsNotRetryable.Inc(1)
			return err
		}
		r.metrics.errors.Inc(1)
	}
	r.metrics.errorsFinal.Inc(1)

	return err
}

// BackoffNanos calculates the backoff for a retry in nanoseconds.
func BackoffNanos(
	retry int,
	jitter bool,
	backoffFactor float64,
	initialBackoff time.Duration,
	maxBackoff time.Duration,
	rngFn RngFn,
) int64 {
	backoff := initialBackoff.Nanoseconds()
	if retry >= 1 {
		backoffFloat64 := float64(backoff) * math.Pow(backoffFactor, float64(retry-1))
		// math.Inf is also larger than math.MaxInt64.
		if backoffFloat64 > math.MaxInt64 {
			return maxBackoff.Nanoseconds()
		}
		backoff = int64(backoffFloat64)
	}
	// Validate the value of backoff to make sure Int63n() does not panic.
	if jitter && backoff >= 2 {
		half := backoff / 2
		backoff = half + rngFn(half)
	}
	if maxBackoff := maxBackoff.Nanoseconds(); backoff > maxBackoff {
		backoff = maxBackoff
	}
	return backoff
}

// histogramWithDurationBuckets returns a histogram with the standard duration buckets.
func histogramWithDurationBuckets(scope tally.Scope, name string) tally.Histogram {
	sub := scope.Tagged(map[string]string{
		// Bump the version if the histogram buckets need to be changed to avoid overlapping buckets
		// in the same query causing errors.
		"schema": "v1",
	})
	buckets := append(tally.DurationBuckets{0, time.Millisecond},
		tally.MustMakeExponentialDurationBuckets(2*time.Millisecond, 1.5, 30)...)
	return sub.Histogram(name, buckets)
}
