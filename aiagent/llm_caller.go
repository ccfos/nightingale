package aiagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llm"

	"github.com/toolkits/pkg/logger"
)

// callLLM 调用 LLM（非流式）。stop 为可选的停止序列，非 ReAct 场景传 nil。
func (a *Agent) callLLM(ctx context.Context, messages []ChatMessage, stop []string) (string, error) {
	if a.llmClient == nil {
		return "", fmt.Errorf("LLM client not initialized, use WithLLMClient option, agent: %v", a)
	}

	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	req := &llm.GenerateRequest{
		Messages: llmMessages,
		Stop:     stop,
	}

	resp, err := a.llmClient.Generate(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM generate error: %w", err)
	}

	return resp.Content, nil
}

// callLLMWithStreamOutput 调用 LLM 并将流式输出转发到 streamChan。stop 为可选的停止序列。
func (a *Agent) callLLMWithStreamOutput(ctx context.Context, messages []ChatMessage, streamChan chan *StreamChunk, requestID string, stop []string) (string, error) {
	if a.llmClient == nil {
		return "", fmt.Errorf("LLM client not initialized, use WithLLMClient option, agent: %v", a)
	}
	logger.Infof("[Agent] LLM client ready, provider=%s", a.llmClient.Name())

	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
		logger.Infof("[Agent] Message[%d]: role=%s, content_len=%d", i, msg.Role, len(msg.Content))
	}

	logger.Infof("[Agent] Calling GenerateStream with %d messages", len(llmMessages))
	streamStart := time.Now()
	streamReq := &llm.GenerateRequest{
		Messages: llmMessages,
		Stop:     stop,
	}

	stream, err := a.llmClient.GenerateStream(ctx, streamReq)
	if err != nil {
		logger.Errorf("[Agent] GenerateStream failed after %v: %v", time.Since(streamStart), err)
		return "", fmt.Errorf("LLM stream error: %w", err)
	}
	logger.Infof("[Agent] GenerateStream returned stream channel after %v", time.Since(streamStart))

	var fullContent strings.Builder
	chunkCount := 0

	for chunk := range stream {
		chunkCount++
		if chunk.Error != nil {
			logger.Errorf("[Agent] Stream error at chunk #%d: %v, accumulated_content_len=%d",
				chunkCount, chunk.Error, fullContent.Len())
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
			logger.Infof("[Agent] Stream done at chunk #%d", chunkCount)
			break
		}
	}

	logger.Infof("[Agent] Stream completed: total_chunks=%d, total_content_len=%d, elapsed=%v",
		chunkCount, fullContent.Len(), time.Since(streamStart))

	return fullContent.String(), nil
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
