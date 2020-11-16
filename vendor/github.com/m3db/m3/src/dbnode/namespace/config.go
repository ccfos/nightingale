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
	"fmt"
	"time"

	"github.com/m3db/m3/src/dbnode/retention"
	"github.com/m3db/m3/src/x/ident"
)

// MapConfiguration is the configuration for a registry of namespaces
type MapConfiguration struct {
	Metadatas []MetadataConfiguration `yaml:"metadatas" validate:"nonzero"`
}

// Map returns a Map corresponding to the receiver struct
func (m *MapConfiguration) Map() (Map, error) {
	metadatas := make([]Metadata, 0, len(m.Metadatas))
	for _, m := range m.Metadatas {
		md, err := m.Metadata()
		if err != nil {
			return nil, fmt.Errorf("unable to construct metadata for [%+v], err: %v", m, err)
		}
		metadatas = append(metadatas, md)
	}
	return NewMap(metadatas)
}

// MetadataConfiguration is the configuration for a single namespace
type MetadataConfiguration struct {
	ID                    string                  `yaml:"id" validate:"nonzero"`
	BootstrapEnabled      *bool                   `yaml:"bootstrapEnabled"`
	FlushEnabled          *bool                   `yaml:"flushEnabled"`
	WritesToCommitLog     *bool                   `yaml:"writesToCommitLog"`
	CleanupEnabled        *bool                   `yaml:"cleanupEnabled"`
	RepairEnabled         *bool                   `yaml:"repairEnabled"`
	ColdWritesEnabled     *bool                   `yaml:"coldWritesEnabled"`
	CacheBlocksOnRetrieve *bool                   `yaml:"cacheBlocksOnRetrieve"`
	Retention             retention.Configuration `yaml:"retention" validate:"nonzero"`
	Index                 IndexConfiguration      `yaml:"index"`
}

// Metadata returns a Metadata corresponding to the receiver struct
func (mc *MetadataConfiguration) Metadata() (Metadata, error) {
	iopts := mc.Index.Options()
	ropts := mc.Retention.Options()
	opts := NewOptions().
		SetRetentionOptions(ropts).
		SetIndexOptions(iopts)
	if v := mc.BootstrapEnabled; v != nil {
		opts = opts.SetBootstrapEnabled(*v)
	}
	if v := mc.FlushEnabled; v != nil {
		opts = opts.SetFlushEnabled(*v)
	}
	if v := mc.WritesToCommitLog; v != nil {
		opts = opts.SetWritesToCommitLog(*v)
	}
	if v := mc.CleanupEnabled; v != nil {
		opts = opts.SetCleanupEnabled(*v)
	}
	if v := mc.RepairEnabled; v != nil {
		opts = opts.SetRepairEnabled(*v)
	}
	if v := mc.ColdWritesEnabled; v != nil {
		opts = opts.SetColdWritesEnabled(*v)
	}
	if v := mc.CacheBlocksOnRetrieve; v != nil {
		opts = opts.SetCacheBlocksOnRetrieve(*v)
	}
	return NewMetadata(ident.StringID(mc.ID), opts)
}

// IndexConfiguration controls the knobs to tweak indexing configuration.
type IndexConfiguration struct {
	Enabled   bool          `yaml:"enabled" validate:"nonzero"`
	BlockSize time.Duration `yaml:"blockSize" validate:"nonzero"`
}

// Options returns the IndexOptions corresponding to the receiver struct.
func (ic *IndexConfiguration) Options() IndexOptions {
	return NewIndexOptions().
		SetEnabled(ic.Enabled).
		SetBlockSize(ic.BlockSize)
}
