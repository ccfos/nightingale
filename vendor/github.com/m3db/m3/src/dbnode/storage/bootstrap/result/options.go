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

package result

import (
	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/dbnode/storage/series"
	"github.com/m3db/m3/src/x/instrument"
)

const (
	defaultNewBlocksLen = 2
)

type options struct {
	clockOpts                 clock.Options
	instrumentOpts            instrument.Options
	blockOpts                 block.Options
	newBlocksLen              int
	seriesCachePolicy         series.CachePolicy
	documentsBuilderAllocator DocumentsBuilderAllocator
}

// NewOptions creates new bootstrap options
func NewOptions() Options {
	return &options{
		clockOpts:                 clock.NewOptions(),
		instrumentOpts:            instrument.NewOptions(),
		blockOpts:                 block.NewOptions(),
		newBlocksLen:              defaultNewBlocksLen,
		seriesCachePolicy:         series.DefaultCachePolicy,
		documentsBuilderAllocator: NewDefaultDocumentsBuilderAllocator(),
	}
}

func (o *options) SetClockOptions(value clock.Options) Options {
	opts := *o
	opts.clockOpts = value
	return &opts
}

func (o *options) ClockOptions() clock.Options {
	return o.clockOpts
}

func (o *options) SetInstrumentOptions(value instrument.Options) Options {
	opts := *o
	opts.instrumentOpts = value
	return &opts
}

func (o *options) InstrumentOptions() instrument.Options {
	return o.instrumentOpts
}

func (o *options) SetDatabaseBlockOptions(value block.Options) Options {
	opts := *o
	opts.blockOpts = value
	return &opts
}

func (o *options) DatabaseBlockOptions() block.Options {
	return o.blockOpts
}

func (o *options) SetNewBlocksLen(value int) Options {
	opts := *o
	opts.newBlocksLen = value
	return &opts
}

func (o *options) NewBlocksLen() int {
	return o.newBlocksLen
}

func (o *options) SetSeriesCachePolicy(value series.CachePolicy) Options {
	opts := *o
	opts.seriesCachePolicy = value
	return &opts
}

func (o *options) SeriesCachePolicy() series.CachePolicy {
	return o.seriesCachePolicy
}

func (o *options) SetIndexDocumentsBuilderAllocator(value DocumentsBuilderAllocator) Options {
	opts := *o
	opts.documentsBuilderAllocator = value
	return &opts
}

func (o *options) IndexDocumentsBuilderAllocator() DocumentsBuilderAllocator {
	return o.documentsBuilderAllocator
}
