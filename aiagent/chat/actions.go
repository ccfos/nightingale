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
//  1. Preflight — hard gate. May emit structured responses and halt the turn
//     without running the agent (e.g. ask the user to pick a busi group
//     before a creation flow). Returns halt=true to stop; halt=false to proceed.
//  2. SelectTools / BuildPrompt — configure the agent for this action.
//
// registry 收缩后仅存对话路径实际可达的 general_chat（默认）与 creation
// （创建动词 fast-path）两个 action。历史上的专用 action（query_generator /
// edit / troubleshooting / doc_qa 等）连同 Validate / BuildInputs /
// ParseResponse / AgentMode 等仅它们使用的扩展点已整体删除——能力由
// general_chat 的工具全集 + 技能目录（load_skill 自取）承接；未来非对话
// 场景（如嵌入式 NL→PromQL）需要专用配置时再按需加回。
type ActionHandler struct {
	Preflight   func(ctx context.Context, deps *aiagent.ToolDeps, req *AIChatRequest, user *models.User) (halt bool, resps []models.AssistantMessageResponse, err error)
	SelectTools func(req *AIChatRequest) []string
	BuildPrompt func(req *AIChatRequest) string

	// RequiredSkills 声明本 action 路径上确定性预载的 skill 名列表。
	// 非 nil 时 router 用它覆盖 agent 默认 SkillConfig。静态映射用 pinSkills 简写。
	//
	// 返回空表示"本次不预载任何 skill"（注意不同于"未声明此字段"——未声明走 agent 默认配置）。
	RequiredSkills func(req *AIChatRequest) []string
}

// pinSkills 返回固定 skill 列表的 RequiredSkills——action→skill 静态 1:1（或 1:N）
// 映射的简写。零参调用 pinSkills() 表示"显式不预载"。
func pinSkills(names ...string) func(*AIChatRequest) []string {
	return func(*AIChatRequest) []string { return names }
}

var registry = map[string]*ActionHandler{
	// general_chat：开放输入的默认 action（路由收缩后的主路径）。
	"general_chat": {
		BuildPrompt: buildGeneralChatPrompt,
		// 挂全部内置工具走 ReAct，开放输入也能真正调用工具（查数据源、查告警、
		// 查资源、带门写操作等），而不是只能凭模型常识空答。
		// 显式不预载任何 skill（防 agent 绑定的 skill 膨胀默认路径 prompt）；
		// 目录 + load_skill 自取路径仍然可用。
		SelectTools:    selectGeneralChatTools,
		RequiredSkills: pinSkills(),
	},
	// creation：创建/新建监控资源（告警规则、仪表盘、屏蔽、订阅、通知规则）。
	// 创建动词 fast-path 的目标——保住零 LLM 即时弹业务组表单的 UX。
	"creation": {
		Preflight:   PreflightCreation,
		SelectTools: selectCreationTools,
		BuildPrompt: buildCreationPrompt,
		// busi_group_id/datasource_id/team_ids 经 router 默认的 ContextForwardInputs
		// 转发抵达工具层。
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

// --- 响应兜底处理 ---

// UnwrapJSONEnvelope handles the case where an LLM mistakenly wraps a markdown
// final answer in a JSON object like {"query": "## 结论\n..."} — the front-end
// then renders the raw JSON with literal "\n" escapes, which looks like garbled
// output. When `content` is such an envelope, returns the extracted body string;
// otherwise returns `content` unchanged.
//
// Heuristic: the extracted value must contain a real newline OR a markdown
// header marker ("## "), so we don't mis-unwrap a single-line value like
// `{"query": "up == 0"}`.
func UnwrapJSONEnvelope(content string) string {
	cleaned := stripCodeFence(strings.TrimSpace(content))
	if !strings.HasPrefix(cleaned, "{") || !strings.HasSuffix(cleaned, "}") {
		return content
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &obj); err != nil {
		return content
	}
	for _, key := range []string{"answer", "content", "markdown", "text", "result", "query"} {
		v, ok := obj[key].(string)
		if !ok || v == "" {
			continue
		}
		if strings.Contains(v, "\n") || strings.Contains(v, "## ") {
			return v
		}
	}
	return content
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
	return fmt.Sprintf(`You are a helpful assistant for the n9e (Nightingale) monitoring platform, specializing in IT operations, monitoring, and observability.

When the user asks what you can do, summarize from the "Available Skills" catalog in your system instructions (plus your read/query/create/update tools) in the user's language — the catalog is the authoritative capability list.

DOMAIN LIMIT (STRICT):
ONLY answer questions in these domains:
  - n9e (Nightingale) usage, configuration, features
  - IT operations / SRE / DevOps practice
  - Alerting (rules, notifications, on-call, suppression, escalation)
  - Observability — metrics, logs, traces, distributed tracing
  - Related infrastructure (Prometheus, Grafana, Loki, ClickHouse, ElasticSearch, OpenSearch, VictoriaMetrics, etc.)
  - Performance / capacity / troubleshooting in the above contexts

If the user asks anything outside these domains (general programming help unrelated to ops, life advice, news, personal questions, creative writing, math homework, etc.), politely decline in the user's language and remind them this assistant is specialized for n9e and observability topics. Do NOT attempt to answer such questions even partially.

This is the default general agent path — handle a broad range of (in-domain) requests yourself, loading a matching skill from the Available Skills catalog when one clearly applies.

Tool usage policy (IMPORTANT — pick the right tool, don't fan out):

1. **Vendor-neutral concept questions** (e.g. "什么是 P99", "PromQL 和 MetricsQL 区别", "histogram vs summary", "黄金信号", "alert fatigue"): answer DIRECTLY from your own knowledge. Do NOT call tools. These don't require product-specific facts.

2. **n9e / categraf / 夜莺 SPECIFIC FACTUAL questions** (e.g. "ping 监控对应的 promql", "Severity 1 是什么", "config.toml http 段作用", "categraf_self 指标怎么获取", "Token 调接口 401 怎么回事", "[http] 段是不是给 Prometheus 抓的"): MUST call **search_n9e_docs** first to get authoritative facts. The doc index contains real toml samples + V9 documentation. NEVER answer such questions from memory — you have a documented history of hallucinating field names / metric names / Severity mappings / Header names when answering without doc retrieval (Severity=Critical / Authorization: Bearer / ping_result_code / [[inputs.xxx]] / enable_queue / N9E_ADDR ... 这些都是历史翻车样本)。

   When search_n9e_docs returns `+"`quality: empty`"+` or `+"`must_refuse: true`"+`, you MUST NOT invent specific identifiers — instead reply with the refusal template (see below) and offer vendor-neutral concept guidance.

3. **CURRENT system state / data queries** (e.g. "我现在有哪些告警", "查一下 ES 索引日志数", "哪些机器掉线了", "datasource 列表"): use the appropriate read tool to fetch real data. Prefer the most specific tool. Don't fan-out.
   - Alert events: default to search_active_alerts (currently firing). ONLY use search_history_alerts when the user explicitly asks about historical/past/recovered alerts (历史告警/已恢复/过去的告警). Severity levels: 1=Critical, 2=Warning, 3=Info.
   - Resource/config listing (告警规则/机器/仪表盘/屏蔽/订阅/通知规则/用户/团队/业务组): use the matching list_* tool first for browsing, the get_*_detail tool for a specific item. If a tool returns "forbidden", tell the user they lack permission.

4. **For datasource data queries**: list_datasources first if datasource not specified; then pick by plugin_type — prometheus → query_prometheus (PromQL); mysql/ck/pgsql/doris/tdengine → query_timeseries (aggregations) or query_log (raw rows); elasticsearch/opensearch/victorialogs → query_timeseries (counts) or query_log (raw docs). For SQL-class datasources use list_databases / list_tables / describe_table first if schema unknown. Default time_range = 1h unless specified.

5. **Creation / modification requests** (创建/新建/修改/编辑 告警规则、仪表盘、通知/屏蔽/订阅规则等): you CAN do these directly with the write tools (create_* / import_*_template / update_alert_rule / update_dashboard / update_notify_rule / update_alert_mute / update_alert_subscribe). Before acting, load the matching skill from the Available Skills catalog (e.g. n9e-create-dashboard) and follow its workflow. If the business group is unknown, just call the create tool — it will pause and ask the user with a structured form; do NOT pick a business group on the user's behalf. Modifications go through a propose→confirm gate automatically.

Refusal template (use VERBATIM when search_n9e_docs is empty for a factual question):
`+"```"+`
我在 n9e/categraf 官方文档里没有找到关于 <用户问题里的关键名词> 的明确描述，
为避免给你错误信息，我不直接回答这个问题。建议:

1. 📖 到 https://flashcat.cloud/docs/ 切换版本手动查
2. 🐛 到 https://github.com/ccfos/nightingale/issues 搜历史问答
3. 💬 加夜莺社区群直接问研发

[可选: 这里附一段 vendor-neutral 的概念引导, 不带任何 n9e/categraf 特定标识符]
`+"```"+`

Answer in the user's language. Use well-formatted Markdown.

User request: %s`, req.UserInput)
}

// --- creation action ---

// selectCreationTools is the union of builtin_tools declared by the functional
// n9e-create-* skills. alert-rule/dashboard 以外，notify-rule/alert-mute/
// alert-subscribe 也都已有对应的 create_* 工具（带 busi_group_id / team_ids 缺参门），
// 一并挂上，创建快路径就能闭环落库，而不再依赖外部 A2A 的 HTTP flow。list_* 工具用于
// 在用户以名字指代业务组/数据源/通知规则时解析 名字 → ID。
func selectCreationTools(req *AIChatRequest) []string {
	return []string{
		"create_alert_rule",
		"import_alert_rule_template",
		"preview_alert_rule_template",
		"create_dashboard",
		"import_dashboard_template",
		"create_notify_rule",
		"create_alert_mute",
		"create_alert_subscribe",
		"list_busi_groups",
		"list_datasources",
		"list_metrics",
		"get_metric_labels",
		"list_notify_rules",
		"list_notify_channels",
		"list_message_templates",
		"list_notify_rule_custom_params",
		"list_teams",
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

Pick the right approach based on the user's intent. 站内对话优先直接调用对应的 create_* 工具落库（缺业务组/团队会自动弹表单），不要去走 skill 里描述的 HTTP 登录+POST 流程（那是给外部 A2A agent 的）：
- Alert rule (告警规则): 想导入 integrations 里现成的规则包就用 import_alert_rule_template（先 preview_alert_rule_template 看包里有啥），完全自定义才用 create_alert_rule（参考 n9e-create-alert-rule skill 的配置说明）
- Dashboard (仪表盘): 想导入现成模板用 import_dashboard_template，否则 create_dashboard
- Alert mute (屏蔽规则): 用 create_alert_mute 工具（config 字段形状见工具说明）
- Alert subscribe (订阅规则): 用 create_alert_subscribe 工具
- Notify rule (通知规则): 用 create_notify_rule 工具（绑定团队/用户组；notify_configs 里的 channel_id 需先确认真实通知媒介 ID）

Guidelines:
- If the request maps to multiple skills (e.g. "创建一个仪表盘和告警"), do them one at a time and confirm each.
- If critical parameters are missing, ask the user concisely in their language instead of guessing.
- After a successful creation, keep the Final Answer short (one sentence). Structured result cards are rendered separately by the UI.`, req.UserInput, ctxHint.String())
}

// ContextForwardInputs forwards structured context (busi_group_id,
// datasource_id, team_ids) to the agent as tool params, so tools like
// create_alert_rule / create_dashboard can read them via getDatasourceId etc.
// without relying on the LLM to thread them through arguments.
//
// router 对所有 action 默认调用本函数：写工具缺参门（tools/form_gate.go）的
// 确定性回退通道是 params["busi_group_id"]，而缺参门的设计目标恰是接住通用
// 路径（general_chat）下的创建。若表单提交值只进提示词文本（ContextDump），
// 模型不复写 group_id 时缺参门会再次弹同一张表单（表单回环）。
func ContextForwardInputs(req *AIChatRequest) map[string]string {
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

// selectGeneralChatTools 是 general_chat 的工具全集：只读全集 + 带门写工具
// （诊断/专项工具随技能注入，见工具渐进披露）。含 search_n9e_docs 以支持
// "问数据 + 问文档"的混合场景——例如先 list_metrics 列出指标名, 再
// search_n9e_docs 查每个指标的真实含义, 避免凭记忆瞎解释。
func selectGeneralChatTools(req *AIChatRequest) []string {
	return []string{
		// 告警
		"search_active_alerts", "search_history_alerts", "get_alert_event_detail",
		"get_alert_eval_logs", "get_event_processing_logs", "get_event_pipeline_executions",
		"list_alert_engine_instances",
		// 资源
		"list_alert_rules", "get_alert_rule_detail", "list_legacy_notify_alert_rules",
		"list_alert_mutes", "get_alert_mute_detail",
		"list_alert_subscribes", "get_alert_subscribe_detail",
		"list_task_tpls", "get_task_tpl_detail",
		"list_notify_rules", "get_notify_rule_detail",
		"list_notify_channels", "list_message_templates", "list_notify_rule_custom_params",
		"list_datasources", "get_datasource_detail",
		"list_dashboards", "get_dashboard_detail",
		"list_targets", "get_target_detail", "list_neighbor_targets",
		"get_target_realtime_status", "probe_target_onboard_status", "query_host_metrics_window",
		"list_users", "list_teams", "list_busi_groups",
		// 数据源查询
		"query_prometheus", "query_timeseries", "query_log",
		"list_metrics", "get_metric_labels",
		"list_databases", "list_tables", "describe_table",
		// 文件 / 网络（只读）
		"read_file", "list_files", "grep_files", "http_fetch",
		// 文档检索: n9e/categraf 特定字段/指标/Header/API 路径等"具体事实"必须从
		// 这里取证, 不能凭训练记忆答 — 否则会编 Severity=Critical / Authorization
		// Bearer / ping_result_code / [[inputs.xxx]] 等不存在的标识符。
		"search_n9e_docs",
		// 写操作（创建/编辑）。历史上通用路径不暴露写工具——"没有 preflight 保护，
		// 缺 busi_group_id 会误建"。该约束已由工具级缺参门解除（agent-routing-
		// contraction §3）：create/import 工具缺业务组(通知规则为团队)时返回 input
		// 中断弹表单，任何路径下都不会瞎选；update 工具自带两阶段提案确认门。
		// 通知规则/屏蔽/订阅也挂上对应 create_* 工具——表单应答这类输入常落到
		// general_chat（无创建动词），只有这里挂了工具才能自愈完成落库，不再只回操作步骤。
		"create_alert_rule", "import_alert_rule_template", "preview_alert_rule_template",
		"create_dashboard", "import_dashboard_template",
		"create_notify_rule", "create_alert_mute", "create_alert_subscribe",
		"update_alert_rule", "update_dashboard",
		"update_notify_rule", "update_alert_mute", "update_alert_subscribe",
	}
}
