# Doris Queries

- **plugin_type**: `doris`
- **Query language**: SQL (MySQL-compatible dialect)
- **Use case**: Log queries

---

## Query Database List

```
POST /api/n9e/db-databases
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "doris", "datasource_id": 1, "query": []}
```

## Query Table List

```
POST /api/n9e/db-tables
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "doris", "datasource_id": 1, "query": ["database_name"]}
```

## Query Table Structure

```
POST /api/n9e/db-desc-table
Authorization: Bearer <token>
Content-Type: application/json
Body: {"cate": "doris", "datasource_id": 1, "query": [{"database": "logs_db", "table": "access_log"}]}
```

---

## Run a Log Query

```
POST /api/n9e/logs-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "doris",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT * FROM logs_db.access_log WHERE log_time >= FROM_UNIXTIME($from) AND log_time < FROM_UNIXTIME($to) AND message LIKE '%error%' ORDER BY log_time DESC LIMIT 100",
      "from": 1712000000,
      "to": 1712003600,
      "database": "logs_db"
    }
  ]
}
```

## Run a Time-Series Query

```
POST /api/n9e/ds-query
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "doris",
  "datasource_id": 1,
  "query": [
    {
      "sql": "SELECT DATE_TRUNC(log_time, INTERVAL 1 MINUTE) AS ts, COUNT(*) AS value FROM logs_db.access_log WHERE log_time >= FROM_UNIXTIME($from) AND log_time < FROM_UNIXTIME($to) GROUP BY ts ORDER BY ts",
      "from": 1712000000,
      "to": 1712003600,
      "database": "logs_db",
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
| `from` | int64 | Yes | Start time, Unix timestamp (seconds) |
| `to` | int64 | Yes | End time, Unix timestamp (seconds) |
| `database` | string | Yes | Database name (required for Doris) |
| `keys.valueKey` | string | No | Numeric column name (required for time-series queries) |
| `keys.labelKey` | string | No | Label/grouping column name |
| `keys.timeKey` | string | No | Time column name |

## Considerations

- **Read-only**: Write operations are prohibited
- **database is required**: A Doris query must specify the `database` field
- **Time functions**: Supports `DATE_TRUNC()`, `NOW()`, `FROM_UNIXTIME()`, etc.
- **MySQL compatible**: Doris SQL syntax is largely compatible with MySQL
