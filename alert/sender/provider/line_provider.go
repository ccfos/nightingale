package provider

// LINE 媒介：用 Messaging API 的推送接口（POST /v2/bot/message/push，
// https://developers.line.biz/en/reference/messaging-api/#send-push-message）。LINE Notify 已于 2025 年 3 月停止服务，
// 所以走官方账号：Channel Access Token 在媒介里配，接收方（用户 / 群组 / 多人聊天的 ID）在通知规则里填。
//
// 每次发送带一个 X-Line-Retry-Key：超时或 5xx 后重试时 LINE 据此去重，已经受理过的请求回 409，按成功处理。
// 接口字段见官方 OpenAPI 定义 line/line-openapi 的 messaging-api.yml，文本组织参考 Uptime Kuma 的 line.js（MIT）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/google/uuid"
)

const (
	lineDefaultAPIURL = "https://api.line.me"
	linePushPath      = "/v2/bot/message/push"
	// 文本消息上限 5000 字符，按 UTF-16 码元计
	lineMaxTextUnits = 5000
	lineMaxTitleRune = 300
)

// 用户 ID 以 U 开头、群组以 C 开头、多人聊天以 R 开头，后面是 32 位十六进制
var lineToRe = regexp.MustCompile(`^[UCR][0-9a-f]{32}$`)

type LineProvider struct{}

func (p *LineProvider) Ident() string { return models.RequestTypeLine }

func (p *LineProvider) Check(config *models.NotifyChannelConfig) error {
	if config == nil || config.RequestType != models.RequestTypeLine {
		return errors.New("line provider requires request_type: line")
	}
	if config.RequestConfig == nil || config.RequestConfig.LineRequestConfig == nil {
		return errors.New("line request config cannot be nil")
	}
	c := config.RequestConfig.LineRequestConfig
	if strings.TrimSpace(c.ChannelAccessToken) == "" {
		return errors.New("line channel access token is required")
	}
	if err := checkAPIURL("line api", c.APIURL); err != nil {
		return err
	}
	return checkNativeRetry("line", &c.NativeNetworkConfig)
}

type linePushRequest struct {
	To       string        `json:"to"`
	Messages []lineMessage `json:"messages"`
}

type lineMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (p *LineProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	// 接收方 ID 不是凭证，但也不好认：有名称显示名称，没有时只留后 4 位
	target := namedTarget(req.CustomParams["bot_name"], req.CustomParams["to"])
	if err := p.Check(req.Config); err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	if len(req.Events) == 0 {
		return &NotifyResult{Target: target, Err: errors.New("events is empty")}
	}
	cfg := *req.Config.RequestConfig.LineRequestConfig
	for _, field := range []*string{&cfg.ChannelAccessToken, &cfg.APIURL} {
		v, err := expandUserVars(strings.TrimSpace(*field))
		if err != nil {
			return &NotifyResult{Target: target, Err: err}
		}
		*field = v
	}
	to, err := expandUserVars(strings.TrimSpace(req.CustomParams["to"]))
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	if to == "" {
		return &NotifyResult{Target: target, Err: errors.New("line recipient id (to) is required in the notify rule")}
	}
	if !lineToRe.MatchString(to) {
		return &NotifyResult{Target: target, Err: withHint(errors.New("invalid line recipient id"),
			"A LINE recipient ID starts with U (user), C (group chat) or R (multi-person chat) followed by 32 hexadecimal characters; it is not the LINE ID shown in the app")}
	}
	target = namedTarget(req.CustomParams["bot_name"], to)

	body, err := json.Marshal(linePushRequest{To: to, Messages: []lineMessage{{Type: "text", Text: buildLineText(req, req.Events[0])}}})
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	base := strings.TrimRight(cfg.APIURL, "/")
	if base == "" {
		base = lineDefaultAPIURL
	}
	// 同一次通知的所有重试共用一个 key，LINE 据此保证只投递一次
	retryKey := uuid.NewString()

	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)
	resp, err := doWithRetry(ctx, req.HttpClient, retryTimes, sleep,
		func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+linePushPath, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+cfg.ChannelAccessToken)
			r.Header.Set("X-Line-Retry-Key", retryKey)
			return r, nil
		}, checkLine)
	if err != nil {
		return &NotifyResult{Target: target, Err: withHint(errors.New(redactSecrets(err.Error(), cfg.ChannelAccessToken)), lineHint(err))}
	}
	if resp.StatusCode == http.StatusConflict {
		return &NotifyResult{Target: target, Response: "sent (accepted by LINE on an earlier attempt)"}
	}
	var sent struct {
		SentMessages []struct {
			ID string `json:"id"`
		} `json:"sentMessages"`
	}
	_ = json.Unmarshal(resp.Body, &sent)
	response := "sent"
	if len(sent.SentMessages) > 0 && sent.SentMessages[0].ID != "" {
		response = "sent, message id " + sent.SentMessages[0].ID
	}
	return &NotifyResult{Target: target, Response: response}
}

// buildLineText 组一条纯文本消息：带级别符号的标题、正文、事件详情链接（LINE 会把链接识别成可点击的）
func buildLineText(req *NotifyRequest, event *models.AlertCurEvent) string {
	parts := []string{severityEmoji(event.Severity, event.IsRecovered) + " " + firstEventTitle(req, event, lineMaxTitleRune)}
	if content := tplString(req.TplContent, "content"); content != "" {
		parts = append(parts, content)
	}
	text := strings.Join(parts, "\n\n")
	if detail := eventDetailURL(req.SiteUrl, event); detail != "" {
		// 先给链接留出位置再截正文，免得链接被截断
		room := lineMaxTextUnits - len([]rune(detail)) - 2
		text, _ = truncateInUTF16(text, room)
		return text + "\n\n" + detail
	}
	text, _ = truncateInUTF16(text, lineMaxTextUnits)
	return text
}

type lineError struct {
	Message string `json:"message"`
	Details []struct {
		Message  string `json:"message"`
		Property string `json:"property"`
	} `json:"details"`
}

func lineErrorDetail(r *nativeResponse) string {
	var e lineError
	if err := json.Unmarshal(r.Body, &e); err != nil || e.Message == "" {
		return truncateBody(r.Body)
	}
	s := e.Message
	for _, d := range e.Details {
		s += "; " + strings.TrimSpace(d.Property+" "+d.Message)
	}
	s, _ = truncateInRunes(s, 512)
	return s
}

// checkLine：200 成功；409 表示带同一个重试 key 的请求已被受理，也算成功；
// 429 里「当月额度用完」不重试，普通限流和 5xx 重试
func checkLine(r *nativeResponse) (bool, error) {
	if r.StatusCode == http.StatusConflict {
		return false, nil
	}
	retry, err := checkStatus(r, lineErrorDetail)
	if err != nil && r.StatusCode == http.StatusTooManyRequests && strings.Contains(strings.ToLower(string(r.Body)), "monthly limit") {
		retry = false
	}
	return retry, err
}

func lineHint(err error) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		return "Nightingale cannot reach LINE; check the network or set a proxy in the media type"
	}
	detail := strings.ToLower(se.Detail)
	switch se.StatusCode {
	case http.StatusUnauthorized:
		return "The channel access token is wrong or expired; issue a new one in the LINE Developers Console (Messaging API tab)"
	case http.StatusForbidden:
		return "The LINE Official Account is not allowed to use this API; check the account's plan and status"
	case http.StatusBadRequest:
		if strings.Contains(detail, "failed to send") || strings.Contains(detail, "'to'") {
			return "LINE cannot deliver to this recipient: check the ID, and that the user has added the official account as a friend (or the account has joined the group)"
		}
		return "LINE rejected the message; check the message template"
	case http.StatusTooManyRequests:
		if strings.Contains(detail, "monthly limit") {
			return "The LINE Official Account has used up its monthly message quota"
		}
		return "Rate limited by LINE, retried automatically"
	}
	return ""
}
