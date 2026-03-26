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
		Steps:  []ReActStep{},
		Memory: config.Memory,
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
		response, err := a.callLLMAuto(ctx, messages, config.StreamChan, config.RequestID)
		if err != nil {
			resp.Error = fmt.Sprintf("LLM call failed at iteration %d: %v", iteration, err)
			resp.Iterations = iteration
			return resp
		}

		// 解析 ReAct 响应
		step := a.parseReActResponse(response)
		resp.Steps = append(resp.Steps, step)

		logger.Debugf("%s iteration %d: thought=%s, action=%s", config.LogPrefix, iteration, step.Thought, step.Action)

		// 发送 thinking（流式模式）
		if streaming && step.Thought != "" {
			config.StreamChan <- &StreamChunk{
				Type:      StreamTypeThinking,
				Content:   step.Thought,
				RequestID: config.RequestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

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
		observation := a.executeTool(ctx, step.Action, step.ActionInput, req)
		step.Observation = observation
		resp.Steps[len(resp.Steps)-1] = step

		// 发送 tool_result（流式模式）
		if streaming {
			config.StreamChan <- &StreamChunk{
				Type:      StreamTypeToolResult,
				Content:   observation,
				RequestID: config.RequestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		// 更新工作记忆
		if config.Memory != nil {
			a.updateWorkingMemory(config.Memory, step)
		}

		// 构建观察消息
		observationMsg := fmt.Sprintf("Observation: %s", observation)
		if config.MemoryEnabled && config.IncludeMemoryInPrompt && config.Memory != nil && len(config.Memory.KeyFindings) > 0 {
			memorySummary := a.formatWorkingMemorySummary(config.Memory)
			observationMsg = fmt.Sprintf("%s\n\n%s", observationMsg, memorySummary)
		}

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
func (a *Agent) executeReAct(ctx context.Context, req *AgentRequest, skills []*SkillContent) *AgentResponse {
	memoryEnabled := a.cfg.Memory != nil && a.cfg.Memory.Enabled

	// 构建用户消息
	userMessage, err := a.buildUserMessage(req)
	if err != nil {
		return &AgentResponse{Error: fmt.Sprintf("failed to build user message: %v", err)}
	}

	// 构建系统提示词（自动包含 Skills 知识）
	systemPrompt := a.buildReActSystemPrompt(skills)

	// 初始化工作记忆
	var memory *WorkingMemory
	if memoryEnabled {
		systemPrompt = a.appendMemoryInstructions(systemPrompt)
		memory = &WorkingMemory{
			KeyFindings:      []KeyFinding{},
			TestedHypotheses: []Hypothesis{},
			Evidence:         []Evidence{},
		}
	}

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

	return a.runReActLoop(ctx, req, messages, &ReActLoopConfig{
		MaxIterations:         a.cfg.MaxIterations,
		Memory:                memory,
		MemoryEnabled:         memoryEnabled,
		IncludeMemoryInPrompt: memoryEnabled && a.cfg.Memory != nil && a.cfg.Memory.IncludeInPrompt != nil && *a.cfg.Memory.IncludeInPrompt,
		TimeoutMessage:        "agent execution timeout",
		LogPrefix:             "AI Agent",
		StreamChan:            req.StreamChan,
		RequestID:             requestID,
		IsComplete:            func(action string) bool { return action == ActionFinalAnswer },
		ExtractPartialResult:  true,
	})
}

// executeReActWithDone 执行 ReAct 并在流式模式下发送 done/error chunk
// 用于流式模式的顶层调用
func (a *Agent) executeReActWithDone(ctx context.Context, req *AgentRequest, skills []*SkillContent) {
	streamChan := req.StreamChan
	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	resp := a.executeReAct(ctx, req, skills)

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
