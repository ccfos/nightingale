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

package pool

import "github.com/m3db/m3/src/x/instrument"

const (
	defaultSize                = 4096
	defaultRefillLowWatermark  = 0.0
	defaultRefillHighWatermark = 0.0
)

type objectPoolOptions struct {
	size                int
	refillLowWatermark  float64
	refillHighWatermark float64
	instrumentOpts      instrument.Options
	onPoolAccessErrorFn OnPoolAccessErrorFn
}

// NewObjectPoolOptions creates a new set of object pool options
func NewObjectPoolOptions() ObjectPoolOptions {
	return &objectPoolOptions{
		size:                defaultSize,
		refillLowWatermark:  defaultRefillLowWatermark,
		refillHighWatermark: defaultRefillHighWatermark,
		instrumentOpts:      instrument.NewOptions(),
		onPoolAccessErrorFn: func(err error) { panic(err) },
	}
}

func (o *objectPoolOptions) SetSize(value int) ObjectPoolOptions {
	opts := *o
	opts.size = value
	return &opts
}

func (o *objectPoolOptions) Size() int {
	return o.size
}

func (o *objectPoolOptions) SetRefillLowWatermark(value float64) ObjectPoolOptions {
	opts := *o
	opts.refillLowWatermark = value
	return &opts
}

func (o *objectPoolOptions) RefillLowWatermark() float64 {
	return o.refillLowWatermark
}

func (o *objectPoolOptions) SetRefillHighWatermark(value float64) ObjectPoolOptions {
	opts := *o
	opts.refillHighWatermark = value
	return &opts
}

func (o *objectPoolOptions) RefillHighWatermark() float64 {
	return o.refillHighWatermark
}

func (o *objectPoolOptions) SetInstrumentOptions(value instrument.Options) ObjectPoolOptions {
	opts := *o
	opts.instrumentOpts = value
	return &opts
}

func (o *objectPoolOptions) InstrumentOptions() instrument.Options {
	return o.instrumentOpts
}

func (o *objectPoolOptions) SetOnPoolAccessErrorFn(value OnPoolAccessErrorFn) ObjectPoolOptions {
	opts := *o
	opts.onPoolAccessErrorFn = value
	return &opts
}

func (o *objectPoolOptions) OnPoolAccessErrorFn() OnPoolAccessErrorFn {
	return o.onPoolAccessErrorFn
}
