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

package service

import (
	"fmt"

	"github.com/m3db/m3/src/cluster/placement"
	"github.com/m3db/m3/src/cluster/placement/algo"
	"github.com/m3db/m3/src/cluster/placement/selector"
	"github.com/m3db/m3/src/cluster/shard"

	"go.uber.org/zap"
)

type placementService struct {
	placement.Storage
	*placementServiceImpl
}

// NewPlacementService returns an instance of placement service.
func NewPlacementService(s placement.Storage, opts placement.Options) placement.Service {
	return &placementService{
		Storage: s,
		placementServiceImpl: newPlacementServiceImpl(
			opts,
			s,

		),
	}
}

func newPlacementServiceImpl(
	opts placement.Options,
	storage minimalPlacementStorage,
) *placementServiceImpl {
	if opts == nil {
		opts = placement.NewOptions()
	}

	instanceSelector := opts.InstanceSelector()
	if instanceSelector == nil {
		instanceSelector = selector.NewInstanceSelector(opts)
	}

	return &placementServiceImpl{
		store:    storage,
		opts:     opts,
		algo:     algo.NewAlgorithm(opts),
		selector: instanceSelector,
		logger:   opts.InstrumentOptions().Logger(),
	}
}

// minimalPlacementStorage is the subset of the placement.Storage interface used by placement.Service
// directly.
type minimalPlacementStorage interface {

	// Set writes a placement.
	Set(p placement.Placement) (placement.Placement, error)

	// CheckAndSet writes a placement.Placement if the current version
	// matches the expected version.
	CheckAndSet(p placement.Placement, version int) (placement.Placement, error)

	// SetIfNotExist writes a placement.Placement.
	SetIfNotExist(p placement.Placement) (placement.Placement, error)

	// Placement reads placement.Placement.
	Placement() (placement.Placement, error)
}

// type assertion
var _ minimalPlacementStorage = placement.Storage(nil)

type placementServiceImpl struct {
	store minimalPlacementStorage

	opts     placement.Options
	algo     placement.Algorithm
	selector placement.InstanceSelector
	logger   *zap.Logger
}

func (ps *placementServiceImpl) BuildInitialPlacement(
	candidates []placement.Instance,
	numShards int,
	rf int,
) (placement.Placement, error) {
	if numShards < 0 {
		return nil, fmt.Errorf("could not build initial placement, invalid numShards %d", numShards)
	}

	if rf <= 0 {
		return nil, fmt.Errorf("could not build initial placement, invalid replica factor %d", rf)
	}

	instances, err := ps.selector.SelectInitialInstances(candidates, rf)
	if err != nil {
		return nil, err
	}

	ids := make([]uint32, numShards)
	for i := 0; i < numShards; i++ {
		ids[i] = uint32(i)
	}

	tempPlacement, err := ps.algo.InitialPlacement(instances, ids, rf)
	if err != nil {
		return nil, err
	}

	if err := placement.Validate(tempPlacement); err != nil {
		return nil, err
	}

	return ps.store.SetIfNotExist(tempPlacement)
}

func (ps *placementServiceImpl) AddReplica() (placement.Placement, error) {
	curPlacement, err := ps.store.Placement()
	if err != nil {
		return nil, err
	}

	if err := ps.opts.ValidateFnBeforeUpdate()(curPlacement); err != nil {
		return nil, err
	}

	tempPlacement, err := ps.algo.AddReplica(curPlacement)
	if err != nil {
		return nil, err
	}

	if err := placement.Validate(tempPlacement); err != nil {
		return nil, err
	}

	return ps.store.CheckAndSet(tempPlacement, curPlacement.Version())
}

func (ps *placementServiceImpl) AddInstances(
	candidates []placement.Instance,
) (placement.Placement, []placement.Instance, error) {
	curPlacement, err := ps.store.Placement()
	if err != nil {
		return nil, nil, err
	}

	if err := ps.opts.ValidateFnBeforeUpdate()(curPlacement); err != nil {
		return nil, nil, err
	}

	addingInstances, err := ps.selector.SelectAddingInstances(candidates, curPlacement)
	if err != nil {
		return nil, nil, err
	}

	tempPlacement, err := ps.algo.AddInstances(curPlacement, addingInstances)
	if err != nil {
		return nil, nil, err
	}

	if err := placement.Validate(tempPlacement); err != nil {
		return nil, nil, err
	}

	for i, instance := range addingInstances {
		addingInstance, ok := tempPlacement.Instance(instance.ID())
		if !ok {
			return nil, nil, fmt.Errorf("unable to find added instance [%s] in new placement", instance.ID())
		}
		addingInstances[i] = addingInstance
	}

	newPlacement, err := ps.store.CheckAndSet(tempPlacement, curPlacement.Version())
	if err != nil {
		return nil, nil, err
	}
	return newPlacement, addingInstances, nil
}

func (ps *placementServiceImpl) RemoveInstances(instanceIDs []string) (placement.Placement, error) {
	curPlacement, err := ps.store.Placement()
	if err != nil {
		return nil, err
	}

	if err := ps.opts.ValidateFnBeforeUpdate()(curPlacement); err != nil {
		return nil, err
	}

	tempPlacement, err := ps.algo.RemoveInstances(curPlacement, instanceIDs)
	if err != nil {
		return nil, err
	}

	if err := placement.Validate(tempPlacement); err != nil {
		return nil, err
	}

	return ps.store.CheckAndSet(tempPlacement, curPlacement.Version())
}

func (ps *placementServiceImpl) ReplaceInstances(
	leavingInstanceIDs []string,
	candidates []placement.Instance,
) (placement.Placement, []placement.Instance, error) {
	curPlacement, err := ps.store.Placement()
	if err != nil {
		return nil, nil, err
	}

	if err := ps.opts.ValidateFnBeforeUpdate()(curPlacement); err != nil {
		return nil, nil, err
	}

	addingInstances, err := ps.selector.SelectReplaceInstances(candidates, leavingInstanceIDs, curPlacement)
	if err != nil {
		return nil, nil, err
	}

	tempPlacement, err := ps.algo.ReplaceInstances(curPlacement, leavingInstanceIDs, addingInstances)
	if err != nil {
		return nil, nil, err
	}

	if err := placement.Validate(tempPlacement); err != nil {
		return nil, nil, err
	}

	addedInstances := make([]placement.Instance, 0, len(addingInstances))
	for _, inst := range addingInstances {
		addedInstance, ok := tempPlacement.Instance(inst.ID())
		if !ok {
			return nil, nil, fmt.Errorf("unable to find added instance [%+v] in new placement [%+v]", inst, curPlacement)
		}
		addedInstances = append(addedInstances, addedInstance)
	}

	newPlacement, err := ps.store.CheckAndSet(tempPlacement, curPlacement.Version())
	if err != nil {
		return nil, nil, err
	}
	return newPlacement, addedInstances, nil
}

func (ps *placementServiceImpl) MarkShardsAvailable(instanceID string, shardIDs ...uint32) (placement.Placement, error) {
	curPlacement, err := ps.store.Placement()
	if err != nil {
		return nil, err
	}

	if err := ps.opts.ValidateFnBeforeUpdate()(curPlacement); err != nil {
		return nil, err
	}

	tempPlacement, err := ps.algo.MarkShardsAvailable(curPlacement, instanceID, shardIDs...)
	if err != nil {
		return nil, err
	}

	if err := placement.Validate(tempPlacement); err != nil {
		return nil, err
	}

	return ps.store.CheckAndSet(tempPlacement, curPlacement.Version())
}

func (ps *placementServiceImpl) MarkInstanceAvailable(instanceID string) (placement.Placement, error) {
	curPlacement, err := ps.store.Placement()
	if err != nil {
		return nil, err
	}

	if err := ps.opts.ValidateFnBeforeUpdate()(curPlacement); err != nil {
		return nil, err
	}

	instance, exist := curPlacement.Instance(instanceID)
	if !exist {
		return nil, fmt.Errorf("could not find instance %s in placement", instanceID)
	}

	shards := instance.Shards().ShardsForState(shard.Initializing)
	shardIDs := make([]uint32, len(shards))
	for i, s := range shards {
		shardIDs[i] = s.ID()
	}

	tempPlacement, err := ps.algo.MarkShardsAvailable(curPlacement, instanceID, shardIDs...)
	if err != nil {
		return nil, err
	}

	if err := placement.Validate(tempPlacement); err != nil {
		return nil, err
	}

	return ps.store.CheckAndSet(tempPlacement, curPlacement.Version())
}

func (ps *placementServiceImpl) MarkAllShardsAvailable() (placement.Placement, error) {
	curPlacement, err := ps.store.Placement()
	if err != nil {
		return nil, err
	}

	if err := ps.opts.ValidateFnBeforeUpdate()(curPlacement); err != nil {
		return nil, err
	}

	tempPlacement, updated, err := ps.algo.MarkAllShardsAvailable(curPlacement)
	if err != nil {
		return nil, err
	}
	if !updated {
		return curPlacement, nil
	}

	if err := placement.Validate(tempPlacement); err != nil {
		return nil, err
	}

	return ps.store.CheckAndSet(tempPlacement, curPlacement.Version())
}
