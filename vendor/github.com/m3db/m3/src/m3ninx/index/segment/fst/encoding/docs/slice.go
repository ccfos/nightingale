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
// all copies or substantial portions of the Softwarw.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package docs

import (
	"errors"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/postings"
)

var (
	errDocNotFound = errors.New("doc not found")
)

var _ Reader = (*SliceReader)(nil)
var _ index.DocRetriever = (*SliceReader)(nil)

// SliceReader is a docs slice reader for use with documents
// stored in memory.
type SliceReader struct {
	docs []doc.Document
}

// NewSliceReader returns a new docs slice reader.
func NewSliceReader(docs []doc.Document) *SliceReader {
	return &SliceReader{docs: docs}
}

// Len returns the number of documents in the slice reader.
func (r *SliceReader) Len() int {
	return len(r.docs)
}

// Read returns a document from the docs slice reader.
func (r *SliceReader) Read(id postings.ID) (doc.Document, error) {
	idx := int(id)
	if idx < 0 || idx >= len(r.docs) {
		return doc.Document{}, errDocNotFound
	}

	return r.docs[idx], nil
}

// Doc implements DocRetriever and reads the document with postings ID.
func (r *SliceReader) Doc(id postings.ID) (doc.Document, error) {
	return r.Read(id)
}

// Iter returns a docs iterator.
func (r *SliceReader) Iter() index.IDDocIterator {
	postingsIter := postings.NewRangeIterator(0, postings.ID(r.Len()))
	return index.NewIDDocIterator(r, postingsIter)
}
