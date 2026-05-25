package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextDump_Empty(t *testing.T) {
	if got := ContextDump(nil); got != "" {
		t.Errorf("nil context should render empty, got %q", got)
	}
	if got := ContextDump(map[string]interface{}{}); got != "" {
		t.Errorf("empty context should render empty, got %q", got)
	}
}

func TestContextDump_StableOrder(t *testing.T) {
	// Same content, different insertion order — output must be identical so
	// the LLM provider's prompt cache stays warm across requests.
	a := ContextDump(map[string]interface{}{"event_id": 7269, "busi_group_id": 12, "rule_id": 99})
	b := ContextDump(map[string]interface{}{"rule_id": 99, "event_id": 7269, "busi_group_id": 12})
	if a != b {
		t.Errorf("dump must be order-independent\n a=%q\n b=%q", a, b)
	}
	// Spot check ordering: busi_group_id < event_id < rule_id alphabetically.
	idxBG := strings.Index(a, "busi_group_id")
	idxEv := strings.Index(a, "event_id")
	idxRu := strings.Index(a, "rule_id")
	if !(idxBG < idxEv && idxEv < idxRu) {
		t.Errorf("keys not alphabetically ordered: %s", a)
	}
}

func TestContextDump_SensitiveKeysFiltered(t *testing.T) {
	out := ContextDump(map[string]interface{}{
		"event_id":      7269,
		"password":      "p@ss",
		"API_KEY":       "k",
		"user_token":    "t",
		"my_secret_val": "s",
		"PrivateKey":    "x",
	})
	for _, banned := range []string{"password", "p@ss", "API_KEY", "user_token", "my_secret_val", "PrivateKey"} {
		if strings.Contains(out, banned) {
			t.Errorf("sensitive token %q leaked into dump: %s", banned, out)
		}
	}
	if !strings.Contains(out, "event_id=7269") {
		t.Errorf("non-sensitive key dropped: %s", out)
	}
}

func TestContextDump_AllSensitiveYieldsEmpty(t *testing.T) {
	// If every key is filtered out, the whole dump should be empty — no
	// dangling "Front-end context:" header with no body.
	out := ContextDump(map[string]interface{}{"password": "x", "TOKEN": "y"})
	if out != "" {
		t.Errorf("dump with only sensitive keys should be empty, got %q", out)
	}
}

func TestContextDump_ValueTruncation(t *testing.T) {
	long := strings.Repeat("a", 500)
	out := ContextDump(map[string]interface{}{"blob": long})
	if !strings.Contains(out, "...(truncated)") {
		t.Errorf("oversized value not truncated: %s", out)
	}
	if strings.Count(out, "a") > 250 {
		t.Errorf("truncation didn't cap length, got %d a's", strings.Count(out, "a"))
	}
}

func TestContextDump_KeyCap(t *testing.T) {
	m := make(map[string]interface{}, 25)
	for i := 0; i < 25; i++ {
		// Two-digit prefix keeps lexical order matching insertion intent.
		m[strings.Repeat("z", 2)+string(rune('0'+i/10))+string(rune('0'+i%10))] = i
	}
	out := ContextDump(m)
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation note for >16 keys, got %s", out)
	}
}

func TestContextDump_ComplexValues(t *testing.T) {
	out := ContextDump(map[string]interface{}{
		"team_ids":      []int64{1, 2, 3},
		"datasource_id": json.Number("42"),
		"is_active":     true,
	})
	if !strings.Contains(out, "team_ids=[1,2,3]") {
		t.Errorf("slice not JSON-rendered: %s", out)
	}
	if !strings.Contains(out, "datasource_id=42") {
		t.Errorf("json.Number not rendered: %s", out)
	}
	if !strings.Contains(out, "is_active=true") {
		t.Errorf("bool not rendered: %s", out)
	}
}
