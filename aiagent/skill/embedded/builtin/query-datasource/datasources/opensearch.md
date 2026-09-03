# OpenSearch Queries

- **plugin_type**: `opensearch`
- **Query language**: Elasticsearch DSL / Lucene (compatible with Elasticsearch)
- **Use case**: Log queries

OpenSearch is queried exactly the same way as Elasticsearch, using dedicated API endpoints.

---

## Get Index List

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

## Get Index Fields

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

## Get Field Values

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

## Search Logs

Execute `_msearch` through the proxy; usage is exactly the same as Elasticsearch:

```
POST /api/n9e/proxy/<datasource_id>/_msearch
Authorization: Bearer <token>
Content-Type: application/x-ndjson
```

```
{"search_type":"query_then_fetch","ignore_unavailable":true,"index":"logs-*"}
{"size":50,"query":{"bool":{"filter":[{"range":{"@timestamp":{"gte":"now-1h","lte":"now","format":"strict_date_optional_time"}}},{"query_string":{"query":"level:ERROR"}}]}},"sort":[{"@timestamp":{"order":"desc"}}]}
```

For detailed usage, see [elasticsearch.md](elasticsearch.md).

---

## Considerations

- **Dedicated endpoints**: Metadata queries use `/os-indices`, `/os-fields`, `/os-variable` (not `/indices`, `/fields`, `/es-variable`)
- **Generic proxy**: Log search (`_msearch`) is executed through the generic proxy `/proxy/<id>/`, consistent with Elasticsearch
- **Same query syntax**: DSL and Lucene query syntax is fully compatible with Elasticsearch
