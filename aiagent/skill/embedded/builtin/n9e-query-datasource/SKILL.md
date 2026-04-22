---
name: n9e-query-datasource
description: 在夜莺(n9e)环境中查询各种数据源的数据。支持 Prometheus 指标查询、Elasticsearch/Loki 日志查询、ClickHouse/MySQL/PostgreSQL/TDengine/Doris 等 SQL 数据源查询。当用户要求查询指标、查看监控数据、搜索日志、执行 PromQL 或 SQL 查询时使用。
---

# 夜莺(n9e) 查询数据源数据

在夜莺监控平台上查询各种数据源的监控指标、日志和时序数据。

根据用户需要的数据源类型，读取 `datasources/` 目录下对应的文件获取查询方式和参数格式：
- [datasources/prometheus.md](datasources/prometheus.md) - Prometheus / VictoriaMetrics 指标查询（PromQL）
- [datasources/elasticsearch.md](datasources/elasticsearch.md) - Elasticsearch 日志查询（ES DSL / Lucene）
- [datasources/loki.md](datasources/loki.md) - Loki 日志查询（LogQL）
- [datasources/clickhouse.md](datasources/clickhouse.md) - ClickHouse 指标/日志查询（SQL）
- [datasources/mysql.md](datasources/mysql.md) - MySQL 指标查询（SQL）
- [datasources/pgsql.md](datasources/pgsql.md) - PostgreSQL 指标查询（SQL）
- [datasources/tdengine.md](datasources/tdengine.md) - TDengine 时序查询（SQL）
- [datasources/doris.md](datasources/doris.md) - Doris 日志查询（SQL）
- [datasources/opensearch.md](datasources/opensearch.md) - OpenSearch 日志查询（ES DSL）
- [datasources/victorialogs.md](datasources/victorialogs.md) - VictoriaLogs 日志查询（LogsQL）

---

## 前置条件

用户需要提供：
- **n9e 地址**：如 `http://<n9e-host>:<port>`
- **用户名/密码**：如 `<username>/<password>`
- **查询需求描述**：如 "查询最近1小时的 CPU 使用率"、"搜索包含 error 的日志"

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

### 第二步：查询可用数据源

获取数据源列表，确定要查询的数据源 ID 和类型：

```
POST /api/n9e/datasource/list
Authorization: Bearer <token>
Content-Type: application/json
Body: {}
```

响应中每个数据源包含 `id`、`name`、`plugin_type`。

如果用户未指定数据源，通过 **AskUserQuestion** 工具展示可用数据源让用户选择。

### 第三步：根据数据源类型执行查询

根据数据源的 `plugin_type`，读取对应的 `datasources/*.md` 文件获取查询 API 和参数格式。

### 第四步：格式化输出

将查询结果以可读的 Markdown 表格或列表形式展示给用户。

---

## 数据源类型速查表

| plugin_type | 数据源 | 查询语言 | 适用场景 | 参考文件 |
|---|---|---|---|---|
| `prometheus` | Prometheus / VictoriaMetrics | PromQL | 指标/时序查询 | [prometheus.md](datasources/prometheus.md) |
| `elasticsearch` | Elasticsearch | ES DSL / Lucene | 日志查询 | [elasticsearch.md](datasources/elasticsearch.md) |
| `opensearch` | OpenSearch | ES DSL / Lucene | 日志查询 | [opensearch.md](datasources/opensearch.md) |
| `loki` | Loki | LogQL | 日志查询 | [loki.md](datasources/loki.md) |
| `ck` | ClickHouse | SQL | 指标/日志查询 | [clickhouse.md](datasources/clickhouse.md) |
| `mysql` | MySQL | SQL | 指标查询 | [mysql.md](datasources/mysql.md) |
| `pgsql` | PostgreSQL | SQL | 指标查询 | [pgsql.md](datasources/pgsql.md) |
| `tdengine` | TDengine | SQL | 时序查询 | [tdengine.md](datasources/tdengine.md) |
| `doris` | Doris | SQL | 日志查询 | [doris.md](datasources/doris.md) |
| `victorialogs` | VictoriaLogs | LogsQL | 日志查询 | [victorialogs.md](datasources/victorialogs.md) |

---

## 通用代理 API

所有数据源都可以通过通用代理访问其原生 API：

```
<ANY_METHOD> /api/n9e/proxy/<datasource_id>/<原生API路径>
Authorization: Bearer <token>
```

例如：
- Prometheus: `/api/n9e/proxy/1/api/v1/query?query=up`
- Elasticsearch: `/api/n9e/proxy/2/_cat/health`
- Loki: `/api/n9e/proxy/3/loki/api/v1/labels`

---

## 通用时序查询 API

所有数据源（Prometheus 除外）都可以使用统一的时序查询接口：

```
POST /api/n9e/ds-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "<plugin_type>",
  "datasource_id": 1,
  "query": [<查询对象>]
}
```

通用日志查询接口：

```
POST /api/n9e/logs-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "<plugin_type>",
  "datasource_id": 1,
  "query": [<查询对象>]
}
```

查询对象的具体结构因数据源类型而异，详见各数据源文件。

---

## SQL 类数据源通用元数据 API

ClickHouse、MySQL、PostgreSQL、Doris 共享以下元数据查询端点：

```
POST /api/n9e/db-databases     // 列出数据库
POST /api/n9e/db-tables        // 列出表
POST /api/n9e/db-desc-table    // 查看表结构
```

TDengine 使用独立端点：

```
POST /api/n9e/tdengine-databases
POST /api/n9e/tdengine-tables
POST /api/n9e/tdengine-columns
```

---

## 关键注意事项

1. **先查询数据源列表获取 ID**：所有查询都需要 `datasource_id`，先通过 `POST /api/n9e/datasource/list` 获取
2. **SQL 查询只读**：SQL 类数据源禁止 CREATE、INSERT、UPDATE、DELETE、ALTER、DROP 等写操作
3. **时间变量**：SQL 查询中用 `$from` 和 `$to` 表示时间范围，系统会自动替换
4. **keys 字段**：时序查询需指定 `valueKey`（数值列）和 `labelKey`（分组列），多列用空格分隔
5. **响应格式统一**：所有 API 响应都包裹在 `{"dat": <data>}` 结构中
