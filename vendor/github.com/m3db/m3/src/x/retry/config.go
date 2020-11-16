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
	"time"

	"github.com/uber-go/tally"
)

// Configuration configures options for retry attempts.
type Configuration struct {
	// Initial retry backoff.
	InitialBackoff time.Duration `yaml:"initialBackoff" validate:"min=0"`

	// Backoff factor for exponential backoff.
	BackoffFactor float64 `yaml:"backoffFactor" validate:"min=0"`

	// Maximum backoff time.
	MaxBackoff time.Duration `yaml:"maxBackoff" validate:"min=0"`

	// Maximum number of retry attempts.
	MaxRetries int `yaml:"maxRetries"`

	// Whether to retry forever until either the attempt succeeds,
	// or the retry condition becomes false.
	Forever *bool `yaml:"forever"`

	// Whether jittering is applied during retries.
	Jitter *bool `yaml:"jitter"`
}

// NewOptions creates a new retry options based on the configuration.
func (c Configuration) NewOptions(scope tally.Scope) Options {
	opts := NewOptions().SetMetricsScope(scope)
	if c.InitialBackoff != 0 {
		opts = opts.SetInitialBackoff(c.InitialBackoff)
	}
	if c.BackoffFactor != 0.0 {
		opts = opts.SetBackoffFactor(c.BackoffFactor)
	}
	if c.MaxBackoff != 0 {
		opts = opts.SetMaxBackoff(c.MaxBackoff)
	}
	if c.MaxRetries != 0 {
		opts = opts.SetMaxRetries(c.MaxRetries)
	}
	if c.Forever != nil {
		opts = opts.SetForever(*c.Forever)
	}
	if c.Jitter != nil {
		opts = opts.SetJitter(*c.Jitter)
	}

	return opts
}

// NewRetrier creates a new retrier based on the configuration.
func (c Configuration) NewRetrier(scope tally.Scope) Retrier {
	return NewRetrier(c.NewOptions(scope))
}
