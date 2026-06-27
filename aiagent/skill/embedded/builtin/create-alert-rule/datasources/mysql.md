# MySQL 告警规则

- `prod`: `"metric"`
- `cate`: `"mysql"`
- `recover_config.judge_type`: `1`（指标类型）
- **必填** `keys.valueKey`：SELECT 语句中数值列的别名（通常叫 `value`）

## OSS 版本限制

**开源版 n9e 的 MySQL 数据源不支持 `$from`/`$to`/`$__timeFilter` 等时间变量**（`macros.Macro` 绑定的是 no-op 实现，变量会原样进入 SQL 导致语法错）。

**正确写法**：使用 MySQL 原生时间函数，比如：
- 过去 N 分钟：`WHERE created_at >= NOW() - INTERVAL 5 MINUTE`
- 过去 N 小时：`WHERE created_at >= NOW() - INTERVAL 1 HOUR`
- 今天开始至今：`WHERE DATE(created_at) = CURDATE()`

## triggers 硬规则（必读）

- `exp` **必填**，是告警引擎唯一评估的字段（不写 exp 的规则建出来永远不会触发，且无任何报错）
- 本数据源的变量写法：`$<ref>.<valueKey 别名>`，如 `$A.value > 10`；只有一个 valueKey 时可省略别名直接写 `$A`，多个 valueKey 时**必须带别名**（裸 `$A` 取值不确定）
- `mode` 固定填 `1`（表达式模式，前端原样展示 exp）；多条件用 `&&` / `||` 连接，如 `"$A.value > 10 && $B.value < 5"`

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count(*) AS value FROM orders WHERE created_at >= NOW() - INTERVAL 5 MINUTE AND status = 'failed'",
        "keys": {
          "valueKey": "value",
          "labelKey": ""
        },
        "interval": 60
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A.value > 10",
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
| `ref` | ✅ | 查询引用名，通常是 `A`、`B` ... |
| `sql` | ✅ | MySQL SQL 查询。**必须有一列别名为 `value`（或与 `keys.valueKey` 一致）作为告警判断值**。时间过滤用 `NOW() - INTERVAL X MINUTE` 等原生写法，不要用 `$from`/`$to` |
| `keys.valueKey` | ✅ | **必填**，数值列的别名，例如 `"value"` |
| `keys.labelKey` | ❌ | 标签列别名，多个用空格分隔，用于按维度分组告警（如 `"host service"`） |
| `interval` | ❌ | 查询执行间隔，**单位：总秒数**。例如 60=1分钟，300=5分钟，3600=1小时。**不要写 `interval_unit`** |

## 多维度示例（按 host 分组）

```json
{
  "queries": [
    {
      "ref": "A",
      "sql": "SELECT host AS label, count(*) AS value FROM errors WHERE created_at >= NOW() - INTERVAL 5 MINUTE GROUP BY host",
      "keys": {"valueKey": "value", "labelKey": "label"},
      "interval": 60
    }
  ],
  "triggers": [
    {
      "mode": 1,
      "exp": "$A.value > 100",
      "severity": 2,
      "recover_config": {"judge_type": 1}
    }
  ]
}
```
