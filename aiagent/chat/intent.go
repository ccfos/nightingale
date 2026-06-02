package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// buildIntentInferencePrompt constructs a system prompt that lists all available
// action keys with descriptions, asking the LLM to pick the best match.
//
// 路由规则按"是否需要产品官方文档作为权威依据"分层：n9e/categraf 特定事实题
// (字段名/指标名/Severity 命名/API/Header/Env) 优先走 doc_qa，纯 vendor-neutral
// 概念题才走 general_chat。否则裸 LLM 在 general_chat 路径上凭训练记忆答, 会
// 把 ping_result_code / Severity=Critical / [[inputs.xxx]] / Authorization Bearer
// 等相邻产品的"行业惯例"误用到 n9e 上。
func buildIntentInferencePrompt() string {
	var sb strings.Builder
	sb.WriteString("Classify the user's message into exactly ONE action below.\n\n")
	sb.WriteString("Actions:\n")
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		handler := registry[key]
		sb.WriteString(fmt.Sprintf("- %s: %s\n", key, handler.Description))
	}
	sb.WriteString(`
Routing rules (apply in priority order, first match wins):

1. PRODUCT-SPECIFIC FACTUAL questions → doc_qa
   The answer must match a specific identifier from the n9e/categraf/夜莺
   codebase or official docs. This OVERRIDES any "knowledge question" heuristic.
   Signals (any of):
   - asks for a specific metric / field / config name
     (e.g. ping_average_response_ms, omit_hostname, [[instances]] syntax,
      [http] section behavior, categraf_self metrics)
   - asks about Severity number ↔ English name mapping (1, 2, 3)
   - asks about a specific API path / port / HTTP header / env var
     (X-User-Token, /api/n9e/heartbeat, 17000, N9E_API_URL, Authorization)
   - asks "what does X mean / X 是什么 / X 代表什么" where X is a product term
     (业务组, 订阅规则, 屏蔽规则, edge 模式, [http] 段, target_info, …)
   - asks how a specific n9e mechanism works / 逻辑 / 原理 of a product feature
     (心跳判定逻辑, 失联检测原理, ingest 队列长度, batch / chan_size 默认值)
   - asks about categraf plugin existence / 默认行为 / supported inputs

2. PRODUCT OPERATIONS → matching specialized action_key
   - 创建 / 新建 / 添加 一个 NEW 资源 → creation
   - 修改 / 编辑 / 调整 / 更新 / 改阈值 / 改级别 / 启用 / 禁用 一个 EXISTING 资源 → edit
     (e.g. '把这条告警规则阈值改成20', '禁用这条规则', '这个告警对应的规则级别调成P1')
   - 查看 / 列出 → resource_query / alert_query
   - 实际跑查询拿数据 → datasource_query
   - 排查告警 / 诊断 → troubleshooting
   - 主机失联（具体一台机器、有 ident / 现象描述）→ host_health_diagnose
   - 主机新装没出现 → host_onboard_diagnose
   - 装 / 部署 categraf → agent_deploy_guide
   - 改通知通道 → notify_channel_copilot
   - 改通知模板 → notify_template_generator
   - 写自愈脚本 → task_tpl_copilot
   - 数据源连不上 → datasource_diagnose

3. GENERIC monitoring concepts (vendor-neutral, NO product-specific terms)
   → general_chat
   Examples: "什么是 P99 延迟", "PromQL vs MetricsQL 区别",
   "如何设计告警阈值", "黄金信号是什么", "histogram vs summary".
   If the same concept references n9e/categraf/夜莺 specifically
   (e.g. "夜莺里的 P99 怎么算", "categraf 的 histogram 怎么写"),
   it falls into rule 1 (doc_qa).

4. AMBIGUOUS between doc_qa and general_chat → choose doc_qa.
   doc_qa has search_n9e_docs + verify_answer for ground-truth check;
   general_chat answers from model memory and tends to hallucinate
   product-specific identifiers.
`)
	sb.WriteString("\nRespond with JSON only: {\"action_key\": \"<chosen_key>\"}")
	return sb.String()
}

// InferAction uses a lightweight LLM call to classify user intent
// into one of the registered action keys. Falls back to "general_chat" on error.
func InferAction(ctx context.Context, llmClient llm.LLM, userInput string, history []aiagent.ChatMessage) string {
	// Optimisation: if only one handler is registered, skip inference.
	if len(registry) <= 1 {
		for key := range registry {
			return key
		}
	}

	systemPrompt := buildIntentInferencePrompt()

	// Build user message with recent history context (last 4 turns max).
	var userMsg strings.Builder
	start := 0
	if len(history) > 4 {
		start = len(history) - 4
	}
	if len(history) > 0 {
		userMsg.WriteString("Recent conversation:\n")
		for _, h := range history[start:] {
			userMsg.WriteString(fmt.Sprintf("[%s]: %s\n", h.Role, h.Content))
		}
		userMsg.WriteString("\n")
	}
	userMsg.WriteString("Current user message: ")
	userMsg.WriteString(userInput)

	tStart := time.Now()
	resp, err := llm.ChatWithSystem(ctx, llmClient, systemPrompt, userMsg.String())
	llmDur := time.Since(tStart)
	if err != nil {
		logger.Warningf("[Assistant] intent inference failed after %dms: %v, falling back to general_chat", llmDur.Milliseconds(), err)
		return string(models.ActionKeyGeneralChat)
	}

	cleaned := stripCodeFence(strings.TrimSpace(resp))
	var result struct {
		ActionKey string `json:"action_key"`
	}
	chosen := string(models.ActionKeyGeneralChat)
	if err := json.Unmarshal([]byte(cleaned), &result); err == nil {
		if _, ok := registry[result.ActionKey]; ok {
			chosen = result.ActionKey
		}
	}
	logger.Infof("[Assistant.Timing] intent_infer llm_dur=%dms sys_prompt_len=%d user_prompt_len=%d action_key=%s",
		llmDur.Milliseconds(), len(systemPrompt), len(userMsg.String()), chosen)
	return chosen
}

// GeneralChatSubMode represents the secondary classification within general_chat:
// "knowledge" answers from model knowledge alone (Direct mode, no tools, tiny prompt),
// "data_query" needs read-only tools to query system state (ReAct mode, full toolset).
type GeneralChatSubMode string

const (
	GeneralChatSubModeKnowledge GeneralChatSubMode = "knowledge"
	GeneralChatSubModeDataQuery GeneralChatSubMode = "data_query"
)

const generalChatSubModePrompt = `You are routing inside the n9e (Nightingale) monitoring assistant's fallback channel.
Classify the user's message into exactly ONE bucket:

- knowledge: a concept/principle/"how-to" question answerable from general knowledge without querying any live system data.
  Examples: "什么是 P99 延迟", "PromQL 和 MetricsQL 区别", "如何设计告警阈值", "Prometheus histogram 怎么用".
- data_query: needs to look at the CURRENT state of this n9e instance to answer — alerts, hosts, rules, datasources, metrics values, log counts, etc.
  Examples: "现在有哪些告警", "查一下 ES 索引日志数量", "哪些机器掉线了", "数据源列表", "看下 CPU 使用率".

When ambiguous, prefer data_query (safer — tools available but may go unused).

Respond with JSON only: {"sub_mode": "knowledge"|"data_query"}`

// InferGeneralChatSubMode does a lightweight LLM classification to decide whether
// a general_chat turn should run in Direct mode (knowledge) or ReAct mode with
// tools (data_query). On any failure (timeout, parse error) it returns data_query
// to preserve fallback safety — the user still gets a useful answer, just at the
// cost of a larger system prompt that turn.
func InferGeneralChatSubMode(ctx context.Context, llmClient llm.LLM, userInput string, history []aiagent.ChatMessage) GeneralChatSubMode {
	// Mirror InferAction's recent-history window so multi-turn context survives
	// (e.g. user previously asked "查一下告警" then says "再看看 P0 的", which
	// in isolation looks like knowledge).
	var userMsg strings.Builder
	start := 0
	if len(history) > 4 {
		start = len(history) - 4
	}
	if len(history) > 0 {
		userMsg.WriteString("Recent conversation:\n")
		for _, h := range history[start:] {
			userMsg.WriteString(fmt.Sprintf("[%s]: %s\n", h.Role, h.Content))
		}
		userMsg.WriteString("\n")
	}
	userMsg.WriteString("Current user message: ")
	userMsg.WriteString(userInput)

	tStart := time.Now()
	resp, err := llm.ChatWithSystem(ctx, llmClient, generalChatSubModePrompt, userMsg.String())
	llmDur := time.Since(tStart)
	if err != nil {
		logger.Warningf("[Assistant] general_chat sub-mode classification failed after %dms: %v, defaulting to data_query", llmDur.Milliseconds(), err)
		return GeneralChatSubModeDataQuery
	}

	cleaned := stripCodeFence(strings.TrimSpace(resp))
	var result struct {
		SubMode string `json:"sub_mode"`
	}
	chosen := GeneralChatSubModeDataQuery
	if err := json.Unmarshal([]byte(cleaned), &result); err == nil {
		switch GeneralChatSubMode(result.SubMode) {
		case GeneralChatSubModeKnowledge:
			chosen = GeneralChatSubModeKnowledge
		case GeneralChatSubModeDataQuery:
			chosen = GeneralChatSubModeDataQuery
		}
	}
	logger.Infof("[Assistant.Timing] general_chat_submode_infer llm_dur=%dms sub_mode=%s",
		llmDur.Milliseconds(), chosen)
	return chosen
}
