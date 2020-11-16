// Copyright (c) 2019 Uber Technologies, Inc.
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

package multi

import (
	"time"

	"github.com/uber-go/tally"
)

type multi struct {
	multiBaseReporters multiBaseReporters
	reporters          []tally.StatsReporter
}

// NewMultiReporter creates a new multi tally.StatsReporter.
func NewMultiReporter(
	r ...tally.StatsReporter,
) tally.StatsReporter {
	var baseReporters multiBaseReporters
	for _, r := range r {
		baseReporters = append(baseReporters, r)
	}
	return &multi{
		multiBaseReporters: baseReporters,
		reporters:          r,
	}
}

func (r *multi) ReportCounter(
	name string,
	tags map[string]string,
	value int64,
) {
	for _, r := range r.reporters {
		r.ReportCounter(name, tags, value)
	}
}

func (r *multi) ReportGauge(
	name string,
	tags map[string]string,
	value float64,
) {
	for _, r := range r.reporters {
		r.ReportGauge(name, tags, value)
	}
}

func (r *multi) ReportTimer(
	name string,
	tags map[string]string,
	interval time.Duration,
) {
	for _, r := range r.reporters {
		r.ReportTimer(name, tags, interval)
	}
}

func (r *multi) ReportHistogramValueSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound float64,
	samples int64,
) {
	for _, r := range r.reporters {
		r.ReportHistogramValueSamples(name, tags, buckets,
			bucketLowerBound, bucketUpperBound, samples)
	}
}

func (r *multi) ReportHistogramDurationSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound time.Duration,
	samples int64,
) {
	for _, r := range r.reporters {
		r.ReportHistogramDurationSamples(name, tags, buckets,
			bucketLowerBound, bucketUpperBound, samples)
	}
}

func (r *multi) Capabilities() tally.Capabilities {
	return r.multiBaseReporters.Capabilities()
}

func (r *multi) Flush() {
	r.multiBaseReporters.Flush()
}

type multiCached struct {
	multiBaseReporters multiBaseReporters
	reporters          []tally.CachedStatsReporter
}

// NewMultiCachedReporter creates a new multi tally.CachedStatsReporter.
func NewMultiCachedReporter(
	r ...tally.CachedStatsReporter,
) tally.CachedStatsReporter {
	var baseReporters multiBaseReporters
	for _, r := range r {
		baseReporters = append(baseReporters, r)
	}
	return &multiCached{
		multiBaseReporters: baseReporters,
		reporters:          r,
	}
}

func (r *multiCached) AllocateCounter(
	name string,
	tags map[string]string,
) tally.CachedCount {
	metrics := make([]tally.CachedCount, 0, len(r.reporters))
	for _, r := range r.reporters {
		metrics = append(metrics, r.AllocateCounter(name, tags))
	}
	return multiMetric{counters: metrics}
}

func (r *multiCached) AllocateGauge(
	name string,
	tags map[string]string,
) tally.CachedGauge {
	metrics := make([]tally.CachedGauge, 0, len(r.reporters))
	for _, r := range r.reporters {
		metrics = append(metrics, r.AllocateGauge(name, tags))
	}
	return multiMetric{gauges: metrics}
}

func (r *multiCached) AllocateTimer(
	name string,
	tags map[string]string,
) tally.CachedTimer {
	metrics := make([]tally.CachedTimer, 0, len(r.reporters))
	for _, r := range r.reporters {
		metrics = append(metrics, r.AllocateTimer(name, tags))
	}
	return multiMetric{timers: metrics}
}

func (r *multiCached) AllocateHistogram(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
) tally.CachedHistogram {
	metrics := make([]tally.CachedHistogram, 0, len(r.reporters))
	for _, r := range r.reporters {
		metrics = append(metrics, r.AllocateHistogram(name, tags, buckets))
	}
	return multiMetric{histograms: metrics}
}

func (r *multiCached) Capabilities() tally.Capabilities {
	return r.multiBaseReporters.Capabilities()
}

func (r *multiCached) Flush() {
	r.multiBaseReporters.Flush()
}

type multiMetric struct {
	counters   []tally.CachedCount
	gauges     []tally.CachedGauge
	timers     []tally.CachedTimer
	histograms []tally.CachedHistogram
}

func (m multiMetric) ReportCount(value int64) {
	for _, m := range m.counters {
		m.ReportCount(value)
	}
}

func (m multiMetric) ReportGauge(value float64) {
	for _, m := range m.gauges {
		m.ReportGauge(value)
	}
}

func (m multiMetric) ReportTimer(interval time.Duration) {
	for _, m := range m.timers {
		m.ReportTimer(interval)
	}
}

func (m multiMetric) ValueBucket(
	bucketLowerBound, bucketUpperBound float64,
) tally.CachedHistogramBucket {
	var multi []tally.CachedHistogramBucket
	for _, m := range m.histograms {
		multi = append(multi,
			m.ValueBucket(bucketLowerBound, bucketUpperBound))
	}
	return multiHistogramBucket{multi}
}

func (m multiMetric) DurationBucket(
	bucketLowerBound, bucketUpperBound time.Duration,
) tally.CachedHistogramBucket {
	var multi []tally.CachedHistogramBucket
	for _, m := range m.histograms {
		multi = append(multi,
			m.DurationBucket(bucketLowerBound, bucketUpperBound))
	}
	return multiHistogramBucket{multi}
}

type multiHistogramBucket struct {
	multi []tally.CachedHistogramBucket
}

func (m multiHistogramBucket) ReportSamples(value int64) {
	for _, m := range m.multi {
		m.ReportSamples(value)
	}
}

type multiBaseReporters []tally.BaseStatsReporter

func (r multiBaseReporters) Capabilities() tally.Capabilities {
	c := &capabilities{reporting: true, tagging: true}
	for _, r := range r {
		c.reporting = c.reporting && r.Capabilities().Reporting()
		c.tagging = c.tagging && r.Capabilities().Tagging()
	}
	return c
}

func (r multiBaseReporters) Flush() {
	for _, r := range r {
		r.Flush()
	}
}

type capabilities struct {
	reporting  bool
	tagging    bool
	histograms bool
}

func (c *capabilities) Reporting() bool {
	return c.reporting
}

func (c *capabilities) Tagging() bool {
	return c.tagging
}
