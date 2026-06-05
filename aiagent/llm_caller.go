package aiagent

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent/llm"
)

// buildLLMRequest 将 Agent 内部的 ChatMessage 转成 llm.GenerateRequest
// （含结构化工具轮字段的透传）
func buildLLMRequest(messages []ChatMessage) *llm.GenerateRequest {
	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{
			Role:           msg.Role,
			Content:        msg.Content,
			ToolCalls:      msg.ToolCalls,
			ToolCallID:     msg.ToolCallID,
			ToolName:       msg.ToolName,
			ThinkingBlocks: msg.ThinkingBlocks,
		}
	}
	return &llm.GenerateRequest{Messages: llmMessages}
}

func (a *Agent) checkLLMClient() error {
	if a.llmClient == nil {
		return fmt.Errorf("LLM client not initialized, use WithLLMClient or WithLLMConfig option")
	}
	return nil
}

// drainStream 消费完 provider 的 stream chan（直到被 provider 侧 close）。
// 用于调用方提前返回（比如遇到错误）后，让 provider 的 goroutine 得以 unblock
// 并正常退出，避免 goroutine 泄漏。
func drainStream(stream <-chan llm.StreamChunk) {
	for range stream {
	}
}
