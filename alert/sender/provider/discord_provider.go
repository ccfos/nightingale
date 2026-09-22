package provider

// Discord 媒介：Webhook 地址在通知规则里填，一条通知配置发一个频道（或论坛帖子 / 线程）。
//
// 消息结构、长度上限与颜色参考 Prometheus Alertmanager notify/discord/discord.go
// （Apache License 2.0）；论坛帖子 / 线程、静默推送与 @ 白名单参考 Uptime Kuma 与 Grafana 的实现。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

const (
	discordMaxTitleRunes       = 256
	discordMaxDescriptionRunes = 4096
	discordMaxEmbedRunes       = 6000
	discordMaxThreadNameRunes  = 100
	discordFlagSuppressNotify  = 1 << 12

	discordTargetChannel   = "channel"
	discordTargetForumPost = "forum_post"
	discordTargetThread    = "thread"
)

var discordThreadRe = regexp.MustCompile(`^\d+$`)

type DiscordProvider struct{}

func (p *DiscordProvider) Ident() string { return models.RequestTypeDiscord }

func (p *DiscordProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != models.RequestTypeDiscord {
		return errors.New("discord provider requires request_type: discord")
	}
	// 媒介配置可以整体为空（Webhook 地址在规则里），不能要求非 nil
	return config.ValidateDiscordRequestConfig()
}

type discordWebhook struct {
	Username        string                 `json:"username,omitempty"`
	AvatarURL       string                 `json:"avatar_url,omitempty"`
	Embeds          []discordEmbed         `json:"embeds"`
	AllowedMentions discordAllowedMentions `json:"allowed_mentions"`
	Flags           int                    `json:"flags,omitempty"`
	ThreadName      string                 `json:"thread_name,omitempty"`
}

type discordEmbed struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	Color       int    `json:"color"`
	Timestamp   string `json:"timestamp,omitempty"`
}

// discordAllowedMentions 的 parse 恒为空：告警内容里出现的 @everyone、<@id> 都不会触发提醒
type discordAllowedMentions struct {
	Parse []string `json:"parse"`
}

type discordParams struct {
	WebhookURL string
	Name       string
	Target     string
	ThreadName string
	ThreadID   string
}

func parseDiscordParams(p map[string]string) (*discordParams, error) {
	dp := &discordParams{
		Name:       strings.TrimSpace(p["bot_name"]),
		Target:     strings.TrimSpace(p["target"]),
		ThreadName: strings.TrimSpace(p["thread_name"]),
		ThreadID:   strings.TrimSpace(p["thread_id"]),
	}
	raw, err := expandUserVars(strings.TrimSpace(p["webhook_url"]))
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, errors.New("discord webhook_url is required in the notify rule")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.Contains(u.Path, "/webhooks/") {
		return nil, withHint(errors.New("invalid discord webhook_url"), "The Discord webhook URL looks like https://discord.com/api/webhooks/<id>/<token>")
	}
	dp.WebhookURL = raw

	switch dp.Target {
	case "":
		dp.Target = discordTargetChannel
	case discordTargetChannel, discordTargetForumPost, discordTargetThread:
	default:
		return nil, fmt.Errorf("invalid discord target %q, must be channel, forum_post or thread", dp.Target)
	}
	if dp.Target == discordTargetThread && !discordThreadRe.MatchString(dp.ThreadID) {
		return nil, errors.New("discord thread_id must be a numeric ID when sending to an existing thread")
	}
	return dp, nil
}

func (p *DiscordProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	params, err := parseDiscordParams(req.CustomParams)
	if err != nil {
		return &NotifyResult{Target: maskWebhookURL(req.CustomParams["webhook_url"]), Err: err}
	}
	target := webhookTarget(params.Name, params.WebhookURL)

	var cfg models.DiscordRequestConfig
	if req.Config != nil && req.Config.RequestConfig != nil && req.Config.RequestConfig.DiscordRequestConfig != nil {
		cfg = *req.Config.RequestConfig.DiscordRequestConfig
	}

	payload := buildDiscordPayload(req, &cfg, params)
	body, err := json.Marshal(payload)
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}

	u, _ := url.Parse(params.WebhookURL)
	q := u.Query()
	// wait=true 让 Discord 同步返回结果；不带时消息没保存成功也会回 204
	q.Set("wait", "true")
	if params.Target == discordTargetThread {
		q.Set("thread_id", params.ThreadID)
	}
	u.RawQuery = q.Encode()
	endpoint := u.String()

	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)
	resp, err := doWithRetry(ctx, req.HttpClient, retryTimes, sleep,
		func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			return r, nil
		},
		func(r *nativeResponse) (bool, error) { return checkStatus(r, discordErrorDetail) })
	if err != nil {
		// 网络错误的文本会内嵌完整地址（含 token），落记录前换成掩码
		msg := strings.ReplaceAll(err.Error(), endpoint, maskWebhookURL(params.WebhookURL))
		return &NotifyResult{Target: target, Err: withHint(errors.New(msg), discordHint(err, resp))}
	}

	var msg struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(resp.Body, &msg)
	response := "sent"
	if msg.ID != "" {
		response = "sent, message id " + msg.ID
	}
	return &NotifyResult{Target: target, Response: response}
}

func buildDiscordPayload(req *NotifyRequest, cfg *models.DiscordRequestConfig, params *discordParams) *discordWebhook {
	var event *models.AlertCurEvent
	if len(req.Events) > 0 {
		event = req.Events[0]
	}

	title := tplString(req.TplContent, "title")
	if title == "" && event != nil {
		title = eventTitle(event)
	}
	title, _ = truncateInRunes(strings.Join(strings.Fields(title), " "), discordMaxTitleRunes)

	// embed 的所有文本合计不能超过 6000 字符
	descMax := discordMaxDescriptionRunes
	if room := discordMaxEmbedRunes - len([]rune(title)); room < descMax {
		descMax = room
	}
	desc, _ := truncateInRunes(tplString(req.TplContent, "content"), descMax)

	embed := discordEmbed{Title: title, Description: desc}
	if event != nil {
		embed.Color = severityColor(event.Severity, event.IsRecovered)
		embed.URL = eventDetailURL(req.SiteUrl, event)
		ts := event.TriggerTime
		if event.IsRecovered && event.LastEvalTime > 0 {
			ts = event.LastEvalTime
		}
		if ts > 0 {
			embed.Timestamp = time.Unix(ts, 0).UTC().Format(time.RFC3339)
		}
	}

	w := &discordWebhook{
		Username:        strings.TrimSpace(cfg.Username),
		AvatarURL:       strings.TrimSpace(cfg.AvatarURL),
		Embeds:          []discordEmbed{embed},
		AllowedMentions: discordAllowedMentions{Parse: []string{}},
	}
	if cfg.Silent {
		w.Flags = discordFlagSuppressNotify
	}
	if params.Target == discordTargetForumPost {
		name := params.ThreadName
		if strings.Contains(name, "{{") {
			tpl := &models.MessageTemplate{Content: map[string]string{"thread_name": name}}
			name = tplString(tpl.RenderEventPlain(req.Events, req.SiteUrl), "thread_name")
		}
		if name == "" {
			name = title
		}
		w.ThreadName, _ = truncateInRunes(strings.Join(strings.Fields(name), " "), discordMaxThreadNameRunes)
	}
	return w
}

// tplString 取已渲染模板里的字段，没有时返回空串
func tplString(tpl map[string]interface{}, key string) string {
	if tpl == nil {
		return ""
	}
	if v, ok := tpl[key]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

// discordErrorDetail 解析 Discord 的错误响应：{"code":50035,"message":"...","errors":{...}}
func discordErrorDetail(r *nativeResponse) string {
	var e struct {
		Code       int             `json:"code"`
		Message    string          `json:"message"`
		Errors     json.RawMessage `json:"errors"`
		RetryAfter float64         `json:"retry_after"`
	}
	if err := json.Unmarshal(r.Body, &e); err != nil || (e.Code == 0 && e.Message == "") {
		return truncateBody(r.Body)
	}
	s := fmt.Sprintf("%d %s", e.Code, e.Message)
	if len(e.Errors) > 0 {
		s += ": " + truncateBody(e.Errors)
	}
	return s
}

var discordHints = map[string]string{
	"10015":  "The webhook was deleted in Discord, create a new one",
	"50027":  "The webhook URL is incomplete or the webhook token was reset",
	"50035":  "The message is too long or malformed; see the field path in the error",
	"220001": "This is a forum channel: choose New forum post or Existing thread as the target",
	"220003": "This is not a forum channel: New forum post cannot be used",
	"160005": "The thread is locked",
}

func discordHint(err error, resp *nativeResponse) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		return "Nightingale cannot reach Discord; check the network or set a proxy in the media type"
	}
	if se.StatusCode == http.StatusTooManyRequests {
		return "Rate limited by Discord, retried according to Retry-After"
	}
	if resp != nil {
		var e struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(resp.Body, &e) == nil {
			if h, ok := discordHints[fmt.Sprint(e.Code)]; ok {
				return h
			}
		}
	}
	return ""
}
