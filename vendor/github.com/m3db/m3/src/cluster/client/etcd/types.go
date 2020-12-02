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
	"crypto/tls"
	"os"
	"time"

	"github.com/m3db/m3/src/cluster/services"
	"github.com/m3db/m3/src/x/instrument"
	"github.com/m3db/m3/src/x/retry"
)

// Options is the Options to create a config service client.
type Options interface {
	Env() string
	SetEnv(e string) Options

	Zone() string
	SetZone(z string) Options

	Service() string
	SetService(id string) Options

	CacheDir() string
	SetCacheDir(dir string) Options

	ServicesOptions() services.Options
	SetServicesOptions(opts services.Options) Options

	Clusters() []Cluster
	SetClusters(clusters []Cluster) Options
	ClusterForZone(z string) (Cluster, bool)

	InstrumentOptions() instrument.Options
	SetInstrumentOptions(iopts instrument.Options) Options

	RetryOptions() retry.Options
	SetRetryOptions(retryOpts retry.Options) Options

	WatchWithRevision() int64
	SetWatchWithRevision(rev int64) Options

	SetNewDirectoryMode(fm os.FileMode) Options
	NewDirectoryMode() os.FileMode

	Validate() error
}

// KeepAliveOptions provide a set of client-side keepAlive options.
type KeepAliveOptions interface {
	// KeepAliveEnabled determines whether keepAlives are enabled.
	KeepAliveEnabled() bool

	// SetKeepAliveEnabled sets whether keepAlives are enabled.
	SetKeepAliveEnabled(value bool) KeepAliveOptions

	// KeepAlivePeriod is the duration of time after which if the client doesn't see
	// any activity the client pings the server to see if transport is alive.
	KeepAlivePeriod() time.Duration

	// SetKeepAlivePeriod sets the duration of time after which if the client doesn't see
	// any activity the client pings the server to see if transport is alive.
	SetKeepAlivePeriod(value time.Duration) KeepAliveOptions

	// KeepAlivePeriodMaxJitter is used to add some jittering to keep alive period
	// to avoid a large number of clients all sending keepalive probes at the
	// same time.
	KeepAlivePeriodMaxJitter() time.Duration

	// SetKeepAlivePeriodMaxJitter sets the maximum jittering to keep alive period
	// to avoid a large number of clients all sending keepalive probes at the
	// same time.
	SetKeepAlivePeriodMaxJitter(value time.Duration) KeepAliveOptions

	// KeepAliveTimeout is the time that the client waits for a response for the
	// keep-alive probe. If the response is not received in this time, the connection is closed.
	KeepAliveTimeout() time.Duration

	// SetKeepAliveTimeout sets the time that the client waits for a response for the
	// keep-alive probe. If the response is not received in this time, the connection is closed.
	SetKeepAliveTimeout(value time.Duration) KeepAliveOptions
}

// TLSOptions defines the options for TLS.
type TLSOptions interface {
	CrtPath() string
	SetCrtPath(string) TLSOptions

	KeyPath() string
	SetKeyPath(string) TLSOptions

	CACrtPath() string
	SetCACrtPath(string) TLSOptions

	Config() (*tls.Config, error)
}

// Cluster defines the configuration for a etcd cluster.
type Cluster interface {
	Zone() string
	SetZone(z string) Cluster

	Endpoints() []string
	SetEndpoints(endpoints []string) Cluster

	KeepAliveOptions() KeepAliveOptions
	SetKeepAliveOptions(value KeepAliveOptions) Cluster

	TLSOptions() TLSOptions
	SetTLSOptions(TLSOptions) Cluster

	SetAutoSyncInterval(value time.Duration) Cluster
	AutoSyncInterval() time.Duration
}
