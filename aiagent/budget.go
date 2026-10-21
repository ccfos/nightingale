package aiagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/llm"

	"github.com/toolkits/pkg/logger"
)

// 本文件是工具循环的"预算"侧：迭代预算将尽时提醒模型收敛、预算用尽后做一次
// 无新工具的收尾总结，以及逐轮/逐工具的脱敏诊断日志。

// budgetWarnRemaining 剩余迭代数降到该值时，在最后一条工具观测尾部提醒模型
// 尽快收敛。循环总预算不大于 budgetWarnRemaining+1 时不提醒（短循环本身就紧）。
const budgetWarnRemaining = 2

// budgetWarningNote 返回预算将尽的提醒文本；不需要提醒时返回空串。
// iteration 为刚执行完工具的迭代下标（0 起）。
func budgetWarningNote(iteration, maxIterations int, lang string) string {
	if maxIterations <= budgetWarnRemaining+1 || maxIterations-(iteration+1) != budgetWarnRemaining {
		return ""
	}
	return LangText(lang,
		fmt.Sprintf("\n\n[系统提示] 工具调用预算仅剩 %d 轮。请优先基于已获取的数据收敛结论，只在确有必要时再调用工具。", budgetWarnRemaining),
		fmt.Sprintf("\n\n[System notice] Only %d tool-call rounds remain. Converge on a conclusion from the data already gathered; call tools only if strictly necessary.", budgetWarnRemaining))
}

// appendToLastMessage 把 note 追加到 messages 最后一条消息（通常是工具观测）
// 的正文。只改喂给模型的投影 messages，不改 transcript——提示是运行期信号，
// 不该在下一轮回放里出现。追加到已有消息而不是新开一条 user 轮，避免破坏
// Claude/Gemini 的 user/assistant 交替约束。
func appendToLastMessage(messages []ChatMessage, note string) {
	if note == "" || len(messages) == 0 {
		return
	}
	messages[len(messages)-1].Content += note
}

// repeatedCallNote 同一工具循环内第 n 次（n>1）以完全相同参数调用读类工具时
// 附在观测尾部的提醒。读类工具仍真实执行（轮询类工具需要重复读），只提示
// 模型复用已有结果，打断"同一查询反复下钻"的空转。
func repeatedCallNote(n int, lang string) string {
	if n <= 1 {
		return ""
	}
	return LangText(lang,
		fmt.Sprintf("\n\n(本轮已用完全相同的参数调用该工具 %d 次，结果不会变化，请直接基于已有结果推进)", n),
		fmt.Sprintf("\n\n(This tool has been called %d times with identical arguments in this turn; the result will not change, proceed with what you have.)", n))
}

// exhaustedHeader 预算用尽收尾时置于正文开头的确定性提示，保证无论模型
// 收尾写成什么样，用户都能看出这是不完整的阶段性结论。
func exhaustedHeader(maxIterations int, lang string) string {
	return LangText(lang,
		fmt.Sprintf("> ⚠️ 已达到工具调用上限（%d 轮），分析未完成。以下是基于已获取数据的阶段性结论，可能不完整。\n\n", maxIterations),
		fmt.Sprintf("> ⚠️ Analysis incomplete: the tool-call limit (%d rounds) was reached. Below is a partial conclusion based on the data gathered so far.\n\n", maxIterations))
}

func finalizeInstruction(lang string) string {
	return LangText(lang,
		"\n\n[系统提示] 工具调用预算已用尽，不能再调用任何工具。请立即基于以上已获取的数据给出最终答复：分别列出（1）已由数据确认的结论；（2）因证据不足尚无法确认的部分及还需要查什么。不要编造未查询到的数据。",
		"\n\n[System notice] The tool-call budget is exhausted; do NOT call any more tools. Give your final answer now from the data gathered above, listing separately (1) conclusions confirmed by the data and (2) what remains unconfirmed for lack of evidence and what would still need to be checked. Do not invent data you did not query.")
}

// finalizeExhaustedLoop 迭代预算用尽后的收尾：再调用一次 LLM，要求只基于已有
// 观测作答。仍带 toolDefs——Anthropic 要求历史含 tool_use 时请求必须声明 tools；
// 模型若仍发起工具调用则忽略之。收尾失败或正文为空时退化为确定性的进度摘要
// （最后一段非空思考 + 已调用工具统计），不再拼出空白的 "Last thought:"。
//
// 流式时 header/收尾正文/兜底摘要都按顺序走 StreamTypeContent，resp.Content 与
// 推流内容一致，因此统一打 contentStreamed。header 推迟到收尾调用的首个正文
// 增量之前才下发：路由层按到达顺序切段、只用 Done.Content 替换最后一个正文段，
// 若 header 先于收尾轮的思考段推出，就会自成一段、与权威正文里的 header 重复。
func (a *Agent) finalizeExhaustedLoop(ctx context.Context, req *AgentRequest, messages []ChatMessage, toolDefs []llm.ToolDefinition, config *ToolLoopConfig, resp *AgentResponse) {
	lang := req.Params["lang"]
	streaming := config.StreamChan != nil
	emit := func(delta string) {
		if streaming && delta != "" {
			config.StreamChan <- &StreamChunk{
				Type:      StreamTypeContent,
				Delta:     delta,
				RequestID: config.RequestID,
				Timestamp: time.Now().UnixMilli(),
			}
		}
	}

	header := exhaustedHeader(config.MaxIterations, lang)
	headerSent := false
	emitHeader := func() {
		if !headerSent {
			headerSent = true
			emit(header)
		}
	}

	final := make([]ChatMessage, len(messages))
	copy(final, messages)
	appendToLastMessage(final, finalizeInstruction(lang))

	// 流式时经代理通道转发收尾调用的增量，在首个正文增量前插入 header。
	var proxy chan *StreamChunk
	forwarded := make(chan struct{})
	if streaming {
		proxy = make(chan *StreamChunk, cap(config.StreamChan))
		go func() {
			defer close(forwarded)
			for ch := range proxy {
				if ch.Type == StreamTypeContent {
					emitHeader()
				}
				config.StreamChan <- ch
			}
		}()
	}
	out, err := a.callLLMNative(ctx, final, toolDefs, proxy, config.RequestID, lang)
	if streaming {
		close(proxy)
		<-forwarded
	}
	emitHeader() // 收尾轮没有任何正文时（空/出错），header 紧挨兜底摘要下发

	body := out.Content // 出错时可能是已推流的半截正文，保留以与流一致
	logger.Infof("%s finalize after max iterations: trace=%s finish_reason=%s content_len=%d ignored_tool_calls=%d err=%v",
		config.LogPrefix, loopTraceID(config.RequestID, req.Params), out.FinishReason, len(out.Content), len(out.ToolCalls), err)
	if err != nil || strings.TrimSpace(out.Content) == "" {
		fallback := progressSummary(resp.Steps, lang)
		if body != "" {
			fallback = "\n\n" + fallback
		}
		emit(fallback)
		body += fallback
	}

	resp.Content = header + body
	resp.contentStreamed = streaming
}

// progressSummary 收尾兜底：最后一段非空的模型评论 + 已调用工具的次数统计。
func progressSummary(steps []ToolStep, lang string) string {
	var lastThought string
	counts := map[string]int{}
	var errs int
	for _, s := range steps {
		if t := strings.TrimSpace(s.Thought); t != "" {
			lastThought = t
		}
		counts[s.Action]++
		if isErrorObservation(s.Observation) {
			errs++
		}
	}
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s×%d", n, counts[n]))
	}

	var b strings.Builder
	b.WriteString(LangText(lang,
		fmt.Sprintf("未能生成最终结论。已执行 %d 次工具调用（%s），其中 %d 次返回错误。", len(steps), strings.Join(parts, ", "), errs),
		fmt.Sprintf("No final conclusion was produced. %d tool calls were made (%s), %d of which returned errors.", len(steps), strings.Join(parts, ", "), errs)))
	if lastThought != "" {
		b.WriteString(LangText(lang, "\n\n最后的分析进展：\n\n", "\n\nLast progress:\n\n"))
		b.WriteString(lastThought)
	}
	b.WriteString(LangText(lang,
		"\n\n可以缩小时间范围或指定重点面板后重试。",
		"\n\nTry again with a narrower time range or by naming the panels to focus on."))
	return b.String()
}

func isErrorObservation(obs string) bool {
	return strings.HasPrefix(strings.TrimSpace(obs), "Error:")
}

// argsDigest 工具参数的短摘要（sha256 前 8 位 hex）：诊断日志据此区分"同名同参
// 重复调用"与"同名不同参"，又不把可能含敏感值的参数原文落日志。
func argsDigest(args string) string {
	sum := sha256.Sum256([]byte(args))
	return hex.EncodeToString(sum[:4])
}

// loopTraceID 诊断日志的关联 ID：优先 request_id（A2A 等），chat 链路没有
// request_id 时退化为 chat_id:seq_id，便于和 assistant 消息对上。
func loopTraceID(requestID string, params map[string]string) string {
	if requestID != "" {
		return requestID
	}
	if params["chat_id"] != "" {
		return params["chat_id"] + ":" + params["seq_id"]
	}
	return ""
}

// logLLMRound 每轮 LLM 返回后的脱敏诊断行。
func logLLMRound(config *ToolLoopConfig, trace string, iteration int, out nativeLLMResult) {
	names := make([]string, 0, len(out.ToolCalls))
	for _, tc := range out.ToolCalls {
		names = append(names, tc.Name)
	}
	logger.Infof("%s round: trace=%s iter=%d/%d finish_reason=%s content_len=%d reasoning_len=%d thinking_blocks=%d tool_calls=%d tools=[%s]",
		config.LogPrefix, trace, iteration+1, config.MaxIterations, out.FinishReason,
		len(out.Content), len(out.Reasoning), len(out.ThinkingBlocks), len(out.ToolCalls), strings.Join(names, ","))
}

// logToolCall 每次工具执行后的脱敏诊断行：只记参数摘要/长度与结果长度/是否报错。
func logToolCall(config *ToolLoopConfig, trace string, iteration int, tc llm.ToolCall, observation string, repeat int, cost time.Duration) {
	logger.Infof("%s tool: trace=%s iter=%d tool=%s args_sha=%s args_len=%d obs_len=%d is_error=%v repeat=%d cost_ms=%d",
		config.LogPrefix, trace, iteration+1, tc.Name, argsDigest(tc.Arguments), len(tc.Arguments),
		len(observation), isErrorObservation(observation), repeat, cost.Milliseconds())
}
