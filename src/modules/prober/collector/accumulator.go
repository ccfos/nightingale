package collector

import (
	"time"

	"github.com/influxdata/telegraf"
)

type accumulator struct{}

// AddFields adds a metric to the accumulator with the given measurement
// name, fields, and tags (and timestamp). If a timestamp is not provided,
// then the accumulator sets it to "now".
func (p *accumulator) AddFields(measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time) {
}

// AddGauge is the same as AddFields, but will add the metric as a "Gauge" type
func (p *accumulator) AddGauge(measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time) {
}

// AddCounter is the same as AddFields, but will add the metric as a "Counter" type
func (p *accumulator) AddCounter(measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time) {
}

// AddSummary is the same as AddFields, but will add the metric as a "Summary" type
func (p *accumulator) AddSummary(measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time) {
}

// AddHistogram is the same as AddFields, but will add the metric as a "Histogram" type
func (p *accumulator) AddHistogram(measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time) {
}

// AddMetric adds an metric to the accumulator.
func (p *accumulator) AddMetric(telegraf.Metric) {
}

// SetPrecision sets the timestamp rounding precision.  All metrics addeds
// added to the accumulator will have their timestamp rounded to the
// nearest multiple of precision.
func (p *accumulator) SetPrecision(precision time.Duration) {
}

// Report an error.
func (p *accumulator) AddError(err error) {
}

// Upgrade to a TrackingAccumulator with space for maxTracked
// metrics/batches.
func (p *accumulator) WithTracking(maxTracked int) TrackingAccumulator {
}
