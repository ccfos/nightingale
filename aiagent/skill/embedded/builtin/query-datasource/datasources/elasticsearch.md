# Elasticsearch 日志查询

- **plugin_type**: `elasticsearch`
- **查询语言**: Elasticsearch DSL / Lucene
- **适用场景**: 日志查询

---

## 获取索引列表

```
POST /api/n9e/indices
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "elasticsearch",
  "datasource_id": 1
}
```

也可通过代理：

```
GET /api/n9e/proxy/<datasource_id>/_cat/indices?format=json&s=index
Authorization: Bearer <token>
```

## 获取索引字段

```
POST /api/n9e/fields
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "elasticsearch",
  "datasource_id": 1,
  "index": "logs-*"
}
```

## 获取字段值（用于过滤）

```
POST /api/n9e/es-variable
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "elasticsearch",
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

## 搜索日志（通过代理执行 _msearch）

```
POST /api/n9e/proxy/<datasource_id>/_msearch
Authorization: Bearer <token>
Content-Type: application/x-ndjson
```

请求体为 NDJSON 格式（每行一个 JSON 对象，header 和 body 交替）：

```
{"search_type":"query_then_fetch","ignore_unavailable":true,"index":"logs-*"}
{"size":50,"query":{"bool":{"filter":[{"range":{"@timestamp":{"gte":"2024-04-01T00:00:00.000Z","lte":"2024-04-02T00:00:00.000Z","format":"strict_date_optional_time"}}},{"query_string":{"query":"level:ERROR AND service:api"}}]}},"sort":[{"@timestamp":{"order":"desc"}}]}
```

**请求体字段说明**：

| 字段 | 说明 |
|---|---|
| `size` | 返回文档数量 |
| `query.bool.filter` | 过滤条件数组 |
| `range` | 时间范围过滤，字段名通常为 `@timestamp` |
| `query_string.query` | Lucene 查询语法搜索 |
| `sort` | 排序，通常按时间倒序 |

**响应格式**：

```json
{
  "responses": [
    {
      "hits": {
        "total": {"value": 1234},
        "hits": [
          {
            "_source": {
              "@timestamp": "2024-04-01T12:00:00.000Z",
              "level": "ERROR",
              "service": "api",
              "message": "Connection timeout"
            }
          }
        ]
      }
    }
  ]
}
```

---

## 聚合查询（统计日志数量趋势）

```
{"search_type":"query_then_fetch","ignore_unavailable":true,"index":"logs-*"}
{"size":0,"query":{"bool":{"filter":[{"range":{"@timestamp":{"gte":"now-1h","lte":"now"}}}]}},"aggs":{"date_histogram":{"date_histogram":{"field":"@timestamp","fixed_interval":"1m"},"aggs":{"count":{"value_count":{"field":"_index"}}}}}}
```

---

## 常用 Lucene 查询语法

| 需求 | 查询 |
|---|---|
| 精确匹配 | `level:ERROR` |
| AND 组合 | `level:ERROR AND service:api` |
| OR 组合 | `level:ERROR OR level:WARN` |
| 通配符 | `message:timeout*` |
| 范围 | `status:[400 TO 599]` |
| 排除 | `NOT level:DEBUG` |
| 短语匹配 | `message:"connection refused"` |

---

## 注意事项

- **NDJSON 格式**：`_msearch` 请求体为 NDJSON 格式，每行一个 JSON，header 和 body 交替出现，末尾必须有换行
- **时间字段**：通常为 `@timestamp`，格式为 ISO 8601
- **索引模式**：支持通配符，如 `logs-*`、`logs-2024.04.*`
