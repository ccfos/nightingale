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

package ident

import (
	"fmt"

	"github.com/golang/mock/gomock"
)

// TagIterMatcher is a gomock.Matcher that matches TagIterator
type TagIterMatcher interface {
	gomock.Matcher
}

// NewTagIterMatcher returns a new TagIterMatcher
func NewTagIterMatcher(iter TagIterator) TagIterMatcher {
	return &tagIterMatcher{iter: iter}
}

type tagIterMatcher struct {
	iter TagIterator
}

func (m *tagIterMatcher) Matches(x interface{}) bool {
	t, ok := x.(TagIterator)
	if !ok {
		return false
	}
	// duplicate to ensure the both iterators can be re-used again
	obs := t.Duplicate()
	exp := m.iter.Duplicate()
	for exp.Next() {
		if !obs.Next() {
			return false
		}
		obsCurrent := obs.Current()
		expCurrent := exp.Current()
		if !expCurrent.Equal(obsCurrent) {
			return false
		}
	}
	if obs.Next() {
		return false
	}
	if exp.Err() != obs.Err() {
		return false
	}
	return true
}

func (m *tagIterMatcher) String() string {
	return fmt.Sprintf("tagIter %v", m.iter)
}
