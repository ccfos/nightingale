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
// File
// =============================================================================

var ListFiles = aiagent.AgentTool{
	Name:        "list_files",
	Description: "列出技能或集成模板目录下的文件和子目录",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "base", Type: "string", Description: "基础目录名: 技能名(如 n9e-create-dashboard)或 integrations/分类(如 integrations/Linux)", Required: true},
		{Name: "path", Type: "string", Description: "相对子路径，如 panels 或 dashboards。不传则列出根目录", Required: false},
	},
}

var ReadFile = aiagent.AgentTool{
	Name:        "read_file",
	Description: "读取技能文档或集成模板文件内容",
	Type:        aiagent.ToolTypeBuiltin,
	Parameters: []aiagent.ToolParameter{
		{Name: "base", Type: "string", Description: "基础目录名: 技能名(如 n9e-create-dashboard)或 integrations/分类(如 integrations/Linux)", Required: true},
		{Name: "path", Type: "string", Description: "相对文件路径，如 panels/timeseries.md 或 dashboards/categraf-detail.json", Required: true},
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
