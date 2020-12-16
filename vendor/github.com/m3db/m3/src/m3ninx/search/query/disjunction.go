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

package query

import (
	"fmt"

	"github.com/m3db/m3/src/m3ninx/generated/proto/querypb"
	"github.com/m3db/m3/src/m3ninx/search"
	"github.com/m3db/m3/src/m3ninx/search/searcher"
)

// DisjuctionQuery finds documents which match at least one of the given queries.
type DisjuctionQuery struct {
	queries []search.Query
}

// NewDisjunctionQuery constructs a new query which matches documents that match any
// of the given queries.
func NewDisjunctionQuery(queries []search.Query) search.Query {
	qs := make([]search.Query, 0, len(queries))
	for _, query := range queries {
		// Merge disjunction queries into slice of top-level queries.
		q, ok := query.(*DisjuctionQuery)
		if ok {
			qs = append(qs, q.queries...)
			continue
		}

		qs = append(qs, query)
	}
	return &DisjuctionQuery{
		queries: qs,
	}
}

// Searcher returns a searcher over the provided readers.
func (q *DisjuctionQuery) Searcher() (search.Searcher, error) {
	switch len(q.queries) {
	case 0:
		return searcher.NewEmptySearcher(), nil

	case 1:
		return q.queries[0].Searcher()
	}

	srs := make(search.Searchers, 0, len(q.queries))
	for _, q := range q.queries {
		sr, err := q.Searcher()
		if err != nil {
			return nil, err
		}
		srs = append(srs, sr)
	}

	return searcher.NewDisjunctionSearcher(srs)
}

// Equal reports whether q is equivalent to o.
func (q *DisjuctionQuery) Equal(o search.Query) bool {
	if len(q.queries) == 1 {
		return q.queries[0].Equal(o)
	}

	inner, ok := o.(*DisjuctionQuery)
	if !ok {
		return false
	}

	if len(q.queries) != len(inner.queries) {
		return false
	}

	// TODO: Should order matter?
	for i := range q.queries {
		if !q.queries[i].Equal(inner.queries[i]) {
			return false
		}
	}
	return true
}

// ToProto returns the Protobuf query struct corresponding to the disjunction query.
func (q *DisjuctionQuery) ToProto() *querypb.Query {
	qs := make([]*querypb.Query, 0, len(q.queries))
	for _, qry := range q.queries {
		qs = append(qs, qry.ToProto())
	}
	disj := querypb.DisjunctionQuery{Queries: qs}

	return &querypb.Query{
		Query: &querypb.Query_Disjunction{Disjunction: &disj},
	}
}

func (q *DisjuctionQuery) String() string {
	return fmt.Sprintf("disjunction(%s)", join(q.queries))
}
