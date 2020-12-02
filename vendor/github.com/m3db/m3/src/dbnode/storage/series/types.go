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
	"time"

	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/persist"
	"github.com/m3db/m3/src/dbnode/retention"
	"github.com/m3db/m3/src/dbnode/runtime"
	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	xtime "github.com/m3db/m3/src/x/time"

	"github.com/uber-go/tally"
)

// DatabaseSeriesOptions is a set of options for creating a database series.
type DatabaseSeriesOptions struct {
	ID                     ident.ID
	Metadata               doc.Document
	UniqueIndex            uint64
	BlockRetriever         QueryableBlockRetriever
	OnRetrieveBlock        block.OnRetrieveBlock
	OnEvictedFromWiredList block.OnEvictedFromWiredList
	Options                Options
}

// DatabaseSeries is a series in the database.
type DatabaseSeries interface {
	block.OnRetrieveBlock
	block.OnEvictedFromWiredList

	// ID returns the ID of the series.
	ID() ident.ID

	// Metadata returns the metadata of the series.
	Metadata() doc.Document

	// UniqueIndex is the unique index for the series (for this current
	// process, unless the time series expires).
	UniqueIndex() uint64

	// Tick executes async updates
	Tick(blockStates ShardBlockStateSnapshot, nsCtx namespace.Context) (TickResult, error)

	// Write writes a new value.
	Write(
		ctx context.Context,
		timestamp time.Time,
		value float64,
		unit xtime.Unit,
		annotation []byte,
		wOpts WriteOptions,
	) (bool, WriteType, error)

	// ReadEncoded reads encoded blocks.
	ReadEncoded(
		ctx context.Context,
		start, end time.Time,
		nsCtx namespace.Context,
	) ([][]xio.BlockReader, error)

	// FetchBlocks returns data blocks given a list of block start times.
	FetchBlocks(
		ctx context.Context,
		starts []time.Time,
		nsCtx namespace.Context,
	) ([]block.FetchBlockResult, error)

	// FetchBlocksForColdFlush fetches blocks for a cold flush. This function
	// informs the series and the buffer that a cold flush for the specified
	// block start is occurring so that it knows to update bucket versions.
	FetchBlocksForColdFlush(
		ctx context.Context,
		start time.Time,
		version int,
		nsCtx namespace.Context,
	) (block.FetchBlockResult, error)

	// FetchBlocksMetadata returns the blocks metadata.
	FetchBlocksMetadata(
		ctx context.Context,
		start, end time.Time,
		opts FetchBlocksMetadataOptions,
	) (block.FetchBlocksMetadataResult, error)

	// IsEmpty returns whether series is empty (includes both cached blocks and in-mem buffer data).
	IsEmpty() bool

	// IsBufferEmptyAtBlockStart returns whether the series buffer is empty at block start
	// (only checks for in-mem buffer data).
	IsBufferEmptyAtBlockStart(time.Time) bool

	// NumActiveBlocks returns the number of active blocks the series currently holds.
	NumActiveBlocks() int

	/// LoadBlock loads a single block into the series.
	LoadBlock(
		block block.DatabaseBlock,
		writeType WriteType,
	) error

	// WarmFlush flushes the WarmWrites of this series for a given start time.
	WarmFlush(
		ctx context.Context,
		blockStart time.Time,
		persistFn persist.DataFn,
		nsCtx namespace.Context,
	) (FlushOutcome, error)

	// Snapshot snapshots the buffer buckets of this series for any data that has
	// not been rotated into a block yet.
	Snapshot(
		ctx context.Context,
		blockStart time.Time,
		persistFn persist.DataFn,
		nsCtx namespace.Context,
	) (SnapshotResult, error)

	// ColdFlushBlockStarts returns the block starts that need cold flushes.
	ColdFlushBlockStarts(blockStates BootstrappedBlockStateSnapshot) OptimizedTimes

	// Bootstrap will moved any bootstrapped data to buffer so series
	// is ready for reading.
	Bootstrap(nsCtx namespace.Context) error

	// Close will close the series and if pooled returned to the pool.
	Close()

	// Reset resets the series for reuse.
	Reset(opts DatabaseSeriesOptions)
}

// SnapshotResult contains metadata regarding the snapshot.
type SnapshotResult struct {
	Persist bool
	Stats   SnapshotResultStats
}

// SnapshotResultStats contains stats regarding the snapshot.
type SnapshotResultStats struct {
	TimeMergeByBucket      time.Duration
	TimeMergeAcrossBuckets time.Duration
	TimeChecksum           time.Duration
	TimePersist            time.Duration
}

// Add adds the result of a snapshot result to this result.
func (r *SnapshotResultStats) Add(other SnapshotResultStats) {
	r.TimeMergeByBucket += other.TimeMergeByBucket
	r.TimeMergeAcrossBuckets += other.TimeMergeAcrossBuckets
	r.TimeChecksum += other.TimeChecksum
	r.TimePersist += other.TimePersist
}

// FetchBlocksMetadataOptions encapsulates block fetch metadata options
// and specifies a few series specific options too.
type FetchBlocksMetadataOptions struct {
	block.FetchBlocksMetadataOptions
}

// QueryableBlockRetriever is a block retriever that can tell if a block
// is retrievable or not for a given start time.
type QueryableBlockRetriever interface {
	block.DatabaseShardBlockRetriever

	// IsBlockRetrievable returns whether a block is retrievable
	// for a given block start time.
	IsBlockRetrievable(blockStart time.Time) (bool, error)

	// RetrievableBlockColdVersion returns the cold version that was
	// successfully persisted.
	RetrievableBlockColdVersion(blockStart time.Time) (int, error)

	// BlockStatesSnapshot returns a snapshot of the whether blocks are
	// retrievable and their flush versions for each block start. This is used
	// to reduce lock contention of acquiring flush state.
	//
	// Flushes may occur and change the actual block state while iterating
	// through this snapshot, so any logic using this function should take this
	// into account.
	BlockStatesSnapshot() ShardBlockStateSnapshot
}

// ShardBlockStateSnapshot represents a snapshot of a shard's block state at
// a moment in time.
type ShardBlockStateSnapshot struct {
	bootstrapped bool
	snapshot     BootstrappedBlockStateSnapshot
}

// NewShardBlockStateSnapshot constructs a new NewShardBlockStateSnapshot.
func NewShardBlockStateSnapshot(
	bootstrapped bool,
	snapshot BootstrappedBlockStateSnapshot,
) ShardBlockStateSnapshot {
	return ShardBlockStateSnapshot{
		bootstrapped: bootstrapped,
		snapshot:     snapshot,
	}
}

// UnwrapValue returns a BootstrappedBlockStateSnapshot and a boolean indicating whether the
// snapshot is bootstrapped or not.
func (s ShardBlockStateSnapshot) UnwrapValue() (BootstrappedBlockStateSnapshot, bool) {
	return s.snapshot, s.bootstrapped
}

// BootstrappedBlockStateSnapshot represents a bootstrapped shard block state snapshot.
type BootstrappedBlockStateSnapshot struct {
	Snapshot map[xtime.UnixNano]BlockState
}

// BlockState contains the state of a block.
type BlockState struct {
	WarmRetrievable bool
	ColdVersion     int
}

// TickStatus is the status of a series for a given tick.
type TickStatus struct {
	// ActiveBlocks is the number of total active blocks.
	ActiveBlocks int
	// WiredBlocks is the number of blocks wired in memory (all data kept)
	WiredBlocks int
	// UnwiredBlocks is the number of blocks unwired (data kept on disk).
	UnwiredBlocks int
	// PendingMergeBlocks is the number of blocks pending merges.
	PendingMergeBlocks int
}

// TickResult is a set of results from a tick.
type TickResult struct {
	TickStatus
	// MadeExpiredBlocks is count of blocks just expired.
	MadeExpiredBlocks int
	// MadeUnwiredBlocks is count of blocks just unwired from memory.
	MadeUnwiredBlocks int
	// MergedOutOfOrderBlocks is count of blocks merged from out of order streams.
	MergedOutOfOrderBlocks int
	// EvictedBuckets is count of buckets just evicted from the buffer map.
	EvictedBuckets int
}

// DatabaseSeriesAllocate allocates a database series for a pool.
type DatabaseSeriesAllocate func() DatabaseSeries

// DatabaseSeriesPool provides a pool for database series.
type DatabaseSeriesPool interface {
	// Get provides a database series from the pool.
	Get() DatabaseSeries

	// Put returns a database series to the pool.
	Put(block DatabaseSeries)
}

// FlushOutcome is an enum that provides more context about the outcome
// of series.WarmFlush() to the caller.
type FlushOutcome int

const (
	// FlushOutcomeErr is just a default value that can be returned when we're
	// also returning an error.
	FlushOutcomeErr FlushOutcome = iota
	// FlushOutcomeBlockDoesNotExist indicates that the series did not have a
	// block for the specified flush blockStart.
	FlushOutcomeBlockDoesNotExist
	// FlushOutcomeFlushedToDisk indicates that a block existed and was flushed
	// to disk successfully.
	FlushOutcomeFlushedToDisk
)

// Options represents the options for series
type Options interface {
	// Validate validates the options
	Validate() error

	// SetClockOptions sets the clock options
	SetClockOptions(value clock.Options) Options

	// ClockOptions returns the clock options
	ClockOptions() clock.Options

	// SetInstrumentOptions sets the instrumentation options
	SetInstrumentOptions(value instrument.Options) Options

	// InstrumentOptions returns the instrumentation options
	InstrumentOptions() instrument.Options

	// SetRetentionOptions sets the retention options
	SetRetentionOptions(value retention.Options) Options

	// RetentionOptions returns the retention options
	RetentionOptions() retention.Options

	// SetDatabaseBlockOptions sets the database block options
	SetDatabaseBlockOptions(value block.Options) Options

	// DatabaseBlockOptions returns the database block options
	DatabaseBlockOptions() block.Options

	// SetCachePolicy sets the series cache policy
	SetCachePolicy(value CachePolicy) Options

	// CachePolicy returns the series cache policy
	CachePolicy() CachePolicy

	// SetContextPool sets the contextPool
	SetContextPool(value context.Pool) Options

	// ContextPool returns the contextPool
	ContextPool() context.Pool

	// SetEncoderPool sets the contextPool
	SetEncoderPool(value encoding.EncoderPool) Options

	// EncoderPool returns the contextPool
	EncoderPool() encoding.EncoderPool

	// SetMultiReaderIteratorPool sets the multiReaderIteratorPool
	SetMultiReaderIteratorPool(value encoding.MultiReaderIteratorPool) Options

	// MultiReaderIteratorPool returns the multiReaderIteratorPool
	MultiReaderIteratorPool() encoding.MultiReaderIteratorPool

	// SetFetchBlockMetadataResultsPool sets the fetchBlockMetadataResultsPool
	SetFetchBlockMetadataResultsPool(value block.FetchBlockMetadataResultsPool) Options

	// FetchBlockMetadataResultsPool returns the fetchBlockMetadataResultsPool
	FetchBlockMetadataResultsPool() block.FetchBlockMetadataResultsPool

	// SetIdentifierPool sets the identifierPool
	SetIdentifierPool(value ident.Pool) Options

	// IdentifierPool returns the identifierPool
	IdentifierPool() ident.Pool

	// SetStats sets the configured Stats.
	SetStats(value Stats) Options

	// Stats returns the configured Stats.
	Stats() Stats

	// SetColdWritesEnabled sets whether cold writes are enabled.
	SetColdWritesEnabled(value bool) Options

	// ColdWritesEnabled returns whether cold writes are enabled.
	ColdWritesEnabled() bool

	// SetBufferBucketVersionsPool sets the BufferBucketVersionsPool.
	SetBufferBucketVersionsPool(value *BufferBucketVersionsPool) Options

	// BufferBucketVersionsPool returns the BufferBucketVersionsPool.
	BufferBucketVersionsPool() *BufferBucketVersionsPool

	// SetBufferBucketPool sets the BufferBucketPool.
	SetBufferBucketPool(value *BufferBucketPool) Options

	// BufferBucketPool returns the BufferBucketPool.
	BufferBucketPool() *BufferBucketPool

	// SetRuntimeOptionsManager sets the runtime options manager.
	SetRuntimeOptionsManager(value runtime.OptionsManager) Options

	// RuntimeOptionsManager returns the runtime options manager.
	RuntimeOptionsManager() runtime.OptionsManager
}

// Stats is passed down from namespace/shard to avoid allocations per series.
type Stats struct {
	encoderCreated            tally.Counter
	coldWrites                tally.Counter
	encodersPerBlock          tally.Histogram
	encoderLimitWriteRejected tally.Counter
	snapshotMergesEachBucket  tally.Counter
}

// NewStats returns a new Stats for the provided scope.
func NewStats(scope tally.Scope) Stats {
	subScope := scope.SubScope("series")

	buckets := append(tally.ValueBuckets{0},
		tally.MustMakeExponentialValueBuckets(1, 2, 20)...)
	return Stats{
		encoderCreated:            subScope.Counter("encoder-created"),
		coldWrites:                subScope.Counter("cold-writes"),
		encodersPerBlock:          subScope.Histogram("encoders-per-block", buckets),
		encoderLimitWriteRejected: subScope.Counter("encoder-limit-write-rejected"),
		snapshotMergesEachBucket:  subScope.Counter("snapshot-merges-each-bucket"),
	}
}

// IncCreatedEncoders incs the EncoderCreated stat.
func (s Stats) IncCreatedEncoders() {
	s.encoderCreated.Inc(1)
}

// IncColdWrites incs the ColdWrites stat.
func (s Stats) IncColdWrites() {
	s.coldWrites.Inc(1)
}

// RecordEncodersPerBlock records the number of encoders histogram.
func (s Stats) RecordEncodersPerBlock(num int) {
	s.encodersPerBlock.RecordValue(float64(num))
}

// IncEncoderLimitWriteRejected incs the encoderLimitWriteRejected stat.
func (s Stats) IncEncoderLimitWriteRejected() {
	s.encoderLimitWriteRejected.Inc(1)
}

// WriteType is an enum for warm/cold write types.
type WriteType int

const (
	// WarmWrite represents warm writes (within the buffer past/future window).
	WarmWrite WriteType = iota

	// ColdWrite represents cold writes (outside the buffer past/future window).
	ColdWrite
)

// WriteTransformOptions describes transforms to run on incoming writes.
type WriteTransformOptions struct {
	// ForceValueEnabled indicates if the values for incoming writes
	// should be forced to `ForceValue`.
	ForceValueEnabled bool
	// ForceValue is the value that incoming writes should be forced to.
	ForceValue float64
}

// WriteOptions provides a set of options for a write.
type WriteOptions struct {
	// SchemaDesc is the schema description.
	SchemaDesc namespace.SchemaDescr
	// TruncateType is the truncation type for incoming writes.
	TruncateType TruncateType
	// TransformOptions describes transformation options for incoming writes.
	TransformOptions WriteTransformOptions
	// BootstrapWrite allows a warm write outside the time window as long as the
	// block hasn't already been flushed to disk. This is useful for
	// bootstrappers filling data that they know has not yet been flushed to
	// disk.
	BootstrapWrite bool
	// SkipOutOfRetention allows for skipping writes that are out of retention
	// by just returning success, this allows for callers to not have to
	// deal with clock skew when they are trying to write a value that may not
	// fall into retention but they do not care if it fails to write due to
	// it just having fallen out of retention (time race).
	SkipOutOfRetention bool
}
