# Datasource Proxy API

Nightingale provides a generic reverse-proxy endpoint that forwards requests verbatim to a backend data source (Prometheus / VictoriaMetrics, Elasticsearch, and so on) based on the data source ID. The caller does not need to know the backend address or maintain backend credentials — once a data source is configured in Nightingale, you can reach the backend's native API using your n9e login session.

---

## Endpoint

```
ANY /api/n9e/proxy/:id/*url
```

- Supports every HTTP method (GET / POST / PUT / DELETE, etc.).
- Requires authentication (a JWT session or `X-User-Token`, see below).
- `:id` is the data source ID and `*url` is the path and query string of the backend's native API.

### Authentication

Pick one of the two:

**1. Browser session (JWT)**
Frontend calls carry the login cookie and `Authorization: Bearer <access_token>` automatically; nothing extra is needed once you are logged into n9e.

**2. X-User-Token (recommended for scripts and external callers)**
Generate a long-lived token in your profile and pass it in a request header:

```
X-User-Token: <your-token>
```

Create and revoke tokens under Profile → User Tokens. A token is bound to a specific user and inherits that user's permissions. The n9e administrator must enable `http.token_auth.enable = true` in the configuration. The default header is `X-User-Token`, which can be renamed via `http.token_auth.header_user_token_key`.

Example:

```bash
curl -H "X-User-Token: 2f8b...c71e" \
  "http://n9e/api/n9e/proxy/1/api/v1/query?query=up"
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Data source ID |
| *url | string | Path of the backend API; it is appended verbatim to the data source address |

### Response

The response body, response headers, and HTTP status code are the backend's own and are passed straight through.

### Common errors

| Status | Response | Description |
|--------|------|------|
| 400 | `no such datasource` | The data source does not exist |
| 400 | `invalid url path` | The URL is missing the backend path (e.g. only `/api/n9e/proxy/1` was given) |
| 500 | `invalid urls: ...` | The address configured on the data source is invalid |
| 502 | `unauthorized access` | The backend returned 401 (usually the data source credentials are wrong) |
| 502 | Other | Cannot connect to the backend, TLS handshake failure, and so on |

---

## Proxying Prometheus / VictoriaMetrics

Both Prometheus and VictoriaMetrics are compatible with the Prometheus HTTP API (`/api/v1/*`), so they can be called directly through the proxy. In the examples below, `1` is the ID of the Prometheus data source.

### Instant query

```bash
curl "http://n9e/api/n9e/proxy/1/api/v1/query?query=up&time=1710000000"
```

### Range query

```bash
curl "http://n9e/api/n9e/proxy/1/api/v1/query_range?query=up&start=1710000000&end=1710003600&step=15"
```

### Metadata

```bash
# List of label names
curl "http://n9e/api/n9e/proxy/1/api/v1/labels"

# Label values
curl "http://n9e/api/n9e/proxy/1/api/v1/label/instance/values"

# Series query
curl "http://n9e/api/n9e/proxy/1/api/v1/series?match[]=up&start=1710000000&end=1710003600"
```

### Sample response (`query_range`)

```json
{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": { "__name__": "up", "instance": "10.0.0.1:9100" },
        "values": [[1710000000, "1"], [1710000015, "1"]]
      }
    ]
  }
}
```

### VictoriaMetrics cluster

In cluster mode, VictoriaMetrics usually has the read-path prefix `/select/<accountID>/prometheus` baked into the data source address. When calling the proxy you only supply the remaining part, for example:

```bash
# Data source address: http://vmselect:8481/select/0/prometheus
curl "http://n9e/api/n9e/proxy/2/api/v1/query?query=up"
# Ends up forwarded to http://vmselect:8481/select/0/prometheus/api/v1/query?query=up
```

---

## Proxying Elasticsearch

Every native Elasticsearch REST API can be called through the proxy. In the examples below, `5` is the ID of the Elasticsearch data source.

### Search documents

```bash
curl -X POST "http://n9e/api/n9e/proxy/5/nginx-*/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "size": 10,
    "query": {
      "bool": {
        "must": [
          { "query_string": { "query": "status:500" } },
          { "range": { "@timestamp": { "gte": "now-1h" } } }
        ]
      }
    },
    "sort": [{ "@timestamp": "desc" }]
  }'
```

### Index metadata

```bash
# List indices
curl "http://n9e/api/n9e/proxy/5/_cat/indices?format=json"

# Field mappings
curl "http://n9e/api/n9e/proxy/5/nginx-*/_mapping/field/*"

# Cluster health
curl "http://n9e/api/n9e/proxy/5/_cluster/health"
```

### Aggregation example

```bash
curl -X POST "http://n9e/api/n9e/proxy/5/nginx-*/_search" \
  -H 'Content-Type: application/json' \
  -d '{
    "size": 0,
    "aggs": {
      "by_status": { "terms": { "field": "status", "size": 10 } }
    }
  }'
```

### Sample response (`_search`)

```json
{
  "took": 12,
  "hits": {
    "total": { "value": 128, "relation": "eq" },
    "hits": [
      { "_index": "nginx-2024.03.01", "_id": "abc", "_source": { "...": "..." } }
    ]
  }
}
```
