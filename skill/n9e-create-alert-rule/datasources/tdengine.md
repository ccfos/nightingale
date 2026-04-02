# TDengine 告警规则

- `prod`: `"metric"`
- `cate`: `"tdengine"`
- `recover_config.judge_type`: `1`（指标类型）

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "query": "SELECT avg(cpu_usage) AS value, host AS label FROM system.cpu WHERE ts >= $from AND ts < $to INTERVAL($interval) GROUP BY host",
        "interval": 1,
        "interval_unit": "min",
        "keys": {
          "labelKey": "label",
          "metricKey": "value",
          "timeFormat": ""
        }
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
    ]
  }
}
```

## query 字段说明

| 字段 | 说明 |
|---|---|
| `query` | TDengine SQL，支持变量 `$from`、`$to`、`$interval` |
| `interval` + `interval_unit` | 查询间隔，单位：`second` / `min` / `hour` |
| `keys.labelKey` | 标签字段名（多个用空格分隔） |
| `keys.metricKey` | 指标值字段名（多个用空格分隔） |
| `keys.timeFormat` | 时间格式（可选） |
