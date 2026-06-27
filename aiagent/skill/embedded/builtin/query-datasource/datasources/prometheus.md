# Prometheus / VictoriaMetrics Metric Queries

- **plugin_type**: `prometheus`
- **Query language**: PromQL
- **Use case**: Metric/time-series queries

---

## Discover Available Metrics

Retrieve the list of all metric names:

```
GET /api/n9e/proxy/<datasource_id>/api/v1/label/__name__/values
Authorization: Bearer <token>
```

## Get All Label Names

```
GET /api/n9e/proxy/<datasource_id>/api/v1/labels
Authorization: Bearer <token>
```

## Get Label Values

```
GET /api/n9e/proxy/<datasource_id>/api/v1/label/<label_name>/values
Authorization: Bearer <token>
```

## Query Time-Series Metadata

```
GET /api/n9e/proxy/<datasource_id>/api/v1/series?match[]=<metric_selector>&start=<unix_ts>&end=<unix_ts>
Authorization: Bearer <token>
```

---

## Instant Query

Query the metric value at a specific point in time:

```
POST /api/n9e/query-instant-batch
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "datasource_id": 1,
  "queries": [
    {
      "time": 1712003600,
      "query": "cpu_usage_active{ident='web-01'}"
    }
  ]
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `datasource_id` | int64 | Yes | Datasource ID |
| `queries[].time` | int64 | Yes | Query time, Unix timestamp (seconds) |
| `queries[].query` | string | Yes | PromQL expression |

**Response format**:

```json
{
  "dat": [
    {
      "resultType": "vector",
      "result": [
        {
          "metric": {"__name__": "cpu_usage_active", "ident": "web-01"},
          "value": [1712003600, "85.6"]
        }
      ]
    }
  ]
}
```

---

## Range Query

Query the metric trend over a period of time:

```
POST /api/n9e/query-range-batch
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "datasource_id": 1,
  "queries": [
    {
      "start": 1712000000,
      "end": 1712003600,
      "step": 60,
      "query": "cpu_usage_active{ident='web-01'}"
    }
  ]
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `datasource_id` | int64 | Yes | Datasource ID |
| `queries[].start` | int64 | Yes | Start time, Unix timestamp (seconds) |
| `queries[].end` | int64 | Yes | End time, Unix timestamp (seconds) |
| `queries[].step` | int64 | Yes | Sampling step (seconds) |
| `queries[].query` | string | Yes | PromQL expression |

**Recommended step values**:

| Time Range | step |
|---|---|
| 1 hour | 15s |
| 6 hours | 60s |
| 24 hours | 300s |
| 7 days | 1800s |

**Response format**:

```json
{
  "dat": [
    {
      "resultType": "matrix",
      "result": [
        {
          "metric": {"__name__": "cpu_usage_active", "ident": "web-01"},
          "values": [
            [1712000000, "82.3"],
            [1712000060, "83.1"],
            [1712000120, "85.6"]
          ]
        }
      ]
    }
  ]
}
```

---

## Query Directly Through the Proxy

You can also call the native Prometheus API directly through the proxy:

```
GET /api/n9e/proxy/<datasource_id>/api/v1/query?query=up&time=<unix_ts>
GET /api/n9e/proxy/<datasource_id>/api/v1/query_range?query=up&start=<ts>&end=<ts>&step=60
Authorization: Bearer <token>
```

---

## Common PromQL Examples

| Requirement | PromQL |
|---|---|
| CPU usage | `cpu_usage_active` |
| Memory usage | `mem_used_percent` |
| Disk usage | `disk_used_percent{path="/"}` |
| Inbound network throughput rate | `rate(net_bytes_recv[5m])` |
| 1-minute load | `system_load1` |
| HTTP request rate | `rate(http_requests_total[5m])` |
| All metrics of a given instance | `{ident="web-01"}` |
| Aggregation: average per instance | `avg by (ident)(cpu_usage_active)` |
| Top 10 CPU | `topk(10, cpu_usage_active)` |
