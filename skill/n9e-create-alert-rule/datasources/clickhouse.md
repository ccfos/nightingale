# ClickHouse 告警规则

- `prod`: `"metric"` 或 `"logging"`
- `cate`: `"ck"`
- `recover_config.judge_type`: `1`（指标类型）
- **必填** `keys.valueKey`：SELECT 语句中数值列的别名

## OSS 版本限制

**开源版 n9e 的 ClickHouse 数据源不支持 `$from`/`$to`/`$__timeFilter` 等时间变量**，变量不会被替换。

**正确写法**：使用 ClickHouse 原生时间函数：
- 过去 N 分钟：`WHERE timestamp >= now() - INTERVAL 5 MINUTE`
- 过去 N 小时：`WHERE timestamp >= now() - INTERVAL 1 HOUR`
- 今天：`WHERE toDate(timestamp) = today()`

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count() AS value FROM logs WHERE timestamp >= now() - INTERVAL 1 MINUTE AND level = 'ERROR'",
        "keys": {
          "valueKey": "value",
          "labelKey": ""
        },
        "interval": 60
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

| 字段 | 必填 | 说明 |
|---|---|---|
| `ref` | ✅ | 查询引用名 |
| `sql` | ✅ | ClickHouse SQL，数值列别名须与 `keys.valueKey` 一致 |
| `keys.valueKey` | ✅ | **必填**，数值列的别名 |
| `keys.labelKey` | ❌ | 标签列别名，多个用空格分隔 |
| `interval` | ❌ | 查询执行间隔，**单位：总秒数**（60=1分钟，300=5分钟，3600=1小时）。**不要写 `interval_unit`** |
