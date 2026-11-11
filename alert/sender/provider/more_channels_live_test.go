package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

// Opt-in smoke tests against the real services: each posts one firing and one recovered
// notification rendered with the built-in template. They are skipped unless the credentials are
// supplied through the environment, and never print them.
//
//	NIGHTINGALE_PUSHOVER_TOKEN, NIGHTINGALE_PUSHOVER_USER_KEY   go test -run '^TestPushoverLive$'
//	NIGHTINGALE_NTFY_TOPIC [NIGHTINGALE_NTFY_SERVER_URL, NIGHTINGALE_NTFY_TOKEN]   go test -run '^TestNtfyLive$'
//	NIGHTINGALE_SPLUNK_ONCALL_API_KEY, NIGHTINGALE_SPLUNK_ONCALL_ROUTING_KEY       go test -run '^TestSplunkOnCallLive$'
//	NIGHTINGALE_LINE_CHANNEL_ACCESS_TOKEN, NIGHTINGALE_LINE_TO  go test -run '^TestLineLive$'
//
// Optional for all: NIGHTINGALE_TEST_SITE_URL (adds the event links) and NIGHTINGALE_TEST_PROXY.
// API acceptance is not proof of delivery: check the space, phone or incident timeline as well.
func runLive(t *testing.T, p NotifyChannelProvider, ident string, rc *models.RequestConfig, params map[string]string, severity int, secrets ...string) {
	t.Helper()
	now := time.Now().Unix()
	run := fmt.Sprintf("Nightingale %s live %s", ident, time.Now().UTC().Format("20060102T150405Z"))
	ch := &models.NotifyChannelConfig{Name: ident + " live test", Ident: ident, RequestType: ident, RequestConfig: rc}
	if n := rc.NativeNetwork(ident); n != nil {
		n.Timeout, n.Proxy = 30000, os.Getenv("NIGHTINGALE_TEST_PROXY")
	}
	if err := p.Check(ch); err != nil {
		t.Fatal(err)
	}
	client, err := models.GetHTTPClient(ch)
	if err != nil {
		t.Fatal(err)
	}
	content := map[string]string{"content": models.NewTplMap[ident]}
	if ident == models.SplunkOnCall {
		content = map[string]string{"title": models.JiraTitle, "content": models.NewTplMap[models.Jira]}
	}
	tpl := &models.MessageTemplate{NotifyChannelIdent: ident, Content: content}
	event := &models.AlertCurEvent{Id: 42, Hash: strings.ReplaceAll(run, " ", "-"), RuleName: "CPU usage above 90% · live test", Cate: "prometheus",
		Severity: severity, TriggerTime: now - 60, FirstTriggerTime: now - 300, LastEvalTime: now, TargetIdent: "db-primary-01", TriggerValue: "92.4",
		RuleNote: "Integration test · 中文 > 90% & \"quoted\" <tag>.", TagsJSON: []string{"ident=db-primary-01", "env=staging", "team=a<b>&c"}}
	event.SetTagsMap()
	site := os.Getenv("NIGHTINGALE_TEST_SITE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, recovered := range []bool{false, true} {
		event.IsRecovered = recovered
		res := p.Notify(ctx, &NotifyRequest{Config: ch, Events: []*models.AlertCurEvent{event},
			TplContent: tpl.RenderEventForChannel(ident, []*models.AlertCurEvent{event}, site), CustomParams: params, HttpClient: client, SiteUrl: site})
		if res.Err != nil {
			t.Fatalf("recovered=%v: %v", recovered, res.Err)
		}
		for _, s := range secrets {
			if s != "" && (strings.Contains(res.Target, s) || strings.Contains(res.Response, s)) {
				t.Fatal("a credential leaked into the notify result")
			}
		}
		t.Logf("%s recovered=%v: target=%s response=%s", run, recovered, res.Target, res.Response)
	}
}

func moreLiveEnv(t *testing.T, keys ...string) []string {
	t.Helper()
	vals := make([]string, len(keys))
	for i, k := range keys {
		if vals[i] = os.Getenv(k); vals[i] == "" {
			t.Skipf("set %s to test against the real service", strings.Join(keys, ", "))
		}
	}
	return vals
}

func TestPushoverLive(t *testing.T) {
	v := moreLiveEnv(t, "NIGHTINGALE_PUSHOVER_TOKEN", "NIGHTINGALE_PUSHOVER_USER_KEY")
	rc := &models.RequestConfig{PushoverRequestConfig: &models.PushoverRequestConfig{Token: v[0]}}
	runLive(t, &PushoverProvider{}, models.Pushover, rc, map[string]string{"user_key": v[1]}, 1, v[0], v[1])
}

func TestNtfyLive(t *testing.T) {
	v := moreLiveEnv(t, "NIGHTINGALE_NTFY_TOPIC")
	rc := &models.RequestConfig{NtfyRequestConfig: &models.NtfyRequestConfig{ServerURL: os.Getenv("NIGHTINGALE_NTFY_SERVER_URL"), Token: os.Getenv("NIGHTINGALE_NTFY_TOKEN")}}
	runLive(t, &NtfyProvider{}, models.Ntfy, rc, map[string]string{"topic": v[0]}, 1, v[0], os.Getenv("NIGHTINGALE_NTFY_TOKEN"))
}

func TestSplunkOnCallLive(t *testing.T) {
	v := moreLiveEnv(t, "NIGHTINGALE_SPLUNK_ONCALL_API_KEY", "NIGHTINGALE_SPLUNK_ONCALL_ROUTING_KEY")
	rc := &models.RequestConfig{SplunkOnCallRequestConfig: &models.SplunkOnCallRequestConfig{APIKey: v[0]}}
	runLive(t, &SplunkOnCallProvider{}, models.SplunkOnCall, rc, map[string]string{"routing_key": v[1]}, 1, v[0])
}

func TestLineLive(t *testing.T) {
	v := moreLiveEnv(t, "NIGHTINGALE_LINE_CHANNEL_ACCESS_TOKEN", "NIGHTINGALE_LINE_TO")
	rc := &models.RequestConfig{LineRequestConfig: &models.LineRequestConfig{ChannelAccessToken: v[0]}}
	runLive(t, &LineProvider{}, models.Line, rc, map[string]string{"to": v[1]}, 2, v[0], v[1])
}
