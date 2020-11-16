// Copyright (c) 2017 Uber Technologies, Inc.
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
	re "regexp"
	"sync"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
	"github.com/m3db/m3/src/m3ninx/util"
)

var (
	errSegmentSealed     = errors.New("unable to seal, segment has already been sealed")
	errSegmentIsUnsealed = errors.New("un-supported operation on an un-sealed mutable segment")
)

// nolint: maligned
type memSegment struct {
	offset    int
	plPool    postings.Pool
	newUUIDFn util.NewUUIDFn

	state struct {
		sync.RWMutex
		closed bool
		sealed bool
	}

	// Mapping of postings ID to document.
	docs struct {
		sync.RWMutex
		data []doc.Document
	}

	// Mapping of term to postings list.
	termsDict termsDictionary

	writer struct {
		sync.Mutex
		idSet  *idsMap
		nextID postings.ID
	}
	readerID postings.AtomicID
}

// NewSegment returns a new in-memory mutable segment. It will start assigning
// postings IDs at the provided offset.
func NewSegment(opts Options) (segment.MutableSegment, error) {
	s := &memSegment{
		plPool:    opts.PostingsListPool(),
		newUUIDFn: opts.NewUUIDFn(),
		termsDict: newTermsDict(opts),
		readerID:  postings.NewAtomicID(0),
	}

	s.docs.data = make([]doc.Document, opts.InitialCapacity())

	s.writer.idSet = newIDsMap(256)
	s.writer.nextID = 0
	return s, nil
}

func (s *memSegment) SetIndexConcurrency(value int) {
	// No-op, does not support concurrent indexing.
}

func (s *memSegment) IndexConcurrency() int {
	return 1
}

func (s *memSegment) Reset() {
	s.state.Lock()
	defer s.state.Unlock()

	s.state.sealed = false

	s.termsDict.Reset()
	s.readerID = postings.NewAtomicID(0)

	var empty doc.Document
	for i := range s.docs.data {
		s.docs.data[i] = empty
	}
	s.docs.data = s.docs.data[:0]

	s.writer.idSet.Reset()
	s.writer.nextID = 0
}

func (s *memSegment) Size() int64 {
	s.state.RLock()
	closed := s.state.closed
	size := int64(s.readerID.Load())
	s.state.RUnlock()
	if closed {
		return 0
	}
	return size
}

func (s *memSegment) Docs() []doc.Document {
	s.state.RLock()
	defer s.state.RUnlock()

	s.docs.RLock()
	defer s.docs.RUnlock()

	return s.docs.data[:s.readerID.Load()]
}

func (s *memSegment) ContainsID(id []byte) (bool, error) {
	s.state.RLock()
	if s.state.closed {
		s.state.RUnlock()
		return false, segment.ErrClosed
	}

	contains := s.containsIDWithStateLock(id)
	s.state.RUnlock()
	return contains, nil
}

func (s *memSegment) containsIDWithStateLock(id []byte) bool {
	return s.termsDict.ContainsTerm(doc.IDReservedFieldName, id)
}

func (s *memSegment) ContainsField(f []byte) (bool, error) {
	s.state.RLock()
	if s.state.closed {
		s.state.RUnlock()
		return false, segment.ErrClosed
	}

	contains := s.termsDict.ContainsField(f)
	s.state.RUnlock()
	return contains, nil
}

func (s *memSegment) Insert(d doc.Document) ([]byte, error) {
	s.state.RLock()
	defer s.state.RUnlock()
	if s.state.closed {
		return nil, segment.ErrClosed
	}

	{
		s.writer.Lock()
		defer s.writer.Unlock()

		b := index.NewBatch([]doc.Document{d})
		b.AllowPartialUpdates = false
		if err := s.prepareDocsWithLocks(b, nil); err != nil {
			return nil, err
		}

		// Update the document in case we generated a UUID for it.
		d = b.Docs[0]

		if err := s.insertDocWithLocks(d); err != nil {
			return nil, err
		}
		s.readerID.Inc()
	}

	return d.ID, nil
}

func (s *memSegment) InsertBatch(b index.Batch) error {
	s.state.RLock()
	defer s.state.RUnlock()
	if s.state.closed {
		return segment.ErrClosed
	}

	batchErr := index.NewBatchPartialError()
	{
		s.writer.Lock()
		defer s.writer.Unlock()

		if err := s.prepareDocsWithLocks(b, batchErr); err != nil {
			return err
		}

		numInserts := uint32(0)
		for i, d := range b.Docs {
			// NB(prateek): we override a document to have no ID when
			// it doesn't need to be inserted.
			if !d.HasID() {
				continue
			}
			if err := s.insertDocWithLocks(d); err != nil {
				if !b.AllowPartialUpdates {
					return err
				}
				batchErr.Add(index.BatchError{Err: err, Idx: i})
				continue
			}
			numInserts++
		}
		s.readerID.Add(numInserts)
	}

	if !batchErr.IsEmpty() {
		return batchErr
	}
	return nil
}

// prepareDocsWithLocks ensures the given documents can be inserted into the index. It
// must be called with the state and writer locks.
func (s *memSegment) prepareDocsWithLocks(
	b index.Batch,
	batchErr *index.BatchPartialError,
) error {
	s.writer.idSet.Reset()

	var emptyDoc doc.Document
	for i := 0; i < len(b.Docs); i++ {
		d := b.Docs[i]
		if err := d.Validate(); err != nil {
			if !b.AllowPartialUpdates {
				return err
			}
			batchErr.Add(index.BatchError{Err: err, Idx: i})
			b.Docs[i] = emptyDoc
			continue
		}

		if d.HasID() {
			if s.containsIDWithStateLock(d.ID) {
				// The segment already contains this document so we can remove it from those
				// we need to index.
				b.Docs[i] = emptyDoc
				continue
			}

			if _, ok := s.writer.idSet.Get(d.ID); ok {
				if !b.AllowPartialUpdates {
					return index.ErrDuplicateID
				}
				batchErr.Add(index.BatchError{Err: index.ErrDuplicateID, Idx: i})
				b.Docs[i] = emptyDoc
				continue
			}
		} else {
			id, err := s.newUUIDFn()
			if err != nil {
				if !b.AllowPartialUpdates {
					return err
				}
				batchErr.Add(index.BatchError{Err: err, Idx: i})
				b.Docs[i] = emptyDoc
				continue
			}

			d.ID = id

			// Update the document in the batch since we added an ID to it.
			b.Docs[i] = d
		}

		s.writer.idSet.SetUnsafe(d.ID, struct{}{}, idsMapSetUnsafeOptions{
			NoCopyKey:     true,
			NoFinalizeKey: true,
		})
	}

	return nil
}

// insertDocWithLocks inserts a document into the index. It must be called with the
// state and writer locks.
func (s *memSegment) insertDocWithLocks(d doc.Document) error {
	nextID := s.writer.nextID
	s.storeDocWithStateLock(nextID, d)
	s.writer.nextID++
	return s.indexDocWithStateLock(nextID, d)
}

// indexDocWithStateLock indexes the fields of a document in the segment's terms
// dictionary. It must be called with the segment's state lock.
func (s *memSegment) indexDocWithStateLock(id postings.ID, d doc.Document) error {
	for _, f := range d.Fields {
		if err := s.termsDict.Insert(f, id); err != nil {
			return err
		}
	}
	return s.termsDict.Insert(doc.Field{
		Name:  doc.IDReservedFieldName,
		Value: d.ID,
	}, id)
}

// storeDocWithStateLock stores a documents into the segment's mapping of postings
// IDs to documents. It must be called with the segment's state lock.
func (s *memSegment) storeDocWithStateLock(id postings.ID, d doc.Document) {
	idx := int(id)

	// Can return early if we have sufficient capacity.
	{
		s.docs.RLock()
		size := len(s.docs.data)
		if size > idx {
			// NB(prateek): We only need a Read-lock here despite an insert operation because
			// we're guaranteed to never have conflicts with docID (it's monotonically increasing),
			// and have checked `i.docs.data` is large enough.
			s.docs.data[idx] = d
			s.docs.RUnlock()
			return
		}
		s.docs.RUnlock()
	}

	// Otherwise we need to expand capacity.
	{
		s.docs.Lock()
		size := len(s.docs.data)

		// The slice has already been expanded since we released the lock.
		if size > idx {
			s.docs.data[idx] = d
			s.docs.Unlock()
			return
		}

		data := make([]doc.Document, 2*(size+1))
		copy(data, s.docs.data)
		s.docs.data = data
		s.docs.data[idx] = d
		s.docs.Unlock()
	}
}

func (s *memSegment) Reader() (segment.Reader, error) {
	s.state.RLock()
	defer s.state.RUnlock()
	if s.state.closed {
		return nil, segment.ErrClosed
	}

	limits := readerDocRange{
		startInclusive: postings.ID(0),
		endExclusive:   s.readerID.Load(),
	}
	return newReader(s, limits, s.plPool), nil
}

func (s *memSegment) AllDocs() (index.IDDocIterator, error) {
	r, err := s.Reader()
	if err != nil {
		return nil, err
	}
	return r.AllDocs()
}

func (s *memSegment) matchTerm(field, term []byte) (postings.List, error) {
	s.state.RLock()
	defer s.state.RUnlock()
	if s.state.closed {
		return nil, segment.ErrClosed
	}

	return s.termsDict.MatchTerm(field, term), nil
}

func (s *memSegment) matchRegexp(field []byte, compiled *re.Regexp) (postings.List, error) {
	s.state.RLock()
	defer s.state.RUnlock()
	if s.state.closed {
		return nil, segment.ErrClosed
	}

	return s.termsDict.MatchRegexp(field, compiled), nil
}

func (s *memSegment) getDoc(id postings.ID) (doc.Document, error) {
	s.state.RLock()
	defer s.state.RUnlock()
	if s.state.closed {
		return doc.Document{}, segment.ErrClosed
	}

	idx := int(id)

	s.docs.RLock()
	if idx >= len(s.docs.data) {
		s.docs.RUnlock()
		return doc.Document{}, index.ErrDocNotFound
	}
	d := s.docs.data[idx]
	s.docs.RUnlock()

	return d, nil
}

func (s *memSegment) Close() error {
	s.state.Lock()
	defer s.state.Unlock()
	if s.state.closed {
		return segment.ErrClosed
	}

	s.state.closed = true
	return nil
}

func (s *memSegment) IsSealed() bool {
	s.state.Lock()
	defer s.state.Unlock()
	if s.state.closed {
		return false
	}
	return s.state.sealed
}

func (s *memSegment) Seal() error {
	s.state.Lock()
	defer s.state.Unlock()
	if s.state.closed {
		return segment.ErrClosed
	}

	if s.state.sealed {
		return errSegmentSealed
	}

	s.state.sealed = true
	return nil
}

func (s *memSegment) Fields() (segment.FieldsIterator, error) {
	s.state.RLock()
	defer s.state.RUnlock()
	if err := s.checkIsSealedWithRLock(); err != nil {
		return nil, err
	}
	return s.termsDict.Fields(), nil
}

func (s *memSegment) FieldsPostingsList() (segment.FieldsPostingsListIterator, error) {
	s.state.RLock()
	defer s.state.RUnlock()
	if err := s.checkIsSealedWithRLock(); err != nil {
		return nil, err
	}
	return s.termsDict.FieldsPostingsList(), nil
}

func (s *memSegment) Terms(name []byte) (segment.TermsIterator, error) {
	s.state.RLock()
	defer s.state.RUnlock()
	if err := s.checkIsSealedWithRLock(); err != nil {
		return nil, err
	}
	return s.termsDict.Terms(name), nil
}

func (s *memSegment) FieldsIterable() segment.FieldsIterable {
	return s
}

func (s *memSegment) FieldsPostingsListIterable() segment.FieldsPostingsListIterable {
	return s
}

func (s *memSegment) TermsIterable() segment.TermsIterable {
	return s
}

func (s *memSegment) checkIsSealedWithRLock() error {
	if s.state.closed {
		return segment.ErrClosed
	}
	if !s.state.sealed {
		return errSegmentIsUnsealed
	}
	return nil
}
