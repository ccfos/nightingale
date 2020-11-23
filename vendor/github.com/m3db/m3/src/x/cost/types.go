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

// Package cost is a library providing utilities for estimating the cost of operations
// and enforcing limits on them. The primary type of interest is Enforcer, which tracks the cost of operations,
// and errors when that cost goes over a particular limit.
package cost

import (
	"math"
)

// Cost represents the cost of an operation.
type Cost float64

const (
	// maxCost is the maximum cost of an operation.
	maxCost = Cost(math.MaxFloat64)
)

// Limit encapulates the configuration of a cost limit for an operation.
type Limit struct {
	Threshold Cost
	Enabled   bool
}

// LimitManager manages configuration of a cost limit for an operation.
type LimitManager interface {
	// Limit returns the current cost limit for an operation.
	Limit() Limit

	// Report reports metrics on the state of the manager.
	Report()

	// Close closes the manager.
	Close()
}

// Tracker tracks the cost of operations seen so far.
type Tracker interface {
	// Add adds c to the tracker's current cost total and returns the new total.
	Add(c Cost) Cost

	// Current returns the tracker's current cost total.
	Current() Cost
}

// Enforcer instances enforce cost limits for operations.
type Enforcer interface {
	Add(op Cost) Report
	State() (Report, Limit)
	Limit() Limit
	Clone() Enforcer
	Reporter() EnforcerReporter
}

// An EnforcerReporter is a listener for Enforcer events.
type EnforcerReporter interface {
	// ReportCost is called on every call to Enforcer#Add with the added cost
	ReportCost(c Cost)

	// ReportCurrent reports the current total on every call to Enforcer#Add
	ReportCurrent(c Cost)

	// ReportOverLimit is called every time an enforcer goes over its limit. enabled is true if the limit manager
	// says the limit is currently enabled.
	ReportOverLimit(enabled bool)
}
