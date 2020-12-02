// Copyright (c) 2017 Uber Technologies, Inc.
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

package namespace

import (
	xwatch "github.com/m3db/m3/src/x/watch"
)

type mapWatch struct {
	xwatch.Watch
}

// NewWatch creates a new watch on a topology map
// from a generic watch that watches a Map
func NewWatch(w xwatch.Watch) Watch {
	return &mapWatch{w}
}

func (w *mapWatch) C() <-chan struct{} {
	return w.Watch.C()
}

func (w *mapWatch) Get() Map {
	value := w.Watch.Get()
	if value == nil {
		return nil
	}
	return value.(Map)
}

func (w *mapWatch) Close() error {
	w.Watch.Close()
	return nil
}
