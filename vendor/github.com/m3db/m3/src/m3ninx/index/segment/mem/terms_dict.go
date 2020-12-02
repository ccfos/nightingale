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

package mem

import (
	re "regexp"
	"sync"

	"github.com/m3db/m3/src/m3ninx/doc"
	sgmt "github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/postings/roaring"
)

// termsDict is an in-memory terms dictionary. It maps fields to postings lists.
type termsDict struct {
	opts Options

	currFieldsPostingsLists []postings.List

	fields struct {
		sync.RWMutex
		*fieldsMap
	}
}

func newTermsDict(opts Options) termsDictionary {
	dict := &termsDict{
		opts: opts,
	}
	dict.fields.fieldsMap = newFieldsMap(fieldsMapOptions{
		InitialSize: opts.InitialCapacity(),
	})
	return dict
}

func (d *termsDict) Insert(field doc.Field, id postings.ID) error {
	postingsMap := d.getOrAddName(field.Name)
	return postingsMap.Add(field.Value, id)
}

func (d *termsDict) ContainsField(field []byte) bool {
	d.fields.RLock()
	defer d.fields.RUnlock()
	_, ok := d.fields.Get(field)
	return ok
}

func (d *termsDict) ContainsTerm(field, term []byte) bool {
	_, found := d.matchTerm(field, term)
	return found
}

func (d *termsDict) MatchTerm(field, term []byte) postings.List {
	pl, found := d.matchTerm(field, term)
	if !found {
		return d.opts.PostingsListPool().Get()
	}
	return pl
}

func (d *termsDict) Fields() sgmt.FieldsIterator {
	d.fields.RLock()
	defer d.fields.RUnlock()
	fields := d.opts.BytesSliceArrayPool().Get()
	for _, entry := range d.fields.Iter() {
		fields = append(fields, entry.Key())
	}
	return newBytesSliceIter(fields, d.opts)
}

func (d *termsDict) FieldsPostingsList() sgmt.FieldsPostingsListIterator {
	d.fields.RLock()
	defer d.fields.RUnlock()
	// NB(bodu): This is probably fine since the terms dict/mem segment is only used in tests.
	fields := make([]uniqueField, 0, d.fields.Len())
	for _, entry := range d.fields.Iter() {
		d.currFieldsPostingsLists = d.currFieldsPostingsLists[:0]
		field := entry.Key()
		pl := roaring.NewPostingsList()
		if postingsMap, ok := d.fields.Get(field); ok {
			for _, entry := range postingsMap.Iter() {
				d.currFieldsPostingsLists = append(d.currFieldsPostingsLists, entry.value)
			}
		}
		pl.UnionMany(d.currFieldsPostingsLists)
		fields = append(fields, uniqueField{
			field:        field,
			postingsList: pl,
		})
	}
	return newUniqueFieldsIter(fields, d.opts)
}

func (d *termsDict) Terms(field []byte) sgmt.TermsIterator {
	d.fields.RLock()
	defer d.fields.RUnlock()
	values, ok := d.fields.Get(field)
	if !ok {
		return sgmt.EmptyTermsIterator
	}
	return values.Keys()
}

func (d *termsDict) matchTerm(field, term []byte) (postings.List, bool) {
	d.fields.RLock()
	postingsMap, ok := d.fields.Get(field)
	d.fields.RUnlock()
	if !ok {
		return nil, false
	}
	pl, ok := postingsMap.Get(term)
	if !ok {
		return nil, false
	}
	return pl, true
}

func (d *termsDict) MatchRegexp(
	field []byte,
	compiled *re.Regexp,
) postings.List {
	d.fields.RLock()
	postingsMap, ok := d.fields.Get(field)
	d.fields.RUnlock()
	if !ok {
		return d.opts.PostingsListPool().Get()
	}
	pl, ok := postingsMap.GetRegex(compiled)
	if !ok {
		return d.opts.PostingsListPool().Get()
	}
	return pl
}

func (d *termsDict) Reset() {
	d.fields.Lock()
	defer d.fields.Unlock()

	// TODO(r): We actually want to keep the terms maps around so that they
	// can be reused and avoid reallocation, so instead of deleting them
	// we should just reset each one - however we were seeing some racey
	// issues so now just deleting all entries for now
	d.fields.Reallocate()
}

func (d *termsDict) getOrAddName(name []byte) *concurrentPostingsMap {
	// Cheap read lock to see if it already exists.
	d.fields.RLock()
	postingsMap, ok := d.fields.Get(name)
	d.fields.RUnlock()
	if ok {
		return postingsMap
	}

	// Acquire write lock and create.
	d.fields.Lock()
	postingsMap, ok = d.fields.Get(name)

	// Check if it's been created since we last acquired the lock.
	if ok {
		d.fields.Unlock()
		return postingsMap
	}

	postingsMap = newConcurrentPostingsMap(d.opts)
	d.fields.SetUnsafe(name, postingsMap, fieldsMapSetUnsafeOptions{
		NoCopyKey:     true,
		NoFinalizeKey: true,
	})
	d.fields.Unlock()
	return postingsMap
}
