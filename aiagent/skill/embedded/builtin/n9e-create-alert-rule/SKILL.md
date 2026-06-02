---
name: n9e-create-alert-rule
description: |
  **创建告警规则**。优先复用 integrations 里验证过的规则（标准组件 Linux/MySQL/Redis/Kafka/PostgreSQL/Elasticsearch 等都有现成规则包），导入几条按用户需求来——单条、一批、或整套都行；integration 里没有贴合的规则时再手写自定义规则。支持 Prometheus / Loki / ES / OpenSearch / MySQL / PG / TDengine / ClickHouse / Doris / VictoriaLogs / Host 全部数据源。
  ⚠️ **不要用这个 skill 做批量 YAML 导入**——用户给的是 URL 或 YAML 文件、awesome-prometheus-alerts、node-exporter.yml 之类，请改用 n9e-import-prom-rule。
  触发：创建一条/加一条告警 / 帮我建个 CPU 告警 / 给 MySQL 加套告警规则 / 给主机配上常用告警 / 我要监控某个指标。
examples:
  - "给主机配上一套常用告警规则"
  - "给 MySQL 加套告警规则"
  - "帮我建一条 CPU 使用率超过 80% 的告警"
  - "新增一条 MySQL 慢查询告警规则"
  - "给主机内存加个告警，超过 90% 报警"
max_iterations: 20
builtin_tools:
  - preview_alert_rule_template
  - import_alert_rule_template
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

两个工具：`import_alert_rule_template`（复用 integrations 里验证过的规则）和 `create_alert_rule`（手写规则）。

**优先复用 integration 里的规则**：`integrations/<组件>/alerts/` 下的规则是人工精调验证过的成品（级别、持续时长、评估周期、附加标签、注释、生效时间窗都配好了），质量远高于手搓。**但导入几条要按用户需求来，不是无脑整包**：

- 用户要给某组件配「一套 / 常用」告警 → 整包导入（不传 `names`）
- 用户只点名某几类（如"磁盘和内存告警"）→ 只导这几条（`names` 传这几条）
- 用户要某个具体指标的**单条**告警 → 如果包里正好有贴合的那条，就只导这一条（`names` 传一个）或参考它的表达式；包里没有贴合的，再用 `create_alert_rule` 手写

只有 integration 里没有贴合的规则、或用户给的是完全自定义的查询条件时，才走方式 B（`create_alert_rule`）。

同一组件常同时有 categraf 和 exporter 两套规则包时，**优先 categraf**：先探测 categraf 指标有没有数据，能查到就用 categraf 包，查不到才退回 exporter（详见方式 A 第 3 步）。

| 用户需求 | 怎么做 |
|------|-----------|
| 给标准组件配一套/几条告警（Linux / MySQL / Redis / Kafka / PostgreSQL / Elasticsearch / Ceph / Oracle / Windows / MongoDB …） | 方式 A：`preview_alert_rule_template` 看包里有啥 → `import_alert_rule_template` 按需导入 |
| 某个具体指标的单条告警，且 integration 包里有贴合的那条 | 方式 A：`import_alert_rule_template` + `names` 只导那一条 |
| 完全自定义、integration 里没有贴合的规则（含 Loki / ES / SQL 查询值 / 日志条数等冷门条件） | 方式 B：`create_alert_rule` |

> ⚠️ **不要用这个 skill 做批量 YAML 导入**——用户给的是 URL 或 YAML 文件、awesome-prometheus-alerts、node-exporter.yml 之类，请改用 n9e-import-prom-rule。

## 第一步（两条路径通用）：确定业务组和数据源

- 调用 `list_busi_groups` 获取业务组列表
- 调用 `list_datasources` 获取数据源列表，找到对应类型的数据源 ID（规则包多为 Prometheus 类型）
- 若会话上下文已预选 `busi_group_id` / `datasource_id`，直接用，**不要**再调 `list_*`
- `cate=host`（机器失联类）不需要数据源

### 业务组选择规则

1. 用户明确指定了业务组名称或 ID，直接使用
2. 否则按优先级：
   a. **优先 `is_default: true` 的业务组**（通常是 "Default Busi Group" 或含"默认"的组）
   b. 只有一个业务组时直接用
   c. 多个候选且都非默认时，**不要盲取第一个**，在回复里列出让用户确认

---

## 方式 A：复用 integration 里验证过的规则（按需：单条 / 一批 / 整包）

`import_alert_rule_template` 从 `integrations/<组件>/alerts/` 下的规则包里导入规则（每条规则的级别、持续时长、评估周期、附加标签、注释、生效时间窗全部保留），并把规则绑定到你选的数据源、统一改为启用态。**导入几条由 `names` 参数控制**，按用户需求来。

### 步骤

1. 看有哪些集成组件：
   ```
   list_files(base="integrations")
   ```
2. 看该组件下有哪些规则包文件：
   ```
   list_files(base="integrations/Linux", path="alerts")
   ```
3. **挑对规则包文件（categraf 优先）**。规则包常分 categraf 和 exporter 两种采集风格，文件名约定各组件不一（Linux 是 `linux_by_categraf.json` / `linux_by_exporter.json`，也可能是别的后缀）。**优先选文件名含 `categraf` 的包**：
   - 先探测 categraf 指标在环境里有没有数据：`list_metrics(datasource_id=<X>, keyword="<categraf 指标关键字>")`。
     - 关键字用能区分两种风格的 categraf 专有指标名，例如 Linux 用 `cpu_usage`（categraf 是 `cpu_usage_idle`，node_exporter 没有这个名字）、`mem_used_percent`、`disk_used_percent`；Redis 用 `redis_used_memory`；MySQL 用 `mysql_global_status_queries`。
   - **只要 categraf 指标能查到数据（返回非空数组），就直接用文件名含 `categraf` 的包**，不必再比较 exporter。
   - 只有 categraf 指标查不到数据时，才退回文件名含 `exporter` 的包（如 `linux_by_exporter.json`），并用对应的 exporter 指标关键字（如 `node_cpu_seconds_total`）`list_metrics` 确认有数据后再导入。
4. **看包里有哪些规则（仅当要按名字挑选时）**。用户要整包导入时可跳过这步；用户只要其中某几条、或要某个具体指标的单条告警时，先预览拿到准确的规则名：
   ```
   preview_alert_rule_template(component="Linux", file="linux_by_categraf.json")
   ```
   返回每条规则的 `name` / `cate` / `severity` / 表达式摘要 / 是否禁用（小载荷）。**不要**用 `read_file` 看大规则包（会截断），用这个工具。
5. **按需导入**：
   - 整包（用户要"一套/常用"告警）——不传 `names`：
     ```
     import_alert_rule_template(group_id=<业务组>, component="Linux", file="linux_by_categraf.json", datasource_id=<数据源>)
     ```
   - 一批 / 单条（用户点名某几类或某个指标）——`names` 传精确规则名（从 preview 拿）：
     ```
     import_alert_rule_template(group_id=<业务组>, component="Linux", file="linux_by_categraf.json",
       datasource_id=<数据源>, names="[\"Machine load - high memory, please pay attention - categraf\"]")
     ```

> **不要**用 `read_file` 把整个规则包读出来再逐条 `create_alert_rule`——包可能很大且会被截断，逐条转换还会丢字段。要看包里有啥用 `preview_alert_rule_template`，要落库用 `import_alert_rule_template`。

### 注意事项

- **`datasource_id` 强烈建议传**：传了就把导入的规则绑定到该数据源；不传则规则匹配该类型全部数据源（范围偏大）。包里的 `host` 心跳类规则不需要数据源，工具会自动跳过绑定。
- 规则包里的规则在模板里默认是**禁用态**（它是一份可浏览的目录）。本工具导入时**默认改为启用**（`disabled=0`），让规则立即生效。如果用户希望"先导入但不启用，自己逐条开"，传 `disabled=1`。
- `names` 里写的名字必须和 `preview_alert_rule_template` 返回的 `name` 完全一致；没匹配上的会在返回的 `not_found_names` 里列出，核对后重试。
- 同名规则（业务组里已存在）**自动跳过**（`status=skipped_duplicate`），不算失败，**不要**因为看到跳过就用 `name_prefix` 重导一遍。只有当用户明确要和现有规则并存（如对比测试）时才用 `name_prefix`/`name_suffix`。
- 返回里有 `created` / `skipped` / `failed` 三个计数和每条规则的明细，输出时汇总即可。
- 导入的规则默认**不绑定任何通知规则**（避免误发告警）。如需绑定，导入后提示用户去规则上手动关联，或改用方式 B 逐条建并传 `notify_rule_ids`。

---

## 方式 B：手写自定义规则（integration 里没有贴合规则时）

用 `create_alert_rule`。支持 **Prometheus / Loki / Elasticsearch / OpenSearch / TDengine / ClickHouse / MySQL / PostgreSQL / Doris / VictoriaLogs / Host** 全部数据源类型。

> 走到这里前先确认 integration 里确实没有贴合的规则：标准组件的常见指标（CPU/内存/磁盘/连接数…）多半在规则包里有，能复用就别手搓。手搓时也可以先 `preview_alert_rule_template` 看一眼有没有现成表达式可借鉴。

工具提供两种调用模式：

1. **Prometheus 简化路径**（最常用）——直接传 `prom_ql` + `threshold` + `operator`，工具自动构建规则
2. **通用路径**——传 `cate` + `rule_config_json`，`rule_config_json` 的结构**先通过 `read_file` 读取 `datasources/<cate>.md` 获取模板**，再填充实际值

### 模式 1：Prometheus 简化路径

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

### 模式 2：通用路径（非 Prometheus）

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

## create_alert_rule 参数说明（方式 B）

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

## 方式 B 详细步骤（create_alert_rule）

> 业务组和数据源的选择见上面「第一步（两条路径通用）」。下面是方式 B 特有的步骤。

### B-1：识别数据源类型（cate）

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

默认值：`prometheus`。数据源也要对应（`cate=mysql` 找 MySQL 数据源、`cate=loki` 找 Loki 数据源等），`cate=host` 不需要数据源。

### B-2（仅非 Prometheus）：读取数据源参考文档

对于 `cate != "prometheus"` 的类型，**必须先读该类型的参考文档**获取 `rule_config` 结构：

```
read_file(base="n9e-create-alert-rule", path="datasources/<cate>.md")
```

例如：
- MySQL → `datasources/mysql.md`
- Loki → `datasources/loki.md`
- Host → `datasources/host.md`

参考文档里有完整的字段说明、可选值表格、完整示例。**按示例里的 `rule_config` 对象照搬字段名**，只替换具体值。

### B-3（仅 SQL 类）：探测真实 schema ⚠️

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

只有在拿到真实 schema 后，才能进入 B-4 拼 SQL。

#### datasource_id 的传法

- 会话上下文已经绑定了数据源（从数据源页面进来的对话）：`datasource_id` 可省略，
  从会话上下文继承
- 告警规则场景（从告警规则页或纯对话进入）：必须**显式传** `datasource_id`，
  否则工具会报 `datasource_id required` 错误

### B-4：构建 PromQL / SQL / LogQL

#### 简化路径（cate=prometheus）

根据用户描述构建 **不带阈值** 的 PromQL。下表都是 **categraf（telegraf 风格）** 指标名，**优先用 categraf**：

| 用户需求 | 推荐 PromQL（categraf 优先） |
|---|---|
| 主机 CPU 使用率 | `avg by (ident) (100 - cpu_usage_idle{cpu="cpu-total"})` |
| 主机内存使用率 | `mem_used_percent` |
| 主机磁盘使用率 | `disk_used_percent` |
| 主机系统负载 | `system_load1` |
| 主机网络入向流量 | `rate(net_bytes_recv[5m])` |
| MySQL QPS | `rate(mysql_global_status_queries[5m])` |
| MySQL 连接数使用率 | `mysql_global_status_threads_connected / mysql_global_variables_max_connections * 100` |
| Redis 内存使用率 | `redis_used_memory / redis_maxmemory * 100` |

**categraf 优先 + 探测**：先用 `list_metrics` 确认 categraf 指标在环境里有没有数据，例如
`list_metrics(datasource_id=<X>, keyword="cpu_usage")`（categraf 是 `cpu_usage_idle`，node_exporter 没有这个名字）。
- **能查到数据** → 直接用上表的 categraf PromQL。
- **查不到** → 环境里很可能用的是 node_exporter，改用下面的 exporter 等价表达式，并再用
  `list_metrics(datasource_id=<X>, keyword="node_")` 确认 node_exporter 指标确实有数据后再建规则。

##### node_exporter 回退等价表（仅 categraf 指标查不到时用）

| 用户需求 | node_exporter 等价 PromQL |
|---|---|
| 主机 CPU 使用率 | `100 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100` |
| 主机内存使用率 | `(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100` |
| 主机磁盘使用率 | `(1 - node_filesystem_avail_bytes{fstype!~"tmpfs\|overlay"} / node_filesystem_size_bytes) * 100` |
| 主机系统负载 | `node_load1` |
| 主机网络入向流量 | `rate(node_network_receive_bytes_total[5m])` |

> 磁盘表达式里的 `\|` 只是为了不破坏 Markdown 表格的转义，写进 PromQL 时要还原成 `|`，即 `fstype!~"tmpfs|overlay"`（否则正则会把它当成字面竖线，过滤失效）。

#### 通用路径（其他 cate）

按照 B-2 读到的参考文档，组装 `rule_config` 对象。关键字段：

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

### B-5：调用 create_alert_rule

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
- **SQL 类调用前必须已经完成 B-3 的 schema 探测**。如果 rule_config_json 里的
  数据库/表/字段没经过 `list_databases`+`list_tables`+`describe_table` 核实过，
  告警规则大概率无法运行（`Unknown database` / `Unknown column` 等运行时错误）

### B-6：（可选）关联通知规则

如果用户要求绑定通知规则：
1. 调用 `list_notify_rules` 获取通知规则列表
2. 调用 `create_alert_rule` 时传入 `notify_rule_ids`，如 `"[1,2]"`

如果用户没明确要求，**不要主动关联通知规则**，避免误发告警。

---

## 最后一步（两条路径通用）：输出结果

**保持简短**。一句话确认即可：

- 方式 A（导入规则包）：汇总 `created` / `skipped` / `failed` 三个计数，例如
  > ✅ 已从 Linux categraf 规则包为您导入 8 条告警规则（跳过 1 条同名），均已启用，详情见下方卡片。
- 方式 B（单条创建）：
  > ✅ 已为您创建告警规则「主机 CPU 使用率过高」，详情请查看下方卡片。

**不要**在 Final Answer 里复述规则 ID、业务组、数据源、PromQL、阈值、告警级别等字段——前端会以结构化卡片展示这些信息，重复输出只会让用户看到两份。如果需要补充说明（例如"这些规则默认未绑定通知规则，请按需关联"），可以再加一两句，但不要把卡片里已有的字段逐条列出来。

## 严重级别速查

| severity | 中文 | 适用场景 |
|---|---|---|
| 1 | Critical（一级） | 服务不可用、核心组件宕机、数据丢失 |
| 2 | Warning（二级） | 资源使用率高、性能退化、配额接近上限 |
| 3 | Info（三级） | 信息性告警、容量规划提示 |

默认用 2（Warning）。除非用户明确说"严重"或"紧急"才用 1。

## 单条告警常用模板（方式 B 参考）

> 用户要的是某个具体指标的单条告警时参考这些。如果用户要的是「给主机加一整套监控告警」，
> 优先走方式 A 导入 `integrations/Linux/alerts/linux_by_categraf.json` 整包，别在这里一条条手搓。

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
