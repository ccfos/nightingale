# ClickHouse 告警规则

- `prod`: `"metric"` 或 `"logging"`
- `cate`: `"ck"`
- `recover_config.judge_type`: `1`

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count() AS value FROM logs WHERE timestamp >= $from AND timestamp < $to AND level = 'ERROR'",
        "interval": 1,
        "interval_unit": "min"
      }
    ],
    "triggers": [
      {
        "mode": 0,
        "expressions": [
          {"ref": "A", "comparisonOperator": ">", "value": 100, "logicalOperator": "&&"}
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
| `sql` | ClickHouse SQL 查询，支持变量 `$from`、`$to` |
| `interval` + `interval_unit` | 查询间隔，单位：`second` / `min` / `hour` |
