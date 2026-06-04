package aiagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
)

// runReActLoop 执行 ReAct 循环的核心逻辑（统一支持流式/非流式）
func (a *Agent) runReActLoop(ctx context.Context, req *AgentRequest, messages []ChatMessage, config *ReActLoopConfig) *AgentResponse {
	resp := &AgentResponse{
		Steps: []ReActStep{},
	}

	streaming := config.StreamChan != nil

	// 连续"格式纠正"重试计数：模型用非 ReAct 形态发工具调用（空 Action）时，
	// 回灌纠错 Observation 让它重发，而不是把整段当 Final Answer 静默终止。
	// 连续超过 maxFormatRetries 次仍纠正不过来，则优雅兜底为最终答案，
	// 避免在格式纠缠上烧光全部迭代。解析出有效 Action 后清零。
	formatRetries := 0
	const maxFormatRetries = 2

	dedup := newTurnWriteDeduper() // 写类工具轮内幂等

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

		// Action 为空：可能是 (a) 真正的最终答案，或 (b) 模型用非 ReAct 形态
		// （裸 JSON 工具调用数组 / 原生 function-calling / 未归一化的 XML）发了工具调用，
		// 被整段吸进了 Thought。后者绝不能当成 Final Answer 静默成功返回——那样工具一个
		// 都没执行，对话会在第 0 轮就"假完成"终止（用户报告的"对话没完成就终止"）。
		if step.Action == "" {
			if looksLikeToolCall(response) && formatRetries < maxFormatRetries {
				formatRetries++
				logger.Warningf("%s iteration %d: empty Action but response looks like a non-ReAct tool call; "+
					"injecting format-correction and retrying (%d/%d)",
					config.LogPrefix, iteration, formatRetries, maxFormatRetries)
				messages = append(messages, ChatMessage{Role: "assistant", Content: response})
				messages = append(messages, ChatMessage{Role: "user", Content: reactFormatCorrection})
				continue
			}
			// 真正的最终答案，或多次纠正未果的优雅兜底。
			resp.Content = response
			resp.Iterations = iteration + 1
			resp.Success = true
			return resp
		}

		// 解析出有效 Action，模型已回到规范形态 → 清零连续纠正计数。
		formatRetries = 0

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

		// 执行工具（中断感知：工具要求用户确认时停轮，确认文案即本轮答复，
		// Pending 由路由层持久化、下一轮确定性 resume——见 interrupt.go）
		var observation string
		if cached, hit := dedup.lookup(step.Action, step.ActionInput); hit {
			// 同轮同名同参的写工具重复调用：复用首次结果，不重复落库。
			observation = cached + "\n\n(本轮已用相同参数执行过该写操作，以上为首次执行结果，未重复执行)"
		} else {
			var interrupt *ToolInterrupt
			observation, interrupt = a.executeToolI(ctx, step.Action, step.ActionInput, req, config.Tools)
			if interrupt != nil {
				emitInterrupt(config, interrupt, step.Action)
				resp.Content = interrupt.Prompt
				resp.Iterations = iteration + 1
				resp.Success = true
				return resp
			}
			dedup.record(step.Action, step.ActionInput, observation)
		}

		// 工具渐进披露：load_skill 成功后注入该技能声明的 builtin_tools。文本协议
		// 的工具说明固化在 system prompt 里，模型只能从观测尾部的附注得知新工具——
		// executeToolI 按 config.Tools 查表，注入后即可执行。
		if step.Action == "load_skill" && !strings.HasPrefix(observation, "Error:") {
			if name := skillNameFromLoadArgs(step.ActionInput); name != "" {
				var added []string
				config.Tools, added = a.injectSkillTools(config.Tools, name)
				if len(added) > 0 {
					observation += "\n\n(已随技能启用工具: " + strings.Join(added, ", ") + "；可在后续 Action 中直接调用)"
				}
			}
		}
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

		toolTurn := ChatMessage{Role: "assistant", Content: response}
		obsTurn := ChatMessage{Role: "user", Content: observationMsg}
		messages = append(messages, toolTurn, obsTurn)

		// 把本轮的工具调用轮 + Observation 轮交给宿主持久化为可回放 transcript，
		// 使工具产物（如 proposal_id）下一轮对模型可见。只发语义上有效的工具步；
		// 格式纠正等轮内噪声不入历史。最终答案轮也不在此发，由路由层统一补
		// assistant(fullContent) 作终态。
		emitTranscript(config, toolTurn, obsTurn)
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
	// 工具协议分流：默认（含空值）走原生 function-calling loop；
	// 显式配置 react 的端点走 ReAct 文本协议降级。
	// 单一分流点：Run 的流式/非流式两条路径都经由本函数（executeReActWithDone
	// 也调它）。
	//
	// 自动降级：原生 loop 首轮即识别出"端点不支持原生 FC"（零 tool_calls 但
	// 正文是文本形态的工具调用）时返回 fallbackToReAct，当轮改跑文本协议——
	// 用户无需等管理员改配置；日志会建议将该 LLM 配置显式置为 react。
	if ResolveToolProtocol(a.cfg.ToolProtocol) == ToolProtocolNative {
		resp := a.executeNative(ctx, req, rc)
		if !resp.fallbackToReAct {
			return resp
		}
		logger.Warningf("[Agent] endpoint appears to lack native function-calling support "+
			"(round-0 emitted a text-form tool call); falling back to the ReAct text protocol for this turn. "+
			"Set tool_protocol=\"react\" on this LLM config to skip the wasted native attempt (provider=%s)",
			a.llmClient.Name())
	}

	return a.executeReActText(ctx, req, rc)
}

// executeReActText ReAct 文本协议的执行体（原 executeReAct 主体）。被两类调用方
// 使用：显式 react 配置的分流，以及原生协议的运行时自动降级。
func (a *Agent) executeReActText(ctx context.Context, req *AgentRequest, rc *runCtx) *AgentResponse {
	// 构建用户消息
	userMessage, err := a.buildUserMessage(req)
	if err != nil {
		return &AgentResponse{Error: fmt.Sprintf("failed to build user message: %v", err)}
	}

	// 构建系统提示词（自动包含 Skills 知识）
	systemPrompt := a.buildReActSystemPrompt(rc)

	// 组装消息：system → 历史投影（canonical transcript 不变，喂模型的是
	// 截断/收窗后的投影，见 context_manager.go）→ 当前 user
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
	}
	if len(req.History) > 0 {
		messages = append(messages, projectHistory(req.History, a.cfg.HistoryBudgetBytes)...)
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
		MaxIterations:        a.maxIterationsForSkills(rc),
		TimeoutMessage:       "agent execution timeout",
		LogPrefix:            "AI Agent",
		Tools:                rc.tools,
		StreamChan:           req.StreamChan,
		RequestID:            requestID,
		IsComplete:           func(action string) bool { return action == ActionFinalAnswer },
		ExtractPartialResult: true,
		EmitTranscript:       true, // 顶层 chat：本轮 transcript 持久化以供下一轮回放
	})
}

// emitInterrupt 把工具的人在环中断交给路由层（持久化 Pending 并结束本轮）。
func emitInterrupt(config *ReActLoopConfig, ti *ToolInterrupt, toolName string) {
	if config.StreamChan == nil {
		return
	}
	config.StreamChan <- &StreamChunk{
		Type:    StreamTypeInterrupt,
		Content: ti.Prompt,
		Metadata: map[string]interface{}{
			"kind":        ti.Kind,
			"tool":        toolName,
			"resume_args": ti.ResumeArgs,
			// form 仅 input 类非空：form_select 载荷，路由层渲染成与 preflight
			// 同契约的表单 response。
			"form": ti.Form,
		},
		RequestID: config.RequestID,
		Timestamp: time.Now().UnixMilli(),
	}
}

// emitTranscript 在流式且开启 EmitTranscript 时，把本轮新追加的规范消息经 StreamChan
// 交给路由层持久化。非流式或关闭时为 no-op。
func emitTranscript(config *ReActLoopConfig, msgs ...ChatMessage) {
	if config.StreamChan == nil || !config.EmitTranscript || len(msgs) == 0 {
		return
	}
	config.StreamChan <- &StreamChunk{
		Type:       StreamTypeTranscript,
		Transcript: msgs,
		RequestID:  config.RequestID,
		Timestamp:  time.Now().UnixMilli(),
	}
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

	done := &StreamChunk{
		Type:      StreamTypeDone,
		Content:   resp.Content,
		Done:      true,
		RequestID: requestID,
		Timestamp: time.Now().UnixMilli(),
	}
	if resp.contentStreamed {
		// native 流式路径：正文已逐 token 走 StreamTypeContent 下发。打标让路由层
		// 把 Done.Content 仅当作解析/持久化用的权威正文（= 最终轮正文，不含中间轮
		// 评论），不再二次推流——否则回答整段重复。
		done.Metadata = map[string]interface{}{"content_streamed": true}
	}
	streamChan <- done
}
