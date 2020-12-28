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

package etcd

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/m3db/m3/src/cluster/etcd/watchmanager"
	"github.com/m3db/m3/src/cluster/generated/proto/placementpb"
	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/cluster/placement"
	"github.com/m3db/m3/src/cluster/services"
	"github.com/m3db/m3/src/x/retry"
	"github.com/m3db/m3/src/x/watch"

	"go.etcd.io/etcd/clientv3"
	"github.com/golang/protobuf/proto"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

const (
	heartbeatKeyPrefix = "_hb"
	keySeparator       = "/"
	keyFormat          = "%s/%s"
)

var (
	noopCancel     func()
	errNoServiceID = errors.New("ServiceID cannot be empty")
)

// NewStore creates a heartbeat store based on etcd
func NewStore(c *clientv3.Client, opts Options) (services.HeartbeatService, error) {
	if opts.ServiceID() == nil {
		return nil, errNoServiceID
	}

	scope := opts.InstrumentsOptions().MetricsScope()

	store := &client{
		cache:      newLeaseCache(),
		watchables: make(map[string]watch.Watchable),
		opts:       opts,
		sid:        opts.ServiceID(),
		logger:     opts.InstrumentsOptions().Logger(),
		retrier:    retry.NewRetrier(opts.RetryOptions()),
		m: clientMetrics{
			etcdGetError:   scope.Counter("etcd-get-error"),
			etcdPutError:   scope.Counter("etcd-put-error"),
			etcdLeaseError: scope.Counter("etcd-lease-error"),
		},

		l:       c.Lease,
		kv:      c.KV,
		watcher: c.Watcher,
	}

	wOpts := watchmanager.NewOptions().
		SetWatcher(c.Watcher).
		SetUpdateFn(store.update).
		SetTickAndStopFn(store.tickAndStop).
		SetWatchOptions([]clientv3.OpOption{
			// WithPrefix so that the watch will receive any changes
			// from the instances under the service
			clientv3.WithPrefix(),
			// periodically (appx every 10 mins) checks for the latest data
			// with or without any update notification
			clientv3.WithProgressNotify(),
			// receive initial notification once the watch channel is created
			clientv3.WithCreatedNotify(),
		}).
		SetWatchChanCheckInterval(opts.WatchChanCheckInterval()).
		SetWatchChanInitTimeout(opts.WatchChanInitTimeout()).
		SetWatchChanResetInterval(opts.WatchChanResetInterval()).
		SetInstrumentsOptions(opts.InstrumentsOptions())

	wm, err := watchmanager.NewWatchManager(wOpts)
	if err != nil {
		return nil, err
	}

	store.wm = wm

	return store, nil
}

type client struct {
	sync.RWMutex

	cache      *leaseCache
	watchables map[string]watch.Watchable
	opts       Options
	sid        services.ServiceID
	logger     *zap.Logger
	retrier    retry.Retrier
	m          clientMetrics

	l       clientv3.Lease
	kv      clientv3.KV
	watcher clientv3.Watcher

	wm watchmanager.WatchManager
}

type clientMetrics struct {
	etcdGetError   tally.Counter
	etcdPutError   tally.Counter
	etcdLeaseError tally.Counter
}

func (c *client) Heartbeat(instance placement.Instance, ttl time.Duration) error {
	leaseID, ok := c.cache.get(c.sid, instance.ID(), ttl)
	if ok {
		ctx, cancel := c.context()
		defer cancel()

		_, err := c.l.KeepAliveOnce(ctx, leaseID)
		// if err != nil, it could because the old lease has already timedout
		// on the server side, we need to try a new lease.
		if err == nil {
			return nil
		}
	}

	ctx, cancel := c.context()
	defer cancel()

	resp, err := c.l.Grant(ctx, int64(ttl/time.Second))
	if err != nil {
		c.m.etcdLeaseError.Inc(1)
		return err
	}

	ctx, cancel = c.context()
	defer cancel()

	instanceProto, err := instance.Proto()
	if err != nil {
		return err
	}

	instanceBytes, err := proto.Marshal(instanceProto)
	if err != nil {
		return err
	}

	_, err = c.kv.Put(
		ctx,
		heartbeatKey(c.sid, instance.ID()),
		string(instanceBytes),
		clientv3.WithLease(resp.ID),
	)
	if err != nil {
		c.m.etcdPutError.Inc(1)
		return err
	}

	c.cache.put(c.sid, instance.ID(), ttl, resp.ID)

	return nil
}

func (c *client) Get() ([]string, error) {
	return c.get(servicePrefix(c.sid))
}

func (c *client) get(key string) ([]string, error) {
	ctx, cancel := c.context()
	defer cancel()

	resp, err := c.kv.Get(
		ctx,
		key,
		clientv3.WithPrefix(),
		clientv3.WithKeysOnly(),
	)

	if err != nil {
		c.m.etcdGetError.Inc(1)
		return nil, err
	}

	r := make([]string, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		r[i] = instanceFromKey(string(kv.Key), key)
	}

	return r, nil
}

func (c *client) GetInstances() ([]placement.Instance, error) {
	return c.getInstances(servicePrefix(c.sid))
}

func (c *client) getInstances(key string) ([]placement.Instance, error) {
	ctx, cancel := c.context()
	defer cancel()

	gr, err := c.kv.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		c.m.etcdGetError.Inc(1)
		return nil, err
	}

	r := make([]placement.Instance, len(gr.Kvs))
	for i, kv := range gr.Kvs {
		var p placementpb.Instance
		if err := proto.Unmarshal(kv.Value, &p); err != nil {
			return nil, err
		}

		pi, err := placement.NewInstanceFromProto(&p)
		if err != nil {
			return nil, err
		}

		r[i] = pi
	}
	return r, nil
}

func (c *client) Delete(instance string) error {
	ctx, cancel := c.context()
	defer cancel()

	r, err := c.kv.Delete(ctx, heartbeatKey(c.sid, instance))
	if err != nil {
		return err
	}

	if r.Deleted == 0 {
		return fmt.Errorf("could not find heartbeat for service: %s, env: %s, instance: %s", c.sid.Name(), c.sid.Environment(), instance)
	}

	// NB(cw) we need to clean up cached lease ID, if not the next heartbeat might reuse the cached lease
	// and keep alive on existing lease wont work since the key is deleted
	c.cache.delete(c.sid, instance)
	return nil
}

func (c *client) Watch() (watch.Watch, error) {
	serviceKey := servicePrefix(c.sid)

	c.Lock()
	watchable, ok := c.watchables[serviceKey]
	if !ok {
		watchable = watch.NewWatchable()
		c.watchables[serviceKey] = watchable

		go c.wm.Watch(serviceKey)
	}
	c.Unlock()

	_, w, err := watchable.Watch()
	return w, err
}

func (c *client) update(key string, _ []*clientv3.Event) error {
	var (
		newValue []string
		err      error
	)
	// we need retry here because if Get() failed on an watch update,
	// it has to wait 10 mins to be notified to try again
	if execErr := c.retrier.Attempt(func() error {
		newValue, err = c.get(key)
		if err == kv.ErrNotFound {
			// do not retry on ErrNotFound
			return retry.NonRetryableError(err)
		}
		return err
	}); execErr != nil {
		return execErr
	}

	c.RLock()
	w, ok := c.watchables[key]
	c.RUnlock()
	if !ok {
		return fmt.Errorf("unexpected: no watchable found for key: %s", key)
	}
	w.Update(newValue)

	return nil
}

func (c *client) tickAndStop(key string) bool {
	// fast path
	c.RLock()
	watchable, ok := c.watchables[key]
	c.RUnlock()
	if !ok {
		c.logger.Warn("unexpected: key is already cleaned up", zap.String("key", key))
		return true
	}

	if watchable.NumWatches() != 0 {
		return false
	}

	// slow path
	c.Lock()
	defer c.Unlock()
	watchable, ok = c.watchables[key]
	if !ok {
		// not expect this to happen
		c.logger.Warn("unexpected: key is already cleaned up", zap.String("key", key))
		return true
	}

	if watchable.NumWatches() != 0 {
		// a new watch has subscribed to the watchable, do not clean up
		return false
	}

	watchable.Close()
	delete(c.watchables, key)
	return true
}

func (c *client) context() (context.Context, context.CancelFunc) {
	ctx := context.Background()
	cancel := noopCancel
	if c.opts.RequestTimeout() > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.opts.RequestTimeout())
	}

	return ctx, cancel
}

func heartbeatKey(sid services.ServiceID, instance string) string {
	return fmt.Sprintf(keyFormat, servicePrefix(sid), instance)
}

func instanceFromKey(key, servicePrefix string) string {
	return strings.TrimPrefix(
		strings.TrimPrefix(key, servicePrefix),
		keySeparator,
	)
}

// heartbeats for a service "svc" in env "test" should be stored under
// "_hb/test/svc". A service "svc" with no environment will be stored under
// "_hb/svc".
func servicePrefix(sid services.ServiceID) string {
	env := sid.Environment()
	if env == "" {
		return fmt.Sprintf(keyFormat, heartbeatKeyPrefix, sid.Name())
	}

	return fmt.Sprintf(
		keyFormat,
		heartbeatKeyPrefix,
		fmt.Sprintf(keyFormat, env, sid.Name()))
}

func newLeaseCache() *leaseCache {
	return &leaseCache{
		leases: make(map[string]map[time.Duration]clientv3.LeaseID),
	}
}

type leaseCache struct {
	sync.RWMutex

	leases map[string]map[time.Duration]clientv3.LeaseID
}

func (c *leaseCache) get(sid services.ServiceID, instance string, ttl time.Duration) (clientv3.LeaseID, bool) {
	c.RLock()
	defer c.RUnlock()

	leases, ok := c.leases[heartbeatKey(sid, instance)]
	if !ok {
		return clientv3.LeaseID(0), false
	}

	id, ok := leases[ttl]
	return id, ok
}

func (c *leaseCache) put(sid services.ServiceID, instance string, ttl time.Duration, id clientv3.LeaseID) {
	key := heartbeatKey(sid, instance)

	c.Lock()
	defer c.Unlock()

	leases, ok := c.leases[key]
	if !ok {
		leases = make(map[time.Duration]clientv3.LeaseID)
		c.leases[key] = leases
	}
	leases[ttl] = id
}

func (c *leaseCache) delete(sid services.ServiceID, instance string) {
	c.Lock()
	delete(c.leases, heartbeatKey(sid, instance))
	c.Unlock()
}
