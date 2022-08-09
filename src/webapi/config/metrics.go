package config

import (
	"github.com/toolkits/pkg/file"
)

// CommonDesc , As load map happens before read map, there is no necessary to use concurrent map for metric desc store
type CommonDesc map[string]string

type metricDesc struct {
	CommonDesc `yaml:",inline"`
	Zh         map[string]string `yaml:"zh"`
	En         map[string]string `yaml:"en"`
}

var MetricDesc metricDesc

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

func loadMetricsYaml() error {
	fp := C.MetricsYamlFilePath()
	if !file.IsExist(fp) {
		return nil
	}

	return file.ReadYaml(fp, &MetricDesc)
}
