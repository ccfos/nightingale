package prompts

import (
	_ "embed"
)

// 工具循环模式系统提示词（身份/原则部分；工具经原生 tools 参数下发）
//
//go:embed native_system.md
var NativeSystemPrompt string

// Guided Follow-up 规则：要求最终答案末尾给 1~2 条接地的"下一步"建议。
// 同时被工具循环 / Direct 两条出答案路径引用，单一真源。
//
//go:embed guided_followup.md
var GuidedFollowupRule string
