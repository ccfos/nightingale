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

package runtime

import (
	"errors"
	"time"

	"github.com/m3db/m3/src/dbnode/ratelimit"
	"github.com/m3db/m3/src/dbnode/topology"
)

const (
	// DefaultWriteConsistencyLevel is the default write consistency level
	DefaultWriteConsistencyLevel = topology.ConsistencyLevelMajority

	// DefaultReadConsistencyLevel is the default read consistency level
	DefaultReadConsistencyLevel = topology.ReadConsistencyLevelMajority

	// DefaultBootstrapConsistencyLevel is the default bootstrap consistency level
	DefaultBootstrapConsistencyLevel = topology.ReadConsistencyLevelMajority

	// DefaultIndexDefaultQueryTimeout is the hard timeout value to use if none is
	// specified for a specific query, zero specifies no timeout.
	DefaultIndexDefaultQueryTimeout = time.Minute

	defaultWriteNewSeriesAsync                  = false
	defaultWriteNewSeriesBackoffDuration        = time.Duration(0)
	defaultWriteNewSeriesLimitPerShardPerSecond = 0
	defaultTickSeriesBatchSize                  = 512
	defaultTickPerSeriesSleepDuration           = 100 * time.Microsecond
	defaultTickMinimumInterval                  = 10 * time.Second
	defaultMaxWiredBlocks                       = uint(1 << 16) // 65,536
)

var (
	errWriteNewSeriesBackoffDurationIsNegative = errors.New(
		"write new series backoff duration cannot be negative")
	errWriteNewSeriesLimitPerShardPerSecondIsNegative = errors.New(
		"write new series limit per shard per cannot be negative")
	errTickSeriesBatchSizeMustBePositive = errors.New(
		"tick series batch size must be positive")
	errTickPerSeriesSleepDurationMustBePositive = errors.New(
		"tick per series sleep duration must be positive")
)

type options struct {
	persistRateLimitOpts                 ratelimit.Options
	writeNewSeriesAsync                  bool
	writeNewSeriesBackoffDuration        time.Duration
	writeNewSeriesLimitPerShardPerSecond int
	encodersPerBlockLimit                int
	tickSeriesBatchSize                  int
	tickPerSeriesSleepDuration           time.Duration
	tickMinimumInterval                  time.Duration
	maxWiredBlocks                       uint
	clientBootstrapConsistencyLevel      topology.ReadConsistencyLevel
	clientReadConsistencyLevel           topology.ReadConsistencyLevel
	clientWriteConsistencyLevel          topology.ConsistencyLevel
	indexDefaultQueryTimeout             time.Duration
}

// NewOptions creates a new set of runtime options with defaults
func NewOptions() Options {
	return &options{
		persistRateLimitOpts:                 ratelimit.NewOptions(),
		writeNewSeriesAsync:                  defaultWriteNewSeriesAsync,
		writeNewSeriesBackoffDuration:        defaultWriteNewSeriesBackoffDuration,
		writeNewSeriesLimitPerShardPerSecond: defaultWriteNewSeriesLimitPerShardPerSecond,
		tickSeriesBatchSize:                  defaultTickSeriesBatchSize,
		tickPerSeriesSleepDuration:           defaultTickPerSeriesSleepDuration,
		tickMinimumInterval:                  defaultTickMinimumInterval,
		maxWiredBlocks:                       defaultMaxWiredBlocks,
		clientBootstrapConsistencyLevel:      DefaultBootstrapConsistencyLevel,
		clientReadConsistencyLevel:           DefaultReadConsistencyLevel,
		clientWriteConsistencyLevel:          DefaultWriteConsistencyLevel,
		indexDefaultQueryTimeout:             DefaultIndexDefaultQueryTimeout,
	}
}

func (o *options) Validate() error {
	// writeNewSeriesBackoffDuration can be zero to specify no backoff
	if o.writeNewSeriesBackoffDuration < 0 {
		return errWriteNewSeriesBackoffDurationIsNegative
	}

	// writeNewSeriesLimitPerShardPerSecond can be zero to specify that
	// no limit should be enforced
	if o.writeNewSeriesLimitPerShardPerSecond < 0 {
		return errWriteNewSeriesLimitPerShardPerSecondIsNegative
	}

	if !(o.tickSeriesBatchSize > 0) {
		return errTickSeriesBatchSizeMustBePositive
	}

	if !(o.tickPerSeriesSleepDuration > 0) {
		return errTickPerSeriesSleepDurationMustBePositive
	}

	// tickMinimumInterval can be zero if user desires

	return nil
}

func (o *options) SetPersistRateLimitOptions(value ratelimit.Options) Options {
	opts := *o
	opts.persistRateLimitOpts = value
	return &opts
}

func (o *options) PersistRateLimitOptions() ratelimit.Options {
	return o.persistRateLimitOpts
}

func (o *options) SetWriteNewSeriesAsync(value bool) Options {
	opts := *o
	opts.writeNewSeriesAsync = value
	return &opts
}

func (o *options) WriteNewSeriesAsync() bool {
	return o.writeNewSeriesAsync
}

func (o *options) SetWriteNewSeriesBackoffDuration(value time.Duration) Options {
	opts := *o
	opts.writeNewSeriesBackoffDuration = value
	return &opts
}

func (o *options) WriteNewSeriesBackoffDuration() time.Duration {
	return o.writeNewSeriesBackoffDuration
}

func (o *options) SetWriteNewSeriesLimitPerShardPerSecond(value int) Options {
	opts := *o
	opts.writeNewSeriesLimitPerShardPerSecond = value
	return &opts
}

func (o *options) WriteNewSeriesLimitPerShardPerSecond() int {
	return o.writeNewSeriesLimitPerShardPerSecond
}

func (o *options) SetEncodersPerBlockLimit(value int) Options {
	opts := *o
	opts.encodersPerBlockLimit = value
	return &opts
}

func (o *options) EncodersPerBlockLimit() int {
	return o.encodersPerBlockLimit
}

func (o *options) SetTickSeriesBatchSize(value int) Options {
	opts := *o
	opts.tickSeriesBatchSize = value
	return &opts
}

func (o *options) TickSeriesBatchSize() int {
	return o.tickSeriesBatchSize
}

func (o *options) SetTickPerSeriesSleepDuration(value time.Duration) Options {
	opts := *o
	opts.tickPerSeriesSleepDuration = value
	return &opts
}

func (o *options) TickPerSeriesSleepDuration() time.Duration {
	return o.tickPerSeriesSleepDuration
}

func (o *options) SetTickMinimumInterval(value time.Duration) Options {
	opts := *o
	opts.tickMinimumInterval = value
	return &opts
}

func (o *options) TickMinimumInterval() time.Duration {
	return o.tickMinimumInterval
}

func (o *options) SetMaxWiredBlocks(value uint) Options {
	opts := *o
	opts.maxWiredBlocks = value
	return &opts
}

func (o *options) MaxWiredBlocks() uint {
	return o.maxWiredBlocks
}

func (o *options) SetClientBootstrapConsistencyLevel(value topology.ReadConsistencyLevel) Options {
	opts := *o
	opts.clientBootstrapConsistencyLevel = value
	return &opts
}

func (o *options) ClientBootstrapConsistencyLevel() topology.ReadConsistencyLevel {
	return o.clientBootstrapConsistencyLevel
}

func (o *options) SetClientReadConsistencyLevel(value topology.ReadConsistencyLevel) Options {
	opts := *o
	opts.clientReadConsistencyLevel = value
	return &opts
}

func (o *options) ClientReadConsistencyLevel() topology.ReadConsistencyLevel {
	return o.clientReadConsistencyLevel
}

func (o *options) SetClientWriteConsistencyLevel(value topology.ConsistencyLevel) Options {
	opts := *o
	opts.clientWriteConsistencyLevel = value
	return &opts
}

func (o *options) ClientWriteConsistencyLevel() topology.ConsistencyLevel {
	return o.clientWriteConsistencyLevel
}

func (o *options) SetIndexDefaultQueryTimeout(value time.Duration) Options {
	opts := *o
	opts.indexDefaultQueryTimeout = value
	return &opts
}

func (o *options) IndexDefaultQueryTimeout() time.Duration {
	return o.indexDefaultQueryTimeout
}
