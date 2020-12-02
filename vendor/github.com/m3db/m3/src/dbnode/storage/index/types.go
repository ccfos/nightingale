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

package index

import (
	"fmt"
	"sort"
	"time"

	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/storage/bootstrap/result"
	"github.com/m3db/m3/src/dbnode/storage/index/compaction"
	"github.com/m3db/m3/src/dbnode/storage/limits"
	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/idx"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/index/segment/builder"
	"github.com/m3db/m3/src/m3ninx/index/segment/fst"
	"github.com/m3db/m3/src/m3ninx/index/segment/mem"
	"github.com/m3db/m3/src/x/context"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/mmap"
	"github.com/m3db/m3/src/x/pool"
	"github.com/m3db/m3/src/x/resource"
	xtime "github.com/m3db/m3/src/x/time"

	opentracinglog "github.com/opentracing/opentracing-go/log"
)

var (
	// ReservedFieldNameID is the field name used to index the ID in the
	// m3ninx subsytem.
	ReservedFieldNameID = doc.IDReservedFieldName
)

// InsertMode specifies whether inserts are synchronous or asynchronous.
type InsertMode byte

// nolint
const (
	InsertSync InsertMode = iota
	InsertAsync
)

// AggregationType specifies what granularity to aggregate upto.
type AggregationType uint8

const (
	// AggregateTagNamesAndValues returns both the tag name and value.
	AggregateTagNamesAndValues AggregationType = iota
	// AggregateTagNames returns tag names only.
	AggregateTagNames
)

// Query is a rich end user query to describe a set of constraints on required IDs.
type Query struct {
	idx.Query
}

// QueryOptions enables users to specify constraints and
// preferences on query execution.
type QueryOptions struct {
	StartInclusive    time.Time
	EndExclusive      time.Time
	SeriesLimit       int
	DocsLimit         int
	RequireExhaustive bool
}

// IterationOptions enables users to specify iteration preferences.
type IterationOptions struct {
	SeriesIteratorConsolidator encoding.SeriesIteratorConsolidator
}

// SeriesLimitExceeded returns whether a given size exceeds the
// series limit the query options imposes, if it is enabled.
func (o QueryOptions) SeriesLimitExceeded(size int) bool {
	return o.SeriesLimit > 0 && size >= o.SeriesLimit
}

// DocsLimitExceeded returns whether a given size exceeds the
// docs limit the query options imposes, if it is enabled.
func (o QueryOptions) DocsLimitExceeded(size int) bool {
	return o.DocsLimit > 0 && size >= o.DocsLimit
}

// AggregationOptions enables users to specify constraints on aggregations.
type AggregationOptions struct {
	QueryOptions
	FieldFilter AggregateFieldFilter
	Type        AggregationType
}

// QueryResult is the collection of results for a query.
type QueryResult struct {
	Results    QueryResults
	Exhaustive bool
}

// AggregateQueryResult is the collection of results for an aggregate query.
type AggregateQueryResult struct {
	Results    AggregateResults
	Exhaustive bool
}

// BaseResults is a collection of basic results for a generic query, it is
// synchronized when access to the results set is used as documented by the
// methods.
type BaseResults interface {
	// Namespace returns the namespace associated with the result.
	Namespace() ident.ID

	// Size returns the number of IDs tracked.
	Size() int

	// TotalDocsCount returns the total number of documents observed.
	TotalDocsCount() int

	// AddDocuments adds the batch of documents to the results set, it will
	// take a copy of the bytes backing the documents so the original can be
	// modified after this function returns without affecting the results map.
	// TODO(r): We will need to change this behavior once index fields are
	// mutable and the most recent need to shadow older entries.
	AddDocuments(batch []doc.Document) (size, docsCount int, err error)

	// Finalize releases any resources held by the Results object,
	// including returning it to a backing pool.
	Finalize()
}

// QueryResults is a collection of results for a query, it is synchronized
// when access to the results set is used as documented by the methods.
type QueryResults interface {
	BaseResults

	// Reset resets the Results object to initial state.
	Reset(nsID ident.ID, opts QueryResultsOptions)

	// Map returns the results map from seriesID -> seriesTags, comprising
	// index results.
	// Since a lock is not held when accessing the map after a call to this
	// method, it is unsafe to read or write to the map if any other caller
	// mutates the state of the results after obtaining a reference to the map
	// with this call.
	Map() *ResultsMap
}

// QueryResultsOptions is a set of options to use for query results.
type QueryResultsOptions struct {
	// SizeLimit will limit the total results set to a given limit and if
	// overflown will return early successfully.
	SizeLimit int

	// FilterID, if provided, can be used to filter out unwanted IDs from
	// the query results.
	// NB(r): This is used to filter out results from shards the DB node
	// node no longer owns but is still included in index segments.
	FilterID func(id ident.ID) bool
}

// QueryResultsAllocator allocates QueryResults types.
type QueryResultsAllocator func() QueryResults

// QueryResultsPool allows users to pool `Results` types.
type QueryResultsPool interface {
	// Init initializes the QueryResults pool.
	Init(alloc QueryResultsAllocator)

	// Get retrieves a QueryResults object for use.
	Get() QueryResults

	// Put returns the provided QueryResults to the pool.
	Put(value QueryResults)
}

// AggregateResults is a collection of results for an aggregation query, it is
// synchronized when access to the results set is used as documented by the
// methods.
type AggregateResults interface {
	BaseResults

	// Reset resets the AggregateResults object to initial state.
	Reset(
		nsID ident.ID,
		aggregateQueryOpts AggregateResultsOptions,
	)

	// AggregateResultsOptions returns the options for this AggregateResult.
	AggregateResultsOptions() AggregateResultsOptions

	// AddFields adds the batch of fields to the results set, it will
	// assume ownership of the idents (and backing bytes) provided to it.
	// i.e. it is not safe to use/modify the idents once this function returns.
	AddFields(
		batch []AggregateResultsEntry,
	) (size, docsCount int)

	// Map returns a map from tag name -> possible tag values,
	// comprising aggregate results.
	// Since a lock is not held when accessing the map after a call to this
	// method, it is unsafe to read or write to the map if any other caller
	// mutates the state of the results after obtaining a reference to the map
	// with this call.
	Map() *AggregateResultsMap
}

// AggregateFieldFilter dictates which fields will appear in the aggregated
// result; if filter values exist, only those whose fields matches a value in the
// filter are returned.
type AggregateFieldFilter [][]byte

// AggregateResultsOptions is a set of options to use for results.
type AggregateResultsOptions struct {
	// SizeLimit will limit the total results set to a given limit and if
	// overflown will return early successfully.
	SizeLimit int

	// Type determines what result is required.
	Type AggregationType

	// FieldFilter is an optional param to filter aggregate values.
	FieldFilter AggregateFieldFilter

	// RestrictByQuery is a query to restrict the set of documents that must
	// be present for an aggregated term to be returned.
	RestrictByQuery *Query
}

// AggregateResultsAllocator allocates AggregateResults types.
type AggregateResultsAllocator func() AggregateResults

// AggregateResultsPool allows users to pool `AggregateResults` types.
type AggregateResultsPool interface {
	// Init initializes the AggregateResults pool.
	Init(alloc AggregateResultsAllocator)

	// Get retrieves a AggregateResults object for use.
	Get() AggregateResults

	// Put returns the provided AggregateResults to the pool.
	Put(value AggregateResults)
}

// AggregateValuesAllocator allocates AggregateValues types.
type AggregateValuesAllocator func() AggregateValues

// AggregateValuesPool allows users to pool `AggregateValues` types.
type AggregateValuesPool interface {
	// Init initializes the AggregateValues pool.
	Init(alloc AggregateValuesAllocator)

	// Get retrieves a AggregateValues object for use.
	Get() AggregateValues

	// Put returns the provided AggregateValues to the pool.
	Put(value AggregateValues)
}

// AggregateResultsEntry is used during block.Aggregate() execution
// to collect entries.
type AggregateResultsEntry struct {
	Field ident.ID
	Terms []ident.ID
}

// OnIndexSeries provides a set of callback hooks to allow the reverse index
// to do lifecycle management of any resources retained during indexing.
type OnIndexSeries interface {
	// OnIndexSuccess is executed when an entry is successfully indexed. The
	// provided value for `blockStart` is the blockStart for which the write
	// was indexed.
	OnIndexSuccess(blockStart xtime.UnixNano)

	// OnIndexFinalize is executed when the index no longer holds any references
	// to the provided resources. It can be used to cleanup any resources held
	// during the course of indexing. `blockStart` is the startTime of the index
	// block for which the write was attempted.
	OnIndexFinalize(blockStart xtime.UnixNano)

	// OnIndexPrepare prepares the Entry to be handed off to the indexing sub-system.
	// NB(prateek): we retain the ref count on the entry while the indexing is pending,
	// the callback executed on the entry once the indexing is completed releases this
	// reference.
	OnIndexPrepare()

	// NeedsIndexUpdate returns a bool to indicate if the Entry needs to be indexed
	// for the provided blockStart. It only allows a single index attempt at a time
	// for a single entry.
	// NB(prateek): NeedsIndexUpdate is a CAS, i.e. when this method returns true, it
	// also sets state on the entry to indicate that a write for the given blockStart
	// is going to be sent to the index, and other go routines should not attempt the
	// same write. Callers are expected to ensure they follow this guideline.
	// Further, every call to NeedsIndexUpdate which returns true needs to have a corresponding
	// OnIndexFinalze() call. This is required for correct lifecycle maintenance.
	NeedsIndexUpdate(indexBlockStartForWrite xtime.UnixNano) bool
}

// Block represents a collection of segments. Each `Block` is a complete reverse
// index for a period of time defined by [StartTime, EndTime).
type Block interface {
	// StartTime returns the start time of the period this Block indexes.
	StartTime() time.Time

	// EndTime returns the end time of the period this Block indexes.
	EndTime() time.Time

	// WriteBatch writes a batch of provided entries.
	WriteBatch(inserts *WriteBatch) (WriteBatchResult, error)

	// Query resolves the given query into known IDs.
	Query(
		ctx context.Context,
		cancellable *resource.CancellableLifetime,
		query Query,
		opts QueryOptions,
		results BaseResults,
		logFields []opentracinglog.Field,
	) (exhaustive bool, err error)

	// Aggregate aggregates known tag names/values.
	// NB(prateek): different from aggregating by means of Query, as we can
	// avoid going to documents, relying purely on the indexed FSTs.
	Aggregate(
		ctx context.Context,
		cancellable *resource.CancellableLifetime,
		opts QueryOptions,
		results AggregateResults,
		logFields []opentracinglog.Field,
	) (exhaustive bool, err error)

	// AddResults adds bootstrap results to the block.
	AddResults(resultsByVolumeType result.IndexBlockByVolumeType) error

	// Tick does internal house keeping operations.
	Tick(c context.Cancellable) (BlockTickResult, error)

	// Stats returns block stats.
	Stats(reporter BlockStatsReporter) error

	// Seal prevents the block from taking any more writes, but, it still permits
	// addition of segments via Bootstrap().
	Seal() error

	// IsSealed returns whether this block was sealed.
	IsSealed() bool

	// NeedsMutableSegmentsEvicted returns whether this block has any mutable segments
	// that are not-empty and sealed.
	// A sealed non-empty mutable segment needs to get evicted from memory as
	// soon as it can be to reduce memory footprint.
	NeedsMutableSegmentsEvicted() bool

	// EvictMutableSegments closes any mutable segments, this is only applicable
	// valid to be called once the block and hence mutable segments are sealed.
	// It is expected that results have been added to the block that covers any
	// data the mutable segments should have held at this time.
	EvictMutableSegments() error

	// NeedsMutableSegmentsEvicted returns whether this block has any cold mutable segments
	// that are not-empty and sealed.
	NeedsColdMutableSegmentsEvicted() bool

	// EvictMutableSegments closes any stale cold mutable segments up to the currently active
	// cold mutable segment (the one we are actively writing to).
	EvictColdMutableSegments() error

	// RotateColdMutableSegments rotates the currently active cold mutable segment out for a
	// new cold mutable segment to write to.
	RotateColdMutableSegments()

	// MemorySegmentsData returns all in memory segments data.
	MemorySegmentsData(ctx context.Context) ([]fst.SegmentData, error)

	// Close will release any held resources and close the Block.
	Close() error
}

// EvictMutableSegmentResults returns statistics about the EvictMutableSegments execution.
type EvictMutableSegmentResults struct {
	NumMutableSegments int64
	NumDocs            int64
}

// Add adds the provided results to the receiver.
func (e *EvictMutableSegmentResults) Add(o EvictMutableSegmentResults) {
	e.NumDocs += o.NumDocs
	e.NumMutableSegments += o.NumMutableSegments
}

// BlockStatsReporter is a block stats reporter that collects
// block stats on a per block basis (without needing to query each
// block and get an immutable list of segments back).
type BlockStatsReporter interface {
	ReportSegmentStats(stats BlockSegmentStats)
	ReportIndexingStats(stats BlockIndexingStats)
}

type blockStatsReporter struct {
	reportSegmentStats  func(stats BlockSegmentStats)
	reportIndexingStats func(stats BlockIndexingStats)
}

// NewBlockStatsReporter returns a new block stats reporter.
func NewBlockStatsReporter(
	reportSegmentStats func(stats BlockSegmentStats),
	reportIndexingStats func(stats BlockIndexingStats),
) BlockStatsReporter {
	return blockStatsReporter{
		reportSegmentStats:  reportSegmentStats,
		reportIndexingStats: reportIndexingStats,
	}
}

func (r blockStatsReporter) ReportSegmentStats(stats BlockSegmentStats) {
	r.reportSegmentStats(stats)
}

func (r blockStatsReporter) ReportIndexingStats(stats BlockIndexingStats) {
	r.reportIndexingStats(stats)
}

// BlockIndexingStats is stats about a block's indexing stats.
type BlockIndexingStats struct {
	IndexConcurrency int
}

// BlockSegmentStats has segment stats.
type BlockSegmentStats struct {
	Type    BlockSegmentType
	Mutable bool
	Age     time.Duration
	Size    int64
}

// BlockSegmentType is a block segment type
type BlockSegmentType uint

const (
	// ActiveForegroundSegment is an active foreground compacted segment.
	ActiveForegroundSegment BlockSegmentType = iota
	// ActiveBackgroundSegment is an active background compacted segment.
	ActiveBackgroundSegment
	// FlushedSegment is an immutable segment that can't change any longer.
	FlushedSegment
)

// WriteBatchResult returns statistics about the WriteBatch execution.
type WriteBatchResult struct {
	NumSuccess int64
	NumError   int64
}

// BlockTickResult returns statistics about tick.
type BlockTickResult struct {
	NumSegments             int64
	NumSegmentsBootstrapped int64
	NumSegmentsMutable      int64
	NumDocs                 int64
	FreeMmap                int64
}

// WriteBatch is a batch type that allows for building of a slice of documents
// with metadata in a separate slice, this allows the documents slice to be
// passed to the segment to batch insert without having to copy into a buffer
// again.
type WriteBatch struct {
	opts   WriteBatchOptions
	sortBy writeBatchSortBy

	entries []WriteBatchEntry
	docs    []doc.Document
}

type writeBatchSortBy uint

const (
	writeBatchSortByUnmarkedAndBlockStart writeBatchSortBy = iota
	writeBatchSortByEnqueued
)

// WriteBatchOptions is a set of options required for a write batch.
type WriteBatchOptions struct {
	InitialCapacity int
	IndexBlockSize  time.Duration
}

// NewWriteBatch creates a new write batch.
func NewWriteBatch(opts WriteBatchOptions) *WriteBatch {
	return &WriteBatch{
		opts:    opts,
		entries: make([]WriteBatchEntry, 0, opts.InitialCapacity),
		docs:    make([]doc.Document, 0, opts.InitialCapacity),
	}
}

// Append appends an entry with accompanying document.
func (b *WriteBatch) Append(
	entry WriteBatchEntry,
	doc doc.Document,
) {
	// Append just using the result from the current entry
	b.appendWithResult(entry, doc, &entry.resultVal)
}

// Options returns the WriteBatchOptions for this batch.
func (b *WriteBatch) Options() WriteBatchOptions {
	return b.opts
}

// AppendAll appends all entries from another batch to this batch
// and ensures they share the same result struct.
func (b *WriteBatch) AppendAll(from *WriteBatch) {
	numEntries, numDocs := len(from.entries), len(from.docs)
	for i := 0; i < numEntries && i < numDocs; i++ {
		b.appendWithResult(from.entries[i], from.docs[i], from.entries[i].result)
	}
}

func (b *WriteBatch) appendWithResult(
	entry WriteBatchEntry,
	doc doc.Document,
	result *WriteBatchEntryResult,
) {
	// Set private WriteBatchEntry fields
	entry.enqueuedIdx = len(b.entries)
	entry.result = result

	// Append
	b.entries = append(b.entries, entry)
	b.docs = append(b.docs, doc)
}

// ForEachWriteBatchEntryFn allows a caller to perform an operation for each
// batch entry.
type ForEachWriteBatchEntryFn func(
	idx int,
	entry WriteBatchEntry,
	doc doc.Document,
	result WriteBatchEntryResult,
)

// ForEach allows a caller to perform an operation for each batch entry.
func (b *WriteBatch) ForEach(fn ForEachWriteBatchEntryFn) {
	for idx, entry := range b.entries {
		fn(idx, entry, b.docs[idx], entry.Result())
	}
}

// ForEachWriteBatchByBlockStartFn allows a caller to perform an operation with
// reference to a restricted set of the write batch for each unique block
// start.
type ForEachWriteBatchByBlockStartFn func(
	blockStart time.Time,
	batch *WriteBatch,
)

// ForEachUnmarkedBatchByBlockStart allows a caller to perform an operation
// with reference to a restricted set of the write batch for each unique block
// start for entries that have not been marked completed yet.
// The underlying batch returned is simply the current batch but with updated
// subslices to the relevant entries and documents that are restored at the
// end of `fn` being applied.
// NOTE: This means `fn` cannot perform any asynchronous work that uses the
// arguments passed to it as the args will be invalid at the synchronous
// execution of `fn`.
func (b *WriteBatch) ForEachUnmarkedBatchByBlockStart(
	fn ForEachWriteBatchByBlockStartFn,
) {
	// Ensure sorted correctly first
	b.SortByUnmarkedAndIndexBlockStart()

	// What we do is a little funky but least alloc intensive, essentially we mutate
	// this batch and then restore the pointers to the original docs after.
	allEntries := b.entries
	allDocs := b.docs
	defer func() {
		b.entries = allEntries
		b.docs = allDocs
	}()

	var (
		blockSize      = b.opts.IndexBlockSize
		startIdx       = 0
		lastBlockStart xtime.UnixNano
	)
	for i := range allEntries {
		if allEntries[i].result.Done {
			// Hit a marked done entry
			b.entries = allEntries[startIdx:i]
			b.docs = allDocs[startIdx:i]
			if len(b.entries) != 0 {
				fn(lastBlockStart.ToTime(), b)
			}
			return
		}

		blockStart := allEntries[i].indexBlockStart(blockSize)
		if !blockStart.Equal(lastBlockStart) {
			prevLastBlockStart := lastBlockStart.ToTime()
			lastBlockStart = blockStart
			// We only want to call the the ForEachUnmarkedBatchByBlockStart once we have calculated the entire group,
			// i.e. once we have gone past the last element for a given blockStart, but the first element
			// in the slice is a special case because we are always starting a new group at that point.
			if i == 0 {
				continue
			}
			b.entries = allEntries[startIdx:i]
			b.docs = allDocs[startIdx:i]
			fn(prevLastBlockStart, b)
			startIdx = i
		}
	}

	// We can unconditionally spill over here since we haven't hit any marked
	// done entries yet and thanks to sort order there weren't any, therefore
	// we can execute all the remaining entries we had.
	if startIdx < len(allEntries) {
		b.entries = allEntries[startIdx:]
		b.docs = allDocs[startIdx:]
		fn(lastBlockStart.ToTime(), b)
	}
}

func (b *WriteBatch) numPending() int {
	numUnmarked := 0
	for i := range b.entries {
		if b.entries[i].result.Done {
			break
		}
		numUnmarked++
	}
	return numUnmarked
}

// PendingDocs returns all the docs in this batch that are unmarked.
func (b *WriteBatch) PendingDocs() []doc.Document {
	b.SortByUnmarkedAndIndexBlockStart() // Ensure sorted by unmarked first
	return b.docs[:b.numPending()]
}

// PendingEntries returns all the entries in this batch that are unmarked.
func (b *WriteBatch) PendingEntries() []WriteBatchEntry {
	b.SortByUnmarkedAndIndexBlockStart() // Ensure sorted by unmarked first
	return b.entries[:b.numPending()]
}

// NumErrs returns the number of errors encountered by the batch.
func (b *WriteBatch) NumErrs() int {
	errs := 0
	for _, entry := range b.entries {
		if entry.result.Err != nil {
			errs++
		}
	}
	return errs
}

// Reset resets the batch for use.
func (b *WriteBatch) Reset() {
	// Memset optimizations
	var entryZeroed WriteBatchEntry
	for i := range b.entries {
		b.entries[i] = entryZeroed
	}
	b.entries = b.entries[:0]
	var docZeroed doc.Document
	for i := range b.docs {
		b.docs[i] = docZeroed
	}
	b.docs = b.docs[:0]
}

// SortByUnmarkedAndIndexBlockStart sorts the batch by unmarked first and then
// by index block start time.
func (b *WriteBatch) SortByUnmarkedAndIndexBlockStart() {
	b.sortBy = writeBatchSortByUnmarkedAndBlockStart
	sort.Stable(b)
}

// SortByEnqueued sorts the entries and documents back to the sort order they
// were enqueued as.
func (b *WriteBatch) SortByEnqueued() {
	b.sortBy = writeBatchSortByEnqueued
	sort.Stable(b)
}

// MarkUnmarkedEntriesSuccess marks all unmarked entries as success.
func (b *WriteBatch) MarkUnmarkedEntriesSuccess() {
	for idx := range b.entries {
		if !b.entries[idx].result.Done {
			blockStart := b.entries[idx].indexBlockStart(b.opts.IndexBlockSize)
			b.entries[idx].OnIndexSeries.OnIndexSuccess(blockStart)
			b.entries[idx].OnIndexSeries.OnIndexFinalize(blockStart)
			b.entries[idx].result.Done = true
			b.entries[idx].result.Err = nil
		}
	}
}

// MarkUnmarkedEntriesError marks all unmarked entries as error.
func (b *WriteBatch) MarkUnmarkedEntriesError(err error) {
	for idx := range b.entries {
		b.MarkUnmarkedEntryError(err, idx)
	}
}

// MarkUnmarkedEntryError marks an unmarked entry at index as error.
func (b *WriteBatch) MarkUnmarkedEntryError(
	err error,
	idx int,
) {
	if b.entries[idx].OnIndexSeries != nil {
		blockStart := b.entries[idx].indexBlockStart(b.opts.IndexBlockSize)
		b.entries[idx].OnIndexSeries.OnIndexFinalize(blockStart)
		b.entries[idx].result.Done = true
		b.entries[idx].result.Err = err
	}
}

// Ensure that WriteBatch meets the sort interface
var _ sort.Interface = (*WriteBatch)(nil)

// Len returns the length of the batch.
func (b *WriteBatch) Len() int {
	return len(b.entries)
}

// Swap will swap two entries and the corresponding docs.
func (b *WriteBatch) Swap(i, j int) {
	b.entries[i], b.entries[j] = b.entries[j], b.entries[i]
	b.docs[i], b.docs[j] = b.docs[j], b.docs[i]
}

// Less returns whether an entry appears before another depending
// on the type of sort.
func (b *WriteBatch) Less(i, j int) bool {
	if b.sortBy == writeBatchSortByEnqueued {
		return b.entries[i].enqueuedIdx < b.entries[j].enqueuedIdx
	}
	if b.sortBy != writeBatchSortByUnmarkedAndBlockStart {
		panic(fmt.Errorf("unexpected sort by: %d", b.sortBy))
	}

	if !b.entries[i].result.Done && b.entries[j].result.Done {
		// This entry has been marked done and the other this hasn't
		return true
	}
	if b.entries[i].result.Done && !b.entries[j].result.Done {
		// This entry has already been marked and other hasn't
		return false
	}

	// They're either both unmarked or marked
	blockStartI := b.entries[i].indexBlockStart(b.opts.IndexBlockSize)
	blockStartJ := b.entries[j].indexBlockStart(b.opts.IndexBlockSize)
	return blockStartI.Before(blockStartJ)
}

// WriteBatchEntry represents the metadata accompanying the document that is
// being inserted.
type WriteBatchEntry struct {
	// Timestamp is the timestamp that this entry should be indexed for
	Timestamp time.Time
	// OnIndexSeries is a listener/callback for when this entry is marked done
	// it is set to nil when the entry is marked done
	OnIndexSeries OnIndexSeries
	// EnqueuedAt is the timestamp that this entry was enqueued for indexing
	// so that we can calculate the latency it takes to index the entry
	EnqueuedAt time.Time
	// enqueuedIdx is the idx of the entry when originally enqueued by the call
	// to append on the write batch
	enqueuedIdx int
	// result is the result for this entry which is updated when marked done,
	// if it is nil then it is not needed, it is a pointer type so many can be
	// shared when write batches are derived from one and another when
	// combining (for instance across from shards into a single write batch).
	result *WriteBatchEntryResult
	// resultVal is used to set the result initially from so it doesn't have to
	// be separately allocated.
	resultVal WriteBatchEntryResult
}

// WriteBatchEntryResult represents a result.
type WriteBatchEntryResult struct {
	Done bool
	Err  error
}

func (e WriteBatchEntry) indexBlockStart(
	indexBlockSize time.Duration,
) xtime.UnixNano {
	return xtime.ToUnixNano(e.Timestamp.Truncate(indexBlockSize))
}

// Result returns the result for this entry.
func (e WriteBatchEntry) Result() WriteBatchEntryResult {
	return *e.result
}

// fieldsAndTermsIterator iterates over all known fields and terms for a segment.
type fieldsAndTermsIterator interface {
	// Next returns a bool indicating if there are any more elements.
	Next() bool

	// Current returns the current element.
	// NB: the element returned is only valid until the subsequent call to Next().
	Current() (field, term []byte)

	// Err returns any errors encountered during iteration.
	Err() error

	// Close releases any resources held by the iterator.
	Close() error

	// Reset resets the iterator to the start iterating the given segment.
	Reset(reader segment.Reader, opts fieldsAndTermsIteratorOpts) error
}

// Options control the Indexing knobs.
type Options interface {
	// Validate validates assumptions baked into the code.
	Validate() error

	// SetIndexInsertMode sets the index insert mode (sync/async).
	SetInsertMode(value InsertMode) Options

	// IndexInsertMode returns the index's insert mode (sync/async).
	InsertMode() InsertMode

	// SetClockOptions sets the clock options.
	SetClockOptions(value clock.Options) Options

	// ClockOptions returns the clock options.
	ClockOptions() clock.Options

	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(value instrument.Options) Options

	// InstrumentOptions returns the instrument options.
	InstrumentOptions() instrument.Options

	// SetSegmentBuilderOptions sets the mem segment options.
	SetSegmentBuilderOptions(value builder.Options) Options

	// SegmentBuilderOptions returns the mem segment options.
	SegmentBuilderOptions() builder.Options

	// SetMemSegmentOptions sets the mem segment options.
	SetMemSegmentOptions(value mem.Options) Options

	// MemSegmentOptions returns the mem segment options.
	MemSegmentOptions() mem.Options

	// SetFSTSegmentOptions sets the fst segment options.
	SetFSTSegmentOptions(value fst.Options) Options

	// FSTSegmentOptions returns the fst segment options.
	FSTSegmentOptions() fst.Options

	// SetIdentifierPool sets the identifier pool.
	SetIdentifierPool(value ident.Pool) Options

	// IdentifierPool returns the identifier pool.
	IdentifierPool() ident.Pool

	// SetCheckedBytesPool sets the checked bytes pool.
	SetCheckedBytesPool(value pool.CheckedBytesPool) Options

	// CheckedBytesPool returns the checked bytes pool.
	CheckedBytesPool() pool.CheckedBytesPool

	// SetQueryResultsPool updates the query results pool.
	SetQueryResultsPool(values QueryResultsPool) Options

	// ResultsPool returns the results pool.
	QueryResultsPool() QueryResultsPool

	// SetAggregateResultsPool updates the aggregate results pool.
	SetAggregateResultsPool(values AggregateResultsPool) Options

	// AggregateResultsPool returns the aggregate results pool.
	AggregateResultsPool() AggregateResultsPool

	// SetAggregateValuesPool updates the aggregate values pool.
	SetAggregateValuesPool(values AggregateValuesPool) Options

	// AggregateValuesPool returns the aggregate values pool.
	AggregateValuesPool() AggregateValuesPool

	// SetDocumentArrayPool sets the document array pool.
	SetDocumentArrayPool(value doc.DocumentArrayPool) Options

	// DocumentArrayPool returns the document array pool.
	DocumentArrayPool() doc.DocumentArrayPool

	// SetAggregateResultsEntryArrayPool sets the aggregate results entry array pool.
	SetAggregateResultsEntryArrayPool(value AggregateResultsEntryArrayPool) Options

	// AggregateResultsEntryArrayPool returns the aggregate results entry array pool.
	AggregateResultsEntryArrayPool() AggregateResultsEntryArrayPool

	// SetForegroundCompactionPlannerOptions sets the compaction planner options.
	SetForegroundCompactionPlannerOptions(v compaction.PlannerOptions) Options

	// ForegroundCompactionPlannerOptions returns the compaction planner options.
	ForegroundCompactionPlannerOptions() compaction.PlannerOptions

	// SetBackgroundCompactionPlannerOptions sets the compaction planner options.
	SetBackgroundCompactionPlannerOptions(v compaction.PlannerOptions) Options

	// BackgroundCompactionPlannerOptions returns the compaction planner options.
	BackgroundCompactionPlannerOptions() compaction.PlannerOptions

	// SetPostingsListCache sets the postings list cache.
	SetPostingsListCache(value *PostingsListCache) Options

	// PostingsListCache returns the postings list cache.
	PostingsListCache() *PostingsListCache

	// SetReadThroughSegmentOptions sets the read through segment cache options.
	SetReadThroughSegmentOptions(value ReadThroughSegmentOptions) Options

	// ReadThroughSegmentOptions returns the read through segment cache options.
	ReadThroughSegmentOptions() ReadThroughSegmentOptions

	// SetForwardIndexProbability sets the probability chance for forward writes.
	SetForwardIndexProbability(value float64) Options

	// ForwardIndexProbability returns the probability chance for forward writes.
	ForwardIndexProbability() float64

	// SetForwardIndexProbability sets the threshold for forward writes as a
	// fraction of the bufferFuture.
	SetForwardIndexThreshold(value float64) Options

	// ForwardIndexProbability returns the threshold for forward writes.
	ForwardIndexThreshold() float64

	// SetMmapReporter sets the mmap reporter.
	SetMmapReporter(mmapReporter mmap.Reporter) Options

	// MmapReporter returns the mmap reporter.
	MmapReporter() mmap.Reporter

	// SetQueryLimits sets current query limits.
	SetQueryLimits(value limits.QueryLimits) Options

	// QueryLimits returns the current query limits.
	QueryLimits() limits.QueryLimits
}
