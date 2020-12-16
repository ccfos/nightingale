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

package block

import (
	"io"
	"time"

	"github.com/m3db/m3/src/query/models"
)

// BlockType describes a block type.
type BlockType uint8

const (
	// BlockM3TSZCompressed is an M3TSZ compressed block.
	BlockM3TSZCompressed BlockType = iota
	// BlockDecompressed is a decompressed raw data block.
	BlockDecompressed
	// BlockScalar is a scalar block with a single value throughout its range.
	BlockScalar
	// BlockTime is a block with datapoint values given by a function of their
	// timestamps.
	BlockTime
	// BlockLazy is a wrapper for an inner block that lazily applies transforms.
	BlockLazy
	// BlockContainer is a block that contains multiple inner blocks that share
	// common metadata.
	BlockContainer
	// BlockEmpty is a block with metadata but no series or values.
	BlockEmpty
	// BlockTest is a block used for testing only.
	BlockTest
)

// Block represents a group of series across a time bound.
type Block interface {
	io.Closer
	// StepIter returns a step-wise block iterator, giving consolidated values
	// across all series comprising the box at a single time step.
	StepIter() (StepIter, error)
	// SeriesIter returns a series-wise block iterator, giving unconsolidated
	// by series.
	SeriesIter() (SeriesIter, error)
	// MultiSeriesIter returns batched series iterators for the block based on
	// given concurrency.
	MultiSeriesIter(concurrency int) ([]SeriesIterBatch, error)
	// Meta returns the metadata for the block.
	Meta() Metadata
	// Info returns information about the block.
	Info() BlockInfo
}

// AccumulatorBlock accumulates incoming blocks and presents them as a single
// Block.
type AccumulatorBlock interface {
	Block
	// AddBlock adds a block to this accumulator.
	AddBlock(bl Block) error
}

// SeriesMeta is metadata data for the series.
type SeriesMeta struct {
	Tags models.Tags
	Name []byte
}

// Iterator is the base iterator.
type Iterator interface {
	// Next moves to the next item in the iterator. It will return false if there
	// are no more items, or if encountering an error.
	//
	// NB: it is important to check that Err() is nil after Next returns false, to
	// ensure that any errors during iteration are detected and accounted for.
	Next() bool
	// Err returns any error encountered during iteration.
	Err() error
	// Close frees up resources held by the iterator.
	Close()
}

// SeriesIterBatch is a batch of SeriesIterators.
type SeriesIterBatch struct {
	// Iter is the series iterator.
	Iter SeriesIter
	// Size is the batch size.
	Size int
}

// SeriesIter iterates through a block horizontally.
type SeriesIter interface {
	Iterator
	// SeriesMeta returns the metadata for each series in the block.
	SeriesMeta() []SeriesMeta
	// SeriesCount returns the number of series.
	SeriesCount() int
	// Current returns the current series for the block.
	Current() UnconsolidatedSeries
}

// StepIter iterates through a block vertically.
type StepIter interface {
	Iterator
	// SeriesMeta returns the metadata for each series in the block.
	SeriesMeta() []SeriesMeta
	// StepCount returns the number of steps.
	StepCount() int
	// Current returns the current step for the block.
	Current() Step
}

// Step is a single time step within a block.
type Step interface {
	Time() time.Time
	Values() []float64
}

// Builder builds Blocks.
type Builder interface {
	// AddCols adds the given number of columns to the block.
	AddCols(num int) error
	// SetRow sets a given block row to the given values and metadata.
	SetRow(idx int, values []float64, meta SeriesMeta) error
	// PopulateColumns sets all columns to the given size.
	PopulateColumns(size int)
	// AppendValue adds a single value to the column at the given index.
	AppendValue(idx int, value float64) error
	// AppendValues adds a slice of values to the column at the given index.
	AppendValues(idx int, values []float64) error
	// Build builds the block.
	Build() Block
	// BuildAsType builds the block, forcing it to the given BlockType.
	BuildAsType(blockType BlockType) Block
}

// Result is a fetch result containing multiple blocks optionally split across
// time boundaries.
type Result struct {
	// Blocks is a list of blocks, optionally split across time boundaries.
	Blocks []Block
	// Metadata contains information on fetch status.
	Metadata ResultMetadata
}

// TimeTransform transforms a timestamp.
type TimeTransform func(time.Time) time.Time

// MetaTransform transforms meta data.
type MetaTransform func(meta Metadata) Metadata

// SeriesMetaTransform transforms series meta data.
type SeriesMetaTransform func(meta []SeriesMeta) []SeriesMeta

// ValueTransform transform a float64.
type ValueTransform func(float64) float64

// LazyOptions describes options for lazy blocks.
type LazyOptions interface {
	// SetTimeTransform sets the time transform function.
	SetTimeTransform(TimeTransform) LazyOptions
	// TimeTransform returns the time transform function.
	TimeTransform() TimeTransform
	// SetValueTransform sets the value transform function.
	SetValueTransform(ValueTransform) LazyOptions
	// ValueTransform returns the value transform function.
	ValueTransform() ValueTransform
	// SetMetaTransform sets the meta transform function.
	SetMetaTransform(MetaTransform) LazyOptions
	// MetaTransform returns the meta transform function.
	MetaTransform() MetaTransform
	// SetSeriesMetaTransform sets the series meta transform function.
	SetSeriesMetaTransform(SeriesMetaTransform) LazyOptions
	// SeriesMetaTransform returns the series meta transform function.
	SeriesMetaTransform() SeriesMetaTransform
}
