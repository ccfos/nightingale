# Loki Log Queries

- **plugin_type**: `loki`
- **Query language**: LogQL
- **Use case**: Log queries

Call the native Loki API through the proxy.

---

## Query Logs (Range Query)

```
GET /api/n9e/proxy/<datasource_id>/loki/api/v1/query_range?query=<logql>&start=<unix_ns>&end=<unix_ns>&limit=100
Authorization: Bearer <token>
```

| Parameter | Type | Required | Description |
|---|---|---|---|
| `query` | string | Yes | LogQL expression |
| `start` | int64 | Yes | Start time, nanosecond Unix timestamp |
| `end` | int64 | Yes | End time, nanosecond Unix timestamp |
| `limit` | int | No | Number of log lines to return, default 100 |

**Timestamp conversion**: nanoseconds = seconds × 1000000000

## Instant Query

```
GET /api/n9e/proxy/<datasource_id>/loki/api/v1/query?query=<logql>&time=<unix_ns>
Authorization: Bearer <token>
```

---

## Get Label List

```
GET /api/n9e/proxy/<datasource_id>/loki/api/v1/labels
Authorization: Bearer <token>
```

## Get Label Values

```
GET /api/n9e/proxy/<datasource_id>/loki/api/v1/label/<label_name>/values
Authorization: Bearer <token>
```

---

## Common LogQL Examples

| Requirement | LogQL |
|---|---|
| View logs of an application | `{job="myapp"}` |
| Search logs containing error | `{job="myapp"} \|= "error"` |
| Exclude a keyword | `{job="myapp"} != "debug"` |
| Regex match | `{job="myapp"} \|~ "timeout\|refused"` |
| Count error logs | `count_over_time({job="myapp"} \|= "error" [5m])` |
| Filter by label | `{job="myapp", level="error"}` |
| Parse JSON logs | `{job="myapp"} \| json \| status >= 500` |
| Log rate | `rate({job="myapp"} \|= "error" [1m])` |

---

## Considerations

- **Timestamps are in nanoseconds**: Loki API time parameters use nanosecond Unix timestamps (seconds × 1000000000)
- **Stream selector is required**: A LogQL query must include at least one stream selector `{label="value"}`
- **Pipeline operators**: `|=` (contains), `!=` (does not contain), `|~` (regex match), `!~` (regex non-match)
