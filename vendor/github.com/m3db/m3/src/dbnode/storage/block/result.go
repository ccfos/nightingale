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

package block

import (
	"sort"
	"time"

	"github.com/m3db/m3/src/dbnode/x/xio"
	"github.com/m3db/m3/src/x/ident"
)

// NewFetchBlockResult creates a new fetch block result
func NewFetchBlockResult(
	start time.Time,
	blocks []xio.BlockReader,
	err error,
) FetchBlockResult {
	return FetchBlockResult{
		Start:  start,
		Blocks: blocks,
		Err:    err,
	}
}

type fetchBlockResultByTimeAscending []FetchBlockResult

func (e fetchBlockResultByTimeAscending) Len() int           { return len(e) }
func (e fetchBlockResultByTimeAscending) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }
func (e fetchBlockResultByTimeAscending) Less(i, j int) bool { return e[i].Start.Before(e[j].Start) }

// SortFetchBlockResultByTimeAscending sorts fetch block results in time ascending order
func SortFetchBlockResultByTimeAscending(results []FetchBlockResult) {
	sort.Sort(fetchBlockResultByTimeAscending(results))
}

// NewFetchBlockMetadataResult creates a new fetch block metadata result.
func NewFetchBlockMetadataResult(
	start time.Time,
	size int64,
	checksum *uint32,
	lastRead time.Time,
	err error,
) FetchBlockMetadataResult {
	return FetchBlockMetadataResult{
		Start:    start,
		Size:     size,
		Checksum: checksum,
		LastRead: lastRead,
		Err:      err,
	}
}

type fetchBlockMetadataResultByTimeAscending []FetchBlockMetadataResult

func (a fetchBlockMetadataResultByTimeAscending) Len() int      { return len(a) }
func (a fetchBlockMetadataResultByTimeAscending) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a fetchBlockMetadataResultByTimeAscending) Less(i, j int) bool {
	return a[i].Start.Before(a[j].Start)
}

type fetchBlockMetadataResults struct {
	results []FetchBlockMetadataResult
	pool    FetchBlockMetadataResultsPool
}

// NewFetchBlockMetadataResults creates a non-pooled fetchBlockMetadataResults
func NewFetchBlockMetadataResults() FetchBlockMetadataResults {
	return &fetchBlockMetadataResults{}
}

func newPooledFetchBlockMetadataResults(
	results []FetchBlockMetadataResult,
	pool FetchBlockMetadataResultsPool,
) FetchBlockMetadataResults {
	return &fetchBlockMetadataResults{results: results, pool: pool}
}

func (s *fetchBlockMetadataResults) Add(res FetchBlockMetadataResult) {
	s.results = append(s.results, res)
}

func (s *fetchBlockMetadataResults) Results() []FetchBlockMetadataResult {
	return s.results
}

func (s *fetchBlockMetadataResults) Sort() {
	sort.Sort(fetchBlockMetadataResultByTimeAscending(s.results))
}

func (s *fetchBlockMetadataResults) Reset() {
	var zeroed FetchBlockMetadataResult
	for i := range s.results {
		s.results[i] = zeroed
	}
	s.results = s.results[:0]
}

func (s *fetchBlockMetadataResults) Close() {
	if s.pool != nil {
		s.pool.Put(s)
	}
}

// NewFetchBlocksMetadataResult creates new database blocks metadata
func NewFetchBlocksMetadataResult(
	id ident.ID,
	tags ident.TagIterator,
	blocks FetchBlockMetadataResults,
) FetchBlocksMetadataResult {
	return FetchBlocksMetadataResult{ID: id, Tags: tags, Blocks: blocks}
}

type fetchBlocksMetadataResults struct {
	results []FetchBlocksMetadataResult
	pool    FetchBlocksMetadataResultsPool
}

// NewFetchBlocksMetadataResults creates a non-pooled FetchBlocksMetadataResults
func NewFetchBlocksMetadataResults() FetchBlocksMetadataResults {
	return &fetchBlocksMetadataResults{}
}

func newPooledFetchBlocksMetadataResults(
	results []FetchBlocksMetadataResult,
	pool FetchBlocksMetadataResultsPool,
) FetchBlocksMetadataResults {
	return &fetchBlocksMetadataResults{results: results, pool: pool}
}

func (s *fetchBlocksMetadataResults) Add(res FetchBlocksMetadataResult) {
	s.results = append(s.results, res)
}

func (s *fetchBlocksMetadataResults) Results() []FetchBlocksMetadataResult {
	return s.results
}

func (s *fetchBlocksMetadataResults) Reset() {
	var zeroed FetchBlocksMetadataResult
	for i := range s.results {
		s.results[i] = zeroed
	}
	s.results = s.results[:0]
}

func (s *fetchBlocksMetadataResults) Close() {
	for i := range s.results {
		if s.results[i].ID != nil {
			s.results[i].ID.Finalize()
			s.results[i].ID = nil
		}
		if s.results[i].Tags != nil {
			s.results[i].Tags.Close()
			s.results[i].Tags = nil
		}
		if s.results[i].Blocks != nil {
			s.results[i].Blocks.Close()
			s.results[i].Blocks = nil
		}
	}
	if s.pool != nil {
		s.pool.Put(s)
	}
}

type filteredBlocksMetadataIter struct {
	res      []FetchBlocksMetadataResult
	id       ident.ID
	metadata Metadata
	resIdx   int
	blockIdx int
	err      error
}

// NewFilteredBlocksMetadataIter creates a new filtered blocks metadata
// iterator, there's no pooling of the tags returned currently.
// Only the repair process uses this currently which is unoptimized.
func NewFilteredBlocksMetadataIter(
	res FetchBlocksMetadataResults,
) FilteredBlocksMetadataIter {
	return &filteredBlocksMetadataIter{res: res.Results()}
}

func (it *filteredBlocksMetadataIter) Next() bool {
	if it.err != nil {
		return false
	}
	if it.resIdx >= len(it.res) {
		return false
	}
	blocks := it.res[it.resIdx].Blocks.Results()
	for it.blockIdx < len(blocks) {
		block := blocks[it.blockIdx]
		if block.Err != nil {
			it.blockIdx++
			continue
		}
		break
	}
	if it.blockIdx >= len(blocks) {
		it.resIdx++
		it.blockIdx = 0
		return it.Next()
	}
	it.id = it.res[it.resIdx].ID
	block := blocks[it.blockIdx]
	var tags ident.Tags
	if tagsIter := it.res[it.resIdx].Tags; tagsIter != nil {
		for tagsIter.Next() {
			curr := tagsIter.Current()
			tags.Append(ident.StringTag(curr.Name.String(), curr.Value.String()))
		}
		if err := tagsIter.Err(); err != nil {
			it.err = err
			return false
		}
		tagsIter.Close()
		// Set to nil so it doesn't get closed again later and trigger a double-put pooling bug.
		it.res[it.resIdx].Tags = nil
	}
	it.metadata = NewMetadata(it.id, tags, block.Start,
		block.Size, block.Checksum, block.LastRead)
	it.blockIdx++
	return true
}

func (it *filteredBlocksMetadataIter) Current() (ident.ID, Metadata) {
	return it.id, it.metadata
}

func (it *filteredBlocksMetadataIter) Err() error {
	return it.err
}
