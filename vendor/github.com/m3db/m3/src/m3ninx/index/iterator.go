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

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/postings"
)

var (
	errIteratorClosed = errors.New("iterator has been closed")
)

type idDocIterator struct {
	retriever    DocRetriever
	postingsIter postings.Iterator

	currDoc doc.Document
	currID  postings.ID
	closed  bool
	err     error
}

// NewIDDocIterator returns a new NewIDDocIterator.
func NewIDDocIterator(r DocRetriever, pi postings.Iterator) IDDocIterator {
	return &idDocIterator{
		retriever:    r,
		postingsIter: pi,
	}
}

func (it *idDocIterator) Next() bool {
	if it.closed || it.err != nil || !it.postingsIter.Next() {
		return false
	}
	id := it.postingsIter.Current()
	it.currID = id

	d, err := it.retriever.Doc(id)
	if err != nil {
		it.err = err
		return false
	}
	it.currDoc = d
	return true
}

func (it *idDocIterator) Current() doc.Document {
	return it.currDoc
}

func (it *idDocIterator) PostingsID() postings.ID {
	return it.currID
}

func (it *idDocIterator) Err() error {
	return it.err
}

func (it *idDocIterator) Close() error {
	if it.closed {
		return errIteratorClosed
	}
	it.closed = true
	it.currDoc = doc.Document{}
	it.currID = postings.ID(0)
	err := it.postingsIter.Close()
	return err
}
