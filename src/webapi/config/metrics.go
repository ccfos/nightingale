package config

import (
	"path"

	cmap "github.com/orcaman/concurrent-map"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

var Metrics = cmap.New()

func loadMetricsYaml() error {
	fp := path.Join(runner.Cwd, "etc", "metrics.yaml")
	if !file.IsExist(fp) {
		return nil
	}

	nmap := make(map[string]string)
	err := file.ReadYaml(fp, &nmap)
	if err != nil {
		return err
	}

	for key, val := range nmap {
		Metrics.Set(key, val)
	}

	return nil
}
