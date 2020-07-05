package plugins

import (
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/didi/nightingale/src/modules/collector/stra"
	"github.com/didi/nightingale/src/modules/collector/sys"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

// key: 60_ntp.py
func ListPlugins() map[string]*Plugin {
	plugins := make(map[string]*Plugin)
	if sys.Config.PluginRemote {
		plugins = ListPluginsFromMonapi()
	} else {
		plugins = ListPluginsFromLocal()
	}
	return plugins
}

func ListPluginsFromMonapi() map[string]*Plugin {
	ret := make(map[string]*Plugin)

	plugins := stra.Collect.GetPlugin()

	for key, p := range plugins {
		plugin := &Plugin{
			FilePath: p.FilePath,
			MTime:    p.LastUpdated.Unix(),
			Cycle:    p.Step,
			Params:   p.Params,
			Env:      p.Env,
			Stdin:    p.Stdin,
		}

		ret[key] = plugin
	}

	return ret
}

func ListPluginsFromLocal() map[string]*Plugin {
	dir := sys.Config.Plugin
	ret := make(map[string]*Plugin)

	if dir == "" || !file.IsExist(dir) || file.IsFile(dir) {
		return ret
	}

	fs, err := ioutil.ReadDir(dir)
	if err != nil {
		logger.Error("[E] can not list files under", dir)
		return ret
	}

	for _, f := range fs {
		if f.IsDir() {
			continue
		}

		filename := f.Name()
		arr := strings.Split(filename, "_")
		if len(arr) < 2 {
			logger.Debugf("plugin:%s name illegal, should be: $cycle_$xx", filename)
			continue
		}

		// filename should be: $cycle_$xx
		var cycle int
		cycle, err = strconv.Atoi(arr[0])
		if err != nil {
			logger.Debugf("plugin:%s name illegal, should be: $cycle_$xx %v", filename, err)
			continue
		}

		fpath, err := filepath.Abs(filepath.Join(dir, filename))
		if err != nil {
			logger.Debugf("plugin:%s absolute path get err:%v", filename, err)
			continue
		}

		plugin := &Plugin{FilePath: fpath, MTime: f.ModTime().Unix(), Cycle: cycle}
		ret[fpath] = plugin
	}

	return ret
}
