---
name: n9e-create-dashboard
description: 在夜莺(n9e)平台上创建监控仪表盘。当用户要求创建仪表盘、监控大盘、Dashboard 时使用。
max_iterations: 18
builtin_tools:
  - import_dashboard_template
  - create_dashboard
  - list_files
  - read_file
  - grep_files
---

# Skill: 夜莺(N9E) 仪表盘创建

创建仪表盘有两条路径，**优先用方式 A（导入集成模板）**，模板是人工精调验证过的成品，质量远高于手工拼装；只有在没有现成模板或用户要自定义指标时才走方式 B。

| 场景 | 用哪个工具 |
|------|-----------|
| 监控主题在 integrations 里有现成模板（Linux / MySQL / Redis / Kafka / PostgreSQL / Elasticsearch / Ceph / Oracle / Nginx / Windows …） | **`import_dashboard_template`**（首选） |
| 自定义指标、没有现成模板、或用户明确要求自定义面板 | `create_dashboard` |

## 第一步（两条路径通用）：确定业务组和数据源

- 调用 `list_busi_groups` 获取业务组列表
- 调用 `list_datasources` 获取数据源列表，找到 **Prometheus 类型的数据源 ID**
- 若会话上下文已预选 `busi_group_id` / `datasource_id`，直接用，**不要**再调 `list_*`

### 业务组选择规则

1. 用户明确指定了业务组名称或 ID，直接使用
2. 否则按优先级：
   a. **优先 `is_default: true` 的业务组**（通常是 "Default Busi Group" 或含"默认"的组）
   b. 只有一个业务组时直接用
   c. 多个候选且都非默认时，**不要盲取第一个**，在回复里列出让用户确认
3. **绝不使用测试组**（名字纯数字 `"123"`、含 `test`/`demo`/`临时`/`tmp` 的组）

---

## 方式 A：导入集成模板（首选）

`import_dashboard_template` 会读取 `integrations/` 下的完整模板（布局、阈值、单位、overrides、value mappings 全部保留），并自动把模板的数据源绑定改写到你选的 Prometheus 数据源上。

### 步骤

1. 看有哪些集成组件：
   ```
   list_files(base="integrations")
   ```
2. 看该组件下有哪些模板文件：
   ```
   list_files(base="integrations/Linux", path="dashboards")
   ```
3. **挑对模板文件**。模板常分 `categraf` 和 `exporter` 两种采集风格：
   - 用 `list_metrics` 探测环境里实际存在的指标，决定用哪种风格
   - **不要**用 `read_file` 把整个模板读出来再拼——模板可能很大且会被截断；挑文件靠文件名 + README/metrics 即可
4. 导入：
   ```
   import_dashboard_template(group_id=<业务组>, component="Linux", file="categraf-overview.json", datasource_id=<Prometheus数据源>)
   ```

### 注意事项

- 数据源绑定、变量、布局全部自动处理，你只需传 `component` + `file`
- `datasource_id` 可选：传了就把它设为大盘数据源变量的默认选中值（首屏即可查询）；不传则由前端在数据源下拉里自动选第一个 Prometheus
- 想改名或改标签：传可选的 `name` / `tags`，不传则沿用模板自带的
- 返回 `Name duplicate` 时，**不要**调 `list_dashboards`，直接换个名字（加 `-v2`、`-AI` 或时间戳后缀）重试

---

## 方式 B：自定义创建（无模板时）

用 `create_dashboard`。**只需提供面板标题、类型和 PromQL**，工具自动生成完整配置（布局、数据源变量、样式、单位等全部自动处理）。

> create_dashboard 接受的是下面这套**简化字段**。除此之外的字段（thresholds、overrides、value mappings、heatmap/hexbin/tableNG/iframe 等）它**不支持**，写了也会被忽略——需要这些丰富配置时请改用方式 A 导入模板。

### 调用示例

```json
{
  "group_id": 1,
  "name": "Linux 主机监控",
  "datasource_id": 1,
  "tags": "linux 主机",
  "variables": "[{\"name\":\"ident\",\"label\":\"主机\",\"definition\":\"label_values(cpu_usage_idle, ident)\"}]",
  "panels": "[{\"name\":\"CPU使用率\",\"type\":\"stat\",\"queries\":[{\"promql\":\"avg(cpu_usage_active{cpu=\\\"cpu-total\\\",ident=~\\\"$ident\\\"})\",\"legend\":\"CPU\"}],\"unit\":\"percent\"},{\"name\":\"CPU使用率趋势\",\"type\":\"timeseries\",\"queries\":[{\"promql\":\"cpu_usage_active{cpu=\\\"cpu-total\\\",ident=~\\\"$ident\\\"}\",\"legend\":\"{{ident}}\"}],\"unit\":\"percent\"}]"
}
```

### 面板字段（仅这些被支持）

每个面板必填 3 个字段：

```json
{"name": "面板标题", "type": "timeseries", "queries": [{"promql": "PromQL表达式", "legend": "{{ident}}"}]}
```

| 字段 | 说明 | 默认 |
|------|------|------|
| `name` | 面板标题（必填） | — |
| `type` | 面板类型（必填，见下表） | — |
| `queries` | 查询列表，每项 `{promql, legend?, instant?}` | — |
| `unit` | 单位 | 无 |
| `w` / `h` | 宽/高（网格列数，总宽24） | 按类型自动 |
| `stack` | 是否堆叠（仅 timeseries） | false |
| `description` | 面板描述 | 无 |

查询字段 `instant`：stat/gauge/barGauge/pie/table 等单值面板建议设 `"instant": true`（即时查询）。

### 支持的面板类型（仅这 8 种）

| type | 说明 | 默认尺寸 w×h |
|------|------|--------------|
| `timeseries` | 时序折线图，最常用 | 12×8 |
| `stat` | 单值统计大数字 | 6×4 |
| `gauge` | 仪表盘 | 6×6 |
| `barGauge` | 水平条形排名 | 8×8 |
| `pie` | 饼图 | 6×6 |
| `table` | 表格 | 12×10 |
| `text` | 文本说明（用 description 作内容） | 6×4 |
| `row` | 分组行（自动全宽） | 24×1 |

### 常用单位(unit)

`percent` | `bytesIEC` | `bitsIEC` | `bytesSecIEC` | `bitsSecIEC` | `seconds` | `milliseconds` | `reqps`

### 布局自动计算

面板从左到右、从上到下自动排列，同类型自动对齐（如 4 个 stat 排成一行），`row` 独占一行作为分组标题。无需手动指定坐标。

### 变量

```json
{"name": "ident", "label": "主机", "definition": "label_values(cpu_usage_idle, ident)"}
```

`label` 和 `multi` 可选（multi 默认 true）。后续变量在 definition 里引用前置变量实现级联：

```json
[
  {"name": "ident", "definition": "label_values(cpu_usage_idle, ident)"},
  {"name": "interface", "definition": "label_values(net_bytes_recv{ident=~\"$ident\"}, interface)"}
]
```

### 注意事项

- 多选变量在 PromQL 里用 `=~` 不用 `=`，如 `ident=~"$ident"`
- counter 类型指标用 `rate(...[3m])` 或 `irate(...[5m])`
- **PromQL 优先从 integrations 模板里抄经过验证的表达式**：`read_file(base="integrations/Linux", path="dashboards/categraf-detail.json")`，关注 targets 里的 `expr`
- 返回 `Name duplicate` 时直接换名重试，不要调 `list_dashboards`

---

## 第三步（两条路径通用）：输出结果

**保持简短**。一句话确认即可，例如：

> ✅ 已为您创建仪表盘「Linux 主机监控」，详情请查看下方卡片。

**不要**复述仪表盘 ID、业务组、数据源、面板清单等字段——前端会以结构化卡片展示。需要补充建议可加一两句，但不要逐条列出卡片已有的字段。

## 无模板时的推荐面板设计（方式 B 参考）

> 这些主题在 integrations 里基本都有模板，优先走方式 A；下面仅供方式 B 手搓时参考。

### Linux 主机监控

**变量：** `ident`（主机）、`interface`（网卡）、`mountpoint`（挂载点）

| 区域 | 面板 | 类型 | 核心指标 |
|------|------|------|----------|
| 概览 | CPU使用率 | stat | `avg(cpu_usage_active{cpu="cpu-total"})` |
| 概览 | 内存使用率 | stat | `avg(mem_used_percent)` |
| 概览 | 磁盘使用率(最高) | stat | `max(disk_used_percent)` |
| CPU | CPU使用率趋势 | timeseries | `cpu_usage_active` + `cpu_usage_iowait` |
| CPU | 系统负载 | timeseries | `system_load1/5/15` |
| 内存 | 内存使用率趋势 | timeseries | `mem_used_percent` |
| 磁盘 | 各挂载点使用率 | barGauge | `disk_used_percent` |
| 网络 | 网络流量 | timeseries | `rate(net_bytes_recv/sent)` |

### MySQL 监控
QPS/TPS、连接数、慢查询、Buffer Pool 命中率、复制延迟

### Redis 监控
OPS、内存使用、连接数、命中率、键空间

### Kubernetes 监控
**变量：** `cluster`、`namespace`、`pod`。Pod CPU/内存、节点资源、部署状态、PV 使用
