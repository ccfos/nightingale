# TDengine 告警规则

- `prod`: `"metric"`
- `cate`: `"tdengine"`
- `recover_config.judge_type`: `1`（指标类型）

## ⚠️ 核心约束

1. **SELECT 结果中必须有 TIMESTAMP 类型列**。n9e 解析时序数据时按 `column_meta` 找类型为 `TIMESTAMP` 的列作为时间轴。如果用 `SELECT *`，原表的 `ts` 或 `_ts` 列会自然包含在内。如果用聚合函数，需要显式 `SELECT _wstart`。
2. **`value` 是 TDengine 保留字，不能用作别名**。用 `AS val`、`AS metric_val` 等替代。如果误用 `AS value` 会报语法错误，但 n9e 会把错误吞掉显示为 "timestamp column not found"。
3. **keys.metricKey 和 keys.labelKey 必须与 SELECT 列名精确匹配**。

## 时间变量支持

TDengine 是 OSS n9e 中**唯一支持时间变量替换**的 SQL 类数据源：

| 变量 | 替换为 | 示例值 |
|---|---|---|
| `$from` | RFC3339 字符串（带单引号） | `'2026-04-09T08:00:00Z'` |
| `$to` | RFC3339 字符串（带单引号） | `'2026-04-09T08:05:00Z'` |
| `$interval` | 秒数字符串 | `60s` |

SQL 里直接写 `_ts >= $from AND _ts < $to`，**不要再加引号**。

## 标准查询模式（推荐）

最简单可靠的模式：`SELECT * FROM db.table WHERE 时间列 >= $from AND 时间列 < $to`，靠 `keys` 声明哪些列是指标、哪些是标签。

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "query": "SELECT * FROM power.meters WHERE ts >= $from AND ts < $to",
        "interval": 60,
        "keys": {
          "metricKey": "current voltage",
          "labelKey": "location",
          "timeFormat": ""
        }
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

**说明**：
- `SELECT *` 返回所有列（含 TIMESTAMP 列 `ts`），满足 n9e 对时间列的要求
- `metricKey: "current voltage"` 告诉引擎 `current` 和 `voltage` 是数值指标（空格分隔多个）
- `labelKey: "location"` 告诉引擎 `location` 是标签维度
- trigger 里的 `A > 10` 表示任何一个 metricKey 列的值超过 10 就触发

## 窗口聚合模式（高级）

需要按时间窗口聚合时（如求平均值），用 `_wstart` + `PARTITION BY` + `INTERVAL`：

```json
{
  "queries": [
    {
      "ref": "A",
      "query": "SELECT _wstart, avg(current) AS avg_current, location FROM power.meters WHERE ts >= $from AND ts < $to PARTITION BY location INTERVAL($interval)",
      "interval": 60,
      "keys": {
        "metricKey": "avg_current",
        "labelKey": "location",
        "timeFormat": ""
      }
    }
  ],
  "triggers": [
    {
      "mode": 0,
      "expressions": [{"ref": "A", "comparisonOperator": ">", "value": 12, "logicalOperator": "&&"}],
      "severity": 2,
      "recover_config": {"judge_type": 1}
    }
  ]
}
```

**注意**：
- **必须 SELECT `_wstart`**，否则结果无 TIMESTAMP 列
- 别名**不能用 `value`**（保留字），用 `avg_current`、`val`、`metric_val` 等
- 分组用 **`PARTITION BY`**（TDengine 3.x），不是 `GROUP BY`
- `metricKey` 必须与别名精确匹配（如 `"avg_current"`）

## query 字段说明

| 字段 | 必填 | 说明 |
|---|---|---|
| `ref` | ✅ | 查询引用名 |
| `query` | ✅ | TDengine SQL。结果集**必须包含 TIMESTAMP 列**，别名**不能用 `value`** |
| `keys.metricKey` | ✅ | 数值列名，多个空格分隔。触发器按这些列的值做阈值判断 |
| `keys.labelKey` | ❌ | 标签列名，多个空格分隔 |
| `keys.timeFormat` | ❌ | 时间格式（一般留空） |
| `interval` | ❌ | 查询间隔，**单位：总秒数**。同时作为 `$interval` 宏的值 |

## 错误排查

| 报错 | 原因 | 修复 |
|---|---|---|
| `timestamp column not found` | SELECT 结果中无 TIMESTAMP 列，或 SQL 语法错误被静默吞掉 | 用 `SELECT *` 模式确保包含时间列；检查别名是否用了保留字 `value` |
| 语法错误但显示 timestamp not found | `value` 是保留字导致 SQL 解析失败，被 QueryTable 吞掉 | 把 `AS value` 改成 `AS val` 或 `AS metric_val` |
| `Query memory exhausted` | INTERVAL 查询时间范围太大 | 缩小 `$from`~`$to` 范围，或增大 INTERVAL 窗口 |
