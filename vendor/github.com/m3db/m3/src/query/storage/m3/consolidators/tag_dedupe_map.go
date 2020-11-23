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

type tagDedupeMap struct {
	fanout     QueryFanoutType
	mapWrapper *fetchResultMapWrapper
	tagOpts    models.TagOptions
}

type tagMapOpts struct {
	size    int
	fanout  QueryFanoutType
	tagOpts models.TagOptions
}

func newTagDedupeMap(opts tagMapOpts) fetchDedupeMap {
	return &tagDedupeMap{
		fanout:     opts.fanout,
		mapWrapper: newFetchResultMapWrapper(opts.size),
		tagOpts:    opts.tagOpts,
	}
}

func (m *tagDedupeMap) close() {
	m.mapWrapper.close()
}

func (m *tagDedupeMap) list() []multiResultSeries {
	return m.mapWrapper.list()
}

func (m *tagDedupeMap) add(
	iter encoding.SeriesIterator,
	attrs storagemetadata.Attributes,
) error {
	tags, err := FromIdentTagIteratorToTags(iter.Tags(), m.tagOpts)
	if err != nil {
		return err
	}

	iter.Tags().Rewind()
	series := multiResultSeries{
		iter:  iter,
		attrs: attrs,
		tags:  tags,
	}

	existing, exists := m.mapWrapper.get(tags)
	if !exists {
		m.mapWrapper.set(tags, series)
		return nil
	}

	var existsBetter bool
	var existsEqual bool
	switch m.fanout {
	case NamespaceCoversAllQueryRange:
		// Already exists and resolution of result we are adding is not as precise
		existsBetter = existing.attrs.Resolution < attrs.Resolution
		existsEqual = existing.attrs.Resolution == attrs.Resolution
	case NamespaceCoversPartialQueryRange:
		// Already exists and either has longer retention, or the same retention
		// and result we are adding is not as precise
		existsLongerRetention := existing.attrs.Retention > attrs.Retention
		existsSameRetentionEqualOrBetterResolution :=
			existing.attrs.Retention == attrs.Retention &&
				existing.attrs.Resolution < attrs.Resolution
		existsBetter = existsLongerRetention || existsSameRetentionEqualOrBetterResolution

		existsEqual = existing.attrs.Retention == attrs.Retention &&
			existing.attrs.Resolution == attrs.Resolution
	default:
		return fmt.Errorf("unknown query fanout type: %d", m.fanout)
	}

	if existsEqual {
		acc, ok := existing.iter.(encoding.SeriesIteratorAccumulator)
		if !ok {
			acc, err = encoding.NewSeriesIteratorAccumulator(existing.iter,
				encoding.SeriesAccumulatorOptions{})
			if err != nil {
				return err
			}
		}

		if err := acc.Add(iter); err != nil {
			return err
		}

		// Update accumulated result series.
		series.iter = acc
		m.mapWrapper.set(tags, series)
		return nil
	}

	if existsBetter {
		// Existing result is already better
		return nil
	}

	// Override
	existing.iter.Close()
	m.mapWrapper.set(tags, series)

	return nil
}
