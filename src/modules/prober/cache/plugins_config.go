package cache

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/config"
	"github.com/didi/nightingale/src/modules/prober/expr"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v2"
)

type MetricConfig struct {
	Name      string         `yaml:"name"`
	Type      string         `yaml:"type"`
	Comment   string         `yaml:"comment"`
	Expr      string         `yaml:"expr"`
	notations expr.Notations `yaml:"-"`
}

type PluginConfig struct {
	Metrics []MetricConfig `metrics`
}

var (
	metricsConfig map[string]MetricConfig
	metricsExpr   map[string]map[string]MetricConfig
	ignoreConfig  bool
)

func InitPluginsConfig(cf *config.ConfYaml) {
	metricsConfig = make(map[string]MetricConfig)
	metricsExpr = make(map[string]map[string]MetricConfig)
	ignoreConfig = cf.IgnoreConfig
	plugins := collector.GetRemoteCollectors()
	for _, plugin := range plugins {
		metricsExpr[plugin] = make(map[string]MetricConfig)
		pluginConfig := PluginConfig{}

		file := filepath.Join(cf.PluginsConfig, plugin+".yml")
		b, err := ioutil.ReadFile(file)
		if err != nil {
			logger.Debugf("readfile %s err %s", plugin, err)
			continue
		}

		if err := yaml.Unmarshal(b, &pluginConfig); err != nil {
			logger.Warningf("yaml.Unmarshal %s err %s", plugin, err)
			continue
		}

		for _, v := range pluginConfig.Metrics {
			if _, ok := metricsConfig[v.Name]; ok {
				panic(fmt.Sprintf("plugin %s metrics %s is already exists", plugin, v.Name))
			}
			if v.Expr == "" {
				// nomore
				metricsConfig[v.Name] = v
			} else {
				err := v.parse()
				if err != nil {
					panic(fmt.Sprintf("plugin %s metrics %s expr %s parse err %s",
						plugin, v.Name, v.Expr, err))
				}
				metricsExpr[plugin][v.Name] = v

			}
		}
		logger.Infof("loaded plugin config %s", file)
	}
}

func (p *MetricConfig) parse() (err error) {
	p.notations, err = expr.NewNotations([]byte(p.Expr))
	return
}

func (p *MetricConfig) Calc(vars map[string]float64) (float64, error) {
	return p.notations.Calc(vars)
}

func Metric(metric string, typ telegraf.ValueType) (c MetricConfig, ok bool) {
	c, ok = metricsConfig[metric]
	if !ok && !ignoreConfig {
		return
	}

	if c.Type == "" {
		c.Type = metricType(typ)
	}

	return
}

func GetMetricExprs(pluginName string) (c map[string]MetricConfig, ok bool) {
	c, ok = metricsExpr[pluginName]
	return
}

func metricType(typ telegraf.ValueType) string {
	switch typ {
	case telegraf.Counter:
		return "COUNTER"
	case telegraf.Gauge:
		return "GAUGE"
	case telegraf.Untyped:
		return "GAUGE"
	case telegraf.Summary: // TODO
		return "SUMMARY"
	case telegraf.Histogram: // TODO
		return "HISTOGRAM"
	default:
		return "GAUGE"
	}
}
