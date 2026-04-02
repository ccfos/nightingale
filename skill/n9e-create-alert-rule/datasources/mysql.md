# MySQL 告警规则

- `prod`: `"metric"`
- `cate`: `"mysql"`
- `recover_config.judge_type`: `1`（指标类型）

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count(*) AS value FROM orders WHERE created_at >= FROM_UNIXTIME($from) AND created_at < FROM_UNIXTIME($to) AND status = 'failed'",
        "interval": 1,
        "interval_unit": "min"
      }
    ],
    "triggers": [
      {
        "mode": 0,
        "expressions": [
          {"ref": "A", "comparisonOperator": ">", "value": 10, "logicalOperator": "&&"}
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
| `sql` | MySQL SQL 查询，支持变量 `$from`、`$to`（Unix 时间戳） |
| `interval` + `interval_unit` | 查询间隔，单位：`second` / `min` / `hour` |
