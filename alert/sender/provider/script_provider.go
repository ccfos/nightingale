package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cmdx"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

const ScriptIdent = "script"

type ScriptProvider struct{}

func (p *ScriptProvider) Ident() string {
	return ScriptIdent
}

func (p *ScriptProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != ScriptIdent {
		return errors.New("script provider requires request_type: script")
	}
	return config.ValidateScriptRequestConfig()
}

func (p *ScriptProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req.Config.RequestConfig == nil || req.Config.RequestConfig.ScriptRequestConfig == nil {
		return &NotifyResult{Target: "", Response: "", Err: errors.New("script request config not found")}
	}
	target, response, err := SendScript(req.Config, req.Events, req.TplContent, req.CustomParams, req.Sendtos)
	targetStr := target
	if targetStr == "" {
		targetStr = strings.Join(req.Sendtos, ",")
	}
	return &NotifyResult{Target: targetStr, Response: response, Err: err}
}

func SendScript(ncc *models.NotifyChannelConfig, events []*models.AlertCurEvent, tpl map[string]interface{}, params map[string]string, sendtos []string) (string, string, error) {
	config := ncc.RequestConfig.ScriptRequestConfig
	if config.Script == "" && config.Path == "" {
		return "", "", fmt.Errorf("script or path is empty")
	}

	fpath := ".notify_script_" + strconv.FormatInt(ncc.ID, 10)
	if config.Path != "" {
		fpath = config.Path
	} else {
		rewrite := true
		if file.IsExist(fpath) {
			oldContent, err := file.ToString(fpath)
			if err != nil {
				return "", "", fmt.Errorf("failed to read script file: %v", err)
			}

			if oldContent == config.Script {
				rewrite = false
			}
		}

		if rewrite {
			_, err := file.WriteString(fpath, config.Script)
			if err != nil {
				return "", "", fmt.Errorf("failed to write script file: %v", err)
			}

			err = os.Chmod(fpath, 0777)
			if err != nil {
				return "", "", fmt.Errorf("failed to chmod script file: %v", err)
			}
		}

		cur, _ := os.Getwd()
		fpath = path.Join(cur, fpath)
	}

	cmd := exec.Command(fpath)
	cmd.Stdin = bytes.NewReader(getStdinBytes(events, tpl, params, sendtos))

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err, isTimeout := cmdx.RunTimeout(cmd, time.Duration(config.Timeout)*time.Millisecond)
	logger.Infof("event_script_notify_result: exec %s output: %s isTimeout: %v err: %v stdin: %s", fpath, buf.String(), isTimeout, err, string(getStdinBytes(events, tpl, params, sendtos)))

	res := buf.String()

	// 截断超出长度的输出
	if len(res) > 512 {
		// 确保在有效的UTF-8字符边界处截断
		validLen := 0
		for i := 0; i < 512 && i < len(res); {
			_, size := utf8.DecodeRuneInString(res[i:])
			if i+size > 512 {
				break
			}
			i += size
			validLen = i
		}
		res = res[:validLen] + "..."
	}

	if isTimeout {
		if err == nil {
			return cmd.String(), res, errors.New("timeout and killed process")
		}

		return cmd.String(), res, err
	}
	if err != nil {
		return cmd.String(), res, fmt.Errorf("failed to execute script: %v", err)
	}

	return cmd.String(), res, nil
}

func getStdinBytes(events []*models.AlertCurEvent, tpl map[string]interface{}, params map[string]string, sendtos []string) []byte {
	if len(events) == 0 {
		return []byte("")
	}

	// 创建一个 map 来存储所有数据
	data := map[string]interface{}{
		"event":   events[0],
		"events":  events,
		"tpl":     tpl,
		"params":  params,
		"sendtos": sendtos,
	}

	// 将数据序列化为 JSON 字节数组
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	return jsonBytes
}
