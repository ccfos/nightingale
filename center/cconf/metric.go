package cconf

import (
	"path"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

// metricDesc , As load map happens before read map, there is no necessary to use concurrent map for metric desc store
type MetricDescType struct {
	CommonDesc map[string]string `yaml:",inline" json:"common"`
	Zh         map[string]string `yaml:"zh" json:"zh"`
	En         map[string]string `yaml:"en" json:"en"`
}

var MetricDesc MetricDescType

// GetMetricDesc , if metric is not registered, empty string will be returned
func GetMetricDesc(lang, metric string) string {
	var m map[string]string
	if lang == "zh" {
		m = MetricDesc.Zh
	} else {
		m = MetricDesc.En
	}
	if m != nil {
		if desc, has := m[metric]; has {
			return desc
		}
	}

	return MetricDesc.CommonDesc[metric]
}

func LoadMetricsYaml(metricsYamlFile string) error {
	fp := metricsYamlFile
	if fp == "" {
		fp = path.Join(runner.Cwd, "etc", "metrics.yaml")
	}
	if !file.IsExist(fp) {
		return nil
	}
	return file.ReadYaml(fp, &MetricDesc)
}
