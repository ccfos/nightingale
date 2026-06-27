# MySQL alert rules

- `prod`: `"metric"`
- `cate`: `"mysql"`
- `recover_config.judge_type`: `1` (metric type)
- **Required** `keys.valueKey`: the alias of the numeric column in the SELECT statement (usually called `value`)

## OSS edition limitation

**The OSS edition of n9e's MySQL data source does not support time variables such as `$from`/`$to`/`$__timeFilter`** (the bound `macros.Macro` is a no-op implementation, so the variables enter the SQL as-is and cause a syntax error).

**Correct approach**: use MySQL's native time functions, for example:
- Last N minutes: `WHERE created_at >= NOW() - INTERVAL 5 MINUTE`
- Last N hours: `WHERE created_at >= NOW() - INTERVAL 1 HOUR`
- From the start of today until now: `WHERE DATE(created_at) = CURDATE()`

## triggers hard rules (must read)

- `exp` is **required** and is the only field the alert engine evaluates (a rule without exp will never fire once created, with no error whatsoever)
- Variable syntax for this data source: `$<ref>.<valueKey alias>`, e.g. `$A.value > 10`; with only one valueKey you may omit the alias and write `$A` directly, but with multiple valueKeys you **must include the alias** (a bare `$A` has an undefined value)
- `mode` is fixed at `1` (expression mode; the frontend displays exp as-is); join multiple conditions with `&&` / `||`, e.g. `"$A.value > 10 && $B.value < 5"`

## rule_config structure

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count(*) AS value FROM orders WHERE created_at >= NOW() - INTERVAL 5 MINUTE AND status = 'failed'",
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
        "exp": "$A.value > 10",
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
| `ref` | ✅ | Query reference name, usually `A`, `B`, ... |
| `sql` | ✅ | MySQL SQL query. **Must have one column aliased `value` (or matching `keys.valueKey`) as the alert judgment value**. Use native syntax for time filtering such as `NOW() - INTERVAL X MINUTE`; do not use `$from`/`$to` |
| `keys.valueKey` | ✅ | **Required**, the alias of the numeric column, e.g. `"value"` |
| `keys.labelKey` | ❌ | Label column alias(es), multiple separated by spaces, used to group alerts by dimension (e.g. `"host service"`) |
| `interval` | ❌ | Query execution interval, **unit: total seconds**. For example 60=1 minute, 300=5 minutes, 3600=1 hour. **Do not write `interval_unit`** |

## Multi-dimension example (grouped by host)

```json
{
  "queries": [
    {
      "ref": "A",
      "sql": "SELECT host AS label, count(*) AS value FROM errors WHERE created_at >= NOW() - INTERVAL 5 MINUTE GROUP BY host",
      "keys": {"valueKey": "value", "labelKey": "label"},
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
```
