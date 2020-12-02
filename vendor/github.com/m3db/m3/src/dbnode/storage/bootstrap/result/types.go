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
	"time"

	"github.com/m3db/m3/src/dbnode/clock"
	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/dbnode/storage/series"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/persist"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/instrument"
	xtime "github.com/m3db/m3/src/x/time"
)

// DataBootstrapResult is the result of a bootstrap of series data.
type DataBootstrapResult interface {
	// Unfulfilled is the unfulfilled time ranges for the bootstrap.
	Unfulfilled() ShardTimeRanges

	// SetUnfulfilled sets the current unfulfilled shard time ranges.
	SetUnfulfilled(unfulfilled ShardTimeRanges)
}

// IndexBootstrapResult is the result of a bootstrap of series index metadata.
type IndexBootstrapResult interface {
	// IndexResults returns a map of all index block results.
	IndexResults() IndexResults

	// Unfulfilled is the unfulfilled time ranges for the bootstrap.
	Unfulfilled() ShardTimeRanges

	// SetUnfulfilled sets the current unfulfilled shard time ranges.
	SetUnfulfilled(unfulfilled ShardTimeRanges)

	// Add adds an index block result.
	Add(blocks IndexBlockByVolumeType, unfulfilled ShardTimeRanges)

	// NumSeries returns the total number of series across all segments.
	NumSeries() int
}

// IndexResults is a set of index blocks indexed by block start.
type IndexResults map[xtime.UnixNano]IndexBlockByVolumeType

// IndexBuilder wraps a index segment builder w/ batching.
type IndexBuilder struct {
	builder segment.DocumentsBuilder
}

// IndexBlockByVolumeType contains the bootstrap data structures for an index block by volume type.
type IndexBlockByVolumeType struct {
	blockStart time.Time
	data       map[persist.IndexVolumeType]IndexBlock
}

// IndexBlock is an index block for a index volume type.
type IndexBlock struct {
	segments  []Segment
	fulfilled ShardTimeRanges
}

// Segment wraps an index segment so we can easily determine whether or not the segment is persisted to disk.
type Segment struct {
	segment   segment.Segment
	persisted bool
}

// NewSegment returns an index segment w/ persistence metadata.
func NewSegment(segment segment.Segment, persisted bool) Segment {
	return Segment{
		segment:   segment,
		persisted: persisted,
	}
}

// IsPersisted returns whether or not the underlying segment was persisted to disk.
func (s Segment) IsPersisted() bool {
	return s.persisted
}

// Segment returns a segment.
func (s Segment) Segment() segment.Segment {
	return s.segment
}

// DocumentsBuilderAllocator allocates a new DocumentsBuilder type when
// creating a bootstrap result to return to the index.
type DocumentsBuilderAllocator func() (segment.DocumentsBuilder, error)

// ShardResult returns the bootstrap result for a shard.
type ShardResult interface {
	// IsEmpty returns whether the result is empty.
	IsEmpty() bool

	// BlockAt returns the block at a given time for a given id,
	// or nil if there is no such block.
	BlockAt(id ident.ID, t time.Time) (block.DatabaseBlock, bool)

	// AllSeries returns a map of all series with their associated blocks.
	AllSeries() *Map

	// NumSeries returns the number of distinct series'.
	NumSeries() int64

	// AddBlock adds a data block.
	AddBlock(id ident.ID, tags ident.Tags, block block.DatabaseBlock)

	// AddSeries adds a single series of blocks.
	AddSeries(id ident.ID, tags ident.Tags, rawSeries block.DatabaseSeriesBlocks)

	// AddResult adds a shard result.
	AddResult(other ShardResult)

	// RemoveBlockAt removes a data block at a given timestamp
	RemoveBlockAt(id ident.ID, t time.Time)

	// RemoveSeries removes a single series of blocks.
	RemoveSeries(id ident.ID)

	// Close closes a shard result.
	Close()
}

// DatabaseSeriesBlocks represents a series of blocks and a associated series ID.
type DatabaseSeriesBlocks struct {
	ID     ident.ID
	Tags   ident.Tags
	Blocks block.DatabaseSeriesBlocks
}

// ShardResults is a map of shards to shard results.
type ShardResults map[uint32]ShardResult

// ShardTimeRanges is a map of shards to time ranges.
type ShardTimeRanges interface {
	// Get time ranges for a shard.
	Get(shard uint32) (xtime.Ranges, bool)

	// Set time ranges for a shard.
	Set(shard uint32, ranges xtime.Ranges) ShardTimeRanges

	// GetOrAdd gets or adds time ranges for a shard.
	GetOrAdd(shard uint32) xtime.Ranges

	// AddRanges adds other shard time ranges to the current shard time ranges.
	AddRanges(ranges ShardTimeRanges)

	// Iter returns the underlying map.
	Iter() map[uint32]xtime.Ranges

	Copy() ShardTimeRanges

	// IsSuperset returns whether the current shard time ranges are a
	// superset of the other shard time ranges.
	IsSuperset(other ShardTimeRanges) bool

	// Equal returns whether two shard time ranges are equal.
	Equal(other ShardTimeRanges) bool

	// ToUnfulfilledDataResult will return a result that is comprised of wholly
	// unfufilled time ranges from the set of shard time ranges.
	ToUnfulfilledDataResult() DataBootstrapResult

	// ToUnfulfilledIndexResult will return a result that is comprised of wholly
	// unfufilled time ranges from the set of shard time ranges.
	ToUnfulfilledIndexResult() IndexBootstrapResult

	// Subtract will subtract another range from the current range.
	Subtract(other ShardTimeRanges)

	// MinMax will return the very minimum time as a start and the
	// maximum time as an end in the ranges.
	MinMax() (time.Time, time.Time)

	// MinMaxRange returns the min and max times, and the duration for this range.
	MinMaxRange() (time.Time, time.Time, time.Duration)

	// String returns a description of the time ranges
	String() string

	// SummaryString returns a summary description of the time ranges
	SummaryString() string

	// IsEmpty returns whether the shard time ranges is empty or not.
	IsEmpty() bool

	// Len returns the number of shards
	Len() int
}

type shardTimeRanges map[uint32]xtime.Ranges

// Options represents the options for bootstrap results.
type Options interface {
	// SetClockOptions sets the clock options.
	SetClockOptions(value clock.Options) Options

	// ClockOptions returns the clock options.
	ClockOptions() clock.Options

	// SetInstrumentOptions sets the instrumentation options.
	SetInstrumentOptions(value instrument.Options) Options

	// InstrumentOptions returns the instrumentation options.
	InstrumentOptions() instrument.Options

	// SetDatabaseBlockOptions sets the database block options.
	SetDatabaseBlockOptions(value block.Options) Options

	// DatabaseBlockOptions returns the database block options.
	DatabaseBlockOptions() block.Options

	// SetNewBlocksLen sets the size of a new blocks map size.
	SetNewBlocksLen(value int) Options

	// NewBlocksLen returns the size of a new blocks map size.
	NewBlocksLen() int

	// SetSeriesCachePolicy sets the series cache policy.
	SetSeriesCachePolicy(value series.CachePolicy) Options

	// SeriesCachePolicy returns the series cache policy.
	SeriesCachePolicy() series.CachePolicy

	// SetIndexDocumentsBuilderAllocator sets the index mutable segment allocator.
	SetIndexDocumentsBuilderAllocator(value DocumentsBuilderAllocator) Options

	// IndexDocumentsBuilderAllocator returns the index documents builder allocator.
	IndexDocumentsBuilderAllocator() DocumentsBuilderAllocator
}
