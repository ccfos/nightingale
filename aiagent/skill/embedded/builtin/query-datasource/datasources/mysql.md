# MySQL Queries

- **plugin_type**: `mysql`
- **Query language**: SQL
- **Use case**: Metric queries

---

## Query Database List

```
POST /api/n9e/db-databases
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "mysql", "datasource_id": 1, "query": []}
```

## Query Table List

```
POST /api/n9e/db-tables
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "mysql", "datasource_id": 1, "query": ["database_name"]}
```

## Query Table Structure

```
POST /api/n9e/db-desc-table
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "mysql", "datasource_id": 1, "query": [{"database": "mydb", "table": "metrics"}]}
```

---

## Run a Time-Series Query

```
POST /api/n9e/ds-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "mysql",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:00') AS ts, COUNT(*) AS value FROM orders WHERE created_at >= FROM_UNIXTIME($from) AND created_at < FROM_UNIXTIME($to) GROUP BY ts ORDER BY ts",
      "keys": {
        "valueKey": "value",
        "labelKey": "",
        "timeKey": "ts"
      }
    }
  ]
}
```

---

## Query Parameters

| Field | Type | Required | Description |
|---|---|---|---|
| `sql` | string | Yes | SQL query statement, supports `$from` and `$to` time variables |
| `keys.valueKey` | string | No | Numeric column name (required for time-series queries); separate multiple with spaces |
| `keys.labelKey` | string | No | Label/grouping column name; separate multiple with spaces |
| `keys.timeKey` | string | No | Time column name |

## Common SQL Examples

| Requirement | SQL |
|---|---|
| Count aggregated per minute | `SELECT DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:00') AS ts, COUNT(*) AS value FROM orders WHERE created_at >= FROM_UNIXTIME($from) AND created_at < FROM_UNIXTIME($to) GROUP BY ts ORDER BY ts` |
| Group statistics by field | `SELECT status, COUNT(*) AS value FROM orders WHERE created_at >= FROM_UNIXTIME($from) AND created_at < FROM_UNIXTIME($to) GROUP BY status` |
| Compute average | `SELECT DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:00') AS ts, AVG(response_time) AS value FROM requests WHERE created_at >= FROM_UNIXTIME($from) AND created_at < FROM_UNIXTIME($to) GROUP BY ts ORDER BY ts` |

## Considerations

- **Read-only**: Write operations such as CREATE, INSERT, UPDATE, DELETE, ALTER, DROP are prohibited
- **Time variables**: `$from` and `$to` are Unix timestamps (seconds) and must be converted with `FROM_UNIXTIME()`
- **Time formatting**: Use `DATE_FORMAT()` for time aggregation
