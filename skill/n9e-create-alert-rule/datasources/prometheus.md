# Prometheus 告警规则

- `prod`: `"metric"`
- `cate`: `"prometheus"`
- `recover_config.judge_type`: `1`

## v1 格式（简单，阈值写在 PromQL 中）

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

注意：v1 格式下阈值必须写在 `prom_ql` 中，不要放在 triggers 里。

## v2 格式（推荐，查询和触发条件分离）

```json
{
  "rule_config": {
    "version": "v2",
    "queries": [
      {
        "ref": "A",
        "query": "cpu_usage_active",
        "interval": 30
      }
    ],
    "triggers": [
      {
        "mode": 0,
        "expressions": [
          {"ref": "A", "comparisonOperator": ">", "value": 80, "logicalOperator": "&&"}
        ],
        "severity": 2,
        "recover_config": {"judge_type": 1}
      }
    ],
    "exp_trigger_disable": false
  }
}
```

## 完整示例（v1 格式，CPU 使用率告警）

```json
[{
  "name": "Host CPU使用率过高",
  "note": "主机CPU使用率超过80%",
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

## 查询可用指标

```
GET /api/n9e/proxy/<datasource_id>/api/v1/label/__name__/values
Authorization: Bearer <token>
```
