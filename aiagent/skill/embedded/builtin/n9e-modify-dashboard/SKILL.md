---
name: n9e-modify-dashboard
description: 修改夜莺(n9e)上已存在的监控仪表盘。当用户要求改某个仪表盘的变量、检查并修复变量、修改图表/曲线（改 PromQL/SQL、legend、单位、增删曲线）、重命名图表时使用。区别于"从零创建仪表盘"(那是 n9e-create-dashboard)。
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
| **改图表曲线** | 某个图表(panel)的曲线：查询表达式(PromQL/SQL)、legend、单位(unit)、增删曲线、改标题 |

## 铁律：生成提案 → 列改动表格 → 等用户确认 → 才写回

`update_dashboard` 是**两阶段写入**：第一次调用（不带 `confirmed`）只生成提案、**不写库**，返回 `proposal_id` 和 `applied:false`；只有第二次带上 `proposal_id` + `confirmed:true` 才真正落库。**绝对不要**在同一轮里既生成提案又确认。流程：

1. 先 `get_dashboard_detail(id, include_config=true)` 读到当前的变量、图表摘要和变量健康检查。
2. 算出要改的部分后，调一次 `update_dashboard`，**只传**要改的 `variables`/`panels`/`fix_datasource`，**不要带 `confirmed`**。这次不写库，会返回 `proposal_id`、`applied:false` 和 `changes`。
3. 把**拟改动项**用 Markdown 表格列出来，一行一项，**「改动前 → 改动后」**两列对照，用一句话问用户「确认按以上改动写回吗？」然后**停下，结束本轮**，等用户回复。本轮**不要**带 `confirmed:true`。
4. 只有当用户在后续消息里明确确认（「确认 / 对 / 改吧 / 可以」）后，再调一次 `update_dashboard`，**只传** `id` + 第 2 步拿到的 `proposal_id` + `confirmed:true`（无需重复传 variables/panels），才真正落库。
5. 用户回复「不对 / 取消 / 先别改 / 再改下」时，**不要**带 `proposal_id`/`confirmed` 去确认；旧提案直接作废，按反馈调整后重新走第 2 步生成新提案。

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

## 第三步：列改动表格

用「改动前 → 改动后」表格。示例：

**改变量：**

| 项 | 改动前 | 改动后 |
|----|--------|--------|
| 变量 `ident` 默认值 | (空) | `web01` |
| 变量 `ident` 是否多选 | 是 | 否 |

**改图表曲线：**

| 图表 | 项 | 改动前 | 改动后 |
|------|----|--------|--------|
| CPU使用率 (panel-2) | 曲线A PromQL | `cpu_usage_active{ident=~"$ident"}` | `avg(cpu_usage_active{cpu="cpu-total",ident=~"$ident"})` |
| CPU使用率 (panel-2) | 单位 | (无) | percent |

**检查并修复变量：** 把 `variable_lint` 命中项逐条列出，并给出每条的修复动作（改 definition / 重命名引用 / 修数据源引用）。

列完表格 → 问确认 → 停。

## 第四步：生成提案（调 update_dashboard，不带 confirmed）

第一次调 `update_dashboard`，**只传**要改的部分（其余配置原样保留，工具不会动）、**不带 `confirmed`**。这次只生成提案、不写库，返回 `proposal_id` 和 `applied:false`：

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

### 校验查询能不能取到值（可选但推荐）

改 PromQL 或修变量 definition 时，可先用 `query_prometheus` / `list_metrics` / `get_metric_labels` 验证新表达式确实有数据，再列进改动表格，避免改完还是空。

## 第五步：用户确认后落库（带 proposal_id + confirmed）

用户在**后续轮次**明确确认后，再调一次 `update_dashboard`，**只传** `id` + 第四步返回的 `proposal_id` + `confirmed:true`（不必重传 variables/panels）。这次才真正写库，返回 `applied:true`。

```json
{"id": 7, "proposal_id": "dbprop_...", "confirmed": true}
```

- 提案是**一次性**的，且必须在生成它之后的**新一轮**才能确认（同一轮里不能既生成又确认）。
- 若提示提案已过期/找不到、或仪表盘在此期间被改过（提案已失效），重新走第四步生成新提案、再列表格确认。

## 第六步：输出结果

写回成功后保持简短，一句话即可：

> ✅ 已更新仪表盘「<名字>」：<改了什么>。刷新页面即可看到生效。

工具返回的 `changes` 列表是实际落库的改动项，可据此向用户复述。

## 注意事项

- 多选变量在 PromQL 里用 `=~` 不用 `=`，如 `ident=~"$ident"`。
- 改/加曲线时 legend 模板用 `{{label}}` 形式，如 `{{ident}}`。
- 不确定指标名/标签时先 `list_metrics` / `get_metric_labels` 探一下，别凭空编。
- 一次对话里用户连续提多个改动，可以合并成一次 `update_dashboard` 调用（`variables` 和 `panels` 一起传），但表格确认这一步不能省。
