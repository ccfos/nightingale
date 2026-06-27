# Elasticsearch Log Queries

- **plugin_type**: `elasticsearch`
- **Query language**: Elasticsearch DSL / Lucene
- **Use case**: Log queries

---

## Get Index List

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

Also available through the proxy:

```
GET /api/n9e/proxy/<datasource_id>/_cat/indices?format=json&s=index
Authorization: Bearer <token>
```

## Get Index Fields

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

## Get Field Values (for filtering)

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

## Search Logs (execute _msearch through the proxy)

```
POST /api/n9e/proxy/<datasource_id>/_msearch
Authorization: Bearer <token>
Content-Type: application/x-ndjson
```

The request body is in NDJSON format (one JSON object per line, with header and body alternating):

```
{"search_type":"query_then_fetch","ignore_unavailable":true,"index":"logs-*"}
{"size":50,"query":{"bool":{"filter":[{"range":{"@timestamp":{"gte":"2024-04-01T00:00:00.000Z","lte":"2024-04-02T00:00:00.000Z","format":"strict_date_optional_time"}}},{"query_string":{"query":"level:ERROR AND service:api"}}]}},"sort":[{"@timestamp":{"order":"desc"}}]}
```

**Request body field descriptions**:

| Field | Description |
|---|---|
| `size` | Number of documents to return |
| `query.bool.filter` | Array of filter conditions |
| `range` | Time-range filter; the field name is usually `@timestamp` |
| `query_string.query` | Search using Lucene query syntax |
| `sort` | Sorting, usually in descending time order |

**Response format**:

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

## Aggregation Query (log count trend statistics)

```
{"search_type":"query_then_fetch","ignore_unavailable":true,"index":"logs-*"}
{"size":0,"query":{"bool":{"filter":[{"range":{"@timestamp":{"gte":"now-1h","lte":"now"}}}]}},"aggs":{"date_histogram":{"date_histogram":{"field":"@timestamp","fixed_interval":"1m"},"aggs":{"count":{"value_count":{"field":"_index"}}}}}}
```

---

## Common Lucene Query Syntax

| Requirement | Query |
|---|---|
| Exact match | `level:ERROR` |
| AND combination | `level:ERROR AND service:api` |
| OR combination | `level:ERROR OR level:WARN` |
| Wildcard | `message:timeout*` |
| Range | `status:[400 TO 599]` |
| Exclude | `NOT level:DEBUG` |
| Phrase match | `message:"connection refused"` |

---

## Considerations

- **NDJSON format**: The `_msearch` request body is in NDJSON format, one JSON per line, with header and body alternating, and a trailing newline is required
- **Time field**: Usually `@timestamp`, in ISO 8601 format
- **Index pattern**: Wildcards are supported, e.g. `logs-*`, `logs-2024.04.*`
