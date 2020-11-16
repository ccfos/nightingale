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

package algo

import (
	"errors"
	"fmt"

	"github.com/m3db/m3/src/cluster/placement"
)

var (
	errNotEnoughIsolationGroups    = errors.New("not enough isolation groups to take shards, please make sure RF is less than number of isolation groups")
	errIncompatibleWithShardedAlgo = errors.New("could not apply sharded algo on the placement")
)

type shardedPlacementAlgorithm struct {
	opts placement.Options
}

func newShardedAlgorithm(opts placement.Options) placement.Algorithm {
	return shardedPlacementAlgorithm{opts: opts}
}

func (a shardedPlacementAlgorithm) IsCompatibleWith(p placement.Placement) error {
	if !p.IsSharded() {
		return errIncompatibleWithShardedAlgo
	}

	return nil
}

func (a shardedPlacementAlgorithm) InitialPlacement(
	instances []placement.Instance,
	shards []uint32,
	rf int,
) (placement.Placement, error) {
	ph := newInitHelper(placement.Instances(instances).Clone(), shards, a.opts)
	if err := ph.placeShards(newShards(shards), nil, ph.Instances()); err != nil {
		return nil, err
	}

	var (
		p   = ph.generatePlacement()
		err error
	)
	for i := 1; i < rf; i++ {
		p, err = a.AddReplica(p)
		if err != nil {
			return nil, err
		}
	}

	return tryCleanupShardState(p, a.opts)
}

func (a shardedPlacementAlgorithm) AddReplica(p placement.Placement) (placement.Placement, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, err
	}

	p = p.Clone()
	ph := newAddReplicaHelper(p, a.opts)
	if err := ph.placeShards(newShards(p.Shards()), nil, nonLeavingInstances(ph.Instances())); err != nil {
		return nil, err
	}

	if err := ph.optimize(safe); err != nil {
		return nil, err
	}

	return tryCleanupShardState(ph.generatePlacement(), a.opts)
}

func (a shardedPlacementAlgorithm) RemoveInstances(
	p placement.Placement,
	instanceIDs []string,
) (placement.Placement, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, err
	}

	p = p.Clone()
	for _, instanceID := range instanceIDs {
		ph, leavingInstance, err := newRemoveInstanceHelper(p, instanceID, a.opts)
		if err != nil {
			return nil, err
		}
		// place the shards from the leaving instance to the rest of the cluster
		if err := ph.placeShards(leavingInstance.Shards().All(), leavingInstance, ph.Instances()); err != nil {
			return nil, err
		}

		if err := ph.optimize(safe); err != nil {
			return nil, err
		}

		if p, _, err = addInstanceToPlacement(ph.generatePlacement(), leavingInstance, withShards); err != nil {
			return nil, err
		}
	}
	return tryCleanupShardState(p, a.opts)
}

func (a shardedPlacementAlgorithm) AddInstances(
	p placement.Placement,
	instances []placement.Instance,
) (placement.Placement, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, err
	}

	p = p.Clone()
	for _, instance := range instances {
		ph, addingInstance, err := newAddInstanceHelper(p, instance, a.opts, withLeavingShardsOnly)
		if err != nil {
			return nil, err
		}

		if err := ph.addInstance(addingInstance); err != nil {
			return nil, err
		}

		p = ph.generatePlacement()
	}

	return tryCleanupShardState(p, a.opts)
}

func (a shardedPlacementAlgorithm) ReplaceInstances(
	p placement.Placement,
	leavingInstanceIDs []string,
	addingInstances []placement.Instance,
) (placement.Placement, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, err
	}

	p = p.Clone()
	ph, leavingInstances, addingInstances, err := newReplaceInstanceHelper(p, leavingInstanceIDs, addingInstances, a.opts)
	if err != nil {
		return nil, err
	}

	for _, leavingInstance := range leavingInstances {
		err = ph.placeShards(leavingInstance.Shards().All(), leavingInstance, addingInstances)
		if err != nil && err != errNotEnoughIsolationGroups {
			// errNotEnoughIsolationGroups means the adding instances do not
			// have enough isolation groups to take all the shards, but the rest
			// instances might have more isolation groups to take all the shards.
			return nil, err
		}
		load := loadOnInstance(leavingInstance)
		if load != 0 && !a.opts.AllowPartialReplace() {
			return nil, fmt.Errorf("could not fully replace all shards from %s, %d shards left unassigned",
				leavingInstance.ID(), load)
		}
	}

	if a.opts.AllowPartialReplace() {
		// Place the shards left on the leaving instance to the rest of the cluster.
		for _, leavingInstance := range leavingInstances {
			if err = ph.placeShards(leavingInstance.Shards().All(), leavingInstance, ph.Instances()); err != nil {
				return nil, err
			}
		}

		if err := ph.optimize(unsafe); err != nil {
			return nil, err
		}
	}

	p = ph.generatePlacement()
	for _, leavingInstance := range leavingInstances {
		if p, _, err = addInstanceToPlacement(p, leavingInstance, withShards); err != nil {
			return nil, err
		}
	}
	return tryCleanupShardState(p, a.opts)
}

func (a shardedPlacementAlgorithm) MarkShardsAvailable(
	p placement.Placement,
	instanceID string,
	shardIDs ...uint32,
) (placement.Placement, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, err
	}

	return markShardsAvailable(p.Clone(), instanceID, shardIDs, a.opts)
}

func (a shardedPlacementAlgorithm) MarkAllShardsAvailable(
	p placement.Placement,
) (placement.Placement, bool, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, false, err
	}

	return markAllShardsAvailable(p, a.opts)
}
