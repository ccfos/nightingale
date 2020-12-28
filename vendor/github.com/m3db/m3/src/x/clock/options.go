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

package clock

import (
	"time"
)

type options struct {
	nowFn           NowFn
	maxPositiveSkew time.Duration
	maxNegativeSkew time.Duration
}

// NewOptions creates new clock options.
func NewOptions() Options {
	return &options{
		nowFn: time.Now,
	}
}

func (o *options) SetNowFn(value NowFn) Options {
	opts := *o
	opts.nowFn = value
	return &opts
}

func (o *options) NowFn() NowFn {
	return o.nowFn
}

func (o *options) SetMaxPositiveSkew(value time.Duration) Options {
	opts := *o
	opts.maxPositiveSkew = value
	return &opts
}

func (o *options) MaxPositiveSkew() time.Duration {
	return o.maxPositiveSkew
}

func (o *options) SetMaxNegativeSkew(value time.Duration) Options {
	opts := *o
	opts.maxNegativeSkew = value
	return &opts
}

func (o *options) MaxNegativeSkew() time.Duration {
	return o.maxNegativeSkew
}
