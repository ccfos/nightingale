# VictoriaLogs alert rules

- `prod`: `"logging"`
- `cate`: `"victorialogs"`
- `recover_config.judge_type`: `0` (log type)

## ⚠️ Core constraint: you must use pipe syntax

VictoriaLogs alerts go through the **`/select/logsql/stats_query`** endpoint (see `dskit/victorialogs/victorialogs.go:StatsQuery`), which **only accepts aggregation queries with a `| stats` pipe**, not pure filter queries.

Wrong approach (only errors out or returns empty):
```
_msg:error
service:payment AND level:error
```

Correct approach:
```
<filter condition> | stats <aggregation function>
```

| Need | LogsQL |
|---|---|
| Number of error logs in the last 5 minutes | `_msg:error \| stats count() as value` |
| Number of error logs for a service | `service:payment AND level:error \| stats count() as value` |
| Count grouped by service | `level:error \| stats by (service) count() as value` |
| Average response time | `* \| stats avg(duration) as value` |
| Multiple aggregations | `_msg:error \| stats count() as error_count, avg(duration) as avg_dur` |

**Key rules**:
- After `| stats`, you must alias each aggregation function (`as value` / `as count`, etc.)
- The alias becomes the field name of the returned result, and the alert engine matches by alias
- Group with `stats by (field1, field2) ...`
- The filter part can be any LogsQL filter: `_msg:keyword`, `field:value`, `field:"value with space"`, `_time:5m`, etc.

## triggers hard rules (must read)

- `exp` is **required** and is the only field the alert engine evaluates (a rule without exp will never fire once created, with no error whatsoever)
- Variable syntax for this data source: `$<ref>.<stats output alias>`, e.g. `$A.value > 20` (corresponding to `stats count() as value`); with only one stats output you may omit the alias and write `$A` directly
- `mode` is fixed at `1` (expression mode; the frontend displays exp as-is); join multiple conditions with `&&` / `||`, e.g. `"$A.value > 10 && $B.value < 5"`

## rule_config structure

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "query": "_msg:error | stats count() as value",
        "interval": 60
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A.value > 20",
        "severity": 2,
        "recover_config": {"judge_type": 0}
      }
    ]
  }
}
```

## query field reference

| Field | Required | Description |
|---|---|---|
| `ref` | ✅ | Query reference name |
| `query` | ✅ | LogsQL query, **must be in the `<filter> \| stats <function>` format** |
| `interval` | ❌ | Query interval, **unit: total seconds** (60=1 minute, 300=5 minutes). **Do not write `interval_unit`** |

## Complete example

```json
[{
  "name": "Application error log alert",
  "note": "More than 20 error logs for the payment service in the last 1 minute",
  "prod": "logging",
  "cate": "victorialogs",
  "datasource_ids": [11],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": [11]}],
  "disabled": 0,
  "prom_eval_interval": 30,
  "prom_for_duration": 60,
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "query": "service:payment AND _msg:error | stats count() as value",
        "interval": 60
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A.value > 20",
        "severity": 2,
        "recover_config": {"judge_type": 0}
      }
    ]
  },
  "notify_version": 1
}]
```

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Rule saves successfully but always has no data | The query only has a filter condition, no `\| stats` | Add `\| stats count() as value` or another aggregation |
| `no stats clause` error | Same as above | Same as above |
| Aggregation value cannot be obtained | The aggregation has no alias | Add `as value` to the aggregation, and make `value` match the threshold |
