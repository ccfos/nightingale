# Doris 告警规则

- `prod`: `"logging"`
- `cate`: `"doris"`
- `recover_config.judge_type`: `1`

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count(*) AS value FROM log_table WHERE log_time >= DATE_TRUNC('minute', NOW() - INTERVAL 5 MINUTE) AND level = 'ERROR'",
        "database": "my_database",
        "interval": 5,
        "interval_unit": "min"
      }
    ],
    "triggers": [
      {
        "mode": 0,
        "expressions": [
          {"ref": "A", "comparisonOperator": ">", "value": 50, "logicalOperator": "&&"}
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
| `database` | **必填**，Doris 数据库名 |
| `sql` | SQL 查询，建议包含时间关键字：`TIMESTAMP` / `DATE` / `INTERVAL` / `DATE_TRUNC` / `NOW()` / `$__timeFilter` |
| `interval` + `interval_unit` | 查询间隔，单位：`second` / `min` / `hour` |
