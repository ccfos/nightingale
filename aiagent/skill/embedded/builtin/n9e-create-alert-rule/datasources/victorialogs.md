# VictoriaLogs 告警规则

- `prod`: `"logging"`
- `cate`: `"victorialogs"`
- `recover_config.judge_type`: `0`（日志类型）

## ⚠️ 核心约束：必须用 pipe 语法

VictoriaLogs 告警走的是 **`/select/logsql/stats_query`** 端点（见 `dskit/victorialogs/victorialogs.go:StatsQuery`），它**只接受带 `| stats` 管道的聚合查询**，不接受纯过滤查询。

错误写法（只会报错或返回空）：
```
_msg:error
service:payment AND level:error
```

正确写法：
```
<过滤条件> | stats <聚合函数>
```

| 需求 | LogsQL |
|---|---|
| 过去 5 分钟错误日志条数 | `_msg:error \| stats count() as value` |
| 某服务的错误日志条数 | `service:payment AND level:error \| stats count() as value` |
| 按服务分组统计 | `level:error \| stats by (service) count() as value` |
| 平均响应时间 | `* \| stats avg(duration) as value` |
| 多个聚合 | `_msg:error \| stats count() as error_count, avg(duration) as avg_dur` |

**关键规则**：
- `| stats` 后面必须给每个聚合函数起别名（`as value` / `as count` 等）
- 别名会作为返回结果的字段名，告警引擎按别名匹配
- 分组用 `stats by (field1, field2) ...`
- 过滤部分可以是任意 LogsQL filter：`_msg:keyword`, `field:value`, `field:"value with space"`, `_time:5m` 等

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "query": "_msg:error | stats count() as value",
        "interval": 60
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

| 字段 | 必填 | 说明 |
|---|---|---|
| `ref` | ✅ | 查询引用名 |
| `query` | ✅ | LogsQL 查询，**必须是 `<过滤> \| stats <函数>` 格式** |
| `interval` | ❌ | 查询间隔，**单位：总秒数**（60=1分钟，300=5分钟）。**不要写 `interval_unit`** |

## 完整示例

```json
[{
  "name": "应用错误日志告警",
  "note": "过去 1 分钟 payment 服务错误日志超过 20 条",
  "prod": "logging",
  "cate": "victorialogs",
  "datasource_ids": [11],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": [11]}],
  "disabled": 0,
  "prom_eval_interval": 30,
  "prom_for_duration": 60,
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "query": "service:payment AND _msg:error | stats count() as value",
        "interval": 60
      }
    ],
    "triggers": [
      {
        "mode": 0,
        "expressions": [{"ref": "A", "comparisonOperator": ">", "value": 20, "logicalOperator": "&&"}],
        "severity": 2,
        "recover_config": {"judge_type": 0}
      }
    ]
  },
  "notify_version": 1
}]
```

## 错误排查

| 现象 | 原因 | 修复 |
|---|---|---|
| 规则保存成功但始终无数据 | query 只写了过滤条件，没有 `\| stats` | 加上 `\| stats count() as value` 或其他聚合 |
| `no stats clause` 错误 | 同上 | 同上 |
| 聚合值取不到 | 聚合没有 alias | 给聚合加 `as value`，并让 `value` 与阈值匹配对应 |
