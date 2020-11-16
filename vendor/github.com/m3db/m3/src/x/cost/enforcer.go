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
)

const (
	defaultCostExceededErrorFmt = "limit reached (current = %v, limit = %v)"
)

var (
	noopManager = NewStaticLimitManager(
		NewLimitManagerOptions().
			SetDefaultLimit(Limit{
				Threshold: maxCost,
				Enabled:   false,
			},
			),
	)
	noopEnforcer = NewEnforcer(noopManager, NewNoopTracker(), nil)
)

// Report is a report on the cost limits of an enforcer.
type Report struct {
	Cost
	Error error
}

// enforcer enforces cost limits for operations.
type enforcer struct {
	LimitManager
	tracker Tracker

	costMsg string
	metrics EnforcerReporter
}

// NewEnforcer returns a new enforcer for cost limits.
func NewEnforcer(m LimitManager, t Tracker, opts EnforcerOptions) Enforcer {
	if opts == nil {
		opts = NewEnforcerOptions()
	}

	reporter := opts.Reporter()
	if reporter == nil {
		reporter = NoopEnforcerReporter()
	}

	return &enforcer{
		LimitManager: m,
		tracker:      t,
		costMsg:      opts.CostExceededMessage(),
		metrics:      reporter,
	}
}

func (e *enforcer) Reporter() EnforcerReporter {
	return e.metrics
}

// Add adds the cost of an operation to the enforcer's current total. If the operation exceeds
// the enforcer's limit the enforcer will return a CostLimit error in addition to the new total.
func (e *enforcer) Add(cost Cost) Report {
	e.metrics.ReportCost(cost)
	current := e.tracker.Add(cost)
	e.metrics.ReportCurrent(current)

	limit := e.Limit()
	overLimit := e.checkLimit(current, limit)

	if overLimit != nil {
		// Emit metrics on number of operations that are over the limit even when not enabled.
		e.metrics.ReportOverLimit(limit.Enabled)
	}

	return Report{
		Cost:  current,
		Error: overLimit,
	}
}

// State returns the current state of the enforcer.
func (e *enforcer) State() (Report, Limit) {
	cost := e.tracker.Current()
	l := e.Limit()
	err := e.checkLimit(cost, l)
	r := Report{
		Cost:  cost,
		Error: err,
	}
	return r, l
}

// Clone clones the current enforcer. The new enforcer uses the same Estimator and LimitManager
// as e buts its Tracker is independent.
func (e *enforcer) Clone() Enforcer {
	return &enforcer{
		LimitManager: e.LimitManager,
		tracker:      NewTracker(),
		costMsg:      e.costMsg,
		metrics:      e.metrics,
	}
}

func (e *enforcer) checkLimit(cost Cost, limit Limit) error {
	if cost < limit.Threshold || !limit.Enabled {
		return nil
	}

	return NewCostExceededError(e.costMsg, cost, limit.Threshold)
}

type costExceededError struct {
	Threshold Cost
	Current   Cost
	CustomMsg string
}

func (ce costExceededError) Error() string {
	baseMsg := fmt.Sprintf(
		defaultCostExceededErrorFmt,
		float64(ce.Current),
		float64(ce.Threshold),
	)
	if ce.CustomMsg == "" {
		return baseMsg
	}

	return fmt.Sprintf("%s: %s", ce.CustomMsg, baseMsg)
}

// NewCostExceededError returns an error for going over an Enforcer's limit.
func NewCostExceededError(customMessage string, cost Cost, threshold Cost) error {
	return costExceededError{
		CustomMsg: customMessage,
		Current:   cost,
		Threshold: threshold,
	}
}

// NoopEnforcer returns a new enforcer that always returns a current cost of 0 and
//  is always disabled.
func NoopEnforcer() Enforcer {
	return noopEnforcer
}

type noopEnforcerReporter struct{}

func (noopEnforcerReporter) ReportCost(c Cost)            {}
func (noopEnforcerReporter) ReportCurrent(c Cost)         {}
func (noopEnforcerReporter) ReportOverLimit(enabled bool) {}

// NoopEnforcerReporter returns an EnforcerReporter which does nothing on all events.
func NoopEnforcerReporter() EnforcerReporter {
	return noopEnforcerReporter{}
}
