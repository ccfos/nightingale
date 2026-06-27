---
name: query-datasource
description: Query data from various datasources in a Nightingale (n9e) environment. Supports Prometheus metric queries, Elasticsearch/Loki log queries, and SQL datasource queries such as ClickHouse/MySQL/PostgreSQL/TDengine/Doris. Use when the user asks to query metrics, view monitoring data, search logs, or run PromQL or SQL queries.
tags:
  - internal
---

# Nightingale (n9e) Query Datasource Data

Query monitoring metrics, logs, and time-series data from various datasources on the Nightingale monitoring platform.

Based on the datasource type the user needs, read the corresponding file under the `datasources/` directory to get the query method and parameter format:
- [datasources/prometheus.md](datasources/prometheus.md) - Prometheus / VictoriaMetrics metric queries (PromQL)
- [datasources/elasticsearch.md](datasources/elasticsearch.md) - Elasticsearch log queries (ES DSL / Lucene)
- [datasources/loki.md](datasources/loki.md) - Loki log queries (LogQL)
- [datasources/clickhouse.md](datasources/clickhouse.md) - ClickHouse metric/log queries (SQL)
- [datasources/mysql.md](datasources/mysql.md) - MySQL metric queries (SQL)
- [datasources/pgsql.md](datasources/pgsql.md) - PostgreSQL metric queries (SQL)
- [datasources/tdengine.md](datasources/tdengine.md) - TDengine time-series queries (SQL)
- [datasources/doris.md](datasources/doris.md) - Doris log queries (SQL)
- [datasources/opensearch.md](datasources/opensearch.md) - OpenSearch log queries (ES DSL)
- [datasources/victorialogs.md](datasources/victorialogs.md) - VictoriaLogs log queries (LogsQL)

---

## Prerequisites

The user needs to provide:
- **n9e address**: e.g. `http://<n9e-host>:<port>`
- **Username/password**: e.g. `<username>/<password>`
- **Query requirement description**: e.g. "query CPU usage over the last hour", "search logs containing error"

If the user has not provided the above information, use the AskUserQuestion tool to ask.

---

## Execution Steps

### Step 1: Log in to obtain a Token

```
POST /api/n9e/auth/login
Content-Type: application/json
Body: {"username":"<username>","password":"<password>"}
```

Extract `dat.access_token` from the response, and include `Authorization: Bearer <token>` in all subsequent requests.

### Step 2: Query available datasources

Retrieve the datasource list to determine the datasource ID and type to query:

```
POST /api/n9e/datasource/list
Authorization: Bearer <token>
Content-Type: application/json
Body: {}
```

Each datasource in the response contains `id`, `name`, and `plugin_type`.

If the user has not specified a datasource, use the **AskUserQuestion** tool to display the available datasources and let the user choose.

### Step 3: Run the query based on the datasource type

Based on the datasource's `plugin_type`, read the corresponding `datasources/*.md` file to get the query API and parameter format.

### Step 4: Format the output

Present the query result to the user as a readable Markdown table or list.

---

## Datasource Type Quick Reference

| plugin_type | Datasource | Query Language | Use Case | Reference File |
|---|---|---|---|---|
| `prometheus` | Prometheus / VictoriaMetrics | PromQL | Metric/time-series query | [prometheus.md](datasources/prometheus.md) |
| `elasticsearch` | Elasticsearch | ES DSL / Lucene | Log query | [elasticsearch.md](datasources/elasticsearch.md) |
| `opensearch` | OpenSearch | ES DSL / Lucene | Log query | [opensearch.md](datasources/opensearch.md) |
| `loki` | Loki | LogQL | Log query | [loki.md](datasources/loki.md) |
| `ck` | ClickHouse | SQL | Metric/log query | [clickhouse.md](datasources/clickhouse.md) |
| `mysql` | MySQL | SQL | Metric query | [mysql.md](datasources/mysql.md) |
| `pgsql` | PostgreSQL | SQL | Metric query | [pgsql.md](datasources/pgsql.md) |
| `tdengine` | TDengine | SQL | Time-series query | [tdengine.md](datasources/tdengine.md) |
| `doris` | Doris | SQL | Log query | [doris.md](datasources/doris.md) |
| `victorialogs` | VictoriaLogs | LogsQL | Log query | [victorialogs.md](datasources/victorialogs.md) |

---

## Generic Proxy API

All datasources can access their native APIs through the generic proxy:

```
<ANY_METHOD> /api/n9e/proxy/<datasource_id>/<native API path>
Authorization: Bearer <token>
```

For example:
- Prometheus: `/api/n9e/proxy/1/api/v1/query?query=up`
- Elasticsearch: `/api/n9e/proxy/2/_cat/health`
- Loki: `/api/n9e/proxy/3/loki/api/v1/labels`

---

## Generic Time-Series Query API

All datasources (except Prometheus) can use the unified time-series query endpoint:

```
POST /api/n9e/ds-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "<plugin_type>",
  "datasource_id": 1,
  "query": [<query object>]
}
```

Generic log query endpoint:

```
POST /api/n9e/logs-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "<plugin_type>",
  "datasource_id": 1,
  "query": [<query object>]
}
```

The exact structure of the query object varies by datasource type; see each datasource file for details.

---

## Generic Metadata API for SQL-type Datasources

ClickHouse, MySQL, PostgreSQL, and Doris share the following metadata query endpoints:

```
POST /api/n9e/db-databases     // List databases
POST /api/n9e/db-tables        // List tables
POST /api/n9e/db-desc-table    // View table structure
```

TDengine uses dedicated endpoints:

```
POST /api/n9e/tdengine-databases
POST /api/n9e/tdengine-tables
POST /api/n9e/tdengine-columns
```

---

## Key Considerations

1. **Query the datasource list first to get the ID**: All queries require a `datasource_id`; obtain it first via `POST /api/n9e/datasource/list`
2. **SQL queries are read-only**: SQL-type datasources prohibit write operations such as CREATE, INSERT, UPDATE, DELETE, ALTER, DROP
3. **Time variables**: In SQL queries, use `$from` and `$to` to represent the time range; the system replaces them automatically
4. **keys field**: Time-series queries must specify `valueKey` (numeric column) and `labelKey` (grouping column); separate multiple columns with spaces
5. **Unified response format**: All API responses are wrapped in a `{"dat": <data>}` structure
