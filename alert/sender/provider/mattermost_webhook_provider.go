package provider

// Mattermost Webhook 媒介：Webhook 地址在通知规则里填，一个地址对应一个频道。
//
// 消息结构与长度上限参考 Prometheus Alertmanager notify/mattermost/mattermost.go（Apache License 2.0）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

const (
	mattermostMaxTitleRunes = 1024
	mattermostMaxTextRunes  = 16383
)

type MattermostWebhookProvider struct{}

func (p *MattermostWebhookProvider) Ident() string { return models.RequestTypeMattermostWebhook }

func (p *MattermostWebhookProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != models.RequestTypeMattermostWebhook {
		return errors.New("mattermost webhook provider requires request_type: mattermostwebhook")
	}
	// 媒介配置可以整体为空（Webhook 地址在规则里），只校验填了的图标
	return config.ValidateMattermostWebhookRequestConfig()
}

type mattermostWebhook struct {
	Username    string              `json:"username,omitempty"`
	IconURL     string              `json:"icon_url,omitempty"`
	IconEmoji   string              `json:"icon_emoji,omitempty"`
	Attachments []webhookAttachment `json:"attachments"`
}

func (p *MattermostWebhookProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	webhookURL, err := parseWebhookURL(req.CustomParams["webhook_url"], "/hooks/", "Mattermost",
		"https://mattermost.example.com/hooks/<id>")
	if err != nil {
		return &NotifyResult{Target: maskWebhookURL(req.CustomParams["webhook_url"]), Err: err}
	}
	target := webhookTarget(req.CustomParams["bot_name"], webhookURL)

	var cfg models.MattermostWebhookRequestConfig
	if req.Config != nil && req.Config.RequestConfig != nil && req.Config.RequestConfig.MattermostWebhookRequestConfig != nil {
		cfg = *req.Config.RequestConfig.MattermostWebhookRequestConfig
	}

	w := mattermostWebhook{
		Username:    strings.TrimSpace(cfg.Username),
		Attachments: []webhookAttachment{buildWebhookAttachment(req, mattermostMaxTitleRunes, mattermostMaxTextRunes, nil)},
	}
	// 图标填图片地址或 emoji 代码，按是否以 http 开头区分
	if icon := strings.TrimSpace(cfg.Icon); strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
		w.IconURL = icon
	} else {
		w.IconEmoji = strings.Trim(icon, ":")
	}
	body, err := json.Marshal(w)
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}

	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)
	resp, err := doWithRetry(ctx, req.HttpClient, retryTimes, sleep,
		func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			return r, nil
		},
		func(r *nativeResponse) (bool, error) { return checkStatus(r, mattermostErrorDetail) })
	if err != nil {
		// 网络错误的文本会内嵌完整地址，Mattermost 的报错会带上 Webhook ID，落记录前都换成掩码
		msg := maskWebhookSecret(err.Error(), webhookURL)
		return &NotifyResult{Target: target, Err: withHint(errors.New(msg), mattermostHint(err, resp))}
	}
	return &NotifyResult{Target: target, Response: "sent"}
}

type mattermostError struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// mattermostErrorDetail 解析 Mattermost 的错误响应：{"id":"web.incoming_webhook.invalid.app_error","message":"..."}
func mattermostErrorDetail(r *nativeResponse) string {
	var e mattermostError
	if err := json.Unmarshal(r.Body, &e); err != nil || e.ID == "" {
		return truncateBody(r.Body)
	}
	return fmt.Sprintf("%s %s", e.ID, e.Message)
}

var mattermostHints = map[string]string{
	"web.incoming_webhook.invalid.app_error":  "The webhook URL is wrong or the webhook was deleted",
	"web.incoming_webhook.disabled.app_error": "Incoming webhooks are turned off; ask an admin to enable them in System Console > Integrations > Integration Management",
	"web.incoming_webhook.parse.app_error":    "Mattermost rejected the message; check the message template",
	"web.incoming_webhook.decode.app_error":   "Mattermost rejected the message; check the message template",
	"web.incoming_webhook.text.app_error":     "The message is empty; check the message template",
	// 地址里的 Webhook ID 不存在、消息为空都会落到这个笼统的错误码上
	"web.incoming_webhook.general.app_error": "Mattermost could not handle the message; check that the webhook URL is complete and the webhook still exists",
}

func mattermostHint(err error, resp *nativeResponse) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		if strings.Contains(err.Error(), "x509") || strings.Contains(err.Error(), "certificate") {
			return "The Mattermost server uses a certificate Nightingale does not trust; enable skip certificate verification in the media type's advanced settings"
		}
		return "Nightingale cannot reach Mattermost; check the network or set a proxy in the media type"
	}
	if se.StatusCode == http.StatusTooManyRequests {
		return "Rate limited by Mattermost, retried according to Retry-After"
	}
	if resp != nil {
		var e mattermostError
		if json.Unmarshal(resp.Body, &e) == nil {
			if h, ok := mattermostHints[e.ID]; ok {
				return h
			}
		}
	}
	return ""
}
