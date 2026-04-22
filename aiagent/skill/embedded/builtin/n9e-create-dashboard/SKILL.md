---
name: n9e-create-dashboard
description: 在夜莺(n9e)平台上创建监控仪表盘。当用户要求创建仪表盘、监控大盘、Dashboard 时使用。
max_iterations: 18
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

#### 业务组选择规则

1. 如果用户在提示词里明确指定了业务组名称或 ID，直接使用
2. 否则从 `list_busi_groups` 返回结果中按以下优先级选择：
   a. **优先选择 `is_default: true` 的业务组**（这是工具返回的"默认组"提示，通常对应 "Default Busi Group" 或含"默认"的组）
   b. 只有一个业务组时直接用
   c. 多个候选且都不是默认组时，**不要盲目取第一个**，在最终回复里列出让用户确认
3. **绝对不要使用看起来是测试组的业务组**（如名字纯数字 `"123"`、含 `test`/`demo`/`临时`/`tmp` 的组）

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
- 如果 `create_dashboard` 返回 `Name duplicate` 错误，**不要调用 `list_dashboards`** 去查重名，直接在原名后面追加后缀（如 `-v2`、`-AI` 或当前时间戳）重新调用 `create_dashboard`

### 第四步：输出结果

**保持简短**。只需一句话确认仪表盘已创建，例如：

> ✅ 已为您创建仪表盘「Linux 主机监控」，详情请查看下方卡片。

**不要**在 Final Answer 里复述仪表盘 ID、业务组、数据源、面板清单、变量列表等字段——前端会以结构化卡片展示这些信息，重复输出只会让用户看到两份。如果需要补充说明（例如"建议再添加一个网络流量面板"），可以再加一两句，但不要把卡片里已有的字段逐条列出来。

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
