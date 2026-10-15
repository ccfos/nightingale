package provider

// ntfy 媒介：服务地址与认证在媒介里配（留空用公共服务 ntfy.sh），Topic 在通知规则里填。
//
// 用 JSON 发布接口（https://docs.ntfy.sh/publish/#publish-as-json）：POST 到服务根地址，topic 放在请求体里。
// 报错格式 {"code":40101,"http":401,"error":"unauthorized"} 见 ntfy 源码 server/errors.go。
// 级别到优先级的映射、标签与跳转按钮参考 Uptime Kuma 的 ntfy.js（MIT）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

const (
	ntfyDefaultServerURL = "https://ntfy.sh"

	ntfyMaxTitleRunes = 250
	// 正文超过 4096 字节会被服务端转成附件，手机上就看不到内容了
	ntfyMaxMessageBytes = 4096

	ntfyDefaultRecoveredPriority = 3
	// 当日消息额度用完，重试没有意义
	ntfyCodeDailyQuota = 42908
)

var ntfyTopicRe = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

// ntfyDefaultPriority：固定为 S1 最高（5）、S2 高（4）、S3 默认（3）。
var ntfyDefaultPriority = map[int]int{1: 5, 2: 4, 3: 3}

type NtfyProvider struct{}

func (p *NtfyProvider) Ident() string { return models.RequestTypeNtfy }

func (p *NtfyProvider) Check(config *models.NotifyChannelConfig) error {
	if config == nil || config.RequestType != models.RequestTypeNtfy {
		return errors.New("ntfy provider requires request_type: ntfy")
	}
	// 媒介配置可以整体为空：公共服务 ntfy.sh、不带认证
	if config.RequestConfig == nil || config.RequestConfig.NtfyRequestConfig == nil {
		return nil
	}
	c := config.RequestConfig.NtfyRequestConfig
	if err := checkAPIURL("ntfy server", c.ServerURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.Token) != "" && strings.TrimSpace(c.Username) != "" {
		return errors.New("ntfy accepts either an access token or a username and password, not both")
	}
	if (strings.TrimSpace(c.Username) == "") != (strings.TrimSpace(c.Password) == "") {
		return errors.New("ntfy username and password must be set together")
	}
	return checkNativeRetry("ntfy", &c.NativeNetworkConfig)
}

type ntfyMessage struct {
	Topic    string       `json:"topic"`
	Title    string       `json:"title,omitempty"`
	Message  string       `json:"message"`
	Priority int          `json:"priority,omitempty"`
	Tags     []string     `json:"tags,omitempty"`
	Click    string       `json:"click,omitempty"`
	Actions  []ntfyAction `json:"actions,omitempty"`
}

type ntfyAction struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"url"`
}

func ntfyPriority(event *models.AlertCurEvent) int {
	if event.IsRecovered {
		return ntfyDefaultRecoveredPriority
	}
	if v, ok := ntfyDefaultPriority[event.Severity]; ok {
		return v
	}
	return ntfyDefaultRecoveredPriority
}

// ntfyTags 的第一项是 ntfy 认识的 emoji 短码（显示成标题前的图标），第二项原样显示成标签
func ntfyTags(event *models.AlertCurEvent) []string {
	icon := "warning"
	switch {
	case event.IsRecovered:
		icon = "white_check_mark"
	case event.Severity == 1:
		icon = "rotating_light"
	}
	return []string{icon, "S" + strconv.Itoa(event.Severity)}
}

func (p *NtfyProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	// 公共服务上 topic 等同于密码，记录里没有名称时只留后 4 位
	target := namedTarget(req.CustomParams["bot_name"], req.CustomParams["topic"])
	if err := p.Check(req.Config); err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	if len(req.Events) == 0 {
		return &NotifyResult{Target: target, Err: errors.New("events is empty")}
	}
	var cfg models.NtfyRequestConfig
	if req.Config.RequestConfig != nil && req.Config.RequestConfig.NtfyRequestConfig != nil {
		cfg = *req.Config.RequestConfig.NtfyRequestConfig
	}
	for _, field := range []*string{&cfg.ServerURL, &cfg.Token, &cfg.Username, &cfg.Password} {
		v, err := expandUserVars(strings.TrimSpace(*field))
		if err != nil {
			return &NotifyResult{Target: target, Err: err}
		}
		*field = v
	}
	server := strings.TrimRight(cfg.ServerURL, "/")
	if server == "" {
		server = ntfyDefaultServerURL
	}
	if u, err := url.Parse(server); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return &NotifyResult{Target: target, Err: errors.New("ntfy server url must start with http:// or https://")}
	}

	topic, err := expandUserVars(strings.TrimSpace(req.CustomParams["topic"]))
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	if topic == "" {
		return &NotifyResult{Target: target, Err: errors.New("ntfy topic is required in the notify rule")}
	}
	if !ntfyTopicRe.MatchString(topic) {
		return &NotifyResult{Target: target, Err: withHint(errors.New("invalid ntfy topic"),
			"An ntfy topic is 1 to 64 letters, digits, underscores or dashes; fill in the topic name only, not the full URL")}
	}
	target = namedTarget(req.CustomParams["bot_name"], topic)
	secrets := []string{cfg.Token, cfg.Password, topic}

	event := req.Events[0]
	body, err := json.Marshal(buildNtfyMessage(req, event, topic))
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}

	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)
	resp, err := doWithRetry(ctx, req.HttpClient, retryTimes, sleep,
		func(ctx context.Context) (*http.Request, error) {
			// JSON 发布必须 POST 到服务根地址，发到 /<topic> 会把整段 JSON 当成正文
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/", bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			switch {
			case cfg.Token != "":
				r.Header.Set("Authorization", "Bearer "+cfg.Token)
			case cfg.Username != "":
				r.SetBasicAuth(cfg.Username, cfg.Password)
			}
			return r, nil
		}, checkNtfy)
	if err != nil {
		return &NotifyResult{Target: target, Err: withHint(errors.New(redactSecrets(err.Error(), secrets...)), ntfyHint(err))}
	}

	var published struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(resp.Body, &published)
	response := "sent"
	if published.ID != "" {
		response = "sent, message id " + published.ID
	}
	return &NotifyResult{Target: target, Response: response}
}

func buildNtfyMessage(req *NotifyRequest, event *models.AlertCurEvent, topic string) *ntfyMessage {
	title := firstEventTitle(req, event, ntfyMaxTitleRunes)
	message, _ := truncateInBytes(tplString(req.TplContent, "content"), ntfyMaxMessageBytes)
	if message == "" {
		// 正文为空时 ntfy 会显示成 triggered，不如把标题放进来
		message = title
	}
	m := &ntfyMessage{
		Topic:    topic,
		Title:    title,
		Message:  message,
		Priority: ntfyPriority(event),
		Tags:     ntfyTags(event),
	}
	detail, silence := eventActionURLs(req.SiteUrl, event)
	if detail != "" {
		// 点通知本身也跳到事件详情
		m.Click = detail
		m.Actions = append(m.Actions, ntfyAction{Action: "view", Label: "Event details", URL: detail})
	}
	if silence != "" {
		m.Actions = append(m.Actions, ntfyAction{Action: "view", Label: "Silence 1h", URL: silence})
	}
	return m
}

type ntfyError struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

// checkNtfy 在默认判定之上排除一种情况：当日消息额度用完（429 + 42908）不重试
func checkNtfy(r *nativeResponse) (bool, error) {
	retry, err := checkStatus(r, func(r *nativeResponse) string {
		var e ntfyError
		if json.Unmarshal(r.Body, &e) != nil || e.Error == "" {
			return ""
		}
		return fmt.Sprintf("%d %s", e.Code, e.Error)
	})
	if err != nil && retry {
		var e ntfyError
		if json.Unmarshal(r.Body, &e) == nil && e.Code == ntfyCodeDailyQuota {
			retry = false
		}
	}
	return retry, err
}

func ntfyHint(err error) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		if strings.Contains(err.Error(), "x509") || strings.Contains(err.Error(), "certificate") {
			return "The ntfy server uses a certificate Nightingale does not trust; enable skip certificate verification in the media type's advanced settings"
		}
		return "Nightingale cannot reach the ntfy server; check the server URL and the network, or set a proxy in the media type"
	}
	switch se.StatusCode {
	case http.StatusUnauthorized:
		return "The ntfy server requires authentication; check the access token or the username and password in the media type"
	case http.StatusForbidden:
		return "The ntfy user or token is not allowed to publish to this topic, or the topic is reserved by someone else"
	case http.StatusNotFound:
		return "The ntfy server URL is wrong; fill in the server root address, e.g. https://ntfy.sh"
	case http.StatusRequestEntityTooLarge:
		return "The message is too large for the ntfy server; shorten the message template"
	case http.StatusTooManyRequests:
		return "Rate limited by the ntfy server, or the daily message quota is used up"
	}
	return ""
}
