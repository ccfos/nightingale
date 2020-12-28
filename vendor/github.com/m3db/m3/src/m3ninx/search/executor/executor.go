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

package executor

import (
	"errors"
	"sync"

	"github.com/m3db/m3/src/m3ninx/doc"
	"github.com/m3db/m3/src/m3ninx/index"
	"github.com/m3db/m3/src/m3ninx/search"
)

var (
	errExecutorClosed = errors.New("executor is closed")
)

type newIteratorFn func(s search.Searcher, rs index.Readers) (doc.Iterator, error)

type executor struct {
	sync.RWMutex

	newIteratorFn newIteratorFn
	readers       index.Readers

	closed bool
}

// NewExecutor returns a new Executor for executing queries.
func NewExecutor(rs index.Readers) search.Executor {
	return &executor{
		newIteratorFn: newIterator,
		readers:       rs,
	}
}

func (e *executor) Execute(q search.Query) (doc.Iterator, error) {
	e.RLock()
	defer e.RUnlock()
	if e.closed {
		return nil, errExecutorClosed
	}

	s, err := q.Searcher()
	if err != nil {
		return nil, err
	}

	iter, err := e.newIteratorFn(s, e.readers)
	if err != nil {
		return nil, err
	}

	return iter, nil
}

func (e *executor) Close() error {
	e.Lock()
	if e.closed {
		e.Unlock()
		return errExecutorClosed
	}
	e.closed = true
	e.Unlock()
	return e.readers.Close()
}
