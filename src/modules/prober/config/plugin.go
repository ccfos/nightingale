package config

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/expr"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v2"
)

var (
	pluginConfigs map[string]*PluginConfig
)

const (
	PluginModeWhitelist = iota
	PluginModeAll
)

type Metric struct {
	Name      string         `yaml:"name"`
	Type      string         `yaml:"type"`
	Comment   string         `yaml:"comment"`
	Expr      string         `yaml:"expr"`
	notations expr.Notations `yaml:"-"`
}

type PluginConfig struct {
	Name        string
	Mode        int
	Metrics     map[string]*Metric
	ExprMetrics map[string]*Metric
}

type pluginConfig struct {
	Metrics []*Metric `yaml:"metrics"`
	Mode    string    `yaml:"mode"`
	mode    int       `yaml:"-"`
}

func (p *pluginConfig) Validate() error {
	switch strings.ToLower(p.Mode) {
	case "whitelist":
		p.mode = PluginModeWhitelist
	case "all":
		p.mode = PluginModeAll
	default:
		p.mode = PluginModeWhitelist
	}

	for k, v := range p.Metrics {
		if v.Name == "" {
			return fmt.Errorf("metrics[%d].name must be set", k)
		}
		if v.Type == "" {
			v.Type = dataobj.GAUGE
		}
		if v.Type != dataobj.GAUGE &&
			v.Type != dataobj.COUNTER &&
			v.Type != dataobj.SUBTRACT {
			return fmt.Errorf("metrics[%s].type.%s unsupported", v.Name, v.Type)
		}
	}
	return nil
}

func InitPluginsConfig(cf *ConfYaml) {
	pluginConfigs = make(map[string]*PluginConfig)
	for _, plugin := range collector.GetRemoteCollectors() {
		c := pluginConfig{}
		config := newPluginConfig()
		pluginConfigs[plugin] = config

		file := filepath.Join(cf.PluginsConfig, plugin+".local.yml")
		b, err := ioutil.ReadFile(file)
		if err != nil {
			file = filepath.Join(cf.PluginsConfig, plugin+".yml")
			b, err = ioutil.ReadFile(file)
		}
		if err != nil {
			logger.Debugf("readfile %s err %s", plugin, err)
			continue
		}

		if err := yaml.Unmarshal(b, &c); err != nil {
			logger.Warningf("yaml.Unmarshal %s err %s", plugin, err)
			continue
		}

		if err := c.Validate(); err != nil {
			logger.Warningf("%s Validate() err %s", plugin, err)
			continue
		}
		config.Name = plugin
		config.Mode = c.mode

		for _, v := range c.Metrics {
			if v.Expr != "" {
				err := v.parse()
				if err != nil {
					panic(fmt.Sprintf("plugin %s metrics %s expr %s parse err %s",
						plugin, v.Name, v.Expr, err))
				}
				config.ExprMetrics[v.Name] = v
			} else {
				config.Metrics[v.Name] = v
			}
		}
		logger.Infof("loaded plugin config %s", file)
	}
}

func (p *Metric) parse() (err error) {
	p.notations, err = expr.NewNotations([]byte(p.Expr))
	return
}

func (p *Metric) Calc(vars map[string]*dataobj.MetricValue) (float64, error) {
	return p.notations.Calc(vars)
}

func GetMetric(plugin, metric string, typ telegraf.ValueType) (c *Metric, ok bool) {
	p, ok := pluginConfigs[plugin]
	if !ok {
		return
	}

	c, ok = p.Metrics[metric]
	if !ok {
		c, ok = p.ExprMetrics[metric]
	}
	if !ok {
		return
	}

	if c.Type == "" {
		c.Type = metricType(typ)
	}

	return
}

func GetPluginConfig(pluginName string) (c *PluginConfig, ok bool) {
	c, ok = pluginConfigs[pluginName]
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

func newPluginConfig() *PluginConfig {
	return &PluginConfig{
		Metrics:     map[string]*Metric{},
		ExprMetrics: map[string]*Metric{},
	}
}
