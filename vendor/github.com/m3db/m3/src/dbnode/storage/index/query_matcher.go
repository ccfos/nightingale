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
	"fmt"

	"github.com/m3db/m3/src/m3ninx/idx"

	"github.com/golang/mock/gomock"
)

// QueryMatcher is a gomock.Matcher that matches index.Query
type QueryMatcher interface {
	gomock.Matcher
}

// NewQueryMatcher returns a new QueryMatcher
func NewQueryMatcher(q Query) QueryMatcher {
	return &queryMatcher{
		query:   q,
		matcher: idx.NewQueryMatcher(q.Query),
	}
}

type queryMatcher struct {
	query   Query
	matcher idx.QueryMatcher
}

func (m *queryMatcher) Matches(x interface{}) bool {
	query, ok := x.(Query)
	if !ok {
		return false
	}

	return m.matcher.Matches(query.Query)
}

func (m *queryMatcher) String() string {
	return fmt.Sprintf("QueryMatcher %v", m.query)
}
