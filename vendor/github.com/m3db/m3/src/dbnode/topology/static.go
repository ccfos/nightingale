// Copyright (c) 2016 Uber Technologies, Inc.
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

package topology

import (
	"errors"

	xwatch "github.com/m3db/m3/src/x/watch"
)

var (
	errUnownedShard = errors.New("unowned shard")
)

type staticInitializer struct {
	opts StaticOptions
}

// NewStaticInitializer creates a static topology initializer.
func NewStaticInitializer(opts StaticOptions) Initializer {
	return staticInitializer{opts}
}

func (i staticInitializer) Init() (Topology, error) {
	if err := i.opts.Validate(); err != nil {
		return nil, err
	}
	return NewStaticTopology(i.opts), nil
}

func (i staticInitializer) TopologyIsSet() (bool, error) {
	// Always has the specified static topology ready.
	return true, nil
}

type staticTopology struct {
	w xwatch.Watchable
}

// NewStaticTopology creates a static topology.
func NewStaticTopology(opts StaticOptions) Topology {
	w := xwatch.NewWatchable()
	w.Update(NewStaticMap(opts))
	return &staticTopology{w: w}
}

func (t *staticTopology) Get() Map {
	return t.w.Get().(Map)
}

func (t *staticTopology) Watch() (MapWatch, error) {
	// Topology is static, the returned watch will not receive any updates.
	_, w, err := t.w.Watch()
	if err != nil {
		return nil, err
	}
	return NewMapWatch(w), nil
}

func (t *staticTopology) Close() {}
