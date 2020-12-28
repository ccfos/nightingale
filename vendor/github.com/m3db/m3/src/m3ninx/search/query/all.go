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

package query

import (
	"fmt"

	"github.com/m3db/m3/src/m3ninx/generated/proto/querypb"
	"github.com/m3db/m3/src/m3ninx/search"
	"github.com/m3db/m3/src/m3ninx/search/searcher"
)

// AllQuery returns a query which matches all known documents.
type AllQuery struct{}

// NewAllQuery constructs a new AllQuery.
func NewAllQuery() search.Query {
	return &AllQuery{}
}

// Searcher returns a searcher over the provided readers.
func (q *AllQuery) Searcher() (search.Searcher, error) {
	return searcher.NewAllSearcher(), nil
}

// Equal reports whether q is equivalent to o.
func (q *AllQuery) Equal(o search.Query) bool {
	o, ok := singular(o)
	if !ok {
		return false
	}

	_, ok = o.(*AllQuery)
	return ok
}

// ToProto returns the Protobuf query struct corresponding to the match all query.
func (q *AllQuery) ToProto() *querypb.Query {
	return &querypb.Query{
		Query: &querypb.Query_All{
			All: &querypb.AllQuery{},
		},
	}
}

func (q *AllQuery) String() string {
	return fmt.Sprintf("all()")
}
