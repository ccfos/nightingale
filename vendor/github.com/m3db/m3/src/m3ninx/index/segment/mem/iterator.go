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
	"github.com/m3db/m3/src/m3ninx/postings"
)

// boundedPostingsIterator wraps a normal postings iterator but only returns IDs
// within a given range.
type boundedPostingsIterator struct {
	postings.Iterator
	curr   postings.ID
	limits readerDocRange
}

func newBoundedPostingsIterator(it postings.Iterator, limits readerDocRange) postings.Iterator {
	return &boundedPostingsIterator{
		Iterator: it,
		limits:   limits,
	}
}

func (it *boundedPostingsIterator) Next() bool {
	for {
		if !it.Iterator.Next() {
			return false
		}

		curr := it.Iterator.Current()
		// We are not assuming that the posting IDs are ordered otherwise we could return
		// false immediately when we exceed the end of the range.
		if curr < it.limits.startInclusive || curr >= it.limits.endExclusive {
			continue
		}

		it.curr = curr
		return true
	}
}

func (it *boundedPostingsIterator) Current() postings.ID {
	return it.curr
}
