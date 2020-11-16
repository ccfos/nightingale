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

package services

import (
	"time"

	"github.com/m3db/m3/src/cluster/generated/proto/metadatapb"
	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/cluster/placement"
	"github.com/m3db/m3/src/cluster/services/leader/campaign"
	"github.com/m3db/m3/src/cluster/shard"
	"github.com/m3db/m3/src/x/instrument"
	xwatch "github.com/m3db/m3/src/x/watch"
)

// Services provides access to the service topology.
type Services interface {
	// Advertise advertises the availability of an instance of a service.
	Advertise(ad Advertisement) error

	// Unadvertise indicates a given instance is no longer available.
	Unadvertise(service ServiceID, id string) error

	// Query returns the topology for a given service.
	Query(service ServiceID, opts QueryOptions) (Service, error)

	// Watch returns a watch on metadata and a list of available instances for a given service.
	Watch(service ServiceID, opts QueryOptions) (Watch, error)

	// Metadata returns the metadata for a given service.
	Metadata(sid ServiceID) (Metadata, error)

	// SetMetadata sets the metadata for a given service.
	SetMetadata(sid ServiceID, m Metadata) error

	// DeleteMetadata deletes the metadata for a given service
	DeleteMetadata(sid ServiceID) error

	// PlacementService returns a client of placement.Service.
	PlacementService(sid ServiceID, popts placement.Options) (placement.Service, error)

	// HeartbeatService returns a heartbeat store for the given service.
	HeartbeatService(service ServiceID) (HeartbeatService, error)

	// LeaderService returns an instance of a leader service for the given
	// service ID.
	LeaderService(service ServiceID, opts ElectionOptions) (LeaderService, error)
}

// KVGen generates a kv store for a given zone.
type KVGen func(zone string) (kv.Store, error)

// HeartbeatGen generates a heartbeat store for a given zone.
type HeartbeatGen func(sid ServiceID) (HeartbeatService, error)

// LeaderGen generates a leader service instance for a given service.
type LeaderGen func(sid ServiceID, opts ElectionOptions) (LeaderService, error)

// Options are options for the client of Services.
type Options interface {
	// InitTimeout is the max time to wait on a new service watch for a valid initial value.
	// If the value is set to 0, then no wait will be done and the watch could return empty value.
	InitTimeout() time.Duration

	// SetInitTimeout sets the InitTimeout.
	SetInitTimeout(t time.Duration) Options

	// KVGen is the function to generate a kv store for a given zone.
	KVGen() KVGen

	// SetKVGen sets the KVGen.
	SetKVGen(gen KVGen) Options

	// HeartbeatGen is the function to generate a heartbeat store for a given zone.
	HeartbeatGen() HeartbeatGen

	// SetHeartbeatGen sets the HeartbeatGen.
	SetHeartbeatGen(gen HeartbeatGen) Options

	// LeaderGen is the function to generate a leader service instance for a
	// given service.
	LeaderGen() LeaderGen

	// SetLeaderGen sets the leader generation function.
	SetLeaderGen(gen LeaderGen) Options

	// InstrumentsOptions is the instrument options.
	InstrumentsOptions() instrument.Options

	// SetInstrumentsOptions sets the InstrumentsOptions.
	SetInstrumentsOptions(iopts instrument.Options) Options

	// NamespaceOptions is the custom namespaces.
	NamespaceOptions() NamespaceOptions

	// SetNamespaceOptions sets the NamespaceOptions.
	SetNamespaceOptions(opts NamespaceOptions) Options

	// Validate validates the Options.
	Validate() error
}

// NamespaceOptions are options to provide custom namespaces in service discovery service.
// TODO(cw): Provide overrides for leader service and heartbeat service.
type NamespaceOptions interface {
	// PlacementNamespace is the custom namespace for placement.
	PlacementNamespace() string

	// SetPlacementNamespace sets the custom namespace for placement.
	SetPlacementNamespace(v string) NamespaceOptions

	// MetadataNamespace is the custom namespace for metadata.
	MetadataNamespace() string

	// SetMetadataNamespace sets the custom namespace for metadata.
	SetMetadataNamespace(v string) NamespaceOptions
}

// OverrideOptions configs the override for service discovery.
type OverrideOptions interface {
	// NamespaceOptions is the namespace options.
	NamespaceOptions() NamespaceOptions

	// SetNamespaceOptions sets namespace options.
	SetNamespaceOptions(opts NamespaceOptions) OverrideOptions
}

// Watch is a watcher that issues notification when a service is updated.
type Watch interface {
	// Close closes the watch.
	Close()

	// C returns the notification channel.
	C() <-chan struct{}

	// Get returns the latest service of the service watchable.
	Get() Service
}

// Service describes the metadata and instances of a service.
type Service interface {
	// Instance returns the service instance with the instance id.
	Instance(instanceID string) (ServiceInstance, error)

	// Instances returns the service instances.
	Instances() []ServiceInstance

	// SetInstances sets the service instances.
	SetInstances(insts []ServiceInstance) Service

	// Replication returns the service replication description or nil if none.
	Replication() ServiceReplication

	// SetReplication sets the service replication description or nil if none.
	SetReplication(r ServiceReplication) Service

	// Sharding returns the service sharding description or nil if none.
	Sharding() ServiceSharding

	// SetSharding sets the service sharding description or nil if none
	SetSharding(s ServiceSharding) Service
}

// ServiceReplication describes the replication of a service.
type ServiceReplication interface {
	// Replicas is the count of replicas.
	Replicas() int

	// SetReplicas sets the count of replicas.
	SetReplicas(r int) ServiceReplication
}

// ServiceSharding describes the sharding of a service.
type ServiceSharding interface {
	// NumShards is the number of shards to use for sharding.
	NumShards() int

	// SetNumShards sets the number of shards to use for sharding.
	SetNumShards(n int) ServiceSharding

	// IsSharded() returns whether this service is sharded.
	IsSharded() bool

	// SetIsSharded sets IsSharded.
	SetIsSharded(s bool) ServiceSharding
}

// ServiceInstance is a single instance of a service.
type ServiceInstance interface {
	// ServiceID returns the service id of the instance.
	ServiceID() ServiceID

	// SetServiceID sets the service id of the instance.
	SetServiceID(service ServiceID) ServiceInstance

	// InstanceID returns the id of the instance.
	InstanceID() string

	// SetInstanceID sets the id of the instance.
	SetInstanceID(id string) ServiceInstance

	// Endpoint returns the endpoint of the instance.
	Endpoint() string

	// SetEndpoint sets the endpoint of the instance.
	SetEndpoint(e string) ServiceInstance

	// Shards returns the shards of the instance.
	Shards() shard.Shards

	// SetShards sets the shards of the instance.
	SetShards(s shard.Shards) ServiceInstance
}

// Advertisement advertises the availability of a given instance of a service.
type Advertisement interface {
	// the service being advertised.
	ServiceID() ServiceID

	// sets the service being advertised.
	SetServiceID(service ServiceID) Advertisement

	// optional health function, return an error to indicate unhealthy.
	Health() func() error

	// sets the health function for the advertised instance.
	SetHealth(health func() error) Advertisement

	// PlacementInstance returns the placement instance associated with this advertisement, which
	// contains the ID of the instance advertising and all other relevant fields.
	PlacementInstance() placement.Instance

	// SetPlacementInstance sets the Instance that is advertising.
	SetPlacementInstance(p placement.Instance) Advertisement
}

// ServiceID contains the fields required to id a service.
type ServiceID interface {
	// Name returns the service name of the ServiceID.
	Name() string

	// SetName sets the service name of the ServiceID.
	SetName(s string) ServiceID

	// Environment returns the environment of the ServiceID.
	Environment() string

	// SetEnvironment sets the environment of the ServiceID.
	SetEnvironment(env string) ServiceID

	// Zone returns the zone of the ServiceID.
	Zone() string

	// SetZone sets the zone of the ServiceID.
	SetZone(zone string) ServiceID

	// Equal retruns if the service IDs are equivalent.
	Equal(value ServiceID) bool

	// String returns a description of the ServiceID.
	String() string
}

// QueryOptions are options to service discovery queries.
type QueryOptions interface {
	// IncludeUnhealthy decides whether unhealthy instances should be returned.
	IncludeUnhealthy() bool

	// SetIncludeUnhealthy sets the value of IncludeUnhealthy.
	SetIncludeUnhealthy(h bool) QueryOptions
}

// Metadata contains the metadata for a service.
type Metadata interface {
	// String returns a description of the metadata.
	String() string

	// Port returns the port to be used to contact the service.
	Port() uint32

	// SetPort sets the port.
	SetPort(p uint32) Metadata

	// LivenessInterval is the ttl interval for an instance to be considered as healthy.
	LivenessInterval() time.Duration

	// SetLivenessInterval sets the LivenessInterval.
	SetLivenessInterval(l time.Duration) Metadata

	// HeartbeatInterval is the interval for heatbeats.
	HeartbeatInterval() time.Duration

	// SetHeartbeatInterval sets the HeartbeatInterval.
	SetHeartbeatInterval(h time.Duration) Metadata

	// Proto returns the proto representation for the Metadata.
	Proto() (*metadatapb.Metadata, error)
}

// HeartbeatService manages heartbeating instances.
type HeartbeatService interface {
	// Heartbeat sends heartbeat for a service instance with a ttl.
	Heartbeat(instance placement.Instance, ttl time.Duration) error

	// Get gets healthy instances for a service.
	Get() ([]string, error)

	// GetInstances returns a deserialized list of healthy Instances.
	GetInstances() ([]placement.Instance, error)

	// Delete deletes the heartbeat for a service instance.
	Delete(instance string) error

	// Watch watches the heartbeats for a service.
	Watch() (xwatch.Watch, error)
}

// ElectionOptions configure specific election-scoped options.
type ElectionOptions interface {
	// Duration after which a call to Leader() will timeout if no response
	// returned from etcd. Defaults to 30 seconds.
	LeaderTimeout() time.Duration
	SetLeaderTimeout(t time.Duration) ElectionOptions

	// Duration after which a call to Resign() will timeout if no response
	// returned from etcd. Defaults to 30 seconds.
	ResignTimeout() time.Duration
	SetResignTimeout(t time.Duration) ElectionOptions

	// TTL returns the TTL used for campaigns. By default (ttl == 0), etcd will
	// set the TTL to 60s.
	TTLSecs() int
	SetTTLSecs(ttl int) ElectionOptions
}

// CampaignOptions provide the ability to override campaign defaults.
type CampaignOptions interface {
	// LeaderValue allows the user to override the value a campaign announces
	// (that is, the value an observer sees upon calling Leader()). This
	// defaults to the hostname of the caller.
	LeaderValue() string
	SetLeaderValue(v string) CampaignOptions
}

// LeaderService provides access to etcd-backed leader elections.
type LeaderService interface {
	// Close closes the election service client entirely. No more campaigns can be
	// started and any outstanding campaigns are closed.
	Close() error

	// Campaign proposes that the caller become the leader for a specified
	// election, with its leadership being refreshed on an interval according to
	// the ElectionOptions the service was created with. It returns a read-only
	// channel of campaign status events that is closed when the user resigns
	// leadership or the campaign is invalidated due to background session
	// expiration (i.e. failing to refresh etcd leadership lease). The caller
	// MUST consume this channel until it is closed or risk goroutine leaks.
	// Users are encouraged to read the package docs of services/leader for
	// advice on proper usage and common gotchas.
	//
	// The leader will announce its hostname to observers unless opts is non-nil
	// and opts.LeaderValue() is non-empty.
	Campaign(electionID string, opts CampaignOptions) (<-chan campaign.Status, error)

	// Resign gives up leadership of a specified election if the caller is the
	// current leader (if the caller is not the leader an error is returned).
	Resign(electionID string) error

	// Leader returns the current leader of a specified election (if there is no
	// leader then leader.ErrNoLeader is returned).
	Leader(electionID string) (string, error)

	// Observe returns a channel on which leader updates for a specified election
	// will be returned. If no one is campaigning for the given election the call
	// will still succeed and the channel will receive its first update when an
	// election is started.
	Observe(electionID string) (<-chan string, error)
}
