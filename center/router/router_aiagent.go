package router

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

// AIChatRequest is the generic chat request dispatched by action_key.
type AIChatRequest struct {
	ActionKey string                 `json:"action_key"` // e.g. "query_generator"
	UserInput string                 `json:"user_input"`
	History   []aiagent.ChatMessage  `json:"history,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"` // action-specific params
}

// actionHandler defines how each action_key is processed.
// The LLM agent config is always resolved via "chat" useCase in processAssistantMessage.
type actionHandler struct {
	description   string // human-readable description used by LLM intent inference
	validate      func(req *AIChatRequest) error
	selectTools   func(req *AIChatRequest) []string
	buildPrompt   func(req *AIChatRequest) string
	buildInputs   func(req *AIChatRequest) map[string]string
	parseResponse func(content string) []models.AssistantMessageResponse // split AI output into typed response elements
}

var actionRegistry = map[string]*actionHandler{
	"query_generator": {
		description:   "Generate PromQL or SQL queries based on user requirements; requires datasource context (datasource_type, datasource_id)",
		validate:      validateQueryGenerator,
		selectTools:   selectQueryGeneratorTools,
		buildPrompt:   buildQueryGeneratorPrompt,
		buildInputs:   buildQueryGeneratorInputs,
		parseResponse: parseQueryGeneratorResponse,
	},
	"general_chat": {
		description:   "General Q&A for operations, monitoring, observability, and other technical questions; no special context required",
		buildPrompt:   buildGeneralChatPrompt,
	},
}

func init() {
	if _, ok := actionRegistry[string(models.ActionKeyGeneralChat)]; !ok {
		panic("actionRegistry must contain general_chat as fallback")
	}
}

// --- query_generator action ---

func ctxStr(ctx map[string]interface{}, key string) string {
	if v, ok := ctx[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func ctxInt64(ctx map[string]interface{}, key string) int64 {
	if v, ok := ctx[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int64:
			return n
		case json.Number:
			i, _ := n.Int64()
			return i
		}
	}
	return 0
}

func validateQueryGenerator(req *AIChatRequest) error {
	dsType := ctxStr(req.Context, "datasource_type")
	dsID := ctxInt64(req.Context, "datasource_id")
	if dsType == "" {
		return fmt.Errorf("context.datasource_type is required")
	}
	if dsID == 0 {
		return fmt.Errorf("context.datasource_id is required")
	}
	return nil
}

func selectQueryGeneratorTools(req *AIChatRequest) []string {
	dsType := ctxStr(req.Context, "datasource_type")
	switch dsType {
	case "prometheus":
		return []string{"list_metrics", "get_metric_labels"}
	case "mysql", "doris", "ck", "clickhouse", "pgsql", "postgresql":
		return []string{"list_databases", "list_tables", "describe_table"}
	default:
		return nil
	}
}

func buildQueryGeneratorPrompt(req *AIChatRequest) string {
	dsType := ctxStr(req.Context, "datasource_type")
	dbName := ctxStr(req.Context, "database_name")
	tableName := ctxStr(req.Context, "table_name")

	switch dsType {
	case "prometheus":
		return fmt.Sprintf(`You are a PromQL expert. The user wants to query Prometheus metrics.

User request: %s

Please use the available tools to explore the metrics and generate the correct PromQL query.
- First use list_metrics to find relevant metrics
- Then use get_metric_labels to understand the label structure
- Finally provide the PromQL query as your Final Answer

Your Final Answer MUST be a valid JSON object with these fields:
{"query": "<the PromQL query>", "explanation": "<brief explanation in the user's language>"}`, req.UserInput)

	default: // SQL-based datasources
		dbContext := ""
		if dbName != "" {
			dbContext += fmt.Sprintf("\nTarget database: %s", dbName)
		}
		if tableName != "" {
			dbContext += fmt.Sprintf("\nTarget table: %s", tableName)
		}

		return fmt.Sprintf(`You are a SQL expert for %s databases. The user wants to query data.
%s
User request: %s

Please use the available tools to explore the database schema and generate the correct SQL query.
- Use list_databases to see available databases
- Use list_tables to see tables in the target database
- Use describe_table to understand the table structure
- Finally provide the SQL query as your Final Answer

Your Final Answer MUST be a valid JSON object with these fields:
{"query": "<the SQL query>", "explanation": "<brief explanation in the user's language>"}`, dsType, dbContext, req.UserInput)
	}
}

func buildQueryGeneratorInputs(req *AIChatRequest) map[string]string {
	inputs := map[string]string{
		"user_input": req.UserInput,
	}
	for _, key := range []string{"datasource_type", "datasource_id", "database_name", "table_name"} {
		if v := ctxStr(req.Context, key); v != "" {
			inputs[key] = v
		}
	}
	// datasource_id may be a number in JSON
	if inputs["datasource_id"] == "" {
		if id := ctxInt64(req.Context, "datasource_id"); id > 0 {
			inputs["datasource_id"] = fmt.Sprintf("%d", id)
		}
	}
	return inputs
}

// parseQueryGeneratorResponse parses the AI JSON output {"query":"...", "explanation":"..."}
// and splits it into a query element + a markdown element.
// Returns nil if parsing fails, so the caller can fall back to a single markdown element.
func parseQueryGeneratorResponse(content string) []models.AssistantMessageResponse {
	cleaned := stripCodeFence(strings.TrimSpace(content))

	var result struct {
		Query       string `json:"query"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil || result.Query == "" {
		return nil
	}

	resp := []models.AssistantMessageResponse{
		{ContentType: models.ContentTypeQuery, Content: result.Query, IsFinish: true, IsFromAI: true},
	}
	if result.Explanation != "" {
		resp = append(resp, models.AssistantMessageResponse{
			ContentType: models.ContentTypeMarkdown, Content: result.Explanation, IsFinish: true, IsFromAI: true,
		})
	}
	return resp
}

// stripCodeFence removes markdown code fences (```json ... ```) that LLMs sometimes wrap around JSON.
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Remove opening fence line
	if idx := strings.Index(s, "\n"); idx != -1 {
		s = s[idx+1:]
	}
	// Remove closing fence
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

// --- general_chat action ---

func buildGeneralChatPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are a helpful assistant specializing in IT operations, monitoring, observability, and general technical topics.
Please answer the user's question clearly and concisely in the user's language.

User request: %s`, req.UserInput)
}

// --- LLM intent inference ---

// buildIntentInferencePrompt constructs a system prompt that lists all available
// action keys with descriptions, asking the LLM to pick the best match.
func buildIntentInferencePrompt() string {
	var sb strings.Builder
	sb.WriteString("You are an intent classifier. Based on the user's message and conversation history, decide which action to take.\n\n")
	sb.WriteString("Available actions:\n")
	keys := make([]string, 0, len(actionRegistry))
	for key := range actionRegistry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		handler := actionRegistry[key]
		sb.WriteString(fmt.Sprintf("- %s: %s\n", key, handler.description))
	}
	sb.WriteString("\nRespond with ONLY a JSON object: {\"action_key\": \"<chosen_key>\"}\n")
	sb.WriteString("Do not include any other text.")
	return sb.String()
}

// inferActionKeyByLLM uses a lightweight LLM call to classify user intent
// into one of the registered action keys. Falls back to "general_chat" on error.
func inferActionKeyByLLM(ctx context.Context, llmClient llm.LLM, userInput string, history []aiagent.ChatMessage) string {
	// Optimisation: if only one handler is registered, skip inference.
	if len(actionRegistry) <= 1 {
		for key := range actionRegistry {
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

	if _, ok := actionRegistry[result.ActionKey]; !ok {
		return string(models.ActionKeyGeneralChat)
	}
	return result.ActionKey
}
