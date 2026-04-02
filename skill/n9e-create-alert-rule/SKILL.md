---
name: n9e-create-alert-rule
description: 在夜莺(n9e)环境中创建告警规则并关联通知规则。当用户要求创建告警规则、添加监控告警、配置告警策略时使用。
---

# 夜莺(n9e) 创建告警规则

在夜莺监控平台上创建告警规则并关联通知规则。支持 Prometheus、Elasticsearch、Loki、TDengine、ClickHouse、MySQL、PostgreSQL、Doris、OpenSearch、VictoriaLogs、Host 等数据源类型。

根据用户需要的数据源类型，读取 `datasources/` 目录下对应的文件获取 `rule_config` 结构：
- [datasources/prometheus.md](datasources/prometheus.md) - Prometheus 指标告警
- [datasources/loki.md](datasources/loki.md) - Loki 日志告警
- [datasources/elasticsearch.md](datasources/elasticsearch.md) - Elasticsearch / OpenSearch 日志告警
- [datasources/tdengine.md](datasources/tdengine.md) - TDengine 指标告警
- [datasources/clickhouse.md](datasources/clickhouse.md) - ClickHouse 指标/日志告警
- [datasources/mysql.md](datasources/mysql.md) - MySQL 指标告警
- [datasources/pgsql.md](datasources/pgsql.md) - PostgreSQL 指标告警
- [datasources/doris.md](datasources/doris.md) - Doris 日志告警
- [datasources/victorialogs.md](datasources/victorialogs.md) - VictoriaLogs 日志告警
- [datasources/host.md](datasources/host.md) - 机器监控告警

---

## 前置条件

用户需要提供：
- **n9e 地址**：如 `http://10.99.1.106:8003`
- **用户名/密码**：如 `root/root`
- **告警内容描述**：如 "host CPU 使用率超过 80%"

如果用户未提供以上信息，使用 AskUserQuestion 工具询问。

---

## 执行步骤

### 第一步：登录获取 Token

```
POST /api/n9e/auth/login
Content-Type: application/json
Body: {"username":"<用户名>","password":"<密码>"}
```

从响应中提取 `dat.access_token`，后续请求都带上 `Authorization: Bearer <token>`。

### 第二步：询问业务组

调用 API 获取业务组列表：

```
GET /api/n9e/busi-groups
Authorization: Bearer <token>
```

将返回的业务组列表通过 **AskUserQuestion** 工具展示给用户，让用户选择要创建告警规则的业务组。

### 第三步：根据用户描述构建告警规则

根据用户的告警需求，确定数据源类型，读取对应的 `datasources/*.md` 文件获取 `rule_config` 结构。

如果是 Prometheus 类型，可查询可用指标：

```
GET /api/n9e/proxy/<datasource_id>/api/v1/label/__name__/values
Authorization: Bearer <token>
```

调用创建 API（payload 必须是**数组**格式）：

```
POST /api/n9e/busi-group/<busi_group_id>/alert-rules
Authorization: Bearer <token>
Content-Type: application/json
Body: [<告警规则对象>]
```

### 第四步：询问并关联通知规则

获取通知规则列表：

```
GET /api/n9e/notify-rules
Authorization: Bearer <token>
```

将返回的通知规则列表通过 **AskUserQuestion** 工具展示给用户，让用户选择要关联的通知规则。

然后更新告警规则，添加 `notify_rule_ids`：

```
PUT /api/n9e/busi-group/<busi_group_id>/alert-rule/<rule_id>
Authorization: Bearer <token>
Content-Type: application/json
Body: { ...原有字段, "notify_rule_ids": [<选择的通知规则ID>] }
```

### 第五步：验证

```
GET /api/n9e/alert-rule/<rule_id>
Authorization: Bearer <token>
```

向用户输出创建结果摘要。

---

## 公共字段模板

所有数据源类型共享以下字段结构，`rule_config` 按数据源类型不同而不同：

```json
{
  "name": "告警规则名称",
  "note": "规则说明/告警通知内容",
  "prod": "metric|logging|host",
  "cate": "prometheus|elasticsearch|loki|tdengine|ck|mysql|pgsql|doris|opensearch|victorialogs|host",
  "datasource_ids": [1],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": [1]}],
  "disabled": 0,
  "prom_eval_interval": 30,
  "prom_for_duration": 60,
  "rule_config": {},
  "enable_in_bg": 0,
  "enable_days_of_weeks": [["0","1","2","3","4","5","6"]],
  "enable_stimes": ["00:00"],
  "enable_etimes": ["00:00"],
  "notify_recovered": 1,
  "notify_repeat_step": 60,
  "notify_max_number": 0,
  "callbacks": [],
  "runbook_url": "",
  "append_tags": [],
  "annotations": {},
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": []
}
```

### datasource_queries 匹配方式

| match_type | 含义 | 示例 |
|---|---|---|
| 0 | 精确匹配（按 ID 或名称） | `{"match_type": 0, "op": "in", "values": [1, 2]}` |
| 1 | 模糊匹配（按名称子串） | `{"match_type": 1, "op": "in", "values": ["prod"]}` |
| 2 | 匹配全部 | `{"match_type": 2, "op": "in", "values": ["all"]}` |

### severity 告警级别

| 值 | 含义 |
|---|---|
| 1 | 一级报警 (Critical) |
| 2 | 二级报警 (Warning) |
| 3 | 三级报警 (Info) |

### 通用 Trigger 结构（非 Host 类型通用）

```json
{
  "mode": 0,
  "expressions": [
    {"ref": "A", "comparisonOperator": ">", "value": 80, "logicalOperator": "&&"}
  ],
  "severity": 2,
  "recover_config": {"judge_type": 1}
}
```

- `mode`：0=构建器模式，1=表达式模式（使用 `exp` 字段，如 `"$A > 100 && $B < 50"`）
- `recover_config.judge_type`：0=日志类型，1=指标类型
- `comparisonOperator`：`>`, `>=`, `<`, `<=`, `==`, `!=`
- `logicalOperator`：`&&`, `||`

---

## 数据源类型速查表

| cate | prod | 查询字段 | recover judge_type | 说明 |
|---|---|---|---|---|
| `prometheus` | `metric` | `prom_ql`(v1) 或 `query`(v2) | 1 | PromQL 指标查询 |
| `loki` | `logging` | `prom_ql`（实际是 LogQL） | 0 | Loki 日志查询 |
| `elasticsearch` | `logging` | `index`+`filter`+`value` | 0 | ES 日志聚合 |
| `opensearch` | `logging` | 同 elasticsearch | 0 | 同 ES，无 index_pattern |
| `tdengine` | `metric` | `query`（TDengine SQL） | 1 | 时序数据库查询 |
| `ck` | `metric`/`logging` | `sql`（ClickHouse SQL） | 1 | ClickHouse 查询 |
| `mysql` | `metric` | `sql`（MySQL SQL） | 1 | MySQL 查询 |
| `pgsql` | `metric` | `sql`（PostgreSQL SQL） | 1 | PostgreSQL 查询 |
| `doris` | `logging` | `sql`+`database` | 1 | Doris 查询，需指定库名 |
| `victorialogs` | `logging` | `query`（LogsQL） | 0 | VictoriaLogs 查询 |
| `host` | `host` | `key`+`op`+`values` | N/A | 机器监控，trigger 结构不同 |

---

## 关键注意事项

1. **创建 API 接收数组**：即使只创建一条规则，payload 也必须是数组格式 `[{...}]`
2. **Prometheus v1 格式阈值写在 prom_ql 中**：如 `cpu_usage_active > 80`，不要把阈值放在 triggers 里
3. **Prometheus v2 格式查询和触发分离**：query 只写指标查询，triggers 配置阈值表达式
4. **日志类型 recover judge_type 用 0**：Loki、ES、OpenSearch、VictoriaLogs
5. **指标类型 recover judge_type 用 1**：Prometheus、TDengine、ClickHouse、MySQL、PostgreSQL
6. **Host 类型结构特殊**：queries 和 triggers 都使用专用结构，不走通用 trigger
7. **datasource_ids**：如不确定可用数据源 ID，先查询已有告警规则获取
8. **生效时间**：`enable_stimes` 和 `enable_etimes` 都设为 `"00:00"` 表示全天 24 小时生效
9. **notify_version**：使用 `1`（新版通知系统），配合 `notify_rule_ids` 关联通知规则
