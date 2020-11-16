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

// ConjuctionQuery finds documents which match at least one of the given queries.
type ConjuctionQuery struct {
	queries   []search.Query
	negations []search.Query
}

// NewConjunctionQuery constructs a new query which matches documents which match all
// of the given queries.
func NewConjunctionQuery(queries []search.Query) search.Query {
	qs := make([]search.Query, 0, len(queries))
	ns := make([]search.Query, 0, len(queries))
	for _, query := range queries {
		switch query := query.(type) {
		case *ConjuctionQuery:
			// Merge conjunction queries into slice of top-level queries.
			qs = append(qs, query.queries...)
			continue
		case *NegationQuery:
			ns = append(ns, query.query)
		default:
			qs = append(qs, query)
		}
	}

	if len(qs) == 0 && len(ns) > 0 {
		// We need to ensure we have at least one non-negation query from which we can calculate
		// the set difference with the other negation queries.
		qs = append(qs, queries[0])
		ns = ns[1:]
	}

	return &ConjuctionQuery{
		queries:   qs,
		negations: ns,
	}
}

// Searcher returns a searcher over the provided readers.
func (q *ConjuctionQuery) Searcher() (search.Searcher, error) {
	switch {
	case len(q.queries) == 0:
		return searcher.NewEmptySearcher(), nil

	case len(q.queries) == 1 && len(q.negations) == 0:
		return q.queries[0].Searcher()
	}

	qsrs := make(search.Searchers, 0, len(q.queries))
	for _, q := range q.queries {
		sr, err := q.Searcher()
		if err != nil {
			return nil, err
		}
		qsrs = append(qsrs, sr)
	}

	nsrs := make(search.Searchers, 0, len(q.negations))
	for _, q := range q.negations {
		sr, err := q.Searcher()
		if err != nil {
			return nil, err
		}
		nsrs = append(nsrs, sr)
	}

	return searcher.NewConjunctionSearcher(qsrs, nsrs)
}

// Equal reports whether q is equivalent to o.
func (q *ConjuctionQuery) Equal(o search.Query) bool {
	if len(q.queries) == 1 && len(q.negations) == 0 {
		return q.queries[0].Equal(o)
	}

	inner, ok := o.(*ConjuctionQuery)
	if !ok {
		return false
	}

	if len(q.queries) != len(inner.queries) {
		return false
	}

	if len(q.negations) != len(inner.negations) {
		return false
	}

	// TODO: Should order matter?
	for i := range q.queries {
		if !q.queries[i].Equal(inner.queries[i]) {
			return false
		}
	}

	for i := range q.negations {
		if !q.negations[i].Equal(inner.negations[i]) {
			return false
		}
	}

	return true
}

// ToProto returns the Protobuf query struct corresponding to the conjunction query.
func (q *ConjuctionQuery) ToProto() *querypb.Query {
	qs := make([]*querypb.Query, 0, len(q.queries)+len(q.negations))

	for _, qry := range q.queries {
		qs = append(qs, qry.ToProto())
	}

	for _, qry := range q.negations {
		neg := NewNegationQuery(qry)
		qs = append(qs, neg.ToProto())
	}

	conj := querypb.ConjunctionQuery{Queries: qs}
	return &querypb.Query{
		Query: &querypb.Query_Conjunction{Conjunction: &conj},
	}
}

func (q *ConjuctionQuery) String() string {
	if len(q.negations) > 0 {
		return fmt.Sprintf("conjunction(%s,%s)",
			join(q.queries), joinNegation(q.negations))
	}

	return fmt.Sprintf("conjunction(%s)", join(q.queries))
}
