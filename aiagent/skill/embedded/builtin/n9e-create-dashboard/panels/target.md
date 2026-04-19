# 查询配置 (ITarget)

每个面板可以包含多个查询 (targets)，通过 refId (A/B/C...) 区分。

## 完整结构

```json
{
  "refId": "A",
  "__mode__": "__query__",
  "expr": "cpu_usage_active{cpu=\"cpu-total\",ident=~\"$ident\"}",
  "legendFormat": "{{ident}}",
  "step": null,
  "maxDataPoints": null,
  "instant": false,
  "hide": false
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `refId` | string | 查询标识，依次使用 A、B、C... |
| `__mode__` | string | `"__query__"` 普通查询 \| `"__expr__"` 表达式模式 |
| `expr` | string | PromQL 表达式 |
| `legendFormat` | string | 图例模板，支持 `{{label_name}}` |
| `step` | number/null | 最小查询步长(秒)，null 为自动 |
| `maxDataPoints` | number/null | 最大返回数据点，null 为自动 |
| `instant` | boolean | 即时查询（仅取最新一个值，用于 stat/table） |
| `hide` | boolean | 隐藏此查询（仍参与计算但不渲染） |

## legendFormat 模板语法

```
{{label_name}}     — 替换为标签值
{{ident}}          — 主机标识
{{instance}}       — 实例地址
{{__name__}}       — 指标名
```

**示例：**
```
"{{ident}}"                    → "dev-n9e-02"
"{{ident}} - {{interface}}"    → "dev-n9e-02 - eth0"
"{{ident}} {{path}} used"      → "dev-n9e-02 / used"
```

空字符串 `""` 表示使用默认图例（完整标签集）。

## 常用 PromQL 模式

### 直接指标（gauge 类型）
```
cpu_usage_active{cpu="cpu-total",ident=~"$ident"}
mem_used_percent{ident=~"$ident"}
disk_used_percent{ident=~"$ident",path=~"$mountpoint"}
system_load1{ident=~"$ident"}
```

### rate 计算（counter 类型）
```
rate(net_bytes_recv{ident=~"$ident",interface=~"$interface"}[3m])
rate(net_packets_sent{ident=~"$ident",interface=~"$interface"}[3m])
irate(mysql_queries_total{ident=~"$ident"}[5m])
```

### 聚合计算
```
avg(cpu_usage_active{cpu="cpu-total",ident=~"$ident"})
max(disk_used_percent{ident=~"$ident"})
sum by (ident) (rate(net_bytes_recv{ident=~"$ident"}[3m]))
```

### 计算表达式
```
100 - (disk_inodes_free{...} / disk_inodes_total{...} * 100)
mem_used{...} / mem_total{...} * 100
```

## 多查询面板示例

一个面板同时展示多个查询：

```json
{
  "targets": [
    { "refId": "A", "__mode__": "__query__", "expr": "cpu_usage_user{cpu=\"cpu-total\",ident=~\"$ident\"}", "legendFormat": "{{ident}} user" },
    { "refId": "B", "__mode__": "__query__", "expr": "cpu_usage_system{cpu=\"cpu-total\",ident=~\"$ident\"}", "legendFormat": "{{ident}} system" },
    { "refId": "C", "__mode__": "__query__", "expr": "cpu_usage_iowait{cpu=\"cpu-total\",ident=~\"$ident\"}", "legendFormat": "{{ident}} iowait" }
  ]
}
```

## Elasticsearch 查询 (非 Prometheus)

当 `datasourceCate` 为 `"elasticsearch"` 时，target 结构不同：

```json
{
  "refId": "A",
  "query": {
    "index": "logs-*",
    "index_type": "index_pattern",
    "filters": "status:500",
    "date_field": "@timestamp",
    "values": [{ "func": "count" }],
    "group_by": [{ "cate": "date_histogram", "field": "@timestamp" }]
  }
}
```
