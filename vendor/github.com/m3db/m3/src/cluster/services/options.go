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

package services

import (
	"errors"
	"os"
	"time"

	"github.com/m3db/m3/src/x/instrument"
)

const (
	defaultInitTimeout   = 5 * time.Second
	defaultLeaderTimeout = 10 * time.Second
	defaultResignTimeout = 10 * time.Second
)

var (
	errNoKVGen            = errors.New("no KVGen function set")
	errNoHeartbeatGen     = errors.New("no HeartbeatGen function set")
	errNoLeaderGen        = errors.New("no LeaderGen function set")
	errInvalidInitTimeout = errors.New("negative init timeout for service watch")
)

type options struct {
	initTimeout time.Duration
	nOpts       NamespaceOptions
	kvGen       KVGen
	hbGen       HeartbeatGen
	ldGen       LeaderGen
	iopts       instrument.Options
}

// NewOptions creates an Option
func NewOptions() Options {
	return options{
		iopts:       instrument.NewOptions(),
		nOpts:       NewNamespaceOptions(),
		initTimeout: defaultInitTimeout,
	}
}

func (o options) Validate() error {
	if o.kvGen == nil {
		return errNoKVGen
	}

	if o.hbGen == nil {
		return errNoHeartbeatGen
	}

	if o.ldGen == nil {
		return errNoLeaderGen
	}

	if o.initTimeout < 0 {
		return errInvalidInitTimeout
	}

	return nil
}

func (o options) InitTimeout() time.Duration {
	return o.initTimeout
}

func (o options) SetInitTimeout(t time.Duration) Options {
	o.initTimeout = t
	return o
}

func (o options) KVGen() KVGen {
	return o.kvGen
}

func (o options) SetKVGen(gen KVGen) Options {
	o.kvGen = gen
	return o
}

func (o options) HeartbeatGen() HeartbeatGen {
	return o.hbGen
}

func (o options) SetHeartbeatGen(gen HeartbeatGen) Options {
	o.hbGen = gen
	return o
}

func (o options) LeaderGen() LeaderGen {
	return o.ldGen
}

func (o options) SetLeaderGen(lg LeaderGen) Options {
	o.ldGen = lg
	return o
}

func (o options) InstrumentsOptions() instrument.Options {
	return o.iopts
}

func (o options) SetInstrumentsOptions(iopts instrument.Options) Options {
	o.iopts = iopts
	return o
}

func (o options) NamespaceOptions() NamespaceOptions {
	return o.nOpts
}

func (o options) SetNamespaceOptions(opts NamespaceOptions) Options {
	o.nOpts = opts
	return o
}

// NewElectionOptions returns an empty ElectionOptions.
func NewElectionOptions() ElectionOptions {
	eo := electionOpts{
		leaderTimeout: defaultLeaderTimeout,
		resignTimeout: defaultResignTimeout,
	}

	return eo
}

type electionOpts struct {
	leaderTimeout time.Duration
	resignTimeout time.Duration
	ttlSecs       int
}

func (e electionOpts) LeaderTimeout() time.Duration {
	return e.leaderTimeout
}

func (e electionOpts) SetLeaderTimeout(t time.Duration) ElectionOptions {
	e.leaderTimeout = t
	return e
}

func (e electionOpts) ResignTimeout() time.Duration {
	return e.resignTimeout
}

func (e electionOpts) SetResignTimeout(t time.Duration) ElectionOptions {
	e.resignTimeout = t
	return e
}

func (e electionOpts) TTLSecs() int {
	return e.ttlSecs
}

func (e electionOpts) SetTTLSecs(ttl int) ElectionOptions {
	e.ttlSecs = ttl
	return e
}

type campaignOpts struct {
	val string
}

// NewCampaignOptions returns an empty CampaignOptions.
func NewCampaignOptions() (CampaignOptions, error) {
	h, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	return campaignOpts{val: h}, nil
}

func (c campaignOpts) LeaderValue() string {
	return c.val
}

func (c campaignOpts) SetLeaderValue(v string) CampaignOptions {
	c.val = v
	return c
}

type overrideOptions struct {
	namespaceOpts NamespaceOptions
}

// NewOverrideOptions constructs a new OverrideOptions.
func NewOverrideOptions() OverrideOptions {
	return &overrideOptions{
		namespaceOpts: NewNamespaceOptions(),
	}
}

func (o overrideOptions) NamespaceOptions() NamespaceOptions {
	return o.namespaceOpts
}

func (o overrideOptions) SetNamespaceOptions(opts NamespaceOptions) OverrideOptions {
	o.namespaceOpts = opts
	return o
}

type namespaceOpts struct {
	placement string
	metadata  string
}

// NewNamespaceOptions constructs a new NamespaceOptions.
func NewNamespaceOptions() NamespaceOptions {
	return &namespaceOpts{}
}

func (opts namespaceOpts) PlacementNamespace() string {
	return opts.placement
}

func (opts namespaceOpts) SetPlacementNamespace(v string) NamespaceOptions {
	opts.placement = v
	return opts
}

func (opts namespaceOpts) MetadataNamespace() string {
	return opts.metadata
}

func (opts namespaceOpts) SetMetadataNamespace(v string) NamespaceOptions {
	opts.metadata = v
	return opts
}

// NewQueryOptions creates new QueryOptions.
func NewQueryOptions() QueryOptions { return new(queryOptions) }

type queryOptions struct {
	includeUnhealthy bool
}

func (qo *queryOptions) IncludeUnhealthy() bool                  { return qo.includeUnhealthy }
func (qo *queryOptions) SetIncludeUnhealthy(h bool) QueryOptions { qo.includeUnhealthy = h; return qo }
