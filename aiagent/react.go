package aiagent

import (
	"context"
	"fmt"
	"time"

	"github.com/toolkits/pkg/logger"
)

// runReActLoop 执行 ReAct 循环的核心逻辑（统一支持流式/非流式）
func (a *Agent) runReActLoop(ctx context.Context, req *AgentRequest, messages []ChatMessage, config *ReActLoopConfig) *AgentResponse {
	resp := &AgentResponse{
		Steps: []ReActStep{},
	}

	streaming := config.StreamChan != nil

	for iteration := 0; iteration < config.MaxIterations; iteration++ {
		select {
		case <-ctx.Done():
			resp.Error = config.TimeoutMessage
			resp.Iterations = iteration
			return resp
		default:
		}

		// 调用 LLM（自动选择流式/非流式）
		// 传 "Observation:" 作为 stop 序列：防止模型在写完 Action Input 后
		// 继续"脑补"工具结果（自造 Observation），保证真实工具输出由宿主注入。
		response, err := a.callLLMAuto(ctx, messages, config.StreamChan, config.RequestID, []string{"Observation:"})
		if err != nil {
			resp.Error = fmt.Sprintf("LLM call failed at iteration %d: %v", iteration, err)
			resp.Iterations = iteration
			return resp
		}

		// 解析 ReAct 响应
		step := a.parseReActResponse(response)
		resp.Steps = append(resp.Steps, step)

		logger.Debugf("%s iteration %d: thought=%s, action=%s", config.LogPrefix, iteration, step.Thought, step.Action)

		// Note: we deliberately do NOT emit a separate StreamTypeThinking chunk
		// here for step.Thought. callLLMAuto already streams the raw LLM
		// output character-by-character as StreamTypeText, which the router
		// accumulates into the message's reasoning channel — that raw stream
		// already contains the Thought text. Re-emitting the parsed Thought
		// would double it (the user reported reasoning duplication after
		// switching models because of this).

		// 检查是否完成
		if config.IsComplete(step.Action) {
			resp.Content = step.ActionInput
			resp.Iterations = iteration + 1
			resp.Success = true
			return resp
		}

		// Action 为空 → 将原始响应作为 Final Answer
		if step.Action == "" {
			resp.Content = response
			resp.Iterations = iteration + 1
			resp.Success = true
			return resp
		}

		// 发送 tool_call（流式模式）
		if streaming {
			config.StreamChan <- &StreamChunk{
				Type:      StreamTypeToolCall,
				Content:   step.Action,
				Metadata:  map[string]interface{}{"input": step.ActionInput},
				RequestID: config.RequestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		// 执行工具
		observation := a.executeTool(ctx, step.Action, step.ActionInput, req, config.Tools)
		step.Observation = observation
		resp.Steps[len(resp.Steps)-1] = step

		// 发送 tool_result（流式模式）
		if streaming {
			config.StreamChan <- &StreamChunk{
				Type:      StreamTypeToolResult,
				Content:   observation,
				Metadata:  map[string]interface{}{"tool": step.Action},
				RequestID: config.RequestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		observationMsg := fmt.Sprintf("Observation: %s", observation)

		messages = append(messages, ChatMessage{Role: "assistant", Content: response})
		messages = append(messages, ChatMessage{Role: "user", Content: observationMsg})
	}

	// 达到最大迭代次数
	resp.Error = fmt.Sprintf("reached maximum iterations (%d)", config.MaxIterations)
	resp.Iterations = config.MaxIterations

	if config.ExtractPartialResult && len(resp.Steps) > 0 {
		lastStep := resp.Steps[len(resp.Steps)-1]
		resp.Content = fmt.Sprintf("Analysis incomplete (max iterations reached). Last thought: %s", lastStep.Thought)
	}
	return resp
}

// executeReAct 执行 ReAct Agent（统一入口，支持流式/非流式 + Skills）
func (a *Agent) executeReAct(ctx context.Context, req *AgentRequest, rc *runCtx) *AgentResponse {
	// 构建用户消息
	userMessage, err := a.buildUserMessage(req)
	if err != nil {
		return &AgentResponse{Error: fmt.Sprintf("failed to build user message: %v", err)}
	}

	// 构建系统提示词（自动包含 Skills 知识）
	systemPrompt := a.buildReActSystemPrompt(rc)

	// 组装消息：system → 历史对话 → 当前 user
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
	}
	if len(req.History) > 0 {
		messages = append(messages, req.History...)
	}
	messages = append(messages, ChatMessage{Role: "user", Content: userMessage})

	logger.Infof("[Agent] ReAct starting: messages=%d, system_len=%d, user_len=%d, history=%d, streaming=%v",
		len(messages), len(systemPrompt), len(userMessage), len(req.History), req.StreamChan != nil)

	// 获取 requestID
	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	// Per-skill override: a skill may declare a higher MaxIterations in its
	// frontmatter for multi-step workflows (e.g. create_dashboard needs ~10+
	// tool calls). Pick the highest value across all active skills so a cheap
	// global default doesn't starve an expensive selected skill.
	maxIter := a.cfg.MaxIterations
	for _, sk := range rc.skills {
		if sk == nil || sk.Metadata == nil {
			continue
		}
		if sk.Metadata.MaxIterations > maxIter {
			maxIter = sk.Metadata.MaxIterations
			logger.Infof("[Agent] skill %q raised MaxIterations to %d", sk.Metadata.Name, maxIter)
		}
	}

	return a.runReActLoop(ctx, req, messages, &ReActLoopConfig{
		MaxIterations:        maxIter,
		TimeoutMessage:       "agent execution timeout",
		LogPrefix:            "AI Agent",
		Tools:                rc.tools,
		StreamChan:           req.StreamChan,
		RequestID:            requestID,
		IsComplete:           func(action string) bool { return action == ActionFinalAnswer },
		ExtractPartialResult: true,
	})
}

// executeReActWithDone 执行 ReAct 并在流式模式下发送 done/error chunk
// 用于流式模式的顶层调用
func (a *Agent) executeReActWithDone(ctx context.Context, req *AgentRequest, rc *runCtx) {
	streamChan := req.StreamChan
	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	resp := a.executeReAct(ctx, req, rc)

	if resp.Error != "" && !resp.Success {
		streamChan <- &StreamChunk{
			Type:      StreamTypeError,
			Error:     resp.Error,
			Done:      true,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
		return
	}

	streamChan <- &StreamChunk{
		Type:      StreamTypeDone,
		Content:   resp.Content,
		Done:      true,
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
	}
}
