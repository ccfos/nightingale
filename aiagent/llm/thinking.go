package llm

import "strings"

// NormalizeThinkingParams 在 extra 上叠加"关闭深度思考"的厂商特定字段，返回新 map。
//
// 当前仅用于连接测试 probe 路径（aiagent/llmconfig）：探测请求 MaxTokens=5，
// 思考模型会把 token 全烧在 reasoning 上导致 content 为空、误报失败，所以探测前
// 按 BaseURL > Provider > Model 三层路由自动注入"关思考"字段。chat 路径不再注入
// ——native 下思考是一等公民，react 下 reasoning 走独立通道不影响协议解析，
// 想关思考的用户在 CustomParams 里显式配置。
//
// 注入规则（优先级从高到低）：
//  1. 用户在 extra 里已显式设置过任何 thinking 控制字段 → 原样返回，不覆盖用户意图
//  2. 模型本身就是"纯思考模型"（关不掉）→ 不注入，避免 400 错误
//  3. BaseURL 命中托管平台（阿里百炼 / 火山方舟 / 硅基流动）→ 用平台统一参数
//     这一步比 model 名匹配更可靠，因为同一个模型在不同平台开关字段不一样
//     （DeepSeek 在百炼用 enable_thinking，在火山方舟用 thinking.type）
//  4. Provider 命中 gemini / openai-o系列 → 用 provider 专属字段
//  5. 模型名前缀匹配（用户直连原厂 endpoint 的情况）
//  6. 兜底：什么也不做
func NormalizeThinkingParams(provider, baseURL, model string, extra map[string]any) map[string]any {
	out := make(map[string]any, len(extra)+1)
	for k, v := range extra {
		out[k] = v
	}

	if userHasThinkingControl(out) {
		return out
	}
	if isPureThinkingModel(model) {
		return out
	}

	var inj map[string]any
	switch {
	case pickByBaseURL(baseURL) != nil:
		inj = pickByBaseURL(baseURL)
	case pickByProvider(provider, model) != nil:
		inj = pickByProvider(provider, model)
	default:
		inj = pickByModel(model)
	}

	for k, v := range inj {
		out[k] = v
	}
	return out
}

// thinkingControlKeys 列出"用户已经在掌舵 thinking"的标记字段。
// 命中任意一个就跳过自动注入，避免覆盖用户意图。
// 注意：这里同时收了 camelCase 和 snake_case 两种写法，因为 Gemini 用 camelCase。
var thinkingControlKeys = []string{
	"enable_thinking",
	"thinking",
	"thinking_config",
	"thinkingConfig",
	"reasoning_effort",
	"chat_template_kwargs", // vLLM 自部署 Qwen3 走这个
	"output_config",        // DeepSeek Anthropic 兼容路径
}

func userHasThinkingControl(m map[string]any) bool {
	for _, k := range thinkingControlKeys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

// isPureThinkingModel 标记官方"无法关闭思考"的模型。
// 命中这些模型时直接放行，因为传 disable 参数会被服务端拒绝（400）。
//
// 名单来源（已经查过官方文档）：
//   - 阿里百炼"深度思考"页：qwq-*、qwen3-*-thinking-2507、deepseek-r1
//   - 火山方舟"深度思考"页：doubao-1.5-thinking-pro（纯思考版）
//   - Kimi 平台：kimi-k2-thinking
//   - OpenAI：o1 / o1-mini / o1-preview（不支持 reasoning_effort）
//   - Gemini 2.5 Pro：N/A: Cannot disable thinking（官方原文）
func isPureThinkingModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	switch {
	case strings.HasPrefix(m, "qwq-"), m == "qwq",
		strings.HasPrefix(m, "qvq-"),
		strings.HasPrefix(m, "deepseek-r1"),
		strings.Contains(m, "-thinking-2507"),
		strings.HasSuffix(m, "-thinking"),
		strings.HasPrefix(m, "doubao-1.5-thinking-pro"),
		strings.HasPrefix(m, "kimi-k2-thinking"),
		m == "gemini-2.5-pro" || strings.HasPrefix(m, "gemini-2.5-pro-"),
		// o1 系列：注意要把 o1-mini / o1-preview 都覆盖，但不能误伤 o3、gpt-5-o3 这种
		m == "o1", strings.HasPrefix(m, "o1-"):
		return true
	}
	return false
}

// pickByBaseURL 按托管平台路由。这是最可靠的一层 ——
// 同一个底模在不同平台往往用不同字段，平台决定的优先级要高于模型名。
func pickByBaseURL(baseURL string) map[string]any {
	u := strings.ToLower(baseURL)
	if u == "" {
		return nil
	}
	switch {
	case strings.Contains(u, "dashscope.aliyuncs.com"),
		strings.Contains(u, "dashscope-intl.aliyuncs.com"),
		strings.Contains(u, "api.siliconflow.cn"),
		strings.Contains(u, "api.siliconflow.com"):
		return map[string]any{"enable_thinking": false}
	case strings.Contains(u, "ark.cn-beijing.volces.com"),
		strings.Contains(u, "ark.ap-southeast.bytepluses.com"),
		strings.Contains(u, "ark-cn-beijing.bytedance.net"):
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	return nil
}

// pickByProvider 处理走专用 provider 接口的厂商（不走 OpenAI 兼容）。
// 注意：这里返回的 map 仍然是按 extra_body 风格平铺，由各 provider 的
// MarshalJSON / convertRequest 决定怎么落到具体请求结构里。
func pickByProvider(provider, model string) map[string]any {
	p := strings.ToLower(provider)
	m := strings.ToLower(model)

	switch p {
	case ProviderGemini:
		// 只对 2.5 Flash 系列注入；2.5 Pro 在 isPureThinkingModel 已拦截
		if strings.HasPrefix(m, "gemini-2.5-flash") && !strings.Contains(m, "lite") {
			return map[string]any{
				"thinking_config": map[string]any{"thinking_budget": 0},
			}
		}
		// gemini-3-* 用 thinkingLevel: minimal
		if strings.HasPrefix(m, "gemini-3") {
			return map[string]any{
				"thinking_config": map[string]any{"thinking_level": "minimal"},
			}
		}
		return nil
	case ProviderClaude:
		// Anthropic Claude 默认不开思考，不需要注入。
		// 走 Claude provider 的 Kimi（Anthropic 兼容路径）由 pickByModel 的模型名兜底
		// 处理 —— 注意不能给 api.moonshot.cn / api.moonshot.ai 加平台白名单，
		// 因为 Kimi 平台上 moonshot-v1-* 和 kimi-k2-*-preview 这些老/普通模型
		// 不带 thinking 功能，平台级注入会给它们塞不认的字段。
		return nil
	case ProviderOpenAI, "":
		// 走 OpenAI 兼容路径的"原生"思考型号
		if strings.HasPrefix(m, "gpt-5.1") || strings.HasPrefix(m, "gpt-5-1") {
			return map[string]any{"reasoning_effort": "none"}
		}
		if strings.HasPrefix(m, "gpt-5") {
			return map[string]any{"reasoning_effort": "minimal"}
		}
		if strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") {
			return map[string]any{"reasoning_effort": "low"}
		}
		return nil
	}
	return nil
}

// pickByModel 是模型名兜底，用于用户直连原厂 endpoint 的场景
// （这种情况下 baseURL 不命中托管平台白名单）。
func pickByModel(model string) map[string]any {
	m := strings.ToLower(model)
	if m == "" {
		return nil
	}

	// ── Qwen3 hybrid（官方 dashscope 之外的部署，如自建 vLLM）──
	if strings.HasPrefix(m, "qwen3") ||
		strings.HasPrefix(m, "qwen-plus") ||
		strings.HasPrefix(m, "qwen-flash") ||
		strings.HasPrefix(m, "qwen-turbo") ||
		strings.HasPrefix(m, "qwen-max") {
		return map[string]any{"enable_thinking": false}
	}
	// ── 智谱 GLM-4.5+ ──
	if strings.HasPrefix(m, "glm-4.5") ||
		strings.HasPrefix(m, "glm-4.6") ||
		strings.HasPrefix(m, "glm-5") {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	// ── 豆包 hybrid（用户直连 ark 之外的入口；走到这里说明 baseURL 没匹配）──
	if strings.HasPrefix(m, "doubao-seed") || strings.HasPrefix(m, "doubao-1.6") {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	// ── DeepSeek V4 hybrid ──
	if strings.HasPrefix(m, "deepseek-v4") {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	// ── Kimi K2.5 / K2.6 hybrid ──
	// 只对带版本号小数的 K2.5/K2.6 注入；moonshot-v1-*、kimi-k2-0905-preview、
	// kimi-k2-0711-preview、kimi-k2-turbo-preview 这些不带 thinking 的模型保持不注入。
	// kimi-k2-thinking / kimi-k2-thinking-turbo 已在 isPureThinkingModel 拦截。
	if strings.HasPrefix(m, "kimi-k2.5") || strings.HasPrefix(m, "kimi-k2.6") {
		return map[string]any{"thinking": map[string]any{"type": "disabled"}}
	}
	return nil
}
