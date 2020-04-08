package plugins

import (
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

// key: 60_ntp.py
func ListPlugins(dir string) map[string]*Plugin {
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
			logger.Warningf("plugin:%s name illegal, should be: $cycle_$xx", filename)
			continue
		}

		// filename should be: $cycle_$xx
		var cycle int
		cycle, err = strconv.Atoi(arr[0])
		if err != nil {
			logger.Warningf("plugin:%s name illegal, should be: $cycle_$xx %v", filename, err)
			continue
		}

		fpath, err := filepath.Abs(filepath.Join(dir, filename))
		if err != nil {
			logger.Warningf("plugin:%s absolute path get err:%v", filename, err)
			continue
		}

		plugin := &Plugin{FilePath: fpath, MTime: f.ModTime().Unix(), Cycle: cycle}
		ret[fpath] = plugin
	}

	return ret
}
