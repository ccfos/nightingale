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

package builder

import (
	"github.com/m3db/m3/src/m3ninx/index/segment"
	"github.com/m3db/m3/src/m3ninx/postings"
)

type termsIter struct {
	err  error
	done bool

	currentIdx int
	current    termElem
	terms      []termElem
}

var _ segment.TermsIterator = &termsIter{}

func newTermsIter(
	orderedTerms []termElem,
) *termsIter {
	return &termsIter{
		currentIdx: -1,
		terms:      orderedTerms,
	}
}

func (b *termsIter) Next() bool {
	if b.done || b.err != nil {
		return false
	}
	b.currentIdx++
	if b.currentIdx >= len(b.terms) {
		b.done = true
		return false
	}
	b.current = b.terms[b.currentIdx]
	return true
}

func (b *termsIter) Current() ([]byte, postings.List) {
	return b.current.term, b.current.postings
}

func (b *termsIter) Err() error {
	return nil
}

func (b *termsIter) Len() int {
	return len(b.terms)
}

func (b *termsIter) Close() error {
	b.current = termElem{}
	b.terms = nil
	return nil
}
