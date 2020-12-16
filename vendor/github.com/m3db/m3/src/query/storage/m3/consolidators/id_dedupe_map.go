// Copyright (c) 2020 Uber Technologies, Inc.
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

package consolidators

import (
	"fmt"

	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/query/models"
	"github.com/m3db/m3/src/query/storage/m3/storagemetadata"
)

type idDedupeMap struct {
	fanout  QueryFanoutType
	series  map[string]multiResultSeries
	tagOpts models.TagOptions
}

func newIDDedupeMap(opts tagMapOpts) fetchDedupeMap {
	return &idDedupeMap{
		fanout:  opts.fanout,
		series:  make(map[string]multiResultSeries, opts.size),
		tagOpts: opts.tagOpts,
	}
}

func (m *idDedupeMap) close() {}

func (m *idDedupeMap) list() []multiResultSeries {
	result := make([]multiResultSeries, 0, len(m.series))
	for _, s := range m.series {
		result = append(result, s)
	}
	return result
}

func (m *idDedupeMap) add(
	iter encoding.SeriesIterator,
	attrs storagemetadata.Attributes,
) error {
	id := iter.ID().String()

	tags, err := FromIdentTagIteratorToTags(iter.Tags(), m.tagOpts)
	if err != nil {
		return err
	}

	iter.Tags().Rewind()
	existing, exists := m.series[id]
	if !exists {
		// Does not exist, new addition
		m.series[id] = multiResultSeries{
			attrs: attrs,
			iter:  iter,
			tags:  tags,
		}
		return nil
	}

	var existsBetter bool
	switch m.fanout {
	case NamespaceCoversAllQueryRange:
		// Already exists and resolution of result we are adding is not as precise
		existsBetter = existing.attrs.Resolution <= attrs.Resolution
	case NamespaceCoversPartialQueryRange:
		// Already exists and either has longer retention, or the same retention
		// and result we are adding is not as precise
		existsLongerRetention := existing.attrs.Retention > attrs.Retention
		existsSameRetentionEqualOrBetterResolution :=
			existing.attrs.Retention == attrs.Retention &&
				existing.attrs.Resolution <= attrs.Resolution
		existsBetter = existsLongerRetention || existsSameRetentionEqualOrBetterResolution
	default:
		return fmt.Errorf("unknown query fanout type: %d", m.fanout)
	}
	if existsBetter {
		// Existing result is already better
		return nil
	}

	// Override
	m.series[id] = multiResultSeries{
		attrs: attrs,
		iter:  iter,
		tags:  tags,
	}

	return nil
}
