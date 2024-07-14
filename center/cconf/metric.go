package cconf

import (
	"path"

	"github.com/toolkits/pkg/file"
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

	switch lang {
	case "en":
		m = MetricDesc.En
	default:
		m = MetricDesc.Zh
	}

	if m != nil {
		if desc, ok := m[metric]; ok {
			return desc
		}
	}

	if MetricDesc.CommonDesc != nil {
		if desc, ok := MetricDesc.CommonDesc[metric]; ok {
			return desc
		}
	}

	return ""
}
func LoadMetricsYaml(configDir, metricsYamlFile string) error {
	fp := metricsYamlFile
	if fp == "" {
		fp = path.Join(configDir, "metrics.yaml")
	}
	if !file.IsExist(fp) {
		return nil
	}
	return file.ReadYaml(fp, &MetricDesc)
}
