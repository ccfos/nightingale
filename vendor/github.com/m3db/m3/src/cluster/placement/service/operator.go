// Copyright (c) 2020 Uber Technologies, Inc.
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
//

package service

import (
	"errors"

	"github.com/m3db/m3/src/cluster/placement"
)

// NewPlacementOperator constructs a placement.Operator which performs transformations on the
// given placement.
// If initialPlacement is nil, BuildInitialPlacement must be called before any operations on the
// placement.
func NewPlacementOperator(initialPlacement placement.Placement, opts placement.Options) placement.Operator {
	store := newDummyStore(initialPlacement)
	return &placementOperator{
		placementServiceImpl: newPlacementServiceImpl(opts, store),
		store:                store,
	}
}

// placementOperator is implemented by a placementServiceImpl backed by a dummyStore, which just
// sets in memory state and doesn't touch versions.
type placementOperator struct {
	*placementServiceImpl
	store *dummyStore
}

func (p *placementOperator) Placement() placement.Placement {
	return p.store.curPlacement
}

// dummyStore is a helper class for placementOperator. It stores a single placement in memory,
// allowing us to use the same code to implement the actual placement.Service (which typically talks
// to a fully fledged backing store) and placement.Operator, which only operates on memory.
// Unlike proper placement.Storage implementations, all operations are unversioned;
// version arguments are ignored, and the store never calls Placement.SetVersion. This makes it
// distinct from e.g. the implementation in mem.NewStore.
type dummyStore struct {
	curPlacement placement.Placement
}

func newDummyStore(initialPlacement placement.Placement) *dummyStore {
	return &dummyStore{curPlacement: initialPlacement}
}

func (d *dummyStore) Set(p placement.Placement) (placement.Placement, error) {
	d.set(p)
	return d.curPlacement, nil
}

func (d *dummyStore) set(p placement.Placement) {
	d.curPlacement = p
}

// CheckAndSet on the dummy store is unconditional (no check).
func (d *dummyStore) CheckAndSet(p placement.Placement, _ int) (placement.Placement, error) {
	d.curPlacement = p
	return d.curPlacement, nil
}

func (d *dummyStore) SetIfNotExist(p placement.Placement) (placement.Placement, error) {
	if d.curPlacement != nil {
		return nil, errors.New(
			"placement already exists and can't be rebuilt. Construct a new placement.Operator",
		)
	}
	d.curPlacement = p
	return d.curPlacement, nil
}

func (d *dummyStore) Placement() (placement.Placement, error) {
	if d.curPlacement == nil {
		return nil, errors.New(
			"no initial placement specified at operator construction; call BuildInitialPlacement or pass one in",
		)
	}
	return d.curPlacement, nil
}

