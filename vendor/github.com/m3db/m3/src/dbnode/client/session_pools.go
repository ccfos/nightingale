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

package client

import (
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/x/xpool"
	"github.com/m3db/m3/src/x/serialize"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
)

type sessionPools struct {
	context                     context.Pool
	id                          ident.Pool
	writeOperation              *writeOperationPool
	writeTaggedOperation        *writeTaggedOperationPool
	fetchBatchOp                *fetchBatchOpPool
	fetchBatchOpArrayArray      *fetchBatchOpArrayArrayPool
	fetchTaggedOp               fetchTaggedOpPool
	aggregateOp                 aggregateOpPool
	fetchState                  fetchStatePool
	multiReaderIteratorArray    encoding.MultiReaderIteratorArrayPool
	tagEncoder                  serialize.TagEncoderPool
	tagDecoder                  serialize.TagDecoderPool
	readerSliceOfSlicesIterator *readerSliceOfSlicesIteratorPool
	multiReaderIterator         encoding.MultiReaderIteratorPool
	seriesIterator              encoding.SeriesIteratorPool
	seriesIterators             encoding.MutableSeriesIteratorsPool
	writeAttempt                *writeAttemptPool
	writeState                  *writeStatePool
	fetchAttempt                *fetchAttemptPool
	fetchTaggedAttempt          fetchTaggedAttemptPool
	aggregateAttempt            aggregateAttemptPool
	checkedBytesWrapper         xpool.CheckedBytesWrapperPool
}

// NB: ensure sessionPools satisfies the fetchTaggedPools interface.
var _ fetchTaggedPools = sessionPools{}

func (s sessionPools) SeriesIterator() encoding.SeriesIteratorPool {
	return s.seriesIterator
}

func (s sessionPools) MultiReaderIteratorArray() encoding.MultiReaderIteratorArrayPool {
	return s.multiReaderIteratorArray
}

func (s sessionPools) ID() ident.Pool {
	return s.id
}

func (s sessionPools) TagDecoder() serialize.TagDecoderPool {
	return s.tagDecoder
}

func (s sessionPools) TagEncoder() serialize.TagEncoderPool {
	return s.tagEncoder
}

func (s sessionPools) ReaderSliceOfSlicesIterator() *readerSliceOfSlicesIteratorPool {
	return s.readerSliceOfSlicesIterator
}

func (s sessionPools) MultiReaderIterator() encoding.MultiReaderIteratorPool {
	return s.multiReaderIterator
}

func (s sessionPools) CheckedBytesWrapper() xpool.CheckedBytesWrapperPool {
	return s.checkedBytesWrapper
}

func (s sessionPools) MutableSeriesIterators() encoding.MutableSeriesIteratorsPool {
	return s.seriesIterators
}
