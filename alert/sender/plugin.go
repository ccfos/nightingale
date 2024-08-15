package sender

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/sys"
)

func MayPluginNotify(ctx *ctx.Context, noticeBytes []byte, notifyScript models.NotifyScript,
	stats *astats.Stats, event *models.AlertCurEvent) {
	if len(noticeBytes) == 0 {
		return
	}
	alertingCallScript(ctx, noticeBytes, notifyScript, stats, event)
}

func alertingCallScript(ctx *ctx.Context, stdinBytes []byte, notifyScript models.NotifyScript,
	stats *astats.Stats, event *models.AlertCurEvent) {
	// not enable or no notify.py? do nothing
	config := notifyScript
	if !config.Enable || config.Content == "" {
		return
	}

	channel := "script"
	stats.AlertNotifyTotal.WithLabelValues(channel).Inc()
	fpath := ".notify_scriptt"
	if config.Type == 1 {
		fpath = config.Content
	} else {
		rewrite := true
		if file.IsExist(fpath) {
			oldContent, err := file.ToString(fpath)
			if err != nil {
				logger.Errorf("event_script_notify_fail: read script file err: %v", err)
				stats.AlertNotifyErrorTotal.WithLabelValues(channel).Inc()
				return
			}

			if oldContent == config.Content {
				rewrite = false
			}
		}

		if rewrite {
			_, err := file.WriteString(fpath, config.Content)
			if err != nil {
				logger.Errorf("event_script_notify_fail: write script file err: %v", err)
				stats.AlertNotifyErrorTotal.WithLabelValues(channel).Inc()
				return
			}

			err = os.Chmod(fpath, 0777)
			if err != nil {
				logger.Errorf("event_script_notify_fail: chmod script file err: %v", err)
				stats.AlertNotifyErrorTotal.WithLabelValues(channel).Inc()
				return
			}
		}
		fpath = "./" + fpath
	}

	cmd := exec.Command(fpath)
	cmd.Stdin = bytes.NewReader(stdinBytes)

	// combine stdout and stderr
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := startCmd(cmd)
	if err != nil {
		logger.Errorf("event_script_notify_fail: run cmd err: %v", err)
		return
	}

	err, isTimeout := sys.WrapTimeout(cmd, time.Duration(config.Timeout)*time.Second)
	NotifyRecord(ctx, event, channel, cmd.String(), "", buildErr(err, isTimeout))

	if isTimeout {
		if err == nil {
			logger.Errorf("event_script_notify_fail: timeout and killed process %s", fpath)
		}

		if err != nil {
			logger.Errorf("event_script_notify_fail: kill process %s occur error %v", fpath, err)
			stats.AlertNotifyErrorTotal.WithLabelValues(channel).Inc()
		}
		return
	}

	if err != nil {
		logger.Errorf("event_script_notify_fail: exec script %s occur error: %v, output: %s", fpath, err, buf.String())
		stats.AlertNotifyErrorTotal.WithLabelValues(channel).Inc()
		return
	}

	logger.Infof("event_script_notify_ok: exec %s output: %s", fpath, buf.String())
}

func buildErr(err error, isTimeout bool) error {
	if err == nil && !isTimeout {
		return nil
	} else {
		return fmt.Errorf("is_timeout: %v, err: %v", isTimeout, err)
	}
}
