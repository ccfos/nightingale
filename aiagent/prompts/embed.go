package prompts

import (
	_ "embed"
)

// ReAct 模式系统提示词
//
//go:embed react_system.md
var ReactSystemPrompt string

// 原生 function-calling 模式系统提示词：
// 与 ReactSystemPrompt 同源的身份/原则部分，但工具经原生 tools 参数下发，
// 不含 ReAct 三行文本协议的格式约束。
//
//go:embed native_system.md
var NativeSystemPrompt string

// 用户提示词默认模板
//
//go:embed user_default.md
var UserDefaultTemplate string

// Guided Follow-up 规则：要求最终答案末尾给 1~2 条接地的"下一步"建议。
// 同时被 ReAct / Direct 两条出答案路径引用，单一真源。
//
//go:embed guided_followup.md
var GuidedFollowupRule string
