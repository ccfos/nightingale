package prompts

import (
	_ "embed"
)

// ReAct 模式系统提示词
//
//go:embed react_system.md
var ReactSystemPrompt string

// Plan+ReAct 模式规划阶段系统提示词
//
//go:embed plan_system.md
var PlanSystemPrompt string

// 步骤执行提示词
//
//go:embed step_execution.md
var StepExecutionPrompt string

// 综合分析提示词
//
//go:embed synthesis.md
var SynthesisPrompt string

// 用户提示词默认模板
//
//go:embed user_default.md
var UserDefaultTemplate string
