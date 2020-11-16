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

package placement

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/m3db/m3/src/cluster/generated/proto/placementpb"
	"github.com/m3db/m3/src/cluster/shard"
)

const (
	// uninitializedShardSetID represents uninitialized shard set id.
	uninitializedShardSetID = 0
)

var (
	errNilPlacementProto          = errors.New("nil placement proto")
	errNilPlacementSnapshotsProto = errors.New("nil placement snapshots proto")
	errNilPlacementInstanceProto  = errors.New("nil placement instance proto")
	errDuplicatedShards           = errors.New("invalid placement, there are duplicated shards in one replica")
	errUnexpectedShards           = errors.New("invalid placement, there are unexpected shard ids on instance")
	errMirrorNotSharded           = errors.New("invalid placement, mirrored placement must be sharded")
)

type placement struct {
	instances        map[string]Instance
	instancesByShard map[uint32][]Instance
	rf               int
	shards           []uint32
	cutoverNanos     int64
	version          int
	maxShardSetID    uint32
	isSharded        bool
	isMirrored       bool
}

// NewPlacement returns a ServicePlacement
func NewPlacement() Placement {
	return &placement{}
}

// NewPlacementFromProto creates a new placement from proto.
func NewPlacementFromProto(p *placementpb.Placement) (Placement, error) {
	if p == nil {
		return nil, errNilPlacementProto
	}

	shards := make([]uint32, p.NumShards)
	for i := uint32(0); i < p.NumShards; i++ {
		shards[i] = i
	}
	instances := make([]Instance, 0, len(p.Instances))
	for _, instance := range p.Instances {
		pi, err := NewInstanceFromProto(instance)
		if err != nil {
			return nil, err
		}
		instances = append(instances, pi)
	}

	return NewPlacement().
		SetInstances(instances).
		SetShards(shards).
		SetReplicaFactor(int(p.ReplicaFactor)).
		SetIsSharded(p.IsSharded).
		SetCutoverNanos(p.CutoverTime).
		SetIsMirrored(p.IsMirrored).
		SetMaxShardSetID(p.MaxShardSetId), nil
}

func (p *placement) InstancesForShard(shard uint32) []Instance {
	if len(p.instancesByShard) == 0 {
		return nil
	}
	return p.instancesByShard[shard]
}

func (p *placement) Instances() []Instance {
	result := make([]Instance, 0, p.NumInstances())
	for _, instance := range p.instances {
		result = append(result, instance)
	}
	sort.Sort(ByIDAscending(result))
	return result
}

func (p *placement) SetInstances(instances []Instance) Placement {
	instancesMap := make(map[string]Instance, len(instances))
	instancesByShard := make(map[uint32][]Instance)
	for _, instance := range instances {
		instancesMap[instance.ID()] = instance
		for _, shard := range instance.Shards().AllIDs() {
			instancesByShard[shard] = append(instancesByShard[shard], instance)
		}
	}

	// Sort the instances by their ids for deterministic ordering.
	for _, instances := range instancesByShard {
		sort.Sort(ByIDAscending(instances))
	}

	p.instancesByShard = instancesByShard
	p.instances = instancesMap
	return p
}

func (p *placement) NumInstances() int {
	return len(p.instances)
}

func (p *placement) Instance(id string) (Instance, bool) {
	instance, ok := p.instances[id]
	return instance, ok
}

func (p *placement) ReplicaFactor() int {
	return p.rf
}

func (p *placement) SetReplicaFactor(rf int) Placement {
	p.rf = rf
	return p
}

func (p *placement) Shards() []uint32 {
	return p.shards
}

func (p *placement) SetShards(shards []uint32) Placement {
	p.shards = shards
	return p
}

func (p *placement) NumShards() int {
	return len(p.shards)
}

func (p *placement) IsSharded() bool {
	return p.isSharded
}

func (p *placement) SetIsSharded(v bool) Placement {
	p.isSharded = v
	return p
}

func (p *placement) IsMirrored() bool {
	return p.isMirrored
}

func (p *placement) SetIsMirrored(v bool) Placement {
	p.isMirrored = v
	return p
}

func (p *placement) MaxShardSetID() uint32 {
	return p.maxShardSetID
}

func (p *placement) SetMaxShardSetID(v uint32) Placement {
	p.maxShardSetID = v
	return p
}

func (p *placement) CutoverNanos() int64 {
	return p.cutoverNanos
}

func (p *placement) SetCutoverNanos(cutoverNanos int64) Placement {
	p.cutoverNanos = cutoverNanos
	return p
}

func (p *placement) Version() int {
	return p.version
}

func (p *placement) SetVersion(v int) Placement {
	p.version = v
	return p
}

func (p *placement) String() string {
	return fmt.Sprintf(
		"Placement[Instances=%s, NumShards=%d, ReplicaFactor=%d, IsSharded=%v, IsMirrored=%v]",
		p.Instances(), p.NumShards(), p.ReplicaFactor(), p.IsSharded(), p.IsMirrored(),
	)
}

func (p *placement) Proto() (*placementpb.Placement, error) {
	instances := make(map[string]*placementpb.Instance, p.NumInstances())
	for _, instance := range p.Instances() {
		pi, err := instance.Proto()
		if err != nil {
			return nil, err
		}
		instances[instance.ID()] = pi
	}

	return &placementpb.Placement{
		Instances:     instances,
		ReplicaFactor: uint32(p.ReplicaFactor()),
		NumShards:     uint32(p.NumShards()),
		IsSharded:     p.IsSharded(),
		CutoverTime:   p.CutoverNanos(),
		IsMirrored:    p.IsMirrored(),
		MaxShardSetId: p.MaxShardSetID(),
	}, nil
}

func (p *placement) Clone() Placement {
	return NewPlacement().
		SetInstances(Instances(p.Instances()).Clone()).
		SetShards(p.Shards()).
		SetReplicaFactor(p.ReplicaFactor()).
		SetIsSharded(p.IsSharded()).
		SetIsMirrored(p.IsMirrored()).
		SetCutoverNanos(p.CutoverNanos()).
		SetMaxShardSetID(p.MaxShardSetID()).
		SetVersion(p.Version())
}

// Placements represents a list of placements.
type Placements []Placement

// NewPlacementsFromProto creates a list of placements from proto.
func NewPlacementsFromProto(p *placementpb.PlacementSnapshots) (Placements, error) {
	if p == nil {
		return nil, errNilPlacementSnapshotsProto
	}

	placements := make([]Placement, 0, len(p.Snapshots))
	for _, snapshot := range p.Snapshots {
		placement, err := NewPlacementFromProto(snapshot)
		if err != nil {
			return nil, err
		}
		placements = append(placements, placement)
	}
	sort.Sort(placementsByCutoverAsc(placements))
	return placements, nil
}

// Proto converts a list of Placement to a proto.
func (placements Placements) Proto() (*placementpb.PlacementSnapshots, error) {
	snapshots := make([]*placementpb.Placement, 0, len(placements))
	for _, p := range placements {
		placementProto, err := p.Proto()
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, placementProto)
	}
	return &placementpb.PlacementSnapshots{
		Snapshots: snapshots,
	}, nil
}

// ActiveIndex finds the index of the last placement whose cutover time is no
// later than timeNanos (a.k.a. the active placement). Assuming the cutover times
// of the placements are sorted in ascending order (i.e., earliest time first).
func (placements Placements) ActiveIndex(timeNanos int64) int {
	idx := 0
	for idx < len(placements) && placements[idx].CutoverNanos() <= timeNanos {
		idx++
	}
	idx--
	return idx
}

// Validate validates a placement to ensure:
// - The shards on each instance are in valid state.
// - The total number of shards match rf * num_shards_per_replica.
// - Each shard shows up rf times.
// - There is one Initializing shard for each Leaving shard.
// - The instances with same shard_set_id owns the same shards.
func Validate(p Placement) error {
	if p.IsMirrored() && !p.IsSharded() {
		return errMirrorNotSharded
	}

	shardCountMap := convertShardSliceToMap(p.Shards())
	if len(shardCountMap) != len(p.Shards()) {
		return errDuplicatedShards
	}

	expectedTotal := len(p.Shards()) * p.ReplicaFactor()
	totalCapacity := 0
	totalLeaving := 0
	totalInit := 0
	totalInitWithSourceID := 0
	maxShardSetID := p.MaxShardSetID()
	instancesByShardSetID := make(map[uint32]Instance, p.NumInstances())
	for _, instance := range p.Instances() {
		if instance.Endpoint() == "" {
			return fmt.Errorf("instance %s does not contain valid endpoint", instance.String())
		}
		if instance.Shards().NumShards() == 0 && p.IsSharded() {
			return fmt.Errorf("instance %s contains no shard in a sharded placement", instance.String())
		}
		if instance.Shards().NumShards() != 0 && !p.IsSharded() {
			return fmt.Errorf("instance %s contains shards in a non-sharded placement", instance.String())
		}
		shardSetID := instance.ShardSetID()
		if shardSetID > maxShardSetID {
			return fmt.Errorf("instance %s shard set id %d is larger than max shard set id %d in the placement", instance.String(), shardSetID, maxShardSetID)
		}
		for _, s := range instance.Shards().All() {
			count, exist := shardCountMap[s.ID()]
			if !exist {
				return errUnexpectedShards
			}
			switch s.State() {
			case shard.Available:
				shardCountMap[s.ID()] = count + 1
				totalCapacity++
			case shard.Initializing:
				totalInit++
				shardCountMap[s.ID()] = count + 1
				totalCapacity++
				if s.SourceID() != "" {
					totalInitWithSourceID++
				}
			case shard.Leaving:
				totalLeaving++
			default:
				return fmt.Errorf("invalid shard state %v for shard %d", s.State(), s.ID())
			}
		}
		if shardSetID == uninitializedShardSetID {
			continue
		}
		existingInstance, exists := instancesByShardSetID[shardSetID]
		if !exists {
			instancesByShardSetID[shardSetID] = instance
		} else {
			// Both existing shard ids and current shard ids are sorted in ascending order.
			existingShardIDs := existingInstance.Shards().AllIDs()
			currShardIDs := instance.Shards().AllIDs()
			if len(existingShardIDs) != len(currShardIDs) {
				return fmt.Errorf("instance %s and %s have the same shard set id %d but different number of shards", existingInstance.String(), instance.String(), shardSetID)
			}
			for i := 0; i < len(existingShardIDs); i++ {
				if existingShardIDs[i] != currShardIDs[i] {
					return fmt.Errorf("instance %s and %s have the same shard set id %d but different shards", existingInstance.String(), instance.String(), shardSetID)
				}
			}
		}
	}

	if !p.IsSharded() {
		return nil
	}

	// initializing could be more than leaving for cases like initial placement
	if totalLeaving > totalInit {
		return fmt.Errorf("invalid placement, %d shards in Leaving state, more than %d in Initializing state", totalLeaving, totalInit)
	}

	if totalLeaving != totalInitWithSourceID {
		return fmt.Errorf("invalid placement, %d shards in Leaving state, not equal %d in Initializing state with source id", totalLeaving, totalInitWithSourceID)
	}

	if expectedTotal != totalCapacity {
		return fmt.Errorf("invalid placement, the total available shards in the placement is %d, expecting %d", totalCapacity, expectedTotal)
	}

	for shard, c := range shardCountMap {
		if p.ReplicaFactor() != c {
			return fmt.Errorf("invalid shard count for shard %d: expected %d, actual %d", shard, p.ReplicaFactor(), c)
		}
	}
	return nil
}

func convertShardSliceToMap(ids []uint32) map[uint32]int {
	shardCounts := make(map[uint32]int)
	for _, id := range ids {
		shardCounts[id] = 0
	}
	return shardCounts
}

// NewInstance returns a new Instance
func NewInstance() Instance {
	return &instance{shards: shard.NewShards(nil)}
}

// NewEmptyInstance returns a Instance with some basic properties but no shards assigned
func NewEmptyInstance(id, isolationGroup, zone, endpoint string, weight uint32) Instance {
	return &instance{
		id:             id,
		isolationGroup: isolationGroup,
		zone:           zone,
		weight:         weight,
		endpoint:       endpoint,
		shards:         shard.NewShards(nil),
	}
}

// NewInstanceFromProto creates a new placement instance from proto.
func NewInstanceFromProto(instance *placementpb.Instance) (Instance, error) {
	if instance == nil {
		return nil, errNilPlacementInstanceProto
	}
	shards, err := shard.NewShardsFromProto(instance.Shards)
	if err != nil {
		return nil, err
	}
	debugPort := uint32(0)
	if instance.Metadata != nil {
		debugPort = instance.Metadata.DebugPort
	}

	return NewInstance().
		SetID(instance.Id).
		SetIsolationGroup(instance.IsolationGroup).
		SetWeight(instance.Weight).
		SetZone(instance.Zone).
		SetEndpoint(instance.Endpoint).
		SetShards(shards).
		SetShardSetID(instance.ShardSetId).
		SetHostname(instance.Hostname).
		SetPort(instance.Port).
		SetMetadata(InstanceMetadata{
			DebugPort: debugPort,
		}), nil
}

type instance struct {
	id             string
	isolationGroup string
	zone           string
	endpoint       string
	hostname       string
	shards         shard.Shards
	port           uint32
	weight         uint32
	shardSetID     uint32
	metadata       InstanceMetadata
}

func (i *instance) String() string {
	return fmt.Sprintf(
		"Instance[ID=%s, IsolationGroup=%s, Zone=%s, Weight=%d, Endpoint=%s, Hostname=%s, Port=%d, ShardSetID=%d, Shards=%s, Metadata=%+v]",
		i.id, i.isolationGroup, i.zone, i.weight, i.endpoint, i.hostname, i.port, i.shardSetID, i.shards.String(), i.metadata,
	)
}

func (i *instance) ID() string {
	return i.id
}

func (i *instance) SetID(id string) Instance {
	i.id = id
	return i
}

func (i *instance) IsolationGroup() string {
	return i.isolationGroup
}

func (i *instance) SetIsolationGroup(r string) Instance {
	i.isolationGroup = r
	return i
}

func (i *instance) Zone() string {
	return i.zone
}

func (i *instance) SetZone(z string) Instance {
	i.zone = z
	return i
}

func (i *instance) Weight() uint32 {
	return i.weight
}

func (i *instance) SetWeight(w uint32) Instance {
	i.weight = w
	return i
}

func (i *instance) Endpoint() string {
	return i.endpoint
}

func (i *instance) SetEndpoint(ip string) Instance {
	i.endpoint = ip
	return i
}

func (i *instance) Hostname() string {
	return i.hostname
}

func (i *instance) SetHostname(value string) Instance {
	i.hostname = value
	return i
}

func (i *instance) Port() uint32 {
	return i.port
}

func (i *instance) SetPort(value uint32) Instance {
	i.port = value
	return i
}

func (i *instance) ShardSetID() uint32 {
	return i.shardSetID
}

func (i *instance) SetShardSetID(value uint32) Instance {
	i.shardSetID = value
	return i
}

func (i *instance) Shards() shard.Shards {
	return i.shards
}

func (i *instance) SetShards(s shard.Shards) Instance {
	i.shards = s
	return i
}

func (i *instance) Metadata() InstanceMetadata {
	return i.metadata
}

func (i *instance) SetMetadata(value InstanceMetadata) Instance {
	i.metadata = value
	return i
}

func (i *instance) Proto() (*placementpb.Instance, error) {
	ss, err := i.Shards().Proto()
	if err != nil {
		return &placementpb.Instance{}, err
	}

	return &placementpb.Instance{
		Id:             i.ID(),
		IsolationGroup: i.IsolationGroup(),
		Zone:           i.Zone(),
		Weight:         i.Weight(),
		Endpoint:       i.Endpoint(),
		Shards:         ss,
		ShardSetId:     i.ShardSetID(),
		Hostname:       i.Hostname(),
		Port:           i.Port(),
		Metadata: &placementpb.InstanceMetadata{
			DebugPort: i.Metadata().DebugPort,
		},
	}, nil
}

func (i *instance) IsLeaving() bool {
	return i.allShardsInState(shard.Leaving)
}

func (i *instance) IsInitializing() bool {
	return i.allShardsInState(shard.Initializing)
}

func (i *instance) IsAvailable() bool {
	return i.allShardsInState(shard.Available)
}

func (i *instance) allShardsInState(s shard.State) bool {
	ss := i.Shards()
	numShards := ss.NumShards()
	if numShards == 0 {
		return false
	}
	return numShards == ss.NumShardsForState(s)
}

func (i *instance) Clone() Instance {
	return NewInstance().
		SetID(i.ID()).
		SetIsolationGroup(i.IsolationGroup()).
		SetZone(i.Zone()).
		SetWeight(i.Weight()).
		SetEndpoint(i.Endpoint()).
		SetHostname(i.Hostname()).
		SetPort(i.Port()).
		SetShardSetID(i.ShardSetID()).
		SetShards(i.Shards().Clone()).
		SetMetadata(i.Metadata())
}

// Instances is a slice of instances that can produce a debug string.
type Instances []Instance

func (instances Instances) String() string {
	if len(instances) == 0 {
		return "[]"
	}
	// 256 should be pretty sufficient for the string representation
	// of each instance.
	strs := make([]string, 0, len(instances)*256)
	strs = append(strs, "[\n")
	for _, elem := range instances {
		strs = append(strs, "\t"+elem.String()+",\n")
	}
	strs = append(strs, "]")
	return strings.Join(strs, "")
}

// Clone returns a set of cloned instances.
func (instances Instances) Clone() Instances {
	cloned := make([]Instance, len(instances))
	for i, instance := range instances {
		cloned[i] = instance.Clone()
	}
	return cloned
}

// ByIDAscending sorts Instance by ID ascending
type ByIDAscending []Instance

func (s ByIDAscending) Len() int {
	return len(s)
}

func (s ByIDAscending) Less(i, j int) bool {
	return strings.Compare(s[i].ID(), s[j].ID()) < 0
}

func (s ByIDAscending) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
