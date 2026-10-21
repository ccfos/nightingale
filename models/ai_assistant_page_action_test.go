package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPageActionsFilterAndKeepExplicitSkillReference(t *testing.T) {
	action := AssistantPageAction{
		Name:        "set_metric_query",
		Description: "Fill a PromQL expression.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"promql":{"type":"string"}},"required":["promql"]}`),
	}
	valid := ValidPageActions([]AssistantPageAction{action, {Name: "bad action", InputSchema: action.InputSchema}})
	if len(valid) != 1 || valid[0].Name != action.Name {
		t.Fatalf("ValidPageActions() = %#v", valid)
	}
	oversized := AssistantPageAction{Name: "oversized", Description: "too large", InputSchema: json.RawMessage(`{"type":"object","properties":{"padding":{"type":"string","description":"` + strings.Repeat("x", 8192) + `"}}}`)}
	if got := ValidPageActions([]AssistantPageAction{oversized}); len(got) != 0 {
		t.Fatalf("oversized schema = %#v", got)
	}
	if got := ValidPageActions([]AssistantPageAction{{Name: "执行查询", Description: "non-ascii", InputSchema: action.InputSchema}}); len(got) != 0 {
		t.Fatalf("non-ASCII action name = %#v", got)
	}

	query := AssistantMessageQuery{References: []AssistantMessageReference{
		{Type: "skill", Skill: AssistantSkillReference{Name: "explorer-query"}},
		{Type: "skill", Skill: AssistantSkillReference{Name: "explorer-query"}},
		{Type: "file", Name: "ignored"},
	}}
	got := query.ExplicitSkillNames()
	if len(got) != 1 || got[0] != "explorer-query" {
		t.Fatalf("ExplicitSkillNames() = %#v", got)
	}
}

func TestPageActionSchemaNormalization(t *testing.T) {
	valid := ValidPageActions([]AssistantPageAction{{
		Name:        "set_metric_query",
		Description: "Fill query.",
		InputSchema: json.RawMessage(`{
            "type":"object",
            "additionalProperties":false,
            "properties":{
                "query":{"type":"string","description":"PromQL"},
                "options":{"type":"object","properties":{"step":{"type":"integer"}},"required":["step"]},
                "matchers":{"type":"array","items":{"type":"string"}}
            },
            "required":["query"]
        }`),
	}})
	if len(valid) != 1 {
		t.Fatalf("valid actions = %#v", valid)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(valid[0].InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("normalized schema = %#v", schema)
	}

	invalidSchemas := []json.RawMessage{
		json.RawMessage(`{"type":"object","$defs":{}}`),
		json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","pattern":".*"}}}`),
		json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["unknown"]}`),
		json.RawMessage(`{"type":"object","additionalProperties":{"type":"string"}}`),
	}
	for _, inputSchema := range invalidSchemas {
		if got := ValidPageActions([]AssistantPageAction{{Name: "set_metric_query", Description: "Fill query.", InputSchema: inputSchema}}); len(got) != 0 {
			t.Fatalf("invalid schema %s retained: %#v", inputSchema, got)
		}
	}
}
