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

// NegationQuery finds document which do not match a given query.
type NegationQuery struct {
	query search.Query
}

// NewNegationQuery constructs a new NegationQuery for the given query.
func NewNegationQuery(q search.Query) search.Query {
	return &NegationQuery{
		query: q,
	}
}

// Searcher returns a searcher over the provided readers.
func (q *NegationQuery) Searcher() (search.Searcher, error) {
	s, err := q.query.Searcher()
	if err != nil {
		return nil, err
	}
	return searcher.NewNegationSearcher(s)
}

// Equal reports whether q is equivalent to o.
func (q *NegationQuery) Equal(o search.Query) bool {
	o, ok := singular(o)
	if !ok {
		return false
	}

	inner, ok := o.(*NegationQuery)
	if !ok {
		return false
	}

	return q.query.Equal(inner.query)
}

// ToProto returns the Protobuf query struct corresponding to the term query.
func (q *NegationQuery) ToProto() *querypb.Query {
	inner := q.query.ToProto()
	qry := querypb.NegationQuery{Query: inner}

	return &querypb.Query{
		Query: &querypb.Query_Negation{Negation: &qry},
	}
}

func (q *NegationQuery) String() string {
	return fmt.Sprintf("negation(%s)", q.query)
}
