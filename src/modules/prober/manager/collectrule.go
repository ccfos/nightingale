package manager

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/prober/config"
	"github.com/didi/nightingale/v4/src/modules/prober/manager/accumulator"
	"github.com/didi/nightingale/v4/src/modules/server/collector"

	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
)

type collectRule struct {
	sync.RWMutex
	*models.CollectRule

	input     telegraf.Input
	acc       telegraf.Accumulator
	metrics   *[]*dataobj.MetricValue
	tags      map[string]string
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

	metrics := []*dataobj.MetricValue{}

	acc, err := accumulator.New(accumulator.Options{
		Name:    fmt.Sprintf("%s-%d", rule.CollectType, rule.Id),
		Tags:    tags,
		Metrics: &metrics})
	if err != nil {
		return nil, err
	}

	return &collectRule{
		CollectRule: rule,
		input:       input,
		acc:         acc,
		metrics:     &metrics,
		tags:        tags,
		updatedAt:   rule.UpdatedAt,
	}, nil
}

func (p *collectRule) reset() {
	p.Lock()
	defer p.Unlock()

	*p.metrics = (*p.metrics)[:0]
}

func (p *collectRule) Metrics() []*dataobj.MetricValue {
	p.RLock()
	defer p.RUnlock()

	return *p.metrics
}

// prepareMetrics
func (p *collectRule) prepareMetrics(pluginConfig *config.PluginConfig) (metrics []*dataobj.MetricValue, err error) {
	p.RLock()
	defer p.RUnlock()

	if len(*p.metrics) == 0 {
		return
	}

	metrics = *p.metrics
	ts := metrics[0].Timestamp
	nid := strconv.FormatInt(p.Nid, 10)

	if pluginConfig.Mode == config.PluginModeWhitelist && len(pluginConfig.Metrics) == 0 {
		return nil, nil
	}

	vars := map[string][]*dataobj.MetricValue{}
	for _, v := range metrics {
		logger.Debugf("get v[%s] %f", v.Metric, v.Value)
		if _, ok := vars[v.Metric]; !ok {
			vars[v.Metric] = []*dataobj.MetricValue{v}
		} else {
			vars[v.Metric] = append(vars[v.Metric], v)
		}

	}

	metrics = metrics[:0]
	for _, metric := range pluginConfig.ExprMetrics {
		f, err := metric.Calc(vars)
		if err != nil {
			logger.Debugf("calc err %s", err)
			continue
		}
		metrics = append(metrics, &dataobj.MetricValue{
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
		for _, v2 := range v {
			if metric, ok := pluginConfig.Metrics[k]; ok {
				metrics = append(metrics, &dataobj.MetricValue{
					Nid:          nid,
					Metric:       k,
					Timestamp:    ts,
					Step:         p.Step,
					CounterType:  metric.Type,
					TagsMap:      v2.TagsMap,
					Value:        v2.Value,
					ValueUntyped: v2.ValueUntyped,
				})
			} else {
				if pluginConfig.Mode == config.PluginModeWhitelist {
					continue
				}
				metrics = append(metrics, &dataobj.MetricValue{
					Nid:          nid,
					Metric:       k,
					Timestamp:    ts,
					Step:         p.Step,
					CounterType:  "GAUGE",
					TagsMap:      v2.TagsMap,
					Value:        v2.Value,
					ValueUntyped: v2.ValueUntyped,
				})
			}
		}
	}
	return
}

func (p *collectRule) update(rule *models.CollectRule) error {
	p.Lock()
	defer p.Unlock()

	if p.updatedAt == rule.UpdatedAt {
		return nil
	}

	logger.Debugf("update %s", rule)

	if si, ok := p.input.(telegraf.ServiceInput); ok {
		si.Stop()
	}

	input, err := telegrafInput(rule)
	if err != nil {
		// ignore error, use old config
		logger.Warningf("telegrafInput %s err %s", rule, err)
	}

	tags, err := dataobj.SplitTagsString(rule.Tags)
	if err != nil {
		return err
	}

	acc, err := accumulator.New(accumulator.Options{
		Name:    fmt.Sprintf("%s-%d", rule.CollectType, rule.Id),
		Tags:    tags,
		Metrics: p.metrics})
	if err != nil {
		return err
	}

	p.input = input
	p.CollectRule = rule
	p.acc = acc
	p.UpdatedAt = rule.UpdatedAt

	return nil
}
