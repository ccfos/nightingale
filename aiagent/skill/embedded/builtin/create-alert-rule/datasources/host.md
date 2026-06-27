# Host 机器监控告警规则

- `prod`: `"host"`
- `cate`: `"host"`
- **不需要指定 datasource_ids**

Host 类型比较特殊，queries 和 triggers 结构与其他类型完全不同。

## rule_config 结构

```json
{
  "rule_config": {
    "queries": [
      {
        "key": "all_hosts",
        "op": "==",
        "values": []
      }
    ],
    "triggers": [
      {
        "type": "target_miss",
        "severity": 2,
        "duration": 30
      }
    ]
  }
}
```

## queries 字段说明

### key 可选值

| key | 说明 | values 示例 |
|---|---|---|
| `all_hosts` | 所有主机 | `[]` |
| `group_ids` | 按业务组筛选 | `[1, 2, 3]` |
| `tags` | 按标签筛选 | `["env=prod", "region=cn"]` |
| `hosts` | 按主机名筛选 | `["web-01", "web-02"]` |

### op 可选值

| op | 说明 |
|---|---|
| `==` | 等于 |
| `!=` | 不等于 |
| `=~` | 正则匹配（仅 `hosts` key 支持） |
| `!~` | 正则不匹配（仅 `hosts` key 支持） |

多个 queries 之间是 AND 逻辑关系。

## triggers 字段说明

### type 可选值

| type | 说明 | 额外字段 |
|---|---|---|
| `target_miss` | 机器失联 | `duration`（秒） |
| `pct_target_miss` | 机器失联百分比 | `duration`（秒）+ `percent`（百分比） |
| `offset` | 时间偏移过大 | `duration`（秒） |

## 完整示例（机器失联告警）

```json
[{
  "name": "机器失联告警",
  "note": "机器超过60秒未上报数据",
  "prod": "host",
  "cate": "host",
  "datasource_ids": [],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": []}],
  "disabled": 0,
  "prom_eval_interval": 30,
  "prom_for_duration": 0,
  "rule_config": {
    "queries": [{"key": "all_hosts", "op": "==", "values": []}],
    "triggers": [{"type": "target_miss", "severity": 1, "duration": 60}]
  },
  "enable_in_bg": 0,
  "enable_days_of_weeks": [["0","1","2","3","4","5","6"]],
  "enable_stimes": ["00:00"],
  "enable_etimes": ["00:00"],
  "notify_recovered": 1,
  "notify_repeat_step": 60,
  "notify_max_number": 0,
  "callbacks": [],
  "append_tags": [],
  "annotations": {},
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": []
}]
```
