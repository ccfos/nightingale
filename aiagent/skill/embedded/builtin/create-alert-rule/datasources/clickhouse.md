# ClickHouse alert rules

- `prod`: `"metric"` or `"logging"`
- `cate`: `"ck"`
- `recover_config.judge_type`: `1` (metric type)
- **Required** `keys.valueKey`: the alias of the numeric column in the SELECT statement

## OSS edition limitation

**The OSS edition of n9e's ClickHouse data source does not support time variables such as `$from`/`$to`/`$__timeFilter`**; the variables are not substituted.

**Correct approach**: use ClickHouse's native time functions:
- Last N minutes: `WHERE timestamp >= now() - INTERVAL 5 MINUTE`
- Last N hours: `WHERE timestamp >= now() - INTERVAL 1 HOUR`
- Today: `WHERE toDate(timestamp) = today()`

## triggers hard rules (must read)

- `exp` is **required** and is the only field the alert engine evaluates (a rule without exp will never fire once created, with no error whatsoever)
- Variable syntax for this data source: `$<ref>.<valueKey alias>`, e.g. `$A.value > 100`; with only one valueKey you may omit the alias and write `$A` directly, but with multiple valueKeys you **must include the alias** (a bare `$A` has an undefined value)
- `mode` is fixed at `1` (expression mode; the frontend displays exp as-is); join multiple conditions with `&&` / `||`, e.g. `"$A.value > 10 && $B.value < 5"`

## rule_config structure

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count() AS value FROM logs WHERE timestamp >= now() - INTERVAL 1 MINUTE AND level = 'ERROR'",
        "keys": {
          "valueKey": "value",
          "labelKey": ""
        },
        "interval": 60
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A.value > 100",
        "severity": 2,
        "recover_config": {"judge_type": 1}
      }
    ]
  }
}
```

## query field reference

| Field | Required | Description |
|---|---|---|
| `ref` | ✅ | Query reference name |
| `sql` | ✅ | ClickHouse SQL; the numeric column alias must match `keys.valueKey` |
| `keys.valueKey` | ✅ | **Required**, the alias of the numeric column |
| `keys.labelKey` | ❌ | Label column alias(es), multiple separated by spaces |
| `interval` | ❌ | Query execution interval, **unit: total seconds** (60=1 minute, 300=5 minutes, 3600=1 hour). **Do not write `interval_unit`** |
