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

package watchmanager

import (
	"context"
	"fmt"
	"time"

	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/uber-go/tally"
	"go.uber.org/zap"
)

// NewWatchManager creates a new watch manager
func NewWatchManager(opts Options) (WatchManager, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	scope := opts.InstrumentsOptions().MetricsScope()
	return &manager{
		opts:   opts,
		logger: opts.InstrumentsOptions().Logger(),
		m: metrics{
			etcdWatchCreate: scope.Counter("etcd-watch-create"),
			etcdWatchError:  scope.Counter("etcd-watch-error"),
			etcdWatchReset:  scope.Counter("etcd-watch-reset"),
		},
		updateFn:      opts.UpdateFn(),
		tickAndStopFn: opts.TickAndStopFn(),
	}, nil
}

type manager struct {
	opts   Options
	logger *zap.Logger
	m      metrics

	updateFn      UpdateFn
	tickAndStopFn TickAndStopFn
}

type metrics struct {
	etcdWatchCreate tally.Counter
	etcdWatchError  tally.Counter
	etcdWatchReset  tally.Counter
}

func (w *manager) watchChanWithTimeout(key string, rev int64) (clientv3.WatchChan, context.CancelFunc, error) {
	doneCh := make(chan struct{})

	ctx, cancelFn := context.WithCancel(clientv3.WithRequireLeader(context.Background()))

	var watchChan clientv3.WatchChan
	go func() {
		wOpts := w.opts.WatchOptions()
		if rev > 0 {
			wOpts = append(wOpts, clientv3.WithRev(rev))
		}
		watchChan = w.opts.Watcher().Watch(
			ctx,
			key,
			wOpts...,
		)
		close(doneCh)
	}()

	timeout := w.opts.WatchChanInitTimeout()
	select {
	case <-doneCh:
		return watchChan, cancelFn, nil
	case <-time.After(timeout):
		cancelFn()
		return nil, nil, fmt.Errorf("etcd watch create timed out after %s for key: %s", timeout.String(), key)
	}
}

func (w *manager) Watch(key string) {
	ticker := time.Tick(w.opts.WatchChanCheckInterval()) //nolint: megacheck

	var (
		revOverride int64
		watchChan   clientv3.WatchChan
		cancelFn    context.CancelFunc
		err         error
	)
	for {
		if watchChan == nil {
			w.m.etcdWatchCreate.Inc(1)
			watchChan, cancelFn, err = w.watchChanWithTimeout(key, revOverride)
			if err != nil {
				w.logger.Error("could not create etcd watch", zap.Error(err))

				// NB(cw) when we failed to create a etcd watch channel
				// we do a get for now and will try to recreate the watch chan later
				if err = w.updateFn(key, nil); err != nil {
					w.logger.Error("failed to get value for key", zap.String("key", key), zap.Error(err))
				}
				// avoid recreating watch channel too frequently
				time.Sleep(w.opts.WatchChanResetInterval())
				continue
			}
		}

		select {
		case r, ok := <-watchChan:
			if !ok {
				// the watch chan is closed, set it to nil so it will be recreated
				// this is unlikely to happen but just to be defensive
				cancelFn()
				watchChan = nil
				w.logger.Warn("etcd watch channel closed on key, recreating a watch channel",
					zap.String("key", key))

				// avoid recreating watch channel too frequently
				time.Sleep(w.opts.WatchChanResetInterval())
				w.m.etcdWatchReset.Inc(1)

				continue
			}

			// handle the update
			if err = r.Err(); err != nil {
				w.logger.Error("received error on watch channel", zap.Error(err))
				w.m.etcdWatchError.Inc(1)
				// do not stop here, even though the update contains an error
				// we still take this chance to attempt a Get() for the latest value

				// If the current revision has been compacted, set watchChan to
				// nil so the watch is recreated with a valid start revision
				if err == rpctypes.ErrCompacted {
					w.logger.Warn("recreating watch at revision", zap.Int64("revision", r.CompactRevision))
					revOverride = r.CompactRevision
					watchChan = nil
				}
			}

			if r.IsProgressNotify() {
				// Do not call updateFn on ProgressNotify as it happens periodically with no update events
				continue
			}
			if err = w.updateFn(key, r.Events); err != nil {
				w.logger.Error("received notification for key, but failed to get value",
					zap.String("key", key), zap.Error(err))
			}
		case <-ticker:
			if w.tickAndStopFn(key) {
				w.logger.Info("watch on key ended", zap.String("key", key))
				return
			}
		}
	}
}
