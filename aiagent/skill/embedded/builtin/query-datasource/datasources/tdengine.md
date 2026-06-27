# TDengine Queries

- **plugin_type**: `tdengine`
- **Query language**: SQL (TDengine dialect)
- **Use case**: Time-series queries

TDengine has dedicated metadata query endpoints, different from the generic SQL datasources.

---

## Query Database List

```
POST /api/n9e/tdengine-databases
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "tdengine",
  "datasource_id": 1
}
```

## Query Table List

```
POST /api/n9e/tdengine-tables
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "tdengine",
  "datasource_id": 1,
  "db": "power",
  "is_stable": false
}
```

| Field | Description |
|---|---|
| `db` | Database name |
| `is_stable` | `false`=regular table, `true`=super table (stable) |

## Query Table Columns

```
POST /api/n9e/tdengine-columns
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "cate": "tdengine",
  "datasource_id": 1,
  "db": "power",
  "table": "meters"
}
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
  "cate": "tdengine",
  "datasource_id": 1,
  "query": [
    {
      "query": "SELECT _wstart AS ts, AVG(current) AS value FROM power.meters WHERE ts >= $from AND ts < $to INTERVAL($interval)",
      "from": "2024-04-01T00:00:00Z",
      "to": "2024-04-02T00:00:00Z",
      "interval": 60,
      "interval_unit": "s",
      "keys": {
        "metricKey": "value",
        "labelKey": "location",
        "timeFormat": ""
      }
    }
  ]
}
```

---

## Query Parameters

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | string | Yes | TDengine SQL query, supports `$from`, `$to`, `$interval` variables |
| `from` | string | Yes | Start time, ISO 8601 format |
| `to` | string | Yes | End time, ISO 8601 format |
| `interval` | int | No | Sampling interval value |
| `interval_unit` | string | No | Interval unit: `s` (seconds), `m` (minutes), `h` (hours) |
| `keys.metricKey` | string | No | Numeric column name |
| `keys.labelKey` | string | No | Label/grouping column name |
| `keys.timeFormat` | string | No | Time format |

## Time Variables

| Variable | Description |
|---|---|
| `$from` | Start time |
| `$to` | End time |
| `$interval` | Sampling interval, e.g. `60s` |

## Common SQL Examples

| Requirement | SQL |
|---|---|
| Aggregate average by interval | `SELECT _wstart AS ts, AVG(current) AS value FROM power.meters WHERE ts >= $from AND ts < $to INTERVAL($interval)` |
| Group by label | `SELECT _wstart AS ts, location, AVG(current) AS value FROM power.meters WHERE ts >= $from AND ts < $to PARTITION BY location INTERVAL($interval)` |
| Latest value | `SELECT LAST(current) AS value, location FROM power.meters GROUP BY location` |

## Considerations

- **Dedicated API**: Metadata queries (databases/tables/columns) use the dedicated `/tdengine-*` endpoints, not the generic `/db-*` endpoints
- **Super table**: Set `is_stable: true` when querying a stable (super table)
- **INTERVAL**: TDengine's `INTERVAL()` clause is used for time-window aggregation; combine it with `_wstart` to get the window start time
