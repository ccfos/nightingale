package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
)

// capturedRequest 记录假服务收到的一次请求
type capturedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

func (c capturedRequest) json(t *testing.T) map[string]interface{} {
	t.Helper()
	out := map[string]interface{}{}
	if err := json.Unmarshal(c.Body, &out); err != nil {
		t.Fatalf("request body is not JSON: %v: %s", err, c.Body)
	}
	return out
}

func (c capturedRequest) form(t *testing.T) url.Values {
	t.Helper()
	v, err := url.ParseQuery(string(c.Body))
	if err != nil {
		t.Fatalf("request body is not a form: %v", err)
	}
	return v
}

// newCaptureServer 起一个假服务：记录每次请求，按 respond 给的状态码和响应体回复
func newCaptureServer(t *testing.T, respond func(n int, r capturedRequest) (int, string)) (*httptest.Server, func() []capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var got []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c := capturedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: b}
		mu.Lock()
		got = append(got, c)
		n := len(got)
		mu.Unlock()
		status, body := respond(n, c)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []capturedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedRequest(nil), got...)
	}
}

func always(status int, body string) func(int, capturedRequest) (int, string) {
	return func(int, capturedRequest) (int, string) { return status, body }
}

// noLeak 断言通知记录里的目标、响应和报错都不含凭证
func noLeak(t *testing.T, res *NotifyResult, secrets ...string) {
	t.Helper()
	text := res.Target + "\n" + res.Response
	if res.Err != nil {
		text += "\n" + res.Err.Error()
	}
	for _, s := range secrets {
		if strings.Contains(text, s) {
			t.Fatalf("credential %q leaked into the notify result: %s", s, text)
		}
	}
}

func fastRetry() models.NativeNetworkConfig {
	return models.NativeNetworkConfig{RetryTimes: 2, RetrySleep: 1}
}

func TestMoreNativeChannelsResolveAndFallback(t *testing.T) {
	// 配置可以整体为空的内置媒介：必须解析到自己的 provider，也必须拿得到 HTTP 客户端
	for _, rt := range []string{models.RequestTypeNtfy} {
		nc := &models.NotifyChannelConfig{Ident: rt, RequestType: rt}
		if got, ok := DefaultRegistry.Resolve(nc); !ok || got.Ident() != rt {
			t.Fatalf("native %s should resolve to its provider, got %v", rt, got)
		}
		if _, err := models.GetHTTPClient(nc); err != nil {
			t.Fatalf("GetHTTPClient must accept an empty %s config: %v", rt, err)
		}
	}
	// 有必填凭证的三个媒介：配置为空时保存校验要拦下
	for _, rt := range []string{models.RequestTypePushover, models.RequestTypeSplunkOnCall, models.RequestTypeLine} {
		err := VerifyChannelConfig(&models.NotifyChannelConfig{Ident: rt, RequestType: rt, RequestConfig: &models.RequestConfig{}})
		if err == nil {
			t.Fatalf("%s without credentials must not pass verification", rt)
		}
		// 报错要说清缺什么，而不是一句 unsupported channel
		if strings.Contains(err.Error(), "unsupported channel") {
			t.Fatalf("%s: verification must surface the provider's own error, got %v", rt, err)
		}
	}
	// 老库里同名 ident 的通用 HTTP 记录照旧走 callback
	for _, rt := range []string{models.RequestTypePushover, models.RequestTypeNtfy, models.RequestTypeSplunkOnCall, models.RequestTypeLine} {
		if !models.IsNativeRequestType(rt) {
			t.Fatalf("%s must be a native request type", rt)
		}
		legacy := &models.NotifyChannelConfig{Ident: rt, RequestType: "http",
			RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://x", Method: "POST"}}}
		if got, ok := DefaultRegistry.Resolve(legacy); !ok || got.Ident() != "callback" {
			t.Fatalf("legacy http %s record should fall back to callback, got %v", rt, got)
		}
	}
}

// ---------- Pushover ----------

const (
	testPushoverToken = "azGDORePK8gMaC0QOYAMyEEuzJnyUi"
	testPushoverUser  = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	testPushoverGroup = "gznej3rKEVAvPUxu9vvNnqpmZpokzF"
)

func pushoverReq(apiURL string, mutate func(*models.PushoverRequestConfig)) *NotifyRequest {
	cfg := &models.PushoverRequestConfig{APIURL: apiURL, Token: testPushoverToken, NativeNetworkConfig: fastRetry()}
	if mutate != nil {
		mutate(cfg)
	}
	return webhookReq(models.RequestTypePushover, &models.RequestConfig{PushoverRequestConfig: cfg},
		map[string]string{"user_key": testPushoverUser + ", " + testPushoverGroup + "," + testPushoverUser})
}

func TestPushoverSendsFormMessage(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"status":1,"request":"647d2300-702c"}`))
	req := pushoverReq(srv.URL, nil)
	req.TplContent["content"] = "Target: db-01\nValue: 92 > 90 & \"hot\" 中文"
	res := (&PushoverProvider{}).Notify(context.Background(), req)
	if res.Err != nil || res.Response != "sent, request 647d2300-702c" {
		t.Fatalf("unexpected result: %+v", res)
	}
	noLeak(t, res, testPushoverToken, testPushoverUser, testPushoverGroup)
	if res.Target != "***VRsG,***okzF" {
		t.Fatalf("target should list masked keys, got %q", res.Target)
	}
	r := got()[0]
	if r.Method != "POST" || r.Path != "/1/messages.json" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || len(r.Query) != 0 {
		t.Fatalf("credentials must travel in the form body, not the URL: %+v", r)
	}
	f := r.form(t)
	want := map[string]string{
		"token": testPushoverToken, "user": testPushoverUser + "," + testPushoverGroup,
		"title": "[S1] Triggered: cpu <high> & hot", "message": req.TplContent["content"].(string),
		"priority": "1", "timestamp": "1700000000",
		"url": "http://n9e.example.com/share/alert-his-events/42", "url_title": "Event details",
	}
	for k, v := range want {
		if f.Get(k) != v {
			t.Fatalf("form %s = %q, want %q", k, f.Get(k), v)
		}
	}
	for _, k := range []string{"retry", "expire", "tags", "sound", "html"} {
		if f.Has(k) {
			t.Fatalf("form must not carry %s for a non-emergency message", k)
		}
	}
}

func TestPushoverFixedPrioritiesAndRecovery(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"status":1,"request":"r1"}`))
	req := pushoverReq(srv.URL, func(c *models.PushoverRequestConfig) {
		c.Sound = "siren"
	})
	req.Events[0].LastEvalTime = 1700000600
	for severity, want := range map[int]string{1: "1", 2: "0", 3: "0"} {
		req.Events[0].Severity = severity
		for _, recovered := range []bool{false, true} {
			req.Events[0].IsRecovered = recovered
			before := len(got())
			res := (&PushoverProvider{}).Notify(context.Background(), req)
			if res.Err != nil {
				t.Fatalf("S%d recovered=%t: %v", severity, recovered, res.Err)
			}
			calls := got()
			if len(calls) != before+1 || calls[before].Path != "/1/messages.json" {
				t.Fatalf("each notification must only send a message: %+v", calls[before:])
			}
			f := calls[before].form(t)
			priority := want
			if recovered {
				priority = "0"
				if f.Get("timestamp") != "1700000600" || !strings.Contains(f.Get("title"), "Recovered:") {
					t.Fatalf("unexpected recovery form: %v", f)
				}
			}
			if f.Get("priority") != priority || f.Get("sound") != "siren" {
				t.Fatalf("S%d recovered=%t: unexpected form: %v", severity, recovered, f)
			}
			for _, key := range []string{"retry", "expire", "tags"} {
				if f.Has(key) {
					t.Fatalf("fixed priorities must not send emergency field %s: %v", key, f)
				}
			}
		}
	}
}

func TestPushoverRetryClassification(t *testing.T) {
	old := pushoverMinRetryWait
	pushoverMinRetryWait = time.Millisecond
	defer func() { pushoverMinRetryWait = old }()

	// 4xx（含 429 月度额度）不重试
	for _, status := range []int{400, 429} {
		srv, got := newCaptureServer(t, always(status, `{"user":"invalid","errors":["user identifier is not a valid user, group, or subscribed user key"],"status":0,"request":"r"}`))
		res := (&PushoverProvider{}).Notify(context.Background(), pushoverReq(srv.URL, nil))
		if res.Err == nil || len(got()) != 1 || !strings.Contains(res.Err.Error(), "user identifier is not a valid user") {
			t.Fatalf("status %d must fail without retry: %+v calls=%d", status, res, len(got()))
		}
		noLeak(t, res, testPushoverToken, testPushoverUser)
	}
	// 5xx 重试
	srv, got := newCaptureServer(t, func(n int, _ capturedRequest) (int, string) {
		if n < 3 {
			return 500, "oops"
		}
		return 200, `{"status":1,"request":"r"}`
	})
	if res := (&PushoverProvider{}).Notify(context.Background(), pushoverReq(srv.URL, nil)); res.Err != nil || len(got()) != 3 {
		t.Fatalf("5xx must be retried: %+v calls=%d", res, len(got()))
	}
	// 200 但 status 不是 1 也算失败
	srv, _ = newCaptureServer(t, always(200, `{"status":0,"errors":["application token is invalid"]}`))
	res := (&PushoverProvider{}).Notify(context.Background(), pushoverReq(srv.URL, nil))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "Check the API token") {
		t.Fatalf("status 0 must fail with a hint: %+v", res)
	}
}

func TestPushoverValidation(t *testing.T) {
	p := &PushoverProvider{}
	ok := func(mutate func(*models.PushoverRequestConfig)) error {
		return p.Check(pushoverReq("", mutate).Config)
	}
	if err := ok(nil); err != nil {
		t.Fatal(err)
	}
	if err := ok(func(c *models.PushoverRequestConfig) { c.Token = "{{.pushover_token}}" }); err != nil {
		t.Fatalf("variable references must pass the save-time check: %v", err)
	}
	bad := []func(*models.PushoverRequestConfig){
		func(c *models.PushoverRequestConfig) { c.Token = "" },
		func(c *models.PushoverRequestConfig) { c.Token = "short" },
		func(c *models.PushoverRequestConfig) { c.APIURL = "api.pushover.net" },
		func(c *models.PushoverRequestConfig) { c.RetryTimes = -1 },
	}
	for i, m := range bad {
		if err := ok(m); err == nil {
			t.Fatalf("invalid config #%d passed", i)
		}
	}
	req := pushoverReq("http://127.0.0.1:1", nil)
	for _, key := range []string{"", "too-short", strings.Repeat(testPushoverUser+",", 2) + "bad"} {
		req.CustomParams["user_key"] = key
		if res := p.Notify(context.Background(), req); res.Err == nil {
			t.Fatalf("accepted invalid user key %q", key)
		}
	}
}

// ---------- ntfy ----------

func ntfyReq(server string, mutate func(*models.NtfyRequestConfig)) *NotifyRequest {
	cfg := &models.NtfyRequestConfig{ServerURL: server, NativeNetworkConfig: fastRetry()}
	if mutate != nil {
		mutate(cfg)
	}
	return webhookReq(models.RequestTypeNtfy, &models.RequestConfig{NtfyRequestConfig: cfg}, map[string]string{"topic": "n9e-secret-topic-9f8e7d"})
}

func TestNtfyPublishesJSON(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"id":"hwQ2YpKdmg","time":1700000000,"event":"message","topic":"n9e-secret-topic-9f8e7d"}`))
	req := ntfyReq(srv.URL+"/", func(c *models.NtfyRequestConfig) { c.Token = "tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2" })
	req.TplContent["content"] = "Target: db-01\nValue: 92 > 90 & \"hot\""
	res := (&NtfyProvider{}).Notify(context.Background(), req)
	if res.Err != nil || res.Response != "sent, message id hwQ2YpKdmg" || res.Target != "***8e7d" {
		t.Fatalf("unexpected result: %+v", res)
	}
	noLeak(t, res, "n9e-secret-topic-9f8e7d", "tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2")
	r := got()[0]
	// JSON 发布必须打到服务根地址
	if r.Method != "POST" || r.Path != "/" || r.Header.Get("Authorization") != "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2" || r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected request: %+v", r)
	}
	var m ntfyMessage
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatal(err)
	}
	if m.Topic != "n9e-secret-topic-9f8e7d" || m.Title != "[S1] Triggered: cpu <high> & hot" || m.Message != req.TplContent["content"] ||
		m.Priority != 5 || strings.Join(m.Tags, ",") != "rotating_light,S1" || m.Click != "http://n9e.example.com/share/alert-his-events/42" {
		t.Fatalf("unexpected message: %s", r.Body)
	}
	if len(m.Actions) != 2 || m.Actions[0].Action != "view" || m.Actions[1].URL != "http://n9e.example.com/alert-mutes/add?__event_id=42" {
		t.Fatalf("unexpected actions: %+v", m.Actions)
	}

	// 恢复：默认优先级、对勾图标、没有屏蔽按钮；用户名密码走 Basic 认证。
	req = ntfyReq(srv.URL, func(c *models.NtfyRequestConfig) {
		c.Username, c.Password = "n9e", "pass-word-123"
	})
	req.Events[0].IsRecovered = true
	if res := (&NtfyProvider{}).Notify(context.Background(), req); res.Err != nil {
		t.Fatal(res.Err)
	}
	r = got()[1]
	json.Unmarshal(r.Body, &m)
	if user, pass, ok := (&http.Request{Header: r.Header}).BasicAuth(); !ok || user != "n9e" || pass != "pass-word-123" {
		t.Fatalf("basic auth missing: %v", r.Header)
	}
	if m.Priority != 3 || m.Tags[0] != "white_check_mark" || len(m.Actions) != 1 {
		t.Fatalf("unexpected recovery message: %s", r.Body)
	}
	req.Events[0].IsRecovered = false
	for severity, want := range map[int]int{2: 4, 3: 3} {
		req.Events[0].Severity = severity
		if res := (&NtfyProvider{}).Notify(context.Background(), req); res.Err != nil {
			t.Fatal(res.Err)
		}
		calls := got()
		if err := json.Unmarshal(calls[len(calls)-1].Body, &m); err != nil {
			t.Fatal(err)
		}
		if m.Priority != want || m.Tags[0] != "warning" {
			t.Fatalf("S%d must use fixed priority %d: %+v", severity, want, m)
		}
	}
}

func TestNtfyTopicIsRedactedInLogs(t *testing.T) {
	got := RedactParamsForLog(map[string]string{"topic": "n9e-secret-topic-9f8e7d", "bot_name": "Ops phones", "routing_key": "database", "to": testLineUser})
	if got["topic"] != redactedMark || got["routing_key"] != redactedMark || got["bot_name"] != "Ops phones" {
		t.Fatalf("unexpected log redaction: %v", got)
	}
}

func TestNtfyTruncatesByBytes(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"id":"x"}`))
	req := ntfyReq(srv.URL, nil)
	// 2000 个中文字符是 6000 字节，按字符数截不会触发，按字节必须截到 4096 以内
	req.TplContent["content"] = strings.Repeat("告", 2000)
	if res := (&NtfyProvider{}).Notify(context.Background(), req); res.Err != nil {
		t.Fatal(res.Err)
	}
	var m ntfyMessage
	json.Unmarshal(got()[0].Body, &m)
	if len(m.Message) > ntfyMaxMessageBytes || !utf8.ValidString(m.Message) || !strings.HasSuffix(m.Message, "…") {
		t.Fatalf("message must be cut to %d bytes on a rune boundary, got %d bytes", ntfyMaxMessageBytes, len(m.Message))
	}
	if len(m.Message) < ntfyMaxMessageBytes-3 {
		t.Fatalf("message cut too short: %d bytes", len(m.Message))
	}
}

func TestNtfyErrorsAndValidation(t *testing.T) {
	// 42901（限流）重试，42908（当日额度用完）不重试
	srv, got := newCaptureServer(t, func(n int, _ capturedRequest) (int, string) {
		if n == 1 {
			return 429, `{"code":42901,"http":429,"error":"limit reached: too many requests"}`
		}
		return 200, `{"id":"x"}`
	})
	if res := (&NtfyProvider{}).Notify(context.Background(), ntfyReq(srv.URL, nil)); res.Err != nil || len(got()) != 2 {
		t.Fatalf("rate limit should be retried: %+v calls=%d", res, len(got()))
	}
	srv, got = newCaptureServer(t, always(429, `{"code":42908,"http":429,"error":"limit reached: daily message quota reached"}`))
	res := (&NtfyProvider{}).Notify(context.Background(), ntfyReq(srv.URL, nil))
	if res.Err == nil || len(got()) != 1 || !strings.Contains(res.Err.Error(), "42908") {
		t.Fatalf("daily quota must fail without retry: %+v calls=%d", res, len(got()))
	}
	srv, _ = newCaptureServer(t, always(403, `{"code":40301,"http":403,"error":"forbidden","link":"https://ntfy.sh/docs/publish/#authentication"}`))
	res = (&NtfyProvider{}).Notify(context.Background(), ntfyReq(srv.URL, func(c *models.NtfyRequestConfig) { c.Token = "tk_secretsecretsecret" }))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "40301 forbidden") || !strings.Contains(res.Err.Error(), "not allowed to publish") {
		t.Fatalf("expected a hinted 403: %+v", res)
	}
	noLeak(t, res, "tk_secretsecretsecret", "n9e-secret-topic-9f8e7d")

	p := &NtfyProvider{}
	for _, topic := range []string{"", "has space", "https://ntfy.sh/mytopic", strings.Repeat("a", 65)} {
		req := ntfyReq("http://127.0.0.1:1", nil)
		req.CustomParams["topic"] = topic
		if res := p.Notify(context.Background(), req); res.Err == nil {
			t.Fatalf("accepted invalid topic %q", topic)
		}
	}
	bad := []func(*models.NtfyRequestConfig){
		func(c *models.NtfyRequestConfig) { c.ServerURL = "ntfy.sh" },
		func(c *models.NtfyRequestConfig) { c.Token, c.Username, c.Password = "tk_x", "u", "p" },
		func(c *models.NtfyRequestConfig) { c.Username = "only-user" },
	}
	for i, m := range bad {
		if err := p.Check(ntfyReq("", m).Config); err == nil {
			t.Fatalf("invalid config #%d passed", i)
		}
	}
}

// ---------- Splunk On-Call ----------

const testSplunkKey = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

func splunkReq(apiURL string, mutate func(*models.SplunkOnCallRequestConfig)) *NotifyRequest {
	cfg := &models.SplunkOnCallRequestConfig{APIURL: apiURL, APIKey: testSplunkKey, NativeNetworkConfig: fastRetry()}
	if mutate != nil {
		mutate(cfg)
	}
	req := webhookReq(models.RequestTypeSplunkOnCall, &models.RequestConfig{SplunkOnCallRequestConfig: cfg}, map[string]string{"routing_key": "database"})
	req.TplContent = map[string]interface{}{"title": "[S1] cpu high · db-01", "content": "Severity: S1 Triggered"}
	req.Events[0].TargetIdent = "db-01"
	return req
}

func TestSplunkOnCallTriggerAndRecovery(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"result":"success","entity_id":"h"}`))
	// 用户把集成页面给的完整地址（带 key 和 $routing_key）整段贴进来也要能用
	req := splunkReq(srv.URL+"/integrations/generic/20131114/alert/"+testSplunkKey+"/$routing_key", nil)
	res := (&SplunkOnCallProvider{}).Notify(context.Background(), req)
	if res.Err != nil || res.Target != "database" || res.Response != "CRITICAL alert sent, entity_id h" {
		t.Fatalf("unexpected result: %+v", res)
	}
	noLeak(t, res, testSplunkKey)
	r := got()[0]
	if r.Method != "POST" || r.Path != "/integrations/generic/20131114/alert/"+testSplunkKey+"/database" {
		t.Fatalf("unexpected endpoint: %s %s", r.Method, r.Path)
	}
	body := r.json(t)
	want := map[string]string{
		"message_type": "CRITICAL", "entity_id": "h", "entity_display_name": "[S1] cpu high · db-01",
		"state_message": "Severity: S1 Triggered", "monitoring_tool": "Nightingale",
		"host_name": "db-01", "vo_annotate.u.Event details": "http://n9e.example.com/share/alert-his-events/42",
	}
	for k, v := range want {
		if body[k] != v {
			t.Fatalf("payload %s = %v, want %q", k, body[k], v)
		}
	}

	// 恢复：同一个 entity_id、RECOVERY、不带注解；测试发送的 nonce 拼进 entity_id
	req.Events[0].IsRecovered = true
	req.CustomParams[TestNonceParam] = "n1"
	req.CustomParams["bot_name"] = "DB team"
	res = (&SplunkOnCallProvider{}).Notify(context.Background(), req)
	if res.Err != nil || res.Target != "DB team" || res.Response != "recovery sent, entity_id h-n1" {
		t.Fatalf("unexpected recovery result: %+v", res)
	}
	body = got()[1].json(t)
	if body["message_type"] != "RECOVERY" || body["entity_id"] != "h-n1" || body["vo_annotate.u.Event details"] != nil {
		t.Fatalf("unexpected recovery payload: %v", body)
	}
}

func TestSplunkOnCallFixedMessageTypesAndFailures(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"result":"success"}`))
	req := splunkReq(srv.URL, nil)
	for sev, want := range map[int]string{1: "CRITICAL", 2: "WARNING", 3: "WARNING"} {
		req.Events[0].Severity = sev
		if res := (&SplunkOnCallProvider{}).Notify(context.Background(), req); res.Err != nil {
			t.Fatal(res.Err)
		}
		calls := got()
		if mt := calls[len(calls)-1].json(t)["message_type"]; mt != want {
			t.Fatalf("S%d message_type = %v, want %s", sev, mt, want)
		}
	}

	// 200 + result=failure 是失败，且不重试
	srv, got = newCaptureServer(t, always(200, `{"result":"failure","message":"Missing fields: message_type"}`))
	res := (&SplunkOnCallProvider{}).Notify(context.Background(), splunkReq(srv.URL, nil))
	if res.Err == nil || len(got()) != 1 || !strings.Contains(res.Err.Error(), "Missing fields: message_type") {
		t.Fatalf("result=failure must fail: %+v", res)
	}
	// 5xx 重试；报错里不能带 API Key
	srv, got = newCaptureServer(t, always(503, "unavailable"))
	res = (&SplunkOnCallProvider{}).Notify(context.Background(), splunkReq(srv.URL, nil))
	if res.Err == nil || len(got()) != 3 {
		t.Fatalf("5xx must be retried: %+v calls=%d", res, len(got()))
	}
	res = (&SplunkOnCallProvider{}).Notify(context.Background(), splunkReq("http://127.0.0.1:1/alert", nil))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "cannot reach Splunk On-Call") {
		t.Fatalf("expected a network error: %+v", res)
	}
	noLeak(t, res, testSplunkKey)

	p := &SplunkOnCallProvider{}
	for _, key := range []string{"", "a/b", "https://alert.victorops.com/x"} {
		req := splunkReq("http://127.0.0.1:1", nil)
		req.CustomParams["routing_key"] = key
		if res := p.Notify(context.Background(), req); res.Err == nil {
			t.Fatalf("accepted invalid routing key %q", key)
		}
	}
	bad := []func(*models.SplunkOnCallRequestConfig){
		func(c *models.SplunkOnCallRequestConfig) { c.APIKey = " " },
		func(c *models.SplunkOnCallRequestConfig) { c.APIURL = "alert.victorops.com" },
	}
	for i, m := range bad {
		if err := p.Check(splunkReq("", m).Config); err == nil {
			t.Fatalf("invalid config #%d passed", i)
		}
	}
}

// ---------- LINE ----------

const (
	testLineToken = "eyJhbGciOiJIUzI1NiJ9.line-channel-access-token.signature0123456789"
	testLineUser  = "U4af4980629abcdef0123456789abcdef"
)

func lineReq(apiURL string) *NotifyRequest {
	cfg := &models.LineRequestConfig{APIURL: apiURL, ChannelAccessToken: testLineToken, NativeNetworkConfig: fastRetry()}
	return webhookReq(models.RequestTypeLine, &models.RequestConfig{LineRequestConfig: cfg}, map[string]string{"to": testLineUser})
}

func TestLinePushesText(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"sentMessages":[{"id":"461230966842064897","quoteToken":"q"}]}`))
	req := lineReq(srv.URL + "/")
	req.TplContent["content"] = "Target: db-01\nValue: 92"
	res := (&LineProvider{}).Notify(context.Background(), req)
	if res.Err != nil || res.Response != "sent, message id 461230966842064897" || res.Target != "***cdef" {
		t.Fatalf("unexpected result: %+v", res)
	}
	noLeak(t, res, testLineToken, testLineUser)
	r := got()[0]
	if r.Method != "POST" || r.Path != "/v2/bot/message/push" || r.Header.Get("Authorization") != "Bearer "+testLineToken || len(r.Header.Get("X-Line-Retry-Key")) != 36 {
		t.Fatalf("unexpected request: %+v", r)
	}
	var body linePushRequest
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatal(err)
	}
	wantText := "🔴 [S1] Triggered: cpu <high> & hot\n\nTarget: db-01\nValue: 92\n\nhttp://n9e.example.com/share/alert-his-events/42"
	if body.To != testLineUser || len(body.Messages) != 1 || body.Messages[0].Type != "text" || body.Messages[0].Text != wantText {
		t.Fatalf("unexpected body: %s", r.Body)
	}
}

func TestLineRetryKeyAndConflict(t *testing.T) {
	// 第一次 500（LINE 实际上可能已经受理），重试时带同一个 key，LINE 回 409：按成功处理，不再发第三次
	srv, got := newCaptureServer(t, func(n int, _ capturedRequest) (int, string) {
		if n == 1 {
			return 500, `{"message":"Internal server error"}`
		}
		return 409, `{"message":"The retry key is already accepted"}`
	})
	res := (&LineProvider{}).Notify(context.Background(), lineReq(srv.URL))
	calls := got()
	if res.Err != nil || len(calls) != 2 || !strings.Contains(res.Response, "earlier attempt") {
		t.Fatalf("409 after a retry must count as delivered: %+v calls=%d", res, len(calls))
	}
	if k := calls[0].Header.Get("X-Line-Retry-Key"); k == "" || k != calls[1].Header.Get("X-Line-Retry-Key") {
		t.Fatal("all attempts of one notification must share the retry key")
	}
	// 两次通知的 key 不同
	(&LineProvider{}).Notify(context.Background(), lineReq(srv.URL))
	if calls = got(); calls[2].Header.Get("X-Line-Retry-Key") == calls[0].Header.Get("X-Line-Retry-Key") {
		t.Fatal("each notification needs its own retry key")
	}
}

func TestLineErrorsAndValidation(t *testing.T) {
	// 当月额度用完不重试；普通限流重试
	srv, got := newCaptureServer(t, always(429, `{"message":"You have reached your monthly limit."}`))
	res := (&LineProvider{}).Notify(context.Background(), lineReq(srv.URL))
	if res.Err == nil || len(got()) != 1 || !strings.Contains(res.Err.Error(), "monthly message quota") {
		t.Fatalf("monthly limit must fail without retry: %+v calls=%d", res, len(got()))
	}
	srv, got = newCaptureServer(t, func(n int, _ capturedRequest) (int, string) {
		if n == 1 {
			return 429, `{"message":"The API rate limit has been exceeded. Try again later."}`
		}
		return 200, `{}`
	})
	if res := (&LineProvider{}).Notify(context.Background(), lineReq(srv.URL)); res.Err != nil || len(got()) != 2 {
		t.Fatalf("rate limit should be retried: %+v calls=%d", res, len(got()))
	}
	srv, got = newCaptureServer(t, always(400, `{"message":"Failed to send messages"}`))
	res = (&LineProvider{}).Notify(context.Background(), lineReq(srv.URL))
	if res.Err == nil || len(got()) != 1 || !strings.Contains(res.Err.Error(), "added the official account as a friend") {
		t.Fatalf("expected a hinted 400: %+v", res)
	}
	srv, _ = newCaptureServer(t, always(401, `{"message":"Authentication failed. Confirm that the access token in the authorization header is valid."}`))
	res = (&LineProvider{}).Notify(context.Background(), lineReq(srv.URL))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "issue a new one") {
		t.Fatalf("expected a hinted 401: %+v", res)
	}
	noLeak(t, res, testLineToken)

	p := &LineProvider{}
	for _, to := range []string{"", "@lineid", "U123", "X4af4980629abcdef0123456789abcdef", "U4AF4980629ABCDEF0123456789ABCDEF"} {
		req := lineReq("http://127.0.0.1:1")
		req.CustomParams["to"] = to
		if res := p.Notify(context.Background(), req); res.Err == nil {
			t.Fatalf("accepted invalid recipient %q", to)
		}
	}
	req := lineReq("")
	req.Config.RequestConfig.LineRequestConfig.ChannelAccessToken = ""
	if err := p.Check(req.Config); err == nil {
		t.Fatal("empty channel access token passed")
	}
}

func TestLineTextKeepsLinkWhenTruncated(t *testing.T) {
	req := lineReq("")
	// emoji 在 UTF-16 里占 2 个码元：3000 个就超过 5000 的上限
	req.TplContent["content"] = strings.Repeat("🔥", 3000)
	text := buildLineText(req, req.Events[0])
	units := 0
	for _, r := range text {
		units++
		if r > 0xFFFF {
			units++
		}
	}
	if units > lineMaxTextUnits || !strings.HasSuffix(text, "/share/alert-his-events/42") || !utf8.ValidString(text) {
		t.Fatalf("text must fit in %d UTF-16 units and keep the link, got %d units", lineMaxTextUnits, units)
	}
}

func TestTruncateHelpers(t *testing.T) {
	if s, cut := truncateInBytes("abc", 3); s != "abc" || cut {
		t.Fatalf("no truncation expected: %q", s)
	}
	if s, cut := truncateInBytes("告警告警", 8); s != "告…" || !cut {
		t.Fatalf("unexpected byte truncation: %q", s)
	}
	if s, cut := truncateInUTF16("ab🔥cd", 6); s != "ab🔥cd" || cut {
		t.Fatalf("exact fit must not be cut: %q", s)
	}
	if s, cut := truncateInUTF16("ab🔥cd", 5); s != "ab🔥…" || !cut {
		t.Fatalf("unexpected utf16 truncation: %q", s)
	}
	if got := redactSecrets("GET /x/secret-value?k=to%2Bken+val abc", "secret-value", "to+ken val", "abc"); got != "GET /x/***?k=*** abc" {
		t.Fatalf("unexpected redaction: %q", got)
	}
}
