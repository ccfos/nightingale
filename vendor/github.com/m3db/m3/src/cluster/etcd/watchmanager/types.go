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
	"time"

	"github.com/m3db/m3/src/x/instrument"

	"go.etcd.io/etcd/clientv3"
)

// WatchManager manages etcd watch on a key
type WatchManager interface {
	// Watch watches the key forever until the CheckAndStopFn returns true
	Watch(key string)
}

// UpdateFn is called when an event on the watch channel happens
type UpdateFn func(key string, events []*clientv3.Event) error

// TickAndStopFn is called every once a while
// to check and stop the watch if needed
type TickAndStopFn func(key string) bool

// Options are options for the etcd watch helper
type Options interface {
	// Watcher is the etcd watcher
	Watcher() clientv3.Watcher
	// SetWatcher sets the Watcher
	SetWatcher(w clientv3.Watcher) Options

	// UpdateFn is the function called when a notification on a key is received
	UpdateFn() UpdateFn
	// SetUpdateFn sets the UpdateFn
	SetUpdateFn(f UpdateFn) Options

	// TickAndStopFn is the function called periodically to check if a watch should be stopped
	TickAndStopFn() TickAndStopFn
	// SetTickAndStopFn sets the TickAndStopFn
	SetTickAndStopFn(f TickAndStopFn) Options

	// WatchOptions is a set of options for the etcd watch
	WatchOptions() []clientv3.OpOption
	// SetWatchOptions sets the WatchOptions
	SetWatchOptions(opts []clientv3.OpOption) Options

	// WatchChanCheckInterval will be used to periodically check if a watch chan
	// is no longer being subscribed and should be closed
	WatchChanCheckInterval() time.Duration
	// SetWatchChanCheckInterval sets the WatchChanCheckInterval
	SetWatchChanCheckInterval(t time.Duration) Options

	// WatchChanResetInterval is the delay before resetting the etcd watch chan
	WatchChanResetInterval() time.Duration
	// SetWatchChanResetInterval sets the WatchChanResetInterval
	SetWatchChanResetInterval(t time.Duration) Options

	// WatchChanInitTimeout is the timeout for a watchChan initialization
	WatchChanInitTimeout() time.Duration
	// SetWatchChanInitTimeout sets the WatchChanInitTimeout
	SetWatchChanInitTimeout(t time.Duration) Options

	// InstrumentsOptions is the instrument options
	InstrumentsOptions() instrument.Options
	// SetInstrumentsOptions sets the InstrumentsOptions
	SetInstrumentsOptions(iopts instrument.Options) Options

	// Validate validates the options
	Validate() error
}
