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
	"math"
	"math/rand"
	"time"

	"github.com/uber-go/tally"
)

const (
	defaultInitialBackoff = time.Second
	defaultBackoffFactor  = 2.0
	defaultMaxBackoff     = time.Duration(math.MaxInt64)
	defaultMaxRetries     = 2
	defaultForever        = false
	defaultJitter         = true
)

type options struct {
	scope          tally.Scope
	initialBackoff time.Duration
	backoffFactor  float64
	maxBackoff     time.Duration
	maxRetries     int
	forever        bool
	jitter         bool
	rngFn          RngFn
}

// NewOptions creates new retry options.
func NewOptions() Options {
	return &options{
		scope:          tally.NoopScope,
		initialBackoff: defaultInitialBackoff,
		backoffFactor:  defaultBackoffFactor,
		maxBackoff:     defaultMaxBackoff,
		maxRetries:     defaultMaxRetries,
		forever:        defaultForever,
		jitter:         defaultJitter,
		rngFn:          rand.Int63n,
	}
}

func (o *options) SetMetricsScope(value tally.Scope) Options {
	opts := *o
	opts.scope = value
	return &opts
}

func (o *options) MetricsScope() tally.Scope {
	return o.scope
}

func (o *options) SetInitialBackoff(value time.Duration) Options {
	opts := *o
	opts.initialBackoff = value
	return &opts
}

func (o *options) InitialBackoff() time.Duration {
	return o.initialBackoff
}

func (o *options) SetBackoffFactor(value float64) Options {
	opts := *o
	opts.backoffFactor = value
	return &opts
}

func (o *options) BackoffFactor() float64 {
	return o.backoffFactor
}

func (o *options) SetMaxBackoff(value time.Duration) Options {
	opts := *o
	opts.maxBackoff = value
	return &opts
}

func (o *options) MaxBackoff() time.Duration {
	return o.maxBackoff
}

func (o *options) SetMaxRetries(value int) Options {
	opts := *o
	opts.maxRetries = value
	return &opts
}

func (o *options) MaxRetries() int {
	return o.maxRetries
}

func (o *options) SetForever(value bool) Options {
	opts := *o
	opts.forever = value
	return &opts
}

func (o *options) Forever() bool {
	return o.forever
}

func (o *options) SetJitter(value bool) Options {
	opts := *o
	opts.jitter = value
	return &opts
}

func (o *options) Jitter() bool {
	return o.jitter
}

func (o *options) SetRngFn(value RngFn) Options {
	opts := *o
	opts.rngFn = value
	return &opts
}

func (o *options) RngFn() RngFn {
	return o.rngFn
}
