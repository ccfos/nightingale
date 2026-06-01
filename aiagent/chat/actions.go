package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

	// RequiredSkills 声明本 action 路径上需要加载的 skill 名列表。
	// 非 nil 时 router 用它覆盖 agent 默认 SkillConfig，跳过 LLM autoselect 的 round-trip。
	// 用函数而非 []string 是为了允许依赖 req.Context 决定具体 skill
	// （如 query_generator 按 datasource_type 在 promql/sql skill 之间二选一）。
	//
	// 返回 nil 表示"本次不需要任何 skill"（注意不同于"未声明此字段"——未声明走 agent 默认配置）。
	RequiredSkills func(req *AIChatRequest) []string

	// AgentMode 覆盖默认 ReAct 模式。空字符串走 ReAct。AgentModeDirect 适用于：
	//   1. SelectTools 为 nil 或返回空（无工具调用）
	//   2. RequiredSkills 涉及的 skill 不含 skill_tools
	//   3. 单轮纯文本生成
	// 满足以上条件时改 Direct 可省 ReAct 的 Thought/Action 包装开销。
	AgentMode string
}

var registry = map[string]*ActionHandler{
	"query_generator": {
		Description:   "Generate PromQL or SQL query statements (编写查询语句). Examples: '帮我写个查CPU的PromQL', '生成一个查询订单表的SQL', 'write a PromQL for memory usage'",
		Validate:      validateQueryGenerator,
		SelectTools:   selectQueryGeneratorTools,
		BuildPrompt:   buildQueryGeneratorPrompt,
		BuildInputs:   buildQueryGeneratorInputs,
		ParseResponse: parseQueryGeneratorResponse,
		RequiredSkills: func(req *AIChatRequest) []string {
			switch ctxStr(req.Context, "datasource_type") {
			case "prometheus":
				return []string{"promql-generator"}
			case "mysql", "doris", "ck", "clickhouse", "pgsql", "postgresql", "tdengine":
				return []string{"sql-generator"}
			}
			return nil
		},
	},
	"general_chat": {
		Description: "GENERIC monitoring/observability concepts ONLY, AND fallback for ambiguous intent (通用问答/兜底). PRIMARY use: vendor-neutral concept questions — '什么是P99延迟', 'PromQL 和 MetricsQL 区别', '如何优化慢查询', '直方图怎么用'. NOT for n9e/categraf/夜莺 specific questions: those must go to a specialized action — `doc_qa` for product behavior / 字段含义 / 指标含义 / 配置项 / 默认值 / 环境变量 (i.e. anything that requires the official docs as authority), `agent_deploy_guide` for categraf 部署, etc. ALSO serves as fallback when intent is genuinely ambiguous (e.g. '帮我看下系统现在状态怎样'). In fallback mode, can call read-only tools to give a real answer. But if the user clearly references a specialized scenario, route there FIRST.",
		BuildPrompt: buildGeneralChatPrompt,
		// 兜底 action：作为意图分类失败或其他 action validate 失败时的最终落脚点，
		// 挂全部内置工具走 ReAct，以便兜底场景也能真正调用工具（查数据源、查告警、查资源等），
		// 而不是只能凭模型常识空答。
		// RequiredSkills 保持 nil（不引入额外 skill 内容膨胀 system prompt）。
		SelectTools:    selectGeneralChatTools,
		RequiredSkills: func(_ *AIChatRequest) []string { return nil },
	},
	"alert_query": {
		Description: "Query and analyze alert events (查询告警事件). Examples: '最近1小时有哪些告警', '当前有多少P1告警', '查看活跃告警', '历史告警统计', '告警ID 123的详情'",
		SelectTools: selectAlertQueryTools,
		BuildPrompt: buildAlertQueryPrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-query-alert-events"}
		},
	},
	"resource_query": {
		Description: "Query monitoring system resources and configurations (查询监控系统资源配置). Examples: '我有哪些业务组', '查看告警规则列表', '有哪些机器', '仪表盘列表', '屏蔽规则', '订阅规则', '自愈脚本', '通知规则', '数据源列表', '用户列表', '团队列表'",
		SelectTools: selectResourceQueryTools,
		BuildPrompt: buildResourceQueryPrompt,
	},
	"datasource_query": {
		Description: "Execute actual queries against monitoring datasources and return the data (查询数据源真实数据并返回结果). Scope: 实际跑查询拿数据 — 'ES 索引 logs-* 最近1h 日志数量', '查 prometheus up{} 现在多少', '帮我查 MySQL orders 表最近1h 写入量', 'CK 错误日志条数', '看下 VictoriaLogs 最近5m 的 error 日志'. NOTE: 只是'帮我写一条 PromQL/SQL'（不执行，仅要查询文本）走 query_generator; 查告警事件走 alert_query; 查资源配置（数据源列表本身、规则列表等）走 resource_query.",
		SelectTools: selectDatasourceQueryTools,
		BuildPrompt: buildDatasourceQueryPrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-query-datasource"}
		},
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
		Description: "Generate or modify Go templates for alert notification messages (告警通知消息模板). Scope: 模板内容/字段/变量/渲染/卡片标题/颜色. Examples: '告警内容里加主机名', '把 trigger_value 保留两位小数', '钉钉模板 at 告警接收人', '生成一个飞书卡片模板', 'add hostname to notification template'. NOTE: 改 URL/Webhook 地址/请求头/签名/秘钥/接入新平台 是 notify_channel_copilot, NOT this.",
		BuildPrompt: buildNotifyTemplatePrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-generate-message-template"}
		},
		AgentMode: aiagent.AgentModeDirect,
	},
	"notify_channel_copilot": {
		Description: "Configure, onboard, or troubleshoot notification channel/media (通知媒介通道配置/接入/排障). Scope: 媒介通道层 (notify_channel) ——URL/Webhook 地址、请求体、headers、签名、秘钥、AppID/AppSecret/CorpID、超时、代理、TLS、@人/接收人字段. Examples: '怎么接入 Slack/通用 webhook/自建 HTTP', '钉钉媒介加签怎么配', '飞书机器人 URL 写哪', '企微媒介 9499 是什么意思', '为什么发不出去 Bad Request', '改一下钉钉媒介的请求头', '新建一个邮件媒介'. NOTE: 改模板内容/字段渲染 是 notify_template_generator; 改发给谁/订阅 是 creation 里的 notify rule.",
		BuildPrompt: buildNotifyChannelPrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-notify-channel-copilot"}
		},
		AgentMode: aiagent.AgentModeDirect,
	},
	"host_health_diagnose": {
		Description: "Diagnose host/agent reachability — distinguish 真宕机 / agent 假死 / 网络抖动 / 维护中 (主机健康综合判断). Trigger when the user asks why a host is offline, whether categraf is alive, or pushes back on a 'host loss' alert. Examples: '为什么 xxx 这台机器失联了', '这台主机是不是宕机了', 'agent 卡住了吗', 'host 失联告警是不是误报', '这台机器心跳停了', 'target down 是真的吗'. NOTE: 创建/修改告警规则 走 creation; 单纯查看机器列表/详情 走 resource_query; 分析'某条告警为什么触发'走 troubleshooting; '新装机器没出现 / 显示 unknown / 没接入' 走 host_onboard_diagnose（曾经能看到现在失联走本 action，没接入过走 onboard）.",
		SelectTools: selectHostHealthTools,
		BuildPrompt: buildHostHealthPrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-host-health-diagnose"}
		},
	},
	"host_onboard_diagnose": {
		Description: "Diagnose host onboarding failure — 主机接入失败排障（装好 categraf 但夜莺看不到 / 显示 unknown / 无指标）. Trigger when the user asks why a newly installed agent doesn't appear, why the host list shows unknown OS/CPU/version, or why Helm-deployed collectors are partially missing. Examples: '新装的机器为什么没出现', '机器列表 OS 都是 unknown', 'Helm 装了 3 个采集器只看到 1 个', 'agent 注册不进来', '装完 categraf 主机没显示', 'categraf 装好了但夜莺看不到', 'why is my new host not showing up'. NOTE: '曾经能看到现在失联' 走 host_health_diagnose, NOT this; ident 改名后想清理残留走 resource_query; '我要装 categraf / 教我部署' 走 agent_deploy_guide（还没装 vs 装完没出现）.",
		SelectTools: selectHostOnboardTools,
		BuildPrompt: buildHostOnboardPrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-host-onboard-diagnose"}
		},
	},
	"agent_deploy_guide": {
		Description: "Teach the user how to install / deploy / configure the categraf collector (部署 / 安装 / 启动 categraf 采集器). Scope: 下载二进制 / systemd 注册 / Docker 跑 categraf / Windows 服务 / K8s DaemonSet / config.toml 怎么写（writers / heartbeat / hostname / labels）/ 验证采集 / 自升级 / 资源限制. Examples: '怎么部署 categraf', 'categraf 怎么安装', '教我装一下采集器', 'docker 跑 categraf', 'categraf --install', 'win-service-install', 'config.toml writers 怎么写', 'categraf 上报到夜莺地址', 'k8s 装 categraf', '怎么验证 categraf 采集到了'. IMPORTANT — 覆盖默认规则: 即使用户用 '如何 / 怎么 / how to' 这类知识问法，只要是问 categraf 部署/安装/配置/启动/上报，**必须**归到本 action，**不要**走 general_chat. NOTE: '装完看不到机器 / 显示 unknown' 走 host_onboard_diagnose; '曾经在线现在失联' 走 host_health_diagnose; 配置某个具体 input 插件（mysql/nginx/snmp 等）的采集细节也归本 action.",
		BuildPrompt: buildAgentDeployGuidePrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"categraf-deploy-guide"}
		},
		AgentMode: aiagent.AgentModeDirect,
	},
	"datasource_diagnose": {
		Description: "Diagnose datasource connectivity or configuration errors (数据源连通性/配置诊断). Examples: 'ES 报 x509 证书错误怎么处理', 'VictoriaMetrics 的 url 怎么写', '数据源测试连通 401 是什么原因', 'timeout 连不上 Loki', 'clickhouse 对接夜莺'",
		SelectTools: selectDatasourceDiagnoseTools,
		BuildPrompt: buildDatasourceDiagnosePrompt,
	},
	"task_tpl_copilot": {
		Description: "Generate or modify Nightingale (n9e) self-healing scripts / 告警自愈脚本 (task_tpl/ibex). Scope: 脚本正文 / stdin 解析 / 超时 / 批次 / 容忍度 / 参数 / 危险命令规避. Examples: '写一个磁盘满清理 /var/log 的自愈脚本', '把这个脚本加上 stdin 解析', '清理 docker 镜像层缓存的脚本', 'Java OOM 自动 dump+重启', 'reload nginx 的安全脚本', '自愈脚本怎么拿告警标签', 'is_recovered 怎么用', 'timeout 应该填多少'. NOTE: 改告警规则/接收人/通知模板都不在这里——分别走 creation / notify rule / notify_template_generator.",
		BuildPrompt: buildTaskTplCopilotPrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-modify-task-tpl"}
		},
		AgentMode: aiagent.AgentModeDirect,
	},
	"doc_qa": {
		// 这是"找文档读"型 action，专门接 n9e 平台特有术语 / UI 操作 / 显式查文档诉求。
		// 边界写得长是因为它跟很多 action 的"配置/如何"语义相邻，必须显式让出地盘。
		// 工作流走 ReAct（默认），因为必须调 search_n9e_docs 工具；SKILL.md 的 frontmatter
		// 已声明 builtin_tools，appendSkillTools 会自动注入，所以这里不设 SelectTools。
		Description: "Look up the n9e (Nightingale) / Flashcat OFFICIAL DOCS + integrations/ config samples to answer platform-specific factual questions. TRIGGER WHEN the user asks about ANY of: (a) doc references — '从文档里查', '文档里怎么说', 'docs 里有没有'; (b) 夜莺-specific terms/UI — 业务组/BusiGroup, 订阅规则, 屏蔽规则, edge 模式, Token, 附属告警, 通知 pipeline, 自愈触发条件; (c) **categraf input plugin field meaning / metric name / default value / environment variable / config syntax** — '[[instances]] 怎么写', 'ping_average_response_ms 单位是什么', 'http_response_result_code 各值含义', 'net_response 失败时返回啥', 'mysql input 哪些字段', 'N9E_API_URL 是干啥的', 'Severity 1 代表什么'; (d) n9e behavior/internal logic — 心跳判定逻辑, target sync 间隔, ingest 队列长度, omit_hostname 影响. Examples: '业务组是什么', 'edge 模式和中心模式区别', '订阅规则怎么用', 'categraf 怎么写 mysql 配置', 'ping 监控对应的指标名是什么', 'Severity 1 是 Critical 吗'. NOTE — STRICT BOUNDARIES (do NOT take these): 创建/新建任何资源 → creation; 教用户从零部署 categraf (二进制下载/systemd注册/docker run/k8s yaml) → agent_deploy_guide; 配通知通道/Webhook/签名 → notify_channel_copilot; 改通知模板字段 → notify_template_generator; 写自愈脚本 → task_tpl_copilot; 排查告警/诊断故障 → troubleshooting; 数据源连不上 → datasource_diagnose; 真正 vendor-neutral 的监控概念 ('什么是P99','PromQL vs MetricsQL') → general_chat.",
		BuildPrompt: buildDocQAPrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-doc-qa"}
		},
		// SKILL.md frontmatter 已声明 builtin_tools (search_n9e_docs / verify_answer),
		// appendSkillTools 自动注入, 这里无需 SelectTools。
	},
	"auto_heal_recommend": {
		Description: "Recommend a self-healing action for a fired alert event (告警自愈推荐 / 半自愈). Given an alert event, gather three-layer evidence (event → rule → history), find the safest matching task_tpl, show what it would do (脚本要点 + 风险 + dry-run hint), and emit a 一键执行 marker — OR draft a new task_tpl spec and hand off to task_tpl_copilot when no candidate fits. Examples: '这条告警能自愈吗', '帮我处理一下这个告警', '推荐一个自愈脚本', '能不能一键修复', '建议下怎么自愈', 'auto-heal this alert', 'fix this incident'. NOTE: 直接写脚本走 task_tpl_copilot; 排查为什么告警触发走 troubleshooting; 判断主机活没活走 host_health_diagnose. Requires context.event_id.",
		Validate:    validateAutoHealRecommend,
		Preflight:   preflightAutoHealRecommend,
		SelectTools: selectAutoHealRecommendTools,
		BuildPrompt: buildAutoHealRecommendPrompt,
		RequiredSkills: func(_ *AIChatRequest) []string {
			return []string{"n9e-recommend-self-heal"}
		},
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

// UnwrapJSONEnvelope handles the case where an LLM mistakenly wraps a markdown
// final answer in a JSON object like {"query": "## 结论\n..."} — the front-end
// then renders the raw JSON with literal "\n" escapes, which looks like garbled
// output. When `content` is such an envelope, returns the extracted body string;
// otherwise returns `content` unchanged.
//
// This is intended for the fallback path only — actions that legitimately emit
// JSON (e.g. query_generator's {"query":"<expr>","explanation":"..."}) have
// their own ParseResponse and never reach this function.
//
// Heuristic: the extracted value must contain a real newline OR a markdown
// header marker ("## "), so we don't mis-unwrap a single-line value like
// `{"query": "up == 0"}` that happens to slip through without ParseResponse.
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

// capabilityListCache 在 init 时一次性算好的"平台能力清单"段落，
// 注入 general_chat 的 prompt，让模型在被问"你能做什么"时讲清楚平台具体功能。
//
// 直接复用每个 action 的 Description 作为单一来源——新增 action 自动出现，
// 不会漂移。但 Description 本职是给意图分类器读的，可能含 "Trigger verbs:"、
// "NOTE: ... NOT xxx" 这类机器语，模型大多数情况会自动剪枝；如果观察到漏到
// 用户答案里频率高，再考虑给 ActionHandler 加专门的 Capabilities 字段。
//
// 用包级变量 + init 而不是每次 BuildPrompt 时实时拼，避免 Go 的初始化循环检测：
// registry 的 BuildPrompt 字段如果直接/间接读 registry 自己，编译器会报
// initialization cycle。这里 init() 在 registry 完成填充之后运行，安全。
var capabilityListCache string

func init() {
	var sb strings.Builder
	sb.WriteString("This monitoring platform supports the following capabilities — mention them when the user asks what you can do (in the user's language):\n")
	keys := make([]string, 0, len(registry))
	for key := range registry {
		if key == string(models.ActionKeyGeneralChat) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sb.WriteString(fmt.Sprintf("- %s\n", registry[key].Description))
	}
	capabilityListCache = sb.String()
}

func buildGeneralChatPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are a helpful assistant for the n9e (Nightingale) monitoring platform, specializing in IT operations, monitoring, and observability.

%s

DOMAIN LIMIT (STRICT):
ONLY answer questions in these domains:
  - n9e (Nightingale) usage, configuration, features
  - IT operations / SRE / DevOps practice
  - Alerting (rules, notifications, on-call, suppression, escalation)
  - Observability — metrics, logs, traces, distributed tracing
  - Related infrastructure (Prometheus, Grafana, Loki, ClickHouse, ElasticSearch, OpenSearch, VictoriaMetrics, etc.)
  - Performance / capacity / troubleshooting in the above contexts

If the user asks anything outside these domains (general programming help unrelated to ops, life advice, news, personal questions, creative writing, math homework, etc.), politely decline in the user's language and remind them this assistant is specialized for n9e and observability topics. Do NOT attempt to answer such questions even partially.

This is the fallback action — earlier intent classification didn't pinpoint a specialized action, so handle a broad range of (in-domain) requests yourself.

Tool usage policy (IMPORTANT — pick the right tool, don't fan out):

1. **Vendor-neutral concept questions** (e.g. "什么是 P99", "PromQL 和 MetricsQL 区别", "histogram vs summary", "黄金信号", "alert fatigue"): answer DIRECTLY from your own knowledge. Do NOT call tools. These don't require product-specific facts.

2. **n9e / categraf / 夜莺 SPECIFIC FACTUAL questions** (e.g. "ping 监控对应的 promql", "Severity 1 是什么", "config.toml http 段作用", "categraf_self 指标怎么获取", "Token 调接口 401 怎么回事", "[http] 段是不是给 Prometheus 抓的"): MUST call **search_n9e_docs** first to get authoritative facts. The doc index contains real toml samples + V9 documentation. NEVER answer such questions from memory — you have a documented history of hallucinating field names / metric names / Severity mappings / Header names when answering without doc retrieval (Severity=Critical / Authorization: Bearer / ping_result_code / [[inputs.xxx]] / enable_queue / N9E_ADDR ... 这些都是历史翻车样本)。

   When search_n9e_docs returns ` + "`quality: empty`" + ` or ` + "`must_refuse: true`" + `, you MUST NOT invent specific identifiers — instead reply with the refusal template (see below) and offer vendor-neutral concept guidance.

3. **CURRENT system state / data queries** (e.g. "我现在有哪些告警", "查一下 ES 索引日志数", "哪些机器掉线了", "datasource 列表"): use the appropriate read tool to fetch real data. Prefer the most specific tool. Don't fan-out.

4. **For datasource data queries**: list_datasources first if datasource not specified; then pick query_prometheus / query_timeseries / query_log by plugin_type.

5. This fallback path is READ-ONLY. If the user asks to create or import resources, instruct them to phrase it as an explicit creation request (e.g. "创建/新建...") so it routes to the dedicated creation flow with proper business-group selection.

Refusal template (use VERBATIM when search_n9e_docs is empty for a factual question):
` + "```" + `
我在 n9e/categraf 官方文档里没有找到关于 <用户问题里的关键名词> 的明确描述，
为避免给你错误信息，我不直接回答这个问题。建议:

1. 📖 到 https://flashcat.cloud/docs/ 切换版本手动查
2. 🐛 到 https://github.com/ccfos/nightingale/issues 搜历史问答
3. 💬 加夜莺社区群直接问研发

[可选: 这里附一段 vendor-neutral 的概念引导, 不带任何 n9e/categraf 特定标识符]
` + "```" + `

Answer in the user's language. Use well-formatted Markdown.

User request: %s`, capabilityListCache, req.UserInput)
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
		"import_dashboard_template",
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

Tool-selection rule: if the Front-end context block below carries event_id, your FIRST tool call MUST be get_alert_event_detail(id=event_id). Do NOT call search_active_alerts to look it up — the user is already pointing you at a specific event.

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
	return fmt.Sprintf(`You are an n9e notification template expert. Classify the user request first, then answer accordingly.

User request: %s

If asking for a template (帮我写/加上/改成/生成/write/add/change/generate): output one-line lead → fenced gotemplate block → short 变量说明 list. Split $event.IsRecovered branches; prefer $labels.<key>; wrap times in timeformat; numeric precision via formatDecimal. Don't ask clarifying questions—state assumptions in the lead.

If asking how-to/conceptual (如何/怎么/支持哪些/有哪些字段/能不能/是什么/how/what/can): answer directly with prose or short snippets, NO full template dump. For strongly ambiguous asks (e.g. "附加信息字段"), give 2-3 short options to pick from.

Respond in the user's language.`, req.UserInput)
}

// --- notify_channel_copilot action ---
//
// Channel-layer configuration: URL/headers/body/signing/auth/proxy/TLS, plus
// onboarding new platforms (Slack, custom HTTP, generic webhook) and
// diagnosing send failures (9499 / Bad Request / 401 etc).
// Substantive knowledge lives in the n9e-notify-channel-copilot skill's
// SKILL.md — this prompt just frames the task.

// --- agent_deploy_guide action ---
//
// Teaches users how to install/deploy/configure categraf collector across
// Linux (binary+systemd), Docker, Windows, K8s. Substantive deployment
// commands and config snippets live in the categraf-deploy-guide skill's
// SKILL.md — this prompt just frames the task and reinforces the routing
// boundary against host_onboard_diagnose (装完没出现) and host_health_diagnose
// (曾经在线失联).

func buildAgentDeployGuidePrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are a categraf deployment expert for n9e (Nightingale). Follow the categraf-deploy-guide skill (SKILL.md) for substantive guidance — deployment matrix, config.toml shape, verification commands.

User request: %s

Stay in the "install / configure / start / verify" stage:
- 选择部署方式（binary+systemd / Docker / Windows / K8s）— 先问清 OS 和形态再给方案。
- config.toml 关键段：[[writers]].url 指向 n9e 的 /prometheus/v1/write；[heartbeat] enable=true + url 指向 /api/n9e/heartbeat；[global].hostname 重名场景显式指定；不要把 omit_hostname 设成 true。
- 每条建议给可粘贴执行的命令或完整配置片段，不要写"修改一下配置"这种废话。
- 部署完后必须提醒做一次 './categraf --test --inputs cpu' 并去夜莺机器列表确认。

Do NOT drift into:
- "装完夜莺看不到机器 / OS 显示 unknown" → 告诉用户这是接入失败排障，转 host_onboard_diagnose。
- "曾经在线现在失联" → 转 host_health_diagnose。
- 写告警规则 / 通知模板 / 自愈脚本 → 对应 creation / notify_template_generator / task_tpl_copilot。

Respond in the user's language (中文用户中文，英文用户英文), Markdown.`, req.UserInput)
}

// buildDocQAPrompt 是 thin shell: 真正的工作流 / 翻车案例 / 输出格式 / 索引版本约束
// 全部在 n9e-doc-qa skill 的 SKILL.md 里, 这里只做三件事:
//   1. 注入 user input
//   2. 强调 "禁止凭训练记忆答" 这一最高指令 (反正 LLM 会读 SKILL.md, 这条再喊一遍兜底)
//   3. 提醒 Final Answer 前必须调 verify_answer (与 SKILL.md 的强制工作流保持一致)
//
// 不再在 prompt 里复述 landmine 案例、source 标记说明、三段式输出协议 ——
// 那些规则只在 SKILL.md + landmines.yaml 里维护, 单一事实源, 避免漂移。
func buildDocQAPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are answering an n9e (Nightingale) / Flashcat platform usage question.

User request: %s

Follow the n9e-doc-qa skill (SKILL.md) for the full workflow: tokenize keywords → call search_n9e_docs → synthesize top results → call verify_answer on your draft → revise if HIGH hits → Final Answer with markdown citations.

红线 (压倒一切): 回答里每一个"具体事实"(字段名 / 配置语法 / API 路径 / Header / 环境变量 / 指标名 / 默认值 / Severity 命名 / 端口) **必须能在 search_n9e_docs 返回的 contents 里逐字找到**。找不到就说"文档里没找到", 禁止凭训练记忆补。错答比不答伤害大 100 倍。

Final Answer 前**必须**调一次 verify_answer(answer="<完整草稿>"), 命中 HIGH 必须按 retry_hint 重搜重写, 直到 clean=true 或仅剩 medium/low 才能输出。

Respond in the user's language (中文用户中文, 英文用户英文), Markdown.`, req.UserInput)
}

func buildNotifyChannelPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are an n9e (Nightingale) notification channel (通知媒介) expert. Follow the n9e-notify-channel-copilot skill (SKILL.md) for substantive guidance.

User request: %s

Stay in the channel/transport layer: URL, request body, headers, signing, auth (AppID/AppSecret/CorpID/token), timeout, proxy, TLS, @-mention/recipient-field plumbing. Do NOT drift into:
- 消息模板/字段渲染 → say so briefly and point at notify_template_generator.
- 发给谁/订阅匹配 → say so briefly and point at notify rule (creation).

If the user is onboarding a platform not in the built-in list (Slack, 自建 HTTP, 其他 IM), default to the "通用 Webhook" channel and walk them through: create channel → JSON body shape (event 字段) → 中转/目标地址 → test send. State assumptions inline instead of asking clarifying questions for every gap.

Respond in the user's language, Markdown (NOT JSON).`, req.UserInput)
}

// --- host_health_diagnose action ---

func selectHostHealthTools(req *AIChatRequest) []string {
	return []string{
		"list_targets", "get_target_detail",
		"get_target_realtime_status", "query_host_metrics_window", "list_neighbor_targets",
		"list_alert_mutes", "get_alert_mute_detail",
		"search_active_alerts",
		"list_datasources",
	}
}

func buildHostHealthPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are an SRE assistant for the Nightingale (n9e) monitoring platform, specialized in judging host / agent (categraf) health. Follow the n9e-host-health-diagnose skill (SKILL.md) for the substantive logic.

User request: %s

Core principle: "agent 失联 ≠ 主机宕机". Always gather evidence from THREE layers before concluding:
1. Realtime heartbeat & meta — get_target_realtime_status (Redis BeatTime, Offset, CpuUtil, MemUtil)
2. Recent metrics window (default 10m) — query_host_metrics_window (cpu_usage_active / mem_available_percent / system_load1 / net_bytes_*)
3. Same busi-group neighbors — list_neighbor_targets (个体故障 vs 集群故障 vs 网络分区)

Also check list_alert_mutes (是否在维护窗口) and search_active_alerts (是否还有其它伴生告警).

Final Answer MUST be Markdown (NOT JSON), in the user's language. Required structure:
## 结论
One of: 真宕机 / agent 假死 / 网络抖动 / 维护中 / 数据不足无法判断
## 关键证据
- 心跳：beat_time / lag / status
- 指标：最近 10m cpu/mem/load/net 末值与窗口内 min/max/avg
- 邻居：同业务组 active/lagging/stale 计数
- 屏蔽：是否命中 mute
## 建议动作
2-3 条具体可执行项
## 误报风险
说明这个结论可能在什么情况下站不住脚`, req.UserInput)
}

// --- host_onboard_diagnose action ---

func selectHostOnboardTools(req *AIChatRequest) []string {
	return []string{
		"probe_target_onboard_status",
		"list_targets", "get_target_detail",
		"query_prometheus", "query_host_metrics_window",
		"list_datasources",
	}
}

func buildHostOnboardPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are an SRE assistant for the Nightingale (n9e) monitoring platform, specialized in diagnosing host onboarding failures — categraf is installed/running but the host doesn't show up in n9e, or shows up with unknown metadata, or has no metrics. Follow the n9e-host-onboard-diagnose skill (SKILL.md) for the substantive logic.

User request: %s

Core principle: "机器没出现" is never a single cause — the onboarding pipeline has 5 segments and you must locate which one broke before recommending a fix:
  [1] categraf 本机进程       — running? heartbeat.enable on?
  [2] 心跳上报 HTTP            — can it reach /v1/n9e/heartbeat? (network/TLS/BasicAuth)
  [3] server / edge 接收       — token / version compatibility / hostname collision
  [4] target 表落库            — DB row exists? redis meta exists?
  [5] Redis + 指标流          — does prom see this ident?

ALWAYS call probe_target_onboard_status FIRST — it returns the 5-segment footprint plus a likely_segment + likely_causes diagnosis aggregated server-side. Trust the likely_segment field; do not re-derive the segment from raw fields.

If likely_segment=segment_5, additionally run THREE PromQL variants via query_prometheus to distinguish "no data" from "ident label mismatch":
  target_up{ident="<ident>"}
  target_up{ident=~".*<host>.*"}
  {instance=~".*<host>.*"}

Final Answer MUST be Markdown (NOT JSON), in the user's language. Required structure:
## 结论
One line: 卡在第 X 段：xxx（或：接入正常，看不到是 yyy 原因）
## 接入链路证据
- 段 1/2（categraf 本机/HTTP）：<未取证 / 推断异常：xxx>
- 段 3（server 接收）：target in_db=..., os=..., agent_version=...
- 段 4（target 落库 + redis）：in_redis_beat=..., lag_seconds=...
- 段 5（Prom）：prom_metrics_hit=..., target_up_last=...
## 修复命令
2-3 条用户可直接粘贴执行的命令，每条带"预期输出"
## 自证步骤
1-2 条验证修复是否生效的命令`, req.UserInput)
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

// --- task_tpl_copilot action ---
//
// Focused assistant for writing/modifying ibex self-healing scripts (告警自愈
// 脚本 / task_tpl). Substantive knowledge — stdin payload shape (flat
// map[string]string built in alert/sender/ibex.go:118-142, NOT $event),
// is_recovered behavior (ibex never fires on recovery, line 39-42),
// timeout semantics (CleanFields default 30s, max 5 days), language-specific
// stdin readers, danger-command blacklist, and the 20-scenario library —
// lives in the n9e-modify-task-tpl skill's SKILL.md.

func buildTaskTplCopilotPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are an n9e (Nightingale) self-healing script (告警自愈/ibex task_tpl) expert. Follow the n9e-modify-task-tpl skill (SKILL.md) for substantive guidance.

User request: %s

Classify first, then answer accordingly:

A) Generate / modify a script (写一个 / 帮我写 / 加上 / 改成 / 生成 / write / add / change / generate):
   Output structure:
   1. One-line lead stating the assumed scenario.
   2. Fenced shell or python block — always include: (a) safe stdin JSON parsing with default fallbacks, (b) explicit timeout / exit-code handling, (c) idempotency or dry-run guard before any destructive action, (d) stdout echo of before/after state so task_record is observable.
   3. Short "stdin 字段说明" list — only the keys actually referenced by the script, plus a note on which ones come from PromQL labels vs. ibex injection.
   4. "运行参数建议" — timeout / batch / tolerance / pause values with reasoning (e.g. "timeout=120 因为 yum 清理可能超过默认 30s").
   5. Risk & rollback note when the action is mutating.
   State assumptions inline. Do NOT ask clarifying questions if a sensible default exists.

B) How-to / conceptual (如何 / 怎么 / 支持哪些 / 字段有哪些 / 能不能 / 为什么 / how / what / can / why):
   Answer directly with prose or short snippets, NO full template dump.

Stay strictly in script-generation scope. Redirect when:
- "告警恢复时跑个脚本" → 一句话说明 ibex 不在恢复事件触发 (alert/sender/ibex.go IsRecovered 直接 return), 让用户改走 notify_rule + callback 媒介, 不要生成带 is_recovered 分支的脚本 (那是死代码).
- "怎么改告警规则 / PromQL" → point at creation or query_generator action.
- "怎么改通知模板 / 消息内容" → point at notify_template_generator action.
- "怎么改 webhook / 通知通道" → point at notify_channel_copilot action.

Refuse to emit, even when requested: 'rm -rf /', 'rm -rf $UNSET_VAR/', 'mkfs', 'dd of=/dev/...', 'shutdown', 'reboot', 'init 0', 'init 6', 'iptables -F' without backup, 'chmod -R 777 /', 'curl ... | sh' from non-allowlisted hosts, base64-encoded shell payloads. If the user explicitly asks for a destructive op, state the risk and emit a safer dry-run / confirm-flag / scoped variant (e.g. clean ~/.cache for a specific user instead of rm -rf /home).

Respond in the user's language, Markdown (NOT JSON).`, req.UserInput)
}

// --- auto_heal_recommend action ---
//
// Half-automation: AI recommends, human confirms, system executes. Three-layer
// evidence gathering (event → rule → history) followed by candidate matching
// against the user's task_tpl library; emits an inline {{action:run_task_tpl}}
// marker for the frontend to render as a one-click button, OR an
// {{action:switch|to=task_tpl_copilot}} marker when no template fits.
//
// Substantive matching algorithm, risk evaluation, and No-Match handoff
// templates live in the n9e-recommend-self-heal skill's SKILL.md.
//
// Requires context.event_id; preflight halts otherwise.

func validateAutoHealRecommend(req *AIChatRequest) error {
	if ctxInt64(req.Context, "event_id") == 0 {
		return fmt.Errorf("context.event_id is required for auto_heal_recommend")
	}
	return nil
}

func preflightAutoHealRecommend(_ context.Context, _ *aiagent.ToolDeps,
	req *AIChatRequest, _ *models.User) (bool, []models.AssistantMessageResponse, error) {
	if ctxInt64(req.Context, "event_id") > 0 {
		return false, nil, nil
	}
	hint := "自愈推荐需要绑定一条告警事件。请从「告警事件详情页」或「通知卡片」打开 Copilot，让系统把 event_id 传进来后再试。"
	return true, []models.AssistantMessageResponse{{
		ContentType: models.ContentTypeMarkdown,
		Content:     hint,
		IsFinish:    true,
		IsFromAI:    true,
	}}, nil
}

func selectAutoHealRecommendTools(_ *AIChatRequest) []string {
	return []string{
		// L1: 当前事件
		"get_alert_event_detail",
		"get_alert_rule_detail",
		// L2: 历史频次 / 上次处置
		"search_history_alerts",
		"search_active_alerts",
		// L3: 候选 task_tpl 匹配
		"list_task_tpls", "get_task_tpl_detail",
		// 取证 / 上下文
		"query_prometheus", "query_timeseries", "query_log",
		"get_target_detail", "list_busi_groups",
		"list_alert_mutes", "get_alert_mute_detail",
	}
}

func buildAutoHealRecommendPrompt(req *AIChatRequest) string {
	eid := ctxInt64(req.Context, "event_id")
	rid := ctxInt64(req.Context, "rule_id")
	bgid := ctxInt64(req.Context, "busi_group_id")

	var ctxHint strings.Builder
	ctxHint.WriteString(fmt.Sprintf("\nContext: event_id=%d", eid))
	if rid > 0 {
		ctxHint.WriteString(fmt.Sprintf(", rule_id=%d", rid))
	}
	if bgid > 0 {
		ctxHint.WriteString(fmt.Sprintf(", busi_group_id=%d", bgid))
	}

	return fmt.Sprintf(`You are an n9e (Nightingale) self-healing advisor. Follow the n9e-recommend-self-heal skill (SKILL.md) for the substantive matching algorithm, risk-evaluation checklist, and No-Match handoff templates. Your job is RECOMMEND — never execute. Execution is a separate UI button the user clicks.

User request: %s%s

Workflow — gather THREE layers of evidence BEFORE recommending:

L1. Current event — get_alert_event_detail(id=event_id). Capture: severity, target_ident, trigger_value, tags_json, rule_name, datasource_id, is_recovered, group_id. If is_recovered=true → output "⏭️ 事件已恢复" and skip everything below (no execution marker).

L2. The rule — get_alert_rule_detail(rule_id). Read the PromQL / query expression. Identify which labels the rule's by() preserves — these are what a self-heal script can read from stdin (see ibex.go:118-142). If a needed label is missing, flag it explicitly.

L3. Frequency & history — search_history_alerts with rule_id filter and a 30d range. Count occurrences. Was the most recent recurrence auto-healed (look at related task records via the event labels)? Recurring (≥5 / 30d) → confidence boost; first-time → recommend with extra caution.

Then candidate matching (skill §3 决策树 has the full logic):

- list_task_tpls with a query built from event labels and rule intent (例: "disk clean log", "restart oom", "nginx reload"). Cap candidates considered at 10, evaluate top 3 by tag/title overlap.
- For each candidate: get_task_tpl_detail and judge:
  (a) intent match: does its script actually address THIS condition?
  (b) bg boundary: tpl.group_id == event.group_id? (跨 bg 即使 admin 也不直接推荐执行 — alert/sender/ibex.go:CanDoIbex 会拒)
  (c) stdin sufficiency: does the script need labels the event doesn't carry? (若缺, 提示用户修 PromQL by 子句)
  (d) destructive safety: rm -rf / shutdown / kubectl delete / iptables -F 等黑名单命令且无 dry-run/护栏 → 标 ⚠️ 或 ❌
  (e) target reachability: target online? (light check via target_ident if available)

Branch decision (skill §3):
- ≥1 candidate passes all checks → ✅ 推荐执行
- candidates exist but all fail → choose the closest reason: N2 (全跨 bg) / N3 (语义错位) / N4 (高风险无护栏)
- 0 candidates → N1 (草案生成)

In ALL no-match branches, emit an inline marker for the frontend to render as a "用此草案写脚本" button:
{{action:switch|to=task_tpl_copilot|prefill=<一段自然语言描述, 包含: 目标 / 必备 stdin 标签 / 步骤骨架 / 建议 timeout / 风险点>}}

When recommending execution, emit:
{{action:run_task_tpl|tpl_id=<id>|host=<event.target_ident>|event_id=<event_id>}}
(Frontend parses this, shows script preview + risks in a confirm dialog, then calls the existing ibex API. AI must NEVER call execution tools itself.)

Final Answer MUST be Markdown (NOT JSON) in the user's language, using this structure:

## 推荐结论
✅ / ⚠️ / ❌ / ⏭️ + 一句话

## 关键证据
- 事件: ...
- 规则: ...
- 历史: ...

## 候选清单 / 新建草案
(取决于分支)

## 一键执行 / 写脚本
(对应 {{action:...}} 标记 — 必须放在此节内, 前端就找这一节)

## 误报风险
何时这个推荐可能不准 (告警标签语义模糊 / 上次执行失败过 / 业务高峰期 / 主机正在维护)

Refuse to emit a {{action:run_task_tpl}} marker for any candidate that, after reading its script, contains: 'rm -rf /', 'mkfs', 'shutdown', 'kubectl delete node', 'iptables -F' without backup, or curl-pipe-to-shell. Instead emit N4 (修法建议) with a {{action:switch}} marker pointing to task_tpl_copilot.

Respond in the user's language.`, req.UserInput, ctxHint.String())
}

// --- datasource_query action ---

func selectDatasourceQueryTools(req *AIChatRequest) []string {
	return []string{
		"list_datasources", "get_datasource_detail",
		"query_prometheus", "query_timeseries", "query_log",
		"list_metrics", "get_metric_labels",
		"list_databases", "list_tables", "describe_table",
	}
}

func buildDatasourceQueryPrompt(req *AIChatRequest) string {
	return fmt.Sprintf(`You are a datasource query expert. The user wants to ACTUALLY execute a query and get the data back (NOT just generate query text for them to run later).

User request: %s

Workflow:
1. If the user didn't specify a datasource, call list_datasources first and pick the matching one (by name keyword or plugin_type). If multiple match, ask the user to choose.
2. Pick the right tool by datasource plugin_type:
   - prometheus → query_prometheus (PromQL)
   - mysql/ck/pgsql/doris/tdengine → query_timeseries (sql + value_key) for metric/aggregation; query_log for raw rows
   - elasticsearch/opensearch → query_timeseries (index + filter, for counts/aggregations) or query_log (raw documents)
   - victorialogs → query_timeseries / query_log with LogsQL query
3. For SQL-class datasources, use list_databases / list_tables / describe_table first if the user didn't specify schema details.
4. Default time_range = 1h unless the user specified otherwise.

IMPORTANT: Your Final Answer MUST be well-formatted Markdown (NOT JSON). Use the user's language. Present numerical/aggregated results in tables or clear statements; truncate long log lists to top-N with a note.`, req.UserInput)
}

// --- general_chat action (fallback) ---
// general_chat 是兜底 action：分类失败/validate 失败时落地于此。挂全部内置工具走 ReAct，
// 让模型在兜底场景也能调用工具去查数据，而不是只能凭常识空答。
//
// 工具集包含 search_n9e_docs, 用于"问数据 + 问文档"的混合场景。例如先 list_metrics
// 列出指标名, 再 search_n9e_docs 查每个指标的真实含义, 避免凭记忆瞎解释。
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
		// 注意：兜底路径故意不暴露写操作工具（create_alert_rule / create_dashboard /
		// import_prom_rule_yaml / preview_prom_rule_yaml）。这些走 creation action 才能
		// 触发 PreflightCreation 让用户先选业务组；兜底场景没有 preflight 保护，
		// 模型直接调写工具会因缺 busi_group_id 误建到错误业务组或参数缺失 500。
	}
}
