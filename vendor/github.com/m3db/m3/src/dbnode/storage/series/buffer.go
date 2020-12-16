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
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/persist"
	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/context"
	xerrors "github.com/m3db/m3/src/x/errors"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/pool"
	xtime "github.com/m3db/m3/src/x/time"

	"go.uber.org/zap"
)

const (
	errBucketMapCacheNotInSync    = "bucket map keys do not match sorted keys cache"
	errBucketMapCacheNotInSyncFmt = errBucketMapCacheNotInSync + ", blockStart: %d"
	errTimestampFormat            = time.RFC822Z
)

var (
	timeZero           time.Time
	errIncompleteMerge = errors.New("bucket merge did not result in only one encoder")
	errTooManyEncoders = xerrors.NewInvalidParamsError(errors.New("too many encoders per block"))
)

const (
	bucketsCacheSize = 2
	// optimizedTimesArraySize is the size of the internal array for the
	// optimizedTimes struct. Since the size of this array determines the
	// effectiveness of minimizing heap allocations, usage of this struct and/or
	// changing this const should only be done after considering its current
	// use cases:
	// 1) The number of buckets that will be removed within a tick due to that
	//    block being recently flushed
	// 2) The number of buckets that contain ColdWrites within a cold flush
	//    cycle
	// TODO(juchan): revisit this after ColdWrites usage to see if this number
	// is sane.
	optimizedTimesArraySize = 8
	writableBucketVersion   = 0
)

type databaseBuffer interface {
	MoveTo(
		buffer databaseBuffer,
		nsCtx namespace.Context,
	) error

	Write(
		ctx context.Context,
		id ident.ID,
		timestamp time.Time,
		value float64,
		unit xtime.Unit,
		annotation []byte,
		wOpts WriteOptions,
	) (bool, WriteType, error)

	Snapshot(
		ctx context.Context,
		blockStart time.Time,
		metadata persist.Metadata,
		persistFn persist.DataFn,
		nsCtx namespace.Context,
	) (SnapshotResult, error)

	WarmFlush(
		ctx context.Context,
		blockStart time.Time,
		metadata persist.Metadata,
		persistFn persist.DataFn,
		nsCtx namespace.Context,
	) (FlushOutcome, error)

	ReadEncoded(
		ctx context.Context,
		start, end time.Time,
		nsCtx namespace.Context,
	) ([][]xio.BlockReader, error)

	FetchBlocksForColdFlush(
		ctx context.Context,
		start time.Time,
		version int,
		nsCtx namespace.Context,
	) (block.FetchBlockResult, error)

	FetchBlocks(
		ctx context.Context,
		starts []time.Time,
		nsCtx namespace.Context,
	) []block.FetchBlockResult

	FetchBlocksMetadata(
		ctx context.Context,
		start, end time.Time,
		opts FetchBlocksMetadataOptions,
	) (block.FetchBlockMetadataResults, error)

	IsEmpty() bool

	IsEmptyAtBlockStart(time.Time) bool

	ColdFlushBlockStarts(blockStates map[xtime.UnixNano]BlockState) OptimizedTimes

	Stats() bufferStats

	Tick(versions ShardBlockStateSnapshot, nsCtx namespace.Context) bufferTickResult

	Load(bl block.DatabaseBlock, writeType WriteType)

	Reset(opts databaseBufferResetOptions)
}

type databaseBufferResetOptions struct {
	BlockRetriever QueryableBlockRetriever
	Options        Options
}

type bufferStats struct {
	wiredBlocks int
}

type bufferTickResult struct {
	mergedOutOfOrderBlocks int
	evictedBucketTimes     OptimizedTimes
}

// OptimizedTimes is a struct that holds an unknown number of times. This is
// used to avoid heap allocations as much as possible by trying to not allocate
// a slice of times. To do this, `optimizedTimesArraySize` needs to be
// strategically sized such that for the vast majority of the time, the internal
// array can hold all the times required so that `slice` is nil.
//
// OptimizedTimes should only be interacted with via its helper functions - its
// fields should never be accessed or modified directly, which could cause an
// invalid state.
type OptimizedTimes struct {
	arrIdx int
	arr    [optimizedTimesArraySize]xtime.UnixNano
	slice  []xtime.UnixNano
}

// Add adds a time to this OptimizedTimes.
func (t *OptimizedTimes) Add(newTime xtime.UnixNano) {
	if t.arrIdx < cap(t.arr) {
		t.arr[t.arrIdx] = newTime
		t.arrIdx++
	} else {
		t.slice = append(t.slice, newTime)
	}
}

// Len returns the number of times in this OptimizedTimes.
func (t *OptimizedTimes) Len() int {
	return t.arrIdx + len(t.slice)
}

// Contains returns whether the target time is in this OptimizedTimes.
func (t *OptimizedTimes) Contains(target xtime.UnixNano) bool {
	for i := 0; i < t.arrIdx; i++ {
		if t.arr[i].Equal(target) {
			return true
		}
	}
	for _, tt := range t.slice {
		if tt.Equal(target) {
			return true
		}
	}
	return false
}

// ForEach runs the given function for each time in this OptimizedTimes.
func (t *OptimizedTimes) ForEach(fn func(t xtime.UnixNano)) {
	for i, tNano := range t.arr {
		if i >= t.arrIdx {
			break
		}
		fn(tNano)
	}
	for _, tNano := range t.slice {
		fn(tNano)
	}
}

type dbBuffer struct {
	opts  Options
	nowFn clock.NowFn

	// bucketsMap is a map from a block start to its corresponding bucket
	// versions.
	bucketsMap map[xtime.UnixNano]*BufferBucketVersions
	// Cache of buckets to avoid map lookup of above.
	bucketVersionsCache [bucketsCacheSize]*BufferBucketVersions
	// This is an in order slice of the block starts in the bucketsMap.
	// We maintain this to avoid sorting the map keys adhoc when we want to
	// perform operations in chronological order.
	inOrderBlockStarts []time.Time
	bucketVersionsPool *BufferBucketVersionsPool
	bucketPool         *BufferBucketPool
	blockRetriever     QueryableBlockRetriever
}

// NB(prateek): databaseBuffer.Reset(...) must be called upon the returned
// object prior to use.
func newDatabaseBuffer() databaseBuffer {
	b := &dbBuffer{
		bucketsMap:         make(map[xtime.UnixNano]*BufferBucketVersions),
		inOrderBlockStarts: make([]time.Time, 0, bucketsCacheSize),
	}
	return b
}

func (b *dbBuffer) Reset(opts databaseBufferResetOptions) {
	b.opts = opts.Options
	b.nowFn = opts.Options.ClockOptions().NowFn()
	b.bucketPool = opts.Options.BufferBucketPool()
	b.bucketVersionsPool = opts.Options.BufferBucketVersionsPool()
	b.blockRetriever = opts.BlockRetriever
}

func (b *dbBuffer) MoveTo(
	buffer databaseBuffer,
	nsCtx namespace.Context,
) error {
	blockSize := b.opts.RetentionOptions().BlockSize()
	for _, buckets := range b.bucketsMap {
		for _, bucket := range buckets.buckets {
			// Load any existing blocks.
			for _, block := range bucket.loadedBlocks {
				// Load block.
				buffer.Load(block, bucket.writeType)
			}

			// Load encoders.
			for _, elem := range bucket.encoders {
				if elem.encoder.Len() == 0 {
					// No data.
					continue
				}
				// Take ownership of the encoder.
				segment := elem.encoder.Discard()
				// Create block and load into new buffer.
				block := b.opts.DatabaseBlockOptions().DatabaseBlockPool().Get()
				block.Reset(bucket.start, blockSize, segment, nsCtx)
				// Load block.
				buffer.Load(block, bucket.writeType)
			}
		}
	}

	return nil
}

func (b *dbBuffer) Write(
	ctx context.Context,
	id ident.ID,
	timestamp time.Time,
	value float64,
	unit xtime.Unit,
	annotation []byte,
	wOpts WriteOptions,
) (bool, WriteType, error) {
	var (
		ropts        = b.opts.RetentionOptions()
		bufferPast   = ropts.BufferPast()
		bufferFuture = ropts.BufferFuture()
		now          = b.nowFn()
		pastLimit    = now.Add(-1 * bufferPast).Truncate(time.Second)
		futureLimit  = now.Add(bufferFuture).Truncate(time.Second)
		blockSize    = ropts.BlockSize()
		blockStart   = timestamp.Truncate(blockSize)
		writeType    WriteType
	)

	switch {
	case wOpts.BootstrapWrite:
		exists, err := b.blockRetriever.IsBlockRetrievable(blockStart)
		if err != nil {
			return false, writeType, err
		}
		// Bootstrap writes are allowed to be outside of time boundaries
		// and determined as cold or warm writes depending on whether
		// the block is retrievable or not.
		if !exists {
			writeType = WarmWrite
		} else {
			writeType = ColdWrite
		}

	case timestamp.Before(pastLimit):
		writeType = ColdWrite
		if !b.opts.ColdWritesEnabled() {
			return false, writeType, xerrors.NewInvalidParamsError(
				fmt.Errorf("datapoint too far in past: "+
					"id=%s, off_by=%s, timestamp=%s, past_limit=%s, "+
					"timestamp_unix_nanos=%d, past_limit_unix_nanos=%d",
					id.Bytes(), pastLimit.Sub(timestamp).String(),
					timestamp.Format(errTimestampFormat),
					pastLimit.Format(errTimestampFormat),
					timestamp.UnixNano(), pastLimit.UnixNano()))
		}

	case !futureLimit.After(timestamp):
		writeType = ColdWrite
		if !b.opts.ColdWritesEnabled() {
			return false, writeType, xerrors.NewInvalidParamsError(
				fmt.Errorf("datapoint too far in future: "+
					"id=%s, off_by=%s, timestamp=%s, future_limit=%s, "+
					"timestamp_unix_nanos=%d, future_limit_unix_nanos=%d",
					id.Bytes(), timestamp.Sub(futureLimit).String(),
					timestamp.Format(errTimestampFormat),
					futureLimit.Format(errTimestampFormat),
					timestamp.UnixNano(), futureLimit.UnixNano()))
		}

	default:
		writeType = WarmWrite

	}

	if writeType == ColdWrite {
		retentionLimit := now.Add(-ropts.RetentionPeriod())
		if wOpts.BootstrapWrite {
			// NB(r): Allow bootstrapping to write to blocks that are
			// still in retention.
			retentionLimit = retentionLimit.Truncate(blockSize)
		}
		if retentionLimit.After(timestamp) {
			if wOpts.SkipOutOfRetention {
				// Allow for datapoint to be skipped since caller does not
				// want writes out of retention to fail.
				return false, writeType, nil
			}
			return false, writeType, xerrors.NewInvalidParamsError(
				fmt.Errorf("datapoint too far in past and out of retention: "+
					"id=%s, off_by=%s, timestamp=%s, retention_past_limit=%s, "+
					"timestamp_unix_nanos=%d, retention_past_limit_unix_nanos=%d",
					id.Bytes(), retentionLimit.Sub(timestamp).String(),
					timestamp.Format(errTimestampFormat),
					retentionLimit.Format(errTimestampFormat),
					timestamp.UnixNano(), retentionLimit.UnixNano()))
		}

		futureRetentionLimit := now.Add(ropts.FutureRetentionPeriod())
		if !futureRetentionLimit.After(timestamp) {
			if wOpts.SkipOutOfRetention {
				// Allow for datapoint to be skipped since caller does not
				// want writes out of retention to fail.
				return false, writeType, nil
			}
			return false, writeType, xerrors.NewInvalidParamsError(
				fmt.Errorf("datapoint too far in future and out of retention: "+
					"id=%s, off_by=%s, timestamp=%s, retention_future_limit=%s, "+
					"timestamp_unix_nanos=%d, retention_future_limit_unix_nanos=%d",
					id.Bytes(), timestamp.Sub(futureRetentionLimit).String(),
					timestamp.Format(errTimestampFormat),
					futureRetentionLimit.Format(errTimestampFormat),
					timestamp.UnixNano(), futureRetentionLimit.UnixNano()))
		}

		b.opts.Stats().IncColdWrites()
	}

	buckets := b.bucketVersionsAtCreate(blockStart)
	b.putBucketVersionsInCache(buckets)

	if wOpts.TruncateType == TypeBlock {
		timestamp = blockStart
	}

	if wOpts.TransformOptions.ForceValueEnabled {
		value = wOpts.TransformOptions.ForceValue
	}

	ok, err := buckets.write(timestamp, value, unit, annotation, writeType, wOpts.SchemaDesc)
	return ok, writeType, err
}

func (b *dbBuffer) IsEmpty() bool {
	// A buffer can only be empty if there are no buckets in its map, since
	// buckets are only created when a write for a new block start is done, and
	// buckets are removed from the map when they are evicted from memory.
	return len(b.bucketsMap) == 0
}

func (b *dbBuffer) IsEmptyAtBlockStart(start time.Time) bool {
	bv, exists := b.bucketVersionsAt(start)
	if !exists {
		return true
	}
	return bv.streamsLen() == 0
}

func (b *dbBuffer) ColdFlushBlockStarts(blockStates map[xtime.UnixNano]BlockState) OptimizedTimes {
	var times OptimizedTimes

	for t, bucketVersions := range b.bucketsMap {
		for _, bucket := range bucketVersions.buckets {
			if bucket.writeType == ColdWrite &&
				// We need to cold flush this bucket if it either:
				// 1) Has new cold writes that need to be flushed, or
				// 2) This bucket version is higher than what has been
				//    successfully flushed. This can happen if a cold flush was
				//    attempted, changing this bucket version, but fails to
				//    completely finish (which is what the shard block state
				//    signifies). In this case, we need to try to flush this
				//    bucket again.
				(bucket.version == writableBucketVersion ||
					blockStates[xtime.ToUnixNano(bucket.start)].ColdVersion < bucket.version) {
				times.Add(t)
				break
			}
		}
	}

	return times
}

func (b *dbBuffer) Stats() bufferStats {
	return bufferStats{
		wiredBlocks: len(b.bucketsMap),
	}
}

func (b *dbBuffer) Tick(blockStates ShardBlockStateSnapshot, nsCtx namespace.Context) bufferTickResult {
	mergedOutOfOrder := 0
	var evictedBucketTimes OptimizedTimes
	for tNano, buckets := range b.bucketsMap {
		// The blockStates map is never written to after creation, so this
		// read access is safe. Since this version map is a snapshot of the
		// versions, the real block flush versions may be higher. This is okay
		// here because it's safe to:
		// 1) not remove a bucket that's actually retrievable, or
		// 2) remove a lower versioned bucket.
		// Retrievable and higher versioned buckets will be left to be
		// collected in the next tick.
		blockStateSnapshot, bootstrapped := blockStates.UnwrapValue()
		// Only use block state snapshot information to make eviction decisions if the block state
		// has been properly bootstrapped already.
		if bootstrapped {
			blockState := blockStateSnapshot.Snapshot[tNano]
			if coldVersion := blockState.ColdVersion; blockState.WarmRetrievable || coldVersion > 0 {
				if blockState.WarmRetrievable {
					// Buckets for WarmWrites that are retrievable will only be version 1, since
					// they only get successfully persisted once.
					buckets.removeBucketsUpToVersion(WarmWrite, 1)
				}
				if coldVersion > 0 {
					buckets.removeBucketsUpToVersion(ColdWrite, coldVersion)
				}

				if buckets.streamsLen() == 0 {
					t := tNano.ToTime()
					// All underlying buckets have been flushed successfully, so we can
					// just remove the buckets from the bucketsMap.
					b.removeBucketVersionsAt(t)
					// Pass which bucket got evicted from the buffer to the series.
					// Data gets read in order of precedence: buffer -> cache -> disk.
					// After a bucket gets removed from the buffer, data from the cache
					// will be served. However, since data just got persisted to disk,
					// the cached block is now stale. To correct this, we can either:
					// 1) evict the stale block from cache so that new data will
					//    be retrieved from disk, or
					// 2) merge the new data into the cached block.
					// It's unclear whether recently flushed data would frequently be
					// read soon afterward, so we're choosing (1) here, since it has a
					// simpler implementation (just removing from a map).
					evictedBucketTimes.Add(tNano)
					continue
				}
			}
		}

		buckets.recordActiveEncoders()

		// Once we've evicted all eligible buckets, we merge duplicate encoders
		// in the remaining ones to try and reclaim memory.
		merges, err := buckets.merge(WarmWrite, nsCtx)
		if err != nil {
			log := b.opts.InstrumentOptions().Logger()
			log.Error("buffer merge encode error", zap.Error(err))
		}
		if merges > 0 {
			mergedOutOfOrder++
		}
	}
	return bufferTickResult{
		mergedOutOfOrderBlocks: mergedOutOfOrder,
		evictedBucketTimes:     evictedBucketTimes,
	}
}

func (b *dbBuffer) Load(bl block.DatabaseBlock, writeType WriteType) {
	var (
		blockStart = bl.StartTime()
		buckets    = b.bucketVersionsAtCreate(blockStart)
		bucket     = buckets.writableBucketCreate(writeType)
	)
	bucket.loadedBlocks = append(bucket.loadedBlocks, bl)
}

func (b *dbBuffer) Snapshot(
	ctx context.Context,
	blockStart time.Time,
	metadata persist.Metadata,
	persistFn persist.DataFn,
	nsCtx namespace.Context,
) (SnapshotResult, error) {
	var (
		start  = b.nowFn()
		result SnapshotResult
	)

	buckets, exists := b.bucketVersionsAt(blockStart)
	if !exists {
		return result, nil
	}

	// Snapshot must take both cold and warm writes because cold flushes don't
	// happen for the current block (since cold flushes can't happen before a
	// warm flush has happened).
	streams, err := buckets.mergeToStreams(ctx, streamsOptions{filterWriteType: false, nsCtx: nsCtx})
	if err != nil {
		return result, err
	}

	afterMergeByBucket := b.nowFn()
	result.Stats.TimeMergeByBucket = afterMergeByBucket.Sub(start)

	var (
		numStreams         = len(streams)
		mergeAcrossBuckets = numStreams != 1
		segment            ts.Segment
	)
	if !mergeAcrossBuckets {
		segment, err = streams[0].Segment()
		if err != nil {
			return result, err
		}
	} else {
		// We may need to merge again here because the regular merge method does
		// not merge warm and cold buckets or buckets that have different versions.
		sr := make([]xio.SegmentReader, 0, numStreams)
		for _, stream := range streams {
			sr = append(sr, stream)
		}

		bopts := b.opts.DatabaseBlockOptions()
		encoder := bopts.EncoderPool().Get()
		encoder.Reset(blockStart, bopts.DatabaseBlockAllocSize(), nsCtx.Schema)
		iter := b.opts.MultiReaderIteratorPool().Get()
		var encoderClosed bool
		defer func() {
			if !encoderClosed {
				encoder.Close()
			}
			iter.Close()
		}()
		iter.Reset(sr, blockStart, b.opts.RetentionOptions().BlockSize(), nsCtx.Schema)

		for iter.Next() {
			dp, unit, annotation := iter.Current()
			if err := encoder.Encode(dp, unit, annotation); err != nil {
				return result, err
			}
		}
		if err := iter.Err(); err != nil {
			return result, err
		}

		segment = encoder.Discard()
		defer segment.Finalize()
		encoderClosed = true
	}

	afterMergeAcrossBuckets := b.nowFn()
	result.Stats.TimeMergeAcrossBuckets = afterMergeAcrossBuckets.Sub(afterMergeByBucket)

	if segment.Len() == 0 {
		// Don't write out series with no data.
		return result, nil
	}

	checksum := segment.CalculateChecksum()

	afterChecksum := b.nowFn()
	result.Stats.TimeChecksum = afterChecksum.Sub(afterMergeAcrossBuckets)

	if err := persistFn(metadata, segment, checksum); err != nil {
		return result, err
	}

	result.Stats.TimePersist = b.nowFn().Sub(afterChecksum)

	result.Persist = true
	return result, nil
}

func (b *dbBuffer) WarmFlush(
	ctx context.Context,
	blockStart time.Time,
	metadata persist.Metadata,
	persistFn persist.DataFn,
	nsCtx namespace.Context,
) (FlushOutcome, error) {
	buckets, exists := b.bucketVersionsAt(blockStart)
	if !exists {
		return FlushOutcomeBlockDoesNotExist, nil
	}

	// Flush only deals with WarmWrites. ColdWrites get persisted to disk via
	// the compaction cycle.
	streams, err := buckets.mergeToStreams(ctx, streamsOptions{filterWriteType: true, writeType: WarmWrite, nsCtx: nsCtx})
	if err != nil {
		return FlushOutcomeErr, err
	}

	var (
		stream xio.SegmentReader
		ok     bool
	)
	if numStreams := len(streams); numStreams == 1 {
		stream = streams[0]
		ok = true
	} else {
		// In the majority of cases, there will only be one stream to persist
		// here. Only when a previous flush fails midway through a shard will
		// there be buckets for previous versions. In this case, we need to try
		// to flush them again, so we merge them together to one stream and
		// persist it.
		encoder, _, err := mergeStreamsToEncoder(blockStart, streams, b.opts, nsCtx)
		if err != nil {
			return FlushOutcomeErr, err
		}

		stream, ok = encoder.Stream(ctx)
		encoder.Close()
	}

	if !ok {
		// Don't write out series with no data.
		return FlushOutcomeBlockDoesNotExist, nil
	}

	segment, err := stream.Segment()
	if err != nil {
		return FlushOutcomeErr, err
	}

	if segment.Len() == 0 {
		// Empty segment is equivalent to no stream, i.e data does not exist.
		return FlushOutcomeBlockDoesNotExist, nil
	}

	checksum := segment.CalculateChecksum()
	err = persistFn(metadata, segment, checksum)
	if err != nil {
		return FlushOutcomeErr, err
	}

	if bucket, exists := buckets.writableBucket(WarmWrite); exists {
		// WarmFlushes only happen once per block, so it makes sense to always
		// set this to 1.
		bucket.version = 1
	}

	return FlushOutcomeFlushedToDisk, nil
}

func (b *dbBuffer) ReadEncoded(
	ctx context.Context,
	start time.Time,
	end time.Time,
	nsCtx namespace.Context,
) ([][]xio.BlockReader, error) {
	var (
		blockSize = b.opts.RetentionOptions().BlockSize()
		// TODO(r): pool these results arrays
		res [][]xio.BlockReader
	)

	for _, blockStart := range b.inOrderBlockStarts {
		if !blockStart.Before(end) || !start.Before(blockStart.Add(blockSize)) {
			continue
		}

		bv, exists := b.bucketVersionsAt(blockStart)
		if !exists {
			// Invariant violated. This means the keys in the bucket map does
			// not match the sorted keys cache, which should never happen.
			instrument.EmitAndLogInvariantViolation(
				b.opts.InstrumentOptions(), func(l *zap.Logger) {
					l.Error(errBucketMapCacheNotInSync, zap.Int64("blockStart", blockStart.UnixNano()))
				})
			return nil, instrument.InvariantErrorf(
				errBucketMapCacheNotInSyncFmt, blockStart.UnixNano())
		}

		if streams := bv.streams(ctx, streamsOptions{filterWriteType: false}); len(streams) > 0 {
			res = append(res, streams)
		}

		// NB(r): Store the last read time, should not set this when
		// calling FetchBlocks as a read is differentiated from
		// a FetchBlocks call. One is initiated by an external
		// entity and the other is used for streaming blocks between
		// the storage nodes. This distinction is important as this
		// data is important for use with understanding access patterns, etc.
		bv.setLastRead(b.nowFn())
	}

	return res, nil
}

func (b *dbBuffer) FetchBlocksForColdFlush(
	ctx context.Context,
	start time.Time,
	version int,
	nsCtx namespace.Context,
) (block.FetchBlockResult, error) {
	res := b.fetchBlocks(ctx, []time.Time{start},
		streamsOptions{filterWriteType: true, writeType: ColdWrite, nsCtx: nsCtx})
	if len(res) == 0 {
		// The lifecycle of calling this function is preceded by first checking
		// which blocks have cold data that have not yet been flushed.
		// If we don't get data here, it means that it has since fallen out of
		// retention and has been evicted.
		return block.FetchBlockResult{}, nil
	}
	if len(res) != 1 {
		// Must be only one result if anything at all, since fetchBlocks returns
		// one result per block start.
		return block.FetchBlockResult{}, fmt.Errorf("fetchBlocks did not return just one block for block start %s", start)
	}

	result := res[0]

	buckets, exists := b.bucketVersionsAt(start)
	if !exists {
		return block.FetchBlockResult{}, fmt.Errorf("buckets do not exist with block start %s", start)
	}
	if bucket, exists := buckets.writableBucket(ColdWrite); exists {
		// Update the version of the writable bucket (effectively making it not
		// writable). This marks this bucket as attempted to be flushed,
		// although it is only actually written to disk successfully at the
		// shard level after every series has completed the flush process.
		// The tick following a successful flush to disk will remove this bucket
		// from memory.
		bucket.version = version
	}
	// No-op if the writable bucket doesn't exist.
	// This function should only get called for blocks that we know need to be
	// cold flushed. However, buckets that get attempted to be cold flushed and
	// fail need to get cold flushed as well. These kinds of buckets will have
	// a non-writable version.

	return result, nil
}

func (b *dbBuffer) FetchBlocks(ctx context.Context, starts []time.Time, nsCtx namespace.Context) []block.FetchBlockResult {
	return b.fetchBlocks(ctx, starts, streamsOptions{filterWriteType: false, nsCtx: nsCtx})
}

func (b *dbBuffer) fetchBlocks(
	ctx context.Context,
	starts []time.Time,
	sOpts streamsOptions,
) []block.FetchBlockResult {
	var res []block.FetchBlockResult

	for _, start := range starts {
		buckets, ok := b.bucketVersionsAt(start)
		if !ok {
			continue
		}

		streams := buckets.streams(ctx, sOpts)
		if len(streams) > 0 {
			result := block.NewFetchBlockResult(
				start,
				streams,
				nil,
			)
			result.FirstWrite = buckets.firstWrite(sOpts)
			res = append(res, result)
		}
	}

	// Result should be sorted in ascending order.
	sort.Slice(res, func(i, j int) bool { return res[i].Start.Before(res[j].Start) })

	return res
}

func (b *dbBuffer) FetchBlocksMetadata(
	ctx context.Context,
	start, end time.Time,
	opts FetchBlocksMetadataOptions,
) (block.FetchBlockMetadataResults, error) {
	blockSize := b.opts.RetentionOptions().BlockSize()
	res := b.opts.FetchBlockMetadataResultsPool().Get()

	for _, blockStart := range b.inOrderBlockStarts {
		if !blockStart.Before(end) || !start.Before(blockStart.Add(blockSize)) {
			continue
		}

		bv, exists := b.bucketVersionsAt(blockStart)
		if !exists {
			// Invariant violated. This means the keys in the bucket map does
			// not match the sorted keys cache, which should never happen.
			instrument.EmitAndLogInvariantViolation(
				b.opts.InstrumentOptions(), func(l *zap.Logger) {
					l.Error(errBucketMapCacheNotInSync, zap.Int64("blockStart", blockStart.UnixNano()))
				})
			return nil, instrument.InvariantErrorf(errBucketMapCacheNotInSyncFmt, blockStart.UnixNano())
		}

		size := int64(bv.streamsLen())
		// If we have no data in this bucket, skip early without appending it to the result.
		if size == 0 {
			continue
		}
		var resultSize int64
		if opts.IncludeSizes {
			resultSize = size
		}
		var resultLastRead time.Time
		if opts.IncludeLastRead {
			resultLastRead = bv.lastRead()
		}

		var (
			checksum *uint32
			err      error
		)
		if opts.IncludeChecksums {
			// Checksum calculations are best effort since we can't calculate one if there
			// are multiple streams without performing an expensive merge.
			checksum, err = bv.checksumIfSingleStream(ctx)
			if err != nil {
				return nil, err
			}
		}
		res.Add(block.FetchBlockMetadataResult{
			Start:    bv.start,
			Size:     resultSize,
			LastRead: resultLastRead,
			Checksum: checksum,
		})
	}

	return res, nil
}

func (b *dbBuffer) bucketVersionsAt(
	t time.Time,
) (*BufferBucketVersions, bool) {
	// First check LRU cache.
	for _, buckets := range b.bucketVersionsCache {
		if buckets == nil {
			continue
		}
		if buckets.start.Equal(t) {
			return buckets, true
		}
	}

	// Then check the map.
	if buckets, exists := b.bucketsMap[xtime.ToUnixNano(t)]; exists {
		return buckets, true
	}

	return nil, false
}

func (b *dbBuffer) bucketVersionsAtCreate(
	t time.Time,
) *BufferBucketVersions {
	if buckets, exists := b.bucketVersionsAt(t); exists {
		return buckets
	}

	buckets := b.bucketVersionsPool.Get()
	buckets.resetTo(t, b.opts, b.bucketPool)
	b.bucketsMap[xtime.ToUnixNano(t)] = buckets
	b.inOrderBlockStartsAdd(t)

	return buckets
}

func (b *dbBuffer) putBucketVersionsInCache(newBuckets *BufferBucketVersions) {
	replaceIdx := bucketsCacheSize - 1
	for i, buckets := range b.bucketVersionsCache {
		// Check if we have the same pointer in cache.
		if buckets == newBuckets {
			replaceIdx = i
		}
	}

	for i := replaceIdx; i > 0; i-- {
		b.bucketVersionsCache[i] = b.bucketVersionsCache[i-1]
	}

	b.bucketVersionsCache[0] = newBuckets
}

func (b *dbBuffer) removeBucketVersionsInCache(oldBuckets *BufferBucketVersions) {
	nilIdx := -1
	for i, buckets := range b.bucketVersionsCache {
		if buckets == oldBuckets {
			nilIdx = i
		}
	}
	if nilIdx == -1 {
		return
	}

	for i := nilIdx; i < bucketsCacheSize-1; i++ {
		b.bucketVersionsCache[i] = b.bucketVersionsCache[i+1]
	}

	b.bucketVersionsCache[bucketsCacheSize-1] = nil
}

func (b *dbBuffer) removeBucketVersionsAt(blockStart time.Time) {
	buckets, exists := b.bucketVersionsAt(blockStart)
	if !exists {
		return
	}
	delete(b.bucketsMap, xtime.ToUnixNano(blockStart))
	b.removeBucketVersionsInCache(buckets)
	b.inOrderBlockStartsRemove(blockStart)
	// nil out pointers.
	buckets.resetTo(timeZero, nil, nil)
	b.bucketVersionsPool.Put(buckets)
}

func (b *dbBuffer) inOrderBlockStartsAdd(newTime time.Time) {
	starts := b.inOrderBlockStarts
	idx := len(starts)
	// There shouldn't be that many starts here, so just linear search through.
	for i, t := range starts {
		if t.After(newTime) {
			idx = i
			break
		}
	}
	// Insert new time without allocating new slice.
	b.inOrderBlockStarts = append(starts, timeZero)
	// Update to new slice
	starts = b.inOrderBlockStarts
	copy(starts[idx+1:], starts[idx:])
	starts[idx] = newTime
}

func (b *dbBuffer) inOrderBlockStartsRemove(removeTime time.Time) {
	starts := b.inOrderBlockStarts
	// There shouldn't be that many starts here, so just linear search through.
	for i, t := range starts {
		if t.Equal(removeTime) {
			b.inOrderBlockStarts = append(starts[:i], starts[i+1:]...)
			return
		}
	}
}

// BufferBucketVersions is a container for different versions of buffer buckets.
// Bucket versions are how the buffer separates writes that have been written
// to disk as a fileset and writes that have not. The bucket with a version of
// `writableBucketVersion` is the bucket that all writes go into (as thus is the
// bucket version that have not yet been persisted). After a bucket gets
// persisted, its version gets set to a version that the shard passes down to it
// (since the shard knows what has been fully persisted to disk).
type BufferBucketVersions struct {
	buckets           []*BufferBucket
	start             time.Time
	opts              Options
	lastReadUnixNanos int64
	bucketPool        *BufferBucketPool
}

func (b *BufferBucketVersions) resetTo(
	start time.Time,
	opts Options,
	bucketPool *BufferBucketPool,
) {
	// nil all elements so that they get GC'd.
	for i := range b.buckets {
		b.buckets[i] = nil
	}
	b.buckets = b.buckets[:0]
	b.start = start
	b.opts = opts
	atomic.StoreInt64(&b.lastReadUnixNanos, 0)
	b.bucketPool = bucketPool
}

// streams returns all the streams for this BufferBucketVersions.
func (b *BufferBucketVersions) streams(ctx context.Context, opts streamsOptions) []xio.BlockReader {
	var res []xio.BlockReader
	for _, bucket := range b.buckets {
		if opts.filterWriteType && bucket.writeType != opts.writeType {
			continue
		}
		res = append(res, bucket.streams(ctx)...)
	}

	return res
}

func (b *BufferBucketVersions) firstWrite(opts streamsOptions) time.Time {
	var res time.Time
	for _, bucket := range b.buckets {
		if opts.filterWriteType && bucket.writeType != opts.writeType {
			continue
		}
		// Get the earliest valid first write time.
		if res.IsZero() ||
			(bucket.firstWrite.Before(res) && !bucket.firstWrite.IsZero()) {
			res = bucket.firstWrite
		}
	}
	return res
}

func (b *BufferBucketVersions) streamsLen() int {
	res := 0
	for _, bucket := range b.buckets {
		res += bucket.streamsLen()
	}
	return res
}

func (b *BufferBucketVersions) checksumIfSingleStream(ctx context.Context) (*uint32, error) {
	if len(b.buckets) != 1 {
		return nil, nil
	}
	return b.buckets[0].checksumIfSingleStream(ctx)
}

func (b *BufferBucketVersions) write(
	timestamp time.Time,
	value float64,
	unit xtime.Unit,
	annotation []byte,
	writeType WriteType,
	schema namespace.SchemaDescr,
) (bool, error) {
	return b.writableBucketCreate(writeType).write(timestamp, value, unit, annotation, schema)
}

func (b *BufferBucketVersions) merge(writeType WriteType, nsCtx namespace.Context) (int, error) {
	res := 0
	for _, bucket := range b.buckets {
		// Only makes sense to merge buckets that are writable.
		if bucket.version == writableBucketVersion && writeType == bucket.writeType {
			merges, err := bucket.merge(nsCtx)
			if err != nil {
				return 0, err
			}
			res += merges
		}
	}

	return res, nil
}

func (b *BufferBucketVersions) removeBucketsUpToVersion(
	writeType WriteType,
	version int,
) {
	// Avoid allocating a new backing array.
	nonEvictedBuckets := b.buckets[:0]

	for _, bucket := range b.buckets {
		bVersion := bucket.version
		if bucket.writeType == writeType && bVersion != writableBucketVersion &&
			bVersion <= version {
			// We no longer need to keep any version which is equal to
			// or less than the retrievable version, since that means
			// that the version has successfully persisted to disk.
			// Bucket gets reset before use.
			b.bucketPool.Put(bucket)
			continue
		}

		nonEvictedBuckets = append(nonEvictedBuckets, bucket)
	}

	b.buckets = nonEvictedBuckets
}

func (b *BufferBucketVersions) setLastRead(value time.Time) {
	atomic.StoreInt64(&b.lastReadUnixNanos, value.UnixNano())
}

func (b *BufferBucketVersions) lastRead() time.Time {
	return time.Unix(0, atomic.LoadInt64(&b.lastReadUnixNanos))
}

func (b *BufferBucketVersions) writableBucket(writeType WriteType) (*BufferBucket, bool) {
	for _, bucket := range b.buckets {
		if bucket.version == writableBucketVersion && bucket.writeType == writeType {
			return bucket, true
		}
	}

	return nil, false
}

func (b *BufferBucketVersions) writableBucketCreate(writeType WriteType) *BufferBucket {
	bucket, exists := b.writableBucket(writeType)

	if exists {
		return bucket
	}

	newBucket := b.bucketPool.Get()
	newBucket.resetTo(b.start, writeType, b.opts)
	b.buckets = append(b.buckets, newBucket)
	return newBucket
}

// mergeToStreams merges each buffer bucket version's streams into one, then
// returns a single stream for each buffer bucket version.
func (b *BufferBucketVersions) mergeToStreams(ctx context.Context, opts streamsOptions) ([]xio.SegmentReader, error) {
	buckets := b.buckets
	res := make([]xio.SegmentReader, 0, len(buckets))

	for _, bucket := range buckets {
		if opts.filterWriteType && bucket.writeType != opts.writeType {
			continue
		}
		stream, ok, err := bucket.mergeToStream(ctx, opts.nsCtx)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		res = append(res, stream)
	}

	return res, nil
}

func (b *BufferBucketVersions) recordActiveEncoders() {
	var numActiveEncoders int
	for _, bucket := range b.buckets {
		if bucket.version == writableBucketVersion {
			numActiveEncoders += len(bucket.encoders)
		}
	}
	b.opts.Stats().RecordEncodersPerBlock(numActiveEncoders)
}

type streamsOptions struct {
	filterWriteType bool
	writeType       WriteType
	nsCtx           namespace.Context
}

// BufferBucket is a specific version of a bucket of encoders, which is where
// writes are ultimately stored before they are persisted to disk as a fileset.
// See comment for BufferBucketVersions for more detail on bucket versions.
type BufferBucket struct {
	opts         Options
	start        time.Time
	encoders     []inOrderEncoder
	loadedBlocks []block.DatabaseBlock
	version      int
	writeType    WriteType
	firstWrite   time.Time
}

type inOrderEncoder struct {
	encoder     encoding.Encoder
	lastWriteAt time.Time
}

func (b *BufferBucket) resetTo(
	start time.Time,
	writeType WriteType,
	opts Options,
) {
	// Close the old context if we're resetting for use.
	b.reset()
	b.opts = opts
	b.start = start
	bopts := b.opts.DatabaseBlockOptions()
	encoder := bopts.EncoderPool().Get()
	encoder.Reset(start, bopts.DatabaseBlockAllocSize(), nil)
	b.encoders = append(b.encoders, inOrderEncoder{
		encoder: encoder,
	})
	b.loadedBlocks = nil
	// We would only ever create a bucket for it to be writable.
	b.version = writableBucketVersion
	b.writeType = writeType
	b.firstWrite = time.Time{}
}

func (b *BufferBucket) reset() {
	b.resetEncoders()
	b.resetLoadedBlocks()
}

func (b *BufferBucket) write(
	timestamp time.Time,
	value float64,
	unit xtime.Unit,
	annotation []byte,
	schema namespace.SchemaDescr,
) (bool, error) {
	datapoint := ts.Datapoint{
		Timestamp:      timestamp,
		TimestampNanos: xtime.ToUnixNano(timestamp),
		Value:          value,
	}

	// Find the correct encoder to write to
	idx := -1
	for i := range b.encoders {
		lastWriteAt := b.encoders[i].lastWriteAt
		if timestamp.Equal(lastWriteAt) {
			lastDatapoint, err := b.encoders[i].encoder.LastEncoded()
			if err != nil {
				return false, err
			}
			lastAnnotation, err := b.encoders[i].encoder.LastAnnotation()
			if err != nil {
				return false, err
			}

			if lastDatapoint.Value == value && bytes.Equal(lastAnnotation, annotation) {
				// No-op since matches the current value. Propagates up to callers that
				// no value was written.
				return false, nil
			}
			continue
		}

		if timestamp.After(lastWriteAt) {
			idx = i
			break
		}
	}

	var err error
	defer func() {
		nowFn := b.opts.ClockOptions().NowFn()
		if err == nil && b.firstWrite.IsZero() {
			b.firstWrite = nowFn()
		}
	}()

	// Upsert/last-write-wins semantics.
	// NB(r): We push datapoints with the same timestamp but differing
	// value into a new encoder later in the stack of in order encoders
	// since an encoder is immutable.
	// The encoders pushed later will surface their values first.
	if idx != -1 {
		err = b.writeToEncoderIndex(idx, datapoint, unit, annotation, schema)
		return err == nil, err
	}

	// Need a new encoder, we didn't find an encoder to write to.
	maxEncoders := b.opts.RuntimeOptionsManager().Get().EncodersPerBlockLimit()
	if maxEncoders != 0 && len(b.encoders) >= int(maxEncoders) {
		b.opts.Stats().IncEncoderLimitWriteRejected()
		return false, errTooManyEncoders
	}

	b.opts.Stats().IncCreatedEncoders()
	bopts := b.opts.DatabaseBlockOptions()
	blockSize := b.opts.RetentionOptions().BlockSize()
	blockAllocSize := bopts.DatabaseBlockAllocSize()

	encoder := b.opts.EncoderPool().Get()
	encoder.Reset(timestamp.Truncate(blockSize), blockAllocSize, schema)

	b.encoders = append(b.encoders, inOrderEncoder{
		encoder:     encoder,
		lastWriteAt: timestamp,
	})

	idx = len(b.encoders) - 1
	err = b.writeToEncoderIndex(idx, datapoint, unit, annotation, schema)
	if err != nil {
		encoder.Close()
		b.encoders = b.encoders[:idx]
		return false, err
	}
	return true, nil
}

func (b *BufferBucket) writeToEncoderIndex(
	idx int,
	datapoint ts.Datapoint,
	unit xtime.Unit,
	annotation []byte,
	schema namespace.SchemaDescr,
) error {
	b.encoders[idx].encoder.SetSchema(schema)
	err := b.encoders[idx].encoder.Encode(datapoint, unit, annotation)
	if err != nil {
		return err
	}

	b.encoders[idx].lastWriteAt = datapoint.Timestamp
	return nil
}

func (b *BufferBucket) streams(ctx context.Context) []xio.BlockReader {
	streams := make([]xio.BlockReader, 0, len(b.loadedBlocks)+len(b.encoders))
	for _, bl := range b.loadedBlocks {
		if bl.Len() == 0 {
			continue
		}
		if s, err := bl.Stream(ctx); err == nil && s.IsNotEmpty() {
			// NB(r): block stream method will register the stream closer already
			streams = append(streams, s)
		}
	}
	for i := range b.encoders {
		start := b.start
		if s, ok := b.encoders[i].encoder.Stream(ctx); ok {
			br := xio.BlockReader{
				SegmentReader: s,
				Start:         start,
				BlockSize:     b.opts.RetentionOptions().BlockSize(),
			}
			ctx.RegisterFinalizer(s)
			streams = append(streams, br)
		}
	}

	return streams
}

func (b *BufferBucket) streamsLen() int {
	length := 0
	for i := range b.loadedBlocks {
		length += b.loadedBlocks[i].Len()
	}
	for i := range b.encoders {
		length += b.encoders[i].encoder.Len()
	}
	return length
}

func (b *BufferBucket) checksumIfSingleStream(ctx context.Context) (*uint32, error) {
	if b.hasJustSingleEncoder() {
		enc := b.encoders[0].encoder
		stream, ok := enc.Stream(ctx)
		if !ok {
			return nil, nil
		}

		segment, err := stream.Segment()
		if err != nil {
			return nil, err
		}

		if segment.Len() == 0 {
			return nil, nil
		}

		checksum := segment.CalculateChecksum()
		return &checksum, nil
	}

	if b.hasJustSingleLoadedBlock() {
		checksum, err := b.loadedBlocks[0].Checksum()
		if err != nil {
			return nil, err
		}
		return &checksum, nil
	}

	return nil, nil
}

func (b *BufferBucket) resetEncoders() {
	var zeroed inOrderEncoder
	for i := range b.encoders {
		// Register when this bucket resets we close the encoder.
		encoder := b.encoders[i].encoder
		encoder.Close()
		b.encoders[i] = zeroed
	}
	b.encoders = b.encoders[:0]
}

func (b *BufferBucket) resetLoadedBlocks() {
	for i := range b.loadedBlocks {
		bl := b.loadedBlocks[i]
		bl.Close()
	}
	b.loadedBlocks = nil
}

func (b *BufferBucket) needsMerge() bool {
	return !(b.hasJustSingleEncoder() || b.hasJustSingleLoadedBlock())
}

func (b *BufferBucket) hasJustSingleEncoder() bool {
	return len(b.encoders) == 1 && len(b.loadedBlocks) == 0
}

func (b *BufferBucket) hasJustSingleLoadedBlock() bool {
	encodersEmpty := len(b.encoders) == 0 ||
		(len(b.encoders) == 1 && b.encoders[0].encoder.Len() == 0)
	return encodersEmpty && len(b.loadedBlocks) == 1
}

func (b *BufferBucket) merge(nsCtx namespace.Context) (int, error) {
	if !b.needsMerge() {
		// Save unnecessary work
		return 0, nil
	}

	var (
		start   = b.start
		readers = make([]xio.SegmentReader, 0, len(b.encoders)+len(b.loadedBlocks))
		streams = make([]xio.SegmentReader, 0, len(b.encoders))
		ctx     = b.opts.ContextPool().Get()
		merges  = 0
	)
	defer func() {
		ctx.Close()
		// NB(r): Only need to close the mutable encoder streams as
		// the context we created for reading the loaded blocks
		// will close those streams when it is closed.
		for _, stream := range streams {
			stream.Finalize()
		}
	}()

	// Rank loaded blocks as data that has appeared before data that
	// arrived locally in the buffer
	for i := range b.loadedBlocks {
		block, err := b.loadedBlocks[i].Stream(ctx)
		if err == nil && block.SegmentReader != nil {
			merges++
			readers = append(readers, block.SegmentReader)
		}
	}

	for i := range b.encoders {
		if s, ok := b.encoders[i].encoder.Stream(ctx); ok {
			merges++
			readers = append(readers, s)
			streams = append(streams, s)
		}
	}

	encoder, lastWriteAt, err := mergeStreamsToEncoder(start, readers, b.opts, nsCtx)
	if err != nil {
		return 0, err
	}

	b.resetEncoders()
	b.resetLoadedBlocks()

	b.encoders = append(b.encoders, inOrderEncoder{
		encoder:     encoder,
		lastWriteAt: lastWriteAt,
	})

	return merges, nil
}

// mergeStreamsToEncoder merges streams to an encoder and returns the last
// write time. It is the responsibility of the caller to close the returned
// encoder when appropriate.
func mergeStreamsToEncoder(
	blockStart time.Time,
	streams []xio.SegmentReader,
	opts Options,
	nsCtx namespace.Context,
) (encoding.Encoder, time.Time, error) {
	bopts := opts.DatabaseBlockOptions()
	encoder := opts.EncoderPool().Get()
	encoder.Reset(blockStart, bopts.DatabaseBlockAllocSize(), nsCtx.Schema)
	iter := opts.MultiReaderIteratorPool().Get()
	defer iter.Close()

	var lastWriteAt time.Time
	iter.Reset(streams, blockStart, opts.RetentionOptions().BlockSize(), nsCtx.Schema)
	for iter.Next() {
		dp, unit, annotation := iter.Current()
		if err := encoder.Encode(dp, unit, annotation); err != nil {
			encoder.Close()
			return nil, timeZero, err
		}
		lastWriteAt = dp.Timestamp
	}
	if err := iter.Err(); err != nil {
		encoder.Close()
		return nil, timeZero, err
	}

	return encoder, lastWriteAt, nil
}

// mergeToStream merges all streams in this BufferBucket into one stream and
// returns it.
func (b *BufferBucket) mergeToStream(ctx context.Context, nsCtx namespace.Context) (xio.SegmentReader, bool, error) {
	if b.hasJustSingleEncoder() {
		b.resetLoadedBlocks()
		// Already merged as a single encoder.
		stream, ok := b.encoders[0].encoder.Stream(ctx)
		if !ok {
			return nil, false, nil
		}
		ctx.RegisterFinalizer(stream)
		return stream, true, nil
	}

	if b.hasJustSingleLoadedBlock() {
		// Need to reset encoders but do not want to finalize the block as we
		// are passing ownership of it to the caller.
		b.resetEncoders()
		stream, err := b.loadedBlocks[0].Stream(ctx)
		if err != nil {
			return nil, false, err
		}
		return stream, true, nil
	}

	_, err := b.merge(nsCtx)
	if err != nil {
		b.resetEncoders()
		b.resetLoadedBlocks()
		return nil, false, err
	}

	// After a successful merge, encoders and loaded blocks will be
	// reset, and the merged encoder appended as the only encoder in the
	// bucket.
	if !b.hasJustSingleEncoder() {
		return nil, false, errIncompleteMerge
	}

	stream, ok := b.encoders[0].encoder.Stream(ctx)
	if !ok {
		return nil, false, nil
	}
	ctx.RegisterFinalizer(stream)
	return stream, true, nil
}

// BufferBucketVersionsPool provides a pool for BufferBucketVersions.
type BufferBucketVersionsPool struct {
	pool pool.ObjectPool
}

// NewBufferBucketVersionsPool creates a new BufferBucketVersionsPool.
func NewBufferBucketVersionsPool(opts pool.ObjectPoolOptions) *BufferBucketVersionsPool {
	p := &BufferBucketVersionsPool{pool: pool.NewObjectPool(opts)}
	p.pool.Init(func() interface{} {
		return &BufferBucketVersions{}
	})
	return p
}

// Get gets a BufferBucketVersions from the pool.
func (p *BufferBucketVersionsPool) Get() *BufferBucketVersions {
	return p.pool.Get().(*BufferBucketVersions)
}

// Put puts a BufferBucketVersions back into the pool.
func (p *BufferBucketVersionsPool) Put(buckets *BufferBucketVersions) {
	p.pool.Put(buckets)
}

// BufferBucketPool provides a pool for BufferBuckets.
type BufferBucketPool struct {
	pool pool.ObjectPool
}

// NewBufferBucketPool creates a new BufferBucketPool.
func NewBufferBucketPool(opts pool.ObjectPoolOptions) *BufferBucketPool {
	p := &BufferBucketPool{pool: pool.NewObjectPool(opts)}
	p.pool.Init(func() interface{} {
		return &BufferBucket{}
	})
	return p
}

// Get gets a BufferBucket from the pool.
func (p *BufferBucketPool) Get() *BufferBucket {
	return p.pool.Get().(*BufferBucket)
}

// Put puts a BufferBucket back into the pool.
func (p *BufferBucketPool) Put(bucket *BufferBucket) {
	p.pool.Put(bucket)
}
