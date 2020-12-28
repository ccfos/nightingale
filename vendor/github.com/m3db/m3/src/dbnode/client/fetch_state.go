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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/dbnode/topology"
	"github.com/m3db/m3/src/dbnode/x/xpool"
	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/serialize"
)

type fetchStateType byte

const (
	fetchTaggedFetchState fetchStateType = iota
	aggregateFetchState
)

const (
	maxUint = ^uint(0)
	maxInt  = int(maxUint >> 1)
)

var (
	errFetchStateStillProcessing = errors.New("[invariant violated] fetch " +
		"state is still processing, unable to create response")
)

type fetchState struct {
	sync.Cond
	sync.Mutex
	refCounter

	fetchTaggedOp *fetchTaggedOp
	aggregateOp   *aggregateOp

	nsID                 ident.ID
	tagResultAccumulator fetchTaggedResultAccumulator
	err                  error

	pool fetchStatePool

	// NB: stateType determines which type of op this fetchState
	// is used for - fetchTagged or Aggregate.
	stateType fetchStateType

	done bool
}

func newFetchState(pool fetchStatePool) *fetchState {
	f := &fetchState{
		tagResultAccumulator: newFetchTaggedResultAccumulator(),
		pool:                 pool,
	}
	f.destructorFn = f.close // Set refCounter completion as close
	f.L = f                  // Set the embedded condition locker to the embedded mutex
	return f
}

func (f *fetchState) close() {
	if f.nsID != nil {
		f.nsID.Finalize()
		f.nsID = nil
	}
	if f.fetchTaggedOp != nil {
		f.fetchTaggedOp.decRef()
		f.fetchTaggedOp = nil
	}
	if f.aggregateOp != nil {
		f.aggregateOp.decRef()
		f.aggregateOp = nil
	}
	f.err = nil
	f.done = false
	f.tagResultAccumulator.Clear()

	if f.pool == nil {
		return
	}
	f.pool.Put(f)
}

func (f *fetchState) ResetFetchTagged(
	startTime time.Time,
	endTime time.Time,
	op *fetchTaggedOp, topoMap topology.Map,
	majority int,
	consistencyLevel topology.ReadConsistencyLevel,
) {
	op.incRef() // take a reference to the provided op
	f.fetchTaggedOp = op
	f.stateType = fetchTaggedFetchState
	f.tagResultAccumulator.Reset(startTime, endTime, topoMap, majority, consistencyLevel)
}

func (f *fetchState) ResetAggregate(
	startTime time.Time,
	endTime time.Time,
	op *aggregateOp, topoMap topology.Map,
	majority int,
	consistencyLevel topology.ReadConsistencyLevel,
) {
	op.incRef() // take a reference to the provided op
	f.aggregateOp = op
	f.stateType = aggregateFetchState
	f.tagResultAccumulator.Reset(startTime, endTime, topoMap, majority, consistencyLevel)
}

func (f *fetchState) completionFn(
	result interface{},
	resultErr error,
) {
	if IsBadRequestError(resultErr) {
		// Wrap with invalid params and non-retryable so it is
		// not retried.
		resultErr = xerrors.NewInvalidParamsError(resultErr)
		resultErr = xerrors.NewNonRetryableError(resultErr)
	}

	f.Lock()
	defer func() {
		f.Unlock()
		f.decRef() // release ref held onto by the hostQueue (via op.completionFn)
	}()

	if f.done {
		// i.e. we've already failed, no need to continue processing any additional
		// responses we receive
		return
	}

	var (
		done bool
		err  error
	)
	switch r := result.(type) {
	case fetchTaggedResultAccumulatorOpts:
		done, err = f.tagResultAccumulator.AddFetchTaggedResponse(r, resultErr)
	case aggregateResultAccumulatorOpts:
		done, err = f.tagResultAccumulator.AddAggregateResponse(r, resultErr)
	default:
		// should never happen
		done = true
		err = fmt.Errorf(
			"[invariant violated] expected result to be one of %v, received: %v",
			[]string{"fetchTaggedResultAccumulatorOpts", "aggregateResultAccumulatorOpts"},
			result)
	}

	if done {
		f.markDoneWithLock(err)
	}
}

func (f *fetchState) markDoneWithLock(err error) {
	f.done = true
	f.err = err
	f.Signal()
}

func (f *fetchState) asTaggedIDsIterator(
	pools fetchTaggedPools,
) (TaggedIDsIterator, FetchResponseMetadata, error) {
	f.Lock()
	defer f.Unlock()

	if expected := fetchTaggedFetchState; f.stateType != expected {
		return nil, FetchResponseMetadata{},
			fmt.Errorf("unexpected fetch state: expected=%v, actual=%v",
				expected, f.stateType)
	}

	if !f.done {
		return nil, FetchResponseMetadata{}, errFetchStateStillProcessing
	}

	if err := f.err; err != nil {
		return nil, FetchResponseMetadata{}, err
	}

	limit := f.fetchTaggedOp.requestLimit(maxInt)
	return f.tagResultAccumulator.AsTaggedIDsIterator(limit, pools)
}

func (f *fetchState) asEncodingSeriesIterators(
	pools fetchTaggedPools,
	descr namespace.SchemaDescr,
	opts index.IterationOptions,
) (encoding.SeriesIterators, FetchResponseMetadata, error) {
	f.Lock()
	defer f.Unlock()

	if expected := fetchTaggedFetchState; f.stateType != expected {
		return nil, FetchResponseMetadata{},
			fmt.Errorf("unexpected fetch state: expected=%v, actual=%v",
				expected, f.stateType)
	}

	if !f.done {
		return nil, FetchResponseMetadata{}, errFetchStateStillProcessing
	}

	if err := f.err; err != nil {
		return nil, FetchResponseMetadata{}, err
	}

	limit := f.fetchTaggedOp.requestLimit(maxInt)
	return f.tagResultAccumulator.AsEncodingSeriesIterators(limit, pools, descr, opts)
}

func (f *fetchState) asAggregatedTagsIterator(pools fetchTaggedPools) (AggregatedTagsIterator, FetchResponseMetadata, error) {
	f.Lock()
	defer f.Unlock()

	if expected := aggregateFetchState; f.stateType != expected {
		return nil, FetchResponseMetadata{},
			fmt.Errorf("unexpected fetch state: expected=%v, actual=%v",
				expected, f.stateType)
	}

	if !f.done {
		return nil, FetchResponseMetadata{}, errFetchStateStillProcessing
	}

	if err := f.err; err != nil {
		return nil, FetchResponseMetadata{}, err
	}

	limit := f.aggregateOp.requestLimit(maxInt)
	return f.tagResultAccumulator.AsAggregatedTagsIterator(limit, pools)
}

// NB(prateek): this is backed by the sessionPools struct, but we're restricting it to a narrow
// interface to force the fetchTagged code-paths to be explicit about the pools they need access
// to. The alternative is to either expose the sessionPools struct (which is a worse abstraction),
// or make a new concrete implemtation (which requires an extra alloc). Chosing the best of the
// three options and leaving as the interface below.
type fetchTaggedPools interface {
	MultiReaderIteratorArray() encoding.MultiReaderIteratorArrayPool
	MultiReaderIterator() encoding.MultiReaderIteratorPool
	MutableSeriesIterators() encoding.MutableSeriesIteratorsPool
	SeriesIterator() encoding.SeriesIteratorPool
	CheckedBytesWrapper() xpool.CheckedBytesWrapperPool
	ID() ident.Pool
	ReaderSliceOfSlicesIterator() *readerSliceOfSlicesIteratorPool
	TagDecoder() serialize.TagDecoderPool
}
