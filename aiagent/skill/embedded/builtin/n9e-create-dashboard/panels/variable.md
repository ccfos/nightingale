# 模板变量 (IVariable)

模板变量允许用户在仪表盘顶部通过下拉框筛选数据维度（主机、网卡、挂载点等）。

## 完整结构

```json
{
  "name": "ident",
  "label": "主机",
  "type": "query",
  "definition": "label_values(cpu_usage_idle{cpu=\"cpu-total\"},ident)",
  "reg": "",
  "multi": true,
  "allOption": true,
  "allValue": "",
  "defaultValue": "",
  "hide": false,
  "datasource": {
    "cate": "prometheus",
    "value": 1
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 变量名，面板中用 `$name` 或 `${name}` 引用 |
| `label` | string | 显示标签（可选，不填则显示 name） |
| `type` | string | 变量类型，见下方 |
| `definition` | string | 变量值来源（PromQL / 自定义值列表） |
| `reg` | string | 正则过滤（对 query 结果做过滤） |
| `multi` | boolean | 是否允许多选 |
| `allOption` | boolean | 是否显示 "All" 选项 |
| `allValue` | string | "All" 选项的实际值（空则自动拼接） |
| `defaultValue` | string | 默认值 |
| `hide` | boolean | 是否隐藏变量（仍可引用） |
| `datasource` | object | 数据源配置 |

## 变量类型 (type)

### query — 查询变量

从 Prometheus 查询结果动态获取值。`definition` 支持以下语法：

```
label_values(metric_name, label_name)
label_values(metric_name{label="value"}, label_name)
query_result(promql_expression)
```

**示例：**
```json
{ "type": "query", "definition": "label_values(cpu_usage_idle{cpu=\"cpu-total\"}, ident)" }
{ "type": "query", "definition": "label_values(net_bytes_recv{ident=~\"$ident\"}, interface)" }
{ "type": "query", "definition": "label_values(disk_used_percent{ident=~\"$ident\"}, path)" }
```

### custom — 自定义变量

`definition` 中用逗号分隔的固定值列表。

```json
{ "type": "custom", "definition": "5m,15m,30m,1h,6h,12h,24h" }
{ "type": "custom", "definition": "dev,staging,prod" }
```

### textbox — 文本框

用户自由输入，`definition` 为默认值。

```json
{ "type": "textbox", "definition": ".*" }
```

### constant — 常量

隐藏的固定值，通常配合 `hide: true` 使用。

```json
{ "type": "constant", "definition": "my-cluster", "hide": true }
```

### datasource — 数据源变量

动态选择数据源，`definition` 为数据源类型。

```json
{ "type": "datasource", "definition": "prometheus" }
```

面板中用 `datasourceValue: "${datasource}"` 引用。

### hostIdent — 主机标识

n9e 内置的主机选择器，自动关联 CMDB 中的主机。

```json
{ "type": "hostIdent" }
```

## 变量级联

后续变量的 `definition` 中引用前置变量，实现联动过滤：

```
变量1: ident     → label_values(cpu_usage_idle{cpu="cpu-total"}, ident)
变量2: interface → label_values(net_bytes_recv{ident=~"$ident"}, interface)
变量3: mountpoint → label_values(disk_used_percent{ident=~"$ident"}, path)
```

用户选择主机后，网卡和挂载点下拉框自动更新为该主机的值。

## 在 PromQL 中引用变量

```
# 单选变量 — 精确匹配
metric_name{ident="$ident"}

# 多选变量 — 正则匹配（推荐始终用这种）
metric_name{ident=~"$ident"}
```

多选时 n9e 自动将 `$ident` 展开为 `value1|value2|value3` 正则。
