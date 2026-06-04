package aiagent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

func TestIsWriteTool(t *testing.T) {
	for name, want := range map[string]bool{
		"create_alert_rule": true,
		"update_dashboard":  true,
		"import_dashboard":  true,
		"delete_panel":      true,
		"get_dashboard":     false,
		"list_dashboards":   false,
		"query_prometheus":  false,
		"load_skill":        false,
		"search_n9e_docs":   false,
		"updated_metrics":   false, // 前缀必须是 update_，updated_ 不算
	} {
		if got := isWriteTool(name); got != want {
			t.Fatalf("isWriteTool(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestTurnWriteDeduper(t *testing.T) {
	d := newTurnWriteDeduper()

	// 读类工具不参与
	d.record("get_dashboard_detail", `{"id":1}`, "r1")
	if _, hit := d.lookup("get_dashboard_detail", `{"id":1}`); hit {
		t.Fatal("read tools must not dedup")
	}

	// 写类工具：同名同参命中，参数不同不命中
	d.record("create_alert_rule", `{"name":"a"}`, "created-a")
	if out, hit := d.lookup("create_alert_rule", `{"name":"a"}`); !hit || out != "created-a" {
		t.Fatalf("identical write call must hit: %v %v", out, hit)
	}
	if _, hit := d.lookup("create_alert_rule", `{"name":"b"}`); hit {
		t.Fatal("different args must not hit (合法的'再建一条'参数必然不同)")
	}
	if _, hit := d.lookup("update_alert_rule", `{"name":"a"}`); hit {
		t.Fatal("different tool must not hit")
	}
}

// TestRunNativeLoop_WriteDedup: 一轮内模型用相同参数重复调用写工具，只真正执行一次。
func TestRunNativeLoop_WriteDedup(t *testing.T) {
	var calls int32
	RegisterBuiltinTool("create_test_counter", &BuiltinTool{
		Definition: AgentTool{Name: "create_test_counter", Type: ToolTypeBuiltin},
		Handler: func(ctx context.Context, deps *ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
			atomic.AddInt32(&calls, 1)
			return `{"created":true}`, nil
		},
	})

	fake := &scriptedNativeLLM{rounds: [][]llm.StreamChunk{
		{{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "create_test_counter", Arguments: `{"name":"x"}`}}}},
		{{ToolCalls: []llm.ToolCall{{ID: "c2", Name: "create_test_counter", Arguments: `{"name":"x"}`}}}}, // 同参重复
		{{Content: "done"}},
	}}
	a := &Agent{cfg: &AgentConfig{MaxIterations: 5, Timeout: 30000}, llmClient: fake}

	streamChan := make(chan *StreamChunk, 100)
	resp := a.runNativeLoop(context.Background(), &AgentRequest{}, []ChatMessage{{Role: "user", Content: "建两次"}}, nil, &ReActLoopConfig{
		MaxIterations:  5,
		StreamChan:     streamChan,
		EmitTranscript: true,
	})
	close(streamChan)

	if !resp.Success || resp.Content != "done" {
		t.Fatalf("resp = %+v", resp)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("write tool executed %d times, want exactly 1 (dedup)", got)
	}
	if len(resp.Steps) != 2 {
		t.Fatalf("steps = %d, want 2 (both calls observed by the model)", len(resp.Steps))
	}
	if !strings.Contains(resp.Steps[1].Observation, "未重复执行") {
		t.Fatalf("second observation must carry the dedup note: %q", resp.Steps[1].Observation)
	}
}
