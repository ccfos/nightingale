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

package storage

import (
	"errors"

	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/cluster/placement"
)

var (
	errPlacementNotAvailable            = errors.New("placement is not available")
	errStagedPlacementNoActivePlacement = errors.New("staged placement with no active placement")
)

type w struct {
	kv.ValueWatch
	opts placement.Options
}

func newPlacementWatch(vw kv.ValueWatch, opts placement.Options) placement.Watch {
	return &w{ValueWatch: vw, opts: opts}
}

func (w *w) Get() (placement.Placement, error) {
	v := w.ValueWatch.Get()
	if v == nil {
		return nil, errPlacementNotAvailable
	}

	if w.opts.IsStaged() {
		p, err := placementsFromValue(v)
		if err != nil {
			return nil, err
		}
		if len(p) == 0 {
			return nil, errStagedPlacementNoActivePlacement
		}

		return p[len(p)-1], nil
	}

	return placementFromValue(v)
}
