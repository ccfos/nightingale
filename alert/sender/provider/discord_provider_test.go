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

type discordCapture struct {
	body  discordWebhook
	query string
	calls int
}

func newDiscordServer(t *testing.T, status int, respBody string) (*discordCapture, *httptest.Server) {
	c := &discordCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.calls++
		c.query = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		c.body = discordWebhook{}
		json.Unmarshal(b, &c.body)
		w.WriteHeader(status)
		w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return c, srv
}

func discordReq(cfg *models.DiscordRequestConfig, params map[string]string) *NotifyRequest {
	rc := &models.RequestConfig{}
	if cfg != nil {
		rc.DiscordRequestConfig = cfg
	}
	return &NotifyRequest{
		Config:       &models.NotifyChannelConfig{Name: "Discord", Ident: "discord", RequestType: models.RequestTypeDiscord, RequestConfig: rc},
		Events:       []*models.AlertCurEvent{{Id: 42, Hash: "h", RuleName: "cpu high", Severity: 1, TriggerTime: 1700000000}},
		TplContent:   map[string]interface{}{"content": "**Rule**: cpu high @everyone"},
		CustomParams: params,
		HttpClient:   &http.Client{Timeout: 5 * time.Second},
		SiteUrl:      "http://n9e.example.com",
	}
}

func TestDiscordCheckToleratesEmptyConfig(t *testing.T) {
	p := &DiscordProvider{}
	nc := &models.NotifyChannelConfig{Ident: "discord", RequestType: models.RequestTypeDiscord}
	if err := p.Check(nc); err != nil {
		t.Fatalf("nil request config must be accepted: %v", err)
	}
	if got, ok := DefaultRegistry.Resolve(nc); !ok || got.Ident() != "discord" {
		t.Fatalf("native discord should resolve to the discord provider, got %v", got)
	}
	legacy := &models.NotifyChannelConfig{Ident: "discord", RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://x", Method: "POST"}}}
	if got, ok := DefaultRegistry.Resolve(legacy); !ok || got.Ident() != "callback" {
		t.Fatalf("legacy http discord record should fall back to callback, got %v", got)
	}
	if _, err := models.GetHTTPClient(nc); err != nil {
		t.Fatalf("GetHTTPClient must accept an empty native config: %v", err)
	}
}

func TestDiscordSendsEmbed(t *testing.T) {
	c, srv := newDiscordServer(t, http.StatusOK, `{"id":"123"}`)
	req := discordReq(&models.DiscordRequestConfig{Username: "n9e", Silent: true}, map[string]string{
		"webhook_url": srv.URL + "/api/webhooks/1/secret-token",
	})
	res := (&DiscordProvider{}).Notify(context.Background(), req)
	if res.Err != nil {
		t.Fatalf("notify: %v", res.Err)
	}
	if !strings.Contains(c.query, "wait=true") {
		t.Fatalf("must call with wait=true, got %q", c.query)
	}
	b := c.body
	if b.Username != "n9e" || b.Flags != discordFlagSuppressNotify {
		t.Fatalf("unexpected payload %+v", b)
	}
	if b.AllowedMentions.Parse == nil || len(b.AllowedMentions.Parse) != 0 {
		t.Fatalf("@everyone in the alert text must never ping: %+v", b.AllowedMentions)
	}
	e := b.Embeds[0]
	if e.Title != "[S1] Triggered: cpu high" || e.Color != severityColor(1, false) || e.URL != "http://n9e.example.com/share/alert-his-events/42" || e.Timestamp == "" {
		t.Fatalf("unexpected embed %+v", e)
	}
	if res.Target != srv.URL+"/api/webhooks/1/***" || strings.Contains(res.Target, "secret-token") {
		t.Fatalf("target must not leak the webhook token: %q", res.Target)
	}
	if res.Response != "sent, message id 123" {
		t.Fatalf("response: %q", res.Response)
	}
}

func TestDiscordTargetUsesRuleName(t *testing.T) {
	_, srv := newDiscordServer(t, http.StatusNoContent, "")
	res := (&DiscordProvider{}).Notify(context.Background(), discordReq(nil, map[string]string{
		"webhook_url": srv.URL + "/api/webhooks/1/tok", "bot_name": "#prod-alerts",
	}))
	if res.Err != nil || res.Target != "#prod-alerts" {
		t.Fatalf("204 must be success and target the rule name: %+v", res)
	}
}

func TestDiscordForumPostAndThread(t *testing.T) {
	c, srv := newDiscordServer(t, http.StatusOK, `{}`)
	p := &DiscordProvider{}
	res := p.Notify(context.Background(), discordReq(nil, map[string]string{
		"webhook_url": srv.URL + "/api/webhooks/1/tok", "target": "forum_post", "thread_name": "{{$event.RuleName}} on host",
	}))
	if res.Err != nil || c.body.ThreadName != "cpu high on host" {
		t.Fatalf("forum post should render the thread name template: %+v %q", res, c.body.ThreadName)
	}
	res = p.Notify(context.Background(), discordReq(nil, map[string]string{
		"webhook_url": srv.URL + "/api/webhooks/1/tok", "target": "thread", "thread_id": "987654",
	}))
	if res.Err != nil || !strings.Contains(c.query, "thread_id=987654") || c.body.ThreadName != "" {
		t.Fatalf("thread target should pass thread_id as a query param: %+v %q", res, c.query)
	}
	res = p.Notify(context.Background(), discordReq(nil, map[string]string{
		"webhook_url": srv.URL + "/api/webhooks/1/tok", "target": "thread", "thread_id": "abc",
	}))
	if res.Err == nil {
		t.Fatal("non-numeric thread id must be rejected")
	}
}

func TestDiscordTerminalErrorWithHint(t *testing.T) {
	c, srv := newDiscordServer(t, http.StatusNotFound, `{"message":"Unknown Webhook","code":10015}`)
	res := (&DiscordProvider{}).Notify(context.Background(), discordReq(nil, map[string]string{"webhook_url": srv.URL + "/api/webhooks/1/tok"}))
	if res.Err == nil || c.calls != 1 {
		t.Fatalf("unknown webhook is terminal and must not be retried: err=%v calls=%d", res.Err, c.calls)
	}
	var he *HintError
	if !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "deleted") || !strings.Contains(res.Err.Error(), "10015 Unknown Webhook") {
		t.Fatalf("error should keep the raw code and carry a hint: %v", res.Err)
	}
}

func TestDiscordRejectsBadParams(t *testing.T) {
	p := &DiscordProvider{}
	for _, params := range []map[string]string{
		{},
		{"webhook_url": "not a url"},
		{"webhook_url": "https://discord.com/api/webhooks/1/t", "target": "dm"},
	} {
		if res := p.Notify(context.Background(), discordReq(nil, params)); res.Err == nil {
			t.Fatalf("params %v should be rejected", params)
		}
	}
}

func TestDiscordEmbedLengthLimits(t *testing.T) {
	c, srv := newDiscordServer(t, http.StatusOK, `{}`)
	req := discordReq(nil, map[string]string{"webhook_url": srv.URL + "/api/webhooks/1/tok"})
	req.TplContent = map[string]interface{}{"title": strings.Repeat("t", 300), "content": strings.Repeat("d", 9000)}
	if res := (&DiscordProvider{}).Notify(context.Background(), req); res.Err != nil {
		t.Fatal(res.Err)
	}
	e := c.body.Embeds[0]
	if n := len([]rune(e.Title)); n != discordMaxTitleRunes {
		t.Fatalf("title runes %d", n)
	}
	if n := len([]rune(e.Description)); n != discordMaxDescriptionRunes || len([]rune(e.Title))+n > discordMaxEmbedRunes {
		t.Fatalf("description runes %d", n)
	}
}

func TestMaskWebhookURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://discord.com/api/webhooks/123/abcDEF?wait=true": "https://discord.com/api/webhooks/123/***",
		"https://hooks.slack.com/services/T1/B2/xyz":            "https://hooks.slack.com/services/T1/B2/***",
		"https://mm.example.com/hooks/abc123/":                  "https://mm.example.com/hooks/***",
		"garbage":                                               "(invalid webhook url)",
	} {
		if got := maskWebhookURL(in); got != want {
			t.Fatalf("maskWebhookURL(%q) = %q, want %q", in, got, want)
		}
	}
}
