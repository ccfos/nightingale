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

package search

import (
	"fmt"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/generated/proto/querypb"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/postings"
)

// Executor is responsible for executing queries over a snapshot.
type Executor interface {
	// Execute executes a query over the Executor's snapshot.
	Execute(q Query) (doc.Iterator, error)

	// Close closes the iterator.
	Close() error
}

// Query is a search query for documents.
type Query interface {
	fmt.Stringer

	// Searcher returns a Searcher for executing the query.
	Searcher() (Searcher, error)

	// Equal reports whether two queries are equivalent.
	Equal(q Query) bool

	// ToProto returns the Protobuf query struct corresponding to this query.
	ToProto() *querypb.Query
}

// Searcher executes a query against a given Reader. It returns the postings lists
// of the documents it matches for the given segment.
type Searcher interface {
	// Search executes a configured query against the given Reader.
	Search(index.Reader) (postings.List, error)
}

// Searchers is a slice of Searcher.
type Searchers []Searcher
