---
name: n9e-modify-dashboard
description: 修改夜莺(n9e)上已存在的监控仪表盘。当用户要求改某个仪表盘的变量、检查并修复变量、修改图表/曲线（改 PromQL、legend、单位、增删曲线）、重命名图表时使用。区别于"从零创建仪表盘"(那是 n9e-create-dashboard)。
max_iterations: 16
examples:
  - "把这个仪表盘的 ident 变量默认值改成 web01"
  - "检查下这个大盘的变量有没有问题，顺手修一下"
  - "把 CPU 使用率这个图的查询改成只看总核"
  - "给内存图表加一条 swap 使用率的曲线"
  - "这个面板的单位改成百分比"
builtin_tools:
  - list_dashboards
  - get_dashboard_detail
  - update_dashboard
  - list_busi_groups
  - list_datasources
  - list_metrics
  - get_metric_labels
  - query_prometheus
---

# Skill: 夜莺(N9E) 修改已有仪表盘

帮用户用自然语言**修改一个已经存在的仪表盘**，不是新建。三类典型诉求：

| 诉求 | 你要改的东西 |
|------|-------------|
| **改变量** | 模板变量的取值表达式(definition)、默认值、是否多选(multi)、显示名(label) |
| **检查并修复变量** | 扫描变量定义和图表里对变量的引用，发现坏味道（图表引用了未定义变量、变量取不到值、数据源引用不一致等）并修复 |
| **改图表曲线** | 某个图表(panel)的曲线：查询表达式(PromQL)、legend、单位(unit)、增删曲线、改标题 |

> 曲线查询(`queries`)编辑**仅支持 Prometheus/VictoriaMetrics 面板**；SQL/日志类面板(mysql、ck、es 等)只能改单位、标题、说明或删除，传 `queries` 会被工具拒绝。

## 铁律：一次提案调用即收尾，确认由系统完成

`update_dashboard` 是**提案式写入**：你调用它（只传要改的部分）后，工具会计算改动、**直接向用户展示改动清单并暂停对话**；用户确认后由系统自动落库——确认环节不需要也不经过你。因此：

1. 先 `get_dashboard_detail(id, include_config=true)` 读到当前的变量、图表摘要和变量健康检查。
2. 算出要改的部分后，调一次 `update_dashboard`，**只传**要改的 `variables`/`panels`/`fix_datasource`。这一调用就是你本轮的最后一步，系统会接管展示与确认。
3. **不要**自己渲染改动表格（系统会展示工具生成的清单），**不要**传 `proposal_id`/`confirmed`（那是系统确认通道的参数）。
4. 用户拒绝或提出新要求时，你会在新一轮收到反馈：按反馈重新算改动、再调一次 `update_dashboard` 即可（旧提案自动作废）。

## 第一步：定位仪表盘

- 上下文里已带 `dashboard_id`（前端从 `/dashboards/<id>` 注入，或上轮已确定）→ 直接用，不要再 `list_dashboards`。
- 用户贴了 `/dashboards/<id>` 链接 → 从里面取 id。
- 只给了名字 → `list_dashboards(query="...")` 按名匹配；多个候选或匹配不到就列出来问用户，**不要**乱猜。

业务组、数据源都从仪表盘本身读，**不要**向用户索要。

## 第二步：读现状（务必带 include_config=true）

```
get_dashboard_detail(id=<id>, include_config=true)
```

返回里：
- `variables`：每个变量的 `name / type / label / definition / multi / default_value / datasource_value`
- `panels`：每个图表的 `id / name / type / unit / queries`（每条曲线含 `ref / promql / legend / instant / step / hide`，未设置的字段会省略）；折叠行(row)里的图表已被拉平进来
- `variable_lint`：变量健康检查命中的坏味道列表（如「图表 X 的查询表达式引用了未定义的变量 $foo」）

**定位图表优先用 `id`**（如 `panel-3`），名字可能重复。

## 第三步：校验查询能不能取到值（可选但推荐）

改 PromQL 或修变量 definition 时，可先用 `query_prometheus` / `list_metrics` / `get_metric_labels` 验证新表达式确实有数据再提案，避免改完还是空。

修复变量类任务：把 `variable_lint` 命中项逐条确定修复动作（改 definition / 重命名引用 / 修数据源引用），一并放进同一次提案。

## 第四步：提交提案（调 update_dashboard，一次即可）

调 `update_dashboard`，**只传**要改的部分（其余配置原样保留，工具不会动）。工具会向用户展示改动清单并等待确认，本轮到此为止：

- **改/增/删变量** → `variables`（JSON 数组，按 `name` 匹配）：
  ```json
  [{"name":"ident","default_value":"web01","multi":false}]
  ```
  - 只写要改的字段；`name` 不存在则新增一个 query 变量；`delete:true` 删除。
- **改图表曲线** → `panels`（JSON 数组，按 `id` 优先、否则 `name` 定位）：
  ```json
  [{"id":"panel-2","unit":"percent","queries":[{"promql":"avg(cpu_usage_active{cpu=\"cpu-total\",ident=~\"$ident\"})","legend":"{{ident}}"}]}]
  ```
  - `queries` 传入即与现有曲线**增量合并**：按 `ref`（原曲线 refId）匹配，只覆盖你写的字段，其余字段（step/hide/__mode__、按 refId 关联的 overrides 等）原样保留；**不带 `ref` 一律视为新增曲线（没有位置匹配）**。**未在 `queries` 里出现的现有曲线不会被删、原样保留**——所以只改某一条曲线时，传那一条（带上它的 `ref`）即可，不必把整图所有曲线都列出来。
  - **改已有曲线必须带上其 `ref`**（不带 `ref` 会新增曲线而不是改原曲线）；要删某条曲线，在该曲线项上带其 `ref` 并写 `delete:true`。
  - `new_name` 改标题，`unit` 改单位，`description` 改说明，`delete:true`（在面板项上）删整个图表。不传 `queries` 就不动曲线。
- **修复数据源引用** → `fix_datasource: true`：把图表/变量里悬空或写死的数据源引用统一重指到大盘数据源变量。适合修「图表查不到数据 / 数据源引用不一致」类坏味道。

## 第五步：特殊情况的答复

- 读完现状发现**无需改动**、查询校验**取不到数**、或**定位不到**目标图表/变量时：不调 `update_dashboard`，直接一句话说明原因并给出建议。
- 用户在新一轮反馈「提案过期/失效」（仪表盘在此期间被他人改过）时：重新读现状、重新提案即可。

工具返回的 `changes` 列表是实际落库的改动项，可据此向用户复述。

## 注意事项

- 多选变量在 PromQL 里用 `=~` 不用 `=`，如 `ident=~"$ident"`。
- 改/加曲线时 legend 模板用 `{{label}}` 形式，如 `{{ident}}`。
- 不确定指标名/标签时先 `list_metrics` / `get_metric_labels` 探一下，别凭空编。
- 一次对话里用户连续提多个改动，可以合并成一次 `update_dashboard` 调用（`variables` 和 `panels` 一起传）；改动清单与确认由系统把关，不会被跳过。
