package provider

// Pushover 媒介：应用的 API Token 在媒介里配，收消息的 User Key / Group Key 在通知规则里填。
//
// 接口见 https://pushover.net/api ：表单 POST /1/messages.json，成功时回 200 和 {"status":1}。
// 4xx（含 429，表示当月额度用完）重发也不会成功，官方要求不要重试，否则会被封 IP；5xx 至少等 5 秒再试。
// 优先级固定为 S1→High（1）、S2/S3→Normal（0），恢复通知使用 Normal，不使用 Emergency。
// 字段与长度上限参考 Prometheus Alertmanager notify/pushover/pushover.go（Apache License 2.0）。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

const (
	pushoverDefaultAPIURL = "https://api.pushover.net"

	pushoverMaxTitleRunes   = 250
	pushoverMaxMessageRunes = 1024
	pushoverMaxURLRunes     = 512
	pushoverMaxUserKeys     = 50
)

// pushoverMinRetryWait 是 5xx 重试前至少等待的时间（官方要求不少于 5 秒），单测里会调小
var pushoverMinRetryWait = 5 * time.Second

var pushoverKeyRe = regexp.MustCompile(`^[A-Za-z0-9]{30}$`)

// pushoverDefaultPriority：S1 用高优先级（绕过免打扰时段），其余用普通优先级
var pushoverDefaultPriority = map[int]int{1: 1, 2: 0, 3: 0}

type PushoverProvider struct{}

func (p *PushoverProvider) Ident() string { return models.RequestTypePushover }

func (p *PushoverProvider) Check(config *models.NotifyChannelConfig) error {
	if config == nil || config.RequestType != models.RequestTypePushover {
		return errors.New("pushover provider requires request_type: pushover")
	}
	if config.RequestConfig == nil || config.RequestConfig.PushoverRequestConfig == nil {
		return errors.New("pushover request config cannot be nil")
	}
	return validatePushoverConfig(config.RequestConfig.PushoverRequestConfig, true)
}

// validatePushoverConfig 只做本地格式校验；allowVars 为 true 时含 {{ 的值是变量引用，发送时展开后再校验
func validatePushoverConfig(c *models.PushoverRequestConfig, allowVars bool) error {
	token := strings.TrimSpace(c.Token)
	if token == "" {
		return errors.New("pushover api token is required")
	}
	if !(allowVars && strings.Contains(token, "{{")) && !pushoverKeyRe.MatchString(token) {
		return errors.New("pushover api token must be 30 letters or digits")
	}
	if err := checkAPIURL("pushover api", c.APIURL); err != nil {
		return err
	}
	return checkNativeRetry("pushover", &c.NativeNetworkConfig)
}

// parsePushoverUserKeys 解析规则里的 User Key / Group Key：逗号分隔，最多 50 个
func parsePushoverUserKeys(raw string) ([]string, error) {
	v, err := expandUserVars(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	var keys []string
	seen := map[string]bool{}
	for _, k := range strings.Split(v, ",") {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		if !pushoverKeyRe.MatchString(k) {
			return nil, withHint(errors.New("invalid pushover user_key"), "A Pushover user or group key is 30 letters or digits; copy it from the Pushover dashboard")
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil, errors.New("pushover user_key is required in the notify rule")
	}
	if len(keys) > pushoverMaxUserKeys {
		return nil, errors.New("pushover accepts at most 50 user keys per message; use a delivery group instead")
	}
	return keys, nil
}

func pushoverTarget(name, rawKeys string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	var masked []string
	for _, k := range strings.Split(rawKeys, ",") {
		if k = strings.TrimSpace(k); k != "" {
			masked = append(masked, maskAPIKey(k))
		}
	}
	return strings.Join(masked, ",")
}

func (p *PushoverProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	target := pushoverTarget(req.CustomParams["bot_name"], req.CustomParams["user_key"])
	if req.Config == nil {
		return &NotifyResult{Target: target, Err: errors.New("pushover channel config is required")}
	}
	if err := p.Check(req.Config); err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	if len(req.Events) == 0 {
		return &NotifyResult{Target: target, Err: errors.New("events is empty")}
	}
	cfg := *req.Config.RequestConfig.PushoverRequestConfig
	for _, field := range []*string{&cfg.Token, &cfg.APIURL, &cfg.Sound} {
		v, err := expandUserVars(strings.TrimSpace(*field))
		if err != nil {
			return &NotifyResult{Target: target, Err: err}
		}
		*field = v
	}
	if err := validatePushoverConfig(&cfg, false); err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	keys, err := parsePushoverUserKeys(req.CustomParams["user_key"])
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	target = pushoverTarget(req.CustomParams["bot_name"], strings.Join(keys, ","))
	secrets := append([]string{cfg.Token}, keys...)

	base := strings.TrimRight(cfg.APIURL, "/")
	if base == "" {
		base = pushoverDefaultAPIURL
	}
	event := req.Events[0]

	form := buildPushoverForm(req, &cfg, event, keys)
	body := form.Encode()
	resp, err := p.post(ctx, req.HttpClient, &cfg, base+"/1/messages.json", body)
	if err != nil {
		return &NotifyResult{Target: target,
			Err: withHint(errors.New(redactSecrets(err.Error(), secrets...)), pushoverHint(err, resp))}
	}

	var ok pushoverResponse
	_ = json.Unmarshal(resp.Body, &ok)
	sent := "sent"
	if ok.Request != "" {
		sent += ", request " + ok.Request
	}
	return &NotifyResult{Target: target, Response: sent}
}

func buildPushoverForm(req *NotifyRequest, cfg *models.PushoverRequestConfig, event *models.AlertCurEvent, keys []string) url.Values {
	title := firstEventTitle(req, event, pushoverMaxTitleRunes)
	message, _ := truncateInRunes(tplString(req.TplContent, "content"), pushoverMaxMessageRunes)
	if message == "" {
		// Pushover 不接受空消息
		message = title
	}

	form := url.Values{}
	form.Set("token", cfg.Token)
	form.Set("user", strings.Join(keys, ","))
	form.Set("title", title)
	form.Set("message", message)
	if ts := eventTime(event); ts > 0 {
		form.Set("timestamp", strconv.FormatInt(ts, 10))
	}
	if detail := eventDetailURL(req.SiteUrl, event); detail != "" && len([]rune(detail)) <= pushoverMaxURLRunes {
		form.Set("url", detail)
		form.Set("url_title", "Event details")
	}
	if cfg.Sound != "" {
		form.Set("sound", cfg.Sound)
	}

	// 级别映射固定在后端，恢复通知使用普通优先级。
	priority := 0
	if !event.IsRecovered {
		priority = pushoverDefaultPriority[event.Severity]
	}
	form.Set("priority", strconv.Itoa(priority))
	return form
}

type pushoverResponse struct {
	Status  int      `json:"status"`
	Request string   `json:"request"`
	Errors  []string `json:"errors"`
}

// checkPushover：200 且 status 为 1 才算成功；4xx 一律不重试；其余（5xx 等）可重试
func checkPushover(r *nativeResponse) (bool, error) {
	var v pushoverResponse
	_ = json.Unmarshal(r.Body, &v)
	if r.StatusCode == http.StatusOK && v.Status == 1 {
		return false, nil
	}
	detail := strings.Join(v.Errors, "; ")
	if detail == "" {
		detail = truncateBody(r.Body)
	}
	return r.StatusCode/100 != 4 && r.StatusCode != http.StatusOK, &HTTPStatusError{StatusCode: r.StatusCode, Detail: detail}
}

func (p *PushoverProvider) post(ctx context.Context, client *http.Client, cfg *models.PushoverRequestConfig, endpoint, body string) (*nativeResponse, error) {
	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)
	if sleep < pushoverMinRetryWait {
		sleep = pushoverMinRetryWait
	}
	// 凭证放在表单里而不是地址上：地址会进代理日志和网络错误的文本
	return doWithRetry(ctx, client, retryTimes, sleep,
		func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return r, nil
		}, checkPushover)
}

func pushoverHint(err error, resp *nativeResponse) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		return "Nightingale cannot reach Pushover; check the network or set a proxy in the media type"
	}
	if se.StatusCode == http.StatusTooManyRequests {
		return "The Pushover application has used up its monthly message limit"
	}
	detail := strings.ToLower(se.Detail)
	switch {
	case strings.Contains(detail, "token"):
		return "Check the API token of the Pushover application in the media type"
	case strings.Contains(detail, "user"):
		return "Check the user or group key in the notification rule, and that the user has an active device"
	}
	return ""
}
