package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

type webhookCapture struct {
	body  map[string]json.RawMessage
	att   webhookAttachment
	calls int
}

func newWebhookServer(t *testing.T, tls bool, respond func(calls int, w http.ResponseWriter)) (*webhookCapture, *httptest.Server) {
	c := &webhookCapture{}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.calls++
		b, _ := io.ReadAll(r.Body)
		c.body = map[string]json.RawMessage{}
		json.Unmarshal(b, &c.body)
		var atts []webhookAttachment
		json.Unmarshal(c.body["attachments"], &atts)
		if len(atts) > 0 {
			c.att = atts[0]
		}
		respond(c.calls, w)
	})
	var srv *httptest.Server
	if tls {
		srv = httptest.NewTLSServer(h)
	} else {
		srv = httptest.NewServer(h)
	}
	t.Cleanup(srv.Close)
	return c, srv
}

func reply(status int, body string) func(int, http.ResponseWriter) {
	return func(_ int, w http.ResponseWriter) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}
}

func webhookReq(requestType string, rc *models.RequestConfig, params map[string]string) *NotifyRequest {
	if rc == nil {
		rc = &models.RequestConfig{}
	}
	return &NotifyRequest{
		Config:       &models.NotifyChannelConfig{Name: requestType, Ident: requestType, RequestType: requestType, RequestConfig: rc},
		Events:       []*models.AlertCurEvent{{Id: 42, Hash: "h", RuleName: "cpu <high> & hot", Severity: 1, TriggerTime: 1700000000}},
		TplContent:   map[string]interface{}{"content": "*Rule*: cpu"},
		CustomParams: params,
		HttpClient:   &http.Client{Timeout: 5 * time.Second},
		SiteUrl:      "http://n9e.example.com",
	}
}

func TestWebhookChannelsToleratesEmptyConfigAndLegacyRecords(t *testing.T) {
	for _, rt := range []string{models.RequestTypeSlackWebhook, models.RequestTypeMattermostWebhook} {
		nc := &models.NotifyChannelConfig{Ident: rt, RequestType: rt}
		got, ok := DefaultRegistry.Resolve(nc)
		if !ok || got.Ident() != rt {
			t.Fatalf("native %s should resolve to its provider, got %v", rt, got)
		}
		if _, err := models.GetHTTPClient(nc); err != nil {
			t.Fatalf("GetHTTPClient must accept an empty %s config: %v", rt, err)
		}
		legacy := &models.NotifyChannelConfig{Ident: rt, RequestType: "http",
			RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://x", Method: "POST"}}}
		if got, ok := DefaultRegistry.Resolve(legacy); !ok || got.Ident() != "callback" {
			t.Fatalf("legacy http %s record should fall back to callback, got %v", rt, got)
		}
	}
}

func TestSlackWebhookSendsAttachment(t *testing.T) {
	c, srv := newWebhookServer(t, false, reply(http.StatusOK, "ok"))
	req := webhookReq(models.RequestTypeSlackWebhook, nil, map[string]string{"webhook_url": srv.URL + "/services/T1/B1/secret-token"})
	res := (&SlackWebhookProvider{}).Notify(context.Background(), req)
	if res.Err != nil {
		t.Fatalf("notify: %v", res.Err)
	}
	a := c.att
	// 标题里的 < > & 必须转义，否则 Slack 会把 < 当成链接语法
	if a.Title != "[S1] Triggered: cpu &lt;high&gt; &amp; hot" || a.Fallback != a.Title {
		t.Fatalf("title must be escaped for Slack: %q", a.Title)
	}
	if a.Color != "#E01E5A" || a.TitleLink != "http://n9e.example.com/share/alert-his-events/42" || a.Text != "*Rule*: cpu" ||
		a.Ts != 1700000000 || len(a.MrkdwnIn) != 1 || a.MrkdwnIn[0] != "text" {
		t.Fatalf("unexpected attachment %+v", a)
	}
	if res.Target != srv.URL+"/services/T1/B1/***" || strings.Contains(res.Target, "secret-token") {
		t.Fatalf("target must not leak the webhook token: %q", res.Target)
	}
}

func TestSlackWebhookSuccessNeedsOKBody(t *testing.T) {
	_, srv := newWebhookServer(t, false, reply(http.StatusOK, "something else"))
	req := webhookReq(models.RequestTypeSlackWebhook, nil, map[string]string{"webhook_url": srv.URL + "/services/T1/B1/tok", "bot_name": "#ops"})
	res := (&SlackWebhookProvider{}).Notify(context.Background(), req)
	if res.Err == nil || res.Target != "#ops" {
		t.Fatalf("200 without an ok body is not a success: err=%v target=%q", res.Err, res.Target)
	}
}

func TestSlackWebhookErrorsAndRetry(t *testing.T) {
	c, srv := newWebhookServer(t, false, reply(http.StatusForbidden, "invalid_token"))
	res := (&SlackWebhookProvider{}).Notify(context.Background(),
		webhookReq(models.RequestTypeSlackWebhook, nil, map[string]string{"webhook_url": srv.URL + "/services/T1/B1/tok"}))
	var he *HintError
	if res.Err == nil || c.calls != 1 || !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "token") ||
		!strings.Contains(res.Err.Error(), "invalid_token") {
		t.Fatalf("invalid_token is terminal and carries a hint: err=%v calls=%d", res.Err, c.calls)
	}

	c, srv = newWebhookServer(t, false, func(calls int, w http.ResponseWriter) {
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	})
	res = (&SlackWebhookProvider{}).Notify(context.Background(),
		webhookReq(models.RequestTypeSlackWebhook, nil, map[string]string{"webhook_url": srv.URL + "/services/T1/B1/tok"}))
	if res.Err != nil || c.calls != 2 {
		t.Fatalf("429 should be retried: err=%v calls=%d", res.Err, c.calls)
	}

	for _, params := range []map[string]string{{}, {"webhook_url": "not a url"}, {"webhook_url": "https://hooks.slack.com/triggers/x"}} {
		if res := (&SlackWebhookProvider{}).Notify(context.Background(), webhookReq(models.RequestTypeSlackWebhook, nil, params)); res.Err == nil {
			t.Fatalf("params %v should be rejected", params)
		}
	}
}

func TestSlackTemplateEscapesAlertFields(t *testing.T) {
	tpl := &models.MessageTemplate{NotifyChannelIdent: models.SlackWebhook, Content: map[string]string{"content": models.NewTplMap[models.SlackWebhook]}}
	event := &models.AlertCurEvent{Id: 7, RuleName: "latency > 1s", RuleNote: "p99 > 1s & rising", Severity: 2, TargetIdent: "web-01",
		TriggerValue: "1.8", FirstTriggerTime: 1700000000, TriggerTime: 1700000060,
		TagsMap: map[string]string{"__name__": "http_latency", "ident": "web-01", "path": "/a&b", "rulename": "latency > 1s"}}
	render := func() string {
		got, _ := tpl.RenderEventForChannel(models.RequestTypeSlackWebhook, []*models.AlertCurEvent{event}, "http://n9e")["content"].(string)
		return got
	}

	got := render()
	for _, want := range []string{
		"*Value*  1.8      *Target*  web-01\n",    // 触发值和监控对象都短，合成一行
		"*Labels*  http_latency, path=/a&amp;b\n", // 指标名只写值，& 已转义
		"*Note*  p99 &gt; 1s &amp; rising\n",
		"*Started*  <!date^1700000000^{ago}|", // 时间交给 Slack 按查看者时区显示
		"<http://n9e/share/alert-his-events/7|Event details>",
		"|Silence 1h>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	// 与标题、监控对象重复的标签不再出现
	for _, dup := range []string{"__name__", "ident=", "rulename="} {
		if strings.Contains(got, dup) {
			t.Fatalf("%q duplicates the title or target and should be dropped:\n%s", dup, got)
		}
	}
	// 带告警描述时正文也不超过 5 行，Slack 才不会把链接折叠进 Show more
	if n := strings.Count(got, "\n") + 1; n > 5 {
		t.Fatalf("the Slack body should stay within 5 lines, got %d:\n%s", n, got)
	}

	event.IsRecovered, event.LastEvalTime = true, 1700000090
	got = render()
	if !strings.HasPrefix(got, "*Duration*  1m 30s      *Target*  web-01\n") || strings.Contains(got, "Silence 1h") || strings.Contains(got, "*Note*") {
		t.Fatalf("recovery should lead with the duration and drop the note and silence link:\n%s", got)
	}

	// 没有触发值和监控对象时不留空行，ident 退回到标签里
	event.IsRecovered, event.TriggerValue, event.TargetIdent = false, "", ""
	got = render()
	if !strings.HasPrefix(got, "*Labels*  http_latency, ident=web-01, path=/a&amp;b\n*Started*  ") {
		t.Fatalf("empty value and target should not leave a blank first line:\n%s", got)
	}
}

func TestMattermostWebhookSendsAttachment(t *testing.T) {
	c, srv := newWebhookServer(t, false, reply(http.StatusOK, "ok"))
	rc := &models.RequestConfig{MattermostWebhookRequestConfig: &models.MattermostWebhookRequestConfig{Username: "n9e", Icon: ":bell:"}}
	res := (&MattermostWebhookProvider{}).Notify(context.Background(),
		webhookReq(models.RequestTypeMattermostWebhook, rc, map[string]string{"webhook_url": srv.URL + "/hooks/secret-id", "bot_name": "#ops"}))
	if res.Err != nil || res.Target != "#ops" {
		t.Fatalf("notify: err=%v target=%q", res.Err, res.Target)
	}
	var username, emoji string
	json.Unmarshal(c.body["username"], &username)
	json.Unmarshal(c.body["icon_emoji"], &emoji)
	if username != "n9e" || emoji != "bell" || c.body["icon_url"] != nil {
		t.Fatalf("appearance overrides not applied: %s", c.body)
	}
	// Mattermost 是标准 Markdown，标题不做 Slack 那套转义
	if c.att.Title != "[S1] Triggered: cpu <high> & hot" || c.att.Color != "#E01E5A" || c.att.TitleLink == "" {
		t.Fatalf("unexpected attachment %+v", c.att)
	}

	rc.MattermostWebhookRequestConfig.Icon = "https://example.com/logo.png"
	(&MattermostWebhookProvider{}).Notify(context.Background(),
		webhookReq(models.RequestTypeMattermostWebhook, rc, map[string]string{"webhook_url": srv.URL + "/hooks/secret-id"}))
	var iconURL string
	json.Unmarshal(c.body["icon_url"], &iconURL)
	if iconURL != "https://example.com/logo.png" || c.body["icon_emoji"] != nil {
		t.Fatalf("an http icon must go to icon_url: %s", c.body)
	}
}

func TestMattermostWebhookErrorsAndCerts(t *testing.T) {
	_, srv := newWebhookServer(t, false, reply(http.StatusBadRequest,
		`{"id":"web.incoming_webhook.invalid.app_error","message":"Invalid webhook.","status_code":400}`))
	res := (&MattermostWebhookProvider{}).Notify(context.Background(),
		webhookReq(models.RequestTypeMattermostWebhook, nil, map[string]string{"webhook_url": srv.URL + "/hooks/x"}))
	var he *HintError
	if !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "deleted") || !strings.Contains(res.Err.Error(), "invalid.app_error") {
		t.Fatalf("invalid webhook should keep the raw id and carry a hint: %v", res.Err)
	}

	// Mattermost 的报错原文带着 Webhook ID（它本身就是凭证），落记录前必须遮掉
	_, srv = newWebhookServer(t, false, reply(http.StatusBadRequest,
		`{"id":"web.incoming_webhook.general.app_error","message":"Failed to handle the payload of media type application/json for incoming webhook k8hrsecrethookid9.","status_code":400}`))
	res = (&MattermostWebhookProvider{}).Notify(context.Background(),
		webhookReq(models.RequestTypeMattermostWebhook, nil, map[string]string{"webhook_url": srv.URL + "/hooks/k8hrsecrethookid9"}))
	if res.Err == nil || strings.Contains(res.Err.Error(), "k8hrsecrethookid9") || !errors.As(res.Err, &he) || he.Hint == "" {
		t.Fatalf("the webhook id must be masked in the error and a hint given: %v", res.Err)
	}

	// 自签名证书：默认失败并提示开启跳过证书校验，开启后成功
	_, tlsSrv := newWebhookServer(t, true, reply(http.StatusOK, "ok"))
	params := map[string]string{"webhook_url": tlsSrv.URL + "/hooks/x"}
	res = (&MattermostWebhookProvider{}).Notify(context.Background(), webhookReq(models.RequestTypeMattermostWebhook, nil, params))
	if !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "certificate") {
		t.Fatalf("a self-signed certificate should point at skip verification: %v", res.Err)
	}
	nc := &models.NotifyChannelConfig{Ident: models.MattermostWebhook, RequestType: models.RequestTypeMattermostWebhook,
		RequestConfig: &models.RequestConfig{MattermostWebhookRequestConfig: &models.MattermostWebhookRequestConfig{
			NativeNetworkConfig: models.NativeNetworkConfig{InsecureSkipVerify: true}}}}
	client, err := models.GetHTTPClient(nc)
	if err != nil {
		t.Fatal(err)
	}
	req := webhookReq(models.RequestTypeMattermostWebhook, nc.RequestConfig, params)
	req.HttpClient = client
	if res := (&MattermostWebhookProvider{}).Notify(context.Background(), req); res.Err != nil {
		t.Fatalf("skip verification should work: %v", res.Err)
	}

	bad := &models.NotifyChannelConfig{Ident: models.MattermostWebhook, RequestType: models.RequestTypeMattermostWebhook,
		RequestConfig: &models.RequestConfig{MattermostWebhookRequestConfig: &models.MattermostWebhookRequestConfig{Icon: "bell"}}}
	if err := (&MattermostWebhookProvider{}).Check(bad); err == nil {
		t.Fatalf("an icon that is neither a url nor an emoji code should be rejected")
	}
}
