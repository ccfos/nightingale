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
	"bytes"
	"fmt"

	"github.com/m3db/m3/src/m3ninx/generated/proto/querypb"
	"github.com/m3db/m3/src/m3ninx/search"
	"github.com/m3db/m3/src/m3ninx/search/searcher"
)

// TermQuery finds document which match the given term exactly.
type TermQuery struct {
	field []byte
	term  []byte
}

// NewTermQuery constructs a new TermQuery for the given field and term.
func NewTermQuery(field, term []byte) search.Query {
	return &TermQuery{
		field: field,
		term:  term,
	}
}

// Searcher returns a searcher over the provided readers.
func (q *TermQuery) Searcher() (search.Searcher, error) {
	return searcher.NewTermSearcher(q.field, q.term), nil
}

// Equal reports whether q is equivalent to o.
func (q *TermQuery) Equal(o search.Query) bool {
	o, ok := singular(o)
	if !ok {
		return false
	}

	inner, ok := o.(*TermQuery)
	if !ok {
		return false
	}

	return bytes.Equal(q.field, inner.field) && bytes.Equal(q.term, inner.term)
}

// ToProto returns the Protobuf query struct corresponding to the term query.
func (q *TermQuery) ToProto() *querypb.Query {
	term := querypb.TermQuery{
		Field: q.field,
		Term:  q.term,
	}

	return &querypb.Query{
		Query: &querypb.Query_Term{Term: &term},
	}
}

func (q *TermQuery) String() string {
	return fmt.Sprintf("term(%s, %s)", q.field, q.term)
}
