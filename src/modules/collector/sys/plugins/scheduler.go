package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/sys"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/sys/funcs"
)

type PluginScheduler struct {
	Ticker *time.Ticker
	Plugin *Plugin
	Quit   chan struct{}
}

func NewPluginScheduler(p *Plugin) *PluginScheduler {
	scheduler := PluginScheduler{Plugin: p}
	scheduler.Ticker = time.NewTicker(time.Duration(p.Cycle) * time.Second)
	scheduler.Quit = make(chan struct{})
	return &scheduler
}

func (p *PluginScheduler) Schedule() {
	go func() {
		for {
			select {
			case <-p.Ticker.C:
				PluginRun(p.Plugin)
			case <-p.Quit:
				p.Ticker.Stop()
				return
			}
		}
	}()
}

func (p *PluginScheduler) Stop() {
	close(p.Quit)
}

func PluginRun(plugin *Plugin) {

	timeout := plugin.Cycle*1000 - 500 //比运行周期少500毫秒

	fpath := plugin.FilePath
	if !file.IsExist(fpath) {
		logger.Error("no such plugin:", fpath)
		return
	}

	logger.Debug(fpath, " running")
	params := strings.Split(plugin.Params, " ")
	cmd := exec.Command(fpath, params...)
	cmd.Dir = filepath.Dir(fpath)
	var stdout bytes.Buffer

	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if plugin.Stdin != "" {
		cmd.Stdin = bytes.NewReader([]byte(plugin.Stdin))
	}

	if plugin.Env != "" {
		envs := make(map[string]string)
		err := json.Unmarshal([]byte(plugin.Env), &envs)
		if err != nil {
			logger.Errorf("plugin:%+v %v", plugin, err)
			return
		}
		for k, v := range envs {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	err := cmd.Start()
	if err != nil {
		logger.Error(err)
		return
	}

	err, isTimeout := sys.WrapTimeout(cmd, time.Duration(timeout)*time.Millisecond)

	errStr := stderr.String()
	if errStr != "" {
		logger.Errorf("exec %s fail: %s", fpath, errStr)
		return
	}

	if isTimeout {
		if err == nil {
			logger.Infof("timeout and kill process %s successfully", fpath)
		}

		if err != nil {
			logger.Errorf("kill process %s occur error %v", fpath, err)
		}

		return
	}

	if err != nil {
		logger.Errorf("exec plugin %s occur error: %v", fpath, err)
		return
	}

	// exec successfully
	data := stdout.Bytes()
	if len(data) == 0 {
		logger.Debug("stdout of ", fpath, " is blank")
		return
	}

	logger.Debug(fpath, " stdout: ", string(data))

	var items []*dataobj.MetricValue
	err = json.Unmarshal(data, &items)
	if err != nil {
		logger.Errorf("json.Unmarshal stdout of %s fail. error:%s stdout: %s", fpath, err, stdout.String())
		return
	}

	if len(items) == 0 {
		logger.Debugf("%s item result is empty", fpath)
		return
	}

	for i := 0; i < len(items); i++ {
		items[i].Step = int64(plugin.Cycle)
	}

	funcs.Push(items)
}
