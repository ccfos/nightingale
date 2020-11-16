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
	"errors"
	"sync"

	"github.com/m3db/m3/src/dbnode/storage/index/convert"
	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
)

var (
	errUnableToAddResultMissingID = errors.New("no id for result")
)

type results struct {
	sync.RWMutex

	nsID ident.ID
	opts QueryResultsOptions

	resultsMap     *ResultsMap
	totalDocsCount int

	idPool    ident.Pool
	bytesPool pool.CheckedBytesPool

	pool       QueryResultsPool
	noFinalize bool
}

// NewQueryResults returns a new query results object.
func NewQueryResults(
	namespaceID ident.ID,
	opts QueryResultsOptions,
	indexOpts Options,
) QueryResults {
	return &results{
		nsID:       namespaceID,
		opts:       opts,
		resultsMap: newResultsMap(indexOpts.IdentifierPool()),
		idPool:     indexOpts.IdentifierPool(),
		bytesPool:  indexOpts.CheckedBytesPool(),
		pool:       indexOpts.QueryResultsPool(),
	}
}

func (r *results) Reset(nsID ident.ID, opts QueryResultsOptions) {
	r.Lock()

	r.opts = opts

	// Finalize existing held nsID.
	if r.nsID != nil {
		r.nsID.Finalize()
	}
	// Make an independent copy of the new nsID.
	if nsID != nil {
		nsID = r.idPool.Clone(nsID)
	}
	r.nsID = nsID

	// Reset all values from map first
	for _, entry := range r.resultsMap.Iter() {
		tags := entry.Value()
		tags.Close()
	}

	// Reset all keys in the map next, this will finalize the keys.
	r.resultsMap.Reset()
	r.totalDocsCount = 0

	// NB: could do keys+value in one step but I'm trying to avoid
	// using an internal method of a code-gen'd type.

	r.Unlock()
}

// NB: If documents with duplicate IDs are added, they are simply ignored and
// the first document added with an ID is returned.
func (r *results) AddDocuments(batch []doc.Document) (int, int, error) {
	r.Lock()
	err := r.addDocumentsBatchWithLock(batch)
	size := r.resultsMap.Len()
	docsCount := r.totalDocsCount + len(batch)
	r.totalDocsCount = docsCount
	r.Unlock()
	return size, docsCount, err
}

func (r *results) addDocumentsBatchWithLock(batch []doc.Document) error {
	for i := range batch {
		_, size, err := r.addDocumentWithLock(batch[i])
		if err != nil {
			return err
		}
		if r.opts.SizeLimit > 0 && size >= r.opts.SizeLimit {
			// Early return if limit enforced and we hit our limit.
			break
		}
	}
	return nil
}

func (r *results) addDocumentWithLock(d doc.Document) (bool, int, error) {
	if len(d.ID) == 0 {
		return false, r.resultsMap.Len(), errUnableToAddResultMissingID
	}

	// NB: can cast the []byte -> ident.ID to avoid an alloc
	// before we're sure we need it.
	tsID := ident.BytesID(d.ID)

	// Need to apply filter if set first.
	if r.opts.FilterID != nil && !r.opts.FilterID(tsID) {
		return false, r.resultsMap.Len(), nil
	}

	// check if it already exists in the map.
	if r.resultsMap.Contains(tsID) {
		return false, r.resultsMap.Len(), nil
	}

	// i.e. it doesn't exist in the map, so we create the tags wrapping
	// fields provided by the document.
	tags := convert.ToSeriesTags(d, convert.Opts{NoClone: true})

	// It is assumed that the document is valid for the lifetime of the index
	// results.
	r.resultsMap.SetUnsafe(tsID, tags, ResultsMapSetUnsafeOptions{
		NoCopyKey:     true,
		NoFinalizeKey: true,
	})

	return true, r.resultsMap.Len(), nil
}

func (r *results) Namespace() ident.ID {
	r.RLock()
	v := r.nsID
	r.RUnlock()
	return v
}

func (r *results) Map() *ResultsMap {
	r.RLock()
	v := r.resultsMap
	r.RUnlock()
	return v
}

func (r *results) Size() int {
	r.RLock()
	v := r.resultsMap.Len()
	r.RUnlock()
	return v
}

func (r *results) TotalDocsCount() int {
	r.RLock()
	count := r.totalDocsCount
	r.RUnlock()
	return count
}

func (r *results) Finalize() {
	// Reset locks so cannot hold onto lock for call to Finalize.
	r.Reset(nil, QueryResultsOptions{})

	if r.pool == nil {
		return
	}
	r.pool.Put(r)
}
