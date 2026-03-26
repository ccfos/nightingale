package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

// executePlanReAct 执行 Plan+ReAct Agent（统一入口，支持流式/非流式 + Skills）
func (a *Agent) executePlanReAct(ctx context.Context, req *AgentRequest, skills []*SkillContent) *AgentResponse {
	streaming := req.StreamChan != nil
	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	resp := &AgentResponse{
		Steps: []ReActStep{},
	}

	// 初始化工作记忆
	memory := a.initWorkingMemory()
	resp.Memory = memory

	// 1. 构建用户消息
	userMessage, err := a.buildUserMessage(req)
	if err != nil {
		resp.Error = fmt.Sprintf("failed to build user message: %v", err)
		return resp
	}

	// 2. 生成计划
	if streaming {
		req.StreamChan <- &StreamChunk{
			Type:      StreamTypePlan,
			Content:   "Generating execution plan...",
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
	}

	plan, err := a.generatePlan(ctx, req, userMessage, skills)
	if err != nil {
		resp.Error = fmt.Sprintf("failed to generate plan: %v", err)
		return resp
	}
	resp.Plan = plan

	// 发送计划详情
	if streaming {
		planJSON, _ := json.Marshal(plan)
		req.StreamChan <- &StreamChunk{
			Type:      StreamTypePlan,
			Content:   string(planJSON),
			Metadata:  map[string]interface{}{"steps": len(plan.Steps)},
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
	}

	// 限制步骤数
	if len(plan.Steps) > a.cfg.MaxPlanSteps {
		plan.Steps = plan.Steps[:a.cfg.MaxPlanSteps]
	}

	// 3. 逐步执行
	var allFindings []string
	for i := range plan.Steps {
		step := &plan.Steps[i]

		select {
		case <-ctx.Done():
			step.Status = "failed"
			step.Error = "context cancelled"
			resp.Error = "execution cancelled"
			return resp
		default:
		}

		plan.CurrentStep = i + 1
		step.Status = "executing"

		if streaming {
			req.StreamChan <- &StreamChunk{
				Type:    StreamTypeStep,
				Content: fmt.Sprintf("Step %d/%d: %s", i+1, len(plan.Steps), step.Goal),
				Metadata: map[string]interface{}{
					"step_number": step.StepNumber,
					"goal":        step.Goal,
				},
				RequestID: requestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		logger.Infof("[Agent] Executing step %d/%d: %s", i+1, len(plan.Steps), step.Goal)

		stepResult := a.executeStep(ctx, req, step, memory, allFindings, skills)

		// 更新步骤状态
		if stepResult.Error != "" {
			step.Status = "failed"
			step.Error = stepResult.Error
			logger.Warningf("[Agent] Step %d failed: %s", i+1, stepResult.Error)
		} else {
			step.Status = "completed"
			step.Summary = stepResult.Content
			step.Findings = a.extractStepFindings(stepResult)
			step.ReActSteps = stepResult.Steps
			step.Iterations = stepResult.Iterations

			if step.Findings != "" {
				allFindings = append(allFindings, step.Findings)
			}
		}

		// 合并步骤的执行轨迹到总响应
		resp.Steps = append(resp.Steps, stepResult.Steps...)
	}

	// 4. 综合分析
	if streaming {
		req.StreamChan <- &StreamChunk{
			Type:      StreamTypeSynthesis,
			Content:   "Synthesizing results...",
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
	}

	synthesis, err := a.synthesizeResults(ctx, req, plan, allFindings, skills)
	if err != nil {
		logger.Warningf("[Agent] Synthesis failed: %v, using step findings as result", err)
		synthesis = strings.Join(allFindings, "\n\n")
	}

	resp.Content = synthesis
	resp.Plan.Synthesis = synthesis
	resp.Success = true
	resp.Iterations = len(resp.Steps)

	return resp
}

// executePlanReActWithDone 执行 Plan+ReAct 并在流式模式下发送 done/error
func (a *Agent) executePlanReActWithDone(ctx context.Context, req *AgentRequest, skills []*SkillContent) {
	streamChan := req.StreamChan
	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	resp := a.executePlanReAct(ctx, req, skills)

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

// generatePlan 生成执行计划（统一支持流式/非流式）
func (a *Agent) generatePlan(ctx context.Context, req *AgentRequest, userMessage string, skills []*SkillContent) (*ExecutionPlan, error) {
	systemPrompt := a.buildPlanningPrompt(skills)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	response, err := a.callLLMAuto(ctx, messages, req.StreamChan, requestID)
	if err != nil {
		return nil, fmt.Errorf("plan generation LLM call failed: %v", err)
	}

	plan := a.parsePlanResponse(response)
	return plan, nil
}

// executeStep 执行单个计划步骤（使用 ReAct 循环）
func (a *Agent) executeStep(ctx context.Context, req *AgentRequest, step *PlanStep, memory *WorkingMemory, previousFindings []string, skills []*SkillContent) *AgentResponse {
	// 构建步骤执行的系统提示词
	systemPrompt := a.buildStepExecutionPrompt(step, previousFindings, skills)

	// 构建步骤的用户消息
	userMessage := a.buildStepUserMessage(req, step)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	maxIter := a.cfg.MaxStepIterations
	if maxIter <= 0 {
		maxIter = DefaultMaxStepIterations
	}

	memoryEnabled := a.cfg.Memory != nil && a.cfg.Memory.Enabled

	return a.runReActLoop(ctx, req, messages, &ReActLoopConfig{
		MaxIterations:         maxIter,
		Memory:                memory,
		MemoryEnabled:         memoryEnabled,
		IncludeMemoryInPrompt: memoryEnabled && a.cfg.Memory.IncludeInPrompt != nil && *a.cfg.Memory.IncludeInPrompt,
		TimeoutMessage:        fmt.Sprintf("step %d timeout", step.StepNumber),
		LogPrefix:             fmt.Sprintf("Step-%d", step.StepNumber),
		StreamChan:            req.StreamChan,
		RequestID:             requestID,
		IsComplete: func(action string) bool {
			return action == ActionFinalAnswer || action == ActionStepComplete
		},
		ExtractPartialResult: true,
	})
}

// synthesizeResults 综合所有步骤的结果（统一支持流式/非流式）
func (a *Agent) synthesizeResults(ctx context.Context, req *AgentRequest, plan *ExecutionPlan, allFindings []string, skills []*SkillContent) (string, error) {
	systemPrompt := a.buildSynthesisPrompt(skills)
	userMsg := a.buildSynthesisUserMessage(req, plan, allFindings)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}

	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	response, err := a.callLLMAuto(ctx, messages, req.StreamChan, requestID)
	if err != nil {
		return "", fmt.Errorf("synthesis LLM call failed: %v", err)
	}

	return response, nil
}
