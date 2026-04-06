---
name: n9e-create-dashboard
description: 在夜莺(n9e)平台上创建监控仪表盘。当用户要求创建仪表盘、监控大盘、Dashboard 时使用。
builtin_tools:
  - create_dashboard
  - list_files
  - read_file
  - grep_files
---

# Skill: 夜莺(N9E) 仪表盘创建

## 概述

使用 `create_dashboard` 工具创建仪表盘。**只需提供面板标题、类型和 PromQL 查询**，工具会自动生成完整的仪表盘配置（布局、数据源绑定、样式等全部自动处理）。

## create_dashboard 调用示例

```json
{
  "group_id": 1,
  "name": "Linux 主机监控",
  "datasource_id": 1,
  "tags": "linux 主机",
  "variables": "[{\"name\":\"ident\",\"label\":\"主机\",\"definition\":\"label_values(cpu_usage_idle, ident)\"}]",
  "panels": "[{\"name\":\"CPU使用率\",\"type\":\"stat\",\"queries\":[{\"promql\":\"avg(cpu_usage_active{cpu=\\\"cpu-total\\\",ident=~\\\"$ident\\\"})\",\"legend\":\"CPU\"}],\"unit\":\"percent\"},{\"name\":\"内存使用率\",\"type\":\"stat\",\"queries\":[{\"promql\":\"avg(mem_used_percent{ident=~\\\"$ident\\\"})\",\"legend\":\"内存\"}],\"unit\":\"percent\"},{\"name\":\"CPU使用率趋势\",\"type\":\"timeseries\",\"queries\":[{\"promql\":\"cpu_usage_active{cpu=\\\"cpu-total\\\",ident=~\\\"$ident\\\"}\",\"legend\":\"{{ident}}\"}],\"unit\":\"percent\"},{\"name\":\"内存使用率趋势\",\"type\":\"timeseries\",\"queries\":[{\"promql\":\"mem_used_percent{ident=~\\\"$ident\\\"}\",\"legend\":\"{{ident}}\"}],\"unit\":\"percent\"}]"
}
```

## 面板描述格式

每个面板只需 3 个必填字段：

```json
{
  "name": "面板标题",
  "type": "timeseries",
  "queries": [
    {"promql": "PromQL表达式", "legend": "{{ident}}"}
  ]
}
```

### 可选字段

| 字段 | 说明 | 默认值 |
|------|------|--------|
| `w` | 宽度(网格列数，总宽24) | 按类型自动: timeseries=12, stat=6 |
| `h` | 高度(网格行数) | 按类型自动: timeseries=8, stat=4 |
| `unit` | 单位 | 无 |
| `stack` | 是否堆叠(仅timeseries) | false |
| `description` | 面板描述 | 无 |

### 面板类型

| type | 说明 | 默认尺寸(w×h) |
|------|------|---------------|
| `timeseries` | 时序折线图，最常用 | 12×8 |
| `stat` | 单值统计大数字 | 6×4 |
| `gauge` | 仪表盘 | 6×6 |
| `barGauge` | 水平条形排名 | 8×8 |
| `pie` | 饼图 | 6×6 |
| `table` | 表格 | 12×10 |
| `row` | 分组行（自动全宽） | 24×1 |

### 常用单位(unit)

`percent` | `bytesIEC` | `bitsIEC` | `bytesSecIEC` | `bitsSecIEC` | `seconds` | `milliseconds` | `reqps`

### 布局自动计算规则

- 面板从左到右、从上到下自动排列
- 同类型面板自动对齐（如 4 个 stat 面板自动排成一行）
- `row` 类型自动独占一行作为分组标题

## 变量描述格式

```json
{
  "name": "ident",
  "label": "主机",
  "definition": "label_values(cpu_usage_idle, ident)"
}
```

`label` 和 `multi` 可选（multi 默认 true）。

### 变量级联

后续变量的 definition 中引用前置变量实现级联：

```json
[
  {"name": "ident", "definition": "label_values(cpu_usage_idle, ident)"},
  {"name": "interface", "definition": "label_values(net_bytes_recv{ident=~\"$ident\"}, interface)"},
  {"name": "mountpoint", "definition": "label_values(disk_used_percent{ident=~\"$ident\"}, path)"}
]
```

## 执行步骤

### 第一步：查询业务组和数据源

- 调用 `list_busi_groups` 获取业务组列表
- 调用 `list_datasources` 获取数据源列表，找到 **Prometheus 类型的数据源 ID**

### 第二步：参考集成模板获取正确的 PromQL

**优先从 integrations 目录获取经过验证的 PromQL 表达式**，而不是自己编写。

1. 查看有哪些集成模板可用：
   ```
   list_files(base="integrations")
   ```
2. 找到对应监控主题的仪表盘模板（如 Linux 主机监控）：
   ```
   list_files(base="integrations/Linux", path="dashboards")
   ```
3. 读取仪表盘模板，提取面板的 PromQL 查询（关注 targets 中的 expr 字段）：
   ```
   read_file(base="integrations/Linux", path="dashboards/categraf-detail.json")
   ```
4. 如果需要了解可用指标，读取 metrics 文件：
   ```
   read_file(base="integrations/Linux", path="metrics/categraf-base.json")
   ```

**注意**：integrations 下的仪表盘模板分 `categraf` 和 `exporter` 两种风格。使用 `list_metrics` 探测环境中实际存在的指标来决定用哪种风格。

### 第三步：组装面板和变量并创建

根据第二步获取的 PromQL，组装 `panels` 和 `variables` JSON 数组，调用 `create_dashboard`。

**注意事项：**
- 多选变量在 PromQL 中用 `=~` 而不是 `=`，如 `ident=~"$ident"`
- counter 类型指标用 `rate(...[3m])` 或 `irate(...[5m])`

### 第四步：输出结果

输出：仪表盘 ID、面板清单（表格形式）、模板变量列表

## 各监控主题的推荐面板设计

### Linux 主机监控

**变量：** `ident`（主机）、`interface`（网卡）、`mountpoint`（挂载点）

| 区域 | 面板 | 类型 | 核心指标 |
|------|------|------|----------|
| 概览 | CPU使用率 | stat | `avg(cpu_usage_active{cpu="cpu-total"})` |
| 概览 | 内存使用率 | stat | `avg(mem_used_percent)` |
| 概览 | 磁盘使用率(最高) | stat | `max(disk_used_percent)` |
| 概览 | 运行时间 | stat | `min(system_uptime)` |
| CPU | CPU使用率趋势 | timeseries | `cpu_usage_active` + `cpu_usage_iowait` |
| CPU | 系统负载 | timeseries | `system_load1/5/15` |
| 内存 | 内存使用率趋势 | timeseries | `mem_used_percent` |
| 磁盘 | 各挂载点使用率 | barGauge | `disk_used_percent` |
| 网络 | 网络流量 | timeseries | `rate(net_bytes_recv/sent)` |
| 网络 | 错误与丢包 | timeseries | `rate(net_err_in/out)` + `rate(net_drop_in/out)` |

### MySQL 监控

推荐面板：QPS/TPS、连接数、慢查询、Buffer Pool命中率、复制延迟

### Redis 监控

推荐面板：OPS、内存使用、连接数、命中率、键空间

### Kubernetes 监控

**变量：** `cluster`、`namespace`、`pod`

推荐面板：Pod CPU/内存、节点资源、部署状态、PV使用
