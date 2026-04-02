# Loki 告警规则

- `prod`: `"logging"`
- `cate`: `"loki"`
- `recover_config.judge_type`: `0`（日志类型）

## rule_config 结构

Loki 使用 LogQL 查询语言，但字段名复用 `prom_ql`。

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
        "mode": 0,
        "expressions": [
          {"ref": "A", "comparisonOperator": ">", "value": 10, "logicalOperator": "&&"}
        ],
        "severity": 2,
        "recover_config": {"judge_type": 0}
      }
    ]
  }
}
```

## 完整示例

```json
[{
  "name": "应用错误日志过多",
  "note": "5分钟内错误日志超过10条",
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
        "mode": 0,
        "expressions": [{"ref": "A", "comparisonOperator": ">", "value": 10, "logicalOperator": "&&"}],
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
