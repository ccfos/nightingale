// Copyright (c) 2018 Uber Technologies, Inc.
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

package block

import (
	"time"

	"github.com/m3db/m3/src/query/ts"
)

// Series is a single series within a block.
type Series struct {
	values []float64
	Meta   SeriesMeta
}

// NewSeries creates a new series.
func NewSeries(values []float64, meta SeriesMeta) Series {
	return Series{values: values, Meta: meta}
}

// ValueAtStep returns the datapoint value at a step index.
func (s Series) ValueAtStep(idx int) float64 {
	return s.values[idx]
}

// Values returns the internal values slice.
func (s Series) Values() []float64 {
	return s.values
}

// Len returns the number of datapoints in the series.
func (s Series) Len() int {
	return len(s.values)
}

// UnconsolidatedSeries is the series with raw datapoints.
type UnconsolidatedSeries struct {
	datapoints ts.Datapoints
	Meta       SeriesMeta
	stats      UnconsolidatedSeriesStats
}

// UnconsolidatedSeriesStats is stats about an unconsolidated series.
type UnconsolidatedSeriesStats struct {
	Enabled        bool
	DecodeDuration time.Duration
}

// NewUnconsolidatedSeries creates a new series with raw datapoints.
func NewUnconsolidatedSeries(
	datapoints ts.Datapoints,
	meta SeriesMeta,
	stats UnconsolidatedSeriesStats,
) UnconsolidatedSeries {
	return UnconsolidatedSeries{
		datapoints: datapoints,
		Meta:       meta,
		stats:      stats,
	}
}

// Datapoints returns the internal datapoints slice.
func (s UnconsolidatedSeries) Datapoints() ts.Datapoints {
	return s.datapoints
}

// Len returns the number of datapoints slices in the series.
func (s UnconsolidatedSeries) Len() int {
	return len(s.datapoints)
}

// Stats returns statistics about the unconsolidated series if they were supplied.
func (s UnconsolidatedSeries) Stats() UnconsolidatedSeriesStats {
	return s.stats
}
