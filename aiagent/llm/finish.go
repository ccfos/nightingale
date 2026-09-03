package llm

import "strings"

// FinishKind 把各 provider 的结束原因归一为三类，消费方据此区分正常收尾、
// 截断提示与硬失败。未知取值一律按 normal 处理——宁可放过，不因 provider
// 新增枚举值误杀正常回答。
type FinishKind int

const (
	FinishNormal FinishKind = iota
	// FinishTruncated 输出达到长度上限被截断：正文是半截但仍有价值。
	FinishTruncated
	// FinishBlocked 被安全策略/内容过滤拦截：正文不可信或为空。
	FinishBlocked
)

// ClassifyFinish 归一 OpenAI(finish_reason)/Claude(stop_reason)/
// Gemini(finishReason) 的取值，大小写不敏感（Gemini 全大写）。
func ClassifyFinish(reason string) FinishKind {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens", "model_length":
		return FinishTruncated
	case "content_filter", "refusal", "safety", "recitation",
		"prohibited_content", "blocklist", "spii", "image_safety":
		return FinishBlocked
	}
	return FinishNormal
}
