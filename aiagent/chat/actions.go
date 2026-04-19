package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// ActionHandler defines how each action_key is processed.
// The LLM agent config is always resolved via "chat" useCase in the router.
//
// Execution order in the router's processAssistantMessage:
//  1. Validate — soft gate. On error, silently fall back to general_chat.
//  2. Preflight — hard gate. May emit structured responses and halt the turn
//     without running the agent (e.g. ask the user to pick a busi group
//     before a creation flow). Returns halt=true to stop; halt=false to proceed.
//  3. SelectTools / BuildPrompt — configure the agent for this action.
type ActionHandler struct {
	Description   string // human-readable description used by LLM intent inference
	Validate      func(req *AIChatRequest) error
	Preflight     func(ctx context.Context, deps *aiagent.ToolDeps, req *AIChatRequest, user *models.User) (halt bool, resps []models.AssistantMessageResponse, err error)
	SelectTools   func(req *AIChatRequest) []string
	BuildPrompt   func(req *AIChatRequest) string
	BuildInputs   func(req *AIChatRequest) map[string]string
	ParseResponse func(content string) []models.AssistantMessageResponse // split AI output into typed response elements
}

var registry = map[string]*ActionHandler{
	"query_generator": {
		Description:   "Generate PromQL or SQL query statements (编写查询语句). Examples: '帮我写个查CPU的PromQL', '生成一个查询订单表的SQL', 'write a PromQL for memory usage'",
		Validate:      validateQueryGenerator,
		SelectTools:   selectQueryGeneratorTools,
		BuildPrompt:   buildQueryGeneratorPrompt,
		BuildInputs:   buildQueryGeneratorInputs,
		ParseResponse: parseQueryGeneratorResponse,
	},
	"general_chat": {
		Description: "General Q&A and other questions (通用问答). Examples: '什么是P99延迟', 'Prometheus和VictoriaMetrics有什么区别', '如何优化慢查询'",
		SelectTools: selectAllBuiltinTools,
		BuildPrompt: buildGeneralChatPrompt,
	},
	"alert_query": {
		Description: "Query and analyze alert events (查询告警事件). Examples: '最近1小时有哪些告警', '当前有多少P1告警', '查看活跃告警', '历史告警统计', '告警ID 123的详情'",
		SelectTools: selectAlertQueryTools,
		BuildPrompt: buildAlertQueryPrompt,
	},
	"resource_query": {
		Description: "Query monitoring system resources and configurations (查询监控系统资源配置). Examples: '我有哪些业务组', '查看告警规则列表', '有哪些机器', '仪表盘列表', '屏蔽规则', '订阅规则', '自愈脚本', '通知规则', '数据源列表', '用户列表', '团队列表'",
		SelectTools: selectResourceQueryTools,
		BuildPrompt: buildResourceQueryPrompt,
	},
	"creation": {
		Description: "Create or add NEW monitoring resources (创建/新建资源). Trigger verbs: 创建/新建/加一条/添加/建一个/create/add/build. Scope: alert rules, dashboards, alert mutes, alert subscribes, notify rules. Examples: '创建一条 CPU 告警', '新建一个仪表盘', '给这条告警加屏蔽', '添加一个订阅规则', '创建通知规则'. NOTE: queries like '查看告警规则', '有哪些仪表盘' are resource_query, NOT creation.",
		Preflight:   PreflightCreation,
		SelectTools: selectCreationTools,
		BuildPrompt: buildCreationPrompt,
		BuildInputs: buildCreationInputs,
	},
	"troubleshooting": {
		Description: "Troubleshoot incidents, diagnose alerts, analyze root causes (故障排查/根因分析). Examples: '这条告警为什么触发', '帮我分析一下刚才的故障', '排查一下 CPU 飙高的原因', 'troubleshoot this incident'",
		SelectTools: selectTroubleshootingTools,
		BuildPrompt: buildTroubleshootingPrompt,
	},
	"notify_template_generator": {
		Description: "Generate or modify Go templates for alert notification messages (告警通知消息模板). Examples: '告警内容里加主机名', '把 trigger_value 保留两位小数', '钉钉模板 at 告警接收人', '生成一个飞书卡片模板', 'add hostname to notification template'",
		BuildPrompt: buildNotifyTemplatePrompt,
	},
	"datasource_diagnose": {
		Description: "Diagnose datasource connectivity or configuration errors (数据源连通性/配置诊断). Examples: 'ES 报 x509 证书错误怎么处理', 'VictoriaMetrics 的 url 怎么写', '数据源测试连通 401 是什么原因', 'timeout 连不上 Loki', 'clickhouse 对接夜莺'",
		SelectTools: selectDatasourceDiagnoseTools,
		BuildPrompt: buildDatasourceDiagnosePrompt,
	},
}

func init() {
	if _, ok := registry[string(models.ActionKeyGeneralChat)]; !ok {
		panic("chat.registry must contain general_chat as fallback")
	}
}

// Lookup returns the handler for the given action key, or (nil, false) if absent.
func Lookup(key string) (*ActionHandler, bool) {
	h, ok := registry[key]
	return h, ok
}

// Keys returns every registered action key. Order is not guaranteed.
func Keys() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
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
	if id := ctxInt64(req.Context, "datasource_id"); id > 0 {
		ctxHint.WriteString(fmt.Sprintf("\nUser-selected datasource_id: %d (use this as datasource_id; do NOT call list_datasources)", id))
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

// buildCreationInputs forwards preflight-selected context (busi_group_id,
// datasource_id, team_ids) to the agent as tool params, so tools like
// create_alert_rule / create_dashboard can read them via getDatasourceId etc.
// without relying on the LLM to thread them through arguments.
func buildCreationInputs(req *AIChatRequest) map[string]string {
	inputs := map[string]string{}
	if id := ctxInt64(req.Context, "busi_group_id"); id > 0 {
		inputs["busi_group_id"] = fmt.Sprintf("%d", id)
	}
	if id := ctxInt64(req.Context, "datasource_id"); id > 0 {
		inputs["datasource_id"] = fmt.Sprintf("%d", id)
	}
	if ids := ctxInt64Slice(req.Context, "team_ids"); len(ids) > 0 {
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = fmt.Sprintf("%d", id)
		}
		inputs["team_ids"] = strings.Join(parts, ",")
	}
	return inputs
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

// --- notify_template_generator action ---
//
// Focused assistant for writing/modifying alert notification templates. The
// substantive knowledge (event fields, helper funcs, channel differences,
// worked examples) lives in the n9e-generate-message-template skill's
// SKILL.md — the skill selector auto-loads it based on the skill description.
// This inline prompt just frames the task and points at the skill.

func buildNotifyTemplatePrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are a Nightingale (n9e) notification template expert. The user wants to author or modify an alert message template rendered by the n9e template engine.

User request: %s

Follow the n9e-generate-message-template skill (SKILL.md) exactly. Core expectations:
- Output structure: one-line lead → fenced gotemplate code block → short "变量说明" list.
- Always split $event.IsRecovered branches unless the user explicitly opts out.
- Prefer $labels.<key> over $event.TagsJSON for tag lookups.
- Wrap all unix timestamps through timeformat; format numeric values via formatDecimal when precision is requested.
- Respond in the user's language; don't ask clarifying questions if a reasonable assumption will do — state the assumption in the lead instead.`, req.UserInput)
}

// --- datasource_diagnose action ---

func selectDatasourceDiagnoseTools(req *AIChatRequest) []string {
	return []string{
		"list_datasources", "get_datasource_detail",
	}
}

func buildDatasourceDiagnosePrompt(req *AIChatRequest) string {
	var ctxHint strings.Builder
	if id := ctxInt64(req.Context, "datasource_id"); id > 0 {
		ctxHint.WriteString(fmt.Sprintf("\nCurrent page datasource_id: %d — start by calling get_datasource_detail on it.", id))
	}
	if t := ctxStr(req.Context, "datasource_type"); t != "" {
		ctxHint.WriteString(fmt.Sprintf("\nCurrent page datasource_type: %s", t))
	}
	return fmt.Sprintf(`You are a datasource connectivity troubleshooter for Nightingale (n9e). Users typically paste an error message or describe a connection failure, and you diagnose the cause.

User request: %s%s

Diagnostic checklist — walk through the layers top-down:
1. URL format
   - Prometheus/VictoriaMetrics: "http://host:port" (NO trailing path; n9e appends /api/v1/query itself).
   - Loki: "http://host:3100" (the /loki/api/v1 suffix is appended).
   - Elasticsearch/OpenSearch: "http://host:9200" (single node) or comma-joined list.
   - ClickHouse HTTP: "http://host:8123"; native TCP: "tcp://host:9000" depending on the driver.
   - TDengine: "http://host:6041" (RESTful) — NOT 6030.
   - Common mistake: double slashes, trailing /api/v1/query, or pasting the UI URL.
2. TLS / certificates
   - "x509: certificate signed by unknown authority" → either import the CA into the n9e host trust store, or toggle "Skip TLS Verify" in the datasource config.
   - "tls: failed to verify certificate" on ES 8.x → ES 8 enables TLS by default; switch scheme to https and either provide CA or skip verify.
3. Authentication
   - 401/403 → basic auth user/password, or missing bearer token.
   - ES 8.x API Key vs user/password; Prometheus remote-write tenant headers.
4. Network
   - "connection refused" → wrong port, service not listening on that interface.
   - "dial tcp ... i/o timeout" → firewall, security group, or wrong IP.
   - "no route to host" → routing.
5. Version compatibility
   - ES client vs server major version mismatch.
   - VictoriaMetrics cluster vs single — vmselect uses "/select/0/prometheus" prefix.

When you have enough context, use the tools to inspect existing datasource configs:
- list_datasources — browse what's configured.
- get_datasource_detail — inspect one datasource's full config (URL, auth, timeout, skip_tls_verify).

Output:
- Your Final Answer MUST be Markdown (NOT JSON), in the user's language.
- Structure: "可能原因" → "验证命令" (curl/telnet) → "修复建议".
- Always include at least one verification curl command the user can run to confirm the fix.`, req.UserInput, ctxHint.String())
}
