package plugins

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
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

func (this *PluginScheduler) Schedule() {
	go func() {
		for {
			select {
			case <-this.Ticker.C:
				PluginRun(this.Plugin)
			case <-this.Quit:
				this.Ticker.Stop()
				return
			}
		}
	}()
}

func (this *PluginScheduler) Stop() {
	close(this.Quit)
}

func PluginRun(plugin *Plugin) {

	timeout := plugin.Cycle*1000 - 500 //比运行周期少500毫秒

	fpath := plugin.FilePath
	if !file.IsExist(fpath) {
		logger.Error("no such plugin:", fpath)
		return
	}

	logger.Debug(fpath, " running")
	cmd := exec.Command(fpath)
	cmd.Dir = filepath.Dir(fpath)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
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
		logger.Debug("stdout of", fpath, "is blank")
		return
	}

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

	funcs.Push(items)
}
