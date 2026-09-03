# TDengine alert rules

- `prod`: `"metric"`
- `cate`: `"tdengine"`
- `recover_config.judge_type`: `1` (metric type)

## ⚠️ Core constraints

1. **The SELECT result must include a column of TIMESTAMP type**. When n9e parses time-series data, it looks at `column_meta` for the column of type `TIMESTAMP` to use as the time axis. If you use `SELECT *`, the original table's `ts` or `_ts` column is naturally included. If you use aggregation functions, you need to explicitly `SELECT _wstart`.
2. **`value` is a TDengine reserved word and cannot be used as an alias**. Use `AS val`, `AS metric_val`, etc. instead. Accidentally using `AS value` will report a syntax error, but n9e swallows the error and displays it as "timestamp column not found".
3. **`keys.metricKey` and `keys.labelKey` must match the SELECT column names exactly**.

## Time variable support

TDengine is the **only** SQL-type data source in OSS n9e that **supports time variable substitution**:

| Variable | Substituted with | Example value |
|---|---|---|
| `$from` | RFC3339 string (with single quotes) | `'2026-04-09T08:00:00Z'` |
| `$to` | RFC3339 string (with single quotes) | `'2026-04-09T08:05:00Z'` |
| `$interval` | seconds string | `60s` |

Write `_ts >= $from AND _ts < $to` directly in the SQL—**do not add quotes again**.

## triggers hard rules (must read)

- `exp` is **required** and is the only field the alert engine evaluates (a rule without exp will never fire once created, with no error whatsoever)
- Variable syntax for this data source: `$<ref>.<metricKey column name>`, e.g. `$A.current > 10`; with only one metricKey column you may omit the column name and write `$A` directly, but **with multiple metric columns you must include the column name** (a bare `$A` has an undefined value)
- `mode` is fixed at `1` (expression mode; the frontend displays exp as-is); join multiple conditions with `&&` / `||`, e.g. `"$A.current > 10 && $A.voltage < 5"`

## Standard query pattern (recommended)

The simplest, most reliable pattern: `SELECT * FROM db.table WHERE <time column> >= $from AND <time column> < $to`, relying on `keys` to declare which columns are metrics and which are labels.

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "query": "SELECT * FROM power.meters WHERE ts >= $from AND ts < $to",
        "interval": 60,
        "keys": {
          "metricKey": "current voltage",
          "labelKey": "location",
          "timeFormat": ""
        }
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A.current > 10",
        "severity": 2,
        "recover_config": {"judge_type": 1}
      }
    ]
  }
}
```

**Notes**:
- `SELECT *` returns all columns (including the TIMESTAMP column `ts`), satisfying n9e's requirement for a time column
- `metricKey: "current voltage"` tells the engine that `current` and `voltage` are numeric metrics (multiple separated by spaces)
- `labelKey: "location"` tells the engine that `location` is a label dimension
- `$A.current > 10` in exp only judges the `current` column; to monitor `voltage` at the same time, append a condition in exp (e.g. `$A.current > 10 || $A.voltage > 240`)

## Windowed aggregation pattern (advanced)

When you need to aggregate over a time window (e.g. computing an average), use `_wstart` + `PARTITION BY` + `INTERVAL`:

```json
{
  "queries": [
    {
      "ref": "A",
      "query": "SELECT _wstart, avg(current) AS avg_current, location FROM power.meters WHERE ts >= $from AND ts < $to PARTITION BY location INTERVAL($interval)",
      "interval": 60,
      "keys": {
        "metricKey": "avg_current",
        "labelKey": "location",
        "timeFormat": ""
      }
    }
  ],
  "triggers": [
    {
      "mode": 1,
      "exp": "$A.avg_current > 12",
      "severity": 2,
      "recover_config": {"judge_type": 1}
    }
  ]
}
```

**Notes**:
- **You must SELECT `_wstart`**, otherwise the result has no TIMESTAMP column
- The alias **cannot be `value`** (a reserved word); use `avg_current`, `val`, `metric_val`, etc.
- Group with **`PARTITION BY`** (TDengine 3.x), not `GROUP BY`
- `metricKey` must match the alias exactly (e.g. `"avg_current"`)

## query field reference

| Field | Required | Description |
|---|---|---|
| `ref` | ✅ | Query reference name |
| `query` | ✅ | TDengine SQL. The result set **must include a TIMESTAMP column**, and the alias **cannot be `value`** |
| `keys.metricKey` | ✅ | Numeric column name(s), multiple separated by spaces. The trigger does threshold judgment on the values of these columns |
| `keys.labelKey` | ❌ | Label column name(s), multiple separated by spaces |
| `keys.timeFormat` | ❌ | Time format (usually left empty) |
| `interval` | ❌ | Query interval, **unit: total seconds**. Also serves as the value of the `$interval` macro |

## Troubleshooting

| Error | Cause | Fix |
|---|---|---|
| `timestamp column not found` | No TIMESTAMP column in the SELECT result, or a SQL syntax error was silently swallowed | Use the `SELECT *` pattern to ensure a time column is included; check whether the alias used the reserved word `value` |
| Syntax error but it shows timestamp not found | `value` is a reserved word causing SQL parsing to fail, swallowed by QueryTable | Change `AS value` to `AS val` or `AS metric_val` |
| `Query memory exhausted` | The INTERVAL query time range is too large | Narrow the `$from`~`$to` range, or enlarge the INTERVAL window |
