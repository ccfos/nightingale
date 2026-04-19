# Elasticsearch / OpenSearch 告警规则

## Elasticsearch

- `prod`: `"logging"`
- `cate`: `"elasticsearch"`
- `recover_config.judge_type`: `0`（日志类型）

## OpenSearch

- `prod`: `"logging"`
- `cate`: `"opensearch"`
- 结构与 Elasticsearch **完全相同**，只是 `cate` 不同，且不支持 `index_pattern`

## rule_config 结构

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
        "mode": 0,
        "expressions": [
          {"ref": "A", "comparisonOperator": ">", "value": 100, "logicalOperator": "&&"}
        ],
        "severity": 2,
        "recover_config": {"judge_type": 0}
      }
    ]
  }
}
```

## query 字段说明

| 字段 | 说明 |
|---|---|
| `index_type` | `"index"` 或 `"index_pattern"`（OpenSearch 不支持 index_pattern） |
| `index` | 索引名，支持通配符如 `logs-*` |
| `filter` | ES 查询过滤条件 |
| `date_field` | 时间字段名，通常为 `@timestamp` |
| `interval` | 查询聚合时间窗口，**单位：总秒数**（60=1分钟，300=5分钟，3600=1小时）。**不要写 `interval_unit`** |
| `value.func` | 聚合函数：`count` / `avg` / `sum` / `max` / `min` / `p90` / `p95` / `p99` |
| `value.field` | 聚合字段名（`count` 时不需要） |
| `group_by` | 分组配置，`cate` 可选 `terms` / `filters` / `histogram` |

## 完整示例（Elasticsearch）

```json
[{
  "name": "ES错误日志过多",
  "note": "5分钟内错误日志超过100条",
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
        "mode": 0,
        "expressions": [{"ref": "A", "comparisonOperator": ">", "value": 100, "logicalOperator": "&&"}],
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
