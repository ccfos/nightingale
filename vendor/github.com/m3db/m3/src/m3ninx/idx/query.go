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

package idx

import (
	"github.com/m3db/m3/src/m3ninx/search"
	"github.com/m3db/m3/src/m3ninx/search/query"
)

// Query encapsulates a search query for an index.
type Query struct {
	query search.Query
}

func (q Query) String() string {
	return q.query.String()
}

// NewQueryFromSearchQuery creates a new Query from a search.Query.
func NewQueryFromSearchQuery(q search.Query) Query {
	return Query{
		query: q,
	}
}

// NewAllQuery returns a new query for matching all documents.
func NewAllQuery() Query {
	return Query{
		query: query.NewAllQuery(),
	}
}

// NewFieldQuery returns a new query for finding documents which match a field exactly.
func NewFieldQuery(field []byte) Query {
	return Query{
		query: query.NewFieldQuery(field),
	}
}

// FieldQuery returns a bool indicating whether the Query is a FieldQuery,
// and the backing bytes of the Field.
func FieldQuery(q Query) (field []byte, ok bool) {
	fieldQuery, ok := q.query.(*query.FieldQuery)
	if !ok {
		return nil, false
	}
	return fieldQuery.Field(), true
}

// NewTermQuery returns a new query for finding documents which match a term exactly.
func NewTermQuery(field, term []byte) Query {
	return Query{
		query: query.NewTermQuery(field, term),
	}
}

// NewRegexpQuery returns a new query for finding documents which match a regular expression.
func NewRegexpQuery(field, regexp []byte) (Query, error) {
	q, err := query.NewRegexpQuery(field, regexp)
	if err != nil {
		return Query{}, err
	}
	return Query{
		query: q,
	}, nil
}

// MustCreateRegexpQuery is like NewRegexpQuery but panics if the query cannot be created.
func MustCreateRegexpQuery(field, regexp []byte) Query {
	q, err := query.NewRegexpQuery(field, regexp)
	if err != nil {
		panic(err)
	}
	return Query{
		query: q,
	}
}

// NewNegationQuery returns a new query for finding documents which don't match a given query.
func NewNegationQuery(q Query) Query {
	return Query{
		query: query.NewNegationQuery(q.SearchQuery()),
	}
}

// NewConjunctionQuery returns a new query for finding documents which match each of the
// given queries.
func NewConjunctionQuery(queries ...Query) Query {
	qs := make([]search.Query, 0, len(queries))
	for _, q := range queries {
		qs = append(qs, q.query)
	}
	return Query{
		query: query.NewConjunctionQuery(qs),
	}
}

// NewDisjunctionQuery returns a new query for finding documents which match at least one
// of the given queries.
func NewDisjunctionQuery(queries ...Query) Query {
	qs := make([]search.Query, 0, len(queries))
	for _, q := range queries {
		qs = append(qs, q.query)
	}
	return Query{
		query: query.NewDisjunctionQuery(qs),
	}
}

// SearchQuery returns the underlying search query for use during execution.
func (q Query) SearchQuery() search.Query {
	return q.query
}

// Equal reports whether q is equal to o.
func (q Query) Equal(o Query) bool {
	return q.query.Equal(o.query)
}
