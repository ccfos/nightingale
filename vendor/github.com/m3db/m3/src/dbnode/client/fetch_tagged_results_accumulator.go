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
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/m3db/m3/src/cluster/shard"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/generated/thrift/rpc"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/dbnode/topology"
	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/ident"
	xtime "github.com/m3db/m3/src/x/time"
)

type fetchTaggedResultAccumulatorOpts struct {
	host     topology.Host
	response *rpc.FetchTaggedResult_
}

type aggregateResultAccumulatorOpts struct {
	host     topology.Host
	response *rpc.AggregateQueryRawResult_
}

func newFetchTaggedResultAccumulator() fetchTaggedResultAccumulator {
	accum := fetchTaggedResultAccumulator{
		calcTransport: &calcTransport{},
	}
	accum.Clear()
	return accum
}

type fetchTaggedResultAccumulator struct {
	// NB(prateek): a fetchTagged request requires we fan out to each shard in the
	// topology. As a result, we track the response consistency per shard.
	// Length of this slice == 1 + max shard id in topology
	shardConsistencyResults []fetchTaggedShardConsistencyResult
	numHostsPending         int32
	numShardsPending        int32

	errors         []error
	fetchResponses fetchTaggedIDResults
	aggResponses   aggregateResults
	exhaustive     bool

	startTime        time.Time
	endTime          time.Time
	majority         int
	consistencyLevel topology.ReadConsistencyLevel
	topoMap          topology.Map

	calcTransport *calcTransport
}

type fetchTaggedShardConsistencyResult struct {
	enqueued int8
	success  int8
	errors   int8
	done     bool
}

type fetchTaggedResultAccumulatorStats struct {
	exhaustive            bool
	responseReplicas      int
	responseBytesEstimate int
}

func (rs fetchTaggedShardConsistencyResult) pending() int32 {
	return int32(rs.enqueued - (rs.success + rs.errors))
}

func (accum *fetchTaggedResultAccumulator) AddFetchTaggedResponse(
	opts fetchTaggedResultAccumulatorOpts,
	resultErr error,
) (bool, error) {
	if opts.response != nil && resultErr == nil {
		accum.exhaustive = accum.exhaustive && opts.response.Exhaustive
		for _, elem := range opts.response.Elements {
			accum.fetchResponses = append(accum.fetchResponses, elem)
		}
	}

	// NB(r): Write the response to calculate transport to work out length.
	opts.response.Write(accum.calcTransport)

	return accum.accumulatedResult(opts.host, resultErr)
}

func (accum *fetchTaggedResultAccumulator) AddAggregateResponse(
	opts aggregateResultAccumulatorOpts,
	resultErr error,
) (bool, error) {
	if opts.response != nil && resultErr == nil {
		accum.exhaustive = accum.exhaustive && opts.response.Exhaustive
		for _, elem := range opts.response.Results {
			accum.aggResponses = append(accum.aggResponses, elem)
		}
	}

	// NB(r): Write the response to calculate transport to work out length.
	opts.response.Write(accum.calcTransport)

	return accum.accumulatedResult(opts.host, resultErr)
}

func (accum *fetchTaggedResultAccumulator) accumulatedResult(
	host topology.Host,
	resultErr error,
) (bool, error) {
	if host == nil {
		// should never happen, guarding against incompatible changes to the `client` package.
		doneAccumulating := true
		err := fmt.Errorf("[invariant violated] nil host in fetchState completionFn")
		return doneAccumulating, xerrors.NewNonRetryableError(err)
	}

	hostShardSet, ok := accum.topoMap.LookupHostShardSet(host.ID())
	if !ok {
		// should never happen, as we've taken a reference to the
		// topology when beginning the request, and the var is immutable.
		doneAccumulating := true
		err := fmt.Errorf(
			"[invariant violated] missing host shard in fetchState completionFn: %s", host.ID())
		return doneAccumulating, xerrors.NewNonRetryableError(err)
	}

	accum.numHostsPending--
	if resultErr != nil {
		accum.errors = append(accum.errors, xerrors.NewRenamedError(resultErr,
			fmt.Errorf("error fetching tagged from host %s: %v", host.ID(), resultErr)))
	}

	// FOLLOWUP(prateek): once we transmit the shards successfully satisfied by a response, the
	// for loop below needs to be updated to filter the `hostShardSet` to only include those
	// in the response. More details in https://github.com/m3db/m3/src/dbnode/issues/550.
	for _, hs := range hostShardSet.ShardSet().All() {
		shardID := int(hs.ID())
		shardResult := accum.shardConsistencyResults[shardID]
		if shardResult.done {
			continue // already been marked done, don't need to do anything for this shard
		}

		if hs.State() != shard.Available {
			// Currently, we only accept responses from shard's which are available
			// NB: as a possible enhancement, we could accept a response from
			// a shard that's not available if we tracked response pairs from
			// a LEAVING+INITIALIZING shard; this would help during node replaces.
			shardResult.errors++
		} else if resultErr == nil {
			shardResult.success++
		} else {
			shardResult.errors++
		}

		pending := shardResult.pending()
		if topology.ReadConsistencyTermination(accum.consistencyLevel, int32(accum.majority), pending, int32(shardResult.success)) {
			shardResult.done = true
			if topology.ReadConsistencyAchieved(accum.consistencyLevel, accum.majority, int(shardResult.enqueued), int(shardResult.success)) {
				accum.numShardsPending--
			}
			// NB(prateek): if !ReadConsistencyAchieved, we have sufficient information to fail the entire request, because we
			// will never be able to satisfy the consistency requirement on the current shard. We explicitly chose not to,
			// instead waiting till all the hosts return a response. This is to reduce the load we would put on the cluster
			// due to retries.
		}

		// update value in slice
		accum.shardConsistencyResults[shardID] = shardResult
	}

	// success case, sufficient responses for each shard
	if accum.numShardsPending == 0 {
		doneAccumulating := true
		return doneAccumulating, nil
	}

	// failure case - we've received all responses but still weren't able to satisfy
	// all shards, so we need to fail
	if accum.numHostsPending == 0 && accum.numShardsPending != 0 {
		doneAccumulating := true
		// NB(r): Use new renamed error to keep the underlying error
		// (invalid/retryable) type.
		err := fmt.Errorf("unable to satisfy consistency requirements: shards=%d, err=%v",
			accum.numShardsPending, accum.errors)
		for i := range accum.errors {
			if IsBadRequestError(accum.errors[i]) {
				err = xerrors.NewInvalidParamsError(err)
				err = xerrors.NewNonRetryableError(err)
				break
			}
		}
		return doneAccumulating, err
	}

	doneAccumulating := false
	return doneAccumulating, nil
}

func (accum *fetchTaggedResultAccumulator) Clear() {
	for i := range accum.fetchResponses {
		accum.fetchResponses[i] = nil
	}
	accum.fetchResponses = accum.fetchResponses[:0]
	for i := range accum.aggResponses {
		accum.aggResponses[i] = nil
	}
	accum.aggResponses = accum.aggResponses[:0]
	for i := range accum.errors {
		accum.errors[i] = nil
	}
	accum.errors = accum.errors[:0]
	accum.shardConsistencyResults = accum.shardConsistencyResults[:0]
	accum.consistencyLevel = topology.ReadConsistencyLevelNone
	accum.majority, accum.numHostsPending, accum.numShardsPending = 0, 0, 0
	accum.startTime, accum.endTime = time.Time{}, time.Time{}
	accum.topoMap = nil
	accum.exhaustive = true
	accum.calcTransport.Reset()
}

func (accum *fetchTaggedResultAccumulator) Reset(
	startTime time.Time,
	endTime time.Time,
	topoMap topology.Map,
	majority int,
	consistencyLevel topology.ReadConsistencyLevel,
) {
	accum.exhaustive = true
	accum.startTime = startTime
	accum.endTime = endTime
	accum.topoMap = topoMap
	accum.majority = majority
	accum.consistencyLevel = consistencyLevel
	accum.numHostsPending = int32(topoMap.HostsLen())
	accum.numShardsPending = int32(len(topoMap.ShardSet().All()))

	// expand shardResults as much as necessary
	targetLen := 1 + int(topoMap.ShardSet().Max())
	accum.shardConsistencyResults = fetchTaggedShardConsistencyResults(
		accum.shardConsistencyResults).initialize(targetLen)
	// initialize shardResults based on current topology
	for _, hss := range topoMap.HostShardSets() {
		for _, hShard := range hss.ShardSet().All() {
			id := int(hShard.ID())
			accum.shardConsistencyResults[id].enqueued++
		}
	}

	accum.calcTransport.Reset()
}

func (accum *fetchTaggedResultAccumulator) sliceResponsesAsSeriesIter(
	pools fetchTaggedPools,
	elems fetchTaggedIDResults,
	descr namespace.SchemaDescr,
	opts index.IterationOptions,
) encoding.SeriesIterator {
	numElems := len(elems)
	iters := pools.MultiReaderIteratorArray().Get(numElems)[:numElems]
	for idx, elem := range elems {
		slicesIter := pools.ReaderSliceOfSlicesIterator().Get()
		slicesIter.Reset(elem.Segments)
		multiIter := pools.MultiReaderIterator().Get()
		multiIter.ResetSliceOfSlices(slicesIter, descr)
		iters[idx] = multiIter
	}

	// pick the first element as they all have identical ids/tags
	// NB: safe to assume this element exists as it's only called within
	// a forEachID lambda, which provides the guarantee that len(elems) != 0
	elem := elems[0]

	encodedTags := pools.CheckedBytesWrapper().Get(elem.EncodedTags)
	decoder := pools.TagDecoder().Get()
	decoder.Reset(encodedTags)

	tsID := pools.CheckedBytesWrapper().Get(elem.ID)
	nsID := pools.CheckedBytesWrapper().Get(elem.NameSpace)
	seriesIter := pools.SeriesIterator().Get()
	seriesIter.Reset(encoding.SeriesIteratorOptions{
		ID:                         pools.ID().BinaryID(tsID),
		Namespace:                  pools.ID().BinaryID(nsID),
		Tags:                       decoder,
		StartInclusive:             xtime.ToUnixNano(accum.startTime),
		EndExclusive:               xtime.ToUnixNano(accum.endTime),
		Replicas:                   iters,
		SeriesIteratorConsolidator: opts.SeriesIteratorConsolidator,
	})

	return seriesIter
}

func (accum *fetchTaggedResultAccumulator) AsEncodingSeriesIterators(
	limit int, pools fetchTaggedPools,
	descr namespace.SchemaDescr, opts index.IterationOptions,
) (encoding.SeriesIterators, FetchResponseMetadata, error) {
	results := fetchTaggedIDResultsSortedByID(accum.fetchResponses)
	sort.Sort(results)
	accum.fetchResponses = fetchTaggedIDResults(results)

	numElements := 0
	accum.fetchResponses.forEachID(func(_ fetchTaggedIDResults, _ bool) bool {
		numElements++
		return numElements < limit
	})

	result := pools.MutableSeriesIterators().Get(numElements)
	result.Reset(numElements)
	count := 0
	moreElems := false
	accum.fetchResponses.forEachID(func(elems fetchTaggedIDResults, hasMore bool) bool {
		seriesIter := accum.sliceResponsesAsSeriesIter(pools, elems, descr, opts)
		result.SetAt(count, seriesIter)
		count++
		moreElems = hasMore
		return count < limit
	})

	exhaustive := accum.exhaustive && count <= limit && !moreElems
	return result, FetchResponseMetadata{
		Exhaustive:         exhaustive,
		Responses:          len(accum.fetchResponses),
		EstimateTotalBytes: accum.calcTransport.GetSize(),
	}, nil
}

func (accum *fetchTaggedResultAccumulator) AsTaggedIDsIterator(
	limit int,
	pools fetchTaggedPools,
) (TaggedIDsIterator, FetchResponseMetadata, error) {
	var (
		iter      = newTaggedIDsIterator(pools)
		count     = 0
		moreElems = false
	)
	results := fetchTaggedIDResultsSortedByID(accum.fetchResponses)
	sort.Sort(results)
	accum.fetchResponses = fetchTaggedIDResults(results)
	accum.fetchResponses.forEachID(func(elems fetchTaggedIDResults, hasMore bool) bool {
		iter.addBacking(elems[0].NameSpace, elems[0].ID, elems[0].EncodedTags)
		count++
		moreElems = hasMore
		return count < limit
	})

	exhaustive := accum.exhaustive && count <= limit && !moreElems
	return iter, FetchResponseMetadata{
		Exhaustive:         exhaustive,
		Responses:          len(accum.aggResponses),
		EstimateTotalBytes: accum.calcTransport.GetSize(),
	}, nil
}

func (accum *fetchTaggedResultAccumulator) AsAggregatedTagsIterator(
	limit int,
	pools fetchTaggedPools,
) (AggregatedTagsIterator, FetchResponseMetadata, error) {
	var (
		iter      = newAggregateTagsIterator(pools)
		count     = 0
		moreElems = false
	)
	results := aggregateResultsSortedByTag(accum.aggResponses)
	sort.Sort(results)

	var tempValues []ident.ID

	accum.aggResponses = aggregateResults(results)
	accum.aggResponses.forEachTag(func(elems aggregateResults, hasMore bool) bool {
		// NB(r): Guaranteed to only get called for results that actually have tags.
		tagResult := iter.addTag(elems[0].TagName)

		for _, tagResponse := range elems {
			// Sort values before adding to final result.
			values := aggregateValueResultsSortedByValue(tagResponse.TagValues)
			sort.Sort(values)

			if len(tagResult.tagValues) == 0 {
				// If first response with values from host then add in order blindly.
				for _, tagValueResponse := range values {
					elem := ident.BytesID(tagValueResponse.TagValue)
					tagResult.tagValues = append(tagResult.tagValues, elem)
					count++
				}
				continue
			}

			// Otherwise add in order and deduplicate.
			if tempValues == nil {
				tempValues = make([]ident.ID, 0, len(tagResult.tagValues))
			}
			tempValues = tempValues[:0]

			lastValueIdx := 0
			addRemaining := false
			nextLastValue := func() {
				lastValueIdx++
				if lastValueIdx >= len(tagResult.tagValues) {
					// None left to compare against, just blindly add the remaining.
					addRemaining = true
				}
			}

			for i := 0; i < len(values); i++ {
				tagValueResponse := values[i]
				currValue := ident.BytesID(tagValueResponse.TagValue)
				if addRemaining {
					// Just add remaining values.
					elem := ident.BytesID(tagValueResponse.TagValue)
					tempValues = append(tempValues, elem)
					count++
					continue
				}

				existingValue := tagResult.tagValues[lastValueIdx]
				cmp := bytes.Compare(currValue.Bytes(), existingValue.Bytes())
				for !addRemaining && cmp > 0 {
					// Take the existing value
					tempValues = append(tempValues, existingValue)

					// Move to next record
					nextLastValue()
					if addRemaining {
						// None left to compare against, just blindly add the remaining.
						break
					}

					// Re-run check
					existingValue = tagResult.tagValues[lastValueIdx]
					cmp = bytes.Compare(currValue.Bytes(), existingValue.Bytes())
				}
				if addRemaining {
					// Reprocess this element
					i--
					continue
				}

				if cmp == 0 {
					// Take existing record, skip this copy
					tempValues = append(tempValues, existingValue)
					nextLastValue()
					continue
				}

				// This record must come before any existing value, take and move to next
				tempValues = append(tempValues, currValue)
			}

			// Copy out of temp values back to final result
			tagResult.tagValues = append(tagResult.tagValues[:0], tempValues...)
		}

		moreElems = hasMore
		return count < limit
	})

	exhaustive := accum.exhaustive && count <= limit && !moreElems
	return iter, FetchResponseMetadata{
		Exhaustive:         exhaustive,
		Responses:          len(accum.aggResponses),
		EstimateTotalBytes: accum.calcTransport.GetSize(),
	}, nil
}

type fetchTaggedShardConsistencyResults []fetchTaggedShardConsistencyResult

func (res fetchTaggedShardConsistencyResults) initialize(length int) fetchTaggedShardConsistencyResults {
	if cap(res) < length {
		res = make(fetchTaggedShardConsistencyResults, length)
	}
	res = res[:length]
	// following compiler optimized memcpy impl:
	// https://github.com/golang/go/wiki/CompilerOptimizations#optimized-memclr
	for i := range res {
		res[i] = fetchTaggedShardConsistencyResult{}
	}
	return res
}

type fetchTaggedIDResults []*rpc.FetchTaggedIDResult_

// lambda to iterate over fetchTagged responses a single id at a time, `hasMore` indicates
// if there are more results to iterate after the current batch of elements, the returned
// bool indicates if the iteration should be continued past the curent batch.
type forEachFetchTaggedIDFn func(responsesForSingleID fetchTaggedIDResults, hasMore bool) (continueIterating bool)

// forEachID iterates over the provide results, and calls `fn` on each
// group of responses with the same ID.
// NB: assumes the results array being operated upon has been sorted.
func (results fetchTaggedIDResults) forEachID(fn forEachFetchTaggedIDFn) {
	var (
		startIdx = 0
		lastID   []byte
	)
	for i := 0; i < len(results); i++ {
		elem := results[i]
		if !bytes.Equal(elem.ID, lastID) {
			lastID = elem.ID
			// We only want to call the the forEachID fn once we have calculated the entire group,
			// i.e. once we have gone past the last element for a given ID, but the first element
			// in the results slice is a special case because we are always starting a new group
			// at that point.
			if i == 0 {
				continue
			}
			continueIterating := fn(results[startIdx:i], i < len(results))
			if !continueIterating {
				return
			}
			startIdx = i
		}
	}
	// spill over
	if startIdx < len(results) {
		fn(results[startIdx:], false)
	}
}

// fetchTaggedIDResultsSortedByID implements sort.Interface for fetchTaggedIDResults
// based on the ID field.
type fetchTaggedIDResultsSortedByID fetchTaggedIDResults

func (a fetchTaggedIDResultsSortedByID) Len() int      { return len(a) }
func (a fetchTaggedIDResultsSortedByID) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a fetchTaggedIDResultsSortedByID) Less(i, j int) bool {
	return bytes.Compare(a[i].ID, a[j].ID) < 0
}

type aggregateResults []*rpc.AggregateQueryRawResultTagNameElement

type aggregateValueResults []*rpc.AggregateQueryRawResultTagValueElement

// lambda to iterate over aggregate tag responses a single tag at a time, `hasMore` indicates
// if there are more results to iterate after the current batch of elements, the returned
// bool indicates if the iteration should be continued past the curent batch.
type forEachAggregateFn func(responsesForSingleTag aggregateResults, hasMore bool) (continueIterating bool)

// forEachTag iterates over the provide results, and calls `fn` on each
// group of responses with the same TagName.
// NB: assumes the results array being operated upon has been sorted.
func (results aggregateResults) forEachTag(fn forEachAggregateFn) {
	var (
		startIdx    = 0
		lastTagName []byte
	)
	for i := 0; i < len(results); i++ {
		elem := results[i]
		if !bytes.Equal(elem.TagName, lastTagName) {
			lastTagName = elem.TagName
			// We only want to call the the forEachID fn once we have calculated the entire group,
			// i.e. once we have gone past the last element for a given ID, but the first element
			// in the results slice is a special case because we are always starting a new group
			// at that point.
			if i == 0 {
				continue
			}
			continueIterating := fn(results[startIdx:i], i < len(results))
			if !continueIterating {
				return
			}
			startIdx = i
		}
	}
	// spill over
	if startIdx < len(results) {
		fn(results[startIdx:], false)
	}
}

// aggregateResultsSortedByTag implements sort.Interface for aggregateResults
// based on the TagName field.
type aggregateResultsSortedByTag aggregateResults

func (a aggregateResultsSortedByTag) Len() int      { return len(a) }
func (a aggregateResultsSortedByTag) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a aggregateResultsSortedByTag) Less(i, j int) bool {
	return bytes.Compare(a[i].TagName, a[j].TagName) < 0
}

// aggregateValueResultsSortedByValue implements sort.Interface for aggregateValueResults
// based on the TagValue field.
type aggregateValueResultsSortedByValue aggregateValueResults

func (a aggregateValueResultsSortedByValue) Len() int      { return len(a) }
func (a aggregateValueResultsSortedByValue) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a aggregateValueResultsSortedByValue) Less(i, j int) bool {
	return bytes.Compare(a[i].TagValue, a[j].TagValue) < 0
}
