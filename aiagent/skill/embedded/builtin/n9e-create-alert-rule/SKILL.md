---
name: n9e-create-alert-rule
description: 在夜莺(n9e)平台上创建告警规则。支持 Prometheus/Loki/ES/MySQL/TDengine/ClickHouse/Doris/Host 等所有数据源类型。
max_iterations: 20
builtin_tools:
  - create_alert_rule
  - list_busi_groups
  - list_datasources
  - list_metrics
  - list_notify_rules
  - list_files
  - read_file
  - list_databases
  - list_tables
  - describe_table
---

# Skill: 夜莺(N9E) 告警规则创建

## 概述

使用 `create_alert_rule` 工具创建告警规则。支持 **Prometheus / Loki / Elasticsearch / OpenSearch / TDengine / ClickHouse / MySQL / PostgreSQL / Doris / VictoriaLogs / Host** 全部数据源类型。

工具提供两种调用模式：

1. **Prometheus 简化路径**（最常用）——直接传 `prom_ql` + `threshold` + `operator`，工具自动构建 v2 rule_config
2. **通用路径**——传 `cate` + `rule_config_json`，`rule_config_json` 的结构**先通过 `read_file` 读取 `datasources/<cate>.md` 获取模板**，再填充实际值

## 模式 1：Prometheus 简化路径

最常用的场景。只需填 PromQL、阈值、操作符。工具内部会把阈值拼进 `prom_ql` 生成 v1 格式的规则（OSS n9e 的 FE v2 编辑器被 `IS_PLUS` 门控，只能用 v1）。

```json
{
  "group_id": 1,
  "name": "CPU 使用率过高",
  "datasource_id": 1,
  "prom_ql": "avg by (ident) (100 - cpu_usage_idle{cpu=\"cpu-total\"})",
  "operator": ">",
  "threshold": 80,
  "severity": 2,
  "note": "CPU 使用率超过 80% 持续 1 分钟",
  "for_duration": 60
}
```

## 模式 2：通用路径（非 Prometheus）

对 Loki / ES / MySQL / TDengine / ClickHouse / Doris / VictoriaLogs / Host 等类型，需传 `cate` 和 `rule_config_json`。

**关键步骤**：先读该数据源类型的参考文档，获取 `rule_config` 结构模板：

```
read_file(base="n9e-create-alert-rule", path="datasources/<cate>.md")
```

然后把文档中的 `rule_config` 对象转成 JSON 字符串传给 `rule_config_json` 参数。示例（MySQL 告警）：

```json
{
  "group_id": 1,
  "name": "MySQL 失败订单过多",
  "cate": "mysql",
  "datasource_id": 4,
  "severity": 2,
  "rule_config_json": "{\"queries\":[{\"ref\":\"A\",\"sql\":\"SELECT count(*) AS value FROM orders WHERE created_at >= NOW() - INTERVAL 5 MINUTE AND status='failed'\",\"keys\":{\"valueKey\":\"value\",\"labelKey\":\"\"},\"interval\":60}],\"triggers\":[{\"mode\":0,\"expressions\":[{\"ref\":\"A\",\"comparisonOperator\":\">\",\"value\":10,\"logicalOperator\":\"&&\"}],\"severity\":2,\"recover_config\":{\"judge_type\":1}}]}"
}
```

**⚠️ 重要通用规则（所有非 Prometheus 类型都适用）**：

1. **`interval` 字段必须是总秒数**，不要写 `interval_unit`。前端保存时会把 `value × unit → seconds`，读取时再从秒数反推显示单位。
   - 过去 1 分钟 → `"interval": 60`
   - 过去 5 分钟 → `"interval": 300`
   - 过去 1 小时 → `"interval": 3600`
   - **不要** 写 `"interval": 5, "interval_unit": "min"` —— FE 会按 5 秒显示。
   - 工具内部有防御式兜底：如果你不小心写了 `interval_unit` 或小于 60 的裸 interval，会自动换算成秒，但最好直接写对。

**⚠️ OSS n9e 的 SQL 类数据源（MySQL/PGSQL/CK/Doris）重要限制**：
1. **`keys.valueKey` 必填** —— SELECT 语句中数值列的别名，通常是 `"value"`。缺失会报 `valueKey is required`。
2. **`$from`/`$to`/`$__timeFilter` 不会被替换** —— OSS 的 `macros.Macro` 是 no-op。必须用数据源原生时间函数，例如 MySQL 用 `NOW() - INTERVAL 5 MINUTE`、PG 用 `NOW() - INTERVAL '5 minutes'`、CK 用 `now() - INTERVAL 5 MINUTE`。
3. **TDengine 是例外** —— 它有独立的变量替换逻辑，`$from`/`$to`/`$interval` 可用。

## 参数说明

| 字段 | 必填 | 说明 |
|---|---|---|
| `group_id` | ✅ | 业务组 ID（从 `list_busi_groups` 获取） |
| `name` | ✅ | 规则名称（同业务组内不能重名） |
| `cate` | ❌ | 数据源类型（默认 `prometheus`）。可选：`prometheus` / `loki` / `elasticsearch` / `opensearch` / `tdengine` / `ck` / `mysql` / `pgsql` / `doris` / `victorialogs` / `host` |
| `prod` | ❌ | 产品类型。不传时按 `cate` 自动推导 |
| `datasource_id` | 条件必填 | 数据源 ID。**`cate=host` 时不需要**，其他都必填 |
| `rule_config_json` | 条件必填 | 完整的 `rule_config` JSON 字符串。**`cate=prometheus` 时可以不传（走简化路径）**；其他类型必填 |
| `prom_ql` | 条件必填 | PromQL 查询（仅 `cate=prometheus` 简化路径用）。**只写查询不写阈值** |
| `threshold` | 条件必填 | 触发阈值（仅简化路径用） |
| `operator` | ❌ | 默认 `>`。可选 `>` `>=` `<` `<=` `==` `!=` |
| `severity` | ❌ | 默认 2。1=Critical, 2=Warning, 3=Info |
| `note` | ❌ | 告警通知正文 |
| `eval_interval` | ❌ | 评估周期（秒），默认 30 |
| `for_duration` | ❌ | 持续时长（秒），默认 60 |
| `append_tags` | ❌ | 附加标签，多个用空格分隔 |
| `runbook_url` | ❌ | 应急处理手册 URL |
| `notify_rule_ids` | ❌ | 关联通知规则 ID 列表 JSON，如 `"[1,2]"` |

## 执行步骤

### 第一步：识别数据源类型（cate）

根据用户的告警需求判断应该用哪个 `cate`：

| 用户诉求关键词 | cate | 触发条件 |
|---|---|---|
| "主机 CPU/内存/磁盘"、"Prometheus 指标" | `prometheus` | 指标阈值 |
| "机器失联"、"节点离线" | `host` | 心跳超时 |
| "应用错误日志"、"Loki 日志" | `loki` | 日志条数 |
| "ES 日志"、"Elasticsearch 聚合" | `elasticsearch` | 日志聚合 |
| "OpenSearch 日志" | `opensearch` | 日志聚合（同 ES） |
| "MySQL 查询结果异常" | `mysql` | SQL 查询值 |
| "PostgreSQL 查询结果异常" | `pgsql` | SQL 查询值 |
| "ClickHouse 指标/日志" | `ck` | SQL 查询值 |
| "Doris 日志" | `doris` | SQL 查询值 |
| "TDengine 时序数据" | `tdengine` | SQL 查询值 |
| "VictoriaLogs 日志" | `victorialogs` | LogsQL 查询 |

默认值：`prometheus`。

### 第二步：查询业务组和数据源

- 调用 `list_busi_groups` 获取业务组列表
- **如果 `cate != "host"`**：调用 `list_datasources` 找到对应类型的数据源 ID

#### 业务组选择规则

1. 如果用户在提示词里明确指定了业务组，直接使用
2. 否则按以下优先级选择：
   a. 优先选择 `is_default: true` 的业务组
   b. 只有一个业务组时直接用
   c. 多个候选都不是默认组时，列出选项让用户确认
3. 不要使用看起来是测试组的业务组（纯数字、`test`、`demo`、`tmp`）

### 第三步（仅非 Prometheus）：读取数据源参考文档

对于 `cate != "prometheus"` 的类型，**必须先读该类型的参考文档**获取 `rule_config` 结构：

```
read_file(base="n9e-create-alert-rule", path="datasources/<cate>.md")
```

例如：
- MySQL → `datasources/mysql.md`
- Loki → `datasources/loki.md`
- Host → `datasources/host.md`

参考文档里有完整的字段说明、可选值表格、完整示例。**按示例里的 `rule_config` 对象照搬字段名**，只替换具体值。

### 第 3.5 步（仅 SQL 类）：探测真实 schema ⚠️

**`cate ∈ {mysql, pgsql, ck, doris, tdengine}` 时这一步必须做，不可跳过**。

> **TDengine 也支持**：虽然 TDengine 是时序数据库，但它的 REST API 支持 `SHOW DATABASES` / `information_schema.ins_tables` / `information_schema.ins_columns`。`list_databases` / `list_tables` / `describe_table` 对 TDengine 完全可用。

**绝对不要凭用户提示词里的字面描述猜数据库/表/字段名**。用户说"查 my_logs 库的 access_log 表"
很可能只是举例，实际数据源里根本没这个库或这个表。

必须按顺序探测：

1. **`list_databases(datasource_id=<X>)`** —— 看真实的数据库列表
   - 对比用户描述，找最贴近的一个
   - 如果用户描述的库名**不在返回列表里**，**不要硬写进 SQL**。用 AskUserQuestion
     列出真实库名让用户确认，或选一个看起来相关的让用户认可

2. **`list_tables(datasource_id=<X>, database=<挑中的库>)`** —— 看真实的表列表
   - 同上：用户说的表不在列表里 → 列出真实表让用户选

3. **`describe_table(datasource_id=<X>, database=<库>, table=<表>)`** —— 拿真实字段
   - 用返回的字段名拼 SQL，**不要编造** `log_time`、`status`、`created_at` 之类
     常见字段名（它们未必真的存在于这张表）
   - 时间字段特别要注意：不同表里可能叫 `ts`、`event_time`、`log_time`、`create_time`、
     `timestamp` 等等，必须用 `describe_table` 返回的字段名决定

只有在拿到真实 schema 后，才能进入第四步拼 SQL。

#### datasource_id 的传法

- 会话上下文已经绑定了数据源（从数据源页面进来的对话）：`datasource_id` 可省略，
  从会话上下文继承
- 告警规则场景（从告警规则页或纯对话进入）：必须**显式传** `datasource_id`，
  否则工具会报 `datasource_id required` 错误

### 第四步：构建 PromQL / SQL / LogQL

#### 简化路径（cate=prometheus）

根据用户描述构建 **不带阈值** 的 PromQL：

| 用户需求 | 推荐 PromQL |
|---|---|
| 主机 CPU 使用率 | `avg by (ident) (100 - cpu_usage_idle{cpu="cpu-total"})` |
| 主机内存使用率 | `mem_used_percent` |
| 主机磁盘使用率 | `disk_used_percent` |
| 主机系统负载 | `system_load1` |
| 主机网络入向流量 | `rate(net_bytes_recv[5m])` |
| MySQL QPS | `rate(mysql_global_status_queries[5m])` |
| MySQL 连接数使用率 | `mysql_global_status_threads_connected / mysql_global_variables_max_connections * 100` |
| Redis 内存使用率 | `redis_used_memory / redis_maxmemory * 100` |

如不确定指标是否存在，调用 `list_metrics` 探测。

#### 通用路径（其他 cate）

按照第三步读到的参考文档，组装 `rule_config` 对象。关键字段：

- **SQL 类**（mysql/pgsql/ck/doris）：
  - 查询必须返回一列别名为 `value` 作为告警判断值
  - **必填** `keys.valueKey: "value"`（缺失 → `valueKey is required` 错误）
  - **时间过滤不能用 `$from`/`$to`**，要用数据源原生时间函数：
    - MySQL: `NOW() - INTERVAL 5 MINUTE`
    - PostgreSQL: `NOW() - INTERVAL '5 minutes'`
    - ClickHouse: `now() - INTERVAL 5 MINUTE`
    - Doris: `NOW() - INTERVAL 5 MINUTE`
- **TDengine**：特殊例外，`$from`/`$to`/`$interval` **可以用**，用 `keys.metricKey` 而不是 `keys.valueKey`
- **日志类**（loki/es/opensearch/victorialogs）：聚合查询返回数值；`recover_config.judge_type` 用 `0`
- **指标类**（prometheus/mysql/pgsql/ck/tdengine）：`recover_config.judge_type` 用 `1`
- **Host 类**（host）：`queries` 是 `{key, op, values}` 结构；`triggers` 是 `{type, severity, duration}` 结构；不需要 `datasource_id`

### 第五步：调用 create_alert_rule

#### 简化路径（Prometheus）

```
create_alert_rule(
  group_id=1, name="CPU 过高", datasource_id=1,
  prom_ql="avg by (ident) (100 - cpu_usage_idle{cpu=\"cpu-total\"})",
  threshold=80, operator=">", severity=2
)
```

#### 通用路径（其他类型）

```
create_alert_rule(
  group_id=1, name="MySQL 错误过多",
  cate="mysql", datasource_id=4, severity=2,
  rule_config_json="<把第三步读到的 rule_config 序列化成 JSON 字符串>"
)
```

**注意事项：**
- Prometheus 简化路径下，阈值单独传 `threshold`，**不要写进 `prom_ql`**
- 默认 `for_duration=60`，比 `eval_interval=30` 大才合理
- 如果用户要求"立即触发"，把 `for_duration` 设为 0
- 如果 `create_alert_rule` 返回 `already exists` 错误，**不要调用 `list_alert_rules`** 去查重名，直接给名字加后缀（如 `-v2`、`-AI` 或时间戳）重试
- 通用路径下，`rule_config_json` 必须是 **JSON 字符串**（不是对象），记得对引号做转义
- **SQL 类调用前必须已经完成第 3.5 步的 schema 探测**。如果 rule_config_json 里的
  数据库/表/字段没经过 `list_databases`+`list_tables`+`describe_table` 核实过，
  告警规则大概率无法运行（`Unknown database` / `Unknown column` 等运行时错误）

### 第六步：（可选）关联通知规则

如果用户要求绑定通知规则：
1. 调用 `list_notify_rules` 获取通知规则列表
2. 调用 `create_alert_rule` 时传入 `notify_rule_ids`，如 `"[1,2]"`

如果用户没明确要求，**不要主动关联通知规则**，避免误发告警。

### 第七步：输出结果

**保持简短**。只需一句话确认告警规则已创建，例如：

> ✅ 已为您创建告警规则「主机 CPU 使用率过高」，详情请查看下方卡片。

**不要**在 Final Answer 里复述规则 ID、业务组、数据源、PromQL、阈值、告警级别等字段——前端会以结构化卡片展示这些信息，重复输出只会让用户看到两份。如果需要补充说明（例如"已按您的要求关联了 XX 通知规则"或"建议稍后手动检查一下标签"），可以再加一两句，但不要把卡片里已有的字段逐条列出来。

## 严重级别速查

| severity | 中文 | 适用场景 |
|---|---|---|
| 1 | Critical（一级） | 服务不可用、核心组件宕机、数据丢失 |
| 2 | Warning（二级） | 资源使用率高、性能退化、配额接近上限 |
| 3 | Info（三级） | 信息性告警、容量规划提示 |

默认用 2（Warning）。除非用户明确说"严重"或"紧急"才用 1。

## 常见模板

### 主机 CPU 使用率告警
```json
{
  "group_id": 1,
  "name": "主机 CPU 使用率过高",
  "datasource_id": 1,
  "prom_ql": "avg by (ident) (100 - cpu_usage_idle{cpu=\"cpu-total\"})",
  "operator": ">",
  "threshold": 80,
  "severity": 2,
  "for_duration": 300,
  "note": "主机 {{ $labels.ident }} CPU 使用率持续 5 分钟超过 80%"
}
```

### 主机内存使用率告警
```json
{
  "group_id": 1,
  "name": "主机内存使用率过高",
  "datasource_id": 1,
  "prom_ql": "mem_used_percent",
  "operator": ">",
  "threshold": 90,
  "severity": 1,
  "for_duration": 180
}
```

### 主机磁盘使用率告警
```json
{
  "group_id": 1,
  "name": "主机磁盘使用率过高",
  "datasource_id": 1,
  "prom_ql": "disk_used_percent",
  "operator": ">",
  "threshold": 85,
  "severity": 2,
  "for_duration": 600,
  "append_tags": "alert_type=capacity"
}
```
