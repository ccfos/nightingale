package manager

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
)

type AccumulatorOptions struct {
	Name    string
	Tags    map[string]string
	Metrics *[]*dataobj.MetricValue
}

func (p *AccumulatorOptions) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("unable to get Name")
	}
	if p.Metrics == nil {
		return fmt.Errorf("unable to get metrics")
	}

	return nil
}

// NewAccumulator return telegraf.Accumulator
func NewAccumulator(opt AccumulatorOptions) (telegraf.Accumulator, error) {
	if err := opt.Validate(); err != nil {
		return nil, err
	}

	return &accumulator{
		name:      opt.Name,
		tags:      opt.Tags,
		metrics:   opt.Metrics,
		precision: time.Second,
	}, nil
}

type accumulator struct {
	sync.RWMutex
	name      string
	tags      map[string]string
	precision time.Duration
	metrics   *[]*dataobj.MetricValue
}

func (p *accumulator) AddFields(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Untyped, t...)
}

func (p *accumulator) AddGauge(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Gauge, t...)
}

func (p *accumulator) AddCounter(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Counter, t...)
}

func (p *accumulator) AddSummary(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Summary, t...)
}

func (p *accumulator) AddHistogram(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Histogram, t...)
}

func (p *accumulator) AddMetric(m telegraf.Metric) {
	m.SetTime(m.Time().Round(p.precision))
	if metrics := p.makeMetric(m); m != nil {
		p.pushMetrics(metrics)
	}
}

func (p *accumulator) SetPrecision(precision time.Duration) {
	p.precision = precision
}

// AddError passes a runtime error to the accumulator.
// The error will be tagged with the plugin name and written to the log.
func (p *accumulator) AddError(err error) {
	if err == nil {
		return
	}
	logger.Debugf("accumulator %s Error: %s", p.name, err)
}

func (p *accumulator) WithTracking(maxTracked int) telegraf.TrackingAccumulator {
	return nil
}

func (p *accumulator) addFields(
	measurement string,
	tags map[string]string,
	fields map[string]interface{},
	tp telegraf.ValueType,
	t ...time.Time,
) {
	m, err := NewMetric(measurement, tags, fields, p.getTime(t), tp)
	if err != nil {
		return
	}
	if metrics := p.makeMetric(m); m != nil {
		p.pushMetrics(metrics)
	}
}

func (p *accumulator) getTime(t []time.Time) time.Time {
	var timestamp time.Time
	if len(t) > 0 {
		timestamp = t[0]
	} else {
		timestamp = time.Now()
	}
	return timestamp.Round(p.precision)
}

// https://docs.influxdata.com/telegraf/v1.14/data_formats/output/prometheus/
func (p *accumulator) makeMetric(metric telegraf.Metric) []*dataobj.MetricValue {
	tags := map[string]string{}
	for _, v := range metric.TagList() {
		tags[v.Key] = v.Value
	}

	for k, v := range p.tags {
		tags[k] = v
	}

	switch metric.Type() {
	case telegraf.Counter:
		return makeCounter(metric, tags)
	case telegraf.Summary, telegraf.Histogram:
		return makeSummary(metric, tags)
	default:
		return makeGauge(metric, tags)
	}

}

func makeSummary(metric telegraf.Metric, tags map[string]string) []*dataobj.MetricValue {
	name := metric.Name()
	ts := metric.Time().Unix()
	fields := metric.Fields()
	ms := make([]*dataobj.MetricValue, 0, len(fields))

	for k, v := range fields {
		f, ok := v.(float64)
		if !ok {
			continue
		}

		countType := "GAUGE"
		if strings.HasSuffix(k, "_count") ||
			strings.HasSuffix(k, "_sum") {
			countType = "COUNTER"
		}

		ms = append(ms, &dataobj.MetricValue{
			Metric:       name + "_" + k,
			CounterType:  countType,
			Timestamp:    ts,
			TagsMap:      tags,
			Value:        f,
			ValueUntyped: f,
		})

	}
	return ms
}

func makeCounter(metric telegraf.Metric, tags map[string]string) []*dataobj.MetricValue {
	name := metric.Name()
	ts := metric.Time().Unix()
	fields := metric.Fields()
	ms := make([]*dataobj.MetricValue, 0, len(fields))

	for k, v := range fields {
		f, ok := v.(float64)
		if !ok {
			continue
		}

		ms = append(ms, &dataobj.MetricValue{
			Metric:       name + "_" + k,
			CounterType:  "COUNTER",
			Timestamp:    ts,
			TagsMap:      tags,
			Value:        f,
			ValueUntyped: f,
		})
	}

	return ms
}

func makeGauge(metric telegraf.Metric, tags map[string]string) []*dataobj.MetricValue {
	name := metric.Name()
	ts := metric.Time().Unix()
	fields := metric.Fields()
	ms := make([]*dataobj.MetricValue, 0, len(fields))

	for k, v := range fields {
		f, ok := v.(float64)
		if !ok {
			continue
		}

		ms = append(ms, &dataobj.MetricValue{
			Metric:       name + "_" + k,
			CounterType:  "GAUGE",
			Timestamp:    ts,
			TagsMap:      tags,
			Value:        f,
			ValueUntyped: f,
		})
	}

	return ms

}

func (p *accumulator) pushMetrics(metrics []*dataobj.MetricValue) {
	p.Lock()
	defer p.Unlock()
	*p.metrics = append(*p.metrics, metrics...)
}
