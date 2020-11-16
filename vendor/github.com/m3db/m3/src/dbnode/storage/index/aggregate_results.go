// Copyright (c) 2019 Uber Technologies, Inc.
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
	"sync"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/x/ident"
	"github.com/m3db/m3/src/x/pool"
)

const missingDocumentFields = "invalid document fields: empty %s"

// NB: emptyValues is an AggregateValues with no values, used for tracking
// terms only rather than terms and values.
var emptyValues = AggregateValues{hasValues: false}

type aggregatedResults struct {
	sync.RWMutex

	nsID          ident.ID
	aggregateOpts AggregateResultsOptions

	resultsMap     *AggregateResultsMap
	totalDocsCount int

	idPool    ident.Pool
	bytesPool pool.CheckedBytesPool

	pool       AggregateResultsPool
	valuesPool AggregateValuesPool
}

// NewAggregateResults returns a new AggregateResults object.
func NewAggregateResults(
	namespaceID ident.ID,
	aggregateOpts AggregateResultsOptions,
	opts Options,
) AggregateResults {
	return &aggregatedResults{
		nsID:          namespaceID,
		aggregateOpts: aggregateOpts,
		resultsMap:    newAggregateResultsMap(opts.IdentifierPool()),
		idPool:        opts.IdentifierPool(),
		bytesPool:     opts.CheckedBytesPool(),
		pool:          opts.AggregateResultsPool(),
		valuesPool:    opts.AggregateValuesPool(),
	}
}

func (r *aggregatedResults) Reset(
	nsID ident.ID,
	aggregateOpts AggregateResultsOptions,
) {
	r.Lock()

	r.aggregateOpts = aggregateOpts

	// finalize existing held nsID
	if r.nsID != nil {
		r.nsID.Finalize()
	}

	// make an independent copy of the new nsID
	if nsID != nil {
		nsID = r.idPool.Clone(nsID)
	}
	r.nsID = nsID

	// reset all values from map first
	for _, entry := range r.resultsMap.Iter() {
		valueMap := entry.Value()
		valueMap.finalize()
	}

	// reset all keys in the map next
	r.resultsMap.Reset()
	r.totalDocsCount = 0

	// NB: could do keys+value in one step but I'm trying to avoid
	// using an internal method of a code-gen'd type.
	r.Unlock()
}

func (r *aggregatedResults) AddDocuments(batch []doc.Document) (int, int, error) {
	r.Lock()
	err := r.addDocumentsBatchWithLock(batch)
	size := r.resultsMap.Len()
	docsCount := r.totalDocsCount + len(batch)
	r.totalDocsCount = docsCount
	r.Unlock()
	return size, docsCount, err
}

func (r *aggregatedResults) AggregateResultsOptions() AggregateResultsOptions {
	return r.aggregateOpts
}

func (r *aggregatedResults) AddFields(batch []AggregateResultsEntry) (int, int) {
	r.Lock()
	valueInsertions := 0
	for _, entry := range batch {
		f := entry.Field
		aggValues, ok := r.resultsMap.Get(f)
		if !ok {
			aggValues = r.valuesPool.Get()
			// we can avoid the copy because we assume ownership of the passed ident.ID,
			// but still need to finalize it.
			r.resultsMap.SetUnsafe(f, aggValues, AggregateResultsMapSetUnsafeOptions{
				NoCopyKey:     true,
				NoFinalizeKey: false,
			})
		} else {
			// because we already have a entry for this field, we release the ident back to
			// the underlying pool.
			f.Finalize()
		}
		valuesMap := aggValues.Map()
		for _, t := range entry.Terms {
			if !valuesMap.Contains(t) {
				// we can avoid the copy because we assume ownership of the passed ident.ID,
				// but still need to finalize it.
				valuesMap.SetUnsafe(t, struct{}{}, AggregateValuesMapSetUnsafeOptions{
					NoCopyKey:     true,
					NoFinalizeKey: false,
				})
				valueInsertions++
			} else {
				// because we already have a entry for this term, we release the ident back to
				// the underlying pool.
				t.Finalize()
			}
		}
	}
	size := r.resultsMap.Len()
	docsCount := r.totalDocsCount + valueInsertions
	r.totalDocsCount = docsCount
	r.Unlock()
	return size, docsCount
}

func (r *aggregatedResults) addDocumentsBatchWithLock(
	batch []doc.Document,
) error {
	for _, doc := range batch {
		switch r.aggregateOpts.Type {
		case AggregateTagNamesAndValues:
			if err := r.addDocumentWithLock(doc); err != nil {
				return err
			}

		case AggregateTagNames:
			// NB: if aggregating by name only, can ignore any additional documents
			// after the result map size exceeds the optional size limit, since all
			// incoming terms are either duplicates or new values which will exceed
			// the limit.
			size := r.resultsMap.Len()
			if r.aggregateOpts.SizeLimit > 0 && size >= r.aggregateOpts.SizeLimit {
				return nil
			}

			if err := r.addDocumentTermsWithLock(doc); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported aggregation type: %v", r.aggregateOpts.Type)
		}
	}

	return nil
}

func (r *aggregatedResults) addDocumentTermsWithLock(
	document doc.Document,
) error {
	for _, field := range document.Fields {
		if err := r.addTermWithLock(field.Name); err != nil {
			return fmt.Errorf("unable to add document terms [%+v]: %v", document, err)
		}
	}

	return nil
}

func (r *aggregatedResults) addTermWithLock(
	term []byte,
) error {
	if len(term) == 0 {
		return fmt.Errorf(missingDocumentFields, "term")
	}

	// if a term filter is provided, ensure this field matches the filter,
	// otherwise ignore it.
	filter := r.aggregateOpts.FieldFilter
	if filter != nil && !filter.Allow(term) {
		return nil
	}

	// NB: can cast the []byte -> ident.ID to avoid an alloc
	// before we're sure we need it.
	termID := ident.BytesID(term)
	if r.resultsMap.Contains(termID) {
		// NB: this term is already added; continue.
		return nil
	}

	// Set results map to an empty AggregateValues since we only care about
	// existence of the term in the map, rather than its set of values.
	r.resultsMap.Set(termID, emptyValues)
	return nil
}

func (r *aggregatedResults) addDocumentWithLock(
	document doc.Document,
) error {
	for _, field := range document.Fields {
		if err := r.addFieldWithLock(field.Name, field.Value); err != nil {
			return fmt.Errorf("unable to add document [%+v]: %v", document, err)
		}
	}

	return nil
}

func (r *aggregatedResults) addFieldWithLock(
	term []byte,
	value []byte,
) error {
	if len(term) == 0 {
		return fmt.Errorf(missingDocumentFields, "term")
	}

	// if a term filter is provided, ensure this field matches the filter,
	// otherwise ignore it.
	filter := r.aggregateOpts.FieldFilter
	if filter != nil && !filter.Allow(term) {
		return nil
	}

	// NB: can cast the []byte -> ident.ID to avoid an alloc
	// before we're sure we need it.
	termID := ident.BytesID(term)
	valueID := ident.BytesID(value)

	valueMap, found := r.resultsMap.Get(termID)
	if found {
		return valueMap.addValue(valueID)
	}

	// NB: if over limit, do not add any new values to the map.
	if r.aggregateOpts.SizeLimit > 0 &&
		r.resultsMap.Len() >= r.aggregateOpts.SizeLimit {
		// Early return if limit enforced and we hit our limit.
		return nil
	}

	aggValues := r.valuesPool.Get()
	if err := aggValues.addValue(valueID); err != nil {
		// Return these values to the pool.
		r.valuesPool.Put(aggValues)
		return err
	}

	r.resultsMap.Set(termID, aggValues)
	return nil
}

func (r *aggregatedResults) Namespace() ident.ID {
	r.RLock()
	ns := r.nsID
	r.RUnlock()
	return ns
}

func (r *aggregatedResults) Map() *AggregateResultsMap {
	r.RLock()
	m := r.resultsMap
	r.RUnlock()
	return m
}

func (r *aggregatedResults) Size() int {
	r.RLock()
	l := r.resultsMap.Len()
	r.RUnlock()
	return l
}

func (r *aggregatedResults) TotalDocsCount() int {
	r.RLock()
	count := r.totalDocsCount
	r.RUnlock()
	return count
}

func (r *aggregatedResults) Finalize() {
	r.Reset(nil, AggregateResultsOptions{})
	if r.pool == nil {
		return
	}

	r.pool.Put(r)
}
