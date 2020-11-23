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

package result

import (
	"fmt"
	"time"

	"github.com/m3db/m3/src/dbnode/namespace"
	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/index/segment/builder"
	"github.com/m3db/m3/src/m3ninx/persist"
	xtime "github.com/m3db/m3/src/x/time"
)

// NewDefaultDocumentsBuilderAllocator returns a default mutable segment
// allocator.
func NewDefaultDocumentsBuilderAllocator() DocumentsBuilderAllocator {
	return func() (segment.DocumentsBuilder, error) {
		return builder.NewBuilderFromDocuments(builder.NewOptions())
	}
}

type indexBootstrapResult struct {
	results     IndexResults
	unfulfilled ShardTimeRanges
}

// NewIndexBootstrapResult returns a new index bootstrap result.
func NewIndexBootstrapResult() IndexBootstrapResult {
	return &indexBootstrapResult{
		results:     make(IndexResults),
		unfulfilled: NewShardTimeRanges(),
	}
}

func (r *indexBootstrapResult) IndexResults() IndexResults {
	return r.results
}

func (r *indexBootstrapResult) Unfulfilled() ShardTimeRanges {
	return r.unfulfilled
}

func (r *indexBootstrapResult) SetUnfulfilled(unfulfilled ShardTimeRanges) {
	r.unfulfilled = unfulfilled
}

func (r *indexBootstrapResult) Add(blocks IndexBlockByVolumeType, unfulfilled ShardTimeRanges) {
	r.results.Add(blocks)
	r.unfulfilled.AddRanges(unfulfilled)
}

func (r *indexBootstrapResult) NumSeries() int {
	var size int64
	for _, blockByVolumeType := range r.results {
		for _, b := range blockByVolumeType.data {
			for _, s := range b.segments {
				size += s.Segment().Size()
			}
		}
	}
	return int(size)
}

// NewIndexBuilder creates a wrapped locakble index seg builder.
func NewIndexBuilder(builder segment.DocumentsBuilder) *IndexBuilder {
	return &IndexBuilder{
		builder: builder,
	}
}

// FlushBatch flushes a batch of documents to the underlying segment builder.
func (b *IndexBuilder) FlushBatch(batch []doc.Document) ([]doc.Document, error) {
	if len(batch) == 0 {
		// Last flush might not have any docs enqueued
		return batch, nil
	}

	// NB(bodu): Prevent concurrent writes.
	// Although it seems like there's no need to lock on writes since
	// each block should ONLY be getting built in a single thread.
	err := b.builder.InsertBatch(index.Batch{
		Docs:                batch,
		AllowPartialUpdates: true,
	})
	if err != nil && index.IsBatchPartialError(err) {
		// If after filtering out duplicate ID errors
		// there are no errors, then this was a successful
		// insertion.
		batchErr := err.(*index.BatchPartialError)
		// NB(r): FilterDuplicateIDErrors returns nil
		// if no errors remain after filtering duplicate ID
		// errors, this case is covered in unit tests.
		err = batchErr.FilterDuplicateIDErrors()
	}
	if err != nil {
		return batch, err
	}

	// Reset docs batch for reuse
	var empty doc.Document
	for i := range batch {
		batch[i] = empty
	}
	batch = batch[:0]
	return batch, nil
}

// Builder returns the underlying index segment docs builder.
func (b *IndexBuilder) Builder() segment.DocumentsBuilder {
	return b.builder
}

// AddBlockIfNotExists adds an index block if it does not already exist to the index results.
func (r IndexResults) AddBlockIfNotExists(
	t time.Time,
	idxopts namespace.IndexOptions,
) {
	// NB(r): The reason we can align by the retention block size and guarantee
	// there is only one entry for this time is because index blocks must be a
	// positive multiple of the data block size, making it easy to map a data
	// block entry to at most one index block entry.
	blockStart := t.Truncate(idxopts.BlockSize())
	blockStartNanos := xtime.ToUnixNano(blockStart)

	_, exists := r[blockStartNanos]
	if !exists {
		r[blockStartNanos] = NewIndexBlockByVolumeType(blockStart)
	}
}

// Add will add an index block to the collection, merging if one already
// exists.
func (r IndexResults) Add(blocks IndexBlockByVolumeType) {
	if blocks.BlockStart().IsZero() {
		return
	}

	// Merge results
	blockStart := xtime.ToUnixNano(blocks.BlockStart())
	existing, ok := r[blockStart]
	if !ok {
		r[blockStart] = blocks
		return
	}

	r[blockStart] = existing.Merged(blocks)
}

// AddResults will add another set of index results to the collection, merging
// if index blocks already exists.
func (r IndexResults) AddResults(other IndexResults) {
	for _, blocks := range other {
		r.Add(blocks)
	}
}

// MarkFulfilled will mark an index block as fulfilled, either partially or
// wholly as specified by the shard time ranges passed.
func (r IndexResults) MarkFulfilled(
	t time.Time,
	fulfilled ShardTimeRanges,
	indexVolumeType persist.IndexVolumeType,
	idxopts namespace.IndexOptions,
) error {
	// NB(r): The reason we can align by the retention block size and guarantee
	// there is only one entry for this time is because index blocks must be a
	// positive multiple of the data block size, making it easy to map a data
	// block entry to at most one index block entry.
	blockStart := t.Truncate(idxopts.BlockSize())
	blockStartNanos := xtime.ToUnixNano(blockStart)

	blockRange := xtime.Range{
		Start: blockStart,
		End:   blockStart.Add(idxopts.BlockSize()),
	}

	// First check fulfilled is correct
	min, max := fulfilled.MinMax()
	if min.Before(blockRange.Start) || max.After(blockRange.End) {
		return fmt.Errorf("fulfilled range %s is outside of index block range: %s",
			fulfilled.SummaryString(), blockRange.String())
	}

	blocks, exists := r[blockStartNanos]
	if !exists {
		blocks = NewIndexBlockByVolumeType(blockStart)
		r[blockStartNanos] = blocks
	}

	block, exists := blocks.data[indexVolumeType]
	if !exists {
		block = NewIndexBlock(nil, nil)
		blocks.data[indexVolumeType] = block
	}
	blocks.data[indexVolumeType] = block.Merged(NewIndexBlock(nil, fulfilled))
	return nil
}

// MergedIndexBootstrapResult returns a merged result of two bootstrap results.
// It is a mutating function that mutates the larger result by adding the
// smaller result to it and then finally returns the mutated result.
func MergedIndexBootstrapResult(i, j IndexBootstrapResult) IndexBootstrapResult {
	if i == nil {
		return j
	}
	if j == nil {
		return i
	}
	sizeI, sizeJ := 0, 0
	for _, ir := range i.IndexResults() {
		for _, b := range ir.data {
			sizeI += len(b.Segments())
		}
	}
	for _, ir := range j.IndexResults() {
		for _, b := range ir.data {
			sizeJ += len(b.Segments())
		}
	}
	if sizeI >= sizeJ {
		i.IndexResults().AddResults(j.IndexResults())
		i.Unfulfilled().AddRanges(j.Unfulfilled())
		return i
	}
	j.IndexResults().AddResults(i.IndexResults())
	j.Unfulfilled().AddRanges(i.Unfulfilled())
	return j
}

// NewIndexBlock returns a new bootstrap index block result.
func NewIndexBlock(
	segments []Segment,
	fulfilled ShardTimeRanges,
) IndexBlock {
	if fulfilled == nil {
		fulfilled = NewShardTimeRanges()
	}
	return IndexBlock{
		segments:  segments,
		fulfilled: fulfilled,
	}
}

// Segments returns the segments.
func (b IndexBlock) Segments() []Segment {
	return b.segments
}

// Fulfilled returns the fulfilled time ranges by this index block.
func (b IndexBlock) Fulfilled() ShardTimeRanges {
	return b.fulfilled
}

// Merged returns a new merged index block, currently it just appends the
// list of segments from the other index block and the caller merges
// as they see necessary.
func (b IndexBlock) Merged(other IndexBlock) IndexBlock {
	r := b
	if len(other.segments) > 0 {
		r.segments = append(r.segments, other.segments...)
	}
	if !other.fulfilled.IsEmpty() {
		r.fulfilled = b.fulfilled.Copy()
		r.fulfilled.AddRanges(other.fulfilled)
	}
	return r
}

// NewIndexBlockByVolumeType returns a new bootstrap index blocks by volume type result.
func NewIndexBlockByVolumeType(blockStart time.Time) IndexBlockByVolumeType {
	return IndexBlockByVolumeType{
		blockStart: blockStart,
		data:       make(map[persist.IndexVolumeType]IndexBlock),
	}
}

// BlockStart returns the block start.
func (b IndexBlockByVolumeType) BlockStart() time.Time {
	return b.blockStart
}

// GetBlock returns an IndexBlock for volumeType.
func (b IndexBlockByVolumeType) GetBlock(volumeType persist.IndexVolumeType) (IndexBlock, bool) {
	block, ok := b.data[volumeType]
	return block, ok
}

// SetBlock sets an IndexBlock for volumeType.
func (b IndexBlockByVolumeType) SetBlock(volumeType persist.IndexVolumeType, block IndexBlock) {
	b.data[volumeType] = block
}

// Iter returns the underlying iterable map data.
func (b IndexBlockByVolumeType) Iter() map[persist.IndexVolumeType]IndexBlock {
	return b.data
}

// Merged returns a new merged index block by volume type.
// It merges the underlying index blocks together by index volume type.
func (b IndexBlockByVolumeType) Merged(other IndexBlockByVolumeType) IndexBlockByVolumeType {
	r := b
	for volumeType, otherBlock := range other.data {
		existing, ok := r.data[volumeType]
		if !ok {
			r.data[volumeType] = otherBlock
			continue
		}
		r.data[volumeType] = existing.Merged(otherBlock)
	}
	return r
}
