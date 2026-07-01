# VictoriaLogs Queries

- **plugin_type**: `victorialogs`
- **Query language**: LogsQL
- **Use case**: Log queries

Call the native VictoriaLogs API through the proxy.

---

## Search Logs

```
GET /api/n9e/proxy/<datasource_id>/select/logsql/query?query=<logsql>&start=<unix_ts>&end=<unix_ts>&limit=100
Authorization: Bearer <token>
```

| Parameter | Type | Required | Description |
|---|---|---|---|
| `query` | string | Yes | LogsQL query expression |
| `start` | int64 | No | Start time, Unix timestamp (seconds) |
| `end` | int64 | No | End time, Unix timestamp (seconds) |
| `limit` | int | No | Number of log lines to return |

---

## Common LogsQL Examples

| Requirement | LogsQL |
|---|---|
| Search logs containing error | `_msg:error` |
| Multiple conditions with AND | `_msg:error AND service:payment` |
| Filter by time stream | `_stream:{job="myapp"} AND _msg:error` |
| Exclude a keyword | `_msg:error NOT _msg:debug` |
| Regex match | `_msg:~"timeout\|refused"` |

---

## Considerations

- **Timestamps are in seconds**: Unlike Loki, VictoriaLogs time parameters use second-level Unix timestamps
- **LogsQL, not LogQL**: VictoriaLogs uses the LogsQL query language, whose syntax differs from Loki's LogQL
- **_msg field**: The default log content field is `_msg`
- **_stream field**: Used for filtering by log stream
