package aiagent

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

func TestTranscriptEnvelopeRoundTrip(t *testing.T) {
	env := TranscriptEnvelope{
		SchemaVersion: TranscriptSchemaVersion,
		Messages: []ChatMessage{
			{Role: "user", Content: "q1"},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "update_dashboard", Arguments: `{"id":5}`}}},
			{Role: llm.RoleTool, ToolCallID: "c1", ToolName: "update_dashboard", Content: `{"proposal_id":"dbprop_x"}`},
			{Role: "assistant", Content: "done"},
		},
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back TranscriptEnvelope
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.SchemaVersion != TranscriptSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", back.SchemaVersion, TranscriptSchemaVersion)
	}
	if len(back.Messages) != len(env.Messages) {
		t.Fatalf("messages len = %d, want %d", len(back.Messages), len(env.Messages))
	}
	for i := range env.Messages {
		if !reflect.DeepEqual(back.Messages[i], env.Messages[i]) {
			t.Fatalf("messages[%d] = %+v, want %+v", i, back.Messages[i], env.Messages[i])
		}
	}
	// schema_version must actually be present in the JSON (forward-evolution hook).
	if !strings.Contains(string(b), "\"schema_version\"") {
		t.Fatalf("serialized envelope missing schema_version: %s", b)
	}
}
