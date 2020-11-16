// Copyright (c) 2020 Uber Technologies, Inc.
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

package block

import (
	"time"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/dbnode/sharding"
	"github.com/m3db/m3/src/dbnode/topology"
	"github.com/m3db/m3/src/dbnode/ts"
	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/clock"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
	xsync "github.com/m3db/m3/src/x/sync"
	xtime "github.com/m3db/m3/src/x/time"
)

// Metadata captures block metadata
type Metadata struct {
	ID       ident.ID
	Tags     ident.Tags
	Start    time.Time
	Size     int64
	Checksum *uint32
	LastRead time.Time
}

// ReplicaMetadata captures block metadata along with corresponding peer identifier
// for a single replica of a block
type ReplicaMetadata struct {
	Metadata

	Host topology.Host
}

// FilteredBlocksMetadataIter iterates over a list of blocks metadata results with filtering applied
type FilteredBlocksMetadataIter interface {
	// Next returns the next item if available
	Next() bool

	// Current returns the current id and block metadata
	Current() (ident.ID, Metadata)

	//  Error returns an error if encountered
	Err() error
}

// FetchBlockResult captures the block start time, the readers for the underlying streams, the
// corresponding checksum and any errors encountered.
type FetchBlockResult struct {
	Start      time.Time
	FirstWrite time.Time
	Blocks     []xio.BlockReader
	Err        error
}

// FetchBlocksMetadataOptions are options used when fetching blocks metadata.
type FetchBlocksMetadataOptions struct {
	IncludeSizes     bool
	IncludeChecksums bool
	IncludeLastRead  bool
	OnlyDisk         bool
}

// FetchBlockMetadataResult captures the block start time, the block size, and any errors encountered
type FetchBlockMetadataResult struct {
	Start    time.Time
	Size     int64
	Checksum *uint32
	LastRead time.Time
	Err      error
}

// FetchBlockMetadataResults captures a collection of FetchBlockMetadataResult
type FetchBlockMetadataResults interface {
	// Add adds a result to the slice
	Add(res FetchBlockMetadataResult)

	// Results returns the result slice
	Results() []FetchBlockMetadataResult

	// SortByTimeAscending sorts the results in time ascending order
	Sort()

	// Reset resets the results
	Reset()

	// Close performs cleanup
	Close()
}

// FetchBlocksMetadataResult captures the fetch results for multiple blocks.
type FetchBlocksMetadataResult struct {
	ID     ident.ID
	Tags   ident.TagIterator
	Blocks FetchBlockMetadataResults
}

// FetchBlocksMetadataResults captures a collection of FetchBlocksMetadataResult
type FetchBlocksMetadataResults interface {
	// Add adds a result to the slice
	Add(res FetchBlocksMetadataResult)

	// Results returns the result slice
	Results() []FetchBlocksMetadataResult

	// Reset resets the results
	Reset()

	// Close performs cleanup
	Close()
}

// NewDatabaseBlockFn creates a new database block.
type NewDatabaseBlockFn func() DatabaseBlock

// DatabaseBlock is the interface for a DatabaseBlock
type DatabaseBlock interface {
	// StartTime returns the start time of the block.
	StartTime() time.Time

	// BlockSize returns the duration of the block.
	BlockSize() time.Duration

	// SetLastReadTime sets the last read time of the block.
	SetLastReadTime(value time.Time)

	// LastReadTime returns the last read time of the block.
	LastReadTime() time.Time

	// Len returns the block length.
	Len() int

	// Checksum returns the block checksum.
	Checksum() (uint32, error)

	// Stream returns the encoded byte stream.
	Stream(blocker context.Context) (xio.BlockReader, error)

	// Merge will merge the current block with the specified block
	// when this block is read. Note: calling this twice
	// will simply overwrite the target for the block to merge with
	// rather than merging three blocks together.
	Merge(other DatabaseBlock) error

	// HasMergeTarget returns whether the block requires multiple blocks to be
	// merged during Stream().
	HasMergeTarget() bool

	// WasRetrievedFromDisk returns whether the block was retrieved from storage.
	WasRetrievedFromDisk() bool

	// Reset resets the block start time, duration, and the segment.
	Reset(startTime time.Time, blockSize time.Duration, segment ts.Segment, nsCtx namespace.Context)

	// ResetFromDisk resets the block start time, duration, segment, and id.
	ResetFromDisk(
		startTime time.Time,
		blockSize time.Duration,
		segment ts.Segment,
		id ident.ID,
		nsCtx namespace.Context,
	)

	// Discard closes the block, but returns the (unfinalized) segment.
	Discard() ts.Segment

	// Close closes the block.
	Close()

	// CloseIfFromDisk atomically checks if the disk was retrieved from disk, and
	// if so, closes it. It is meant as a layered protection for the WiredList
	// which should only close blocks that were retrieved from disk.
	CloseIfFromDisk() bool

	// SetOnEvictedFromWiredList sets the owner of the block
	SetOnEvictedFromWiredList(OnEvictedFromWiredList)

	// OnEvictedFromWiredList returns the owner of the block
	OnEvictedFromWiredList() OnEvictedFromWiredList

	// Private methods because only the Wired List itself should use them.
	databaseBlock
}

// databaseBlock is the private portion of the DatabaseBlock interface
type databaseBlock interface {
	next() DatabaseBlock
	setNext(block DatabaseBlock)
	prev() DatabaseBlock
	setPrev(block DatabaseBlock)
	enteredListAtUnixNano() int64
	setEnteredListAtUnixNano(value int64)
	wiredListEntry() wiredListEntry
}

// OnEvictedFromWiredList is implemented by a struct that wants to be notified
// when a block is evicted from the wired list.
type OnEvictedFromWiredList interface {
	// OnEvictedFromWiredList is called when a block is evicted from the wired list.
	OnEvictedFromWiredList(id ident.ID, blockStart time.Time)
}

// OnRetrieveBlock is an interface to callback on when a block is retrieved.
type OnRetrieveBlock interface {
	OnRetrieveBlock(
		id ident.ID,
		tags ident.TagIterator,
		startTime time.Time,
		segment ts.Segment,
		nsCtx namespace.Context,
	)
}

// OnReadBlock is an interface to callback on when a block is read.
type OnReadBlock interface {
	OnReadBlock(b DatabaseBlock)
}

// OnRetrieveBlockFn is a function implementation for the
// OnRetrieveBlock interface.
type OnRetrieveBlockFn func(
	id ident.ID,
	tags ident.TagIterator,
	startTime time.Time,
	segment ts.Segment,
	nsCtx namespace.Context,
)

// OnRetrieveBlock implements the OnRetrieveBlock interface.
func (fn OnRetrieveBlockFn) OnRetrieveBlock(
	id ident.ID,
	tags ident.TagIterator,
	startTime time.Time,
	segment ts.Segment,
	nsCtx namespace.Context,
) {
	fn(id, tags, startTime, segment, nsCtx)
}

// RetrievableBlockMetadata describes a retrievable block.
type RetrievableBlockMetadata struct {
	ID       ident.ID
	Length   int
	Checksum uint32
}

// DatabaseBlockRetriever is a block retriever.
type DatabaseBlockRetriever interface {
	// CacheShardIndices will pre-parse the indexes for given shards
	// to improve times when streaming a block.
	CacheShardIndices(shards []uint32) error

	// Stream will stream a block for a given shard, id and start.
	Stream(
		ctx context.Context,
		shard uint32,
		id ident.ID,
		blockStart time.Time,
		onRetrieve OnRetrieveBlock,
		nsCtx namespace.Context,
	) (xio.BlockReader, error)

	AssignShardSet(shardSet sharding.ShardSet)
}

// DatabaseShardBlockRetriever is a block retriever bound to a shard.
type DatabaseShardBlockRetriever interface {
	// Stream will stream a block for a given id and start.
	Stream(
		ctx context.Context,
		id ident.ID,
		blockStart time.Time,
		onRetrieve OnRetrieveBlock,
		nsCtx namespace.Context,
	) (xio.BlockReader, error)
}

// DatabaseBlockRetrieverManager creates and holds block retrievers
// for different namespaces.
type DatabaseBlockRetrieverManager interface {
	// Retriever provides the DatabaseBlockRetriever for the given namespace.
	Retriever(
		nsMetadata namespace.Metadata,
		shardSet sharding.ShardSet,
	) (DatabaseBlockRetriever, error)
}

// DatabaseShardBlockRetrieverManager creates and holds shard block
// retrievers binding shards to an existing retriever.
type DatabaseShardBlockRetrieverManager interface {
	// ShardRetriever provides the DatabaseShardBlockRetriever for the given shard.
	ShardRetriever(shard uint32) DatabaseShardBlockRetriever
}

// DatabaseSeriesBlocks represents a collection of data blocks.
type DatabaseSeriesBlocks interface {
	// Len returns the number of blocks contained in the collection.
	Len() int

	// AddBlock adds a data block.
	AddBlock(block DatabaseBlock)

	// AddSeries adds a raw series.
	AddSeries(other DatabaseSeriesBlocks)

	// MinTime returns the min time of the blocks contained.
	MinTime() time.Time

	// MaxTime returns the max time of the blocks contained.
	MaxTime() time.Time

	// BlockAt returns the block at a given time if any.
	BlockAt(t time.Time) (DatabaseBlock, bool)

	// AllBlocks returns all the blocks in the series.
	AllBlocks() map[xtime.UnixNano]DatabaseBlock

	// RemoveBlockAt removes the block at a given time if any.
	RemoveBlockAt(t time.Time)

	// RemoveAll removes all blocks.
	RemoveAll()

	// Reset resets the DatabaseSeriesBlocks so they can be re-used
	Reset()

	// Close closes all the blocks.
	Close()
}

// DatabaseBlockAllocate allocates a database block for a pool.
type DatabaseBlockAllocate func() DatabaseBlock

// DatabaseBlockPool provides a pool for database blocks.
type DatabaseBlockPool interface {
	// Init initializes the pool.
	Init(alloc DatabaseBlockAllocate)

	// Get provides a database block from the pool.
	Get() DatabaseBlock

	// Put returns a database block to the pool.
	Put(block DatabaseBlock)
}

// FetchBlockMetadataResultsPool provides a pool for fetchBlockMetadataResults
type FetchBlockMetadataResultsPool interface {
	// Get returns an FetchBlockMetadataResults
	Get() FetchBlockMetadataResults

	// Put puts an FetchBlockMetadataResults back to pool
	Put(res FetchBlockMetadataResults)
}

// FetchBlocksMetadataResultsPool provides a pool for fetchBlocksMetadataResults
type FetchBlocksMetadataResultsPool interface {
	// Get returns an fetchBlocksMetadataResults
	Get() FetchBlocksMetadataResults

	// Put puts an fetchBlocksMetadataResults back to pool
	Put(res FetchBlocksMetadataResults)
}

// LeaseManager is a manager of block leases and leasers.
type LeaseManager interface {
	// RegisterLeaser registers the leaser to receive UpdateOpenLease()
	// calls when leases need to be updated.
	RegisterLeaser(leaser Leaser) error
	// UnregisterLeaser unregisters the leaser from receiving UpdateOpenLease()
	// calls.
	UnregisterLeaser(leaser Leaser) error
	// OpenLease opens a lease.
	OpenLease(
		leaser Leaser,
		descriptor LeaseDescriptor,
		state LeaseState,
	) error
	// OpenLatestLease opens a lease for the latest LeaseState for a given
	// LeaseDescriptor.
	OpenLatestLease(leaser Leaser, descriptor LeaseDescriptor) (LeaseState, error)
	// UpdateOpenLeases propagate a call to UpdateOpenLease() to each registered
	// leaser.
	UpdateOpenLeases(
		descriptor LeaseDescriptor,
		state LeaseState,
	) (UpdateLeasesResult, error)
	// SetLeaseVerifier sets the LeaseVerifier (for delayed initialization).
	SetLeaseVerifier(leaseVerifier LeaseVerifier) error
}

// UpdateLeasesResult is the result of a call to update leases.
type UpdateLeasesResult struct {
	LeasersUpdatedLease int
	LeasersNoOpenLease  int
}

// LeaseDescriptor describes a lease (like an ID).
type LeaseDescriptor struct {
	Namespace  ident.ID
	Shard      uint32
	BlockStart time.Time
}

// LeaseState is the current state of a lease which can be
// requested to be updated.
type LeaseState struct {
	Volume int
}

// LeaseVerifier verifies that a lease is valid.
type LeaseVerifier interface {
	// VerifyLease is called to determine if the requested lease is valid.
	VerifyLease(
		descriptor LeaseDescriptor,
		state LeaseState,
	) error

	// LatestState returns the latest LeaseState for a given descriptor.
	LatestState(descriptor LeaseDescriptor) (LeaseState, error)
}

// UpdateOpenLeaseResult is the result of processing an update lease.
type UpdateOpenLeaseResult uint

const (
	// UpdateOpenLease is used to communicate a lease updated successfully.
	UpdateOpenLease UpdateOpenLeaseResult = iota
	// NoOpenLease is used to communicate there is no related open lease.
	NoOpenLease
)

// Leaser is a block leaser.
type Leaser interface {
	// UpdateOpenLease is called on the Leaser when the latest state
	// has changed and the leaser needs to update their lease. The leaser
	// should update its state (releasing any resources as necessary and
	// optionally acquiring new ones related to the updated lease) accordingly,
	// but it should *not* call LeaseManager.OpenLease() with the provided
	// descriptor and state.
	//
	// UpdateOpenLease will never be called concurrently on the same Leaser. Each
	// call to UpdateOpenLease() must return before the next one will begin.
	UpdateOpenLease(
		descriptor LeaseDescriptor,
		state LeaseState,
	) (UpdateOpenLeaseResult, error)
}

// Options represents the options for a database block
type Options interface {
	// SetClockOptions sets the clock options
	SetClockOptions(value clock.Options) Options

	// ClockOptions returns the clock options
	ClockOptions() clock.Options

	// SetDatabaseBlockAllocSize sets the databaseBlockAllocSize
	SetDatabaseBlockAllocSize(value int) Options

	// DatabaseBlockAllocSize returns the databaseBlockAllocSize
	DatabaseBlockAllocSize() int

	// SetCloseContextWorkers sets the workers for closing contexts
	SetCloseContextWorkers(value xsync.WorkerPool) Options

	// CloseContextWorkers returns the workers for closing contexts
	CloseContextWorkers() xsync.WorkerPool

	// SetDatabaseBlockPool sets the databaseBlockPool
	SetDatabaseBlockPool(value DatabaseBlockPool) Options

	// DatabaseBlockPool returns the databaseBlockPool
	DatabaseBlockPool() DatabaseBlockPool

	// SetContextPool sets the contextPool
	SetContextPool(value context.Pool) Options

	// ContextPool returns the contextPool
	ContextPool() context.Pool

	// SetEncoderPool sets the contextPool
	SetEncoderPool(value encoding.EncoderPool) Options

	// EncoderPool returns the contextPool
	EncoderPool() encoding.EncoderPool

	// SetReaderIteratorPool sets the readerIteratorPool
	SetReaderIteratorPool(value encoding.ReaderIteratorPool) Options

	// ReaderIteratorPool returns the readerIteratorPool
	ReaderIteratorPool() encoding.ReaderIteratorPool

	// SetMultiReaderIteratorPool sets the multiReaderIteratorPool
	SetMultiReaderIteratorPool(value encoding.MultiReaderIteratorPool) Options

	// MultiReaderIteratorPool returns the multiReaderIteratorPool
	MultiReaderIteratorPool() encoding.MultiReaderIteratorPool

	// SetSegmentReaderPool sets the contextPool
	SetSegmentReaderPool(value xio.SegmentReaderPool) Options

	// SegmentReaderPool returns the contextPool
	SegmentReaderPool() xio.SegmentReaderPool

	// SetBytesPool sets the bytesPool
	SetBytesPool(value pool.CheckedBytesPool) Options

	// BytesPool returns the bytesPool
	BytesPool() pool.CheckedBytesPool

	// SetWiredList sets the database block wired list
	SetWiredList(value *WiredList) Options

	// WiredList returns the database block wired list
	WiredList() *WiredList
}
