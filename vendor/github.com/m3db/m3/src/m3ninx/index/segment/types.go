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

package segment

import (
	"errors"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/postings"
)

var (
	// ErrClosed is the error returned when attempting to perform operations on a
	// segment that has already been closed.
	ErrClosed = errors.New("segment has been closed")
)

// Segment is a sub-collection of documents within an index.
type Segment interface {
	// FieldsIterable returns an iterable fields, for which is not
	// safe for concurrent use. For concurrent use call FieldsIterable
	// multiple times.
	FieldsIterable() FieldsIterable

	// TermsIterable returns an iterable terms, for which is not
	// safe for concurrent use. For concurrent use call TermsIterable
	// multiple times.
	TermsIterable() TermsIterable

	// Size returns the number of documents within the Segment. It returns
	// 0 if the Segment has been closed.
	Size() int64

	// ContainsID returns a bool indicating if the Segment contains the provided ID.
	ContainsID(docID []byte) (bool, error)

	// ContainsField returns a bool indicating if the Segment contains the provided field.
	ContainsField(field []byte) (bool, error)

	// Reader returns a point-in-time accessor to search the segment.
	Reader() (Reader, error)

	// Close closes the segment and releases any internal resources.
	Close() error
}

// Reader extends index reader interface to allow for reading
// of fields and terms.
type Reader interface {
	index.Reader
	FieldsIterable
	TermsIterable

	// ContainsField returns a bool indicating if the Segment contains the provided field.
	ContainsField(field []byte) (bool, error)
}

// FieldsIterable can iterate over segment fields, it is not by default
// concurrency safe.
type FieldsIterable interface {
	// Fields returns an iterator over the list of known fields, in order
	// by name, it is not valid for reading after mutating the
	// builder by inserting more documents.
	Fields() (FieldsIterator, error)
}

// FieldsPostingsListIterable can iterate over segment fields/postings lists, it is not by default
// concurrency safe.
type FieldsPostingsListIterable interface {
	// Fields returns an iterator over the list of known fields, in order
	// by name, it is not valid for reading after mutating the
	// builder by inserting more documents.
	FieldsPostingsList() (FieldsPostingsListIterator, error)
}

// TermsIterable can iterate over segment terms, it is not by default
// concurrency safe.
type TermsIterable interface {
	// Terms returns an iterator over the known terms values for the given
	// field, in order by name, it is not valid for reading after mutating the
	// builder by inserting more documents.
	Terms(field []byte) (TermsIterator, error)
}

// OrderedBytesIterator iterates over a collection of []bytes in lexicographical order.
type OrderedBytesIterator interface {
	// Next returns a bool indicating if there are any more elements.
	Next() bool

	// Current returns the current element.
	// NB: the element returned is only valid until the subsequent call to Next().
	Current() []byte

	// Err returns any errors encountered during iteration.
	Err() error

	// Close releases any resources held by the iterator.
	Close() error
}

// FieldsPostingsListIterator iterates over all known fields.
type FieldsPostingsListIterator interface {
	Iterator

	// Current returns the current field and associated postings list.
	// NB: the field returned is only valid until the subsequent call to Next().
	Current() ([]byte, postings.List)
}

// FieldsIterator iterates over all known fields.
type FieldsIterator interface {
	Iterator

	// Current returns the current field.
	// NB: the field returned is only valid until the subsequent call to Next().
	Current() []byte
}

// TermsIterator iterates over all known terms for the provided field.
type TermsIterator interface {
	Iterator

	// Current returns the current element.
	// NB: the element returned is only valid until the subsequent call to Next().
	Current() (term []byte, postings postings.List)
}

// Iterator holds common iterator methods.
type Iterator interface {
	// Next returns a bool indicating if there are any more elements.
	Next() bool

	// Err returns any errors encountered during iteration.
	Err() error

	// Close releases any resources held by the iterator.
	Close() error
}

// MutableSegment is a segment which can be updated.
type MutableSegment interface {
	Segment
	DocumentsBuilder

	// Fields returns an iterator over the list of known fields, in order
	// by name, it is not valid for reading after mutating the
	// builder by inserting more documents.
	Fields() (FieldsIterator, error)

	// Seal marks the Mutable Segment immutable.
	Seal() error

	// IsSealed returns true iff the segment is open and un-sealed.
	IsSealed() bool
}

// ImmutableSegment is segment that has been written to disk.
type ImmutableSegment interface {
	Segment

	FreeMmap() error
}

// Builder is a builder that can be used to construct segments.
type Builder interface {
	FieldsPostingsListIterable
	TermsIterable

	// Reset resets the builder for reuse.
	Reset()

	// Docs returns the current docs slice, this is not safe to modify
	// and is invalidated on a call to reset.
	Docs() []doc.Document

	// AllDocs returns an iterator over the documents known to the Reader.
	AllDocs() (index.IDDocIterator, error)
}

// DocumentsBuilder is a builder that has documents written to it.
type DocumentsBuilder interface {
	Builder
	index.Writer

	// SetIndexConcurrency sets the concurrency used for building the segment.
	SetIndexConcurrency(value int)

	// IndexConcurrency returns the concurrency used for building the segment.
	IndexConcurrency() int
}

// CloseableDocumentsBuilder is a builder that has documents written to it and has freeable resources.
type CloseableDocumentsBuilder interface {
	DocumentsBuilder

	Close() error
}

// SegmentsBuilder is a builder that is built from segments.
type SegmentsBuilder interface {
	Builder

	// AddSegments adds segments to build from.
	AddSegments(segments []Segment) error
}
