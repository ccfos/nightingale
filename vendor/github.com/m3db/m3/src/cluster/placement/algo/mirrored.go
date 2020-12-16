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

package algo

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/m3db/m3/src/cluster/placement"
	"github.com/m3db/m3/src/cluster/shard"
)

var (
	errIncompatibleWithMirrorAlgo = errors.New("could not apply mirrored algo on the placement")
)

type mirroredAlgorithm struct {
	opts        placement.Options
	shardedAlgo placement.Algorithm
}

func newMirroredAlgorithm(opts placement.Options) placement.Algorithm {
	return mirroredAlgorithm{
		opts: opts,
		// Mirrored algorithm requires full replacement.
		shardedAlgo: newShardedAlgorithm(opts.SetAllowPartialReplace(false)),
	}
}

func (a mirroredAlgorithm) IsCompatibleWith(p placement.Placement) error {
	if !p.IsMirrored() {
		return errIncompatibleWithMirrorAlgo
	}

	if !p.IsSharded() {
		return errIncompatibleWithMirrorAlgo
	}

	return nil
}

func (a mirroredAlgorithm) InitialPlacement(
	instances []placement.Instance,
	shards []uint32,
	rf int,
) (placement.Placement, error) {
	mirrorInstances, err := groupInstancesByShardSetID(instances, rf)
	if err != nil {
		return nil, err
	}

	// We use the sharded algorithm to generate a mirror placement with rf equals 1.
	mirrorPlacement, err := a.shardedAlgo.InitialPlacement(mirrorInstances, shards, 1)
	if err != nil {
		return nil, err
	}

	return placementFromMirror(mirrorPlacement, instances, rf)
}

func (a mirroredAlgorithm) AddReplica(p placement.Placement) (placement.Placement, error) {
	// TODO(cw): We could support AddReplica(p placement.Placement, instances []placement.Instance)
	// and apply the shards from the new replica to the adding instances in the future.
	return nil, errors.New("not supported")
}

func (a mirroredAlgorithm) RemoveInstances(
	p placement.Placement,
	instanceIDs []string,
) (placement.Placement, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, err
	}

	nowNanos := a.opts.NowFn()().UnixNano()
	// If the instances being removed are all the initializing instances in the placement.
	// We just need to return these shards back to their sources.
	if allInitializing(p, instanceIDs, nowNanos) {
		return a.returnInitializingShards(p, instanceIDs)
	}

	p, _, err := a.MarkAllShardsAvailable(p)
	if err != nil {
		return nil, err
	}

	removingInstances := make([]placement.Instance, 0, len(instanceIDs))
	for _, id := range instanceIDs {
		instance, ok := p.Instance(id)
		if !ok {
			return nil, fmt.Errorf("instance %s does not exist in the placement", id)
		}
		removingInstances = append(removingInstances, instance)
	}

	mirrorPlacement, err := mirrorFromPlacement(p)
	if err != nil {
		return nil, err
	}

	mirrorInstances, err := groupInstancesByShardSetID(removingInstances, p.ReplicaFactor())
	if err != nil {
		return nil, err
	}

	for _, instance := range mirrorInstances {
		if mirrorPlacement, err = a.shardedAlgo.RemoveInstances(
			mirrorPlacement,
			[]string{instance.ID()},
		); err != nil {
			return nil, err
		}
	}
	return placementFromMirror(mirrorPlacement, p.Instances(), p.ReplicaFactor())
}

func (a mirroredAlgorithm) AddInstances(
	p placement.Placement,
	addingInstances []placement.Instance,
) (placement.Placement, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, err
	}

	nowNanos := a.opts.NowFn()().UnixNano()
	// If the instances being added are all the leaving instances in the placement.
	// We just need to get their shards back.
	if allLeaving(p, addingInstances, nowNanos) {
		return a.reclaimLeavingShards(p, addingInstances)
	}

	p, _, err := a.MarkAllShardsAvailable(p)
	if err != nil {
		return nil, err
	}

	// At this point, all leaving instances in the placement are cleaned up.
	if addingInstances, err = validAddingInstances(p, addingInstances); err != nil {
		return nil, err
	}

	mirrorPlacement, err := mirrorFromPlacement(p)
	if err != nil {
		return nil, err
	}

	mirrorInstances, err := groupInstancesByShardSetID(addingInstances, p.ReplicaFactor())
	if err != nil {
		return nil, err
	}

	for _, instance := range mirrorInstances {
		if mirrorPlacement, err = a.shardedAlgo.AddInstances(
			mirrorPlacement,
			[]placement.Instance{instance},
		); err != nil {
			return nil, err
		}
	}

	return placementFromMirror(mirrorPlacement, append(p.Instances(), addingInstances...), p.ReplicaFactor())
}

func (a mirroredAlgorithm) ReplaceInstances(
	p placement.Placement,
	leavingInstanceIDs []string,
	addingInstances []placement.Instance,
) (placement.Placement, error) {
	err := a.IsCompatibleWith(p)
	if err != nil {
		return nil, err
	}

	if len(addingInstances) != len(leavingInstanceIDs) {
		return nil, fmt.Errorf("could not replace %d instances with %d instances for mirrored replace", len(leavingInstanceIDs), len(addingInstances))
	}

	nowNanos := a.opts.NowFn()().UnixNano()
	if allLeaving(p, addingInstances, nowNanos) && allInitializing(p, leavingInstanceIDs, nowNanos) {
		if p, err = a.reclaimLeavingShards(p, addingInstances); err != nil {
			return nil, err
		}

		// NB(cw) I don't think we will ever get here, but just being defensive.
		return a.returnInitializingShards(p, leavingInstanceIDs)
	}

	if p, _, err = a.MarkAllShardsAvailable(p); err != nil {
		return nil, err
	}

	// At this point, all leaving instances in the placement are cleaned up.
	if addingInstances, err = validAddingInstances(p, addingInstances); err != nil {
		return nil, err
	}
	for i := range leavingInstanceIDs {
		// We want full replacement for each instance.
		if p, err = a.shardedAlgo.ReplaceInstances(
			p,
			leavingInstanceIDs[i:i+1],
			addingInstances[i:i+1],
		); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (a mirroredAlgorithm) MarkShardsAvailable(
	p placement.Placement,
	instanceID string,
	shardIDs ...uint32,
) (placement.Placement, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, err
	}

	return a.shardedAlgo.MarkShardsAvailable(p, instanceID, shardIDs...)
}

func (a mirroredAlgorithm) MarkAllShardsAvailable(
	p placement.Placement,
) (placement.Placement, bool, error) {
	if err := a.IsCompatibleWith(p); err != nil {
		return nil, false, err
	}

	return a.shardedAlgo.MarkAllShardsAvailable(p)
}

// allInitializing returns true when
// 1: the given list of instances matches all the initializing instances in the placement.
// 2: the shards are not cutover yet.
func allInitializing(p placement.Placement, instances []string, nowNanos int64) bool {
	ids := make(map[string]struct{}, len(instances))
	for _, i := range instances {
		ids[i] = struct{}{}
	}

	return allInstancesInState(ids, p, func(s shard.Shard) bool {
		return s.State() == shard.Initializing && s.CutoverNanos() > nowNanos
	})
}

// allLeaving returns true when
// 1: the given list of instances matches all the leaving instances in the placement.
// 2: the shards are not cutoff yet.
func allLeaving(p placement.Placement, instances []placement.Instance, nowNanos int64) bool {
	ids := make(map[string]struct{}, len(instances))
	for _, i := range instances {
		ids[i.ID()] = struct{}{}
	}

	return allInstancesInState(ids, p, func(s shard.Shard) bool {
		return s.State() == shard.Leaving && s.CutoffNanos() > nowNanos
	})
}

func instanceCheck(instance placement.Instance, shardCheckFn func(s shard.Shard) bool) bool {
	for _, s := range instance.Shards().All() {
		if !shardCheckFn(s) {
			return false
		}
	}
	return true
}

func allInstancesInState(
	instanceIDs map[string]struct{},
	p placement.Placement,
	forEachShardFn func(s shard.Shard) bool,
) bool {
	for _, instance := range p.Instances() {
		if !instanceCheck(instance, forEachShardFn) {
			continue
		}
		if _, ok := instanceIDs[instance.ID()]; !ok {
			return false
		}
		delete(instanceIDs, instance.ID())
	}

	return len(instanceIDs) == 0
}

// returnInitializingShards tries to return initializing shards on the given instances
// and retries until no more initializing shards could be returned.
func (a mirroredAlgorithm) returnInitializingShards(
	p placement.Placement,
	instanceIDs []string,
) (placement.Placement, error) {
	for {
		madeProgess := false
		for _, id := range instanceIDs {
			_, exist := p.Instance(id)
			if !exist {
				continue
			}
			ph, instance, err := newRemoveInstanceHelper(p, id, a.opts)
			if err != nil {
				return nil, err
			}
			numInitShards := instance.Shards().NumShardsForState(shard.Initializing)
			ph.returnInitializingShards(instance)
			if instance.Shards().NumShardsForState(shard.Initializing) < numInitShards {
				// Made some progress on returning shards.
				madeProgess = true
			}
			p = ph.generatePlacement()
			if instance.Shards().NumShards() > 0 {
				p = p.SetInstances(append(p.Instances(), instance))
			}
		}
		if !madeProgess {
			break
		}
	}

	for _, id := range instanceIDs {
		instance, ok := p.Instance(id)
		if !ok {
			continue
		}
		numInitializingShards := instance.Shards().NumShardsForState(shard.Initializing)
		if numInitializingShards != 0 {
			return nil, fmt.Errorf("there are %d initializing shards could not be returned for instance %s", numInitializingShards, id)
		}
	}

	return p, nil
}

// reclaimLeavingShards tries to reclaim leaving shards on the given instances
// and retries until no more leaving shards could be reclaimed.
func (a mirroredAlgorithm) reclaimLeavingShards(
	p placement.Placement,
	addingInstances []placement.Instance,
) (placement.Placement, error) {
	for {
		madeProgess := false
		for _, instance := range addingInstances {
			ph, instance, err := newAddInstanceHelper(p, instance, a.opts, withAvailableOrLeavingShardsOnly)
			if err != nil {
				return nil, err
			}
			numLeavingShards := instance.Shards().NumShardsForState(shard.Leaving)
			ph.reclaimLeavingShards(instance)
			if instance.Shards().NumShardsForState(shard.Leaving) < numLeavingShards {
				// Made some progress on reclaiming shards.
				madeProgess = true
			}
			p = ph.generatePlacement()
		}
		if !madeProgess {
			break
		}
	}

	for _, instance := range addingInstances {
		id := instance.ID()
		instance, ok := p.Instance(id)
		if !ok {
			return nil, fmt.Errorf("could not find instance %s in placement after reclaiming leaving shards", id)
		}
		numLeavingShards := instance.Shards().NumShardsForState(shard.Leaving)
		if numLeavingShards != 0 {
			return nil, fmt.Errorf("there are %d leaving shards could not be reclaimed for instance %s", numLeavingShards, id)
		}
	}

	return p, nil
}

func validAddingInstances(p placement.Placement, addingInstances []placement.Instance) ([]placement.Instance, error) {
	for i, instance := range addingInstances {
		if _, exist := p.Instance(instance.ID()); exist {
			return nil, fmt.Errorf("instance %s already exist in the placement", instance.ID())
		}
		if instance.IsLeaving() {
			// The instance was leaving in placement, after markAllShardsAsAvailable it is now removed
			// from the placement, so we should treat them as fresh new instances.
			addingInstances[i] = instance.SetShards(shard.NewShards(nil))
		}
	}
	return addingInstances, nil
}

func groupInstancesByShardSetID(
	instances []placement.Instance,
	rf int,
) ([]placement.Instance, error) {
	var (
		shardSetMap = make(map[uint32]*shardSetMetadata, len(instances))
		res         = make([]placement.Instance, 0, len(instances))
	)
	for _, instance := range instances {
		var (
			ssID   = instance.ShardSetID()
			weight = instance.Weight()
			group  = instance.IsolationGroup()
			shards = instance.Shards()
		)
		meta, ok := shardSetMap[ssID]
		if !ok {
			meta = &shardSetMetadata{
				weight: weight,
				groups: make(map[string]struct{}, rf),
				shards: shards,
			}
			shardSetMap[ssID] = meta
		}
		if _, ok := meta.groups[group]; ok {
			return nil, fmt.Errorf("found duplicated isolation group %s for shardset id %d", group, ssID)
		}

		if meta.weight != weight {
			return nil, fmt.Errorf("found different weights: %d and %d, for shardset id %d", meta.weight, weight, ssID)
		}

		if !meta.shards.Equals(shards) {
			return nil, fmt.Errorf("found different shards: %v and %v, for shardset id %d", meta.shards, shards, ssID)
		}

		meta.groups[group] = struct{}{}
		meta.count++
	}

	for ssID, meta := range shardSetMap {
		if meta.count != rf {
			return nil, fmt.Errorf("found %d count of shard set id %d, expecting %d", meta.count, ssID, rf)
		}

		// NB(cw) The shard set ID should to be assigned in placement service,
		// the algorithm does not change the shard set id assigned to each instance.
		ssIDStr := strconv.Itoa(int(ssID))
		res = append(
			res,
			placement.NewInstance().
				SetID(ssIDStr).
				SetIsolationGroup(ssIDStr).
				SetWeight(meta.weight).
				SetShardSetID(ssID).
				SetShards(meta.shards.Clone()),
		)
	}

	return res, nil
}

// mirrorFromPlacement zips all instances with the same shardSetID into a virtual instance
// and create a placement with those virtual instance and rf=1.
func mirrorFromPlacement(p placement.Placement) (placement.Placement, error) {
	mirrorInstances, err := groupInstancesByShardSetID(p.Instances(), p.ReplicaFactor())
	if err != nil {
		return nil, err
	}

	return placement.NewPlacement().
		SetInstances(mirrorInstances).
		SetReplicaFactor(1).
		SetShards(p.Shards()).
		SetCutoverNanos(p.CutoverNanos()).
		SetIsSharded(true).
		SetIsMirrored(true).
		SetMaxShardSetID(p.MaxShardSetID()), nil
}

// placementFromMirror duplicates the shards for each shard set id and assign
// them to the instance with the shard set id.
func placementFromMirror(
	mirror placement.Placement,
	instances []placement.Instance,
	rf int,
) (placement.Placement, error) {
	var (
		mirrorInstances     = mirror.Instances()
		shardSetMap         = make(map[uint32][]placement.Instance, len(mirrorInstances))
		instancesWithShards = make([]placement.Instance, 0, len(instances))
	)
	for _, instance := range instances {
		instances, ok := shardSetMap[instance.ShardSetID()]
		if !ok {
			instances = make([]placement.Instance, 0, rf)
		}
		instances = append(instances, instance)
		shardSetMap[instance.ShardSetID()] = instances
	}

	for _, mirrorInstance := range mirrorInstances {
		instances, err := instancesFromMirror(mirrorInstance, shardSetMap)
		if err != nil {
			return nil, err
		}
		instancesWithShards = append(instancesWithShards, instances...)
	}

	return placement.NewPlacement().
		SetInstances(instancesWithShards).
		SetReplicaFactor(rf).
		SetShards(mirror.Shards()).
		SetCutoverNanos(mirror.CutoverNanos()).
		SetIsMirrored(true).
		SetIsSharded(true).
		SetMaxShardSetID(mirror.MaxShardSetID()), nil
}

func instancesFromMirror(
	mirrorInstance placement.Instance,
	instancesMap map[uint32][]placement.Instance,
) ([]placement.Instance, error) {
	ssID := mirrorInstance.ShardSetID()
	instances, ok := instancesMap[ssID]
	if !ok {
		return nil, fmt.Errorf("could not find shard set id %d in placement", ssID)
	}

	shards := mirrorInstance.Shards()
	for i, instance := range instances {
		newShards := make([]shard.Shard, shards.NumShards())
		for j, s := range shards.All() {
			// TODO move clone() to shard interface
			newShard := shard.NewShard(s.ID()).SetState(s.State()).SetCutoffNanos(s.CutoffNanos()).SetCutoverNanos(s.CutoverNanos())
			sourceID := s.SourceID()
			if sourceID != "" {
				// The sourceID in the mirror placement is shardSetID, need to be converted
				// to instanceID.
				shardSetID, err := strconv.Atoi(sourceID)
				if err != nil {
					return nil, fmt.Errorf("could not convert source id %s to shard set id", sourceID)
				}
				sourceInstances, ok := instancesMap[uint32(shardSetID)]
				if !ok {
					return nil, fmt.Errorf("could not find source id %s in placement", sourceID)
				}

				sourceID = sourceInstances[i].ID()
			}
			newShards[j] = newShard.SetSourceID(sourceID)
		}
		instances[i] = instance.SetShards(shard.NewShards(newShards))
	}
	return instances, nil
}

type shardSetMetadata struct {
	weight uint32
	count  int
	groups map[string]struct{}
	shards shard.Shards
}
