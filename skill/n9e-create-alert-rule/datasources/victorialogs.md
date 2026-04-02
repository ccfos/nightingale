# VictoriaLogs 告警规则

- `prod`: `"logging"`
- `cate`: `"victorialogs"`
- `recover_config.judge_type`: `0`（日志类型）

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "query": "_msg:error AND service:payment",
        "interval": 1,
        "interval_unit": "min"
      }
    ],
    "triggers": [
      {
        "mode": 0,
        "expressions": [
          {"ref": "A", "comparisonOperator": ">", "value": 20, "logicalOperator": "&&"}
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
| `query` | LogsQL 查询语言 |
| `interval` + `interval_unit` | 查询间隔，单位：`second` / `min` / `hour` |
