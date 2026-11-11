package provider

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestTeamsCardRecoveryAndContextActions(t *testing.T) {
	req := webhookReq(models.Teams, nil, nil)
	event := req.Events[0]
	event.TargetIdent = "db_primary_01"
	event.TriggerValue = "92.4%"
	event.FirstTriggerTime = 1700000000
	event.LastEvalTime = 1700000300
	event.Cate = "prometheus"
	req.SiteUrl = "https://n9e.example.com/monitoring/"
	card := buildTeamsCard(req)
	if card.Body[0].Style != "attention" || len(card.Actions) != 3 {
		t.Fatalf("missing firing context: %+v", card)
	}
	if card.Body[1].Columns[1].Items[1].Text != "92.4%" {
		t.Fatal("trigger value must be prominent")
	}
	if card.Body[1].Columns[0].Items[1].Text != `db\_primary\_01` {
		t.Fatal("target must not be interpreted as Markdown")
	}
	if !strings.HasPrefix(card.Actions[0].URL, "https://n9e.example.com/monitoring/share/") || card.Actions[1].Title != "Silence 1h" {
		t.Fatalf("invalid action URLs: %+v", card.Actions)
	}
	if !strings.Contains(card.Body[len(card.Body)-1].Text, "{{DATE(2023-11-14T22:13:20Z, SHORT)}}") {
		t.Fatal("time must be localized by the viewing Teams client")
	}
	event.IsRecovered = true
	card = buildTeamsCard(req)
	summary := card.Body[1].Columns[1].Items
	if card.Body[0].Style != "good" || summary[0].Text != "Duration" || summary[1].Text != "5m 0s" {
		t.Fatalf("invalid recovered summary: %+v", summary)
	}
	for _, action := range card.Actions {
		if action.Title == "Silence 1h" {
			t.Fatal("recovery should not offer a silence action")
		}
	}
	event.Cate = "host"
	if len(buildTeamsCard(req).Actions) != 1 {
		t.Fatal("non-Prometheus events must not link to metric explorer")
	}
	for _, site := range []string{"", "/relative", "javascript:alert(1)", "https://user:password@example.com", "https://example.com?token=x"} {
		req.SiteUrl = site
		if len(buildTeamsCard(req).Actions) != 0 {
			t.Fatalf("unsafe/incomplete site URL must not produce actions: %q", site)
		}
	}
}

func TestTeamsCardOmitsMissingMetadataAndKeepsCustomBody(t *testing.T) {
	req := webhookReq(models.Teams, nil, nil)
	req.Events = nil
	req.TplContent = map[string]interface{}{"content": "Custom **Markdown**\n第二行 > &", "title": "Database [alert]"}
	card := buildTeamsCard(req)
	if len(card.Body) != 2 || card.Body[1].Text != req.TplContent["content"] || len(card.Actions) != 0 {
		t.Fatalf("missing events must preserve the custom body without fabricated metadata: %+v", card)
	}
	if card.Body[0].Items[1].Text != `Database \[alert\]` {
		t.Fatal("custom title was not preserved as literal text")
	}
	req.Events = []*models.AlertCurEvent{{IsRecovered: true, FirstTriggerTime: 200, LastEvalTime: 100}}
	card = buildTeamsCard(req)
	for _, block := range card.Body {
		if block.Type == "ColumnSet" {
			t.Fatal("do not display a negative duration")
		}
	}
}

func TestTeamsDefaultBodyFocusesOnNotesAndExtraLabels(t *testing.T) {
	event := &models.AlertCurEvent{RuleName: "CPU high", TargetIdent: "db-01", Severity: 1, TriggerValue: "92.4%", RuleNote: "CPU > 90% & hot",
		TagsMap: map[string]string{"ident": "db-01", "rulename": "CPU high", "__name__": "cpu_usage", "env": "production", "region": "ap-southeast-1"}}
	tpl := &models.MessageTemplate{NotifyChannelIdent: models.Teams, Content: map[string]string{"content": models.NewTplMap[models.Teams]}}
	got := tpl.RenderEventForChannel(models.Teams, []*models.AlertCurEvent{event}, "https://n9e.example.com")["content"].(string)
	for _, want := range []string{"CPU > 90% & hot", "**Labels**  cpu_usage · env=production · region=ap-southeast-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	for _, duplicate := range []string{"Rule Title", "Level Status", "Send Time", "rulename=", "ident=", "__name__=", "/share/", "92.4%"} {
		if strings.Contains(got, duplicate) {
			t.Fatalf("default body duplicates native card metadata: %q", got)
		}
	}
	event.RuleNote = ""
	event.TagsMap = nil
	if got := tpl.RenderEventForChannel(models.Teams, []*models.AlertCurEvent{event}, "")["content"]; got != "" {
		t.Fatalf("empty notes/labels should not leave empty headings: %q", got)
	}
}
