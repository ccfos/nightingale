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

package client

import (
	"github.com/m3db/m3/src/cluster/kv"
	"github.com/m3db/m3/src/cluster/services"
)

// Client is the base interface into the cluster management system, providing
// access to cluster services.
type Client interface {
	// Services returns access to the set of services.
	Services(opts services.OverrideOptions) (services.Services, error)

	// KV returns access to the distributed configuration store.
	// To be deprecated.
	KV() (kv.Store, error)

	// Txn returns access to the transaction store.
	// To be deprecated.
	Txn() (kv.TxnStore, error)

	// Store returns access to the distributed configuration store with a namespace.
	Store(opts kv.OverrideOptions) (kv.Store, error)

	// TxnStore returns access to the transaction store with a namespace.
	TxnStore(opts kv.OverrideOptions) (kv.TxnStore, error)
}
