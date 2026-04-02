# OpenSearch 查询

- **plugin_type**: `opensearch`
- **查询语言**: Elasticsearch DSL / Lucene（与 Elasticsearch 兼容）
- **适用场景**: 日志查询

OpenSearch 的查询方式与 Elasticsearch 完全相同，使用独立的 API 端点。

---

## 获取索引列表

```
POST /api/n9e/os-indices
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "opensearch",
  "datasource_id": 1
}
```

## 获取索引字段

```
POST /api/n9e/os-fields
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "opensearch",
  "datasource_id": 1,
  "index": "logs-*"
}
```

## 获取字段值

```
POST /api/n9e/os-variable
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "opensearch",
  "datasource_id": 1,
  "index": "logs-*",
  "query": {
    "find": "terms",
    "field": "service",
    "query": ""
  }
}
```

---

## 搜索日志

通过代理执行 `_msearch`，用法与 Elasticsearch 完全相同：

```
POST /api/n9e/proxy/<datasource_id>/_msearch
Authorization: Bearer <token>
Content-Type: application/x-ndjson
```

```
{"search_type":"query_then_fetch","ignore_unavailable":true,"index":"logs-*"}
{"size":50,"query":{"bool":{"filter":[{"range":{"@timestamp":{"gte":"now-1h","lte":"now","format":"strict_date_optional_time"}}},{"query_string":{"query":"level:ERROR"}}]}},"sort":[{"@timestamp":{"order":"desc"}}]}
```

详细用法参见 [elasticsearch.md](elasticsearch.md)。

---

## 注意事项

- **独立端点**：元数据查询使用 `/os-indices`、`/os-fields`、`/os-variable`（不是 `/indices`、`/fields`、`/es-variable`）
- **代理通用**：日志搜索（`_msearch`）通过通用代理 `/proxy/<id>/` 执行，与 Elasticsearch 一致
- **查询语法相同**：DSL 和 Lucene 查询语法与 Elasticsearch 完全兼容
