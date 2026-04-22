package aiagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llm"

	"github.com/toolkits/pkg/logger"
)

// buildLLMRequest 将 Agent 内部的 ChatMessage 转成 llm.GenerateRequest
func buildLLMRequest(messages []ChatMessage, stop []string) *llm.GenerateRequest {
	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{Role: msg.Role, Content: msg.Content}
	}
	return &llm.GenerateRequest{Messages: llmMessages, Stop: stop}
}

func (a *Agent) checkLLMClient() error {
	if a.llmClient == nil {
		return fmt.Errorf("LLM client not initialized, use WithLLMClient or WithLLMConfig option")
	}
	return nil
}

// callLLM 调用 LLM（非流式）。stop 为可选的停止序列，非 ReAct 场景传 nil。
func (a *Agent) callLLM(ctx context.Context, messages []ChatMessage, stop []string) (string, error) {
	if err := a.checkLLMClient(); err != nil {
		return "", err
	}
	resp, err := a.llmClient.Generate(ctx, buildLLMRequest(messages, stop))
	if err != nil {
		return "", fmt.Errorf("LLM generate error: %w", err)
	}
	return resp.Content, nil
}

// callLLMWithStreamOutput 调用 LLM 并将流式输出转发到 streamChan。stop 为可选的停止序列。
func (a *Agent) callLLMWithStreamOutput(ctx context.Context, messages []ChatMessage, streamChan chan *StreamChunk, requestID string, stop []string) (string, error) {
	if err := a.checkLLMClient(); err != nil {
		return "", err
	}

	stream, err := a.llmClient.GenerateStream(ctx, buildLLMRequest(messages, stop))
	if err != nil {
		logger.Errorf("[Agent] GenerateStream failed provider=%s: %v", a.llmClient.Name(), err)
		return "", fmt.Errorf("LLM stream error: %w", err)
	}

	var fullContent strings.Builder
	for chunk := range stream {
		if chunk.Error != nil {
			logger.Errorf("[Agent] stream chunk error provider=%s content_len=%d: %v",
				a.llmClient.Name(), fullContent.Len(), chunk.Error)
			// 早退前排干 provider 的 ch：否则 provider 那边的 streamResponse goroutine
			// 还在往 100-buffer 里 blocking send，buffer 满就永久卡死（直到 HTTP ctx
			// 取消 + 最后一个 error chunk 也写进去为止——但那个最后写同样是 blocking）。
			go drainStream(stream)
			return fullContent.String(), fmt.Errorf("stream error: %w", chunk.Error)
		}
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			streamChan <- &StreamChunk{
				Type:      StreamTypeText,
				Delta:     chunk.Content,
				Content:   fullContent.String(),
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}
		if chunk.Done {
			break
		}
	}
	return fullContent.String(), nil
}

// drainStream 消费完 provider 的 stream chan（直到被 provider 侧 close）。
// 用于调用方提前返回（比如遇到错误）后，让 provider 的 goroutine 得以 unblock
// 并正常退出，避免 goroutine 泄漏。
func drainStream(stream <-chan llm.StreamChunk) {
	for range stream {
	}
}

// callLLMAuto 统一的 LLM 调用（自动选择流式/非流式）。stop 为可选的停止序列，
// ReAct 循环内需要传 []string{"Observation:"} 让模型在该前缀处停下等工具真实结果；
// 计划生成/综合等一次性 LLM 调用传 nil。
func (a *Agent) callLLMAuto(ctx context.Context, messages []ChatMessage, streamChan chan *StreamChunk, requestID string, stop []string) (string, error) {
	if streamChan != nil {
		return a.callLLMWithStreamOutput(ctx, messages, streamChan, requestID, stop)
	}
	return a.callLLM(ctx, messages, stop)
}
