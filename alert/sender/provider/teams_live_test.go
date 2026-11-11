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

// TestTeamsLive is opt-in: it posts a firing and a recovered card to the supplied
// Teams test workflow. Keep the signed URL out of source control and test logs.
func TestTeamsLive(t *testing.T) {
	webhook := os.Getenv("NIGHTINGALE_TEAMS_WEBHOOK_URL")
	if webhook == "" {
		t.Skip("set NIGHTINGALE_TEAMS_WEBHOOK_URL to test a real Teams workflow")
	}
	now := time.Now().Unix()
	run := fmt.Sprintf("Nightingale Teams E2E %s", time.Now().UTC().Format("20060102T150405Z"))
	ch := &models.NotifyChannelConfig{Name: "Microsoft Teams live test", Ident: models.Teams, RequestType: models.RequestTypeTeams,
		RequestConfig: &models.RequestConfig{TeamsRequestConfig: &models.TeamsRequestConfig{NativeNetworkConfig: models.NativeNetworkConfig{Timeout: 30000, RetryTimes: 0}}}}
	tpl := &models.MessageTemplate{NotifyChannelIdent: models.Teams, Content: map[string]string{"content": models.NewTplMap[models.Teams]}}
	event := &models.AlertCurEvent{Id: 42, Hash: run, RuleName: "CPU usage above 90% · Teams test", Cate: "prometheus", Severity: 1, TriggerTime: now - 60, FirstTriggerTime: now - 300, LastEvalTime: now,
		TargetIdent: "db-primary-01", TriggerValue: "92.4%", RuleNote: "CPU stayed above 90% for 5 minutes. Check busy processes or recent deployments.\nIntegration test · 中文 > 90% & \"quoted\".",
		TagsJSON: []string{"ident=db-primary-01", "env=staging", "region=ap-southeast-1"}}
	event.SetTagsMap()
	client, err := models.GetHTTPClient(ch)
	if err != nil {
		t.Fatal(err)
	}
	if proxy := os.Getenv("NIGHTINGALE_TEAMS_PROXY"); proxy != "" {
		ch.RequestConfig.TeamsRequestConfig.Proxy = proxy
		client, err = models.GetHTTPClient(ch)
		if err != nil {
			t.Fatal("invalid test proxy configuration")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, recovered := range []bool{false, true} {
		event.IsRecovered = recovered
		content := tpl.RenderEventForChannel(models.Teams, []*models.AlertCurEvent{event}, os.Getenv("NIGHTINGALE_TEST_SITE_URL"))
		res := (&TeamsProvider{}).Notify(ctx, &NotifyRequest{Config: ch, Events: []*models.AlertCurEvent{event}, TplContent: content,
			CustomParams: map[string]string{"webhook_url": webhook, "bot_name": "Nightingale integration test"}, HttpClient: client, SiteUrl: os.Getenv("NIGHTINGALE_TEST_SITE_URL")})
		if res.Err != nil {
			t.Fatalf("recovered=%v: %v", recovered, res.Err)
		}
		if strings.Contains(res.Target, webhook) || strings.Contains(res.Response, webhook) {
			t.Fatal("webhook URL leaked in result")
		}
		t.Logf("%s recovered=%v: %s", run, recovered, res.Response)
	}
}
