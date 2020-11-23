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

package namespace

import (
	"errors"
	"time"

	"github.com/m3db/m3/src/cluster/client"
	"github.com/m3db/m3/src/x/instrument"
)

const (
	defaultInitTimeout   = 30 * time.Second
	defaultNsRegistryKey = "m3db.node.namespace_registry"
)

var (
	errInitTimeoutPositive = errors.New("init timeout must be positive")
	errNsRegistryKeyEmpty  = errors.New("namespace registry key must not be empty")
	errCsClientNotSet      = errors.New("config service client not set")
)

type dynamicOpts struct {
	iopts                  instrument.Options
	csClient               client.Client
	nsRegistryKey          string
	initTimeout            time.Duration
	forceColdWritesEnabled bool
}

// NewDynamicOptions creates a new DynamicOptions
func NewDynamicOptions() DynamicOptions {
	return &dynamicOpts{
		iopts:         instrument.NewOptions(),
		nsRegistryKey: defaultNsRegistryKey,
		initTimeout:   defaultInitTimeout,
	}
}

func (o *dynamicOpts) Validate() error {
	if o.initTimeout <= 0 {
		return errInitTimeoutPositive
	}
	if o.nsRegistryKey == "" {
		return errNsRegistryKeyEmpty
	}
	if o.csClient == nil {
		return errCsClientNotSet
	}
	return nil
}

func (o *dynamicOpts) SetInstrumentOptions(value instrument.Options) DynamicOptions {
	opts := *o
	opts.iopts = value
	return &opts
}

func (o *dynamicOpts) InstrumentOptions() instrument.Options {
	return o.iopts
}

func (o *dynamicOpts) SetConfigServiceClient(c client.Client) DynamicOptions {
	opts := *o
	opts.csClient = c
	return &opts
}

func (o *dynamicOpts) ConfigServiceClient() client.Client {
	return o.csClient
}

func (o *dynamicOpts) SetNamespaceRegistryKey(k string) DynamicOptions {
	opts := *o
	opts.nsRegistryKey = k
	return &opts
}

func (o *dynamicOpts) NamespaceRegistryKey() string {
	return o.nsRegistryKey
}

func (o *dynamicOpts) SetForceColdWritesEnabled(enabled bool) DynamicOptions {
	opts := *o
	opts.forceColdWritesEnabled = enabled
	return &opts
}

func (o *dynamicOpts) ForceColdWritesEnabled() bool {
	return o.forceColdWritesEnabled
}
