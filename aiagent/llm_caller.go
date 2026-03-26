package aiagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llm"

	"github.com/toolkits/pkg/logger"
)

// initLLMClient 初始化 LLM 客户端
func (a *Agent) initLLMClient() error {
	if a.llmClient != nil {
		return nil
	}

	provider := a.cfg.Provider
	if provider == "" {
		if a.cfg.LLMURL != "" {
			provider = llm.DetectProvider(a.cfg.LLMURL)
		} else if a.cfg.Model != "" {
			provider = llm.DetectProviderFromModel(a.cfg.Model)
		} else {
			provider = llm.ProviderOpenAI
		}
	}

	cfg := &llm.Config{
		Provider:      provider,
		BaseURL:       a.cfg.LLMURL,
		APIKey:        a.cfg.APIKey,
		Model:         a.cfg.Model,
		Headers:       a.cfg.Headers,
		Timeout:       a.cfg.Timeout,
		SkipSSLVerify: a.cfg.SkipSSLVerify,
		Proxy:         a.cfg.Proxy,
	}

	client, err := llm.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	a.llmClient = client
	logger.Infof("AI Agent LLM client initialized: provider=%s, model=%s", provider, a.cfg.Model)
	return nil
}

// callLLM 调用 LLM（非流式）
func (a *Agent) callLLM(ctx context.Context, messages []ChatMessage) (string, error) {
	if err := a.initLLMClient(); err != nil {
		return "", err
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
	}
	if a.cfg.Temperature != nil {
		req.Temperature = *a.cfg.Temperature
	}
	if a.cfg.MaxTokens != nil {
		req.MaxTokens = *a.cfg.MaxTokens
	}

	resp, err := a.llmClient.Generate(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM generate error: %w", err)
	}

	return resp.Content, nil
}

// callLLMWithStreamOutput 调用 LLM 并将流式输出转发到 streamChan
func (a *Agent) callLLMWithStreamOutput(ctx context.Context, messages []ChatMessage, streamChan chan *StreamChunk, requestID string) (string, error) {
	if err := a.initLLMClient(); err != nil {
		logger.Errorf("[Agent] Failed to init LLM client: %v", err)
		return "", err
	}
	logger.Infof("[Agent] LLM client ready, provider=%s, model=%s", a.cfg.Provider, a.cfg.Model)

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
	}
	if a.cfg.Temperature != nil {
		streamReq.Temperature = *a.cfg.Temperature
	}
	if a.cfg.MaxTokens != nil {
		streamReq.MaxTokens = *a.cfg.MaxTokens
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

// callLLMAuto 统一的 LLM 调用（自动选择流式/非流式）
func (a *Agent) callLLMAuto(ctx context.Context, messages []ChatMessage, streamChan chan *StreamChunk, requestID string) (string, error) {
	if streamChan != nil {
		return a.callLLMWithStreamOutput(ctx, messages, streamChan, requestID)
	}
	return a.callLLM(ctx, messages)
}
