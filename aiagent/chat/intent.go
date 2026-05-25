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
// Kept deliberately short — the handler Descriptions in actions.go already
// encode the trigger verbs and examples per action, so re-stating them here as
// a "VERB-FIRST RULE" block was largely duplicative. We only keep the one
// disambiguation rule that the Descriptions cannot express naturally:
// knowledge-style questions ("how to / what is") should fall through to
// general_chat instead of the matching action.
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
	sb.WriteString("\nRule: knowledge questions (\"how to ...\", \"什么是\", \"...的步骤\") → general_chat, even if they mention a resource.\n")
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
