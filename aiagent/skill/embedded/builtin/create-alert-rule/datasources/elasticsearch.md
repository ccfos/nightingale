# Elasticsearch / OpenSearch alert rules

## Elasticsearch

- `prod`: `"logging"`
- `cate`: `"elasticsearch"`
- `recover_config.judge_type`: `0` (log type)

## OpenSearch

- `prod`: `"logging"`
- `cate`: `"opensearch"`
- The structure is **exactly the same** as Elasticsearch, only `cate` differs, and `index_pattern` is not supported

## triggers hard rules (must read)

- `exp` is **required** and is the only field the alert engine evaluates (a rule without exp will never fire once created, with no error whatsoever)
- Variable syntax for this data source: use `$<ref>` for single-value queries such as count, e.g. `$A > 100`
- `mode` is fixed at `1` (expression mode; the frontend displays exp as-is); join multiple conditions with `&&` / `||`, e.g. `"$A > 10 && $B < 5"`

## rule_config structure

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "index_type": "index",
        "index": "logs-*",
        "filter": "level:ERROR",
        "date_field": "@timestamp",
        "interval": 300,
        "value": {
          "func": "count"
        },
        "group_by": [
          {"cate": "terms", "field": "service", "size": 10}
        ],
        "keys": {
          "labelKey": [],
          "valueKey": []
        }
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A > 100",
        "severity": 2,
        "recover_config": {"judge_type": 0}
      }
    ]
  }
}
```

## query field reference

| Field | Description |
|---|---|
| `index_type` | `"index"` or `"index_pattern"` (OpenSearch does not support index_pattern) |
| `index` | Index name, supports wildcards such as `logs-*` |
| `filter` | ES query filter condition |
| `date_field` | Time field name, usually `@timestamp` |
| `interval` | Query aggregation time window, **unit: total seconds** (60=1 minute, 300=5 minutes, 3600=1 hour). **Do not write `interval_unit`** |
| `value.func` | Aggregation function: `count` / `avg` / `sum` / `max` / `min` / `p90` / `p95` / `p99` |
| `value.field` | Aggregation field name (not needed for `count`) |
| `group_by` | Grouping configuration; `cate` can be `terms` / `filters` / `histogram` |

## Complete example (Elasticsearch)

```json
[{
  "name": "Too many ES error logs",
  "note": "More than 100 error logs within 5 minutes",
  "prod": "logging",
  "cate": "elasticsearch",
  "datasource_ids": [2],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": [2]}],
  "disabled": 0,
  "prom_eval_interval": 60,
  "prom_for_duration": 0,
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "index_type": "index",
        "index": "logs-*",
        "filter": "level:ERROR",
        "date_field": "@timestamp",
        "interval": 300,
        "value": {"func": "count"}
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A > 100",
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
