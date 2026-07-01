# PostgreSQL Queries

- **plugin_type**: `pgsql`
- **Query language**: SQL
- **Use case**: Metric queries

---

## Query Database List

```
POST /api/n9e/db-databases
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "pgsql", "datasource_id": 1, "query": []}
```

## Query Table List

```
POST /api/n9e/db-tables
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "pgsql", "datasource_id": 1, "query": ["database_name"]}
```

## Query Table Structure

```
POST /api/n9e/db-desc-table
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "pgsql", "datasource_id": 1, "query": [{"database": "mydb", "table": "metrics"}]}
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
  "cate": "pgsql",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT date_trunc('minute', created_at) AS ts, COUNT(*) AS value FROM events WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) GROUP BY ts ORDER BY ts",
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
| Count aggregated per minute | `SELECT date_trunc('minute', created_at) AS ts, COUNT(*) AS value FROM events WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) GROUP BY ts ORDER BY ts` |
| Group statistics by field | `SELECT status, COUNT(*) AS value FROM events WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) GROUP BY status` |
| Compute percentile | `SELECT date_trunc('minute', created_at) AS ts, percentile_cont(0.95) WITHIN GROUP (ORDER BY response_time) AS value FROM requests WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) GROUP BY ts ORDER BY ts` |

## Considerations

- **Read-only**: Write operations such as CREATE, INSERT, UPDATE, DELETE, ALTER, DROP are prohibited
- **Time variables**: `$from` and `$to` are Unix timestamps (seconds) and must be converted with `to_timestamp()`
- **Time aggregation**: Use `date_trunc('minute', col)` for time bucketing
