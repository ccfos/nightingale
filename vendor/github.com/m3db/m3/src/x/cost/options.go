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
	"github.com/m3db/m3/src/cluster/kv/util"
	"github.com/m3db/m3/src/x/instrument"

	"github.com/uber-go/tally"
)

// LimitManagerOptions provides a set of options for a LimitManager.
type LimitManagerOptions interface {
	// SetDefaultLimit sets the default limit for the manager.
	SetDefaultLimit(val Limit) LimitManagerOptions

	// DefaultLimit returns the default limit for the manager.
	DefaultLimit() Limit

	// SetValidateLimitFn sets the validation function applied to updates to the cost limit.
	SetValidateLimitFn(val util.ValidateFn) LimitManagerOptions

	// ValidateLimitFn returns the validation function applied to updates to the cost limit.
	ValidateLimitFn() util.ValidateFn

	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(val instrument.Options) LimitManagerOptions

	// InstrumentOptions returns the instrument options.
	InstrumentOptions() instrument.Options
}

type limitManagerOptions struct {
	defaultLimit    Limit
	validateLimitFn util.ValidateFn
	iOpts           instrument.Options
}

// NewLimitManagerOptions returns a new set of LimitManager options.
func NewLimitManagerOptions() LimitManagerOptions {
	return &limitManagerOptions{
		defaultLimit: Limit{
			Threshold: maxCost,
			Enabled:   false,
		},
		iOpts: instrument.NewOptions(),
	}
}

func (o *limitManagerOptions) SetDefaultLimit(val Limit) LimitManagerOptions {
	opts := *o
	opts.defaultLimit = val
	return &opts
}

func (o *limitManagerOptions) DefaultLimit() Limit {
	return o.defaultLimit
}

func (o *limitManagerOptions) SetValidateLimitFn(val util.ValidateFn) LimitManagerOptions {
	opts := *o
	opts.validateLimitFn = val
	return &opts
}

func (o *limitManagerOptions) ValidateLimitFn() util.ValidateFn {
	return o.validateLimitFn
}

func (o *limitManagerOptions) SetInstrumentOptions(val instrument.Options) LimitManagerOptions {
	opts := *o
	opts.iOpts = val
	return &opts
}

func (o *limitManagerOptions) InstrumentOptions() instrument.Options {
	return o.iOpts
}

// EnforcerOptions provides a set of options for an enforcer.
type EnforcerOptions interface {
	// Reporter is the reporter which will be used on Enforcer events.
	Reporter() EnforcerReporter

	// SetReporter sets Reporter()
	SetReporter(r EnforcerReporter) EnforcerOptions

	// SetCostExceededMessage sets CostExceededMessage
	SetCostExceededMessage(val string) EnforcerOptions

	// CostExceededMessage returns the message appended to cost limit errors to provide
	// more context on the cost limit that was exceeded.
	CostExceededMessage() string
}

type enforcerOptions struct {
	msg      string
	buckets  tally.ValueBuckets
	reporter EnforcerReporter
	iOpts    instrument.Options
}

// NewEnforcerOptions returns a new set of enforcer options.
func NewEnforcerOptions() EnforcerOptions {
	return &enforcerOptions{
		buckets: tally.MustMakeExponentialValueBuckets(1, 2, 20),
		iOpts:   instrument.NewOptions(),
	}
}

func (o *enforcerOptions) Reporter() EnforcerReporter {
	return o.reporter
}

func (o *enforcerOptions) SetReporter(r EnforcerReporter) EnforcerOptions {
	opts := *o
	opts.reporter = r
	return &opts
}

func (o *enforcerOptions) SetCostExceededMessage(val string) EnforcerOptions {
	opts := *o
	opts.msg = val
	return &opts
}

func (o *enforcerOptions) CostExceededMessage() string {
	return o.msg
}

func (o *enforcerOptions) SetValueBuckets(val tally.ValueBuckets) EnforcerOptions {
	opts := *o
	opts.buckets = val
	return &opts
}

func (o *enforcerOptions) ValueBuckets() tally.ValueBuckets {
	return o.buckets
}

func (o *enforcerOptions) SetInstrumentOptions(val instrument.Options) EnforcerOptions {
	opts := *o
	opts.iOpts = val
	return &opts
}

func (o *enforcerOptions) InstrumentOptions() instrument.Options {
	return o.iOpts
}
