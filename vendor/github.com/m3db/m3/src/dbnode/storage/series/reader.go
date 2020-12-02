// Copyright (c) 2017 Uber Technologies, Inc.
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
	"errors"
	"fmt"
	"time"

	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/retention"
	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/context"
	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/ident"
)

var (
	errSeriesReadInvalidRange = errors.New(
		"series invalid time range read argument specified")
)

// Reader reads results from a series, or a series block
// retriever or both.
// It is implemented as a struct so it can be allocated on
// the stack.
type Reader struct {
	opts       Options
	id         ident.ID
	retriever  QueryableBlockRetriever
	onRetrieve block.OnRetrieveBlock
	onRead     block.OnReadBlock
}

// NewReaderUsingRetriever returns a reader for a series
// block retriever, it will use the block retriever as the
// source to read blocks from.
func NewReaderUsingRetriever(
	id ident.ID,
	retriever QueryableBlockRetriever,
	onRetrieveBlock block.OnRetrieveBlock,
	onReadBlock block.OnReadBlock,
	opts Options,
) Reader {
	return Reader{
		opts:       opts,
		id:         id,
		retriever:  retriever,
		onRetrieve: onRetrieveBlock,
		onRead:     onReadBlock,
	}
}

// ReadEncoded reads encoded blocks using just a block retriever.
func (r Reader) ReadEncoded(
	ctx context.Context,
	start, end time.Time,
	nsCtx namespace.Context,
) ([][]xio.BlockReader, error) {
	return r.readersWithBlocksMapAndBuffer(ctx, start, end, nil, nil, nsCtx)
}

func (r Reader) readersWithBlocksMapAndBuffer(
	ctx context.Context,
	start, end time.Time,
	seriesBlocks block.DatabaseSeriesBlocks,
	seriesBuffer databaseBuffer,
	nsCtx namespace.Context,
) ([][]xio.BlockReader, error) {
	// Two-dimensional slice such that the first dimension is unique by blockstart
	// and the second dimension is blocks of data for that blockstart (not necessarily
	// in chronological order).
	//
	// ex. (querying 2P.M -> 6P.M with a 2-hour blocksize):
	// [][]xio.BlockReader{
	//   {block0, block1, block2}, // <- 2P.M
	//   {block0, block1}, // <-4P.M
	// }
	var results [][]xio.BlockReader

	if end.Before(start) {
		return nil, xerrors.NewInvalidParamsError(errSeriesReadInvalidRange)
	}

	var (
		nowFn        = r.opts.ClockOptions().NowFn()
		now          = nowFn()
		cachePolicy  = r.opts.CachePolicy()
		ropts        = r.opts.RetentionOptions()
		size         = ropts.BlockSize()
		alignedStart = start.Truncate(size)
		alignedEnd   = end.Truncate(size)
	)

	if alignedEnd.Equal(end) {
		// Move back to make range [start, end)
		alignedEnd = alignedEnd.Add(-1 * size)
	}

	// Squeeze the lookup window by what's available to make range queries like [0, infinity) possible
	earliest := retention.FlushTimeStart(ropts, now)
	if alignedStart.Before(earliest) {
		alignedStart = earliest
	}
	latest := now.Add(ropts.BufferFuture()).Truncate(size)
	if alignedEnd.After(latest) {
		alignedEnd = latest
	}

	first, last := alignedStart, alignedEnd
	for blockAt := first; !blockAt.After(last); blockAt = blockAt.Add(size) {
		// resultsBlock holds the results from one block. The flow is:
		// 1) Look in the cache for metrics for a block.
		// 2) If there is nothing in the cache, try getting metrics from disk.
		// 3) Regardless of (1) or (2), look for metrics in the series buffer.
		//
		// It is important to look for data in the series buffer one block at
		// a time within this loop so that the returned results contain data
		// from blocks in chronological order. Failure to do this will result
		// in an out of order error in the MultiReaderIterator on query.
		var resultsBlock []xio.BlockReader

		retrievedFromDiskCache := false
		if seriesBlocks != nil {
			if block, ok := seriesBlocks.BlockAt(blockAt); ok {
				// Block served from in-memory or in-memory metadata
				// will defer to disk read
				streamedBlock, err := block.Stream(ctx)
				if err != nil {
					return nil, err
				}
				if streamedBlock.IsNotEmpty() {
					resultsBlock = append(resultsBlock, streamedBlock)
					// NB(r): Mark this block as read now
					block.SetLastReadTime(now)
					if r.onRead != nil {
						r.onRead.OnReadBlock(block)
					}
				}
				retrievedFromDiskCache = true
			}
		}

		// Avoid going to disk if data was already in the cache.
		if !retrievedFromDiskCache {
			switch {
			case cachePolicy == CacheAll:
				// No-op, block metadata should have been in-memory
			case r.retriever != nil:
				// Try to stream from disk
				isRetrievable, err := r.retriever.IsBlockRetrievable(blockAt)
				if err != nil {
					return nil, err
				}
				if isRetrievable {
					streamedBlock, err := r.retriever.Stream(ctx, r.id, blockAt, r.onRetrieve, nsCtx)
					if err != nil {
						return nil, err
					}
					if streamedBlock.IsNotEmpty() {
						resultsBlock = append(resultsBlock, streamedBlock)
					}
				}
			}
		}

		if seriesBuffer != nil {
			bufferResults, err := seriesBuffer.ReadEncoded(ctx, blockAt, blockAt.Add(size), nsCtx)
			if err != nil {
				return nil, err
			}
			// Multiple block results may be returned here (for the same block
			// start) - one for warm writes and another for cold writes.
			for _, bufferRes := range bufferResults {
				resultsBlock = append(resultsBlock, bufferRes...)
			}
		}

		if len(resultsBlock) > 0 {
			results = append(results, resultsBlock)
		}
	}

	return results, nil
}

// FetchBlocks returns data blocks given a list of block start times using
// just a block retriever.
func (r Reader) FetchBlocks(
	ctx context.Context,
	starts []time.Time,
	nsCtx namespace.Context,
) ([]block.FetchBlockResult, error) {
	return r.fetchBlocksWithBlocksMapAndBuffer(ctx, starts, nil, nil, nsCtx)
}

func (r Reader) fetchBlocksWithBlocksMapAndBuffer(
	ctx context.Context,
	starts []time.Time,
	seriesBlocks block.DatabaseSeriesBlocks,
	seriesBuffer databaseBuffer,
	nsCtx namespace.Context,
) ([]block.FetchBlockResult, error) {
	var (
		// Two-dimensional slice (each block.FetchBlockResult has a []xio.BlockReader internally)
		// such that the first dimension is unique by blockstart and the second dimension is blocks
		// of data for that blockstart (not necessarily in chronological order).
		//
		// ex. (querying 2P.M -> 6P.M with a 2-hour blocksize):
		// []block.FetchBlockResult{
		//   block.FetchBlockResult{
		//     Start: 2P.M,
		//     Blocks: []xio.BlockReader{block0, block1, block2},
		//   },
		//   block.FetchBlockResult{
		//     Start: 4P.M,
		//     Blocks: []xio.BlockReader{block0},
		//   },
		// }
		res         = make([]block.FetchBlockResult, 0, len(starts))
		cachePolicy = r.opts.CachePolicy()
		// NB(r): Always use nil for OnRetrieveBlock so we don't cache the
		// series after fetching it from disk, the fetch blocks API is called
		// during streaming so to cache it in memory would mean we would
		// eventually cache all series in memory when we stream results to a
		// peer.
		onRetrieve block.OnRetrieveBlock
	)
	for _, start := range starts {
		// Slice of xio.BlockReader such that all data belong to the same blockstart.
		var blockReaders []xio.BlockReader

		retrievedFromDiskCache := false
		if seriesBlocks != nil {
			if b, exists := seriesBlocks.BlockAt(start); exists {
				streamedBlock, err := b.Stream(ctx)
				if err != nil {
					// Short-circuit this entire blockstart if an error was encountered.
					r := block.NewFetchBlockResult(start, nil,
						fmt.Errorf("unable to retrieve block stream for series %s time %v: %v",
							r.id.String(), start, err))
					res = append(res, r)
					continue
				}

				if streamedBlock.IsNotEmpty() {
					blockReaders = append(blockReaders, streamedBlock)
				}
				retrievedFromDiskCache = true
			}
		}

		// Avoid going to disk if data was already in the cache.
		if !retrievedFromDiskCache {
			switch {
			case cachePolicy == CacheAll:
				// No-op, block metadata should have been in-memory
			case r.retriever != nil:
				// Try to stream from disk
				isRetrievable, err := r.retriever.IsBlockRetrievable(start)
				if err != nil {
					// Short-circuit this entire blockstart if an error was encountered.
					r := block.NewFetchBlockResult(start, nil,
						fmt.Errorf("unable to retrieve block stream for series %s time %v: %v",
							r.id.String(), start, err))
					res = append(res, r)
					continue
				}

				if isRetrievable {
					streamedBlock, err := r.retriever.Stream(ctx, r.id, start, onRetrieve, nsCtx)
					if err != nil {
						// Short-circuit this entire blockstart if an error was encountered.
						r := block.NewFetchBlockResult(start, nil,
							fmt.Errorf("unable to retrieve block stream for series %s time %v: %v",
								r.id.String(), start, err))
						res = append(res, r)
						continue
					}

					if streamedBlock.IsNotEmpty() {
						blockReaders = append(blockReaders, streamedBlock)
					}
				}
			}
		}

		if len(blockReaders) > 0 {
			res = append(res, block.NewFetchBlockResult(start, blockReaders, nil))
		}
	}

	if seriesBuffer != nil && !seriesBuffer.IsEmpty() {
		bufferResults := seriesBuffer.FetchBlocks(ctx, starts, nsCtx)

		// Ensure both slices are sorted before merging as two sorted lists.
		block.SortFetchBlockResultByTimeAscending(res)
		block.SortFetchBlockResultByTimeAscending(bufferResults)
		bufferIdx := 0
		for i, blockResult := range res {
			if !(bufferIdx < len(bufferResults)) {
				break
			}

			currBufferResult := bufferResults[bufferIdx]
			if blockResult.Start.Equal(currBufferResult.Start) {
				if currBufferResult.Err != nil {
					res[i].Err = currBufferResult.Err
				} else {
					res[i].Blocks = append(res[i].Blocks, currBufferResult.Blocks...)
				}
				bufferIdx++
				continue
			}
		}

		// Add any buffer results for which there was no existing blockstart
		// to the end.
		if bufferIdx < len(bufferResults) {
			res = append(res, bufferResults[bufferIdx:]...)
		}
	}

	// Should still be sorted but do it again for sanity.
	block.SortFetchBlockResultByTimeAscending(res)
	return res, nil
}
