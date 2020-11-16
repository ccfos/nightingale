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

// Package retry provides utilities for retrying functions.
package retry

import (
	"time"

	"github.com/m3db/m3/src/x/errors"

	"github.com/uber-go/tally"
)

// RetryableError returns a retryable error.
func RetryableError(err error) error {
	return errors.NewRetryableError(err)
}

// NonRetryableError returns a non-retryable error.
func NonRetryableError(err error) error {
	return errors.NewNonRetryableError(err)
}

// RngFn returns a non-negative pseudo-random number in [0,n).
type RngFn func(n int64) int64

// Fn is a function that can be retried.
type Fn func() error

// ContinueFn is a function that returns whether to continue attempting an operation.
type ContinueFn func(attempt int) bool

// Retrier is a executor that can retry attempts on executing methods.
type Retrier interface {
	// Options returns the options used to construct the retrier, useful
	// for changing instrumentation options, etc while preserving other options.
	Options() Options

	// Attempt will attempt to perform a function with retries.
	Attempt(fn Fn) error

	// Attempt will attempt to perform a function with retries.
	AttemptWhile(continueFn ContinueFn, fn Fn) error
}

// Options is a set of retry options.
type Options interface {
	// SetMetricsScope sets the metrics scope.
	SetMetricsScope(value tally.Scope) Options

	// MetricsScope returns the metrics scope.
	MetricsScope() tally.Scope

	// SetInitialBackoff sets the initial delay duration.
	SetInitialBackoff(value time.Duration) Options

	// InitialBackoff gets the initial delay duration.
	InitialBackoff() time.Duration

	// SetBackoffFactor sets the backoff factor multiplier when moving to next attempt.
	SetBackoffFactor(value float64) Options

	// BackoffFactor gets the backoff factor multiplier when moving to next attempt.
	BackoffFactor() float64

	// SetMaxBackoff sets the maximum backoff delay.
	SetMaxBackoff(value time.Duration) Options

	// MaxBackoff returns the maximum backoff delay.
	MaxBackoff() time.Duration

	// SetMaxRetries sets the maximum retry attempts.
	SetMaxRetries(value int) Options

	// Max gets the maximum retry attempts.
	MaxRetries() int

	// SetForever sets whether to retry forever until either the attempt succeeds,
	// or the retry condition becomes false.
	SetForever(value bool) Options

	// Forever returns whether to retry forever until either the attempt succeeds,
	// or the retry condition becomes false.
	Forever() bool

	// SetJitter sets whether to jitter between the current backoff and the next
	// backoff when moving to next attempt.
	SetJitter(value bool) Options

	// Jitter gets whether to jitter between the current backoff and the next
	// backoff when moving to next attempt.
	Jitter() bool

	// SetRngFn sets the RngFn.
	SetRngFn(value RngFn) Options

	// RngFn returns the RngFn.
	RngFn() RngFn
}
