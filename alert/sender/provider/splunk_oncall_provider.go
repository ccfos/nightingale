package provider

// Splunk On-Call（原 VictorOps）媒介：走 REST 集成接口
// {api_url}/{api_key}/{routing_key}（https://help.splunk.com/en/splunk-observability-cloud/splunk-on-call/integrations-with-splunk-on-call/rest-endpoint-integration-for-splunk-on-call）。
// API Key 属于整个组织的 REST 集成，放在媒介里；Routing Key 决定告警归哪个团队，填在通知规则里。
//
// 告警以事件哈希作 entity_id：S1 固定发 CRITICAL、S2/S3 发 WARNING，恢复时用同一个 entity_id 发 RECOVERY，
// Splunk On-Call 据此把两次请求对到同一个事件上。成功判定要看响应体里的 result：字段缺失时接口回
// {"result":"failure","message":"..."}。字段与截断参考 Prometheus Alertmanager notify/victorops/victorops.go
// （Apache License 2.0）。

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
)

const (
	splunkOnCallDefaultAPIURL = "https://alert.victorops.com/integrations/generic/20131114/alert"

	splunkOnCallMaxDisplayNameRunes = 1024
	// 同 Alertmanager 的 state_message 上限
	splunkOnCallMaxMessageRunes = 20480
	// 注解（vo_annotate.*）的上限
	splunkOnCallMaxAnnotationRunes = 1124

	splunkOnCallRecovery       = "RECOVERY"
	splunkOnCallMonitoringTool = "Nightingale"
)

var splunkOnCallDefaultMessageType = map[int]string{1: "CRITICAL", 2: "WARNING", 3: "WARNING"}

type SplunkOnCallProvider struct{}

func (p *SplunkOnCallProvider) Ident() string { return models.RequestTypeSplunkOnCall }

func (p *SplunkOnCallProvider) Check(config *models.NotifyChannelConfig) error {
	if config == nil || config.RequestType != models.RequestTypeSplunkOnCall {
		return errors.New("splunk on-call provider requires request_type: splunk_oncall")
	}
	if config.RequestConfig == nil || config.RequestConfig.SplunkOnCallRequestConfig == nil {
		return errors.New("splunk on-call request config cannot be nil")
	}
	c := config.RequestConfig.SplunkOnCallRequestConfig
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("splunk on-call api key is required")
	}
	if err := checkAPIURL("splunk on-call api", c.APIURL); err != nil {
		return err
	}
	return checkNativeRetry("splunk on-call", &c.NativeNetworkConfig)
}

// 级别映射固定在后端；恢复必须使用同一事件的 RECOVERY 消息。
func splunkOnCallMessageType(event *models.AlertCurEvent) string {
	if event.IsRecovered {
		return splunkOnCallRecovery
	}
	if v, ok := splunkOnCallDefaultMessageType[event.Severity]; ok {
		return v
	}
	return "CRITICAL"
}

// splunkOnCallBaseURL 返回不带 key 的接口地址。集成页面给的是带 key 的完整地址
// （.../alert/<api_key>/$routing_key），用户整段贴进来时只留到 /alert 为止
func splunkOnCallBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return splunkOnCallDefaultAPIURL
	}
	if i := strings.Index(base, "/alert/"); i >= 0 {
		base = base[:i+len("/alert")]
	}
	return base
}

func (p *SplunkOnCallProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	name := strings.TrimSpace(req.CustomParams["bot_name"])
	target := name
	if err := p.Check(req.Config); err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	cfg := *req.Config.RequestConfig.SplunkOnCallRequestConfig
	for _, field := range []*string{&cfg.APIKey, &cfg.APIURL} {
		v, err := expandUserVars(strings.TrimSpace(*field))
		if err != nil {
			return &NotifyResult{Target: target, Err: err}
		}
		*field = v
	}
	routingKey, err := expandUserVars(strings.TrimSpace(req.CustomParams["routing_key"]))
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	if routingKey == "" {
		return &NotifyResult{Target: target, Err: errors.New("splunk on-call routing_key is required in the notify rule")}
	}
	if strings.ContainsAny(routingKey, "/?# \t") {
		return &NotifyResult{Target: target, Err: withHint(errors.New("invalid splunk on-call routing_key"),
			"Fill in the routing key only (Settings > Routing Keys in Splunk On-Call), not the endpoint URL")}
	}
	// Routing Key 是团队路由的名字，不是凭证，没填名称时直接显示它
	if target == "" {
		target = routingKey
	}
	endpoint := splunkOnCallBaseURL(cfg.APIURL) + "/" + url.PathEscape(cfg.APIKey) + "/" + url.PathEscape(routingKey)
	nonce := req.CustomParams[TestNonceParam]

	var responses []string
	for _, event := range req.Events {
		resp, err := p.notifyEvent(ctx, req, &cfg, endpoint, event, nonce)
		if err != nil {
			// 网络错误的文本会内嵌完整地址，API Key 就在路径里
			msg := redactSecrets(err.Error(), cfg.APIKey)
			return &NotifyResult{Target: target, Response: strings.Join(responses, "\n"), Err: withHint(errors.New(msg), splunkOnCallHint(err))}
		}
		responses = append(responses, resp)
	}
	return &NotifyResult{Target: target, Response: strings.Join(responses, "\n")}
}

func (p *SplunkOnCallProvider) notifyEvent(ctx context.Context, req *NotifyRequest, cfg *models.SplunkOnCallRequestConfig,
	endpoint string, event *models.AlertCurEvent, nonce string) (string, error) {
	payload := buildSplunkOnCallPayload(req, event, nonce)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)
	_, err = doWithRetry(ctx, req.HttpClient, retryTimes, sleep,
		func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			return r, nil
		}, checkSplunkOnCall)
	if err != nil {
		return "", err
	}
	if event.IsRecovered {
		return "recovery sent, entity_id " + payload["entity_id"], nil
	}
	return fmt.Sprintf("%s alert sent, entity_id %s", payload["message_type"], payload["entity_id"]), nil
}

func buildSplunkOnCallPayload(req *NotifyRequest, event *models.AlertCurEvent, nonce string) map[string]string {
	entityID := event.Hash
	if nonce != "" {
		// 测试发送用每次唯一的 entity_id，免得和上一次测试建的事件合并；with_recovery 两次共用同一个
		entityID += "-" + nonce
	}
	title := tplString(req.TplContent, "title")
	if title == "" {
		title = eventTitle(event)
	}
	title, _ = truncateInRunes(strings.Join(strings.Fields(title), " "), splunkOnCallMaxDisplayNameRunes)
	message, _ := truncateInRunes(tplString(req.TplContent, "content"), splunkOnCallMaxMessageRunes)

	msg := map[string]string{
		"message_type":        splunkOnCallMessageType(event),
		"entity_id":           entityID,
		"entity_display_name": title,
		"state_message":       message,
		"monitoring_tool":     splunkOnCallMonitoringTool,
	}
	if event.TargetIdent != "" {
		msg["host_name"] = event.TargetIdent
	}
	// 注解只对 INFO / WARNING / CRITICAL 生效，恢复时不带
	if detail := eventDetailURL(req.SiteUrl, event); detail != "" && !event.IsRecovered && len([]rune(detail)) <= splunkOnCallMaxAnnotationRunes {
		msg["vo_annotate.u.Event details"] = detail
	}
	return msg
}

type splunkOnCallResponse struct {
	Result  string `json:"result"`
	Message string `json:"message"`
}

// checkSplunkOnCall：2xx 且 result 不是 failure 才算成功；5xx 与 429 可重试
func checkSplunkOnCall(r *nativeResponse) (bool, error) {
	var v splunkOnCallResponse
	_ = json.Unmarshal(r.Body, &v)
	detail := func(*nativeResponse) string { return v.Message }
	if r.StatusCode/100 == 2 {
		if strings.EqualFold(v.Result, "failure") {
			d := v.Message
			if d == "" {
				d = truncateBody(r.Body)
			}
			return false, &HTTPStatusError{StatusCode: r.StatusCode, Detail: d}
		}
		return false, nil
	}
	return checkStatus(r, detail)
}

func splunkOnCallHint(err error) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		return "Nightingale cannot reach Splunk On-Call; check the network or set a proxy in the media type"
	}
	switch {
	case se.StatusCode == http.StatusTooManyRequests:
		return "Rate limited by Splunk On-Call, retried automatically"
	case se.StatusCode/100 == 4:
		return "Check the API key of the REST endpoint integration in the media type and the routing key in the notification rule (Splunk On-Call: Integrations > REST - Generic)"
	}
	return ""
}
