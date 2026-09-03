package llm

import "testing"

func TestClassifyFinish(t *testing.T) {
	cases := []struct {
		reason string
		want   FinishKind
	}{
		// 正常收尾（含工具调用、空值、未知枚举）
		{"stop", FinishNormal},
		{"tool_calls", FinishNormal},
		{"end_turn", FinishNormal},
		{"tool_use", FinishNormal},
		{"STOP", FinishNormal},
		{"", FinishNormal},
		{"some_future_reason", FinishNormal},
		// 截断（OpenAI/Claude/Gemini）
		{"length", FinishTruncated},
		{"max_tokens", FinishTruncated},
		{"MAX_TOKENS", FinishTruncated},
		// 拦截
		{"content_filter", FinishBlocked},
		{"refusal", FinishBlocked},
		{"SAFETY", FinishBlocked},
		{"RECITATION", FinishBlocked},
		{"PROHIBITED_CONTENT", FinishBlocked},
	}
	for _, c := range cases {
		if got := ClassifyFinish(c.reason); got != c.want {
			t.Errorf("ClassifyFinish(%q) = %v, want %v", c.reason, got, c.want)
		}
	}
}
