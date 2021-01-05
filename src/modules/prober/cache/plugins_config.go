package cache

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

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
	Metrics []*MetricConfig `yaml:"metrics"`
	Mode    string          `yaml:"mode"`
	mode    int             `yaml:"-"`
}

type CachePluginConfig struct {
	Name    string
	Mode    int
	Metrics map[string]*MetricConfig
}

const (
	PluginModeWhitelist = iota
	PluginModeOverlay
)

func (p *PluginConfig) Validate() error {
	switch strings.ToLower(p.Mode) {
	case "whitelist":
		p.mode = PluginModeWhitelist
	case "overlay":
		p.mode = PluginModeOverlay
	default:
		p.mode = PluginModeWhitelist
	}
	return nil
}

var (
	metricsConfig map[string]*MetricConfig
	metricsExpr   map[string]*CachePluginConfig
)

func InitPluginsConfig(cf *config.ConfYaml) {
	metricsConfig = make(map[string]*MetricConfig)
	metricsExpr = make(map[string]*CachePluginConfig)
	plugins := collector.GetRemoteCollectors()
	for _, plugin := range plugins {
		cacheConfig := newCachePluginConfig()
		config := PluginConfig{}
		metricsExpr[plugin] = cacheConfig

		file := filepath.Join(cf.PluginsConfig, plugin+".yml")
		b, err := ioutil.ReadFile(file)
		if err != nil {
			logger.Debugf("readfile %s err %s", plugin, err)
			continue
		}

		if err := yaml.Unmarshal(b, &config); err != nil {
			logger.Warningf("yaml.Unmarshal %s err %s", plugin, err)
			continue
		}

		if err := config.Validate(); err != nil {
			logger.Warningf("%s Validate() err %s", plugin, err)
			continue
		}
		cacheConfig.Name = plugin
		cacheConfig.Mode = config.mode

		for _, v := range config.Metrics {
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
				cacheConfig.Metrics[v.Name] = v
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

func Metric(metric string, typ telegraf.ValueType) (c *MetricConfig, ok bool) {
	c, ok = metricsConfig[metric]
	if !ok {
		return
	}

	if c.Type == "" {
		c.Type = metricType(typ)
	}

	return
}

func GetMetricExprs(pluginName string) (c *CachePluginConfig, ok bool) {
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

func newCachePluginConfig() *CachePluginConfig {
	return &CachePluginConfig{
		Metrics: map[string]*MetricConfig{},
	}
}
