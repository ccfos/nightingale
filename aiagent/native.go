package aiagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llm"

	"github.com/toolkits/pkg/logger"
)

// 本文件实现原生 function-calling 工具协议：loop 与 ReAct 共用同一套内部表示
// （runCtx 工具表、StreamChunk 契约、结构化 transcript），只是工具落线方式不同——
// tools 参数 + tool_calls 解析，替代三行文本协议 + "Observation:" 注入。
//
// 流式通道约定（与路由层 demux/抽卡/A2A 的既有契约严格一致；字段独占路由：
// 思考与正文互斥，绝不双发）：
//   - 推理增量（reasoning_content / 思考块）→ StreamTypeThinking（thinking 面板）。
//   - 模型正文增量 → StreamTypeContent 实时流，直接落 content 通道。中间轮的
//     pre-tool-call 评论同样进 content（轮间补段落分隔）。最终答案因此已逐
//     token 下发——executeReActWithDone 的 Done chunk 据 contentStreamed 打标，
//     路由层只把 Done.Content 当作解析/持久化用的权威正文（= 最终轮正文，
//     不含中间轮评论），不再二次推流，无重复。
//   - 唯一押后：首轮带工具时，疑似文本形态工具调用的正文（裸 JSON/<tool_call>/
//     代码围栏起头）先押住不发——坐实则 fallback ReAct 重跑且该段丢弃，证伪则
//     整段补发（见 callLLMNative 与 looksLikeNativeFallback，两处判定必须同口径）。
//   - 工具调用/结果 emit 与 ReAct 完全相同形状的 StreamTypeToolCall /
//     StreamTypeToolResult（Metadata["tool"]）→ 抽卡与 A2A 零改动。

// ResolveToolProtocol 归一工具协议：显式 "react" 才走文本协议，其余（含空值）
// 一律原生 function calling——**原生是默认**，react 是不支持原生 FC 端点的显式
// 降级。dispatch（executeReAct）与 thinking 策略（router）共用此语义，保证
// "同一配置同一协议"。
func ResolveToolProtocol(s string) string {
	if s == ToolProtocolReAct {
		return ToolProtocolReAct
	}
	return ToolProtocolNative
}

// executeNative 原生协议的执行入口，与 executeReAct 一一对应。
func (a *Agent) executeNative(ctx context.Context, req *AgentRequest, rc *runCtx) *AgentResponse {
	userMessage, err := a.buildUserMessage(req)
	if err != nil {
		return &AgentResponse{Error: fmt.Sprintf("failed to build user message: %v", err)}
	}

	systemPrompt := a.buildNativeSystemPrompt(rc)

	// 历史投影：与 ReAct 路径共用同一投影点，canonical transcript 不变。
	messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
	if len(req.History) > 0 {
		messages = append(messages, projectHistory(req.History, a.cfg.HistoryBudgetBytes)...)
	}
	messages = append(messages, ChatMessage{Role: "user", Content: userMessage})

	requestID := ""
	if req.Metadata != nil {
		requestID = req.Metadata["request_id"]
	}

	logger.Infof("[Agent] native starting: messages=%d, system_len=%d, user_len=%d, history=%d, tools=%d, streaming=%v",
		len(messages), len(systemPrompt), len(userMessage), len(req.History), len(rc.tools), req.StreamChan != nil)

	return a.runNativeLoop(ctx, req, messages, buildNativeToolDefs(rc.tools), &ReActLoopConfig{
		MaxIterations:        a.maxIterationsForSkills(rc),
		TimeoutMessage:       "agent execution timeout",
		LogPrefix:            "AI Agent(native)",
		Tools:                rc.tools,
		StreamChan:           req.StreamChan,
		RequestID:            requestID,
		ExtractPartialResult: true,
		EmitTranscript:       true, // 顶层 chat：本轮 transcript 持久化以供下一轮回放
	})
}

// runNativeLoop 原生 function-calling 循环：调 LLM（带 tools）→ 无 tool_calls
// 即最终答案；有则逐个执行、回灌结构化 tool 结果轮，并 emit 与 ReAct 同形状的
// 流式事件 + 结构化 transcript。
func (a *Agent) runNativeLoop(ctx context.Context, req *AgentRequest, messages []ChatMessage, toolDefs []llm.ToolDefinition, config *ReActLoopConfig) *AgentResponse {
	resp := &AgentResponse{Steps: []ReActStep{}}
	streaming := config.StreamChan != nil
	dedup := newTurnWriteDeduper() // 写类工具轮内幂等

	for iteration := 0; iteration < config.MaxIterations; iteration++ {
		select {
		case <-ctx.Done():
			resp.Error = config.TimeoutMessage
			resp.Iterations = iteration
			return resp
		default:
		}

		// holdSuspicious：仅首轮带工具时可能触发 fallbackToReAct，该轮疑似文本
		// 形态工具调用的正文须押住——fallback 后它绝不能已经进了 content 通道。
		holdSuspicious := iteration == 0 && len(toolDefs) > 0
		content, calls, thinking, err := a.callLLMNative(ctx, messages, toolDefs, config.StreamChan, config.RequestID, holdSuspicious)
		if err != nil {
			resp.Error = fmt.Sprintf("LLM call failed at iteration %d: %v", iteration, err)
			resp.Iterations = iteration
			return resp
		}

		logger.Debugf("%s iteration %d: content_len=%d, tool_calls=%d", config.LogPrefix, iteration, len(content), len(calls))

		// 无工具调用 = 最终答案（原生协议的天然完成信号，无需 IsComplete 解析）。
		if len(calls) == 0 {
			// 自动降级探测：首轮就零 tool_calls、但正文是文本形态的工具调用
			// （裸 JSON {"name":...,"arguments":...} / <tool_call> 标签 / tool_calls
			// envelope），说明端点把 tools 参数当空气——典型的"OpenAI 兼容但不
			// 支持原生 FC"。这坨文本绝不是答案：置 fallbackToReAct，由
			// executeReAct 改用文本协议重跑本轮（首轮无任何已执行工具，重跑干净；
			// callLLMNative 以同口径押住了这段正文，content 通道未被污染）。
			if iteration == 0 && len(toolDefs) > 0 && looksLikeNativeFallback(content) {
				resp.fallbackToReAct = true
				resp.Iterations = 1
				return resp
			}
			resp.Content = content
			resp.contentStreamed = streaming // 正文已逐 token 走 StreamTypeContent，Done 不再推流
			resp.Iterations = iteration + 1
			resp.Success = true
			return resp
		}

		// 中间轮评论（与 tool_calls 同轮的正文）已实时进 content 通道；补一个
		// 段落分隔，避免与后续轮正文/最终答案在 markdown 里粘连成一段。
		if streaming && strings.TrimSpace(content) != "" {
			config.StreamChan <- &StreamChunk{
				Type:      StreamTypeContent,
				Delta:     "\n\n",
				RequestID: config.RequestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}

		// Gemini 等不回 call id 的端点：合成确定性 id，保证 transcript 回放时
		// tool_call_id 配对完整（OpenAI/Claude 回放都要求成对）。
		for i := range calls {
			if calls[i].ID == "" {
				calls[i].ID = fmt.Sprintf("call_%d_%d", iteration, i)
			}
		}

		// ThinkingBlocks 随 assistant 轮进入上下文与 transcript：Anthropic 系开启
		// thinking 后，续轮回填这些块（带签名）是 API 硬性要求。
		asstTurn := ChatMessage{Role: "assistant", Content: content, ToolCalls: calls, ThinkingBlocks: thinking}
		messages = append(messages, asstTurn)
		turn := []ChatMessage{asstTurn}

		for _, tc := range calls {
			step := ReActStep{Thought: content, Action: tc.Name, ActionInput: tc.Arguments}

			if streaming {
				config.StreamChan <- &StreamChunk{
					Type:      StreamTypeToolCall,
					Content:   tc.Name,
					Metadata:  map[string]interface{}{"input": tc.Arguments},
					RequestID: config.RequestID,
					Timestamp: time.Now().UnixMilli(),
				}
			}

			var observation string
			if cached, hit := dedup.lookup(tc.Name, tc.Arguments); hit {
				// 同轮同名同参的写工具重复调用：复用首次结果，不重复落库。
				observation = cached + "\n\n(本轮已用相同参数执行过该写操作，以上为首次执行结果，未重复执行)"
			} else {
				var interrupt *ToolInterrupt
				observation, interrupt = a.executeToolI(ctx, tc.Name, tc.Arguments, req, config.Tools)
				if interrupt != nil {
					// 人在环中断：确认文案即本轮答复；本轮不产 ToolResult/transcript
					// （重放所需状态在 Pending 里，无需进入对话历史）。
					emitInterrupt(config, interrupt, tc.Name)
					resp.Content = interrupt.Prompt
					resp.Iterations = iteration + 1
					resp.Success = true
					return resp
				}
				dedup.record(tc.Name, tc.Arguments, observation)
			}

			// 工具渐进披露：load_skill 成功后把该
			// 技能声明的 builtin_tools 注入工具表并重建 toolDefs，下一迭代原生
			// tools 参数即包含它们；观测尾部附注新工具名，模型当轮即知。
			if tc.Name == "load_skill" && !strings.HasPrefix(observation, "Error:") {
				if name := skillNameFromLoadArgs(tc.Arguments); name != "" {
					var added []string
					config.Tools, added = a.injectSkillTools(config.Tools, name)
					if len(added) > 0 {
						toolDefs = buildNativeToolDefs(config.Tools)
						observation += "\n\n(已随技能启用工具: " + strings.Join(added, ", ") + ")"
					}
				}
			}
			step.Observation = observation
			resp.Steps = append(resp.Steps, step)

			if streaming {
				config.StreamChan <- &StreamChunk{
					Type:      StreamTypeToolResult,
					Content:   observation,
					Metadata:  map[string]interface{}{"tool": tc.Name},
					RequestID: config.RequestID,
					Timestamp: time.Now().UnixMilli(),
				}
			}

			toolTurn := ChatMessage{Role: llm.RoleTool, ToolCallID: tc.ID, ToolName: tc.Name, Content: observation}
			messages = append(messages, toolTurn)
			turn = append(turn, toolTurn)
		}

		// 结构化 transcript：assistant(tool_calls) + tool 结果轮成组持久化，
		// 工具产物（如 proposal_id）下一轮直接以原生形态回放。
		emitTranscript(config, turn...)
	}

	resp.Error = fmt.Sprintf("reached maximum iterations (%d)", config.MaxIterations)
	resp.Iterations = config.MaxIterations
	if config.ExtractPartialResult && len(resp.Steps) > 0 {
		last := resp.Steps[len(resp.Steps)-1]
		resp.Content = fmt.Sprintf("Analysis incomplete (max iterations reached). Last thought: %s", last.Thought)
	}
	return resp
}

// callLLMNative 以原生 tools 参数调用 LLM 并聚合一轮结果（正文、工具调用、
// 思考块）。
//
// 流式时按字段独占路由（杜绝"思考面板 ≡ 回答"的双发）：推理增量走
// StreamTypeThinking，正文增量走 StreamTypeContent，互斥。
// 工具调用增量按"空 ID+空 Name → 续接上一个调用的 Arguments"规则合并 OpenAI
// 风格的分片（provider 流式把一次调用的 arguments 拆成多个 delta 下发）；
// Anthropic 系的完整思考块（含签名）在块收尾时整块到达，累积后随 assistant 轮回填。
//
// holdSuspicious（仅首轮带工具时为 true）：疑似文本形态工具调用的正文先押住
// 不进 content 通道——坐实（looksLikeNativeFallback，与 runNativeLoop 的降级
// 判定同口径）则丢弃，该轮将由 ReAct 文本协议重跑；证伪则整段补发，仅损失
// 这一轮的逐 token 流式（答案以 JSON/标签/围栏起头的场景，罕见且可接受）。
func (a *Agent) callLLMNative(ctx context.Context, messages []ChatMessage, toolDefs []llm.ToolDefinition, streamChan chan *StreamChunk, requestID string, holdSuspicious bool) (string, []llm.ToolCall, []llm.ThinkingBlock, error) {
	if err := a.checkLLMClient(); err != nil {
		return "", nil, nil, err
	}

	llmReq := buildLLMRequest(messages, nil)
	llmReq.Tools = toolDefs

	if streamChan == nil {
		genResp, err := a.llmClient.Generate(ctx, llmReq)
		if err != nil {
			return "", nil, nil, err
		}
		return genResp.Content, genResp.ToolCalls, genResp.ThinkingBlocks, nil
	}

	stream, err := a.llmClient.GenerateStream(ctx, llmReq)
	if err != nil {
		logger.Errorf("[Agent] GenerateStream(native) failed provider=%s: %v", a.llmClient.Name(), err)
		return "", nil, nil, fmt.Errorf("LLM stream error: %w", err)
	}

	var content strings.Builder
	var calls []llm.ToolCall
	var thinking []llm.ThinkingBlock
	emit := func(chunkType, delta string) {
		streamChan <- &StreamChunk{
			Type:      chunkType,
			Delta:     delta,
			RequestID: requestID,
			Timestamp: time.Now().UnixMilli(),
		}
	}

	// 正文押后缓冲：holdSuspicious 时在前缀形态判明前先积攒。判明无嫌疑即整段
	// 补发并转为逐 token 直发；判明疑似则押到收尾，由"是否坐实文本形态工具调用"
	// 决定补发还是丢弃。
	var hold strings.Builder
	holding := holdSuspicious

	for chunk := range stream {
		if chunk.Error != nil {
			go drainStream(stream)
			return content.String(), calls, thinking, fmt.Errorf("stream error: %w", chunk.Error)
		}
		if chunk.Reasoning != "" {
			emit(StreamTypeThinking, chunk.Reasoning)
		}
		if chunk.ThinkingBlock != nil {
			thinking = append(thinking, *chunk.ThinkingBlock)
		}
		if chunk.Content != "" {
			content.WriteString(chunk.Content)
			if holding {
				hold.WriteString(chunk.Content)
				if suspicious, determined := suspiciousToolCallPrefix(hold.String()); determined && !suspicious {
					holding = false
					emit(StreamTypeContent, hold.String())
					hold.Reset()
				}
			} else {
				emit(StreamTypeContent, chunk.Content)
			}
		}
		for _, tc := range chunk.ToolCalls {
			if tc.ID == "" && tc.Name == "" && len(calls) > 0 {
				calls[len(calls)-1].Arguments += tc.Arguments
			} else {
				calls = append(calls, tc)
			}
		}
		if chunk.Done {
			break
		}
	}

	// 押到收尾仍未发的正文：坐实为文本形态工具调用（且无原生 calls）时丢弃——
	// runNativeLoop 将以同一口径触发 fallbackToReAct，这段不该进回答区；否则补发。
	if hold.Len() > 0 && !(len(calls) == 0 && looksLikeNativeFallback(content.String())) {
		emit(StreamTypeContent, hold.String())
	}
	return content.String(), calls, thinking, nil
}

// suspiciousToolCallPrefix 判断正文开头是否可能是文本形态的工具调用——端点把
// tools 参数当空气时，典型输出从第一个非空白字符就是 JSON 对象/数组、XML 标签
// 或代码围栏。determined=false 表示目前还全是空白，需继续观望。
func suspiciousToolCallPrefix(s string) (suspicious, determined bool) {
	t := strings.TrimLeft(s, " \t\r\n")
	if t == "" {
		return false, false
	}
	switch t[0] {
	case '{', '[', '<', '`':
		return true, true
	}
	return false, true
}

// looksLikeNativeFallback 原生 loop"端点不支持原生 FC"的降级判定：在
// looksLikeToolCall 的形状匹配之上再要求疑似前缀。收紧的目的是与 callLLMNative
// 的正文押后保持同一口径——fallback 触发 ⇒ 该轮正文从未进入 content 通道，
// ReAct 重跑的答案不会与之重复。正文以普通文字开头、中段才嵌工具 JSON 的漂移
// 不再触发降级（目标场景"端点完全无视 tools"的输出都是裸 JSON 从头开始）。
func looksLikeNativeFallback(content string) bool {
	suspicious, determined := suspiciousToolCallPrefix(content)
	return determined && suspicious && looksLikeToolCall(content)
}

// buildNativeToolDefs 把 AgentTool 的扁平参数表编译成 JSON-Schema 形态的
// llm.ToolDefinition，供原生 tools 参数下发。
func buildNativeToolDefs(tools []AgentTool) []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		props := map[string]interface{}{}
		var required []string
		for _, p := range t.Parameters {
			props[p.Name] = map[string]interface{}{
				"type":        normalizeJSONSchemaType(p.Type),
				"description": p.Description,
			}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		params := map[string]interface{}{"type": "object", "properties": props}
		if len(required) > 0 {
			params["required"] = required
		}
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		})
	}
	return defs
}

// normalizeJSONSchemaType 把工具定义里随手写的类型名归一到 JSON-Schema 类型，
// 未知类型按 string 兜底（provider 对非法类型会整单拒绝，宁可宽松）。
func normalizeJSONSchemaType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "int", "integer", "int64":
		return "integer"
	case "bool", "boolean":
		return "boolean"
	case "float", "double", "number":
		return "number"
	case "object", "map":
		return "object"
	case "array", "list":
		return "array"
	default:
		return "string"
	}
}

// maxIterationsForSkills 取全局默认与所有激活 skill 声明的 MaxIterations 的最大值
// （skill 可在 frontmatter 为多步工作流抬高上限），ReAct 与 native 两条协议共用。
func (a *Agent) maxIterationsForSkills(rc *runCtx) int {
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
	return maxIter
}
