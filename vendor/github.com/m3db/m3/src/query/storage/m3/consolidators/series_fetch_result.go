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

package consolidators

import (
	"fmt"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/query/block"
	"github.com/m3db/m3/src/query/models"
)

// NewSeriesFetchResult creates a new series fetch result using the given
// iterators.
func NewSeriesFetchResult(
	iters encoding.SeriesIterators,
	tags []*models.Tags,
	meta block.ResultMetadata,
) (SeriesFetchResult, error) {
	if iters == nil || iters.Len() == 0 {
		return SeriesFetchResult{
			Metadata: meta,
			seriesData: seriesData{
				seriesIterators: nil,
				tags:            []*models.Tags{},
			},
		}, nil
	}

	if tags == nil {
		tags = make([]*models.Tags, iters.Len())
	}

	return SeriesFetchResult{
		Metadata: meta,
		seriesData: seriesData{
			seriesIterators: iters,
			tags:            tags,
		},
	}, nil
}

// NewEmptyFetchResult creates a new empty series fetch result.
func NewEmptyFetchResult(
	meta block.ResultMetadata,
) SeriesFetchResult {
	return SeriesFetchResult{
		Metadata: meta,
		seriesData: seriesData{
			seriesIterators: nil,
			tags:            []*models.Tags{},
		},
	}
}

// Verify verifies the fetch result is valid.
func (r *SeriesFetchResult) Verify() error {
	if r.seriesData.tags == nil || r.seriesData.seriesIterators == nil {
		return nil
	}

	tagLen := len(r.seriesData.tags)
	iterLen := r.seriesData.seriesIterators.Len()
	if tagLen != iterLen {
		return fmt.Errorf("tag length %d does not match iterator length %d",
			tagLen, iterLen)
	}

	return nil
}

// Count returns the total number of contained series iterators.
func (r *SeriesFetchResult) Count() int {
	if r.seriesData.seriesIterators == nil {
		return 0
	}

	return r.seriesData.seriesIterators.Len()
}

// Close no-ops; these should be closed by the enclosing iterator.
func (r *SeriesFetchResult) Close() {

}

// IterTagsAtIndex returns the tag iterator and tags at the given index.
func (r *SeriesFetchResult) IterTagsAtIndex(
	idx int, tagOpts models.TagOptions,
) (encoding.SeriesIterator, models.Tags, error) {
	tags := models.EmptyTags()
	if idx < 0 || idx > len(r.seriesData.tags) {
		return nil, tags, fmt.Errorf("series idx(%d) out of "+
			"bounds %d ", idx, len(r.seriesData.tags))
	}

	iters := r.seriesData.seriesIterators.Iters()
	if idx < len(r.seriesData.tags) {
		if r.seriesData.tags[idx] == nil {
			var err error
			iter := iters[idx].Tags()
			tags, err = FromIdentTagIteratorToTags(iter, tagOpts)
			if err != nil {
				return nil, models.EmptyTags(), err
			}

			iter.Rewind()
			r.seriesData.tags[idx] = &tags
		} else {
			tags = *r.seriesData.tags[idx]
		}
	}

	return iters[idx], tags, nil
}

// SeriesIterators returns the series iterators.
func (r *SeriesFetchResult) SeriesIterators() []encoding.SeriesIterator {
	if r.seriesData.seriesIterators == nil {
		return []encoding.SeriesIterator{}
	}

	return r.seriesData.seriesIterators.Iters()
}
