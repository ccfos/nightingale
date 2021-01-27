package manager

import (
	"strconv"
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/config"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
)

// not thread-safe
type collectRule struct {
	sync.RWMutex
	telegraf.Input
	*models.CollectRule
	tags      map[string]string
	precision time.Duration
	metrics   []*dataobj.MetricValue
	lastAt    int64
	updatedAt int64
}

func newCollectRule(rule *models.CollectRule) (*collectRule, error) {
	c, err := collector.GetCollector(rule.CollectType)
	if err != nil {
		return nil, err
	}

	input, err := c.TelegrafInput(rule)
	if err != nil {
		return nil, err
	}

	tags, err := dataobj.SplitTagsString(rule.Tags)
	if err != nil {
		return nil, err
	}

	return &collectRule{
		Input:       input,
		CollectRule: rule,
		tags:        tags,
		metrics:     []*dataobj.MetricValue{},
		precision:   time.Second,
		updatedAt:   rule.UpdatedAt,
	}, nil
}

// prepareMetrics
func (p *collectRule) prepareMetrics() error {
	if len(p.metrics) == 0 {
		return nil
	}
	ts := p.metrics[0].Timestamp
	nid := strconv.FormatInt(p.Nid, 10)

	pluginConfig, ok := config.GetPluginConfig(p.PluginName())
	if !ok {
		return nil
	}

	vars := map[string]*dataobj.MetricValue{}
	for _, v := range p.metrics {
		logger.Debugf("get v[%s] %f", v.Metric, v.Value)
		vars[v.Metric] = v
	}

	p.metrics = p.metrics[:0]
	for _, metric := range pluginConfig.ExprMetrics {
		f, err := metric.Calc(vars)
		if err != nil {
			logger.Debugf("calc err %s", err)
			continue
		}
		p.metrics = append(p.metrics, &dataobj.MetricValue{
			Nid:          nid,
			Metric:       metric.Name,
			Timestamp:    ts,
			Step:         p.Step,
			CounterType:  metric.Type,
			TagsMap:      p.tags,
			Value:        f,
			ValueUntyped: f,
		})
	}

	for k, v := range vars {
		if metric, ok := pluginConfig.Metrics[k]; ok {
			p.metrics = append(p.metrics, &dataobj.MetricValue{
				Nid:          nid,
				Metric:       k,
				Timestamp:    ts,
				Step:         p.Step,
				CounterType:  metric.Type,
				TagsMap:      v.TagsMap,
				Value:        v.Value,
				ValueUntyped: v.ValueUntyped,
			})
		} else {
			if pluginConfig.Mode == config.PluginModeWhitelist {
				continue
			}
			p.metrics = append(p.metrics, &dataobj.MetricValue{
				Nid:          nid,
				Metric:       k,
				Timestamp:    ts,
				Step:         p.Step,
				CounterType:  "GAUGE",
				TagsMap:      v.TagsMap,
				Value:        v.Value,
				ValueUntyped: v.ValueUntyped,
			})

		}
	}
	return nil
}

func (p *collectRule) update(rule *models.CollectRule) error {
	if p.updatedAt == rule.UpdatedAt {
		return nil
	}

	logger.Debugf("update %s", rule)

	input, err := telegrafInput(rule)
	if err != nil {
		// ignore error, use old config
		logger.Warningf("telegrafInput %s err %s", rule, err)
	}

	tags, err := dataobj.SplitTagsString(rule.Tags)
	if err != nil {
		return err
	}

	p.Input = input
	p.CollectRule = rule
	p.tags = tags
	p.UpdatedAt = rule.UpdatedAt

	return nil
}

// https://docs.influxdata.com/telegraf/v1.14/data_formats/output/prometheus/
func (p *collectRule) MakeMetric(metric telegraf.Metric) []*dataobj.MetricValue {
	tags := map[string]string{}
	for _, v := range metric.TagList() {
		tags[v.Key] = v.Value
	}

	for k, v := range p.tags {
		tags[k] = v
	}

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
			Timestamp:    ts,
			TagsMap:      tags,
			Value:        f,
			ValueUntyped: f,
		})
	}

	return ms
}

func (p *collectRule) AddFields(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Untyped, t...)
}

func (p *collectRule) AddGauge(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Gauge, t...)
}

func (p *collectRule) AddCounter(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Counter, t...)
}

func (p *collectRule) AddSummary(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Summary, t...)
}

func (p *collectRule) AddHistogram(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Histogram, t...)
}

func (p *collectRule) AddMetric(m telegraf.Metric) {
	m.SetTime(m.Time().Round(p.precision))
	if metrics := p.MakeMetric(m); m != nil {
		p.pushMetrics(metrics)
	}
}

func (p *collectRule) pushMetrics(metrics []*dataobj.MetricValue) {
	p.Lock()
	defer p.Unlock()
	p.metrics = append(p.metrics, metrics...)
}

func (p *collectRule) addFields(
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
	if metrics := p.MakeMetric(m); m != nil {
		p.pushMetrics(metrics)
	}
}

// AddError passes a runtime error to the accumulator.
// The error will be tagged with the plugin name and written to the log.
func (p *collectRule) AddError(err error) {
	if err == nil {
		return
	}
	logger.Debugf("collectRule %s.%s(%d) Error: %s", p.CollectType, p.Name, p.Id, err)
}

func (p *collectRule) SetPrecision(precision time.Duration) {
	p.precision = precision
}

func (p *collectRule) getTime(t []time.Time) time.Time {
	var timestamp time.Time
	if len(t) > 0 {
		timestamp = t[0]
	} else {
		timestamp = time.Now()
	}
	return timestamp.Round(p.precision)
}

func (p *collectRule) WithTracking(maxTracked int) telegraf.TrackingAccumulator {
	return nil
}
