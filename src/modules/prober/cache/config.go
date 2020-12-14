package cache

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/config"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v2"
)

type MetricConfig struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	Comment string `yaml:"comment"`
}

type PluginConfig struct {
	Metrics []MetricConfig `metrics`
}

var (
	metricsConfig map[string]MetricConfig
	ignoreConfig  bool
)

func InitPluginsConfig(cf *config.ConfYaml) {
	metricsConfig = make(map[string]MetricConfig)
	ignoreConfig = cf.IgnoreConfig
	plugins := collector.GetRemoteCollectors()
	for _, plugin := range plugins {
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
			metricsConfig[v.Name] = v
		}
		logger.Infof("loaded plugin config %s", file)
	}
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
