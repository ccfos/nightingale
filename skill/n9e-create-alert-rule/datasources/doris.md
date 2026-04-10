# Doris 告警规则

- `prod`: `"logging"`
- `cate`: `"doris"`
- `recover_config.judge_type`: `1`
- **必填** `keys.valueKey`：SELECT 语句中数值列的别名
- **必填** `database`：查询所在数据库名

## OSS 版本限制

**开源版 n9e 的 Doris 数据源不支持 `$from`/`$to`/`$__timeFilter` 等时间变量**，变量不会被替换。

**正确写法**：使用 Doris 原生时间函数：
- 过去 N 分钟：`WHERE log_time >= NOW() - INTERVAL 5 MINUTE`
- 过去 N 小时：`WHERE log_time >= NOW() - INTERVAL 1 HOUR`

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "database": "my_database",
        "sql": "SELECT count(*) AS value FROM log_table WHERE log_time >= NOW() - INTERVAL 5 MINUTE AND level = 'ERROR'",
        "keys": {
          "valueKey": "value",
          "labelKey": ""
        },
        "interval": 300
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

| 字段 | 必填 | 说明 |
|---|---|---|
| `ref` | ✅ | 查询引用名 |
| `database` | ✅ | Doris 数据库名 |
| `sql` | ✅ | SQL 查询，数值列别名须与 `keys.valueKey` 一致 |
| `keys.valueKey` | ✅ | **必填**，数值列的别名 |
| `keys.labelKey` | ❌ | 标签列别名，多个用空格分隔 |
| `interval` | ❌ | 查询执行间隔，**单位：总秒数**（60=1分钟，300=5分钟，3600=1小时）。**不要写 `interval_unit`** |
