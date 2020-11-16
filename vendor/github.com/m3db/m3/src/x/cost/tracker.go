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

import "go.uber.org/atomic"

type tracker struct {
	total *atomic.Float64
}

// NewTracker returns a new Tracker which maintains a simple running total of all the
// costs it has seen so far.
func NewTracker() Tracker         { return tracker{total: atomic.NewFloat64(0)} }
func (t tracker) Add(c Cost) Cost { return Cost(t.total.Add(float64(c))) }
func (t tracker) Current() Cost   { return Cost(t.total.Load()) }

type noopTracker struct{}

// NewNoopTracker returns a tracker which always always returns a cost of 0.
func NewNoopTracker() Tracker         { return noopTracker{} }
func (t noopTracker) Add(c Cost) Cost { return 0 }
func (t noopTracker) Current() Cost   { return 0 }
