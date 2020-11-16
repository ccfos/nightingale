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

package placement

import (
	"time"

	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/x/clock"
	"github.com/m3db/m3/src/x/instrument"
)

const (
	defaultInitWatchTimeout = 10 * time.Second
)

type stagedPlacementWatcherOptions struct {
	clockOpts            clock.Options
	instrumentOpts       instrument.Options
	stagedPlacementOpts  ActiveStagedPlacementOptions
	stagedPlacementKey   string
	stagedPlacementStore kv.Store
	initWatchTimeout     time.Duration
}

// NewStagedPlacementWatcherOptions create a new set of topology options.
func NewStagedPlacementWatcherOptions() StagedPlacementWatcherOptions {
	return &stagedPlacementWatcherOptions{
		clockOpts:           clock.NewOptions(),
		instrumentOpts:      instrument.NewOptions(),
		stagedPlacementOpts: NewActiveStagedPlacementOptions(),
		initWatchTimeout:    defaultInitWatchTimeout,
	}
}

func (o *stagedPlacementWatcherOptions) SetClockOptions(value clock.Options) StagedPlacementWatcherOptions {
	opts := *o
	opts.clockOpts = value
	return &opts
}

func (o *stagedPlacementWatcherOptions) ClockOptions() clock.Options {
	return o.clockOpts
}

func (o *stagedPlacementWatcherOptions) SetInstrumentOptions(value instrument.Options) StagedPlacementWatcherOptions {
	opts := *o
	opts.instrumentOpts = value
	return &opts
}

func (o *stagedPlacementWatcherOptions) InstrumentOptions() instrument.Options {
	return o.instrumentOpts
}

func (o *stagedPlacementWatcherOptions) SetActiveStagedPlacementOptions(value ActiveStagedPlacementOptions) StagedPlacementWatcherOptions {
	opts := *o
	opts.stagedPlacementOpts = value
	return &opts
}

func (o *stagedPlacementWatcherOptions) ActiveStagedPlacementOptions() ActiveStagedPlacementOptions {
	return o.stagedPlacementOpts
}

func (o *stagedPlacementWatcherOptions) SetStagedPlacementKey(value string) StagedPlacementWatcherOptions {
	opts := *o
	opts.stagedPlacementKey = value
	return &opts
}

func (o *stagedPlacementWatcherOptions) StagedPlacementKey() string {
	return o.stagedPlacementKey
}

func (o *stagedPlacementWatcherOptions) SetStagedPlacementStore(value kv.Store) StagedPlacementWatcherOptions {
	opts := *o
	opts.stagedPlacementStore = value
	return &opts
}

func (o *stagedPlacementWatcherOptions) StagedPlacementStore() kv.Store {
	return o.stagedPlacementStore
}

func (o *stagedPlacementWatcherOptions) SetInitWatchTimeout(value time.Duration) StagedPlacementWatcherOptions {
	opts := *o
	opts.initWatchTimeout = value
	return &opts
}

func (o *stagedPlacementWatcherOptions) InitWatchTimeout() time.Duration {
	return o.initWatchTimeout
}
