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

// Package clock implements facilities for working with wall clock time.
package clock

import (
	"time"
)

// NowFn is the function supplied to determine "now".
type NowFn func() time.Time

// Options represents the options for the clock.
type Options interface {
	// SetNowFn sets the NowFn.
	SetNowFn(value NowFn) Options

	// NowFn returns the NowFn.
	NowFn() NowFn

	// SetMaxPositiveSkew sets the maximum positive clock skew
	// with regard to a reference clock.
	SetMaxPositiveSkew(value time.Duration) Options

	// MaxPositiveSkew returns the maximum positive clock skew
	// with regard to a reference clock.
	MaxPositiveSkew() time.Duration

	// SetMaxNegativeSkew sets the maximum negative clock skew
	// with regard to a reference clock.
	SetMaxNegativeSkew(value time.Duration) Options

	// MaxNegativeSkew returns the maximum negative clock skew
	// with regard to a reference clock.
	MaxNegativeSkew() time.Duration
}

// ConditionFn specifies a predicate to check.
type ConditionFn func() bool

// WaitUntil returns true if the condition specified evaluated to
// true before the timeout, false otherwise.
func WaitUntil(fn ConditionFn, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
