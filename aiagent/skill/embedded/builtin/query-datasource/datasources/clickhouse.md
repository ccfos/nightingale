# ClickHouse Queries

- **plugin_type**: `ck`
- **Query language**: SQL
- **Use case**: Metric/log queries

---

## Query Database List

```
POST /api/n9e/db-databases
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": []
}
```

## Query Table List

```
POST /api/n9e/db-tables
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": ["database_name"]
}
```

## Query Table Structure

```
POST /api/n9e/db-desc-table
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": [{"database": "default", "table": "logs"}]
}
```

---

## Run a Time-Series Data Query

```
POST /api/n9e/ds-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": [
    {
      "ref": "A",
      "sql": "SELECT toStartOfMinute(timestamp) AS ts, avg(value) AS value FROM metrics WHERE timestamp >= $from AND timestamp < $to GROUP BY ts ORDER BY ts",
      "from": 1712000000,
      "to": 1712003600,
      "keys": {
        "valueKey": "value",
        "labelKey": "",
        "timeKey": "ts"
      }
    }
  ]
}
```

## Run a Log Query

```
POST /api/n9e/logs-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "ck",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT * FROM logs WHERE timestamp >= $from AND timestamp < $to AND message LIKE '%error%' ORDER BY timestamp DESC LIMIT 100",
      "from": 1712000000,
      "to": 1712003600
    }
  ]
}
```

---

## Query Parameters

| Field | Type | Required | Description |
|---|---|---|---|
| `ref` | string | No | Query reference identifier, e.g. `"A"` |
| `sql` | string | Yes | SQL query statement, supports `$from` and `$to` time variables |
| `from` | int64 | Yes | Start time, Unix timestamp (seconds) |
| `to` | int64 | Yes | End time, Unix timestamp (seconds) |
| `keys.valueKey` | string | No | Numeric column name (required for time-series queries); separate multiple with spaces |
| `keys.labelKey` | string | No | Label/grouping column name; separate multiple with spaces |
| `keys.timeKey` | string | No | Time column name |
| `database` | string | No | Database name |
| `table` | string | No | Table name |
| `limit` | int | No | Row count limit for returned results |

## Time-Series Query Response Format

```json
{
  "dat": [
    {
      "ref": "A",
      "metric": {"host": "web-01"},
      "values": [[1712000000, 85.6], [1712000060, 83.2]],
      "query": "SELECT ..."
    }
  ]
}
```

---

## Common SQL Examples

| Requirement | SQL |
|---|---|
| Count aggregated per minute | `SELECT toStartOfMinute(ts) AS t, count() AS value FROM logs WHERE ts >= $from AND ts < $to GROUP BY t ORDER BY t` |
| Group statistics by field | `SELECT service, count() AS value FROM logs WHERE ts >= $from AND ts < $to GROUP BY service ORDER BY value DESC LIMIT 10` |
| Search logs | `SELECT * FROM logs WHERE ts >= $from AND ts < $to AND message LIKE '%error%' ORDER BY ts DESC LIMIT 100` |

## Considerations

- **Read-only**: Write operations such as CREATE, INSERT, UPDATE, DELETE, ALTER, DROP are prohibited
- **Time variables**: Use `$from` and `$to` in SQL; the system automatically replaces them with Unix timestamps (seconds)
- **keys.valueKey**: Required for time-series queries; separate multiple columns with spaces
- **Time functions**: Use ClickHouse functions such as `toStartOfMinute()` and `toStartOfHour()` for time aggregation
