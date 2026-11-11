package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	target, response, err := SendScript(ctx, req.Config, req.Events, req.TplContent, req.CustomParams, req.Sendtos, req.HttpClient)
	targetStr := target
	if targetStr == "" {
		targetStr = strings.Join(req.Sendtos, ",")
	}
	return &NotifyResult{Target: targetStr, Response: response, Err: err}
}

// scriptUploadTransport adds configured headers without modifying shared requests.
type scriptUploadTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *scriptUploadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for name, value := range t.headers {
		req.Header.Set(name, value)
	}
	return t.base.RoundTrip(req)
}

func prepareScriptEvents(ctx context.Context, ncc *models.NotifyChannelConfig, events []*models.AlertCurEvent, client *http.Client) []*models.AlertCurEvent {
	// The proxy address is applied by models.GetHTTPClient independently of headers.
	proxy := ncc.RequestConfig.ScriptRequestConfig.Proxy
	var headers map[string]string
	if proxy != nil {
		headers = proxy.Headers
	}
	if client != nil && len(headers) > 0 {
		copied := *client
		base := copied.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		copied.Transport = &scriptUploadTransport{base: base, headers: headers}
		client = &copied
	}
	var tokenOnce sync.Once
	var token string
	var tokenErr error
	uploadCfg := ncc.RequestConfig.ScriptRequestConfig.ImageUpload
	return prepareImageEvents(ctx, events, func(imageBase64 string) (string, error) {
		if uploadCfg.Platform == "dingtalk" {
			tokenOnce.Do(func() {
				token, tokenErr = GetAccessToken(ctx, client, uploadCfg.DingtalkRequestConfig.AppKey, uploadCfg.DingtalkRequestConfig.AppSecret)
			})
			if tokenErr != nil {
				return "", tokenErr
			}
			return UploadMedia(ctx, client, token, "image", imageBase64)
		}
		if uploadCfg.Platform == "feishu" {
			tokenOnce.Do(func() {
				token, tokenErr = GetFeishuTenantAccessToken(ctx, client, uploadCfg.FeishuRequestConfig.AppID, uploadCfg.FeishuRequestConfig.AppSecret)
			})
			if tokenErr != nil {
				return "", tokenErr
			}
			return UploadFeishuImage(ctx, client, token, imageBase64)
		}
		if uploadCfg.Platform == "flashduty" {
			if client == nil {
				return "", errors.New("http client not found")
			}
			return uploadFlashDutyImage(ctx, uploadCfg.FlashDutyRequestConfig.IntegrationUrl, imageBase64, client)
		}
		return "", errors.New("image upload config not found")
	})
}

// prepareImageEvents copies events and replaces the inline base64 screenshots with the
// image key returned by upload, so other notification channels can still use the
// original screenshots and the script receives a much smaller payload.
func prepareImageEvents(ctx context.Context, events []*models.AlertCurEvent, upload func(imageBase64 string) (string, error)) []*models.AlertCurEvent {
	prepared := make([]*models.AlertCurEvent, len(events))
	var uploads sync.WaitGroup
	// Bound concurrent uploads to avoid excessive memory usage for large batches.
	uploadSlots := make(chan struct{}, 5)
	for i, event := range events {
		if event == nil {
			continue
		}
		copied := *event
		// Never pass inline image data to the script when an upload was requested.
		copied.ShotImageBase64 = nil
		prepared[i] = &copied
		// The snapshot field holds one image. Pick it consistently when multiple screenshots exist.
		keys := make([]string, 0, len(event.ShotImageBase64))
		for key := range event.ShotImageBase64 {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			imageBase64 := event.ShotImageBase64[key]
			if strings.TrimSpace(imageBase64) == "" {
				continue
			}
			var ruleConfig map[string]json.RawMessage
			configJSON := []byte(event.RuleConfig)
			if event.RuleConfigJson != nil {
				var err error
				configJSON, err = json.Marshal(event.RuleConfigJson)
				if err != nil {
					logger.Warningf("send_script: skip image attachment event_id=%d: marshal rule config: %v", event.Id, err)
					break
				}
			}
			if len(configJSON) > 0 {
				if err := json.Unmarshal(configJSON, &ruleConfig); err != nil {
					logger.Warningf("send_script: skip image attachment event_id=%d: parse rule config: %v", event.Id, err)
					break
				}
			}
			if ruleConfig == nil {
				ruleConfig = make(map[string]json.RawMessage)
			}
			uploadSlots <- struct{}{}
			uploads.Add(1)
			go func() {
				defer uploads.Done()
				defer func() { <-uploadSlots }()
				if ctx.Err() != nil {
					return
				}
				imageKey, err := upload(imageBase64)
				if err != nil {
					// Screenshots are optional: keep executing the script even if uploading fails.
					logger.Warningf("send_script: upload image failed event_id=%d: %v", event.Id, err)
					return
				}
				ruleConfig["snapshot_pic_url"], _ = json.Marshal(imageKey)
				copied.RuleConfigJson = ruleConfig
				copied.AnnotationsJSON = make(map[string]string, len(event.AnnotationsJSON)+1)
				for name, value := range event.AnnotationsJSON {
					copied.AnnotationsJSON[name] = value
				}
				copied.AnnotationsJSON["img_key"] = imageKey
			}()
			break
		}
	}
	uploads.Wait()
	return prepared
}

func SendScript(ctx context.Context, ncc *models.NotifyChannelConfig, events []*models.AlertCurEvent, tpl map[string]interface{}, params map[string]string, sendtos []string, client *http.Client) (string, string, error) {
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

	if config.ImageUpload.Verify() == nil {
		events = prepareScriptEvents(ctx, ncc, events, client)
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
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
