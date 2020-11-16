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

package ratelimit

const (
	// defaultLimitEnabled determines whether rate limiting is enabled
	defaultLimitEnabled = false

	// defaultLimitCheckEvery is the default frequency for checking
	// whether the limit is exceeded
	defaultLimitCheckEvery = 128

	// defaultLimitMbps is the default limit in Mb/s
	defaultLimitMbps = 100.0
)

type options struct {
	limitEnabled    bool
	limitMbps       float64
	limitCheckEvery int
}

// NewOptions creates a new rate limit options
func NewOptions() Options {
	return &options{
		limitEnabled:    defaultLimitEnabled,
		limitMbps:       defaultLimitMbps,
		limitCheckEvery: defaultLimitCheckEvery,
	}
}

func (o *options) SetLimitEnabled(value bool) Options {
	opts := *o
	opts.limitEnabled = value
	return &opts
}

func (o *options) LimitEnabled() bool {
	return o.limitEnabled
}

func (o *options) SetLimitMbps(value float64) Options {
	opts := *o
	opts.limitMbps = value
	return &opts
}

func (o *options) LimitMbps() float64 {
	return o.limitMbps
}

func (o *options) SetLimitCheckEvery(value int) Options {
	opts := *o
	opts.limitCheckEvery = value
	return &opts
}

func (o *options) LimitCheckEvery() int {
	return o.limitCheckEvery
}
