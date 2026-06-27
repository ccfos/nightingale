# Prometheus 告警规则

- `prod`: `"metric"`
- `cate`: `"prometheus"`

## 格式说明：只用 v1

开源版（OSS）n9e 的前端 v2 规则编辑器被 `IS_PLUS` 开关门控，OSS 环境下 `IS_PLUS=false`，v2 规则会被**按 v1 格式加载**，读不到 `query` 字段，导致 PromQL 输入框为空。

**结论**：OSS n9e 只应使用 **v1 格式**，把阈值**直接写进 `prom_ql`**。v2 格式（查询和触发条件分离）仅在企业版 FE 中可用。

## v1 格式（OSS 唯一可用格式）

```json
{
  "rule_config": {
    "queries": [
      {
        "prom_ql": "cpu_usage_active > 80",
        "severity": 2
      }
    ]
  }
}
```

阈值直接作为比较运算符写在 PromQL 字符串里，不要放在 triggers 里。

## 完整示例（CPU 使用率告警）

```json
[{
  "name": "Host CPU使用率过高",
  "note": "主机CPU使用率超过80%",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [1],
  "datasource_queries": [{"match_type": 0, "op": "in", "values": [1]}],
  "disabled": 0,
  "prom_eval_interval": 30,
  "prom_for_duration": 60,
  "rule_config": {
    "queries": [{"prom_ql": "cpu_usage_active > 80", "severity": 2}]
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

## 多查询 + 表达式

v1 格式也支持多个 query，每个都是独立的"PromQL + 阈值"判断，OR 关系：

```json
{
  "queries": [
    {"prom_ql": "cpu_usage_active > 80", "severity": 2},
    {"prom_ql": "mem_used_percent > 90", "severity": 1}
  ]
}
```

## PromQL 写法备忘

| 需求 | PromQL |
|---|---|
| 平均 CPU 使用率 > 80% | `avg(100 - cpu_usage_idle{cpu="cpu-total"}) > 80` |
| 任一主机内存 > 90% | `mem_used_percent > 90` |
| 某主机内存使用量 > 1GiB | `mem_used{ident="web-01"} > 1073741824` |
| rate 类（QPS > 1000） | `rate(http_requests_total[5m]) > 1000` |

注意：包含算术运算或聚合的 PromQL，**整体**和阈值比较时阈值直接跟在后面即可，n9e engine 能正确解析。
