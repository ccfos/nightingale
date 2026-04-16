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
//
// Execution order in processAssistantMessage:
//  1. validate — soft gate. On error, silently fall back to general_chat.
//  2. preflight — hard gate. May emit structured responses and halt the turn
//     without running the agent (e.g. ask the user to pick a busi group
//     before a creation flow). Returns halt=true to stop; halt=false to proceed.
//  3. selectTools / buildPrompt — configure the agent for this action.
type actionHandler struct {
	description   string // human-readable description used by LLM intent inference
	validate      func(req *AIChatRequest) error
	preflight     func(ctx context.Context, req *AIChatRequest, user *models.User) (halt bool, resps []models.AssistantMessageResponse, err error)
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
	"creation": {
		description: "Create or add NEW monitoring resources (创建/新建资源). Trigger verbs: 创建/新建/加一条/添加/建一个/create/add/build. Scope: alert rules, dashboards, alert mutes, alert subscribes, notify rules. Examples: '创建一条 CPU 告警', '新建一个仪表盘', '给这条告警加屏蔽', '添加一个订阅规则', '创建通知规则'. NOTE: queries like '查看告警规则', '有哪些仪表盘' are resource_query, NOT creation.",
		preflight:   preflightCreation,
		selectTools: selectCreationTools,
		buildPrompt: buildCreationPrompt,
	},
	"troubleshooting": {
		description: "Troubleshoot incidents, diagnose alerts, analyze root causes (故障排查/根因分析). Examples: '这条告警为什么触发', '帮我分析一下刚才的故障', '排查一下 CPU 飙高的原因', 'troubleshoot this incident'",
		selectTools: selectTroubleshootingTools,
		buildPrompt: buildTroubleshootingPrompt,
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
	case "mysql", "doris", "ck", "clickhouse", "pgsql", "postgresql", "tdengine":
		return []string{"list_databases", "list_tables", "describe_table"}
	case "loki", "elasticsearch", "opensearch", "victorialogs":
		// Log datasources don't have a first-class schema introspection tool
		// (labels/fields vary by stream/index); leave the agent without extra
		// tools — it writes the DSL directly from the user's natural-language
		// intent, which is how users do it today anyway.
		return nil
	default:
		return nil
	}
}

// isLogDatasource identifies datasources whose native query language is log-
// oriented (LogQL / ES DSL / SPL-like). Used to branch the query_generator
// prompt into a log-specific template.
func isLogDatasource(dsType string) bool {
	switch dsType {
	case "loki", "elasticsearch", "opensearch", "victorialogs":
		return true
	}
	return false
}

func buildQueryGeneratorPrompt(req *AIChatRequest) string {
	dsType := ctxStr(req.Context, "datasource_type")
	dbName := ctxStr(req.Context, "database_name")
	tableName := ctxStr(req.Context, "table_name")

	switch {
	case dsType == "prometheus":
		return fmt.Sprintf(`You are a PromQL expert. The user wants to query Prometheus metrics.

User request: %s

Please use the available tools to explore the metrics and generate the correct PromQL query.
- First use list_metrics to find relevant metrics
- Then use get_metric_labels to understand the label structure
- Finally provide the PromQL query as your Final Answer

Your Final Answer MUST be a valid JSON object with these fields:
{"query": "<the PromQL query>", "explanation": "<brief explanation in the user's language>"}`, req.UserInput)

	case isLogDatasource(dsType):
		// Log query generation. Each engine has its own syntax:
		//   loki → LogQL (`{app="foo"} |= "error" | json`)
		//   elasticsearch / opensearch → query_string or KQL-style
		//   victorialogs → LogsQL
		// We don't have schema introspection tools here (labels/fields are
		// stream/index specific); the LLM writes directly from the user's
		// natural-language intent.
		langHint := "the native log query language of this datasource"
		switch dsType {
		case "loki":
			langHint = "LogQL (Loki). Pipeline syntax like `{app=\"foo\"} |= \"error\" | json | status=500`"
		case "elasticsearch", "opensearch":
			langHint = "Elasticsearch query_string (or Lucene) syntax, e.g. `level:ERROR AND service:checkout`"
		case "victorialogs":
			langHint = "VictoriaLogs LogsQL, e.g. `_time:5m AND app:order-svc AND level:error`"
		}
		return fmt.Sprintf(`You are a log query expert. The user wants to search logs stored in %s.

Target datasource type: %s
Target query language: %s

User request: %s

Generate a correct log query that matches the user's intent. Favor concise, production-safe queries:
- Always scope by time range when the user implies one (last N minutes/hours).
- Add filters rather than regex scans whenever possible.
- For structured logs, suggest label/field filters before full-text scan.
- If the user's intent is ambiguous, make a reasonable assumption and state it in the explanation.

Your Final Answer MUST be a valid JSON object with these fields:
{"query": "<the log query>", "explanation": "<brief explanation in the user's language>"}`, dsType, dsType, langHint, req.UserInput)

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

// --- creation action ---

// selectCreationTools is the union of builtin_tools declared by the functional
// n9e-create-* skills (alert-rule, dashboard). The non-tool-backed creation
// skills (alert-mute, alert-subscribe, notify-rule) rely on HTTP flows and
// don't contribute to this list. list_* tools are included so the agent can
// resolve names → IDs when the user refers to groups/datasources by name.
func selectCreationTools(req *AIChatRequest) []string {
	return []string{
		"create_alert_rule",
		"create_dashboard",
		"list_busi_groups",
		"list_datasources",
		"list_metrics",
		"get_metric_labels",
		"list_notify_rules",
		"list_files",
		"read_file",
		"grep_files",
		"list_databases",
		"list_tables",
		"describe_table",
	}
}

func buildCreationPrompt(req *AIChatRequest) string {
	// busi_group_id and team_ids are injected by the frontend after the
	// preflight selector. Surface them to the LLM so it doesn't re-ask.
	var ctxHint strings.Builder
	if id := ctxInt64(req.Context, "busi_group_id"); id > 0 {
		ctxHint.WriteString(fmt.Sprintf("\nUser-selected busi_group_id: %d (use this as group_id; do NOT call list_busi_groups)", id))
	}
	if v, ok := req.Context["team_ids"]; ok {
		ctxHint.WriteString(fmt.Sprintf("\nUser-selected team_ids: %v", v))
	}

	return fmt.Sprintf(`You are a monitoring system assistant helping the user CREATE a new resource in Nightingale (n9e).

User request: %s%s

Pick the correct creation skill based on the user's intent and follow its SKILL.md:
- Alert rule (告警规则): n9e-create-alert-rule skill → use create_alert_rule tool
- Dashboard (仪表盘): n9e-create-dashboard skill → use create_dashboard tool
- Alert mute (屏蔽规则): n9e-create-alert-mute skill
- Alert subscribe (订阅规则): n9e-create-alert-subscribe skill
- Notify rule (通知规则): n9e-create-notify-rule skill

Guidelines:
- If the request maps to multiple skills (e.g. "创建一个仪表盘和告警"), do them one at a time and confirm each.
- If critical parameters are missing, ask the user concisely in their language instead of guessing.
- After a successful creation, keep the Final Answer short (one sentence). Structured result cards are rendered separately by the UI.`, req.UserInput, ctxHint.String())
}

// --- troubleshooting action ---

func selectTroubleshootingTools(req *AIChatRequest) []string {
	return []string{
		"search_active_alerts", "search_history_alerts", "get_alert_event_detail",
		"list_alert_rules", "get_alert_rule_detail",
		"list_datasources", "get_datasource_detail",
		"list_metrics", "get_metric_labels",
		"query_prometheus", "query_timeseries", "query_log",
		"list_databases", "list_tables", "describe_table",
		"list_targets", "get_target_detail",
		"list_dashboards", "get_dashboard_detail",
		"list_busi_groups",
	}
}

func buildTroubleshootingPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are a senior SRE troubleshooting an incident on the Nightingale (n9e) monitoring platform.

User request: %s

Follow the ops-troubleshooting skill (SKILL.md) exactly. Core principles:
- Evidence-driven: every inference must be backed by alerts, metrics, logs, or target data.
- Query on demand, don't bulk-pull; keep time ranges tight.
- Work forward from the timeline: find the anomaly starting point, then trace up/downstream.
- Focus on direct cause + mitigation, not exhaustive root-cause coverage.

IMPORTANT: Your Final Answer MUST be in well-formatted Markdown (NOT JSON). Use the user's language. Include: timeline, evidence, likely cause, suggested mitigation.`, req.UserInput)
}

// --- LLM intent inference ---

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
