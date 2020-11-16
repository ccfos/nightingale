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

package series

import (
	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/retention"
	m3dbruntime "github.com/m3db/m3/src/dbnode/runtime"
	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/pool"
)

type options struct {
	clockOpts                     clock.Options
	instrumentOpts                instrument.Options
	retentionOpts                 retention.Options
	blockOpts                     block.Options
	cachePolicy                   CachePolicy
	contextPool                   context.Pool
	encoderPool                   encoding.EncoderPool
	multiReaderIteratorPool       encoding.MultiReaderIteratorPool
	fetchBlockMetadataResultsPool block.FetchBlockMetadataResultsPool
	identifierPool                ident.Pool
	stats                         Stats
	coldWritesEnabled             bool
	bufferBucketPool              *BufferBucketPool
	bufferBucketVersionsPool      *BufferBucketVersionsPool
	runtimeOptsMgr                m3dbruntime.OptionsManager
}

// NewOptions creates new database series options
func NewOptions() Options {
	bytesPool := pool.NewCheckedBytesPool([]pool.Bucket{
		pool.Bucket{Count: 4096, Capacity: 128},
	}, nil, func(s []pool.Bucket) pool.BytesPool {
		return pool.NewBytesPool(s, nil)
	})
	bytesPool.Init()
	iopts := instrument.NewOptions()
	return &options{
		clockOpts:                     clock.NewOptions(),
		instrumentOpts:                iopts,
		retentionOpts:                 retention.NewOptions(),
		blockOpts:                     block.NewOptions(),
		cachePolicy:                   DefaultCachePolicy,
		contextPool:                   context.NewPool(context.NewOptions()),
		encoderPool:                   encoding.NewEncoderPool(nil),
		multiReaderIteratorPool:       encoding.NewMultiReaderIteratorPool(nil),
		fetchBlockMetadataResultsPool: block.NewFetchBlockMetadataResultsPool(nil, 0),
		identifierPool:                ident.NewPool(bytesPool, ident.PoolOptions{}),
		stats:                         NewStats(iopts.MetricsScope()),
	}
}

func (o *options) Validate() error {
	if err := o.retentionOpts.Validate(); err != nil {
		return err
	}
	return ValidateCachePolicy(o.cachePolicy)
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

func (o *options) SetRetentionOptions(value retention.Options) Options {
	opts := *o
	opts.retentionOpts = value
	return &opts
}

func (o *options) RetentionOptions() retention.Options {
	return o.retentionOpts
}

func (o *options) SetDatabaseBlockOptions(value block.Options) Options {
	opts := *o
	opts.blockOpts = value
	return &opts
}

func (o *options) DatabaseBlockOptions() block.Options {
	return o.blockOpts
}

func (o *options) SetCachePolicy(value CachePolicy) Options {
	opts := *o
	opts.cachePolicy = value
	return &opts
}

func (o *options) CachePolicy() CachePolicy {
	return o.cachePolicy
}

func (o *options) SetContextPool(value context.Pool) Options {
	opts := *o
	opts.contextPool = value
	return &opts
}

func (o *options) ContextPool() context.Pool {
	return o.contextPool
}

func (o *options) SetEncoderPool(value encoding.EncoderPool) Options {
	opts := *o
	opts.encoderPool = value
	return &opts
}

func (o *options) EncoderPool() encoding.EncoderPool {
	return o.encoderPool
}

func (o *options) SetMultiReaderIteratorPool(value encoding.MultiReaderIteratorPool) Options {
	opts := *o
	opts.multiReaderIteratorPool = value
	return &opts
}

func (o *options) MultiReaderIteratorPool() encoding.MultiReaderIteratorPool {
	return o.multiReaderIteratorPool
}

func (o *options) SetFetchBlockMetadataResultsPool(value block.FetchBlockMetadataResultsPool) Options {
	opts := *o
	opts.fetchBlockMetadataResultsPool = value
	return &opts
}

func (o *options) FetchBlockMetadataResultsPool() block.FetchBlockMetadataResultsPool {
	return o.fetchBlockMetadataResultsPool
}

func (o *options) SetIdentifierPool(value ident.Pool) Options {
	opts := *o
	opts.identifierPool = value
	return &opts
}

func (o *options) IdentifierPool() ident.Pool {
	return o.identifierPool
}

func (o *options) SetStats(value Stats) Options {
	opts := *o
	opts.stats = value
	return &opts
}

func (o *options) Stats() Stats {
	return o.stats
}

func (o *options) SetColdWritesEnabled(value bool) Options {
	opts := *o
	opts.coldWritesEnabled = value
	return &opts
}

func (o *options) ColdWritesEnabled() bool {
	return o.coldWritesEnabled
}

func (o *options) SetBufferBucketVersionsPool(value *BufferBucketVersionsPool) Options {
	opts := *o
	opts.bufferBucketVersionsPool = value
	return &opts
}

func (o *options) BufferBucketVersionsPool() *BufferBucketVersionsPool {
	return o.bufferBucketVersionsPool
}

func (o *options) SetBufferBucketPool(value *BufferBucketPool) Options {
	opts := *o
	opts.bufferBucketPool = value
	return &opts
}

func (o *options) BufferBucketPool() *BufferBucketPool {
	return o.bufferBucketPool
}

func (o *options) SetRuntimeOptionsManager(value m3dbruntime.OptionsManager) Options {
	opts := *o
	opts.runtimeOptsMgr = value
	return &opts
}

func (o *options) RuntimeOptionsManager() m3dbruntime.OptionsManager {
	return o.runtimeOptsMgr
}
