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

package mem

import (
	"errors"
	"sync"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	sgmt "github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
)

var (
	errSegmentReaderClosed = errors.New("segment reader is closed")
	errReaderNilRegex      = errors.New("nil regex received")
)

type reader struct {
	sync.RWMutex

	segment ReadableSegment
	limits  readerDocRange
	plPool  postings.Pool

	closed bool
}

type readerDocRange struct {
	startInclusive postings.ID
	endExclusive   postings.ID
}

func newReader(s ReadableSegment, l readerDocRange, p postings.Pool) sgmt.Reader {
	return &reader{
		segment: s,
		limits:  l,
		plPool:  p,
	}
}

func (r *reader) Fields() (sgmt.FieldsIterator, error) {
	return r.segment.Fields()
}

func (r *reader) ContainsField(field []byte) (bool, error) {
	return r.segment.ContainsField(field)
}

func (r *reader) Terms(field []byte) (sgmt.TermsIterator, error) {
	return r.segment.Terms(field)
}

func (r *reader) MatchField(field []byte) (postings.List, error) {
	// falling back to regexp .* as this segment implementation is only used in tests.
	return r.MatchRegexp(field, index.DotStarCompiledRegex())
}

func (r *reader) MatchTerm(field, term []byte) (postings.List, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errSegmentReaderClosed
	}

	// A reader can return IDs in the posting list which are greater than its limit.
	// The reader only guarantees that when fetching the documents associated with a
	// postings list through a call to Docs, IDs greater than or equal to the limit
	// will be filtered out.
	pl, err := r.segment.matchTerm(field, term)
	return pl, err
}

func (r *reader) MatchRegexp(field []byte, compiled index.CompiledRegex) (postings.List, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errSegmentReaderClosed
	}

	// A reader can return IDs in the posting list which are greater than its maximum
	// permitted ID. The reader only guarantees that when fetching the documents associated
	// with a postings list through a call to Docs will IDs greater than the maximum be
	// filtered out.
	compileRE := compiled.Simple
	if compileRE == nil {
		return nil, errReaderNilRegex
	}

	return r.segment.matchRegexp(field, compileRE)
}

func (r *reader) MatchAll() (postings.MutableList, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errSegmentReaderClosed
	}

	pl := r.plPool.Get()
	err := pl.AddRange(r.limits.startInclusive, r.limits.endExclusive)
	if err != nil {
		return nil, err
	}
	return pl, nil
}

func (r *reader) Doc(id postings.ID) (doc.Document, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return doc.Document{}, errSegmentReaderClosed
	}

	if id < r.limits.startInclusive || id >= r.limits.endExclusive {
		return doc.Document{}, index.ErrDocNotFound
	}

	return r.segment.getDoc(id)
}

func (r *reader) Docs(pl postings.List) (doc.Iterator, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errSegmentReaderClosed
	}
	boundedIter := newBoundedPostingsIterator(pl.Iterator(), r.limits)
	return r.getDocIterWithLock(boundedIter), nil
}

func (r *reader) AllDocs() (index.IDDocIterator, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errSegmentReaderClosed
	}

	pi := postings.NewRangeIterator(r.limits.startInclusive, r.limits.endExclusive)
	return r.getDocIterWithLock(pi), nil
}

func (r *reader) getDocIterWithLock(iter postings.Iterator) index.IDDocIterator {
	return index.NewIDDocIterator(r, iter)
}

func (r *reader) Close() error {
	r.Lock()
	if r.closed {
		r.Unlock()
		return errSegmentReaderClosed
	}
	r.closed = true
	r.Unlock()
	return nil
}
