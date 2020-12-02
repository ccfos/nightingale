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
	"errors"
	"sync"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"

	"github.com/pborman/uuid"
)

var (
	errCantGetReaderFromClosedSegment = errors.New("cant get reader from closed segment")
	errCantCloseClosedSegment         = errors.New("cant close closed segment")
)

// Ensure FST segment implements ImmutableSegment so can be casted upwards
// and mmap's can be freed.
var _ segment.ImmutableSegment = (*ReadThroughSegment)(nil)

// ReadThroughSegment wraps a segment with a postings list cache so that
// queries can be transparently cached in a read through manner. In addition,
// the postings lists returned by the segments may not be safe to use once the
// underlying segments are closed due to the postings lists pointing into the
// segments mmap'd region. As a result, the close method of the ReadThroughSegment
// will make sure that the cache is purged of all the segments postings lists before
// the segment itself is closed.
type ReadThroughSegment struct {
	sync.RWMutex

	segment segment.ImmutableSegment

	uuid              uuid.UUID
	postingsListCache *PostingsListCache

	opts ReadThroughSegmentOptions

	closed bool
}

// ReadThroughSegmentOptions is the options struct for the
// ReadThroughSegment.
type ReadThroughSegmentOptions struct {
	// Whether the postings list for regexp queries should be cached.
	CacheRegexp bool
	// Whether the postings list for term queries should be cached.
	CacheTerms bool
}

// NewReadThroughSegment creates a new read through segment.
func NewReadThroughSegment(
	seg segment.ImmutableSegment,
	cache *PostingsListCache,
	opts ReadThroughSegmentOptions,
) segment.Segment {
	return &ReadThroughSegment{
		segment:           seg,
		opts:              opts,
		uuid:              uuid.NewUUID(),
		postingsListCache: cache,
	}
}

// Reader returns a read through reader for the read through segment.
func (r *ReadThroughSegment) Reader() (segment.Reader, error) {
	r.RLock()
	defer r.RUnlock()
	if r.closed {
		return nil, errCantGetReaderFromClosedSegment
	}

	reader, err := r.segment.Reader()
	if err != nil {
		return nil, err
	}
	return newReadThroughSegmentReader(
		reader, r.uuid, r.postingsListCache, r.opts), nil
}

// Close purges all entries in the cache associated with this segment,
// and then closes the underlying segment.
func (r *ReadThroughSegment) Close() error {
	r.Lock()
	defer r.Unlock()
	if r.closed {
		return errCantCloseClosedSegment
	}

	r.closed = true

	if r.postingsListCache != nil {
		// Purge segments from the cache before closing the segment to avoid
		// temporarily having postings lists in the cache whose underlying
		// bytes are no longer mmap'd.
		r.postingsListCache.PurgeSegment(r.uuid)
	}
	return r.segment.Close()
}

// FieldsIterable is a pass through call to the segment, since there's no
// postings lists to cache for queries.
func (r *ReadThroughSegment) FieldsIterable() segment.FieldsIterable {
	return r.segment.FieldsIterable()
}

// TermsIterable is a pass through call to the segment, since there's no
// postings lists to cache for queries.
func (r *ReadThroughSegment) TermsIterable() segment.TermsIterable {
	return r.segment.TermsIterable()
}

// ContainsID is a pass through call to the segment, since there's no
// postings lists to cache for queries.
func (r *ReadThroughSegment) ContainsID(id []byte) (bool, error) {
	return r.segment.ContainsID(id)
}

// ContainsField is a pass through call to the segment, since there's no
// postings lists to cache for queries.
func (r *ReadThroughSegment) ContainsField(field []byte) (bool, error) {
	return r.segment.ContainsField(field)
}

// FreeMmap frees the mmapped data if any.
func (r *ReadThroughSegment) FreeMmap() error {
	return r.segment.FreeMmap()
}

// Size is a pass through call to the segment, since there's no
// postings lists to cache for queries.
func (r *ReadThroughSegment) Size() int64 {
	return r.segment.Size()
}

type readThroughSegmentReader struct {
	// reader is explicitly not embedded at the top level
	// of the struct to force new methods added to index.Reader
	// to be explicitly supported by the read through cache.
	reader            segment.Reader
	opts              ReadThroughSegmentOptions
	uuid              uuid.UUID
	postingsListCache *PostingsListCache
}

func newReadThroughSegmentReader(
	reader segment.Reader,
	uuid uuid.UUID,
	cache *PostingsListCache,
	opts ReadThroughSegmentOptions,
) segment.Reader {
	return &readThroughSegmentReader{
		reader:            reader,
		opts:              opts,
		uuid:              uuid,
		postingsListCache: cache,
	}
}

// MatchRegexp returns a cached posting list or queries the underlying
// segment if their is a cache miss.
func (s *readThroughSegmentReader) MatchRegexp(
	field []byte,
	c index.CompiledRegex,
) (postings.List, error) {
	if s.postingsListCache == nil || !s.opts.CacheRegexp {
		return s.reader.MatchRegexp(field, c)
	}

	// TODO(rartoul): Would be nice to not allocate strings here.
	fieldStr := string(field)
	patternStr := c.FSTSyntax.String()
	pl, ok := s.postingsListCache.GetRegexp(s.uuid, fieldStr, patternStr)
	if ok {
		return pl, nil
	}

	pl, err := s.reader.MatchRegexp(field, c)
	if err == nil {
		s.postingsListCache.PutRegexp(s.uuid, fieldStr, patternStr, pl)
	}
	return pl, err
}

// MatchTerm returns a cached posting list or queries the underlying
// segment if their is a cache miss.
func (s *readThroughSegmentReader) MatchTerm(
	field []byte, term []byte,
) (postings.List, error) {
	if s.postingsListCache == nil || !s.opts.CacheTerms {
		return s.reader.MatchTerm(field, term)
	}

	// TODO(rartoul): Would be nice to not allocate strings here.
	fieldStr := string(field)
	patternStr := string(term)
	pl, ok := s.postingsListCache.GetTerm(s.uuid, fieldStr, patternStr)
	if ok {
		return pl, nil
	}

	pl, err := s.reader.MatchTerm(field, term)
	if err == nil {
		s.postingsListCache.PutTerm(s.uuid, fieldStr, patternStr, pl)
	}
	return pl, err
}

// MatchField returns a cached posting list or queries the underlying
// segment if their is a cache miss.
func (s *readThroughSegmentReader) MatchField(field []byte) (postings.List, error) {
	if s.postingsListCache == nil || !s.opts.CacheTerms {
		return s.reader.MatchField(field)
	}

	// TODO(rartoul): Would be nice to not allocate strings here.
	fieldStr := string(field)
	pl, ok := s.postingsListCache.GetField(s.uuid, fieldStr)
	if ok {
		return pl, nil
	}

	pl, err := s.reader.MatchField(field)
	if err == nil {
		s.postingsListCache.PutField(s.uuid, fieldStr, pl)
	}
	return pl, err
}

// MatchAll is a pass through call, since there's no postings list to cache.
// NB(r): The postings list returned by match all is just an iterator
// from zero to the maximum document number indexed by the segment and as such
// causes no allocations to compute and construct.
func (s *readThroughSegmentReader) MatchAll() (postings.MutableList, error) {
	return s.reader.MatchAll()
}

// AllDocs is a pass through call, since there's no postings list to cache.
func (s *readThroughSegmentReader) AllDocs() (index.IDDocIterator, error) {
	return s.reader.AllDocs()
}

// Doc is a pass through call, since there's no postings list to cache.
func (s *readThroughSegmentReader) Doc(id postings.ID) (doc.Document, error) {
	return s.reader.Doc(id)
}

// Docs is a pass through call, since there's no postings list to cache.
func (s *readThroughSegmentReader) Docs(pl postings.List) (doc.Iterator, error) {
	return s.reader.Docs(pl)
}

// Fields is a pass through call.
func (s *readThroughSegmentReader) Fields() (segment.FieldsIterator, error) {
	return s.reader.Fields()
}

// ContainsField is a pass through call.
func (s *readThroughSegmentReader) ContainsField(field []byte) (bool, error) {
	return s.reader.ContainsField(field)
}

// Terms is a pass through call.
func (s *readThroughSegmentReader) Terms(field []byte) (segment.TermsIterator, error) {
	return s.reader.Terms(field)
}

// Close is a pass through call.
func (s *readThroughSegmentReader) Close() error {
	return s.reader.Close()
}
