---
name: n9e-create-dashboard
description: 在夜莺(n9e)平台上通过 API 创建监控仪表盘。当用户要求创建仪表盘、监控大盘、Dashboard 时使用。支持指定监控主题（Linux主机、MySQL、Redis等）、目标环境地址、认证信息。
---

# Skill: 夜莺(N9E) 仪表盘创建

## 概述

通过 n9e REST API 自动创建监控仪表盘。流程：登录获取 Token → 查询数据源和业务组 → 探测可用指标 → 根据监控主题组装面板 → 调用 API 创建仪表盘。

## 参考文档索引

组装仪表盘 JSON 时，按需读取以下文件获取完整配置结构：

### 通用配置（每次必读）

| 文件 | 内容 |
|------|------|
| [panels/dashboard-config.md](panels/dashboard-config.md) | 仪表盘整体结构、configs 对象、graphTooltip、links |
| [panels/variable.md](panels/variable.md) | 模板变量 6 种类型、级联设计、PromQL 引用语法 |
| [panels/target.md](panels/target.md) | 查询配置、legendFormat 模板、常用 PromQL 模式 |
| [panels/panel-structure.md](panels/panel-structure.md) | 面板通用结构、布局系统(24列网格)、各类型推荐尺寸、repeat |
| [panels/common-options.md](panels/common-options.md) | 通用 options：standardOptions、thresholds、legend、tooltip、valueMappings、overrides、transformationsNG |
| [panels/units.md](panels/units.md) | 全部单位值(80+种)、聚合函数(calc) |

### 面板类型配置（按需读取）

| 面板类型 | 文件 | 一句话说明 |
|----------|------|-----------|
| timeseries | [panels/timeseries.md](panels/timeseries.md) | 时序折线/柱状图，最常用 |
| stat | [panels/stat.md](panels/stat.md) | 单值统计，概览大数字 |
| gauge | [panels/gauge.md](panels/gauge.md) | 圆弧仪表盘，阈值区间 |
| barGauge | [panels/barGauge.md](panels/barGauge.md) | 水平条形排名对比 |
| pie | [panels/pie.md](panels/pie.md) | 饼图/甜甜圈，占比分析 |
| table | [panels/table.md](panels/table.md) | 经典表格，多种转换模式 |
| tableNG | [panels/tableNG.md](panels/tableNG.md) | 新表格(ag-grid)，列筛选 |
| hexbin | [panels/hexbin.md](panels/hexbin.md) | 蜂窝图，大量实例状态 |
| heatmap | [panels/heatmap.md](panels/heatmap.md) | 热力图，二维分布 |
| barchart | [panels/barchart.md](panels/barchart.md) | 分类柱状图，分组对比 |
| text | [panels/text.md](panels/text.md) | Markdown 文本 |
| iframe | [panels/iframe.md](panels/iframe.md) | 嵌入外部页面 |
| row | [panels/row.md](panels/row.md) | 分组行，折叠/展开 |

## 面板类型选择指南

| 需求 | 推荐类型 |
|------|----------|
| 指标随时间变化趋势 | `timeseries` |
| 当前值概览（大数字） | `stat` |
| 当前值+仪表盘区间 | `gauge` |
| 多实例同指标排序对比 | `barGauge` |
| 占比分析 | `pie` |
| 多维度数据列表 | `table` / `tableNG` |
| 大量实例状态概览 | `hexbin` |
| 二维数值分布 | `heatmap` |
| 分类数值对比 | `barchart` |
| 静态说明文字 | `text` |
| 嵌入外部页面 | `iframe` |
| 面板分组 | `row` |

## 输入参数

从用户消息中提取以下信息（缺少时主动询问）：

| 参数 | 必填 | 说明 | 示例 |
|------|------|------|------|
| `base_url` | 是 | n9e 访问地址 | `http://<n9e-host>:<port>` |
| `username` | 是 | 登录用户名 | `<username>` |
| `password` | 是 | 登录密码 | `<password>` |
| `topic` | 是 | 监控主题 | `linux`、`mysql`、`redis`、`kubernetes` |
| `busi_group` | 否 | 业务组名称或ID，不填则列出可选项让用户选择 | `n9e_test` |
| `dashboard_name` | 否 | 仪表盘名称，不填则自动生成 | `Linux 主机监控` |

## 执行步骤

### 第一步：登录获取 Token

```bash
curl -s -X POST '${base_url}/api/n9e/auth/login' \
  -H 'Content-Type: application/json' \
  -d '{"username":"${username}","password":"${password}"}'
```

从响应 `dat.access_token` 提取 Token，后续所有请求携带 `Authorization: Bearer ${TOKEN}`。

### 第二步：查询业务组和数据源

并行执行两个请求：

```bash
# 获取业务组列表
curl -s '${base_url}/api/n9e/busi-groups' \
  -H "Authorization: Bearer ${TOKEN}"

# 获取数据源列表
curl -s '${base_url}/api/n9e/datasource/brief' \
  -H "Authorization: Bearer ${TOKEN}"
```

- 从业务组响应中匹配用户指定的 `busi_group`（按名称或 ID），若未指定则列出让用户选择
- 从数据源中找到 `plugin_type: "prometheus"` 的数据源，记录其 `id`（优先选 `is_default: true` 的）

### 第三步：探测可用指标

根据监控主题，查询 Prometheus 中实际存在的指标，确定指标命名风格（categraf 风格 vs node_exporter 风格）：

```bash
# 获取所有指标名
curl -s "${base_url}/api/n9e/proxy/${datasource_id}/api/v1/label/__name__/values" \
  -H "Authorization: Bearer ${TOKEN}"
```

筛选与主题相关的指标，并查询样本数据确认标签结构：

```bash
# 查询样本确认标签
curl -s "${base_url}/api/n9e/proxy/${datasource_id}/api/v1/query?query=${sample_metric}" \
  -H "Authorization: Bearer ${TOKEN}"
```

**关键：根据实际指标名和标签来构建 PromQL，不要假设指标格式。**

常见指标风格对照：

| 监控项 | categraf 风格 | node_exporter 风格 |
|--------|--------------|-------------------|
| CPU 使用率 | `cpu_usage_active` | `node_cpu_seconds_total` |
| 内存使用率 | `mem_used_percent` | 需计算：`node_memory_*` |
| 磁盘使用率 | `disk_used_percent` | 需计算：`node_filesystem_*` |
| 网络流量 | `net_bytes_recv/sent` | `node_network_receive/transmit_bytes_total` |
| 系统负载 | `system_load1/5/15` | `node_load1/5/15` |
| 主机标识标签 | `ident` | `instance` |

### 第四步：组装仪表盘 JSON

读取上方「参考文档索引」中的通用配置文件和所需面板类型文件，按以下顺序组装：

1. 构建 `configs` 对象（结构见 [dashboard-config.md](panels/dashboard-config.md)）
2. 定义模板变量 `var`（结构见 [variable.md](panels/variable.md)）
3. 逐个构建面板，每个面板包含：
   - 通用字段 + layout（结构见 [panel-structure.md](panels/panel-structure.md)）
   - targets 查询（结构见 [target.md](panels/target.md)）
   - options 可视化选项（结构见 [common-options.md](panels/common-options.md)，单位见 [units.md](panels/units.md)）
   - custom 面板特有配置（见对应面板类型文件）

### 第五步：调用 API 创建仪表盘

```bash
curl -s -X POST "${base_url}/api/n9e/busi-group/${busi_group_id}/boards" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "仪表盘名称",
    "tags": "标签",
    "configs": "<JSON字符串化的configs>"
  }'
```

从响应中获取 `dat.id`，拼接访问地址：`${base_url}/dashboards/${id}`

### 第六步：验证并输出结果

输出内容：
1. 仪表盘访问 URL
2. 所属业务组
3. 面板清单（表格形式，包含区域、面板名、类型、查询的指标）
4. 模板变量列表

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
| CPU | CPU详情(堆叠) | timeseries(stack) | `cpu_usage_user/system/iowait/steal` |
| 内存 | 内存使用率趋势 | timeseries | `mem_used_percent` |
| 内存 | 内存组成(堆叠) | timeseries(stack) | `mem_used/buffered/cached/free` |
| 磁盘 | 各挂载点使用率 | barGauge | `disk_used_percent` |
| 磁盘 | 磁盘空间 | timeseries | `disk_total` / `disk_free` |
| 磁盘 | Inode使用率 | timeseries | `disk_inodes_free/total` |
| 网络 | 网络流量 | timeseries | `rate(net_bytes_recv/sent)` |
| 网络 | 网络数据包 | timeseries | `rate(net_packets_recv/sent)` |
| 网络 | 错误与丢包 | timeseries | `rate(net_err_in/out)` + `rate(net_drop_in/out)` |

### MySQL 监控

**变量：** `ident`（主机）

推荐面板：QPS/TPS、连接数、慢查询、Buffer Pool命中率、复制延迟、表锁等待、InnoDB行操作

### Redis 监控

**变量：** `ident`（主机）

推荐面板：OPS、内存使用、连接数、命中率、键空间、慢日志、持久化状态

### Kubernetes 监控

**变量：** `cluster`、`namespace`、`pod`

推荐面板：Pod CPU/内存、节点资源、部署状态、网络策略、PV使用

## 注意事项

1. **指标探测优先**：一定要先查询实际存在的指标，再构建 PromQL，不同环境采集器不同（categraf vs node_exporter vs telegraf）
2. **configs 是字符串**：创建 API 的 `configs` 字段必须是 JSON 字符串，不是对象
3. **面板 ID 唯一性**：每个面板的 `id` 和 `layout.i` 必须一致且全局唯一
4. **布局不重叠**：计算好 `y` 值，确保面板不互相覆盖
5. **变量级联**：后续变量的 `definition` 中引用前置变量 `$ident` 实现级联过滤
6. **阈值颜色**：绿 `#3FC453`、橙 `#FF9919`、红 `#FF656B` 是 n9e 标准色
7. **rate 函数**：对 counter 类型指标（如 `net_bytes_recv`）使用 `rate(...[3m])` 或 `irate(...[5m])`
8. **version 字段**：dashboard 和每个 panel 的 version 都统一使用 `"3.4.0"`
