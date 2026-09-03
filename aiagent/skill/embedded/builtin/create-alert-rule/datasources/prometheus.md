# Prometheus alert rules

- `prod`: `"metric"`
- `cate`: `"prometheus"`

## Format note: use v1 only

In the open-source (OSS) edition of n9e, the frontend v2 rule editor is gated by the `IS_PLUS` switch. In an OSS environment `IS_PLUS=false`, and v2 rules are **loaded in v1 format**; the `query` field cannot be read, leaving the PromQL input box empty.

**Conclusion**: OSS n9e should use only **v1 format**, writing the threshold **directly into `prom_ql`**. The v2 format (query and trigger condition separated) is only available in the enterprise edition FE.

## v1 format (the only usable format in OSS)

```json
{
  "rule_config": {
    "queries": [
      {
        "prom_ql": "cpu_usage_active > 80",
        "severity": 2
      }
    ]
  }
}
```

The threshold is written directly into the PromQL string as a comparison operator; do not put it in triggers.

## Complete example (CPU usage alert)

```json
[{
  "name": "Host CPU usage too high",
  "note": "Host CPU usage exceeds 80%",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [1],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": [1]}],
  "disabled": 0,
  "prom_eval_interval": 30,
  "prom_for_duration": 60,
  "rule_config": {
    "queries": [{"prom_ql": "cpu_usage_active > 80", "severity": 2}]
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

## Multiple queries + expression

The v1 format also supports multiple queries; each is an independent "PromQL + threshold" judgment, in an OR relationship:

```json
{
  "queries": [
    {"prom_ql": "cpu_usage_active > 80", "severity": 2},
    {"prom_ql": "mem_used_percent > 90", "severity": 1}
  ]
}
```

## PromQL writing memo

| Need | PromQL |
|---|---|
| Average CPU usage > 80% | `avg(100 - cpu_usage_idle{cpu="cpu-total"}) > 80` |
| Any host memory > 90% | `mem_used_percent > 90` |
| A host's memory usage > 1GiB | `mem_used{ident="web-01"} > 1073741824` |
| rate type (QPS > 1000) | `rate(http_requests_total[5m]) > 1000` |

Note: for PromQL that contains arithmetic operations or aggregations, when comparing the **whole** against a threshold, simply append the threshold after it; the n9e engine parses it correctly.
