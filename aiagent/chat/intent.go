package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// buildIntentInferencePrompt constructs a system prompt that lists all available
// action keys with descriptions, asking the LLM to pick the best match.
func buildIntentInferencePrompt() string {
	var sb strings.Builder
	sb.WriteString(`You are an intent classifier for a monitoring system. Classify the user's message into exactly one action.

VERB-FIRST RULE — decide by the action verb before the noun:
- "创建/新建/加一条/添加/建一个/create/add/build" (构造新资源) → creation
- "查/查看/有哪些/列出/show/list" + resource nouns (告警规则、仪表盘、屏蔽、订阅、通知规则、机器、业务组等) → resource_query
- "查/分析" + alert events (告警、告警事件、活跃告警、历史告警) → alert_query
- "写/生成" + query language (PromQL/SQL/查询语句) → query_generator
- "排查/定位/诊断/根因分析/troubleshoot/debug/investigate" → troubleshooting
- 其它通用问答/knowledge → general_chat

Edge cases:
- "创建告警规则的步骤是什么" (asking HOW to, not DO) → general_chat
- "查一下最近创建的告警规则" (query, not create) → resource_query
- "这条告警为什么触发" (diagnosis, not query) → troubleshooting

Available actions:
`)
	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		handler := registry[key]
		sb.WriteString(fmt.Sprintf("- %s: %s\n", key, handler.Description))
	}
	sb.WriteString("\nRespond with ONLY a JSON object: {\"action_key\": \"<chosen_key>\"}\n")
	sb.WriteString("Do not include any other text.")
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

	resp, err := llm.ChatWithSystem(ctx, llmClient, systemPrompt, userMsg.String())
	if err != nil {
		logger.Warningf("[Assistant] intent inference failed: %v, falling back to general_chat", err)
		return string(models.ActionKeyGeneralChat)
	}

	cleaned := stripCodeFence(strings.TrimSpace(resp))
	var result struct {
		ActionKey string `json:"action_key"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return string(models.ActionKeyGeneralChat)
	}

	if _, ok := registry[result.ActionKey]; !ok {
		return string(models.ActionKeyGeneralChat)
	}
	return result.ActionKey
}
