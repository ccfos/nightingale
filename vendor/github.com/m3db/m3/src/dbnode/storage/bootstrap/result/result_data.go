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

	"github.com/m3db/m3/src/dbnode/storage/block"
	"github.com/m3db/m3/src/x/ident"
)

type dataBootstrapResult struct {
	unfulfilled ShardTimeRanges
}

// NewDataBootstrapResult creates a new result.
func NewDataBootstrapResult() DataBootstrapResult {
	return &dataBootstrapResult{
		unfulfilled: NewShardTimeRanges(),
	}
}

func (r *dataBootstrapResult) Unfulfilled() ShardTimeRanges {
	return r.unfulfilled
}

func (r *dataBootstrapResult) SetUnfulfilled(unfulfilled ShardTimeRanges) {
	r.unfulfilled = unfulfilled
}

// MergedDataBootstrapResult returns a merged result of two bootstrap results.
// It is a mutating function that mutates the larger result by adding the
// smaller result to it and then finally returns the mutated result.
func MergedDataBootstrapResult(i, j DataBootstrapResult) DataBootstrapResult {
	if i == nil {
		return j
	}
	if j == nil {
		return i
	}
	i.Unfulfilled().AddRanges(j.Unfulfilled())
	return i
}

type shardResult struct {
	opts   Options
	blocks *Map
}

// NewShardResult creates a new shard result.
func NewShardResult(capacity int, opts Options) ShardResult {
	return &shardResult{
		opts: opts,
		blocks: NewMap(MapOptions{
			InitialSize: capacity,
			KeyCopyPool: opts.DatabaseBlockOptions().BytesPool().BytesPool(),
		}),
	}
}

// IsEmpty returns whether the result is empty.
func (sr *shardResult) IsEmpty() bool {
	return sr.blocks.Len() == 0
}

// AddBlock adds a data block.
func (sr *shardResult) AddBlock(id ident.ID, tags ident.Tags, b block.DatabaseBlock) {
	curSeries, exists := sr.blocks.Get(id)
	if !exists {
		curSeries = sr.newBlocks(id, tags)
		sr.blocks.Set(id, curSeries)
	}
	curSeries.Blocks.AddBlock(b)
}

// AddSeries adds a single series.
func (sr *shardResult) AddSeries(id ident.ID, tags ident.Tags, rawSeries block.DatabaseSeriesBlocks) {
	curSeries, exists := sr.blocks.Get(id)
	if !exists {
		curSeries = sr.newBlocks(id, tags)
		sr.blocks.Set(id, curSeries)
	}
	curSeries.Blocks.AddSeries(rawSeries)
}

func (sr *shardResult) newBlocks(id ident.ID, tags ident.Tags) DatabaseSeriesBlocks {
	size := sr.opts.NewBlocksLen()
	return DatabaseSeriesBlocks{
		ID:     id,
		Tags:   tags,
		Blocks: block.NewDatabaseSeriesBlocks(size),
	}
}

// AddResult adds a shard result.
func (sr *shardResult) AddResult(other ShardResult) {
	if other == nil {
		return
	}
	otherSeries := other.AllSeries()
	for _, entry := range otherSeries.Iter() {
		series := entry.Value()
		sr.AddSeries(series.ID, series.Tags, series.Blocks)
	}
}

// RemoveBlockAt removes a data block at a given timestamp
func (sr *shardResult) RemoveBlockAt(id ident.ID, t time.Time) {
	curSeries, exists := sr.blocks.Get(id)
	if !exists {
		return
	}
	curSeries.Blocks.RemoveBlockAt(t)
	if curSeries.Blocks.Len() == 0 {
		sr.RemoveSeries(id)
	}
}

// RemoveSeries removes a single series of blocks.
func (sr *shardResult) RemoveSeries(id ident.ID) {
	sr.blocks.Delete(id)
}

// AllSeries returns all series in the map.
func (sr *shardResult) AllSeries() *Map {
	return sr.blocks
}

func (sr *shardResult) NumSeries() int64 {
	return int64(sr.blocks.Len())
}

func (sr *shardResult) BlockAt(id ident.ID, t time.Time) (block.DatabaseBlock, bool) {
	series, exists := sr.blocks.Get(id)
	if !exists {
		return nil, false
	}
	return series.Blocks.BlockAt(t)
}

// Close closes a shard result.
func (sr *shardResult) Close() {
	for _, entry := range sr.blocks.Iter() {
		series := entry.Value()
		series.Blocks.Close()
	}
}

// NumSeries returns the number of series' across all shards.
func (r ShardResults) NumSeries() int64 {
	var numSeries int64
	for _, result := range r {
		numSeries += result.NumSeries()
	}
	return numSeries
}

// AddResults adds other shard results to the current shard results.
func (r ShardResults) AddResults(other ShardResults) {
	for shard, result := range other {
		if result == nil || result.NumSeries() == 0 {
			continue
		}
		if existing, ok := r[shard]; ok {
			existing.AddResult(result)
		} else {
			r[shard] = result
		}
	}
}

// Equal returns whether another shard results is equal to the current shard results,
// will not perform a deep equal only a shallow equal of series and their block addresses.
func (r ShardResults) Equal(other ShardResults) bool {
	for shard, result := range r {
		otherResult, ok := r[shard]
		if !ok {
			return false
		}
		allSeries := result.AllSeries()
		otherAllSeries := otherResult.AllSeries()
		if allSeries.Len() != otherAllSeries.Len() {
			return false
		}
		for _, entry := range allSeries.Iter() {
			id, series := entry.Key(), entry.Value()
			otherSeries, ok := otherAllSeries.Get(id)
			if !ok {
				return false
			}
			allBlocks := series.Blocks.AllBlocks()
			otherAllBlocks := otherSeries.Blocks.AllBlocks()
			if len(allBlocks) != len(otherAllBlocks) {
				return false
			}
			for start, block := range allBlocks {
				otherBlock, ok := otherAllBlocks[start]
				if !ok {
					return false
				}
				// Just performing shallow equals so simply compare block addresses
				if block != otherBlock {
					return false
				}
			}
		}
	}
	return true
}

// EstimateMapBytesSize estimates the size (in bytes) of the results map. It's only an
// estimate because its impossible to know if some of the references like the series
// name as well as tags are exclusive to this object or shared with other structures in
// memory.
func EstimateMapBytesSize(m *Map) int64 {
	if m == nil {
		return 0
	}

	var sum int64
	for _, elem := range m.Iter() {
		id := elem.Key()
		sum += int64(len(id.Bytes()))

		blocks := elem.Value()
		for _, tag := range blocks.Tags.Values() {
			// Name/Value should never be nil but be precautious.
			if tag.Name != nil {
				sum += int64(len(tag.Name.Bytes()))
			}
			if tag.Value != nil {
				sum += int64(len(tag.Value.Bytes()))
			}
		}
		for _, block := range blocks.Blocks.AllBlocks() {
			sum += int64(block.Len())
		}
	}
	return sum
}
