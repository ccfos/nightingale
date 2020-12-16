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

package consolidators

import (
	"sync"

	"github.com/m3db/m3/src/dbnode/client"
	"github.com/m3db/m3/src/query/block"
	"github.com/m3db/m3/src/query/models"
	xerrors "github.com/m3db/m3/src/x/errors"
)

const initSize = 10

type multiSearchResult struct {
	sync.Mutex
	meta      block.ResultMetadata
	err       xerrors.MultiError
	seenIters []client.TaggedIDsIterator // track known iterators to avoid leaking
	dedupeMap map[string]MultiTagResult
	filters   models.Filters
}

// NewMultiFetchTagsResult builds a new multi fetch tags result.
func NewMultiFetchTagsResult(opts models.TagOptions) MultiFetchTagsResult {
	return &multiSearchResult{
		dedupeMap: make(map[string]MultiTagResult, initSize),
		meta:      block.NewResultMetadata(),
		filters:   opts.Filters(),
	}
}

func (r *multiSearchResult) Close() error {
	r.Lock()
	defer r.Unlock()
	for _, iters := range r.seenIters {
		iters.Finalize()
	}

	r.seenIters = nil
	r.dedupeMap = nil
	r.err = xerrors.NewMultiError()

	return nil
}

func (r *multiSearchResult) FinalResult() (TagResult, error) {
	r.Lock()
	defer r.Unlock()

	err := r.err.FinalError()
	if err != nil {
		return TagResult{Metadata: r.meta}, err
	}

	result := make([]MultiTagResult, 0, len(r.dedupeMap))
	for _, it := range r.dedupeMap {
		result = append(result, it)
	}

	return TagResult{
		Tags:     result,
		Metadata: r.meta,
	}, nil
}

func (r *multiSearchResult) Add(
	newIterator client.TaggedIDsIterator,
	meta block.ResultMetadata,
	err error,
) {
	r.Lock()
	defer r.Unlock()

	if err != nil {
		r.err = r.err.Add(err)
		return
	}

	if r.seenIters == nil {
		r.seenIters = make([]client.TaggedIDsIterator, 0, initSize)
		r.meta = meta
	} else {
		r.meta = r.meta.CombineMetadata(meta)
	}

	r.seenIters = append(r.seenIters, newIterator)
	// Need to check the error to bail early after accumulating the iterators
	// otherwise when we close the the multi fetch result
	if !r.err.Empty() {
		// don't need to do anything if the final result is going to be an error
		return
	}

	for newIterator.Next() {
		_, ident, tagIter := newIterator.Current()
		shouldFilter, err := filterTagIterator(tagIter, r.filters)
		if err != nil {
			r.err = r.err.Add(err)
			return
		}

		if shouldFilter {
			// NB: skip here, the closer will free the tag iterator regardless.
			continue
		}

		id := ident.String()
		_, exists := r.dedupeMap[id]
		if !exists {
			r.dedupeMap[id] = MultiTagResult{
				ID:   ident,
				Iter: tagIter.Duplicate(),
			}
		}
	}

	if err := newIterator.Err(); err != nil {
		r.err = r.err.Add(err)
	}
}
