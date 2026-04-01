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
		description:   "Generate PromQL or SQL query statements (编写查询语句). Examples: '帮我写个查CPU的PromQL', '生成一个查询订单表的SQL', 'write a PromQL for memory usage'",
		validate:      validateQueryGenerator,
		selectTools:   selectQueryGeneratorTools,
		buildPrompt:   buildQueryGeneratorPrompt,
		buildInputs:   buildQueryGeneratorInputs,
		parseResponse: parseQueryGeneratorResponse,
	},
	"general_chat": {
		description: "General Q&A and other questions (通用问答). Examples: '什么是P99延迟', 'Prometheus和VictoriaMetrics有什么区别', '如何优化慢查询'",
		selectTools: selectAllBuiltinTools,
		buildPrompt: buildGeneralChatPrompt,
	},
	"alert_query": {
		description:   "Query and analyze alert events (查询告警事件). Examples: '最近1小时有哪些告警', '当前有多少P1告警', '查看活跃告警', '历史告警统计', '告警ID 123的详情'",
		selectTools:   selectAlertQueryTools,
		buildPrompt:   buildAlertQueryPrompt,
	},
	"resource_query": {
		description: "Query monitoring system resources and configurations (查询监控系统资源配置). Examples: '我有哪些业务组', '查看告警规则列表', '有哪些机器', '仪表盘列表', '屏蔽规则', '订阅规则', '自愈脚本', '通知规则', '数据源列表', '用户列表', '团队列表'",
		selectTools: selectResourceQueryTools,
		buildPrompt: buildResourceQueryPrompt,
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

func selectAllBuiltinTools(req *AIChatRequest) []string {
	defs := aiagent.GetAllBuiltinToolDefs()
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

func buildGeneralChatPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are a helpful assistant specializing in IT operations, monitoring, observability, and general technical topics.
Please answer the user's question clearly and concisely in the user's language.

User request: %s`, req.UserInput)
}

// --- alert_query action ---

func selectAlertQueryTools(req *AIChatRequest) []string {
	return []string{"search_active_alerts", "search_history_alerts", "get_alert_event_detail"}
}

func buildAlertQueryPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are an alert analysis expert for a monitoring system. The user wants to query or analyze alert events.

User request: %s

Tool selection strategy:
- By DEFAULT, use search_active_alerts to query currently active (unrecovered) alerts
- ONLY use search_history_alerts when the user explicitly mentions historical/past/recovered alerts (e.g. "历史告警", "已恢复", "过去的告警")
- Use get_alert_event_detail to get full details of a specific alert event
- Severity levels: 1=Critical, 2=Warning, 3=Info

IMPORTANT: Your Final Answer MUST be in well-formatted Markdown (NOT JSON). Use the user's language. Structure your response like this:

## 告警概览
- 总数、活跃数、已恢复数
- 按级别分布

## 告警详情
Use a markdown table to list the alerts:
| 告警规则 | 级别 | 触发对象 | 触发时间 | 状态 |

## 分析与建议
- Notable patterns
- Recommendations`, req.UserInput)
}

// --- resource_query action ---

func selectResourceQueryTools(req *AIChatRequest) []string {
	return []string{
		"list_alert_rules", "get_alert_rule_detail",
		"list_targets", "get_target_detail",
		"list_dashboards", "get_dashboard_detail",
		"list_alert_mutes", "get_alert_mute_detail",
		"list_alert_subscribes", "get_alert_subscribe_detail",
		"list_task_tpls", "get_task_tpl_detail",
		"list_notify_rules", "get_notify_rule_detail",
		"list_datasources", "get_datasource_detail",
		"list_users",
		"list_teams",
		"list_busi_groups",
	}
}

func buildResourceQueryPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are a monitoring system assistant. The user wants to query system resources or configurations.

User request: %s

Choose the appropriate tool based on the user's question:
- Alert rules (告警规则): list_alert_rules / get_alert_rule_detail
- Targets/hosts (机器/主机): list_targets / get_target_detail
- Dashboards (仪表盘): list_dashboards / get_dashboard_detail
- Alert mutes (屏蔽规则): list_alert_mutes / get_alert_mute_detail
- Alert subscribes (订阅规则): list_alert_subscribes / get_alert_subscribe_detail
- Task templates (自愈脚本): list_task_tpls / get_task_tpl_detail
- Notify rules (通知规则): list_notify_rules / get_notify_rule_detail
- Datasources (数据源): list_datasources / get_datasource_detail
- Users (用户): list_users
- Teams (团队): list_teams
- Business groups (业务组): list_busi_groups

Use the list tool first for browsing. Use the detail tool when the user asks about a specific item by ID or name.
If a tool returns a "forbidden" error, inform the user they don't have permission.

IMPORTANT: Your Final Answer MUST be in well-formatted Markdown (NOT JSON). Use the user's language. Use tables for list results.`, req.UserInput)
}

// --- LLM intent inference ---

// buildIntentInferencePrompt constructs a system prompt that lists all available
// action keys with descriptions, asking the LLM to pick the best match.
func buildIntentInferencePrompt() string {
	var sb strings.Builder
	sb.WriteString(`You are an intent classifier for a monitoring system. Classify the user's message into exactly one action.

Key distinction:
- "告警事件/alert events" related messages (查告警、告警数量、活跃告警、历史告警) → alert_query
- "写查询/生成查询/PromQL/SQL" related messages (编写查询语句) → query_generator
- "查询资源/配置" related messages (告警规则、机器、仪表盘、屏蔽规则、订阅规则、自愈脚本、通知规则、数据源、用户、团队、业务组) → resource_query
- General knowledge questions or other topics → general_chat

Available actions:
`)
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
