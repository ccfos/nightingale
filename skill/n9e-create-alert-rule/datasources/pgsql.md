# PostgreSQL 告警规则

- `prod`: `"metric"`
- `cate`: `"pgsql"`
- `recover_config.judge_type`: `1`（指标类型）

结构与 MySQL 相同，SQL 语法需符合 PostgreSQL。

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count(*) AS value FROM events WHERE created_at >= to_timestamp($from) AND created_at < to_timestamp($to) AND severity = 'critical'",
        "interval": 1,
        "interval_unit": "min"
      }
    ],
    "triggers": [
      {
        "mode": 0,
        "expressions": [
          {"ref": "A", "comparisonOperator": ">", "value": 5, "logicalOperator": "&&"}
        ],
        "severity": 1,
        "recover_config": {"judge_type": 1}
      }
    ]
  }
}
```

## query 字段说明

| 字段 | 说明 |
|---|---|
| `sql` | PostgreSQL SQL 查询，支持变量 `$from`、`$to`（Unix 时间戳） |
| `interval` + `interval_unit` | 查询间隔，单位：`second` / `min` / `hour` |
