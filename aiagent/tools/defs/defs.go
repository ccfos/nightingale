// Package defs holds all builtin tool metadata as pure-data AgentTool values.
// Handlers live in aiagent/tools/ and register themselves in init() by pairing
// their handler function with the corresponding def var from this package.
package defs

import "github.com/ccfos/nightingale/v6/aiagent"

// =============================================================================
// Alert
// =============================================================================

var SearchActiveAlerts = aiagent.AgentTool{
	Name:        "search_active_alerts",
	Description: "搜索当前活跃的告警事件（未恢复的告警）",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配告警规则名称或标签", Required: false},
		{Name: "severity", Type: "integer", Description: "告警级别过滤: 1=一级告警, 2=二级告警, 3=三级告警, -1=全部（默认-1）", Required: false},
		{Name: "time_range", Type: "string", Description: "时间范围，如 1h, 6h, 24h, 7d（默认不限）", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认20，最大100", Required: false},
	},
}

var SearchHistoryAlerts = aiagent.AgentTool{
	Name:        "search_history_alerts",
	Description: "搜索历史告警事件（包含已恢复和未恢复的告警）",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配告警规则名称或标签", Required: false},
		{Name: "severity", Type: "integer", Description: "告警级别过滤: 1=一级告警, 2=二级告警, 3=三级告警, -1=全部（默认-1）", Required: false},
		{Name: "time_range", Type: "string", Description: "时间范围，如 1h, 6h, 24h, 7d（默认24h）", Required: false},
		{Name: "is_recovered", Type: "integer", Description: "恢复状态过滤: 0=未恢复, 1=已恢复, -1=全部（默认-1）", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认20，最大100", Required: false},
	},
}

var GetAlertEventDetail = aiagent.AgentTool{
	Name:        "get_alert_event_detail",
	Description: "获取单条告警事件的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "event_id", Type: "integer", Description: "告警事件ID", Required: true},
		{Name: "event_type", Type: "string", Description: "事件类型: active=活跃告警, history=历史告警（默认active）", Required: false},
	},
}

var GetAlertEvalLogs = aiagent.AgentTool{
	Name: "get_alert_eval_logs",
	Description: `获取指定告警规则在告警引擎上的执行日志（alert-eval-detail）。
排查"告警规则没产生事件"时的核心证据：可以看到每次评估是否查到数据、查到的数据是否满足条件、是否产生 event、是否被屏蔽。
返回日志按时间倒序，包含负责该规则的告警引擎实例地址。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "rule_id", Type: "integer", Description: "告警规则ID", Required: true},
	},
}

var GetEventProcessingLogs = aiagent.AgentTool{
	Name: "get_event_processing_logs",
	Description: `获取指定告警事件（按事件 hash）的下游处理日志（event-detail）。
排查"已产生告警事件但没收到通知"时使用：可以看到是否被屏蔽、是否走通知规则、callback / webhook 是否发送成功、订阅是否命中等完整链路。
事件 hash 可以从 search_history_alerts / get_alert_event_detail 返回的 hash 字段获取。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "event_hash", Type: "string", Description: "告警事件 hash（不是事件 ID）", Required: true},
	},
}

var ListAlertEngineInstances = aiagent.AgentTool{
	Name: "list_alert_engine_instances",
	Description: `列出当前所有告警引擎实例（n9e-server）及其心跳。
排查"规则没人跑/某条规则被哪个实例跑"时使用：返回实例地址、所属引擎集群、关联数据源 ID、最近一次心跳时间戳。
心跳时间戳距今超过 30s 视为离线，可能是引擎进程挂了或忘记升级。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "datasource_id", Type: "integer", Description: "按数据源 ID 过滤，仅返回纳管该数据源的引擎实例", Required: false},
		{Name: "engine_cluster", Type: "string", Description: "按告警引擎集群名过滤", Required: false},
	},
}

var GetEventPipelineExecutions = aiagent.AgentTool{
	Name: "get_event_pipeline_executions",
	Description: `获取指定告警事件触发的事件处理器（event pipeline）执行记录列表。
排查"某个事件处理器没生效"时使用：可以看到针对该事件运行了哪些 pipeline、状态（running/success/failed）、失败节点和错误信息、耗时。
如需查看单条执行的节点级详情，再用 status/error_message 字段定位，必要时让用户去 pipeline 执行详情页看完整 node_results。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "event_id", Type: "integer", Description: "告警事件 ID（不是 hash）", Required: true},
	},
}

// =============================================================================
// Alert rule
// =============================================================================

var ListAlertRules = aiagent.AgentTool{
	Name:        "list_alert_rules",
	Description: "查询当前用户有权限的告警规则列表，支持关键词搜索和状态过滤",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配规则名称", Required: false},
		{Name: "disabled", Type: "integer", Description: "状态过滤: 0=启用, 1=禁用, -1=全部（默认-1）", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

var GetAlertRuleDetail = aiagent.AgentTool{
	Name:        "get_alert_rule_detail",
	Description: "获取单条告警规则的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "id", Type: "integer", Description: "告警规则ID", Required: true},
	},
}

var ListLegacyNotifyAlertRules = aiagent.AgentTool{
	Name: "list_legacy_notify_alert_rules",
	Description: `审计哪些告警规则还在用老式接收组（notify_groups）而没有迁移到新版通知规则（notify_rule_ids）。
**只用于迁移审计场景**，例如"扫一下还有哪些老式接收组配置没迁"。日常告警规则查询请用 list_alert_rules，不要用本工具。
判定口径：notify_version=0 即认为是老版本（写入时该字段与 notify_rule_ids 互斥，是平台层的权威迁移标识）。
返回 items 每条带 id / name / group_id / group_name / disabled / severity / cate / notify_groups / notify_rule_ids / update_at / update_by，外加 summary{total, enabled, disabled, with_groups_configured, empty_legacy}。
非 admin 用户按业务组权限自动过滤；admin 看全量。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "include_disabled", Type: "boolean", Description: "是否包含已禁用规则。默认 false（禁用的迁移影响小，先聚焦在跑的）", Required: false},
		{Name: "group_id", Type: "integer", Description: "限定单个业务组 ID，可选；不传则扫所有有权限的业务组", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量上限，默认 500，最大 2000", Required: false},
	},
}

var CreateAlertRule = aiagent.AgentTool{
	Name: "create_alert_rule",
	Description: `创建告警规则，支持 Prometheus/Loki/ES/OpenSearch/TDengine/ClickHouse/MySQL/PostgreSQL/Doris/VictoriaLogs/Host 等数据源类型。
- Prometheus 阈值告警：直接传 prom_ql + threshold + operator，工具自动构建 v2 rule_config
- 其他类型：传 cate + rule_config_json（先读 skill 的 datasources/<cate>.md 获取结构）
- Host 类型：cate="host"，不需要 datasource_id`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "group_id", Type: "integer", Description: "业务组 ID（从 list_busi_groups 获取，优先选择 is_default=true 的组）", Required: true},
		{Name: "name", Type: "string", Description: "告警规则名称（同业务组内不能重名）", Required: true},
		{Name: "cate", Type: "string", Description: "数据源类型: prometheus|loki|elasticsearch|opensearch|tdengine|ck|mysql|pgsql|doris|victorialogs|host（默认 prometheus）", Required: false},
		{Name: "prod", Type: "string", Description: "产品类型: metric|logging|host。不传时按 cate 自动推导", Required: false},
		{Name: "datasource_id", Type: "integer", Description: "数据源 ID（host 类型不需要；其他类型必填）", Required: false},
		{Name: "rule_config_json", Type: "string", Description: "完整的 rule_config JSON 对象字符串。仅在 cate != prometheus 时必填；先调用 read_file(base=\"n9e-create-alert-rule\", path=\"datasources/<cate>.md\") 获取该类型的结构模板", Required: false},
		{Name: "prom_ql", Type: "string", Description: "PromQL 查询表达式（仅 cate=prometheus 简化路径使用），只写查询不要包含阈值，例如 cpu_usage_active{cpu=\"cpu-total\"}", Required: false},
		{Name: "threshold", Type: "number", Description: "触发阈值（仅 cate=prometheus 简化路径使用），例如 80", Required: false},
		{Name: "operator", Type: "string", Description: "比较操作符: > / >= / < / <= / == / !=（默认 >，仅 cate=prometheus 简化路径使用）", Required: false},
		{Name: "severity", Type: "integer", Description: "告警级别: 1=Critical, 2=Warning, 3=Info（默认 2）", Required: false},
		{Name: "note", Type: "string", Description: "告警说明/通知正文", Required: false},
		{Name: "eval_interval", Type: "integer", Description: "评估周期（秒），默认 30", Required: false},
		{Name: "for_duration", Type: "integer", Description: "持续时长（秒），告警条件需持续这么久才触发，默认 60", Required: false},
		{Name: "append_tags", Type: "string", Description: "附加标签，多个用空格分隔，如 \"service=cpu mod=host\"", Required: false},
		{Name: "runbook_url", Type: "string", Description: "应急处理手册 URL", Required: false},
		{Name: "notify_rule_ids", Type: "string", Description: "关联通知规则 ID 列表 JSON，如 \"[1,2]\"。不传则不绑定", Required: false},
	},
}

var PreviewPromRuleYAML = aiagent.AgentTool{
	Name: "preview_prom_rule_yaml",
	Description: `预览 Prometheus 告警规则 YAML 解析后的内容，不写入数据库。
用于把远端 Prom rule YAML（支持 groups / rules 数组 / 单条规则三种格式）映射为 n9e 的告警规则结构，让用户确认后再调 import_prom_rule_yaml 落库。
返回每条规则的 name / severity / prom_ql / for_duration / labels / annotations，不需要任何权限或业务组参数。
推荐入参用 payload_file（http_fetch save_to_file=true 拿到的 file_path）而不是 payload 文本，可避免大文件占用 LLM 上下文。两者二选一。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "payload", Type: "string", Description: "Prometheus rule YAML 文本（与 prometheus 官方格式一致）。和 payload_file 二选一", Required: false},
		{Name: "payload_file", Type: "string", Description: "Prometheus rule YAML 文件路径（必须是 http_fetch save_to_file=true 返回的 file_path）。和 payload 二选一", Required: false},
	},
}

var ImportPromRuleYAML = aiagent.AgentTool{
	Name: "import_prom_rule_yaml",
	Description: `把 Prometheus 告警规则 YAML 批量导入到指定业务组。
- 支持三种格式：顶层 groups / 纯 rules 数组 / 单条 rule
- labels.severity (critical/warning/info) 会自动映射到 n9e 的 1/2/3
- 其他 labels 自动转成 append_tags
- **同名规则自动跳过**，返回 status=skipped_duplicate；不算 failed，**不要**用 name_prefix 重试整份 YAML（会让已创建的规则全部多写一份）
- name_prefix/name_suffix 用于刻意让导入的规则与现有规则并存（如对比测试），不是用于"重试失败项"
返回每条规则的结果（status=created|skipped_duplicate|failed，对应 id 或 error）。建议先调 preview_prom_rule_yaml 让用户确认。
推荐入参用 payload_file（http_fetch save_to_file=true 拿到的 file_path）而不是 payload 文本。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "group_id", Type: "integer", Description: "业务组 ID（从 list_busi_groups 获取）", Required: true},
		{Name: "datasource_ids", Type: "string", Description: "Prometheus 数据源 ID 列表 JSON，如 \"[1]\" 或 \"[1,3]\"。从 list_datasources 获取", Required: true},
		{Name: "payload", Type: "string", Description: "Prometheus rule YAML 文本。和 payload_file 二选一", Required: false},
		{Name: "payload_file", Type: "string", Description: "Prometheus rule YAML 文件路径（必须是 http_fetch save_to_file=true 返回的 file_path）。和 payload 二选一", Required: false},
		{Name: "disabled", Type: "integer", Description: "导入后是否禁用：0=启用（默认），1=禁用", Required: false},
		{Name: "name_prefix", Type: "string", Description: "给所有规则名加前缀，避免与现有规则重名", Required: false},
		{Name: "name_suffix", Type: "string", Description: "给所有规则名加后缀，避免与现有规则重名", Required: false},
	},
}

var PreviewAlertRuleTemplate = aiagent.AgentTool{
	Name: "preview_alert_rule_template",
	Description: `预览 integrations 目录下某个告警规则包里有哪些规则，不写入数据库。
返回每条规则的 name / cate / severity / 表达式摘要 / 是否禁用，是小载荷（不含完整配置），
用于在导入前让用户挑选：想整包导、导其中几条、还是只参考某一条建单规则。
大规则包（如 Kubernetes 上百 KB）用 read_file 会被截断，要看包里有哪些规则就用本工具，不要用 read_file。
先用 list_files(base="integrations/<component>", path="alerts") 找到文件名再调本工具。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "component", Type: "string", Description: `集成组件名，对应 integrations 下的目录名，如 "Linux"、"MySQL"、"Redis"`, Required: true},
		{Name: "file", Type: "string", Description: `alerts 子目录下的规则包文件名，如 "linux_by_categraf.json"（用 list_files(base="integrations/<component>", path="alerts") 获取）`, Required: true},
	},
}

var ImportAlertRuleTemplate = aiagent.AgentTool{
	Name: "import_alert_rule_template",
	Description: `从 integrations 目录下经过验证的告警规则包里导入规则到指定业务组（每条规则含告警级别、持续时长、评估周期、附加标签、注释、生效时间窗等完整配置），
比 create_alert_rule 手搓质量更高、字段更全。**导入几条按用户需求来**：用 names 选一条（建单规则）、选几条（一批），或不传 names（整包）。
先用 list_files(base="integrations/<component>", path="alerts") 找到文件名；想知道包里有哪些规则、按名字挑，先用 preview_alert_rule_template 看一眼。
不要用 read_file 把整个规则包读出来再逐条 create_alert_rule（会丢字段且大文件会被截断）。
模板里的规则默认是禁用态，本工具导入时默认改为启用（disabled=0）。同名规则自动跳过（status=skipped_duplicate），不算失败，不要用 name_prefix 重试。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "group_id", Type: "integer", Description: "业务组 ID（从 list_busi_groups 获取）", Required: true},
		{Name: "component", Type: "string", Description: `集成组件名，对应 integrations 下的目录名，如 "Linux"、"MySQL"、"Redis"`, Required: true},
		{Name: "file", Type: "string", Description: `alerts 子目录下的规则包文件名，如 "linux_by_categraf.json"（用 list_files(base="integrations/<component>", path="alerts") 获取）。优先选文件名含 categraf 的`, Required: true},
		{Name: "names", Type: "string", Description: `要导入的规则名 JSON 数组（精确匹配 preview_alert_rule_template 返回的 name），如 "[\"CPU high\",\"Mem high\"]"。单条就传一个，多条传多个。不传=导入整包全部规则`, Required: false},
		{Name: "datasource_id", Type: "integer", Description: "数据源 ID（从 list_datasources 获取，规则包多为 prometheus 类型）。强烈建议传：传了就把规则绑定到该数据源；不传则规则匹配该类型的全部数据源。host 心跳类规则不需要数据源，自动跳过绑定", Required: false},
		{Name: "disabled", Type: "integer", Description: "导入后启用状态：0=启用（默认，因为模板里规则默认是禁用态），1=保持禁用", Required: false},
		{Name: "name_prefix", Type: "string", Description: "给所有规则名加前缀，避免与现有规则重名", Required: false},
		{Name: "name_suffix", Type: "string", Description: "给所有规则名加后缀，避免与现有规则重名", Required: false},
	},
}

// =============================================================================
// Busi group
// =============================================================================

var ListBusiGroups = aiagent.AgentTool{
	Name:        "list_busi_groups",
	Description: "查询当前用户有权限的业务组列表，支持关键词模糊搜索",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，模糊匹配业务组名称", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

// =============================================================================
// Dashboard
// =============================================================================

var ListDashboards = aiagent.AgentTool{
	Name:        "list_dashboards",
	Description: "查询当前用户有权限的仪表盘列表，支持关键词搜索",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配仪表盘名称或标签", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

var GetDashboardDetail = aiagent.AgentTool{
	Name:        "get_dashboard_detail",
	Description: "获取单个仪表盘的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "id", Type: "integer", Description: "仪表盘ID", Required: true},
	},
}

var CreateDashboard = aiagent.AgentTool{
	Name: "create_dashboard",
	Description: `创建监控仪表盘。只需提供面板描述和变量，工具会自动生成完整的仪表盘配置。
面板布局自动计算，无需手动指定坐标。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "group_id", Type: "integer", Description: "业务组ID", Required: true},
		{Name: "name", Type: "string", Description: "仪表盘名称", Required: true},
		{Name: "datasource_id", Type: "integer", Description: "Prometheus 数据源ID（从 list_datasources 获取）", Required: true},
		{Name: "panels", Type: "string", Description: `面板列表 JSON 数组。每个面板: {"name":"标题", "type":"timeseries", "queries":[{"promql":"PromQL表达式", "legend":"{{label}}"}]}。type 可选: timeseries/stat/gauge/barGauge/pie/table/row。可选字段: w(宽度)/h(高度)/unit(单位:percent,bytesIEC,seconds等)/stack(是否堆叠)`, Required: true},
		{Name: "variables", Type: "string", Description: `变量列表 JSON 数组。每个变量: {"name":"变量名", "definition":"label_values(metric, label)"}。可选字段: label(显示名)/multi(是否多选,默认true)`, Required: false},
		{Name: "tags", Type: "string", Description: "仪表盘标签，多个用空格分隔", Required: false},
	},
}

var ImportDashboardTemplate = aiagent.AgentTool{
	Name: "import_dashboard_template",
	Description: `直接导入 integrations 目录下经过验证的仪表盘模板（含完整布局、阈值、单位、overrides 等），
比 create_dashboard 手工拼装质量更高。先用 list_files 找到 component 和 dashboards 下的文件名，
再把 component + file 传进来即可——不要用 read_file 把整个模板读出来再拼（模板可能很大且会被截断）。
适用于用户监控主题在 integrations 里有现成模板的场景（如 Linux/MySQL/Redis/Kafka 等）。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "group_id", Type: "integer", Description: "业务组ID", Required: true},
		{Name: "component", Type: "string", Description: `集成组件名，对应 integrations 下的目录名，如 "Linux"、"MySQL"、"Redis"`, Required: true},
		{Name: "file", Type: "string", Description: `dashboards 子目录下的模板文件名，如 "categraf-overview.json"（用 list_files(base="integrations/<component>", path="dashboards") 获取）`, Required: true},
		{Name: "datasource_id", Type: "integer", Description: "Prometheus 数据源ID（从 list_datasources 获取）。可选：传了就作为模板数据源变量的默认选中值，保证首屏可查询；不传则由前端在数据源下拉里自动选第一个 Prometheus", Required: false},
		{Name: "name", Type: "string", Description: "覆盖仪表盘名称。不传则沿用模板自带名称", Required: false},
		{Name: "tags", Type: "string", Description: "覆盖仪表盘标签，多个用空格分隔。不传则沿用模板自带标签", Required: false},
	},
}

// =============================================================================
// Datasource
// =============================================================================

var ListDatasources = aiagent.AgentTool{
	Name:        "list_datasources",
	Description: "查询数据源列表，支持按类型过滤",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "plugin_type", Type: "string", Description: "数据源类型过滤，如 prometheus、mysql、elasticsearch", Required: false},
		{Name: "query", Type: "string", Description: "搜索关键词，匹配数据源名称", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

var GetDatasourceDetail = aiagent.AgentTool{
	Name:        "get_datasource_detail",
	Description: "获取单个数据源的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "id", Type: "integer", Description: "数据源ID", Required: true},
	},
}

// =============================================================================
// Datasource query
// =============================================================================

var QueryPrometheus = aiagent.AgentTool{
	Name:        "query_prometheus",
	Description: "执行 PromQL 查询 Prometheus/VictoriaMetrics 数据源，支持即时查询和范围查询",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "datasource_id", Type: "integer", Description: "Prometheus 数据源 ID（先用 list_datasources 查到）。若调用方已通过会话上下文预选数据源可不传", Required: false},
		{Name: "query", Type: "string", Description: "PromQL 表达式，如 cpu_usage_active{ident='web-01'}", Required: true},
		{Name: "query_type", Type: "string", Description: "查询类型: instant(即时查询,默认) 或 range(范围查询)", Required: false},
		{Name: "time_range", Type: "string", Description: "时间范围，如 15m、1h、6h、24h、7d，默认 1h", Required: false},
		{Name: "step", Type: "integer", Description: "范围查询步长(秒)，不填则根据 time_range 自动推算", Required: false},
	},
}

var QueryTimeseries = aiagent.AgentTool{
	Name: "query_timeseries",
	Description: `执行时序数据查询，通过 Datasource.QueryData() 接口。
适用数据源: mysql, ck(ClickHouse), pgsql(PostgreSQL), doris, tdengine, elasticsearch, opensearch, victorialogs。
SQL 类数据源需提供 sql + value_key；ES 类需提供 index + filter；VictoriaLogs 需提供 query。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "datasource_id", Type: "integer", Description: "数据源 ID（先用 list_datasources 查到）。若调用方已通过会话上下文预选数据源可不传", Required: false},
		{Name: "datasource_type", Type: "string", Description: "数据源类型，取自 list_datasources 返回的 plugin_type 字段，如 mysql/ck/pgsql/doris/tdengine/elasticsearch/opensearch/victorialogs。若调用方已通过会话上下文预选数据源可不传", Required: false},
		// SQL 类 (mysql, ck, pgsql, doris, tdengine)
		{Name: "sql", Type: "string", Description: "SQL 查询语句，支持 $from/$to 时间变量（SQL类数据源使用）", Required: false},
		{Name: "value_key", Type: "string", Description: "数值列名，多个用空格分隔（SQL类数据源时序查询必填）", Required: false},
		{Name: "label_key", Type: "string", Description: "标签/分组列名，多个用空格分隔", Required: false},
		{Name: "time_key", Type: "string", Description: "时间列名", Required: false},
		{Name: "database", Type: "string", Description: "数据库名（SQL类数据源可选）", Required: false},
		// ES/OpenSearch 类
		{Name: "index", Type: "string", Description: "索引名或索引模式，如 logs-*（ES/OpenSearch 使用）", Required: false},
		{Name: "filter", Type: "string", Description: "Lucene 查询语法过滤条件（ES/OpenSearch 使用）", Required: false},
		{Name: "date_field", Type: "string", Description: "时间字段名，默认 @timestamp（ES/OpenSearch 使用）", Required: false},
		// VictoriaLogs
		{Name: "query", Type: "string", Description: "LogsQL 查询表达式（VictoriaLogs 使用）", Required: false},
		{Name: "step", Type: "string", Description: "步长，如 1m、5m（VictoriaLogs 使用）", Required: false},
		// 通用
		{Name: "time_range", Type: "string", Description: "时间范围，如 15m、1h、6h、24h、7d，默认 1h", Required: false},
	},
}

var QueryLog = aiagent.AgentTool{
	Name: "query_log",
	Description: `查询原始日志/数据，通过 Datasource.QueryLog() 接口。
适用数据源: mysql, ck(ClickHouse), pgsql(PostgreSQL), doris, tdengine, elasticsearch, opensearch, victorialogs。
SQL 类数据源需提供 sql；ES 类需提供 index + filter；VictoriaLogs 需提供 query。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "datasource_id", Type: "integer", Description: "数据源 ID（先用 list_datasources 查到）。若调用方已通过会话上下文预选数据源可不传", Required: false},
		{Name: "datasource_type", Type: "string", Description: "数据源类型，取自 list_datasources 返回的 plugin_type 字段，如 mysql/ck/pgsql/doris/tdengine/elasticsearch/opensearch/victorialogs。若调用方已通过会话上下文预选数据源可不传", Required: false},
		// SQL 类
		{Name: "sql", Type: "string", Description: "SQL 查询语句，支持 $from/$to 时间变量（SQL类数据源使用）", Required: false},
		{Name: "database", Type: "string", Description: "数据库名（SQL类数据源可选）", Required: false},
		// ES/OpenSearch 类
		{Name: "index", Type: "string", Description: "索引名或索引模式，如 logs-*（ES/OpenSearch 使用）", Required: false},
		{Name: "filter", Type: "string", Description: "Lucene 查询语法过滤条件（ES/OpenSearch 使用）", Required: false},
		{Name: "date_field", Type: "string", Description: "时间字段名，默认 @timestamp（ES/OpenSearch 使用）", Required: false},
		// VictoriaLogs
		{Name: "query", Type: "string", Description: "LogsQL 查询表达式（VictoriaLogs 使用）", Required: false},
		// 通用
		{Name: "time_range", Type: "string", Description: "时间范围，如 15m、1h、6h、24h、7d，默认 1h", Required: false},
		{Name: "limit", Type: "integer", Description: "返回行数限制，默认 50，最大 500", Required: false},
	},
}

// =============================================================================
// HTTP
// =============================================================================

var HTTPFetch = aiagent.AgentTool{
	Name: "http_fetch",
	Description: `GET 一个公网 URL，返回响应正文文本（或写入临时文件）。用于把外部公开资源（如 GitHub raw 上的 Prometheus rule YAML、Grafana dashboard JSON）拉进当前会话。
仅支持 http/https；自动拒绝指向内网/回环/链路本地等非公网地址（含 DNS rebinding）；响应正文最多 8 MiB、超时最长 60 秒。
推荐配合 save_to_file=true 使用：抓到的内容直接落临时文件，返回 file_path，由 import_prom_rule_yaml / preview_prom_rule_yaml 的 payload_file 参数读取，避免大文件占用 LLM 上下文。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "url", Type: "string", Description: "要抓取的 URL，必须是 http 或 https", Required: true},
		{Name: "save_to_file", Type: "boolean", Description: "true=把响应正文写入临时文件，返回里只含 file_path 不含 body（推荐大文件用，省 token）；false=返回正文文本 body（默认）", Required: false},
		{Name: "max_bytes", Type: "integer", Description: "正文最大字节数，默认 1048576 (1 MiB)，上限 8388608 (8 MiB)", Required: false},
		{Name: "timeout_seconds", Type: "integer", Description: "请求超时秒数，默认 10，上限 60", Required: false},
	},
}

// =============================================================================
// N9e Docs
// =============================================================================

// VerifyAnswer 给 n9e-doc-qa skill 用: LLM 在 Final Answer 之前调一次, 让代码层
// 用正则扫一遍草稿, 把"编造的字段名/环境变量/Severity 命名"等 n9e 域确定性错误捞回来。
// 规则放在 <skillsPath>/n9e-doc-qa/landmines.yaml 里, 跟 skill 同居; 这里只声明
// tool 接口和入参。规则是数据, 不是代码 — 这条工具的 Go 实现是通用的, 任何 skill
// 想用都可以拷一份 yaml 然后在自己的 SKILL.md 里 builtin_tools 引一下。
var VerifyAnswer = aiagent.AgentTool{
	Name: "verify_answer",
	Description: `在 Final Answer **之前**强制调一次, 把你即将发给用户的 markdown 草稿传进来。
工具会用正则跑一遍 n9e 域的确定性错误规则 (来自 n9e-doc-qa/landmines.yaml), 返回:
  - hits: 命中的规则列表, 每条带 matched (匹配到的具体字符串)、severity、annotate (给用户的警告)、retry_hint (修复方向)
  - must_revise: 是否必须修改后重新调本工具 (HIGH severity 命中即 true)
  - clean: hits 为空时为 true
工作流: 草稿 → 调本工具 → 命中就按 retry_hint 用 search_n9e_docs 重搜 → 重写 → 再调本工具直到 clean=true → 才能 Final Answer。
must_revise=true 时不要 Final Answer, 否则你的答案会带 ⚠️ 警告标志发给用户。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "answer", Type: "string", Description: "你打算 Final Answer 的完整 markdown 草稿", Required: true},
	},
}

var SearchN9eDocs = aiagent.AgentTool{
	Name: "search_n9e_docs",
	Description: `在 Flashcat/夜莺(n9e) 官方文档站（flashcat.cloud/docs）做关键词搜索，返回 top N 篇匹配文档的标题、链接、描述和**完整正文**。
专门用于回答"平台使用类"问题（怎么操作 / 如何配置 / 概念解释）。索引每天自动同步一次，常驻内存；只含 V9 文档（V5-V8 旧版已过滤）。
打分：关键词命中 title +5、description +3、contents 每次 +1（封顶 3）。
返回字段 contents 是该文档的完整正文（rune 截断到 6000，覆盖 99% 文档全长）——必须以此为答题依据，**禁止**凭训练记忆补充正文里没出现的字段名/接口路径/Header 格式。permalink 必须在回答末尾以引用形式带给用户。
若 total=0，换关键词重试一次；仍无 → 告诉用户文档里没找到。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "keywords", Type: "string", Description: "搜索关键词，多个用空格分隔（如 \"告警规则 配置\"）。所有词都会做 lowercase 子串匹配", Required: true},
		{Name: "top_n", Type: "integer", Description: "返回 top N 篇，默认 3，上限 10", Required: false},
	},
}

// =============================================================================
// File
// =============================================================================

var ListFiles = aiagent.AgentTool{
	Name:        "list_files",
	Description: "列出技能或集成模板目录下的文件和子目录",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "base", Type: "string", Description: "基础目录名: 技能名(如 n9e-create-dashboard)或 integrations/分类(如 integrations/Linux)", Required: true},
		{Name: "path", Type: "string", Description: "相对子路径，如 dashboards 或 metrics。不传则列出根目录", Required: false},
	},
}

var ReadFile = aiagent.AgentTool{
	Name:        "read_file",
	Description: "读取技能文档或集成模板文件内容",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "base", Type: "string", Description: "基础目录名: 技能名(如 n9e-create-dashboard)或 integrations/分类(如 integrations/Linux)", Required: true},
		{Name: "path", Type: "string", Description: "相对文件路径，如 dashboards/categraf-detail.json 或 metrics/categraf-base.json", Required: true},
	},
}

var GrepFiles = aiagent.AgentTool{
	Name:        "grep_files",
	Description: "在技能或集成模板目录下搜索包含指定关键词的文件和行",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "base", Type: "string", Description: "基础目录名: 技能名(如 n9e-create-dashboard)或 integrations/分类(如 integrations/Linux)", Required: true},
		{Name: "pattern", Type: "string", Description: "搜索关键词（不区分大小写）", Required: true},
		{Name: "path", Type: "string", Description: "相对搜索路径，不传则搜索整个目录", Required: false},
	},
}

// =============================================================================
// Metric
// =============================================================================

var ListMetrics = aiagent.AgentTool{
	Name:        "list_metrics",
	Description: "搜索 Prometheus 数据源的指标名称，支持关键词模糊匹配。需要通过 datasource_id 指定 Prometheus 数据源（用 list_datasources 查到）",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "datasource_id", Type: "integer", Description: "Prometheus 数据源 ID（从 list_datasources 获取）", Required: true},
		{Name: "keyword", Type: "string", Description: "搜索关键词，模糊匹配指标名", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认30", Required: false},
	},
}

var GetMetricLabels = aiagent.AgentTool{
	Name:        "get_metric_labels",
	Description: "获取 Prometheus 指标的所有标签键及其可选值。需要通过 datasource_id 指定 Prometheus 数据源",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "datasource_id", Type: "integer", Description: "Prometheus 数据源 ID（从 list_datasources 获取）", Required: true},
		{Name: "metric", Type: "string", Description: "指标名称", Required: true},
	},
}

// =============================================================================
// Mute
// =============================================================================

var ListAlertMutes = aiagent.AgentTool{
	Name:        "list_alert_mutes",
	Description: "查询当前用户有权限的告警屏蔽规则列表",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配屏蔽原因", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

var GetAlertMuteDetail = aiagent.AgentTool{
	Name:        "get_alert_mute_detail",
	Description: "获取单条告警屏蔽规则的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "id", Type: "integer", Description: "屏蔽规则ID", Required: true},
	},
}

// =============================================================================
// Notify rule
// =============================================================================

var ListNotifyRules = aiagent.AgentTool{
	Name:        "list_notify_rules",
	Description: "查询当前用户有权限的通知规则列表",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配通知规则名称", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

var GetNotifyRuleDetail = aiagent.AgentTool{
	Name:        "get_notify_rule_detail",
	Description: "获取单条通知规则的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "id", Type: "integer", Description: "通知规则ID", Required: true},
	},
}

// =============================================================================
// SQL
// =============================================================================

var ListDatabases = aiagent.AgentTool{
	Name:        "list_databases",
	Description: "列出 SQL 数据源（MySQL/Doris/ClickHouse/PostgreSQL）中的所有数据库。创建 SQL 类告警规则前先用这个探测真实的数据库名，不要凭空猜。",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "datasource_id", Type: "integer", Description: "SQL 数据源 ID。会话上下文已绑定数据源时可省略；告警规则创建等场景必须显式传", Required: false},
		{Name: "datasource_type", Type: "string", Description: "数据源类型（mysql/doris/ck/pgsql）。一般不用传，会自动从 datasource_id 反查", Required: false},
	},
}

var ListTables = aiagent.AgentTool{
	Name:        "list_tables",
	Description: "列出指定数据库中的所有表。创建 SQL 类告警规则前先用这个探测真实的表名，不要凭空猜。",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "database", Type: "string", Description: "数据库名", Required: true},
		{Name: "datasource_id", Type: "integer", Description: "SQL 数据源 ID。会话上下文已绑定数据源时可省略；告警规则创建等场景必须显式传", Required: false},
		{Name: "datasource_type", Type: "string", Description: "数据源类型（mysql/doris/ck/pgsql）。一般不用传，会自动从 datasource_id 反查", Required: false},
	},
}

var DescribeTable = aiagent.AgentTool{
	Name:        "describe_table",
	Description: "获取表的字段结构（字段名、类型、注释）。创建 SQL 类告警规则前先用这个拿到真实字段名，不要编造字段。",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "database", Type: "string", Description: "数据库名", Required: true},
		{Name: "table", Type: "string", Description: "表名", Required: true},
		{Name: "datasource_id", Type: "integer", Description: "SQL 数据源 ID。会话上下文已绑定数据源时可省略；告警规则创建等场景必须显式传", Required: false},
		{Name: "datasource_type", Type: "string", Description: "数据源类型（mysql/doris/ck/pgsql）。一般不用传，会自动从 datasource_id 反查", Required: false},
	},
}

// =============================================================================
// Subscribe
// =============================================================================

var ListAlertSubscribes = aiagent.AgentTool{
	Name:        "list_alert_subscribes",
	Description: "查询当前用户有权限的告警订阅规则列表",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配订阅名称", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

var GetAlertSubscribeDetail = aiagent.AgentTool{
	Name:        "get_alert_subscribe_detail",
	Description: "获取单条告警订阅规则的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "id", Type: "integer", Description: "订阅规则ID", Required: true},
	},
}

// =============================================================================
// Target
// =============================================================================

var ListTargets = aiagent.AgentTool{
	Name:        "list_targets",
	Description: "查询当前用户有权限的机器/主机列表，支持关键词搜索（ident、IP、标签等）",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配 ident、IP、备注、标签、操作系统", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

var GetTargetDetail = aiagent.AgentTool{
	Name:        "get_target_detail",
	Description: "获取单台机器/主机的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "id", Type: "integer", Description: "机器ID", Required: false},
		{Name: "ident", Type: "string", Description: "机器标识（ident），与id二选一", Required: false},
	},
}

var GetTargetRealtimeStatus = aiagent.AgentTool{
	Name: "get_target_realtime_status",
	Description: `从 Redis 读取目标主机的实时心跳与元数据，用于判断 "agent 失联 / 假死 / 真宕机"。
返回字段：beat_time(最近一次心跳秒级时间戳) / lag_seconds(now - beat_time) / status(active|lagging|stale) /
offset(agent 时钟与 server 偏移) / cpu_util / mem_util / agent_version / remote_addr / extend_info。
status 判定：lag<60s=active, 60s≤lag<180s=lagging, lag≥180s=stale；redis 里完全没有心跳 key 时 status=stale_no_heartbeat。
本工具只读不写，权限按目标所属业务组判定。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "ident", Type: "string", Description: "机器标识（ident），必填", Required: true},
	},
}

var QueryHostMetricsWindow = aiagent.AgentTool{
	Name: "query_host_metrics_window",
	Description: `查询主机最近窗口的核心健康指标聚合（cpu_usage_active / mem_available_percent / system_load1 / net_bytes_recv / net_bytes_sent），用于判断 "数据真停了 / 还在动 / 有突变"。
返回每个指标的 samples_count / first_ts / last_ts / min / max / avg / last。
不返回完整时序点，避免炸 token；要看趋势再单独调 query_prometheus。
默认窗口 10m。datasource_id 不传时按 params 里的 chat-level datasource_id 兜底；都没有时报错并提示先 list_datasources。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "ident", Type: "string", Description: "机器标识（ident），必填", Required: true},
		{Name: "datasource_id", Type: "integer", Description: "Prometheus 数据源 ID。不传时使用 chat-level params 兜底", Required: false},
		{Name: "metrics", Type: "string", Description: "指标列表，空格或逗号分隔。不传则使用默认四件套 cpu_usage_active/mem_available_percent/system_load1/net_bytes_recv,net_bytes_sent", Required: false},
		{Name: "time_range", Type: "string", Description: "时间窗口，如 10m / 30m / 1h，默认 10m", Required: false},
	},
}

var ListNeighborTargets = aiagent.AgentTool{
	Name: "list_neighbor_targets",
	Description: `列出目标主机所在业务组（同一个 group_id）下的其它机器，并补全每台的实时心跳，用于判断 "个体故障 vs 集群故障 vs 网络分区"。
返回 items 列表（ident / host_ip / lag_seconds / status）+ summary 聚合（total / active / lagging / stale）。
limit 默认 30，避免大业务组返回过多。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "ident", Type: "string", Description: "目标机器标识（ident），必填", Required: true},
		{Name: "limit", Type: "integer", Description: "邻居数量上限，默认 30，最大 100", Required: false},
	},
}

var ProbeTargetOnboardStatus = aiagent.AgentTool{
	Name: "probe_target_onboard_status",
	Description: `沿"接入链路 5 段"探测一台机器在 n9e 平台的接入足迹，用于排查"机器没出现 / 元信息 unknown / 心跳没建立"的接入失败问题。
与 get_target_realtime_status 的关键差异：本工具**容忍 target 不在 DB 中**——这正是 onboard 场景的常态。
返回字段：
- in_target_db / target{os, agent_version, host_ip, has_group}：段 3/4，DB 落库情况
- in_redis_beat / in_redis_meta / redis_meta{cpu_util, mem_util, remote_addr, hostname, offset}：段 4，redis 心跳与 meta
- datasource_checked / in_prom_target_up / target_up_last / prom_metrics_hit：段 5，时序库是否能查到该 ident
- likely_segment：诊断聚合，取值 segment_1_or_2 / segment_3 / segment_4 / segment_5 / ok
- likely_causes：likely_segment 对应的高频根因列表（带相关 issue 编号）
权限：target 在 DB 且有业务组归属时按组交集鉴权；target 不在 DB 或未归组时允许查询（onboard 排障必须能查"还没归过组"的机器）。
段 5（prom）需要 datasource_id：不传时按 chat-level params 兜底，仍取不到则跳过段 5，不报错。`,
	Type: aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "ident", Type: "string", Description: "机器标识（ident），必填", Required: true},
		{Name: "datasource_id", Type: "integer", Description: "Prometheus 数据源 ID（段 5 用）。不传时用 chat-level params 兜底；都没有则只跑段 3/4。", Required: false},
	},
}

// =============================================================================
// Task template
// =============================================================================

var ListTaskTpls = aiagent.AgentTool{
	Name:        "list_task_tpls",
	Description: "查询当前用户有权限的自愈脚本/任务模板列表",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配标题或标签", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

var GetTaskTplDetail = aiagent.AgentTool{
	Name:        "get_task_tpl_detail",
	Description: "获取单个自愈脚本/任务模板的详细信息",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "id", Type: "integer", Description: "任务模板ID", Required: true},
	},
}

// =============================================================================
// Team
// =============================================================================

var ListTeams = aiagent.AgentTool{
	Name:        "list_teams",
	Description: "查询当前用户可见的团队列表（自己所在的团队及自己创建的团队）",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词，匹配团队名称", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}

// =============================================================================
// User
// =============================================================================

var ListUsers = aiagent.AgentTool{
	Name:        "list_users",
	Description: "查询用户列表，支持关键词搜索（用户名、昵称、邮箱、手机号）",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "query", Type: "string", Description: "搜索关键词", Required: false},
		{Name: "limit", Type: "integer", Description: "返回数量限制，默认50，最大200", Required: false},
	},
}
