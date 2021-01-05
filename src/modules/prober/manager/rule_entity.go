package manager

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/cache"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
)

// not thread-safe
type ruleEntity struct {
	sync.RWMutex
	telegraf.Input
	rule      *models.CollectRule
	tags      map[string]string
	precision time.Duration
	metrics   []*dataobj.MetricValue
}

func newRuleEntity(rule *models.CollectRule) (*ruleEntity, error) {
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

	return &ruleEntity{
		Input:     input,
		rule:      rule,
		tags:      tags,
		metrics:   []*dataobj.MetricValue{},
		precision: time.Second,
	}, nil
}

// calc metrics with expression
func (p *ruleEntity) calc() error {
	if len(p.metrics) == 0 {
		return nil
	}
	sample := p.metrics[0]

	configs, ok := cache.GetMetricExprs(p.rule.CollectType)
	if !ok {
		return nil
	}

	vars := map[string]float64{}
	for _, v := range p.metrics {
		logger.Debugf("get v[%s] %f", v.Metric, v.Value)
		vars[v.Metric] = v.Value
	}

	for _, config := range configs.Metrics {
		f, err := config.Calc(vars)
		if err != nil {
			logger.Debugf("calc err %s", err)
			continue
		}
		p.metrics = append(p.metrics, &dataobj.MetricValue{
			Nid:          sample.Nid,
			Metric:       config.Name,
			Timestamp:    sample.Timestamp,
			Step:         sample.Step,
			CounterType:  config.Type,
			TagsMap:      sample.TagsMap,
			Value:        f,
			ValueUntyped: f,
		})
	}

	if configs.Mode == cache.PluginModeOverlay {
		for k, v := range vars {
			if _, ok := configs.Metrics[k]; ok {
				continue
			}
			p.metrics = append(p.metrics, &dataobj.MetricValue{
				Nid:          sample.Nid,
				Metric:       k,
				Timestamp:    sample.Timestamp,
				Step:         sample.Step,
				CounterType:  "GAUGE",
				TagsMap:      sample.TagsMap,
				Value:        v,
				ValueUntyped: v,
			})
		}
	}
	return nil
}

func (p *ruleEntity) update(rule *models.CollectRule) error {
	if p.rule.LastUpdated == rule.LastUpdated {
		return nil
	}

	input, err := telegrafInput(rule)
	if err != nil {
		// ignore error, use old config
		log.Printf("telegrafInput() id %d type %s name %s err %s",
			rule.Id, rule.CollectType, rule.Name, err)
	}

	tags, err := dataobj.SplitTagsString(rule.Tags)
	if err != nil {
		return err
	}

	p.Input = input
	p.rule = rule
	p.tags = tags

	return nil
}

// https://docs.influxdata.com/telegraf/v1.14/data_formats/output/prometheus/
func (p *ruleEntity) MakeMetric(metric telegraf.Metric) []*dataobj.MetricValue {
	tags := map[string]string{}
	for _, v := range metric.TagList() {
		tags[v.Key] = v.Value
	}

	for k, v := range p.tags {
		tags[k] = v
	}

	nid := strconv.FormatInt(p.rule.Nid, 10)
	name := metric.Name()
	ts := metric.Time().Unix()
	step := int64(p.rule.Step) // deprecated

	fields := metric.Fields()
	ms := make([]*dataobj.MetricValue, 0, len(fields))
	for k, v := range fields {
		f, ok := v.(float64)
		if !ok {
			continue
		}

		c, ok := cache.Metric(name+"_"+k, metric.Type())
		if !ok {
			continue
		}

		ms = append(ms, &dataobj.MetricValue{
			Nid:          nid,
			Metric:       c.Name,
			Timestamp:    ts,
			Step:         step,
			CounterType:  c.Type,
			TagsMap:      tags,
			Value:        f,
			ValueUntyped: f,
		})
	}

	return ms
}

func (p *ruleEntity) AddFields(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Untyped, t...)
}

func (p *ruleEntity) AddGauge(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Gauge, t...)
}

func (p *ruleEntity) AddCounter(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Counter, t...)
}

func (p *ruleEntity) AddSummary(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Summary, t...)
}

func (p *ruleEntity) AddHistogram(
	measurement string,
	fields map[string]interface{},
	tags map[string]string,
	t ...time.Time,
) {
	p.addFields(measurement, tags, fields, telegraf.Histogram, t...)
}

func (p *ruleEntity) AddMetric(m telegraf.Metric) {
	m.SetTime(m.Time().Round(p.precision))
	if metrics := p.MakeMetric(m); m != nil {
		p.pushMetrics(metrics)
	}
}

func (p *ruleEntity) pushMetrics(metrics []*dataobj.MetricValue) {
	p.Lock()
	defer p.Unlock()
	p.metrics = append(p.metrics, metrics...)
}

func (p *ruleEntity) addFields(
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
func (p *ruleEntity) AddError(err error) {
	if err == nil {
		return
	}
	log.Printf("Error in plugin: %v", err)
}

func (p *ruleEntity) SetPrecision(precision time.Duration) {
	p.precision = precision
}

func (p *ruleEntity) getTime(t []time.Time) time.Time {
	var timestamp time.Time
	if len(t) > 0 {
		timestamp = t[0]
	} else {
		timestamp = time.Now()
	}
	return timestamp.Round(p.precision)
}

func (p *ruleEntity) WithTracking(maxTracked int) telegraf.TrackingAccumulator {
	return nil
}
