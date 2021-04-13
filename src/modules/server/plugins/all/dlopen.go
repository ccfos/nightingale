package all

import (
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"

	"github.com/toolkits/pkg/logger"
)

const pluginDir = "plugins"

func init() {
	plugins, err := listPlugins(pluginDir)
	if err != nil {
		logger.Warningf("list plugins: %s", err)
		return
	}

	for _, file := range plugins {
		_, err := plugin.Open(file)
		if err != nil {
			logger.Warningf("plugin.Open %s err %s", file, err)
			continue
		}
	}
}

func listPlugins(dir string) ([]string, error) {
	df, err := os.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("failed opening directory: %s", err)
	}
	defer df.Close()

	list, err := df.Readdirnames(0) // 0 to read all files and folders
	if err != nil {
		return nil, fmt.Errorf("read dir names: %s", err)
	}

	var plugins []string
	for _, name := range list {
		if !strings.HasSuffix(name, ".so") {
			continue
		}

		file := filepath.Join(dir, name)
		if fi, err := os.Stat(file); err != nil {
			continue
		} else if fi.IsDir() {
			continue
		}
		plugins = append(plugins, file)
	}

	return plugins, nil
}
