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
	"container/heap"
	"errors"
	"fmt"
	"math"

	"github.com/m3db/m3/src/cluster/placement"
	"github.com/m3db/m3/src/cluster/shard"

	"go.uber.org/zap"
)

var (
	errAddingInstanceAlreadyExist         = errors.New("the adding instance is already in the placement")
	errInstanceContainsNonLeavingShards   = errors.New("the adding instance contains non leaving shards")
	errInstanceContainsInitializingShards = errors.New("the adding instance contains initializing shards")
)

type instanceType int

const (
	anyType instanceType = iota
	withShards
	withLeavingShardsOnly
	withAvailableOrLeavingShardsOnly
)

type optimizeType int

const (
	// safe optimizes the load distribution without violating
	// minimal shard movemoment.
	safe optimizeType = iota
	// unsafe optimizes the load distribution with the potential of violating
	// minimal shard movement in order to reach best shard distribution
	unsafe
)

type assignLoadFn func(instance placement.Instance) error

type placementHelper interface {
	PlacementHelper

	// placeShards distributes shards to the instances in the helper, with aware of where are the shards coming from.
	placeShards(shards []shard.Shard, from placement.Instance, candidates []placement.Instance) error

	// addInstance adds an instance to the placement.
	addInstance(addingInstance placement.Instance) error

	// optimize rebalances the load distribution in the cluster.
	optimize(t optimizeType) error

	// generatePlacement generates a placement.
	generatePlacement() placement.Placement

	// reclaimLeavingShards reclaims all the leaving shards on the given instance
	// by pulling them back from the rest of the cluster.
	reclaimLeavingShards(instance placement.Instance)

	// returnInitializingShards returns all the initializing shards on the given instance
	// by returning them back to the original owners.
	returnInitializingShards(instance placement.Instance)
}

// PlacementHelper helps the algorithm to place shards.
type PlacementHelper interface {
	// Instances returns the list of instances managed by the PlacementHelper.
	Instances() []placement.Instance

	// CanMoveShard checks if the shard can be moved from the instance to the target isolation group.
	CanMoveShard(shard uint32, fromInstance placement.Instance, toIsolationGroup string) bool
}

type helper struct {
	targetLoad          map[string]int
	shardToInstanceMap  map[uint32]map[placement.Instance]struct{}
	groupToInstancesMap map[string]map[placement.Instance]struct{}
	groupToWeightMap    map[string]uint32
	rf                  int
	uniqueShards        []uint32
	instances           map[string]placement.Instance
	log                 *zap.Logger
	opts                placement.Options
	totalWeight         uint32
	maxShardSetID       uint32
}

// NewPlacementHelper returns a placement helper
func NewPlacementHelper(p placement.Placement, opts placement.Options) PlacementHelper {
	return newHelper(p, p.ReplicaFactor(), opts)
}

func newInitHelper(instances []placement.Instance, ids []uint32, opts placement.Options) placementHelper {
	emptyPlacement := placement.NewPlacement().
		SetInstances(instances).
		SetShards(ids).
		SetReplicaFactor(0).
		SetIsSharded(true).
		SetCutoverNanos(opts.PlacementCutoverNanosFn()())
	return newHelper(emptyPlacement, emptyPlacement.ReplicaFactor()+1, opts)
}

func newAddReplicaHelper(p placement.Placement, opts placement.Options) placementHelper {
	return newHelper(p, p.ReplicaFactor()+1, opts)
}

func newAddInstanceHelper(
	p placement.Placement,
	instance placement.Instance,
	opts placement.Options,
	t instanceType,
) (placementHelper, placement.Instance, error) {
	instanceInPlacement, exist := p.Instance(instance.ID())
	if !exist {
		return newHelper(p.SetInstances(append(p.Instances(), instance)), p.ReplicaFactor(), opts), instance, nil
	}

	switch t {
	case withLeavingShardsOnly:
		if !instanceInPlacement.IsLeaving() {
			return nil, nil, errInstanceContainsNonLeavingShards
		}
	case withAvailableOrLeavingShardsOnly:
		shards := instanceInPlacement.Shards()
		if shards.NumShards() != shards.NumShardsForState(shard.Available)+shards.NumShardsForState(shard.Leaving) {
			return nil, nil, errInstanceContainsInitializingShards
		}
	default:
		return nil, nil, fmt.Errorf("unexpected type %v", t)
	}

	return newHelper(p, p.ReplicaFactor(), opts), instanceInPlacement, nil
}

func newRemoveInstanceHelper(
	p placement.Placement,
	instanceID string,
	opts placement.Options,
) (placementHelper, placement.Instance, error) {
	p, leavingInstance, err := removeInstanceFromPlacement(p, instanceID)
	if err != nil {
		return nil, nil, err
	}
	return newHelper(p, p.ReplicaFactor(), opts), leavingInstance, nil
}

func newReplaceInstanceHelper(
	p placement.Placement,
	instanceIDs []string,
	addingInstances []placement.Instance,
	opts placement.Options,
) (placementHelper, []placement.Instance, []placement.Instance, error) {
	var (
		leavingInstances = make([]placement.Instance, len(instanceIDs))
		err              error
	)
	for i, instanceID := range instanceIDs {
		p, leavingInstances[i], err = removeInstanceFromPlacement(p, instanceID)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	newAddingInstances := make([]placement.Instance, len(addingInstances))
	for i, instance := range addingInstances {
		p, newAddingInstances[i], err = addInstanceToPlacement(p, instance, anyType)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return newHelper(p, p.ReplicaFactor(), opts), leavingInstances, newAddingInstances, nil
}

func newHelper(p placement.Placement, targetRF int, opts placement.Options) placementHelper {
	ph := &helper{
		rf:            targetRF,
		instances:     make(map[string]placement.Instance, p.NumInstances()),
		uniqueShards:  p.Shards(),
		maxShardSetID: p.MaxShardSetID(),
		log:           opts.InstrumentOptions().Logger(),
		opts:          opts,
	}

	for _, instance := range p.Instances() {
		ph.instances[instance.ID()] = instance
	}

	ph.scanCurrentLoad()
	ph.buildTargetLoad()
	return ph
}

func (ph *helper) scanCurrentLoad() {
	ph.shardToInstanceMap = make(map[uint32]map[placement.Instance]struct{}, len(ph.uniqueShards))
	ph.groupToInstancesMap = make(map[string]map[placement.Instance]struct{})
	ph.groupToWeightMap = make(map[string]uint32)
	totalWeight := uint32(0)
	for _, instance := range ph.instances {
		if _, exist := ph.groupToInstancesMap[instance.IsolationGroup()]; !exist {
			ph.groupToInstancesMap[instance.IsolationGroup()] = make(map[placement.Instance]struct{})
		}
		ph.groupToInstancesMap[instance.IsolationGroup()][instance] = struct{}{}

		if instance.IsLeaving() {
			// Leaving instances are not counted as usable capacities in the placement.
			continue
		}

		ph.groupToWeightMap[instance.IsolationGroup()] = ph.groupToWeightMap[instance.IsolationGroup()] + instance.Weight()
		totalWeight += instance.Weight()

		for _, s := range instance.Shards().All() {
			if s.State() == shard.Leaving {
				continue
			}
			ph.assignShardToInstance(s, instance)
		}
	}
	ph.totalWeight = totalWeight
}

func (ph *helper) buildTargetLoad() {
	overWeightedGroups := 0
	overWeight := uint32(0)
	for _, weight := range ph.groupToWeightMap {
		if isOverWeighted(weight, ph.totalWeight, ph.rf) {
			overWeightedGroups++
			overWeight += weight
		}
	}

	targetLoad := make(map[string]int, len(ph.instances))
	for _, instance := range ph.instances {
		if instance.IsLeaving() {
			// We should not set a target load for leaving instances.
			continue
		}
		igWeight := ph.groupToWeightMap[instance.IsolationGroup()]
		if isOverWeighted(igWeight, ph.totalWeight, ph.rf) {
			// If the instance is on a over-sized isolation group, the target load
			// equals (shardLen / capacity of the isolation group).
			targetLoad[instance.ID()] = int(math.Ceil(float64(ph.getShardLen()) * float64(instance.Weight()) / float64(igWeight)))
		} else {
			// If the instance is on a normal isolation group, get the target load
			// with aware of other over-sized isolation group.
			targetLoad[instance.ID()] = ph.getShardLen() * (ph.rf - overWeightedGroups) * int(instance.Weight()) / int(ph.totalWeight-overWeight)
		}
	}
	ph.targetLoad = targetLoad
}

func (ph *helper) Instances() []placement.Instance {
	res := make([]placement.Instance, 0, len(ph.instances))
	for _, instance := range ph.instances {
		res = append(res, instance)
	}
	return res
}

func (ph *helper) getShardLen() int {
	return len(ph.uniqueShards)
}

func (ph *helper) targetLoadForInstance(id string) int {
	return ph.targetLoad[id]
}

func (ph *helper) moveOneShard(from, to placement.Instance) bool {
	// The order matter here:
	// The Unknown shards were just moved, so free to be moved around.
	// The Initializing shards were still being initialized on the instance,
	// so moving them are cheaper than moving those Available shards.
	return ph.moveOneShardInState(from, to, shard.Unknown) ||
		ph.moveOneShardInState(from, to, shard.Initializing) ||
		ph.moveOneShardInState(from, to, shard.Available)
}

// nolint: unparam
func (ph *helper) moveOneShardInState(from, to placement.Instance, state shard.State) bool {
	for _, s := range from.Shards().ShardsForState(state) {
		if ph.moveShard(s, from, to) {
			return true
		}
	}
	return false
}

func (ph *helper) moveShard(candidateShard shard.Shard, from, to placement.Instance) bool {
	shardID := candidateShard.ID()
	if !ph.canAssignInstance(shardID, from, to) {
		return false
	}

	if candidateShard.State() == shard.Leaving {
		// should not move a Leaving shard,
		// Leaving shard will be removed when the Initializing shard is marked as Available
		return false
	}

	newShard := shard.NewShard(shardID)

	if from != nil {
		switch candidateShard.State() {
		case shard.Unknown, shard.Initializing:
			from.Shards().Remove(shardID)
			newShard.SetSourceID(candidateShard.SourceID())
		case shard.Available:
			candidateShard.
				SetState(shard.Leaving).
				SetCutoffNanos(ph.opts.ShardCutoffNanosFn()())
			newShard.SetSourceID(from.ID())
		}

		delete(ph.shardToInstanceMap[shardID], from)
	}

	curShard, ok := to.Shards().Shard(shardID)
	if ok && curShard.State() == shard.Leaving {
		// NB(cw): if the instance already owns the shard in Leaving state,
		// simply mark it as Available
		newShard = shard.NewShard(shardID).SetState(shard.Available)
		// NB(cw): Break the link between new owner of this shard with this Leaving instance
		instances := ph.shardToInstanceMap[shardID]
		for instance := range instances {
			shards := instance.Shards()
			initShard, ok := shards.Shard(shardID)
			if ok && initShard.SourceID() == to.ID() {
				initShard.SetSourceID("")
			}
		}

	}

	ph.assignShardToInstance(newShard, to)
	return true
}

func (ph *helper) CanMoveShard(shard uint32, from placement.Instance, toIsolationGroup string) bool {
	if from != nil {
		if from.IsolationGroup() == toIsolationGroup {
			return true
		}
	}
	for instance := range ph.shardToInstanceMap[shard] {
		if instance.IsolationGroup() == toIsolationGroup {
			return false
		}
	}
	return true
}

func (ph *helper) buildInstanceHeap(instances []placement.Instance, availableCapacityAscending bool) (heap.Interface, error) {
	return newHeap(instances, availableCapacityAscending, ph.targetLoad, ph.groupToWeightMap)
}

func (ph *helper) generatePlacement() placement.Placement {
	var instances = make([]placement.Instance, 0, len(ph.instances))

	for _, instance := range ph.instances {
		if instance.Shards().NumShards() > 0 {
			instances = append(instances, instance)
		}
	}

	maxShardSetID := ph.maxShardSetID
	for _, instance := range instances {
		shards := instance.Shards()
		for _, s := range shards.ShardsForState(shard.Unknown) {
			shards.Add(shard.NewShard(s.ID()).
				SetSourceID(s.SourceID()).
				SetState(shard.Initializing).
				SetCutoverNanos(ph.opts.ShardCutoverNanosFn()()))
		}
		if shardSetID := instance.ShardSetID(); shardSetID >= maxShardSetID {
			maxShardSetID = shardSetID
		}
	}

	return placement.NewPlacement().
		SetInstances(instances).
		SetShards(ph.uniqueShards).
		SetReplicaFactor(ph.rf).
		SetIsSharded(true).
		SetIsMirrored(ph.opts.IsMirrored()).
		SetCutoverNanos(ph.opts.PlacementCutoverNanosFn()()).
		SetMaxShardSetID(maxShardSetID)
}

func (ph *helper) placeShards(
	shards []shard.Shard,
	from placement.Instance,
	candidates []placement.Instance,
) error {
	shardSet := getShardMap(shards)
	if from != nil {
		// NB(cw) when removing an adding instance that has not finished bootstrapping its
		// Initializing shards, prefer to return those Initializing shards back to the leaving instance
		// to reduce some bootstrapping work in the cluster.
		ph.returnInitializingShardsToSource(shardSet, from, candidates)
	}

	instanceHeap, err := ph.buildInstanceHeap(nonLeavingInstances(candidates), true)
	if err != nil {
		return err
	}
	// if there are shards left to be assigned, distribute them evenly
	var triedInstances []placement.Instance
	for _, s := range shardSet {
		if s.State() == shard.Leaving {
			continue
		}
		moved := false
		for instanceHeap.Len() > 0 {
			tryInstance := heap.Pop(instanceHeap).(placement.Instance)
			triedInstances = append(triedInstances, tryInstance)
			if ph.moveShard(s, from, tryInstance) {
				moved = true
				break
			}
		}
		if !moved {
			// This should only happen when RF > number of isolation groups.
			return errNotEnoughIsolationGroups
		}
		for _, triedInstance := range triedInstances {
			heap.Push(instanceHeap, triedInstance)
		}
		triedInstances = triedInstances[:0]
	}
	return nil
}

func (ph *helper) returnInitializingShards(instance placement.Instance) {
	shardSet := getShardMap(instance.Shards().All())
	ph.returnInitializingShardsToSource(shardSet, instance, ph.Instances())
}

func (ph *helper) returnInitializingShardsToSource(
	shardSet map[uint32]shard.Shard,
	from placement.Instance,
	candidates []placement.Instance,
) {
	candidateMap := make(map[string]placement.Instance, len(candidates))
	for _, candidate := range candidates {
		candidateMap[candidate.ID()] = candidate
	}
	for _, s := range shardSet {
		if s.State() != shard.Initializing {
			continue
		}
		sourceID := s.SourceID()
		if sourceID == "" {
			continue
		}
		sourceInstance, ok := candidateMap[sourceID]
		if !ok {
			// NB(cw): This is not an error because the candidates are not
			// necessarily all the instances in the placement.
			continue
		}
		if sourceInstance.IsLeaving() {
			continue
		}
		if ph.moveShard(s, from, sourceInstance) {
			delete(shardSet, s.ID())
		}
	}
}

func (ph *helper) mostUnderLoadedInstance() (placement.Instance, bool) {
	var (
		res        placement.Instance
		maxLoadGap int
	)

	for id, instance := range ph.instances {
		loadGap := ph.targetLoad[id] - loadOnInstance(instance)
		if loadGap > maxLoadGap {
			maxLoadGap = loadGap
			res = instance
		}
	}
	if maxLoadGap > 0 {
		return res, true
	}

	return nil, false
}

func (ph *helper) optimize(t optimizeType) error {
	var fn assignLoadFn
	switch t {
	case safe:
		fn = ph.assignLoadToInstanceSafe
	case unsafe:
		fn = ph.assignLoadToInstanceUnsafe
	}
	uniq := make(map[string]struct{}, len(ph.instances))
	for {
		ins, ok := ph.mostUnderLoadedInstance()
		if !ok {
			return nil
		}
		if _, exist := uniq[ins.ID()]; exist {
			return nil
		}

		uniq[ins.ID()] = struct{}{}
		if err := fn(ins); err != nil {
			return err
		}
	}
}

func (ph *helper) assignLoadToInstanceSafe(addingInstance placement.Instance) error {
	return ph.assignTargetLoad(addingInstance, func(from, to placement.Instance) bool {
		return ph.moveOneShardInState(from, to, shard.Unknown)
	})
}

func (ph *helper) assignLoadToInstanceUnsafe(addingInstance placement.Instance) error {
	return ph.assignTargetLoad(addingInstance, func(from, to placement.Instance) bool {
		return ph.moveOneShard(from, to)
	})
}

func (ph *helper) reclaimLeavingShards(instance placement.Instance) {
	if instance.Shards().NumShardsForState(shard.Leaving) == 0 {
		// Shortcut if there is nothing to be reclaimed.
		return
	}
	id := instance.ID()
	for _, i := range ph.instances {
		for _, s := range i.Shards().ShardsForState(shard.Initializing) {
			if s.SourceID() == id {
				// NB(cw) in very rare case, the leaving shards could not be taken back.
				// For example: in a RF=2 case, instance a and b on ig1, instance c on ig2,
				// c took shard1 from instance a, before we tried to assign shard1 back to instance a,
				// b got assigned shard1, now if we try to add instance a back to the topology, a can
				// no longer take shard1 back.
				// But it's fine, the algo will fil up those load with other shards from the cluster
				ph.moveShard(s, i, instance)
			}
		}
	}
}

func (ph *helper) addInstance(addingInstance placement.Instance) error {
	ph.reclaimLeavingShards(addingInstance)
	return ph.assignLoadToInstanceUnsafe(addingInstance)
}

func (ph *helper) assignTargetLoad(
	targetInstance placement.Instance,
	moveOneShardFn func(from, to placement.Instance) bool,
) error {
	targetLoad := ph.targetLoadForInstance(targetInstance.ID())
	// try to take shards from the most loaded instances until the adding instance reaches target load
	instanceHeap, err := ph.buildInstanceHeap(nonLeavingInstances(ph.Instances()), false)
	if err != nil {
		return err
	}
	for targetInstance.Shards().NumShards() < targetLoad && instanceHeap.Len() > 0 {
		fromInstance := heap.Pop(instanceHeap).(placement.Instance)
		if moved := moveOneShardFn(fromInstance, targetInstance); moved {
			heap.Push(instanceHeap, fromInstance)
		}
	}
	return nil
}

func (ph *helper) canAssignInstance(shardID uint32, from, to placement.Instance) bool {
	s, ok := to.Shards().Shard(shardID)
	if ok && s.State() != shard.Leaving {
		// NB(cw): a Leaving shard is not counted to the load of the instance
		// so the instance should be able to take the ownership back if needed
		// assuming i1 owns shard 1 as Available, this case can be triggered by:
		// 1: add i2, now shard 1 is "Leaving" on i1 and "Initializing" on i2
		// 2: remove i2, now i2 needs to return shard 1 back to i1
		// and i1 should be able to take it and mark it as "Available"
		return false
	}
	return ph.CanMoveShard(shardID, from, to.IsolationGroup())
}

func (ph *helper) assignShardToInstance(s shard.Shard, to placement.Instance) {
	to.Shards().Add(s)

	if _, exist := ph.shardToInstanceMap[s.ID()]; !exist {
		ph.shardToInstanceMap[s.ID()] = make(map[placement.Instance]struct{})
	}
	ph.shardToInstanceMap[s.ID()][to] = struct{}{}
}

// instanceHeap provides an easy way to get best candidate instance to assign/steal a shard
type instanceHeap struct {
	instances         []placement.Instance
	igToWeightMap     map[string]uint32
	targetLoad        map[string]int
	capacityAscending bool
}

func newHeap(
	instances []placement.Instance,
	capacityAscending bool,
	targetLoad map[string]int,
	igToWeightMap map[string]uint32,
) (*instanceHeap, error) {
	h := &instanceHeap{
		capacityAscending: capacityAscending,
		instances:         instances,
		targetLoad:        targetLoad,
		igToWeightMap:     igToWeightMap,
	}
	heap.Init(h)
	return h, nil
}

func (h *instanceHeap) targetLoadForInstance(id string) int {
	return h.targetLoad[id]
}

func (h *instanceHeap) Len() int {
	return len(h.instances)
}

func (h *instanceHeap) Less(i, j int) bool {
	instanceI := h.instances[i]
	instanceJ := h.instances[j]
	leftLoadOnI := h.targetLoadForInstance(instanceI.ID()) - loadOnInstance(instanceI)
	leftLoadOnJ := h.targetLoadForInstance(instanceJ.ID()) - loadOnInstance(instanceJ)
	// If both instance has tokens to be filled, prefer the one from bigger isolation group
	// since it tends to be more picky in accepting shards
	if leftLoadOnI > 0 && leftLoadOnJ > 0 {
		if instanceI.IsolationGroup() != instanceJ.IsolationGroup() {
			return h.igToWeightMap[instanceI.IsolationGroup()] > h.igToWeightMap[instanceJ.IsolationGroup()]
		}
	}
	// compare left capacity on both instances
	if h.capacityAscending {
		return leftLoadOnI > leftLoadOnJ
	}
	return leftLoadOnI < leftLoadOnJ
}

func (h instanceHeap) Swap(i, j int) {
	h.instances[i], h.instances[j] = h.instances[j], h.instances[i]
}

func (h *instanceHeap) Push(i interface{}) {
	instance := i.(placement.Instance)
	h.instances = append(h.instances, instance)
}

func (h *instanceHeap) Pop() interface{} {
	n := len(h.instances)
	instance := h.instances[n-1]
	h.instances = h.instances[0 : n-1]
	return instance
}

func isOverWeighted(igWeight, totalWeight uint32, rf int) bool {
	return float64(igWeight)/float64(totalWeight) >= 1.0/float64(rf)
}

func addInstanceToPlacement(
	p placement.Placement,
	i placement.Instance,
	t instanceType,
) (placement.Placement, placement.Instance, error) {
	if _, exist := p.Instance(i.ID()); exist {
		return nil, nil, errAddingInstanceAlreadyExist
	}

	switch t {
	case anyType:
	case withShards:
		if i.Shards().NumShards() == 0 {
			return p, i, nil
		}
	default:
		return nil, nil, fmt.Errorf("unexpected type %v", t)
	}

	instance := i.Clone()
	return p.SetInstances(append(p.Instances(), instance)), instance, nil
}

func removeInstanceFromPlacement(p placement.Placement, id string) (placement.Placement, placement.Instance, error) {
	leavingInstance, exist := p.Instance(id)
	if !exist {
		return nil, nil, fmt.Errorf("instance %s does not exist in placement", id)
	}
	return p.SetInstances(removeInstanceFromList(p.Instances(), id)), leavingInstance, nil
}

func getShardMap(shards []shard.Shard) map[uint32]shard.Shard {
	r := make(map[uint32]shard.Shard, len(shards))

	for _, s := range shards {
		r[s.ID()] = s
	}
	return r
}

func loadOnInstance(instance placement.Instance) int {
	return instance.Shards().NumShards() - instance.Shards().NumShardsForState(shard.Leaving)
}

func nonLeavingInstances(instances []placement.Instance) []placement.Instance {
	r := make([]placement.Instance, 0, len(instances))
	for _, instance := range instances {
		if instance.IsLeaving() {
			continue
		}
		r = append(r, instance)
	}

	return r
}

func newShards(shardIDs []uint32) []shard.Shard {
	r := make([]shard.Shard, len(shardIDs))
	for i, id := range shardIDs {
		r[i] = shard.NewShard(id).SetState(shard.Unknown)
	}
	return r
}

func removeInstanceFromList(instances []placement.Instance, instanceID string) []placement.Instance {
	for i, instance := range instances {
		if instance.ID() == instanceID {
			last := len(instances) - 1
			instances[i] = instances[last]
			return instances[:last]
		}
	}
	return instances
}

func markShardsAvailable(p placement.Placement, instanceID string, shardIDs []uint32, opts placement.Options) (placement.Placement, error) {
	instance, exist := p.Instance(instanceID)
	if !exist {
		return nil, fmt.Errorf("instance %s does not exist in placement", instanceID)
	}

	shards := instance.Shards()
	for _, shardID := range shardIDs {
		s, exist := shards.Shard(shardID)
		if !exist {
			return nil, fmt.Errorf("shard %d does not exist in instance %s", shardID, instanceID)
		}

		if s.State() != shard.Initializing {
			return nil, fmt.Errorf("could not mark shard %d as available, it's not in Initializing state", s.ID())
		}

		isCutoverFn := opts.IsShardCutoverFn()
		if isCutoverFn != nil {
			if err := isCutoverFn(s); err != nil {
				return nil, err
			}
		}

		p = p.SetCutoverNanos(opts.PlacementCutoverNanosFn()())
		sourceID := s.SourceID()
		shards.Add(shard.NewShard(shardID).SetState(shard.Available))

		// There could be no source for cases like initial placement.
		if sourceID == "" {
			continue
		}

		sourceInstance, exist := p.Instance(sourceID)
		if !exist {
			return nil, fmt.Errorf("source instance %s for shard %d does not exist in placement", sourceID, shardID)
		}

		sourceShards := sourceInstance.Shards()
		leavingShard, exist := sourceShards.Shard(shardID)
		if !exist {
			return nil, fmt.Errorf("shard %d does not exist in source instance %s", shardID, sourceID)
		}

		if leavingShard.State() != shard.Leaving {
			return nil, fmt.Errorf("shard %d is not leaving instance %s", shardID, sourceID)
		}

		isCutoffFn := opts.IsShardCutoffFn()
		if isCutoffFn != nil {
			if err := isCutoffFn(leavingShard); err != nil {
				return nil, err
			}
		}

		sourceShards.Remove(shardID)
		if sourceShards.NumShards() == 0 {
			p = p.SetInstances(removeInstanceFromList(p.Instances(), sourceInstance.ID()))
		}
	}

	return p, nil
}

// tryCleanupShardState cleans up the shard states if the user only
// wants to keep stable shard state in the placement.
func tryCleanupShardState(
	p placement.Placement,
	opts placement.Options,
) (placement.Placement, error) {
	if opts.ShardStateMode() == placement.StableShardStateOnly {
		p, _, err := markAllShardsAvailable(
			p,
			opts.SetIsShardCutoverFn(nil).SetIsShardCutoffFn(nil),
		)
		return p, err
	}
	return p, nil
}

func markAllShardsAvailable(
	p placement.Placement,
	opts placement.Options,
) (placement.Placement, bool, error) {
	var (
		err     error
		updated = false
	)
	p = p.Clone()
	for _, instance := range p.Instances() {
		for _, s := range instance.Shards().All() {
			if s.State() == shard.Initializing {
				p, err = markShardsAvailable(p, instance.ID(), []uint32{s.ID()}, opts)
				if err != nil {
					return nil, false, err
				}
				updated = true
			}
		}
	}
	return p, updated, nil
}
