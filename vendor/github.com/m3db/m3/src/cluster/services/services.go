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
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/m3db/m3/src/cluster/generated/proto/metadatapb"
	"github.com/m3db/m3/src/cluster/generated/proto/placementpb"
	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/cluster/placement"
	ps "github.com/m3db/m3/src/cluster/placement/service"
	"github.com/m3db/m3/src/cluster/placement/storage"
	"github.com/m3db/m3/src/cluster/shard"
	xwatch "github.com/m3db/m3/src/x/watch"

	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

const (
	defaultGaugeInterval = 10 * time.Second
)

var (
	errNoServiceName             = errors.New("no service specified")
	errNoServiceID               = errors.New("no service id specified")
	errNoInstanceID              = errors.New("no instance id specified")
	errAdPlacementMissing        = errors.New("advertisement is missing placement instance")
	errInstanceNotFound          = errors.New("instance not found")
	errNilPlacementProto         = errors.New("nil placement proto")
	errNilPlacementInstanceProto = errors.New("nil placement instance proto")
	errNilMetadataProto          = errors.New("nil metadata proto")
)

// NewServices returns a client of Services.
func NewServices(opts Options) (Services, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	return &client{
		opts:           opts,
		placementKeyFn: keyFnWithNamespace(placementNamespace(opts.NamespaceOptions().PlacementNamespace())),
		metadataKeyFn:  keyFnWithNamespace(metadataNamespace(opts.NamespaceOptions().MetadataNamespace())),
		kvManagers:     make(map[string]*kvManager),
		hbStores:       make(map[string]HeartbeatService),
		adDoneChs:      make(map[string]chan struct{}),
		ldSvcs:         make(map[leaderKey]LeaderService),
		logger:         opts.InstrumentsOptions().Logger(),
		m:              opts.InstrumentsOptions().MetricsScope(),
	}, nil
}

type client struct {
	sync.RWMutex

	opts           Options
	placementKeyFn keyFn
	metadataKeyFn  keyFn
	kvManagers     map[string]*kvManager
	hbStores       map[string]HeartbeatService
	ldSvcs         map[leaderKey]LeaderService
	adDoneChs      map[string]chan struct{}
	logger         *zap.Logger
	m              tally.Scope
}

func (c *client) Metadata(sid ServiceID) (Metadata, error) {
	if err := validateServiceID(sid); err != nil {
		return nil, err
	}

	m, err := c.getKVManager(sid.Zone())
	if err != nil {
		return nil, err
	}

	v, err := m.kv.Get(c.metadataKeyFn(sid))
	if err != nil {
		return nil, err
	}

	var mp metadatapb.Metadata
	if err = v.Unmarshal(&mp); err != nil {
		return nil, err
	}

	return NewMetadataFromProto(&mp)
}

func (c *client) SetMetadata(sid ServiceID, meta Metadata) error {
	if err := validateServiceID(sid); err != nil {
		return err
	}

	m, err := c.getKVManager(sid.Zone())
	if err != nil {
		return err
	}

	mp, err := meta.Proto()
	if err != nil {
		return err
	}
	_, err = m.kv.Set(c.metadataKeyFn(sid), mp)
	return err
}

func (c *client) DeleteMetadata(sid ServiceID) error {
	if err := validateServiceID(sid); err != nil {
		return err
	}

	m, err := c.getKVManager(sid.Zone())
	if err != nil {
		return err
	}

	_, err = m.kv.Delete(c.metadataKeyFn(sid))
	return err
}

func (c *client) PlacementService(sid ServiceID, opts placement.Options) (placement.Service, error) {
	if err := validateServiceID(sid); err != nil {
		return nil, err
	}

	store, err := c.opts.KVGen()(sid.Zone())
	if err != nil {
		return nil, err
	}

	return ps.NewPlacementService(
		storage.NewPlacementStorage(store, c.placementKeyFn(sid), opts),
		opts,
	), nil
}

func (c *client) Advertise(ad Advertisement) error {
	pi := ad.PlacementInstance()
	if pi == nil {
		return errAdPlacementMissing
	}

	if err := validateAdvertisement(ad.ServiceID(), pi.ID()); err != nil {
		return err
	}

	m, err := c.Metadata(ad.ServiceID())
	if err != nil {
		return err
	}

	hb, err := c.getHeartbeatService(ad.ServiceID())
	if err != nil {
		return err
	}

	key := adKey(ad.ServiceID(), pi.ID())
	c.Lock()
	ch, ok := c.adDoneChs[key]
	if ok {
		c.Unlock()
		return fmt.Errorf("service %s, instance %s is already being advertised", ad.ServiceID(), pi.ID())
	}
	ch = make(chan struct{})
	c.adDoneChs[key] = ch
	c.Unlock()

	go func() {
		sid := ad.ServiceID()
		errCounter := c.serviceTaggedScope(sid).Counter("heartbeat.error")

		tickFn := func() {
			if isHealthy(ad) {
				if err := hb.Heartbeat(pi, m.LivenessInterval()); err != nil {
					c.logger.Error("could not heartbeat service",
						zap.String("service", sid.String()),
						zap.Error(err))
					errCounter.Inc(1)
				}
			}
		}

		tickFn()

		ticker := time.NewTicker(m.HeartbeatInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				tickFn()
			case <-ch:
				return
			}
		}
	}()
	return nil
}

func (c *client) Unadvertise(sid ServiceID, id string) error {
	if err := validateAdvertisement(sid, id); err != nil {
		return err
	}

	key := adKey(sid, id)

	c.Lock()
	if ch, ok := c.adDoneChs[key]; ok {
		// If this client is advertising the instance, stop it.
		close(ch)
		delete(c.adDoneChs, key)
	}
	c.Unlock()

	hbStore, err := c.getHeartbeatService(sid)
	if err != nil {
		return err
	}

	return hbStore.Delete(id)
}

func (c *client) Query(sid ServiceID, opts QueryOptions) (Service, error) {
	if err := validateServiceID(sid); err != nil {
		return nil, err
	}

	v, err := c.getPlacementValue(sid)
	if err != nil {
		return nil, err
	}

	service, err := getServiceFromValue(v, sid)
	if err != nil {
		return nil, err
	}

	if !opts.IncludeUnhealthy() {
		hbStore, err := c.getHeartbeatService(sid)
		if err != nil {
			return nil, err
		}

		ids, err := hbStore.Get()
		if err != nil {
			return nil, err
		}

		service = filterInstances(service, ids)
	}

	return service, nil
}

func (c *client) Watch(sid ServiceID, opts QueryOptions) (Watch, error) {
	if err := validateServiceID(sid); err != nil {
		return nil, err
	}

	c.logger.Info("adding a watch",
		zap.String("service", sid.Name()),
		zap.String("env", sid.Environment()),
		zap.String("zone", sid.Zone()),
		zap.Bool("includeUnhealthy", opts.IncludeUnhealthy()),
	)

	kvm, err := c.getKVManager(sid.Zone())
	if err != nil {
		return nil, err
	}

	kvm.RLock()
	watchable, exist := kvm.serviceWatchables[sid.String()]
	kvm.RUnlock()
	if exist {
		_, w, err := watchable.watch()
		return w, err
	}

	// Prepare the watch of placement outside of lock.
	key := c.placementKeyFn(sid)
	placementWatch, err := kvm.kv.Watch(key)
	if err != nil {
		return nil, err
	}

	initValue, err := c.waitForInitValue(kvm.kv, placementWatch, sid, c.opts.InitTimeout())
	if err != nil {
		return nil, fmt.Errorf("could not get init value for '%s' within timeout, err: %v", key, err)
	}

	initService, err := getServiceFromValue(initValue, sid)
	if err != nil {
		placementWatch.Close()
		return nil, err
	}

	kvm.Lock()
	defer kvm.Unlock()
	watchable, exist = kvm.serviceWatchables[sid.String()]
	if exist {
		// If a watchable already exist now, we need to clean up the placement watch we just created.
		placementWatch.Close()
		_, w, err := watchable.watch()
		return w, err
	}

	watchable = newServiceWatchable()
	sdm := newServiceDiscoveryMetrics(c.serviceTaggedScope(sid))

	if !opts.IncludeUnhealthy() {
		hbStore, err := c.getHeartbeatService(sid)
		if err != nil {
			placementWatch.Close()
			return nil, err
		}
		heartbeatWatch, err := hbStore.Watch()
		if err != nil {
			placementWatch.Close()
			return nil, err
		}
		watchable.update(filterInstancesWithWatch(initService, heartbeatWatch))
		go c.watchPlacementAndHeartbeat(watchable, placementWatch, heartbeatWatch, initValue, sid, initService, sdm.serviceUnmalshalErr)
	} else {
		watchable.update(initService)
		go c.watchPlacement(watchable, placementWatch, initValue, sid, sdm.serviceUnmalshalErr)
	}

	kvm.serviceWatchables[sid.String()] = watchable

	go updateVersionGauge(placementWatch, sdm.versionGauge)

	_, w, err := watchable.watch()
	return w, err
}

func (c *client) HeartbeatService(sid ServiceID) (HeartbeatService, error) {
	if err := validateServiceID(sid); err != nil {
		return nil, err
	}

	return c.getHeartbeatService(sid)
}

func (c *client) getPlacementValue(sid ServiceID) (kv.Value, error) {
	kvm, err := c.getKVManager(sid.Zone())
	if err != nil {
		return nil, err
	}

	v, err := kvm.kv.Get(c.placementKeyFn(sid))
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (c *client) getHeartbeatService(sid ServiceID) (HeartbeatService, error) {
	c.Lock()
	defer c.Unlock()
	hb, ok := c.hbStores[sid.String()]
	if ok {
		return hb, nil
	}

	hb, err := c.opts.HeartbeatGen()(sid)
	if err != nil {
		return nil, err
	}

	c.hbStores[sid.String()] = hb
	return hb, nil
}

func (c *client) LeaderService(sid ServiceID, opts ElectionOptions) (LeaderService, error) {
	if sid == nil {
		return nil, errNoServiceID
	}

	if opts == nil {
		opts = NewElectionOptions()
	}

	key := leaderCacheKey(sid, opts)

	c.RLock()
	if ld, ok := c.ldSvcs[key]; ok {
		c.RUnlock()
		return ld, nil
	}
	c.RUnlock()

	c.Lock()
	defer c.Unlock()

	if ld, ok := c.ldSvcs[key]; ok {
		return ld, nil
	}

	ld, err := c.opts.LeaderGen()(sid, opts)
	if err != nil {
		return nil, err
	}

	c.ldSvcs[key] = ld
	return ld, nil
}

func (c *client) getKVManager(zone string) (*kvManager, error) {
	c.Lock()
	defer c.Unlock()
	m, ok := c.kvManagers[zone]
	if ok {
		return m, nil
	}

	kv, err := c.opts.KVGen()(zone)
	if err != nil {
		return nil, err
	}

	m = &kvManager{
		kv:                kv,
		serviceWatchables: map[string]serviceWatchable{},
	}

	c.kvManagers[zone] = m
	return m, nil
}

func (c *client) watchPlacement(
	w serviceWatchable,
	vw kv.ValueWatch,
	initValue kv.Value,
	sid ServiceID,
	errCounter tally.Counter,
) {
	for range vw.C() {
		newService := c.serviceFromUpdate(vw.Get(), initValue, sid, errCounter)
		if newService == nil {
			continue
		}

		w.update(newService)
	}
}

func (c *client) watchPlacementAndHeartbeat(
	w serviceWatchable,
	vw kv.ValueWatch,
	heartbeatWatch xwatch.Watch,
	initValue kv.Value,
	sid ServiceID,
	service Service,
	errCounter tally.Counter,
) {
	for {
		select {
		case <-vw.C():
			newService := c.serviceFromUpdate(vw.Get(), initValue, sid, errCounter)
			if newService == nil {
				continue
			}

			service = newService
		case <-heartbeatWatch.C():
			c.logger.Info("received heartbeat update")
		}
		w.update(filterInstancesWithWatch(service, heartbeatWatch))
	}
}

func (c *client) serviceFromUpdate(
	value kv.Value,
	initValue kv.Value,
	sid ServiceID,
	errCounter tally.Counter,
) Service {
	if value == nil {
		// NB(cw) this can only happen when the placement has been deleted
		// it is safer to let the user keep using the old topology
		c.logger.Info("received placement update with nil value")
		return nil
	}

	if initValue != nil && !value.IsNewer(initValue) {
		// NB(cw) this can only happen when the init wait called a Get() itself
		// so the init value did not come from the watch, when the watch gets created
		// the first update from it may from the same version.
		c.logger.Info("received stale placement update, skip",
			zap.Int("version", value.Version()))
		return nil
	}

	newService, err := getServiceFromValue(value, sid)
	if err != nil {
		c.logger.Error("could not unmarshal update from kv store for placement",
			zap.Int("version", value.Version()),
			zap.Error(err))
		errCounter.Inc(1)
		return nil
	}

	c.logger.Info("successfully parsed placement", zap.Int("version", value.Version()))
	return newService
}

func (c *client) serviceTaggedScope(sid ServiceID) tally.Scope {
	return c.m.Tagged(
		map[string]string{
			"sd_service": sid.Name(),
			"sd_env":     sid.Environment(),
			"sd_zone":    sid.Zone(),
		},
	)
}

func isHealthy(ad Advertisement) bool {
	healthFn := ad.Health()
	return healthFn == nil || healthFn() == nil
}

func filterInstances(s Service, ids []string) Service {
	if s == nil {
		return nil
	}

	instances := make([]ServiceInstance, 0, len(s.Instances()))
	for _, id := range ids {
		if instance, err := s.Instance(id); err == nil {
			instances = append(instances, instance)
		}
	}

	return NewService().
		SetInstances(instances).
		SetSharding(s.Sharding()).
		SetReplication(s.Replication())
}

func filterInstancesWithWatch(s Service, hbw xwatch.Watch) Service {
	if hbw.Get() == nil {
		return s
	}
	return filterInstances(s, hbw.Get().([]string))
}

func updateVersionGauge(vw kv.ValueWatch, versionGauge tally.Gauge) {
	for range time.Tick(defaultGaugeInterval) {
		v := vw.Get()
		if v != nil {
			versionGauge.Update(float64(v.Version()))
		}
	}
}

func getServiceFromValue(value kv.Value, sid ServiceID) (Service, error) {
	p, err := placementFromValue(value)
	if err != nil {
		return nil, err
	}

	return NewServiceFromPlacement(p, sid), nil
}

func (c *client) waitForInitValue(kvStore kv.Store, w kv.ValueWatch, sid ServiceID, timeout time.Duration) (kv.Value, error) {
	if timeout < 0 {
		timeout = defaultInitTimeout
	} else if timeout == 0 {
		// We want no timeout if specifically asking for none
		<-w.C()
		return w.Get(), nil
	}
	select {
	case <-w.C():
		return w.Get(), nil
	case <-time.After(timeout):
		return kvStore.Get(c.placementKeyFn(sid))
	}
}

func validateAdvertisement(sid ServiceID, id string) error {
	if sid == nil {
		return errNoServiceID
	}

	if id == "" {
		return errNoInstanceID
	}

	return nil
}

// cache key for leader service clients
type leaderKey struct {
	sid           string
	leaderTimeout time.Duration
	resignTimeout time.Duration
	ttl           int
}

func leaderCacheKey(sid ServiceID, opts ElectionOptions) leaderKey {
	return leaderKey{
		sid:           sid.String(),
		leaderTimeout: opts.LeaderTimeout(),
		resignTimeout: opts.ResignTimeout(),
		ttl:           opts.TTLSecs(),
	}
}

type kvManager struct {
	sync.RWMutex

	kv                kv.Store
	serviceWatchables map[string]serviceWatchable
}

func newServiceDiscoveryMetrics(m tally.Scope) serviceDiscoveryMetrics {
	return serviceDiscoveryMetrics{
		versionGauge:        m.Gauge("placement.version"),
		serviceUnmalshalErr: m.Counter("placement.unmarshal.error"),
	}
}

type serviceDiscoveryMetrics struct {
	versionGauge        tally.Gauge
	serviceUnmalshalErr tally.Counter
}

// NewService creates a new Service.
func NewService() Service { return new(service) }

// NewServiceFromProto takes the data from a placement and a service id and
// returns the corresponding Service object.
func NewServiceFromProto(
	p *placementpb.Placement,
	sid ServiceID,
) (Service, error) {
	if p == nil {
		return nil, errNilPlacementProto
	}
	r := make([]ServiceInstance, 0, len(p.Instances))
	for _, instance := range p.Instances {
		instance, err := NewServiceInstanceFromProto(instance, sid)
		if err != nil {
			return nil, err
		}
		r = append(r, instance)
	}

	return NewService().
		SetReplication(NewServiceReplication().SetReplicas(int(p.ReplicaFactor))).
		SetSharding(NewServiceSharding().SetNumShards(int(p.NumShards)).SetIsSharded(p.IsSharded)).
		SetInstances(r), nil
}

// NewServiceFromPlacement creates a Service from the placement and service ID.
func NewServiceFromPlacement(p placement.Placement, sid ServiceID) Service {
	var (
		placementInstances = p.Instances()
		serviceInstances   = make([]ServiceInstance, len(placementInstances))
	)

	for i, placementInstance := range placementInstances {
		serviceInstances[i] = NewServiceInstanceFromPlacementInstance(placementInstance, sid)
	}

	return NewService().
		SetReplication(NewServiceReplication().SetReplicas(p.ReplicaFactor())).
		SetSharding(NewServiceSharding().SetNumShards(p.NumShards()).SetIsSharded(p.IsSharded())).
		SetInstances(serviceInstances)
}

type service struct {
	instances   []ServiceInstance
	replication ServiceReplication
	sharding    ServiceSharding
}

func (s *service) Instance(instanceID string) (ServiceInstance, error) {
	for _, instance := range s.instances {
		if instance.InstanceID() == instanceID {
			return instance, nil
		}
	}
	return nil, errInstanceNotFound
}
func (s *service) Instances() []ServiceInstance                 { return s.instances }
func (s *service) Replication() ServiceReplication              { return s.replication }
func (s *service) Sharding() ServiceSharding                    { return s.sharding }
func (s *service) SetInstances(insts []ServiceInstance) Service { s.instances = insts; return s }
func (s *service) SetReplication(r ServiceReplication) Service  { s.replication = r; return s }
func (s *service) SetSharding(ss ServiceSharding) Service       { s.sharding = ss; return s }

// NewServiceReplication creates a new ServiceReplication.
func NewServiceReplication() ServiceReplication { return new(serviceReplication) }

type serviceReplication struct {
	replicas int
}

func (r *serviceReplication) Replicas() int                          { return r.replicas }
func (r *serviceReplication) SetReplicas(rep int) ServiceReplication { r.replicas = rep; return r }

// NewServiceSharding creates a new ServiceSharding.
func NewServiceSharding() ServiceSharding { return new(serviceSharding) }

type serviceSharding struct {
	isSharded bool
	numShards int
}

func (s *serviceSharding) NumShards() int                      { return s.numShards }
func (s *serviceSharding) IsSharded() bool                     { return s.isSharded }
func (s *serviceSharding) SetNumShards(n int) ServiceSharding  { s.numShards = n; return s }
func (s *serviceSharding) SetIsSharded(v bool) ServiceSharding { s.isSharded = v; return s }

// NewServiceInstance creates a new ServiceInstance.
func NewServiceInstance() ServiceInstance { return new(serviceInstance) }

// NewServiceInstanceFromProto creates a new service instance from proto.
func NewServiceInstanceFromProto(
	instance *placementpb.Instance,
	sid ServiceID,
) (ServiceInstance, error) {
	if instance == nil {
		return nil, errNilPlacementInstanceProto
	}
	shards, err := shard.NewShardsFromProto(instance.Shards)
	if err != nil {
		return nil, err
	}
	return NewServiceInstance().
		SetServiceID(sid).
		SetInstanceID(instance.Id).
		SetEndpoint(instance.Endpoint).
		SetShards(shards), nil
}

// NewServiceInstanceFromPlacementInstance creates a new service instance from placement instance.
func NewServiceInstanceFromPlacementInstance(
	instance placement.Instance,
	sid ServiceID,
) ServiceInstance {
	return NewServiceInstance().
		SetServiceID(sid).
		SetInstanceID(instance.ID()).
		SetEndpoint(instance.Endpoint()).
		SetShards(instance.Shards())
}

type serviceInstance struct {
	service  ServiceID
	id       string
	endpoint string
	shards   shard.Shards
}

func (i *serviceInstance) InstanceID() string                       { return i.id }
func (i *serviceInstance) Endpoint() string                         { return i.endpoint }
func (i *serviceInstance) Shards() shard.Shards                     { return i.shards }
func (i *serviceInstance) ServiceID() ServiceID                     { return i.service }
func (i *serviceInstance) SetInstanceID(id string) ServiceInstance  { i.id = id; return i }
func (i *serviceInstance) SetEndpoint(e string) ServiceInstance     { i.endpoint = e; return i }
func (i *serviceInstance) SetShards(s shard.Shards) ServiceInstance { i.shards = s; return i }

func (i *serviceInstance) SetServiceID(service ServiceID) ServiceInstance {
	i.service = service
	return i
}

// NewAdvertisement creates a new Advertisement.
func NewAdvertisement() Advertisement { return new(advertisement) }

type advertisement struct {
	instance placement.Instance
	service  ServiceID
	health   func() error
}

func (a *advertisement) ServiceID() ServiceID                   { return a.service }
func (a *advertisement) Health() func() error                   { return a.health }
func (a *advertisement) PlacementInstance() placement.Instance  { return a.instance }
func (a *advertisement) SetServiceID(s ServiceID) Advertisement { a.service = s; return a }
func (a *advertisement) SetHealth(h func() error) Advertisement { a.health = h; return a }
func (a *advertisement) SetPlacementInstance(p placement.Instance) Advertisement {
	a.instance = p
	return a
}

// NewServiceID creates new ServiceID.
func NewServiceID() ServiceID { return new(serviceID) }

type serviceID struct {
	name string
	env  string
	zone string
}

func (sid *serviceID) Name() string                      { return sid.name }
func (sid *serviceID) Environment() string               { return sid.env }
func (sid *serviceID) Zone() string                      { return sid.zone }
func (sid *serviceID) SetName(n string) ServiceID        { sid.name = n; return sid }
func (sid *serviceID) SetEnvironment(e string) ServiceID { sid.env = e; return sid }
func (sid *serviceID) SetZone(z string) ServiceID        { sid.zone = z; return sid }

func (sid *serviceID) Equal(other ServiceID) bool {
	if other == nil {
		return false
	}
	return sid.Name() == other.Name() &&
		sid.Zone() == other.Zone() &&
		sid.Environment() == other.Environment()
}

func (sid *serviceID) String() string {
	return fmt.Sprintf("[name: %s, env: %s, zone: %s]", sid.name, sid.env, sid.zone)
}

// NewMetadata creates new Metadata.
func NewMetadata() Metadata { return new(metadata) }

// NewMetadataFromProto converts a Metadata proto message to an instance of
// Metadata.
func NewMetadataFromProto(m *metadatapb.Metadata) (Metadata, error) {
	if m == nil {
		return nil, errNilMetadataProto
	}
	return NewMetadata().
		SetPort(m.Port).
		SetLivenessInterval(time.Duration(m.LivenessInterval)).
		SetHeartbeatInterval(time.Duration(m.HeartbeatInterval)), nil
}

type metadata struct {
	port              uint32
	livenessInterval  time.Duration
	heartbeatInterval time.Duration
}

func (m *metadata) Port() uint32                     { return m.port }
func (m *metadata) LivenessInterval() time.Duration  { return m.livenessInterval }
func (m *metadata) HeartbeatInterval() time.Duration { return m.heartbeatInterval }
func (m *metadata) SetPort(p uint32) Metadata        { m.port = p; return m }

func (m *metadata) SetLivenessInterval(l time.Duration) Metadata {
	m.livenessInterval = l
	return m
}

func (m *metadata) SetHeartbeatInterval(l time.Duration) Metadata {
	m.heartbeatInterval = l
	return m
}

func (m *metadata) String() string {
	return fmt.Sprintf("[port: %d, livenessInterval: %v, heartbeatInterval: %v]",
		m.port,
		m.livenessInterval,
		m.heartbeatInterval,
	)
}

func (m *metadata) Proto() (*metadatapb.Metadata, error) {
	return &metadatapb.Metadata{
		Port:              m.Port(),
		LivenessInterval:  int64(m.LivenessInterval()),
		HeartbeatInterval: int64(m.HeartbeatInterval()),
	}, nil
}
