package aiagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

// executeDirect 执行单次 LLM 调用（无工具循环包装）。适用于无工具、纯文本生成的 action。
//
// 跟工具循环模式的关键差异：
//   - system prompt 不带 NativeSystemPrompt，也不下发 tools 参数
//   - 单次调用即终态，无迭代
//   - 流式 token 用 StreamTypeContent 类型发出，router 会路由进 content 通道而非 reason
func (a *Agent) executeDirect(ctx context.Context, req *AgentRequest, rc *runCtx) *AgentResponse {
	// Direct 模式不消费 tools——如果上游配了，记 warn 但不中断（让 action 配置错误
	// 表现为"答案不带工具结果"而非整轮失败，便于排查）
	if len(rc.tools) > 0 {
		logger.Warningf("[Agent] direct mode has %d tools configured but won't be used", len(rc.tools))
	}

	userMessage, err := a.buildUserMessage(req)
	if err != nil {
		return &AgentResponse{Error: fmt.Sprintf("failed to build user message: %v", err)}
	}

	systemPrompt := a.buildDirectSystemPrompt(rc)

	messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
	if len(req.History) > 0 {
		messages = append(messages, req.History...)
	}
	messages = append(messages, ChatMessage{Role: "user", Content: userMessage})

	logger.Infof("[Agent] Direct starting: system_len=%d, user_len=%d, history=%d, streaming=%v",
		len(systemPrompt), len(userMessage), len(req.History), req.StreamChan != nil)

	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	content, err := a.callLLMDirect(ctx, messages, req.StreamChan, requestID)
	if err != nil {
		return &AgentResponse{Error: fmt.Sprintf("LLM call failed: %v", err)}
	}

	return &AgentResponse{Content: content, Success: true, Iterations: 1}
}

// executeDirectWithDone 流式模式下的顶层入口，复用 executeDirect 后补一个 Done/Error chunk。
//
// 注意：StreamTypeDone 的 Content 字段刻意置空——内容已经通过 StreamTypeContent
// chunk 流式发完了；router 在 StreamTypeDone case 里 if chunk.Content != "" 才会
// 再次往 content 通道 append，置空就能避免内容翻倍。
func (a *Agent) executeDirectWithDone(ctx context.Context, req *AgentRequest, rc *runCtx) {
	streamChan := req.StreamChan
	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	resp := a.executeDirect(ctx, req, rc)

	if resp.Error != "" {
		streamChan <- &StreamChunk{
			Type:      StreamTypeError,
			Content:   resp.Error,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
		return
	}

	streamChan <- &StreamChunk{
		Type:      StreamTypeDone,
		Content:   "", // 内容已流式发完，避免 router 的 done case 重复 append
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
	}
}

// callLLMDirect 单次 LLM 调用：非流式直接返回正文；流式把 token 逐个以
// StreamTypeContent 发出（不经 thinking/工具通道）。
func (a *Agent) callLLMDirect(ctx context.Context, messages []ChatMessage, streamChan chan *StreamChunk, requestID string) (string, error) {
	if err := a.checkLLMClient(); err != nil {
		return "", err
	}

	// 非流式
	if streamChan == nil {
		resp, err := a.llmClient.Generate(ctx, buildLLMRequest(messages))
		if err != nil {
			return "", fmt.Errorf("LLM generate error: %w", err)
		}
		return resp.Content, nil
	}

	// 流式
	tStreamStart := time.Now()
	stream, err := a.llmClient.GenerateStream(ctx, buildLLMRequest(messages))
	if err != nil {
		logger.Errorf("[Agent] direct GenerateStream failed provider=%s: %v", a.llmClient.Name(), err)
		return "", fmt.Errorf("LLM stream error: %w", err)
	}

	var sb strings.Builder
	var firstTokenAt time.Time
	chunkCount := 0
	for chunk := range stream {
		if chunk.Error != nil {
			logger.Errorf("[Agent] direct stream chunk error provider=%s content_len=%d: %v",
				a.llmClient.Name(), sb.Len(), chunk.Error)
			go drainStream(stream)
			return sb.String(), fmt.Errorf("stream error: %w", chunk.Error)
		}
		if chunk.Content != "" {
			if firstTokenAt.IsZero() {
				firstTokenAt = time.Now()
				logger.Infof("[Agent] direct TTFT=%dms provider=%s", firstTokenAt.Sub(tStreamStart).Milliseconds(), a.llmClient.Name())
			}
			chunkCount++
			sb.WriteString(chunk.Content)
			streamChan <- &StreamChunk{
				Type:      StreamTypeContent,
				Delta:     chunk.Content,
				Content:   sb.String(),
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}
		if chunk.Done {
			break
		}
	}
	total := time.Since(tStreamStart)
	var genMs int64
	if !firstTokenAt.IsZero() {
		genMs = time.Since(firstTokenAt).Milliseconds()
	}
	logger.Infof("[Agent] direct done: total=%dms gen=%dms chunks=%d content_len=%d provider=%s",
		total.Milliseconds(), genMs, chunkCount, sb.Len(), a.llmClient.Name())
	return sb.String(), nil
}
