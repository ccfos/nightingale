package provider

// Slack Webhook 媒介：Webhook 地址在通知规则里填，一个地址对应一个频道。
//
// 成功判定（200 且响应体为 ok）参考 Prometheus Alertmanager notify/slack/slack.go，报错码与说明参考
// Grafana alerting receivers/slack（均为 Apache License 2.0）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

const (
	// Slack 单条消息上限 4 万字符，附件正文过长会折叠显示；8000 足够放下告警正文
	slackMaxTitleRunes = 1024
	slackMaxTextRunes  = 8000
)

type SlackWebhookProvider struct{}

func (p *SlackWebhookProvider) Ident() string { return models.RequestTypeSlackWebhook }

func (p *SlackWebhookProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != models.RequestTypeSlackWebhook {
		return errors.New("slack webhook provider requires request_type: slackwebhook")
	}
	// 媒介配置可以整体为空（Webhook 地址在规则里）
	return nil
}

// webhookAttachment 是 Slack 附件格式的消息卡片：色条、可点击的标题、正文和时间。
// Mattermost 的 Incoming Webhook 兼容同一套字段，两边共用。
type webhookAttachment struct {
	Fallback  string   `json:"fallback"`
	Color     string   `json:"color,omitempty"`
	Title     string   `json:"title,omitempty"`
	TitleLink string   `json:"title_link,omitempty"`
	Text      string   `json:"text,omitempty"`
	MrkdwnIn  []string `json:"mrkdwn_in,omitempty"`
	Footer    string   `json:"footer,omitempty"`
	Ts        int64    `json:"ts,omitempty"`
}

// buildWebhookAttachment 组一张告警卡片：标题固定为「[S级别] Triggered/Recovered: 规则名」，正文是渲染后的消息模板。
// escapeTitle 用于 Slack：标题里的 & < > 必须转义，否则 < 会被当成链接语法。
func buildWebhookAttachment(req *NotifyRequest, maxTitle, maxText int, escapeTitle func(interface{}) string) webhookAttachment {
	var event *models.AlertCurEvent
	if len(req.Events) > 0 {
		event = req.Events[0]
	}
	title := tplString(req.TplContent, "title")
	if title == "" && event != nil {
		title = eventTitle(event)
	}
	title, _ = truncateInRunes(strings.Join(strings.Fields(title), " "), maxTitle)
	if escapeTitle != nil {
		title = escapeTitle(title)
	}
	text, _ := truncateInRunes(tplString(req.TplContent, "content"), maxText)

	a := webhookAttachment{Fallback: title, Title: title, Text: text, Footer: "Nightingale"}
	if event != nil {
		a.Color = fmt.Sprintf("#%06X", severityColor(event.Severity, event.IsRecovered))
		a.TitleLink = eventDetailURL(req.SiteUrl, event)
		a.Ts = event.TriggerTime
		if event.IsRecovered && event.LastEvalTime > 0 {
			a.Ts = event.LastEvalTime
		}
	}
	return a
}

// parseWebhookURL 取规则里的 Webhook 地址并展开变量引用；pathHint 是地址路径里必须出现的一段，
// 不限域名：经反向代理或网关转发的地址也要能填
func parseWebhookURL(raw, pathHint, product, example string) (string, error) {
	v, err := expandUserVars(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", fmt.Errorf("%s webhook_url is required in the notify rule", strings.ToLower(product))
	}
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || !strings.Contains(u.Path, pathHint) {
		return "", withHint(fmt.Errorf("invalid %s webhook_url", strings.ToLower(product)),
			fmt.Sprintf("The %s webhook URL looks like %s", product, example))
	}
	return v, nil
}

func (p *SlackWebhookProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	webhookURL, err := parseWebhookURL(req.CustomParams["webhook_url"], "/services/", "Slack",
		"https://hooks.slack.com/services/T.../B.../...")
	if err != nil {
		return &NotifyResult{Target: maskWebhookURL(req.CustomParams["webhook_url"]), Err: err}
	}
	target := webhookTarget(req.CustomParams["bot_name"], webhookURL)

	var cfg models.SlackWebhookRequestConfig
	if req.Config != nil && req.Config.RequestConfig != nil && req.Config.RequestConfig.SlackWebhookRequestConfig != nil {
		cfg = *req.Config.RequestConfig.SlackWebhookRequestConfig
	}

	a := buildWebhookAttachment(req, slackMaxTitleRunes, slackMaxTextRunes, tplx.SlackEscape)
	a.MrkdwnIn = []string{"text"}
	body, err := json.Marshal(map[string]interface{}{"attachments": []webhookAttachment{a}})
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
		checkSlackWebhook)
	if err != nil {
		// 网络错误的文本会内嵌完整地址，Mattermost 的报错会带上 Webhook ID，落记录前都换成掩码
		msg := maskWebhookSecret(err.Error(), webhookURL)
		return &NotifyResult{Target: target, Err: withHint(errors.New(msg), slackHint(err, resp))}
	}
	return &NotifyResult{Target: target, Response: "sent"}
}

// checkSlackWebhook Slack Webhook 成功时回 200 和纯文本 ok；出错时回 4xx 和纯文本错误码（如 invalid_token）
func checkSlackWebhook(r *nativeResponse) (bool, error) {
	if r.StatusCode == http.StatusOK {
		if strings.TrimSpace(string(r.Body)) == "ok" {
			return false, nil
		}
		return false, &HTTPStatusError{StatusCode: r.StatusCode, Detail: truncateBody(r.Body)}
	}
	return checkStatus(r, nil)
}

var slackHints = map[string]string{
	"invalid_token":                     "The webhook token is wrong; copy the webhook URL again from the Slack app's Incoming Webhooks page",
	"no_service":                        "The webhook was removed or disabled; add a new webhook on the Slack app's Incoming Webhooks page",
	"no_active_hooks":                   "The webhook was removed or disabled; add a new webhook on the Slack app's Incoming Webhooks page",
	"channel_not_found":                 "The channel bound to the webhook was deleted",
	"channel_is_archived":               "The channel bound to the webhook is archived",
	"action_prohibited":                 "A workspace admin restricted posting to this channel",
	"posting_to_general_channel_denied": "The workspace does not allow apps to post to #general",
	"invalid_payload":                   "Slack rejected the message; check the message template",
	"no_text":                           "The message is empty; check the message template",
}

func slackHint(err error, resp *nativeResponse) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		return "Nightingale cannot reach Slack; check the network or set a proxy in the media type"
	}
	if se.StatusCode == http.StatusTooManyRequests {
		return "Rate limited by Slack, retried according to Retry-After"
	}
	if resp != nil {
		if h, ok := slackHints[strings.TrimSpace(string(resp.Body))]; ok {
			return h
		}
	}
	return ""
}
