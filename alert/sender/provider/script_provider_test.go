package provider

import (
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestScriptStdinCarriesRealNewlines(t *testing.T) {
	tpl := &models.MessageTemplate{
		NotifyChannelIdent: "custom",
		Content:            map[string]string{"content": "rule: {{$event.RuleName}}\nnote: \"{{$event.RuleNote}}\""},
	}
	evts := []*models.AlertCurEvent{{RuleName: "cpu", RuleNote: "a&b"}}
	content := tpl.RenderEventForChannel(ScriptIdent, evts, "http://n9e")

	var stdin struct {
		Tpl map[string]string `json:"tpl"`
	}
	if err := json.Unmarshal(getStdinBytes(evts, content, nil, nil), &stdin); err != nil {
		t.Fatal(err)
	}
	if want := "rule: cpu\nnote: \"a&b\""; stdin.Tpl["content"] != want {
		t.Fatalf("script stdin content = %q, want %q", stdin.Tpl["content"], want)
	}
}
