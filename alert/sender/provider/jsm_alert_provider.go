package provider

// JSM（Jira Service Management Operations，原 Opsgenie）告警媒介：走面向监控工具的集成接口
// {api_url}/jsm/ops/integration/v2/alerts，认证头 GenieKey <API 集成的 key>。
// key 属于某个团队的 API 集成，决定告警归哪个团队，所以填在通知规则里。
//
// 告警以事件哈希作 alias：触发时建告警（JSM 按 alias 对未关闭的告警去重，只累加次数），
// 恢复时按 alias 关闭。组包、截断与关闭逻辑移植自 Prometheus Alertmanager
// notify/opsgenie/opsgenie.go（Apache License 2.0）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

const (
	jsmDefaultAPIURL = "https://api.atlassian.com"
	jsmAlertsPath    = "/jsm/ops/integration/v2/alerts"

	// 字段上限见 JSM / Opsgenie Alert API 文档
	jsmMaxMessageRunes     = 130
	jsmMaxAliasRunes       = 512
	jsmMaxDescriptionRunes = 15000
	jsmMaxNoteRunes        = 25000
	jsmMaxEntityRunes      = 512
	jsmMaxSourceRunes      = 100
	jsmMaxTags             = 20
	jsmMaxTagRunes         = 50
	jsmMaxDetailsRunes     = 8000

	jsmSource = "Nightingale"
)

// jsmDefaultPriority 与旧版通用 HTTP 媒介的 P{{$event.Severity}} 一致：S1→P1、S2→P2、S3→P3
var jsmDefaultPriority = map[int]string{1: "P1", 2: "P2", 3: "P3"}

type JSMAlertProvider struct{}

func (p *JSMAlertProvider) Ident() string { return models.RequestTypeJSMAlert }

func (p *JSMAlertProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != models.RequestTypeJSMAlert {
		return errors.New("jsm alert provider requires request_type: jsm_alert")
	}
	// 媒介配置可以整体为空（key 在规则里），不能要求非 nil
	return config.ValidateJSMAlertRequestConfig()
}

type jsmCreateAlert struct {
	Message     string            `json:"message"`
	Alias       string            `json:"alias"`
	Description string            `json:"description,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
	Entity      string            `json:"entity,omitempty"`
	Source      string            `json:"source"`
	Priority    string            `json:"priority,omitempty"`
}

type jsmCloseAlert struct {
	Source string `json:"source"`
	Note   string `json:"note,omitempty"`
}

type jsmParams struct {
	APIKey   string
	Name     string
	Priority map[int]string
}

func parseJSMParams(p map[string]string) (*jsmParams, error) {
	key, err := expandUserVars(strings.TrimSpace(p["api_key"]))
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, errors.New("jsm alert api_key is required in the notify rule")
	}
	jp := &jsmParams{APIKey: key, Name: strings.TrimSpace(p["bot_name"]), Priority: map[int]string{}}
	for k, v := range jsmDefaultPriority {
		jp.Priority[k] = v
	}
	if raw := strings.TrimSpace(p["priority_map"]); raw != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, fmt.Errorf("invalid jsm alert priority_map: %v", err)
		}
		for sev, pr := range m {
			var s int
			if _, err := fmt.Sscanf(sev, "%d", &s); err != nil || s < 1 || s > 3 {
				return nil, fmt.Errorf("invalid severity %q in jsm alert priority_map, must be 1, 2 or 3", sev)
			}
			pr = strings.ToUpper(strings.TrimSpace(pr))
			switch pr {
			case "":
				delete(jp.Priority, s)
			case "P1", "P2", "P3", "P4", "P5":
				jp.Priority[s] = pr
			default:
				return nil, fmt.Errorf("invalid priority %q in jsm alert priority_map, must be P1 to P5", pr)
			}
		}
	}
	return jp, nil
}

// maskAPIKey 只留最后 4 位，用在通知记录的目标和报错里
func maskAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 4 {
		return "***"
	}
	return "***" + key[len(key)-4:]
}

func jsmBaseURL(cfg *models.JSMAlertRequestConfig) string {
	base := jsmDefaultAPIURL
	if cfg != nil && strings.TrimSpace(cfg.APIURL) != "" {
		base = strings.TrimSpace(cfg.APIURL)
	}
	base = strings.TrimRight(base, "/")
	// 允许用户直接贴集成页面给的完整地址
	for _, suffix := range []string{"/v2/alerts", "/jsm/ops/integration"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return base + jsmAlertsPath
}

func (p *JSMAlertProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	params, err := parseJSMParams(req.CustomParams)
	if err != nil {
		return &NotifyResult{Target: jsmTarget(req.CustomParams["bot_name"], req.CustomParams["api_key"]), Err: err}
	}
	target := jsmTarget(params.Name, params.APIKey)

	var cfg models.JSMAlertRequestConfig
	if req.Config != nil && req.Config.RequestConfig != nil && req.Config.RequestConfig.JSMAlertRequestConfig != nil {
		cfg = *req.Config.RequestConfig.JSMAlertRequestConfig
	}
	endpoint := jsmBaseURL(&cfg)
	nonce := req.CustomParams[TestNonceParam]

	var responses []string
	for _, event := range req.Events {
		resp, err := p.notifyEvent(ctx, req, &cfg, endpoint, params, event, nonce)
		if err != nil {
			return &NotifyResult{Target: target, Response: strings.Join(responses, "\n"), Err: err}
		}
		responses = append(responses, resp)
	}
	return &NotifyResult{Target: target, Response: strings.Join(responses, "\n")}
}

func (p *JSMAlertProvider) notifyEvent(ctx context.Context, req *NotifyRequest, cfg *models.JSMAlertRequestConfig,
	endpoint string, params *jsmParams, event *models.AlertCurEvent, nonce string) (string, error) {
	alias := event.Hash
	if nonce != "" {
		// 测试发送用每次唯一的 alias，免得和上一次测试建的告警合并；with_recovery 两次共用同一个
		alias += "-" + nonce
	}
	alias, _ = truncateInRunes(alias, jsmMaxAliasRunes)

	var (
		method = http.MethodPost
		u      string
		body   interface{}
		action string
	)
	if event.IsRecovered {
		u = endpoint + "/" + url.PathEscape(alias) + "/close?identifierType=alias"
		note, _ := truncateInRunes(tplString(req.TplContent, "content"), jsmMaxNoteRunes)
		body = &jsmCloseAlert{Source: jsmSource, Note: note}
		action = "close"
	} else {
		u = endpoint
		body = buildJSMCreate(req, params, event, alias)
		action = "create"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)
	resp, err := doWithRetry(ctx, req.HttpClient, retryTimes, sleep,
		func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
			if err != nil {
				return nil, err
			}
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "GenieKey "+params.APIKey)
			return r, nil
		},
		func(r *nativeResponse) (bool, error) { return checkStatus(r, jsmErrorDetail) })
	if err != nil {
		msg := strings.ReplaceAll(err.Error(), params.APIKey, maskAPIKey(params.APIKey))
		return "", withHint(errors.New(msg), jsmHint(err))
	}

	// 接口只回 202 + requestId，真正的处理是异步的；测试发送时等它处理完，把真实结果回给用户
	var accepted struct {
		RequestID string `json:"requestId"`
	}
	_ = json.Unmarshal(resp.Body, &accepted)
	if nonce == "" || accepted.RequestID == "" {
		if accepted.RequestID == "" {
			return action + " accepted", nil
		}
		return fmt.Sprintf("%s accepted, request id %s", action, accepted.RequestID), nil
	}
	st, err := p.waitRequest(ctx, req.HttpClient, endpoint, params.APIKey, accepted.RequestID)
	if err != nil {
		return fmt.Sprintf("%s accepted, request id %s (status unknown: %v)", action, accepted.RequestID, err), nil
	}
	if !st.Success {
		return "", withHint(fmt.Errorf("jsm alert %s failed: %s", action, st.Status), "")
	}
	if action == "create" {
		return "alert created, id " + st.AlertID, nil
	}
	return "alert closed, id " + st.AlertID, nil
}

func buildJSMCreate(req *NotifyRequest, params *jsmParams, event *models.AlertCurEvent, alias string) *jsmCreateAlert {
	title := tplString(req.TplContent, "title")
	if title == "" {
		title = eventTitle(event)
	}
	message, _ := truncateInRunes(strings.Join(strings.Fields(title), " "), jsmMaxMessageRunes)
	desc, _ := truncateInRunes(tplString(req.TplContent, "content"), jsmMaxDescriptionRunes)
	entity, _ := truncateInRunes(event.TargetIdent, jsmMaxEntityRunes)

	var tags []string
	for _, t := range event.TagsJSON {
		if len(tags) >= jsmMaxTags {
			break
		}
		t, _ = truncateInRunes(strings.TrimSpace(t), jsmMaxTagRunes)
		if t != "" {
			tags = append(tags, t)
		}
	}

	return &jsmCreateAlert{
		Message:     message,
		Alias:       alias,
		Description: desc,
		Tags:        tags,
		Details:     jsmDetails(event),
		Entity:      entity,
		Source:      jsmSource,
		Priority:    params.Priority[event.Severity],
	}
}

// jsmDetails 放告警标签和注解（同 Alertmanager 放 CommonLabels），整体不超过 8000 字符
func jsmDetails(event *models.AlertCurEvent) map[string]string {
	kv := map[string]string{}
	for k, v := range event.TagsMap {
		kv[k] = v
	}
	for k, v := range event.AnnotationsJSON {
		if _, ok := kv[k]; !ok {
			kv[k] = v
		}
	}
	if len(kv) == 0 {
		return nil
	}
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]string{}
	budget := jsmMaxDetailsRunes
	for _, k := range keys {
		cost := len([]rune(k)) + len([]rune(kv[k]))
		if cost > budget {
			break
		}
		out[k] = kv[k]
		budget -= cost
	}
	return out
}

func jsmTarget(name, key string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return maskAPIKey(key)
}

type jsmRequestStatus struct {
	Success bool   `json:"success"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	AlertID string `json:"alertId"`
	Alias   string `json:"alias"`
}

// waitRequest 轮询异步请求的处理结果，最多等 10 秒；处理前查询会返回 404
func (p *JSMAlertProvider) waitRequest(ctx context.Context, client *http.Client, endpoint, key, requestID string) (*jsmRequestStatus, error) {
	deadline := time.Now().Add(10 * time.Second)
	u := endpoint + "/requests/" + url.PathEscape(requestID)
	for {
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Set("Authorization", "GenieKey "+key)
		resp, err := client.Do(r)
		if err != nil {
			return nil, err
		}
		var out struct {
			Data jsmRequestStatus `json:"data"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && decodeErr == nil && (out.Data.Status != "" || out.Data.Success) {
			return &out.Data, nil
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
		}
		if time.Now().After(deadline) {
			return nil, errors.New("request not processed within 10s")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// jsmErrorDetail 解析错误响应：{"message":"...","took":0.0,"requestId":"...","errors":{...}}
func jsmErrorDetail(r *nativeResponse) string {
	var e struct {
		Message string            `json:"message"`
		Errors  map[string]string `json:"errors"`
	}
	if err := json.Unmarshal(r.Body, &e); err != nil || e.Message == "" {
		return truncateBody(r.Body)
	}
	s := e.Message
	if len(e.Errors) > 0 {
		b, _ := json.Marshal(e.Errors)
		s += ": " + truncateBody(b)
	}
	return s
}

func jsmHint(err error) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		return "Nightingale cannot reach Jira Service Management; check the network or set a proxy in the media type"
	}
	switch se.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "Check the API key of the JSM API integration, and that the integration is turned on"
	case http.StatusUnprocessableEntity:
		return "JSM rejected the alert fields; see the field in the error"
	case http.StatusTooManyRequests:
		return "Rate limited by Jira Service Management, retried automatically"
	}
	return ""
}
