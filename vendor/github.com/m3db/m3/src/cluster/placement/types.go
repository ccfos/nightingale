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
	"time"

	"github.com/m3db/m3/src/cluster/generated/proto/placementpb"
	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/cluster/shard"
	"github.com/m3db/m3/src/x/clock"
	"github.com/m3db/m3/src/x/instrument"

	"github.com/golang/protobuf/proto"
)

// Instance represents an instance in a placement.
type Instance interface {
	// String is for debugging.
	String() string

	// ID is the id of the instance.
	ID() string

	// SetID sets the id of the instance.
	SetID(id string) Instance

	// IsolationGroup is the isolation group of the instance,
	// which usually but not necessarily refers to the rack of the instance.
	IsolationGroup() string

	// SetIsolationGroup sets the isolation group of the instance.
	SetIsolationGroup(r string) Instance

	// Zone is the zone of the instance.
	Zone() string

	// SetZone sets the zone of the instance.
	SetZone(z string) Instance

	// Weight is the weight of the instance.
	Weight() uint32

	// SetWeight sets the weight of the instance.
	SetWeight(w uint32) Instance

	// Endpoint is the endpoint of the instance.
	Endpoint() string

	// SetEndpoint sets the endpoint of the instance.
	SetEndpoint(ip string) Instance

	// Shards returns the shards owned by the instance.
	Shards() shard.Shards

	// SetShards sets the shards owned by the instance.
	SetShards(s shard.Shards) Instance

	// ShardSetID returns the shard set id.
	ShardSetID() uint32

	// SetShardSetID sets the shard set id.
	SetShardSetID(value uint32) Instance

	// Hostname returns the hostname of the instance.
	Hostname() string

	// SetHostname sets the hostname of the instance.
	SetHostname(value string) Instance

	// Port returns the port of the instance.
	Port() uint32

	// SetPort sets the port of the instance.
	SetPort(value uint32) Instance

	// Metadata returns the metadata of the instance.
	Metadata() InstanceMetadata

	// SetMetadata sets the metadata of the instance.
	SetMetadata(value InstanceMetadata) Instance

	// Proto returns the proto representation for the Instance.
	Proto() (*placementpb.Instance, error)

	// IsLeaving returns whether the instance contains only leaving shards.
	IsLeaving() bool

	// IsInitializing returns whether the instance contains only initializing shards.
	IsInitializing() bool

	// IsAvailable returns whether the instance contains only available shards.
	IsAvailable() bool

	// Clone returns a clone of the Instance.
	Clone() Instance
}

// InstanceMetadata represents the metadata for a single Instance in the placement.
type InstanceMetadata struct {
	DebugPort uint32
}

// Placement describes how instances are placed.
type Placement interface {
	// InstancesForShard returns the instances for a given shard id.
	InstancesForShard(shard uint32) []Instance

	// Instances returns all instances in the placement
	Instances() []Instance

	// SetInstances sets the instances
	SetInstances(instances []Instance) Placement

	// NumInstances returns the number of instances in the placement
	NumInstances() int

	// Instance returns the instance for the requested id
	Instance(id string) (Instance, bool)

	// ReplicaFactor returns the replica factor in the placement
	ReplicaFactor() int

	// SetReplicaFactor sets the ReplicaFactor
	SetReplicaFactor(rf int) Placement

	// Shards returns all the unique shard ids for a replica
	Shards() []uint32

	// SetShards sets the unique shard ids for a replica
	SetShards(s []uint32) Placement

	// ShardsLen returns the number of shards in a replica
	NumShards() int

	// IsSharded() returns whether this placement is sharded
	IsSharded() bool

	// SetIsSharded() sets IsSharded
	SetIsSharded(v bool) Placement

	// CutoverNanos returns the cutover time in nanoseconds.
	CutoverNanos() int64

	// SetCutoverNanos sets the cutover time in nanoseconds.
	SetCutoverNanos(cutoverNanos int64) Placement

	// IsMirrored() returns whether the placement is mirrored.
	IsMirrored() bool

	// SetIsMirrored() sets IsMirrored.
	SetIsMirrored(v bool) Placement

	// MaxShardSetID returns the maximum shard set id used before to guarantee unique
	// shard set id generations across placement changes.
	MaxShardSetID() uint32

	// SetMaxShardSetID sets the maximum shard set id used before to guarantee unique
	// shard set id generations across placement changes.
	SetMaxShardSetID(value uint32) Placement

	// String returns a description of the placement
	String() string

	// Version() returns the version of the placement retreived from the
	// backing MVCC store.
	Version() int

	// SetVersion() sets the version of the placement object. Since version
	// is determined by the backing MVCC store, calling this method has no
	// effect in terms of the updated ServicePlacement that is written back
	// to the MVCC store.
	SetVersion(v int) Placement

	// Proto returns the proto representation for the Placement.
	Proto() (*placementpb.Placement, error)

	// Clone returns a clone of the Placement.
	Clone() Placement
}

// Watch watches for updates of a placement.
type Watch interface {
	// C returns the notification channel.
	C() <-chan struct{}

	// Get returns the latest version of the placement.
	Get() (Placement, error)

	// Close stops watching for placement updates.
	Close()
}

// DoneFn is called when caller is done using the resource.
type DoneFn func()

// StagedPlacementWatcher watches for updates to staged placement.
type StagedPlacementWatcher interface {
	// Watch starts watching the updates.
	Watch() error

	// ActiveStagedPlacement returns the currently active staged placement, the
	// callback function when the caller is done using the active staged placement,
	// and any errors encountered.
	ActiveStagedPlacement() (ActiveStagedPlacement, DoneFn, error)

	// Unwatch stops watching the updates.
	Unwatch() error
}

// StagedPlacementWatcherOptions provide a set of staged placement watcher options.
type StagedPlacementWatcherOptions interface {
	// SetClockOptions sets the clock options.
	SetClockOptions(value clock.Options) StagedPlacementWatcherOptions

	// ClockOptions returns the clock options.
	ClockOptions() clock.Options

	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(value instrument.Options) StagedPlacementWatcherOptions

	// InstrumentOptions returns the instrument options.
	InstrumentOptions() instrument.Options

	// SetActiveStagedPlacementOptions sets the active staged placement options.
	SetActiveStagedPlacementOptions(value ActiveStagedPlacementOptions) StagedPlacementWatcherOptions

	// ActiveStagedPlacementOptions returns the active staged placement options.
	ActiveStagedPlacementOptions() ActiveStagedPlacementOptions

	// SetStagedPlacementKey sets the kv key to watch for staged placement.
	SetStagedPlacementKey(value string) StagedPlacementWatcherOptions

	// StagedPlacementKey returns the kv key to watch for staged placement.
	StagedPlacementKey() string

	// SetStagedPlacementStore sets the staged placement store.
	SetStagedPlacementStore(store kv.Store) StagedPlacementWatcherOptions

	// StagedPlacementStore returns the staged placement store.
	StagedPlacementStore() kv.Store

	// SetInitWatchTimeout sets the initial watch timeout.
	SetInitWatchTimeout(value time.Duration) StagedPlacementWatcherOptions

	// InitWatchTimeout returns the initial watch timeout.
	InitWatchTimeout() time.Duration
}

// ActiveStagedPlacement describes active staged placement.
type ActiveStagedPlacement interface {
	// ActivePlacement returns the currently active placement for a given time, the callback
	// function when the caller is done using the placement, and any errors encountered.
	ActivePlacement() (Placement, DoneFn, error)

	// Close closes the active staged placement.
	Close() error
}

// OnPlacementsAddedFn is called when placements are added.
type OnPlacementsAddedFn func(placements []Placement)

// OnPlacementsRemovedFn is called when placements are removed.
type OnPlacementsRemovedFn func(placements []Placement)

// ActiveStagedPlacementOptions provide a set of options for active staged placement.
type ActiveStagedPlacementOptions interface {
	// SetClockOptions sets the clock options.
	SetClockOptions(value clock.Options) ActiveStagedPlacementOptions

	// ClockOptions returns the clock options.
	ClockOptions() clock.Options

	// SetOnPlacementsAddedFn sets the callback function for adding placement.
	SetOnPlacementsAddedFn(value OnPlacementsAddedFn) ActiveStagedPlacementOptions

	// OnPlacementsAddedFn returns the callback function for adding placement.
	OnPlacementsAddedFn() OnPlacementsAddedFn

	// SetOnPlacementsRemovedFn sets the callback function for removing placement.
	SetOnPlacementsRemovedFn(value OnPlacementsRemovedFn) ActiveStagedPlacementOptions

	// OnPlacementsRemovedFn returns the callback function for removing placement.
	OnPlacementsRemovedFn() OnPlacementsRemovedFn
}

// StagedPlacement describes a series of placements applied in staged fashion.
type StagedPlacement interface {
	// ActiveStagedPlacement returns the active staged placement for a given time.
	ActiveStagedPlacement(timeNanos int64) ActiveStagedPlacement

	// Version returns the version of the staged placement.
	Version() int

	// SetVersion sets the version of the staged placement.
	SetVersion(version int) StagedPlacement

	// Placements return the placements in the staged placement.
	Placements() Placements

	// SetPlacements sets the placements in the staged placement.
	SetPlacements(placements []Placement) StagedPlacement

	// ActiveStagedPlacementOptions returns the active staged placement options.
	ActiveStagedPlacementOptions() ActiveStagedPlacementOptions

	// SetActiveStagedPlacementOptions sets the active staged placement options.
	SetActiveStagedPlacementOptions(opts ActiveStagedPlacementOptions) StagedPlacement

	// Proto returns the proto representation for the StagedPlacement.
	Proto() (*placementpb.PlacementSnapshots, error)
}

// TimeNanosFn returns the time in the format of Unix nanoseconds.
type TimeNanosFn func() int64

// ShardValidateFn validates the shard.
type ShardValidateFn func(s shard.Shard) error

// ValidateFn validates the placement.
type ValidateFn func(p Placement) error

// Options is the interface for placement options.
type Options interface {
	// AllowPartialReplace allows shards from the leaving instance to be
	// placed on instances other than the new instances in a replace operation
	AllowPartialReplace() bool

	// SetAllowPartialReplace sets AllowPartialReplace.
	SetAllowPartialReplace(allowPartialReplace bool) Options

	// AllowAllZones will enable the placement to contain hosts that
	// are not contained within the same zone of the actual placement. This is
	// needed for services that require cross zone communication.
	AllowAllZones() bool

	// SetAllowAllZones sets AllowAllZones.
	SetAllowAllZones(allowAllZones bool) Options

	// AddAllCandidates determines whether the placement will attempt to add all
	// candidates when adding instances or just a single one.
	AddAllCandidates() bool

	// SetAddAllCandidates sets AddAllCandidates.
	SetAddAllCandidates(addAllCandidates bool) Options

	// InstanceSelector defines the strategy used to select new instances from a list of
	// candidates when adding or replacing nodes in the placement. The default is determined
	// by IsMirrored(); false => selector.NewNonMirroredSelector, true => NewPortMirroredSelector.
	InstanceSelector() InstanceSelector

	// SetInstanceSelector -- see InstanceSelector.
	SetInstanceSelector(s InstanceSelector) Options

	// IsSharded describes whether a placement needs to be sharded,
	// when set to false, no specific shards will be assigned to any instance.
	IsSharded() bool

	// SetIsSharded sets IsSharded.
	SetIsSharded(sharded bool) Options

	// ShardStateMode describes the mode to manage shard state in the placement.
	ShardStateMode() ShardStateMode

	// SetShardStateMode sets ShardStateMode.
	SetShardStateMode(value ShardStateMode) Options

	// Dryrun will try to perform the placement operation but will not persist the final result.
	Dryrun() bool

	// SetDryrun sets whether the Dryrun value.
	SetDryrun(d bool) Options

	// IsMirrored returns whether the shard distribution should be mirrored
	// to support master/slave model.
	IsMirrored() bool

	// SetIsMirrored sets IsMirrored.
	SetIsMirrored(m bool) Options

	// IsStaged returns whether the placement should keep all the snapshots.
	IsStaged() bool

	// SetIsStaged sets whether the placement should keep all the snapshots.
	SetIsStaged(v bool) Options

	// InstrumentOptions is the options for instrument.
	InstrumentOptions() instrument.Options

	// SetInstrumentOptions sets the instrument options.
	SetInstrumentOptions(iopts instrument.Options) Options

	// ValidZone returns the zone that added instances must be in in order
	// to be added to a placement.
	ValidZone() string

	// SetValidZone sets the zone that added instances must be in in order to
	// be added to a placement. By default the valid zone will be the zone of
	// instances already in a placement, however if a placement is empty then
	// it is necessary to specify the valid zone when adding the first
	// instance.
	SetValidZone(z string) Options

	// PlacementCutoverNanosFn returns the TimeNanosFn for placement cutover time.
	PlacementCutoverNanosFn() TimeNanosFn

	// SetPlacementCutoverNanosFn sets the TimeNanosFn for placement cutover time.
	SetPlacementCutoverNanosFn(fn TimeNanosFn) Options

	// ShardCutoverNanosFn returns the TimeNanosFn for shard cutover time.
	ShardCutoverNanosFn() TimeNanosFn

	// SetShardCutoverNanosFn sets the TimeNanosFn for shard cutover time.
	SetShardCutoverNanosFn(fn TimeNanosFn) Options

	// ShardCutoffNanosFn returns the TimeNanosFn for shard cutoff time.
	ShardCutoffNanosFn() TimeNanosFn

	// SetShardCutoffNanosFn sets the TimeNanosFn for shard cutoff time.
	SetShardCutoffNanosFn(fn TimeNanosFn) Options

	// IsShardCutoverFn returns the validation function for shard cutover.
	IsShardCutoverFn() ShardValidateFn

	// SetIsShardCutoverFn sets the validation function for shard cutover.
	SetIsShardCutoverFn(fn ShardValidateFn) Options

	// IsShardCutoffFn returns the validation function for shard cutoff.
	IsShardCutoffFn() ShardValidateFn

	// SetIsShardCutoffFn sets the validation function for shard cutoff.
	SetIsShardCutoffFn(fn ShardValidateFn) Options

	// ValidateFnBeforeUpdate returns the validate function to be applied before
	// a placement update.
	ValidateFnBeforeUpdate() ValidateFn

	// SetValidateFnBeforeUpdate sets the validate function to be applied before
	// a placement update.
	SetValidateFnBeforeUpdate(fn ValidateFn) Options

	// NowFn returns the function to get time now.
	NowFn() clock.NowFn

	// SetNowFn sets the function to get time now.
	SetNowFn(fn clock.NowFn) Options
}

// ShardStateMode describes the way to manage shard state in the placement.
type ShardStateMode int

const (
	// StableShardStateOnly means the placement should only keep stable shard state.
	StableShardStateOnly ShardStateMode = iota

	// IncludeTransitionalShardStates means the placement will include transitional shard states.
	IncludeTransitionalShardStates
)

// Storage provides read and write access to placement.
type Storage interface {
	// Set writes a placement.
	Set(p Placement) (Placement, error)

	// CheckAndSet writes a placement if the current version
	// matches the expected version.
	CheckAndSet(p Placement, version int) (Placement, error)

	// SetIfNotExist writes a placement.
	SetIfNotExist(p Placement) (Placement, error)

	// Placement reads placement.
	Placement() (Placement, error)

	// Watch returns a watch for the placement updates.
	Watch() (Watch, error)

	// Delete deletes the placement.
	Delete() error

	// SetProto sets the proto as the placement.
	SetProto(p proto.Message) (int, error)

	// CheckAndSetProto writes a proto if the current version
	// matches the expected version.
	CheckAndSetProto(p proto.Message, version int) (int, error)

	// Proto returns the placement proto.
	Proto() (proto.Message, int, error)

	// PlacementForVersion returns the placement of a specific version.
	PlacementForVersion(version int) (Placement, error)
}

// Service handles the placement related operations for registered services
// all write or update operations will persist the generated placement before returning success.
type Service interface {
	Storage
	operations
}

// Operator is a purely in-memory version of Service; it applies placement related operations to
// a local copy of a placement without persisting anything to backing storage. This can be useful
// to apply multiple placement operations in a row before persisting them, e.g.:
//
// func DoMultipleOps(opts placement.Options, store placement.Storage) {
//    curPlacement := store.Placement()
//    op := placement.NewOperator(curPlacement, opts)
//    op.ReplaceInstances(...)
//    op.MarkAllShardsAvailable()
//    store.CheckAndSet(op.Placement())
// }
type Operator interface {
	operations

	Placement() Placement
}

// operations are the methods shared by Service and Operator. This type is private because it's
// not intended to be implemented directly; Operator and Service are the correct ways to access
// these methods.
type operations interface {
	// BuildInitialPlacement initialize a placement.
	BuildInitialPlacement(instances []Instance, numShards int, rf int) (Placement, error)

	// AddReplica up the replica factor by 1 in the placement.
	AddReplica() (Placement, error)

	// AddInstances adds instances from the candidate list to the placement.
	AddInstances(candidates []Instance) (newPlacement Placement, addedInstances []Instance, err error)

	// RemoveInstances removes instances from the placement.
	RemoveInstances(leavingInstanceIDs []string) (Placement, error)

	// ReplaceInstances picks instances from the candidate list to replace instances in current placement.
	ReplaceInstances(
		leavingInstanceIDs []string,
		candidates []Instance,
	) (
		newPlacement Placement,
		usedInstances []Instance,
		err error,
	)

	// MarkShardsAvailable marks given shards as available.
	MarkShardsAvailable(instanceID string, shardIDs ...uint32) (Placement, error)

	// MarkInstanceAvailable marks all the shards on a given instance as available.
	MarkInstanceAvailable(instanceID string) (Placement, error)

	// MarkAllShardsAvailable marks shard states as available where applicable.
	MarkAllShardsAvailable() (Placement, error)
}

// Algorithm places shards on instances.
type Algorithm interface {
	// InitPlacement initialize a sharding placement with given replica factor.
	InitialPlacement(instances []Instance, shards []uint32, rf int) (Placement, error)

	// AddReplica up the replica factor by 1 in the placement.
	AddReplica(p Placement) (Placement, error)

	// AddInstances adds a list of instance to the placement.
	AddInstances(p Placement, instances []Instance) (Placement, error)

	// RemoveInstances removes a list of instances from the placement.
	RemoveInstances(p Placement, leavingInstanceIDs []string) (Placement, error)

	// ReplaceInstance replace a list of instances with new instances.
	ReplaceInstances(
		p Placement,
		leavingInstanecIDs []string,
		addingInstances []Instance,
	) (Placement, error)

	// IsCompatibleWith checks whether the algorithm could be applied to given placement.
	IsCompatibleWith(p Placement) error

	// MarkShardsAvailable marks given shards as available.
	MarkShardsAvailable(p Placement, instanceID string, shardIDs ...uint32) (Placement, error)

	// MarkAllShardsAvailable marks shard states as available where applicable.
	MarkAllShardsAvailable(p Placement) (Placement, bool, error)
}

// InstanceSelector selects valid instances for the placement change.
type InstanceSelector interface {
	// SelectInitialInstances selects instances for the initial placement.
	SelectInitialInstances(
		candidates []Instance,
		rf int,
	) ([]Instance, error)

	// SelectAddingInstances selects instances to be added to the placement.
	SelectAddingInstances(
		candidates []Instance,
		p Placement,
	) ([]Instance, error)

	// SelectReplaceInstances selects instances to replace existing instances in the placement.
	SelectReplaceInstances(
		candidates []Instance,
		leavingInstanceIDs []string,
		p Placement,
	) ([]Instance, error)
}

// DeploymentPlanner generates deployment steps for a placement
type DeploymentPlanner interface {
	// DeploymentSteps returns the deployment steps
	DeploymentSteps(p Placement) [][]Instance
}

// DeploymentOptions provides options for DeploymentPlanner
type DeploymentOptions interface {
	// MaxStepSize limits the number of instances to be deployed in one step
	MaxStepSize() int
	SetMaxStepSize(stepSize int) DeploymentOptions
}
