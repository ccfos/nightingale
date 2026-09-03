package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGemini_ConvertRequest_ThinkingBudgetZero 确认 ExtraBody.thinking_config.thinking_budget=0
// 真的被翻译到 generationConfig.thinkingConfig.thinkingBudget=0。
// 关键 case：omitempty 不能把 0 吃掉。
func TestGemini_ConvertRequest_ThinkingBudgetZero(t *testing.T) {
	g := &Gemini{
		config: &Config{
			Model: "gemini-2.5-flash",
			ExtraBody: map[string]any{
				"thinking_config": map[string]any{
					"thinking_budget": 0,
				},
			},
		},
	}
	req := g.convertRequest(&GenerateRequest{})
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"thinkingBudget":0`) {
		t.Errorf("thinkingBudget:0 missing from body: %s", s)
	}
}

// TestGemini_ConvertRequest_ThinkingLevel 覆盖 Gemini 3 路径的 thinkingLevel:"minimal"
func TestGemini_ConvertRequest_ThinkingLevel(t *testing.T) {
	g := &Gemini{
		config: &Config{
			Model: "gemini-3-pro",
			ExtraBody: map[string]any{
				"thinking_config": map[string]any{
					"thinking_level": "minimal",
				},
			},
		},
	}
	req := g.convertRequest(&GenerateRequest{})
	data, _ := json.Marshal(req)
	if !strings.Contains(string(data), `"thinkingLevel":"minimal"`) {
		t.Errorf("thinkingLevel missing: %s", data)
	}
}

// TestGemini_ConvertRequest_NoExtraBody 不带 thinking_config 时 body 里也不该出现 thinkingConfig
func TestGemini_ConvertRequest_NoExtraBody(t *testing.T) {
	g := &Gemini{
		config: &Config{Model: "gemini-2.5-flash"},
	}
	req := g.convertRequest(&GenerateRequest{})
	data, _ := json.Marshal(req)
	if strings.Contains(string(data), "thinkingConfig") {
		t.Errorf("thinkingConfig leaked when not configured: %s", data)
	}
}

// TestGemini_ConvertRequest_Float64Budget 模拟用户从 JSON 配置流入 float64 的情形
func TestGemini_ConvertRequest_Float64Budget(t *testing.T) {
	g := &Gemini{
		config: &Config{
			Model: "gemini-2.5-flash",
			ExtraBody: map[string]any{
				"thinking_config": map[string]any{
					"thinking_budget": float64(0),
				},
			},
		},
	}
	req := g.convertRequest(&GenerateRequest{})
	data, _ := json.Marshal(req)
	if !strings.Contains(string(data), `"thinkingBudget":0`) {
		t.Errorf("float64(0) not coerced to int 0: %s", data)
	}
}

// TestGemini_ConvertRequest_CamelCaseKeys 用户照 Gemini 官方文档拷的 camelCase 写法也要识别
func TestGemini_ConvertRequest_CamelCaseKeys(t *testing.T) {
	g := &Gemini{
		config: &Config{
			Model: "gemini-2.5-flash",
			ExtraBody: map[string]any{
				"thinkingConfig": map[string]any{
					"thinkingBudget": 0,
				},
			},
		},
	}
	req := g.convertRequest(&GenerateRequest{})
	data, _ := json.Marshal(req)
	if !strings.Contains(string(data), `"thinkingBudget":0`) {
		t.Errorf("camelCase thinkingConfig was ignored: %s", data)
	}
}

// TestGemini_ConvertRequest_CamelCaseLevel 同上，覆盖 thinkingLevel
func TestGemini_ConvertRequest_CamelCaseLevel(t *testing.T) {
	g := &Gemini{
		config: &Config{
			Model: "gemini-3-pro",
			ExtraBody: map[string]any{
				"thinkingConfig": map[string]any{
					"thinkingLevel": "minimal",
				},
			},
		},
	}
	req := g.convertRequest(&GenerateRequest{})
	data, _ := json.Marshal(req)
	if !strings.Contains(string(data), `"thinkingLevel":"minimal"`) {
		t.Errorf("camelCase thinkingLevel was ignored: %s", data)
	}
}

// TestGemini_ConvertRequest_UnusedKeysDontBreakThinking 用户在 ExtraBody 里塞了其它字段时，
// thinking_config 的注入不应受影响（其它字段会被 debug 日志提示但不影响主流程）。
func TestGemini_ConvertRequest_UnusedKeysDontBreakThinking(t *testing.T) {
	g := &Gemini{
		config: &Config{
			Model: "gemini-2.5-flash",
			ExtraBody: map[string]any{
				"thinking_config":  map[string]any{"thinking_budget": 0},
				"some_other_field": "value", // Gemini 不消费，应被忽略
			},
		},
	}
	req := g.convertRequest(&GenerateRequest{})
	data, _ := json.Marshal(req)
	if !strings.Contains(string(data), `"thinkingBudget":0`) {
		t.Errorf("unused ExtraBody key broke thinking injection: %s", data)
	}
	if strings.Contains(string(data), "some_other_field") {
		t.Errorf("unused key leaked into request body: %s", data)
	}
}
