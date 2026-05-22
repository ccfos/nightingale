package llm

import (
	"reflect"
	"testing"
)

func TestNormalizeThinkingParams(t *testing.T) {
	type args struct {
		provider string
		baseURL  string
		model    string
		extra    map[string]any
	}
	tests := []struct {
		name string
		args args
		want map[string]any
	}{
		// ── BaseURL 路由（最高优先级）──
		{
			name: "Bailian DeepSeek 走 enable_thinking（平台规则胜过模型名）",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				model:    "deepseek-v3.1",
			},
			want: map[string]any{"enable_thinking": false},
		},
		{
			name: "Bailian qwen-plus",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				model:    "qwen-plus",
			},
			want: map[string]any{"enable_thinking": false},
		},
		{
			name: "Bailian 国际版",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
				model:    "qwen3-235b-a22b",
			},
			want: map[string]any{"enable_thinking": false},
		},
		{
			name: "Ark Doubao 走 thinking.type",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
				model:    "doubao-seed-1.6-250615",
			},
			want: map[string]any{"thinking": map[string]any{"type": "disabled"}},
		},
		{
			name: "Ark Kimi（同平台规则）",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
				model:    "kimi-k2",
			},
			want: map[string]any{"thinking": map[string]any{"type": "disabled"}},
		},
		{
			name: "Ark 国际版 ByteDance Plus",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://ark.ap-southeast.bytepluses.com/api/v3",
				model:    "doubao-seed-1.6",
			},
			want: map[string]any{"thinking": map[string]any{"type": "disabled"}},
		},
		{
			name: "硅基流动",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.siliconflow.cn/v1/chat/completions",
				model:    "Qwen/Qwen3-32B",
			},
			want: map[string]any{"enable_thinking": false},
		},

		// ── 用户已显式设置时跳过（最高优先级的"放行"）──
		{
			name: "用户已显式 enable_thinking=true 时不覆盖",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				model:    "qwen-plus",
				extra:    map[string]any{"enable_thinking": true},
			},
			want: map[string]any{"enable_thinking": true},
		},
		{
			name: "用户已显式 thinking.type=enabled 时不覆盖",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://ark.cn-beijing.volces.com/api/v3",
				model:    "doubao-seed-1.6",
				extra:    map[string]any{"thinking": map[string]any{"type": "enabled"}},
			},
			want: map[string]any{"thinking": map[string]any{"type": "enabled"}},
		},
		{
			name: "用户填了其它 extra 字段时保留并叠加",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				model:    "qwen-plus",
				extra:    map[string]any{"foo": "bar"},
			},
			want: map[string]any{"foo": "bar", "enable_thinking": false},
		},

		// ── 纯思考模型黑名单（关不掉，跳过）──
		{
			name: "Bailian 上的 qwen3-thinking-2507 即便平台支持也不注入",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				model:    "qwen3-235b-a22b-thinking-2507",
			},
			want: map[string]any{},
		},
		{
			name: "deepseek-r1 关不掉",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				model:    "deepseek-r1",
			},
			want: map[string]any{},
		},
		{
			name: "qwq-32b 关不掉",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				model:    "qwq-32b",
			},
			want: map[string]any{},
		},
		{
			name: "kimi-k2-thinking 关不掉",
			args: args{
				provider: ProviderClaude,
				baseURL:  "https://api.moonshot.cn/v1",
				model:    "kimi-k2-thinking",
			},
			want: map[string]any{},
		},
		{
			name: "OpenAI o1 关不掉",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.openai.com/v1",
				model:    "o1-mini",
			},
			want: map[string]any{},
		},
		{
			name: "Gemini 2.5 Pro 关不掉",
			args: args{
				provider: ProviderGemini,
				baseURL:  "",
				model:    "gemini-2.5-pro",
			},
			want: map[string]any{},
		},

		// ── Provider 路由：Gemini ──
		{
			name: "Gemini 2.5 Flash 走 thinking_config.thinking_budget=0",
			args: args{
				provider: ProviderGemini,
				baseURL:  "",
				model:    "gemini-2.5-flash",
			},
			want: map[string]any{
				"thinking_config": map[string]any{"thinking_budget": 0},
			},
		},
		{
			name: "Gemini 2.5 Flash Lite 默认不思考，不注入",
			args: args{
				provider: ProviderGemini,
				baseURL:  "",
				model:    "gemini-2.5-flash-lite",
			},
			want: map[string]any{},
		},

		// ── Provider 路由：OpenAI 思考模型 ──
		{
			name: "GPT-5 用 reasoning_effort=minimal",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.openai.com/v1",
				model:    "gpt-5",
			},
			want: map[string]any{"reasoning_effort": "minimal"},
		},
		{
			name: "GPT-5.1 用 reasoning_effort=none",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.openai.com/v1",
				model:    "gpt-5.1",
			},
			want: map[string]any{"reasoning_effort": "none"},
		},
		{
			name: "o3 用 reasoning_effort=low",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.openai.com/v1",
				model:    "o3-mini",
			},
			want: map[string]any{"reasoning_effort": "low"},
		},

		// ── 模型名兜底（用户直连原厂 endpoint）──
		{
			name: "直连 deepseek.com 的 deepseek-v4-pro",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.deepseek.com/v1",
				model:    "deepseek-v4-pro",
			},
			want: map[string]any{"thinking": map[string]any{"type": "disabled"}},
		},
		{
			name: "直连智谱 glm-4.6",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://open.bigmodel.cn/api/paas/v4",
				model:    "glm-4.6",
			},
			want: map[string]any{"thinking": map[string]any{"type": "disabled"}},
		},
		{
			name: "直连 moonshot 的 kimi-k2.5",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.moonshot.cn/v1",
				model:    "kimi-k2.5",
			},
			want: map[string]any{"thinking": map[string]any{"type": "disabled"}},
		},
		{
			name: "直连 moonshot 国际版的 kimi-k2.6",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.moonshot.ai/v1",
				model:    "kimi-k2.6",
			},
			want: map[string]any{"thinking": map[string]any{"type": "disabled"}},
		},
		{
			name: "kimi-k2-thinking-turbo 关不掉（黑名单覆盖 -turbo 后缀）",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.moonshot.cn/v1",
				model:    "kimi-k2-thinking-turbo",
			},
			want: map[string]any{},
		},
		{
			name: "moonshot-v1-128k 不带 thinking 功能，不注入",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.moonshot.cn/v1",
				model:    "moonshot-v1-128k",
			},
			want: map[string]any{},
		},
		{
			name: "kimi-k2-0905-preview 不带 thinking 功能，不注入",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.moonshot.cn/v1",
				model:    "kimi-k2-0905-preview",
			},
			want: map[string]any{},
		},
		{
			name: "kimi-k2-turbo-preview 不带 thinking 功能，不注入",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://api.moonshot.cn/v1",
				model:    "kimi-k2-turbo-preview",
			},
			want: map[string]any{},
		},

		// ── 未知模型不注入（兜底安全）──
		{
			name: "未知模型不注入",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "https://example.com/v1",
				model:    "some-unknown-model",
			},
			want: map[string]any{},
		},
		{
			name: "Claude provider 默认不注入（默认就关）",
			args: args{
				provider: ProviderClaude,
				baseURL:  "https://api.anthropic.com",
				model:    "claude-sonnet-4-5",
			},
			want: map[string]any{},
		},

		// ── 不污染入参 ──
		{
			name: "入参 nil 也能返回非 nil 空 map",
			args: args{
				provider: ProviderOpenAI,
				baseURL:  "",
				model:    "",
				extra:    nil,
			},
			want: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeThinkingParams(tt.args.provider, tt.args.baseURL, tt.args.model, tt.args.extra)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NormalizeThinkingParams() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestNormalizeThinkingParams_DoesNotMutateInput 防止后续重构破坏"不污染入参"语义
func TestNormalizeThinkingParams_DoesNotMutateInput(t *testing.T) {
	in := map[string]any{"foo": "bar"}
	_ = NormalizeThinkingParams(ProviderOpenAI, "https://dashscope.aliyuncs.com/v1", "qwen-plus", in)
	if _, leaked := in["enable_thinking"]; leaked {
		t.Fatalf("input map was mutated: %#v", in)
	}
}
