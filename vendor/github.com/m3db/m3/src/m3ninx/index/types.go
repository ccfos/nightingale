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

package index

import (
	"errors"
	"regexp"
	"regexp/syntax"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/postings"
	xerrors "github.com/m3db/m3/src/x/errors"

	vregex "github.com/m3dbx/vellum/regexp"
)

// ErrDocNotFound is the error returned when there is no document for a given postings ID.
var ErrDocNotFound = errors.New("no document with given postings ID")

// Index is a collection of searchable documents.
type Index interface {
	Writer

	// Readers returns a set of readers representing a point-in-time snapshot of the index.
	Readers() (Readers, error)

	// Close closes the index and releases any internal resources.
	Close() error
}

// Writer is used to insert documents into an index.
type Writer interface {
	// Insert inserts the given document into the index and returns its ID. The document
	// is guaranteed to be searchable once the Insert method returns.
	Insert(d doc.Document) ([]byte, error)

	// InsertBatch inserts a batch of metrics into the index. The documents are guaranteed
	// to be searchable all at once when the Batch method returns. If the batch supports
	// partial updates and any errors are encountered on individual documents then a
	// BatchPartialError is returned.
	InsertBatch(b Batch) error
}

// Readable provides a point-in-time accessor to the documents in an index.
type Readable interface {
	DocRetriever

	// MatchField returns a postings list over all documents which match the given field.
	MatchField(field []byte) (postings.List, error)

	// MatchTerm returns a postings list over all documents which match the given term.
	MatchTerm(field, term []byte) (postings.List, error)

	// MatchRegexp returns a postings list over all documents which match the given
	// regular expression.
	MatchRegexp(field []byte, c CompiledRegex) (postings.List, error)

	// MatchAll returns a postings list for all documents known to the Reader.
	MatchAll() (postings.MutableList, error)

	// Docs returns an iterator over the documents whose IDs are in the provided
	// postings list.
	Docs(pl postings.List) (doc.Iterator, error)

	// AllDocs returns an iterator over the documents known to the Reader.
	AllDocs() (IDDocIterator, error)
}

// CompiledRegex is a collection of regexp compiled structs to allow
// amortisation of regexp construction costs.
type CompiledRegex struct {
	Simple      *regexp.Regexp
	FST         *vregex.Regexp
	FSTSyntax   *syntax.Regexp
	PrefixBegin []byte
	PrefixEnd   []byte
}

// DocRetriever returns the document associated with a postings ID. It returns
// ErrDocNotFound if there is no document corresponding to the given postings ID.
type DocRetriever interface {
	Doc(id postings.ID) (doc.Document, error)
}

// IDDocIterator is an extented documents Iterator which can also return the postings
// ID of the current document.
type IDDocIterator interface {
	doc.Iterator

	// PostingsID returns the current document postings ID.
	PostingsID() postings.ID
}

// IDIterator is a document ID iterator.
type IDIterator interface {
	// Next returns a bool indicating if the iterator has any more documents
	// to return.
	Next() bool

	// Current returns the current document ID.
	Current() []byte

	// PostingsID returns the current document postings ID.
	PostingsID() postings.ID

	// Err returns any errors encountered during iteration.
	Err() error

	// Close releases any internal resources used by the iterator.
	Close() error
}

// Reader provides a point-in-time accessor to the documents in an index.
type Reader interface {
	Readable

	// Close closes the reader and releases any internal resources.
	Close() error
}

// Readers is a slice of Reader.
type Readers []Reader

// Close closes all of the Readers in rs.
func (rs Readers) Close() error {
	multiErr := xerrors.NewMultiError()
	for _, r := range rs {
		err := r.Close()
		if err != nil {
			multiErr = multiErr.Add(err)
		}
	}
	return multiErr.FinalError()
}
