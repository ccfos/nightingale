package sender

import (
	"bytes"
	"os/exec"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/notifier"
	"github.com/didi/nightingale/v5/src/pkg/sys"
	"github.com/didi/nightingale/v5/src/server/config"
)

func MayPluginNotify(noticeBytes []byte) {
	if len(noticeBytes) == 0 {
		return
	}
	alertingCallPlugin(noticeBytes)
	alertingCallScript(noticeBytes)
}

func alertingCallScript(stdinBytes []byte) {
	// not enable or no notify.py? do nothing
	if !config.C.Alerting.CallScript.Enable || config.C.Alerting.CallScript.ScriptPath == "" {
		return
	}

	fpath := config.C.Alerting.CallScript.ScriptPath
	cmd := exec.Command(fpath)
	cmd.Stdin = bytes.NewReader(stdinBytes)

	// combine stdout and stderr
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := startCmd(cmd)
	if err != nil {
		logger.Errorf("event_notify: run cmd err: %v", err)
		return
	}

	err, isTimeout := sys.WrapTimeout(cmd, time.Duration(config.C.Alerting.Timeout)*time.Millisecond)

	if isTimeout {
		if err == nil {
			logger.Errorf("event_notify: timeout and killed process %s", fpath)
		}

		if err != nil {
			logger.Errorf("event_notify: kill process %s occur error %v", fpath, err)
		}

		return
	}

	if err != nil {
		logger.Errorf("event_notify: exec script %s occur error: %v, output: %s", fpath, err, buf.String())
		return
	}

	logger.Infof("event_notify: exec %s output: %s", fpath, buf.String())
}

// call notify.so via golang plugin build
// ig. etc/script/notify/notify.so
func alertingCallPlugin(stdinBytes []byte) {
	if !config.C.Alerting.CallPlugin.Enable {
		return
	}

	logger.Debugf("alertingCallPlugin begin")
	logger.Debugf("payload:", string(stdinBytes))
	notifier.Instance.Notify(stdinBytes)
	logger.Debugf("alertingCallPlugin done")
}
