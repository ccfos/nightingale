package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

// fakeJSM 模拟 JSM 集成接口：建告警 / 按 alias 关闭都回 202 + requestId，请求状态第一次查回 404（还没处理）
type fakeJSM struct {
	mu        sync.Mutex
	calls     []jsmCall
	status    int    // 非 0 时所有写请求返回该状态码
	tooMany   bool   // 第一次写请求返回 429
	failClose bool   // 关闭请求处理失败（告警不存在）
	asyncFail string // 非空时建告警的异步处理失败，状态为该文本
	polled    map[string]int
}

func init() {
	// 模拟服务第二次查询就有结果，测试里不用真等 500ms
	jsmPollGap = 10 * time.Millisecond
}

type jsmCall struct {
	Method, Path, Query, Auth string
	Body                      map[string]interface{}
}

func newFakeJSM(t *testing.T) (*fakeJSM, *httptest.Server) {
	f := &fakeJSM{polled: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeJSM) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(r.URL.Path, "/v2/alerts/requests/") {
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		f.polled[id]++
		if f.polled[id] == 1 {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Request not found. It might not be processed, yet."}`))
			return
		}
		ok := !(strings.HasPrefix(id, "close") && f.failClose)
		status := "Created alert"
		if strings.HasPrefix(id, "create") && f.asyncFail != "" {
			ok, status = false, f.asyncFail
		}
		if strings.HasPrefix(id, "close") {
			status = "Closed alert"
			if !ok {
				// JSM 对已关闭告警再关的真实状态文案
				status = "There is no open alert with alias [h1-n1]."
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"success": ok, "status": status, "alertId": "alert-1"}})
		return
	}
	var body map[string]interface{}
	b, _ := io.ReadAll(r.Body)
	json.Unmarshal(b, &body)
	f.calls = append(f.calls, jsmCall{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), body})
	if f.tooMany {
		f.tooMany = false
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
		w.Write([]byte(`{"message":"Key format is not valid!","took":0.001,"requestId":"x"}`))
		return
	}
	id := "create-1"
	if strings.HasSuffix(r.URL.Path, "/close") {
		id = "close-1"
	}
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"result":"Request will be processed","took":0.1,"requestId":"` + id + `"}`))
}

func jsmReq(apiURL string, params map[string]string, ev *models.AlertCurEvent) *NotifyRequest {
	return &NotifyRequest{
		Config: &models.NotifyChannelConfig{Name: "JSM Alert", Ident: models.JSMAlert, RequestType: models.RequestTypeJSMAlert,
			RequestConfig: &models.RequestConfig{JSMAlertRequestConfig: &models.JSMAlertRequestConfig{APIURL: apiURL,
				NativeNetworkConfig: models.NativeNetworkConfig{RetryTimes: 2, RetrySleep: 1}}}},
		Events:       []*models.AlertCurEvent{ev},
		TplContent:   map[string]interface{}{"title": "[S1] cpu high · host-01", "content": "Rule: cpu high\nValue: 95"},
		CustomParams: params,
		HttpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func jsmEvent(recovered bool) *models.AlertCurEvent {
	return &models.AlertCurEvent{Hash: "h1", RuleName: "cpu high", Severity: 1, TargetIdent: "host-01", TriggerTime: 100,
		TagsJSON: []string{"ident=host-01", "service=api"}, TagsMap: map[string]string{"ident": "host-01", "service": "api"},
		AnnotationsJSON: map[string]string{"runbook": "https://wiki/cpu"}, IsRecovered: recovered}
}

func TestJSMAlertCheckAndFallback(t *testing.T) {
	p := &JSMAlertProvider{}
	nc := &models.NotifyChannelConfig{Ident: models.JSMAlert, RequestType: models.RequestTypeJSMAlert}
	if err := p.Check(nc); err != nil {
		t.Fatalf("nil request config must be accepted: %v", err)
	}
	if got, ok := DefaultRegistry.Resolve(nc); !ok || got.Ident() != models.RequestTypeJSMAlert {
		t.Fatalf("native jsm_alert should resolve to its provider, got %v", got)
	}
	legacy := &models.NotifyChannelConfig{Ident: models.JSMAlert, RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://x", Method: "POST"}}}
	if got, ok := DefaultRegistry.Resolve(legacy); !ok || got.Ident() != "callback" {
		t.Fatalf("legacy http jsm_alert record should fall back to callback, got %v", got)
	}
	bad := &models.NotifyChannelConfig{Ident: models.JSMAlert, RequestType: models.RequestTypeJSMAlert,
		RequestConfig: &models.RequestConfig{JSMAlertRequestConfig: &models.JSMAlertRequestConfig{APIURL: "api.atlassian.com"}}}
	if err := p.Check(bad); err == nil {
		t.Fatal("api url without scheme should be rejected")
	}
	if _, err := models.GetHTTPClient(nc); err != nil {
		t.Fatalf("GetHTTPClient must accept an empty native config: %v", err)
	}
}

func TestJSMAlertCreatePayload(t *testing.T) {
	f, srv := newFakeJSM(t)
	res := (&JSMAlertProvider{}).Notify(context.Background(), jsmReq(srv.URL, map[string]string{"api_key": "key-1234abcd", "bot_name": "SRE team"}, jsmEvent(false)))
	if res.Err != nil {
		t.Fatalf("notify: %v", res.Err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected one request, got %d", len(f.calls))
	}
	c := f.calls[0]
	if c.Method != http.MethodPost || c.Path != "/jsm/ops/integration/v2/alerts" || c.Auth != "GenieKey key-1234abcd" {
		t.Fatalf("unexpected request %s %s auth=%s", c.Method, c.Path, c.Auth)
	}
	b := c.Body
	if b["alias"] != "h1" || b["message"] != "[S1] cpu high · host-01" || b["priority"] != "P1" || b["entity"] != "host-01" || b["source"] != "Nightingale" {
		t.Fatalf("unexpected payload %+v", b)
	}
	if !strings.Contains(b["description"].(string), "Value: 95") {
		t.Fatalf("description should be the rendered content: %v", b["description"])
	}
	details := b["details"].(map[string]interface{})
	if details["service"] != "api" || details["runbook"] != "https://wiki/cpu" {
		t.Fatalf("details should carry labels and annotations: %+v", details)
	}
	// 生产路径也查异步处理结果，记录里是真实结果
	if res.Target != "SRE team" || res.Response != "alert created, id alert-1" {
		t.Fatalf("unexpected result target=%s resp=%s", res.Target, res.Response)
	}
}

func TestJSMAlertCloseByAlias(t *testing.T) {
	f, srv := newFakeJSM(t)
	res := (&JSMAlertProvider{}).Notify(context.Background(), jsmReq(srv.URL+"/jsm/ops/integration/", map[string]string{"api_key": "k"}, jsmEvent(true)))
	if res.Err != nil {
		t.Fatalf("notify: %v", res.Err)
	}
	c := f.calls[0]
	if c.Path != "/jsm/ops/integration/v2/alerts/h1/close" || c.Query != "identifierType=alias" {
		t.Fatalf("recovery should close by alias, got %s?%s", c.Path, c.Query)
	}
	if c.Body["note"] != "Rule: cpu high\nValue: 95" || c.Body["source"] != "Nightingale" {
		t.Fatalf("unexpected close body %+v", c.Body)
	}
	if res.Target != "***" {
		t.Fatalf("a short key must be fully masked in the target, got %s", res.Target)
	}
}

func TestJSMAlertPriorityMapAndLimits(t *testing.T) {
	f, srv := newFakeJSM(t)
	ev := jsmEvent(false)
	ev.Severity = 2
	for i := 0; i < 30; i++ {
		ev.TagsJSON = append(ev.TagsJSON, "k"+strings.Repeat("x", 60)+"=v")
	}
	req := jsmReq(srv.URL, map[string]string{"api_key": "k"}, ev)
	// 优先级映射在媒介里配；小写也认
	req.Config.RequestConfig.JSMAlertRequestConfig.PriorityMap = map[string]string{"2": "p4"}
	req.TplContent["title"] = strings.Repeat("长", 200)
	if res := (&JSMAlertProvider{}).Notify(context.Background(), req); res.Err != nil {
		t.Fatalf("notify: %v", res.Err)
	}
	b := f.calls[0].Body
	if b["priority"] != "P4" {
		t.Fatalf("priority_map should override the default, got %v", b["priority"])
	}
	if n := len([]rune(b["message"].(string))); n > jsmMaxMessageRunes {
		t.Fatalf("message must be cut to %d runes, got %d", jsmMaxMessageRunes, n)
	}
	tags := b["tags"].([]interface{})
	if len(tags) != jsmMaxTags {
		t.Fatalf("tags must be capped at %d, got %d", jsmMaxTags, len(tags))
	}
	for _, tg := range tags {
		if len([]rune(tg.(string))) > jsmMaxTagRunes {
			t.Fatalf("tag too long: %s", tg)
		}
	}

	for _, bad := range []map[string]string{{"2": "urgent"}, {"4": "P1"}} {
		nc := &models.NotifyChannelConfig{Ident: models.JSMAlert, RequestType: models.RequestTypeJSMAlert,
			RequestConfig: &models.RequestConfig{JSMAlertRequestConfig: &models.JSMAlertRequestConfig{PriorityMap: bad}}}
		if err := (&JSMAlertProvider{}).Check(nc); err == nil {
			t.Fatalf("invalid priority map %v should be rejected when saving the channel", bad)
		}
	}
	// 没配映射时按默认 S1→P1、S2→P2、S3→P3
	var empty *models.JSMAlertRequestConfig
	if empty.Priority(1) != "P1" || empty.Priority(3) != "P3" {
		t.Fatal("default priorities")
	}
	if _, err := parseJSMParams(map[string]string{}); err == nil {
		t.Fatal("missing api_key should be rejected")
	}
}

func TestJSMAlertTestSendWaitsForProcessing(t *testing.T) {
	f, srv := newFakeJSM(t)
	params := map[string]string{"api_key": "k", TestNonceParam: "n1"}
	p := &JSMAlertProvider{}
	res := p.Notify(context.Background(), jsmReq(srv.URL, params, jsmEvent(false)))
	if res.Err != nil || res.Response != "alert created, id alert-1" {
		t.Fatalf("test send should report the processed result: %+v", res)
	}
	if f.calls[0].Body["alias"] != "h1-n1" {
		t.Fatalf("test sends must use a per-test alias, got %v", f.calls[0].Body["alias"])
	}
	res = p.Notify(context.Background(), jsmReq(srv.URL, params, jsmEvent(true)))
	if res.Err != nil || res.Response != "alert closed, id alert-1" || !strings.Contains(f.calls[1].Path, "/h1-n1/close") {
		t.Fatalf("recovery of a test send should close the same alias: %+v %s", res, f.calls[1].Path)
	}

	// 恢复时告警已不存在是正常结局
	f.failClose = true
	f.polled = map[string]int{}
	res = p.Notify(context.Background(), jsmReq(srv.URL, params, jsmEvent(true)))
	if res.Err != nil || !strings.Contains(res.Response, "nothing to close") {
		t.Fatalf("closing an alert that is gone should not fail: %+v", res)
	}
}

// 集成被关闭时接口照样回 202，失败只在异步处理结果里：生产路径也必须报出来，不能记成成功
func TestJSMAlertAsyncFailureSurfaces(t *testing.T) {
	f, srv := newFakeJSM(t)
	f.asyncFail = "Integration is disabled."
	res := (&JSMAlertProvider{}).Notify(context.Background(), jsmReq(srv.URL, map[string]string{"api_key": "k"}, jsmEvent(false)))
	var he *HintError
	if res.Err == nil || !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "turned off") {
		t.Fatalf("a disabled integration must fail with a hint: %+v", res)
	}
}

func TestJSMAlertProcessingTimeoutIsAccepted(t *testing.T) {
	old := jsmProdWait
	jsmProdWait = 0
	t.Cleanup(func() { jsmProdWait = old })
	_, srv := newFakeJSM(t)
	res := (&JSMAlertProvider{}).Notify(context.Background(), jsmReq(srv.URL, map[string]string{"api_key": "k"}, jsmEvent(false)))
	if res.Err != nil || !strings.Contains(res.Response, "create accepted, request id create-1") {
		t.Fatalf("a request not processed in time is accepted, not failed: %+v", res)
	}
}

func TestJSMAlertErrorsAndRetry(t *testing.T) {
	f, srv := newFakeJSM(t)
	f.tooMany = true
	if res := (&JSMAlertProvider{}).Notify(context.Background(), jsmReq(srv.URL, map[string]string{"api_key": "k"}, jsmEvent(false))); res.Err != nil {
		t.Fatalf("429 should be retried: %v", res.Err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("expected a retry after 429, got %d calls", len(f.calls))
	}

	f.status = http.StatusUnprocessableEntity
	f.calls = nil
	res := (&JSMAlertProvider{}).Notify(context.Background(), jsmReq(srv.URL, map[string]string{"api_key": "secret-key-9876"}, jsmEvent(false)))
	if res.Err == nil || len(f.calls) != 1 {
		t.Fatalf("422 is terminal and must not be retried: err=%v calls=%d", res.Err, len(f.calls))
	}
	var he *HintError
	if !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "fields") || !strings.Contains(res.Err.Error(), "Key format is not valid") {
		t.Fatalf("422 should carry the JSM message and a hint: %v", res.Err)
	}
	if strings.Contains(res.Err.Error(), "secret-key-9876") || res.Target != "***9876" {
		t.Fatalf("the api key must never appear in errors or targets: err=%v target=%s", res.Err, res.Target)
	}

	f.status = http.StatusUnauthorized
	res = (&JSMAlertProvider{}).Notify(context.Background(), jsmReq(srv.URL, map[string]string{"api_key": "k"}, jsmEvent(false)))
	if !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "API key") {
		t.Fatalf("401 should hint at the integration key: %v", res.Err)
	}
}

func TestJSMBaseURL(t *testing.T) {
	for in, want := range map[string]string{
		"":                              "https://api.atlassian.com/jsm/ops/integration/v2/alerts",
		"https://api.eu.atlassian.com/": "https://api.eu.atlassian.com/jsm/ops/integration/v2/alerts",
		"https://api.atlassian.com/jsm/ops/integration/v2/alerts": "https://api.atlassian.com/jsm/ops/integration/v2/alerts",
	} {
		if got := jsmBaseURL(&models.JSMAlertRequestConfig{APIURL: in}); got != want {
			t.Errorf("jsmBaseURL(%q) = %s, want %s", in, got, want)
		}
	}
}
