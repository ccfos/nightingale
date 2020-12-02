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

	"github.com/m3db/m3/src/m3ninx/search"
)

const (
	separator         = ", "
	negationPrefix    = "negation("
	negationSeparator = "), negation("
	negationPostfix   = ")"
)

// singular returns a bool indicating whether a given query is composed of a single
// query and returns that query if so.
func singular(q search.Query) (search.Query, bool) {
	switch q := q.(type) {
	case *ConjuctionQuery:
		if len(q.queries) == 1 {
			return q.queries[0], true
		}
		return nil, false
	case *DisjuctionQuery:
		if len(q.queries) == 1 {
			return q.queries[0], true
		}
		return nil, false
	}
	return q, true
}

// join concatenates a slice of queries.
func join(qs []search.Query) string {
	switch len(qs) {
	case 0:
		return ""
	case 1:
		return qs[0].String()
	}

	var b bytes.Buffer
	b.WriteString(qs[0].String())
	for _, q := range qs[1:] {
		b.WriteString(separator)
		b.WriteString(q.String())
	}

	return b.String()
}

// joinNegation concatenates a slice of negated queries.
func joinNegation(qs []search.Query) string {
	switch len(qs) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("%s%s%s", negationPrefix, qs[0].String(), negationPostfix)
	}

	var b bytes.Buffer
	b.WriteString(negationPrefix)
	b.WriteString(qs[0].String())
	for _, q := range qs[1:] {
		b.WriteString(negationSeparator)
		b.WriteString(q.String())
	}

	b.WriteString(negationPostfix)
	return b.String()
}
