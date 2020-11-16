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

package etcd

import (
	"os"
	"time"

	"github.com/m3db/m3/src/cluster/client"
	"github.com/m3db/m3/src/cluster/services"
	"github.com/m3db/m3/src/x/instrument"
)

// ClusterConfig is the config for a zoned etcd cluster.
type ClusterConfig struct {
	Zone             string           `yaml:"zone"`
	Endpoints        []string         `yaml:"endpoints"`
	KeepAlive        *KeepAliveConfig `yaml:"keepAlive"`
	TLS              *TLSConfig       `yaml:"tls"`
	AutoSyncInterval time.Duration    `yaml:"autoSyncInterval"`
}

// NewCluster creates a new Cluster.
func (c ClusterConfig) NewCluster() Cluster {
	keepAliveOpts := NewKeepAliveOptions()
	if c.KeepAlive != nil {
		keepAliveOpts = c.KeepAlive.NewOptions()
	}
	return NewCluster().
		SetZone(c.Zone).
		SetEndpoints(c.Endpoints).
		SetKeepAliveOptions(keepAliveOpts).
		SetTLSOptions(c.TLS.newOptions()).
		SetAutoSyncInterval(c.AutoSyncInterval)
}

// TLSConfig is the config for TLS.
type TLSConfig struct {
	CrtPath   string `yaml:"crtPath"`
	CACrtPath string `yaml:"caCrtPath"`
	KeyPath   string `yaml:"keyPath"`
}

func (c *TLSConfig) newOptions() TLSOptions {
	opts := NewTLSOptions()
	if c == nil {
		return opts
	}

	return opts.
		SetCrtPath(c.CrtPath).
		SetKeyPath(c.KeyPath).
		SetCACrtPath(c.CACrtPath)
}

// KeepAliveConfig configures keepAlive behavior.
type KeepAliveConfig struct {
	Enabled bool          `yaml:"enabled"`
	Period  time.Duration `yaml:"period"`
	Jitter  time.Duration `yaml:"jitter"`
	Timeout time.Duration `yaml:"timeout"`
}

// NewOptions constructs options based on the config.
func (c *KeepAliveConfig) NewOptions() KeepAliveOptions {
	return NewKeepAliveOptions().
		SetKeepAliveEnabled(c.Enabled).
		SetKeepAlivePeriod(c.Period).
		SetKeepAlivePeriodMaxJitter(c.Jitter).
		SetKeepAliveTimeout(c.Timeout)
}

// Configuration is for config service client.
type Configuration struct {
	Zone              string                 `yaml:"zone"`
	Env               string                 `yaml:"env"`
	Service           string                 `yaml:"service" validate:"nonzero"`
	CacheDir          string                 `yaml:"cacheDir"`
	ETCDClusters      []ClusterConfig        `yaml:"etcdClusters"`
	SDConfig          services.Configuration `yaml:"m3sd"`
	WatchWithRevision int64                  `yaml:"watchWithRevision"`
	NewDirectoryMode  *os.FileMode           `yaml:"newDirectoryMode"`
}

// NewClient creates a new config service client.
func (cfg Configuration) NewClient(iopts instrument.Options) (client.Client, error) {
	return NewConfigServiceClient(cfg.NewOptions().SetInstrumentOptions(iopts))
}

// NewOptions returns a new Options.
func (cfg Configuration) NewOptions() Options {
	opts := NewOptions().
		SetZone(cfg.Zone).
		SetEnv(cfg.Env).
		SetService(cfg.Service).
		SetCacheDir(cfg.CacheDir).
		SetClusters(cfg.etcdClusters()).
		SetServicesOptions(cfg.SDConfig.NewOptions()).
		SetWatchWithRevision(cfg.WatchWithRevision)

	if v := cfg.NewDirectoryMode; v != nil {
		opts = opts.SetNewDirectoryMode(*v)
	} else {
		opts = opts.SetNewDirectoryMode(defaultDirectoryMode)
	}

	return opts
}

func (cfg Configuration) etcdClusters() []Cluster {
	res := make([]Cluster, len(cfg.ETCDClusters))
	for i, c := range cfg.ETCDClusters {
		res[i] = c.NewCluster()
	}

	return res
}
