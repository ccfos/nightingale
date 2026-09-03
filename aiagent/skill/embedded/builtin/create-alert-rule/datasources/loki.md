# Loki alert rules

- `prod`: `"logging"`
- `cate`: `"loki"`
- `recover_config.judge_type`: `0` (log type)

## triggers hard rules (must read)

- `exp` is **required** and is the only field the alert engine evaluates (a rule without exp will never fire once created, with no error whatsoever)
- Variable syntax for this data source: use `$<ref>` for a single-value query, e.g. `$A > 10` (when the threshold is already written into the LogQL, exp acts as a second confirmation, but it is still required)
- `mode` is fixed at `1` (expression mode; the frontend displays exp as-is); join multiple conditions with `&&` / `||`, e.g. `"$A > 10 && $B < 5"`

## rule_config structure

Loki uses the LogQL query language, but the field name reuses `prom_ql`.

```json
{
  "rule_config": {
    "queries": [
      {
        "prom_ql": "count_over_time({job=\"myapp\"} |= \"error\" [5m]) > 10",
        "severity": 2
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A > 10",
        "severity": 2,
        "recover_config": {"judge_type": 0}
      }
    ]
  }
}
```

## Complete example

```json
[{
  "name": "Too many application error logs",
  "note": "More than 10 error logs within 5 minutes",
  "prod": "logging",
  "cate": "loki",
  "datasource_ids": [2],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": [2]}],
  "disabled": 0,
  "prom_eval_interval": 30,
  "prom_for_duration": 60,
  "rule_config": {
    "queries": [
      {"prom_ql": "count_over_time({job=\"myapp\"} |= \"error\" [5m]) > 10", "severity": 2}
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A > 10",
        "severity": 2,
        "recover_config": {"judge_type": 0}
      }
    ]
  },
  "enable_in_bg": 0,
  "enable_days_of_weeks": [["0","1","2","3","4","5","6"]],
  "enable_stimes": ["00:00"],
  "enable_etimes": ["00:00"],
  "notify_recovered": 1,
  "notify_repeat_step": 60,
  "notify_max_number": 0,
  "callbacks": [],
  "append_tags": [],
  "annotations": {},
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": []
}]
```
