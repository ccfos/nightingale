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

package services

import (
	"time"
)

// OverrideConfiguration configs the override options.
type OverrideConfiguration struct {
	Namespaces NamespacesConfiguration `yaml:"namespaces"`
}

// NewOptions creates a new override options.
func (cfg OverrideConfiguration) NewOptions() OverrideOptions {
	return NewOverrideOptions().
		SetNamespaceOptions(cfg.Namespaces.NewOptions())
}

// NamespacesConfiguration configs the NamespaceOptions.
type NamespacesConfiguration struct {
	Placement string `yaml:"placement"`
	Metadata  string `yaml:"metadata"`
}

// NewOptions creates a new NamespaceOptions.
func (cfg NamespacesConfiguration) NewOptions() NamespaceOptions {
	return NewNamespaceOptions().
		SetPlacementNamespace(cfg.Placement).
		SetMetadataNamespace(cfg.Metadata)
}

// Configuration is the config for service options.
type Configuration struct {
	InitTimeout *time.Duration `yaml:"initTimeout"`
}

// NewOptions creates an Option.
func (cfg Configuration) NewOptions() Options {
	opts := NewOptions()
	if cfg.InitTimeout != nil {
		opts = opts.SetInitTimeout(*cfg.InitTimeout)
	}
	return opts
}

// ElectionConfiguration is for configuring election timeouts and TTLs
type ElectionConfiguration struct {
	LeaderTimeout *time.Duration `yaml:"leaderTimeout"`
	ResignTimeout *time.Duration `yaml:"resignTimeout"`
	TTLSeconds    *int           `yaml:"TTLSeconds"`
}

// NewOptions creates an ElectionOptions.
func (cfg ElectionConfiguration) NewOptions() ElectionOptions {
	opts := NewElectionOptions()
	if cfg.LeaderTimeout != nil {
		opts = opts.SetLeaderTimeout(*cfg.LeaderTimeout)
	}
	if cfg.ResignTimeout != nil {
		opts = opts.SetResignTimeout(*cfg.ResignTimeout)
	}
	if cfg.TTLSeconds != nil {
		opts = opts.SetTTLSecs(*cfg.TTLSeconds)
	}
	return opts
}

// ServiceIDConfiguration is for configuring serviceID.
type ServiceIDConfiguration struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	Zone        string `yaml:"zone"`
}

// NewServiceID creates a ServiceID.
func (cfg ServiceIDConfiguration) NewServiceID() ServiceID {
	return NewServiceID().
		SetName(cfg.Name).
		SetEnvironment(cfg.Environment).
		SetZone(cfg.Zone)
}
