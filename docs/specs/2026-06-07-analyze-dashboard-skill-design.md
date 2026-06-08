# 分析仪表盘 Skill 设计(n9e-analyze-dashboard)

日期:2026-06-07 | 状态:已批准(分节评审通过,用户选择直接实现)

## 1. 背景与目标

用户诉求:"分析 etcd 仪表盘 24 小时内数据有哪些问题"——对一个已有仪表盘做时间窗内的健康分析,输出异常清单与建议。

### 调研结论(摘要)

**fc-model-server**(灭火图分析的 CheckDashboard 检查项)已有同类实现:取大盘配置→解析变量→逐 panel 批量查询→压缩成文本(30 点/曲线、平直线剔除)→嵌套调 LLM 判断异常。但其"原始点喂 LLM 判异常"的模式被业界与学术界双重否定:

- MIT SigLLM([arxiv 2405.14755](https://arxiv.org/abs/2405.14755)):LLM 直接看原始点找异常 F1 仅 0.13~0.22,远低于 ARIMA(0.582);
- Datadog Bits AI SRE 工程博客:早期"堆工具调用让 LLM 汇总原始遥测"的版本被官方判定失败放弃(token 线性膨胀+噪声带偏);
- Grafana(Sift: DBSCAN+MAD)/Datadog(Watchdog)/Dynatrace(Davis)/New Relic(Holt-Winters)全部采用"**确定性统计算法做检测,LLM 做编排归因解释**"架构;
- Grafana panel 查询事实标准 ~100 点/曲线(interval=duration/100);下采样反而提升 LLM 表现([arxiv 2410.05440](https://arxiv.org/html/2410.05440v3))。

**设计原则:统计预筛在服务端,LLM 只看预筛结果做归因与下钻。**

## 2. 架构

新增 builtin 工具 `get_dashboard_data`(取数+统计预筛)+ skill `n9e-analyze-dashboard`(工作流编排)。

```
用户 → load_skill → 定位大盘 → get_dashboard_data(id, time_range, vars?, panel_ids?)
  服务端:① 解析变量 ② 过滤 panel ③ 双窗 query_range ④ 统计特征+四种检测 ⑤ 分层渲染
模型 → 跨 panel 关联归因 → (可选)query_prometheus 缩窗下钻 → 输出报告
```

## 3. 工具定义

名称:`get_dashboard_data`(只读;权限对齐 get_dashboard_detail:大盘读 + 业务组读)

| 参数 | 必填 | 说明 |
|---|---|---|
| `id` | 是 | 仪表盘 ID |
| `time_range` | 否 | 默认 `1h`,语法同 query_prometheus(`24h`/`7d`...) |
| `vars` | 否 | JSON `{"ident":["host1"]}`,前端页面上下文注入或模型按用户意图传 |
| `panel_ids` | 否 | JSON 数组,只分析指定面板(超大盘分批/聚焦) |

### 3.1 变量解析(简化版,按用户决策)

1. `vars` 参数里有 → 直接用(多值拼 `(v1|v2)` 正则);
2. 变量有 defaultValue → 用默认值;
3. query 变量无默认值 → 用 prom `LabelValues`(解析 `label_values(expr, label)` 定义)取全量拼正则;取不到 → `.*` 兜底并在输出注明;
4. datasource 变量 → vars 指定 > 大盘面板 datasourceValue 解析 > 第一个 Prometheus 数据源;解析不出报错。
5. 替换语法:`$var` / `${var}` / `[[var]]`。

### 3.2 panel 过滤

- 查:timeseries / barGauge / barchart / pie / hexbin / gauge / heatmap / **stat**(与 fc 不同:stat 也查,24h 巡检场景下 stat 指标的趋势有价值);
- 跳过:row(只当分组名)/ table / text / iframe,折叠 row 内嵌套 panel 拉平;
- 仅 datasourceCate=prometheus(v1 决策),其他 cate 计数注明。

### 3.3 查询策略(双窗)

- 当前窗口 `[now-range, now]`,step = `max(60, range/300)`(≤300 点/曲线,仅服务端使用,不进上下文);
- 同比窗口:平移 `max(24h, range)`(≤24h → 昨日同时段;7d → 环比上周);每 panel 两次 range query;
- 单 panel 查询超时 10s,失败跳过并在输出列明(不静默)。

### 3.4 统计特征与四种检测(每条曲线)

特征:avg / min / max(带时刻) / last。检测:

| 检测 | 算法 | 标记示例 |
|---|---|---|
| MAD 离群 | 全窗中位数 ± k×MAD(k=3) | `离群 96@14:32` |
| 突变 | 相邻点差 > k×相邻差MAD | `突变+312%@14:30` |
| 趋势 | 首/末 1/4 窗口均值变化率(阈值 ±30%) | `趋势↑156%/24h` |
| 同比昨日 | 双窗 avg/max 对比(阈值 ±50%);周期性降噪:今日尖峰在昨日同时段(±1 step)有幅度相近尖峰 → 标"疑似周期性"降级 | `同比avg+45%` / `14:00尖峰昨日同现 疑似周期性` |

可疑分 = 命中检测项数 × 幅度,用于排序与截断。平直线(全窗等值)只计数。
曲线匹配:双窗序列按 labelset 精确匹配;昨日有今日无 → 计数提示;昨日无数据 → 标注并跳过同比。

### 3.5 分层输出(markdown 文本)

```
## 仪表盘: etcd (id=5) | 窗口: 24h | 87条曲线: 12可疑 / 68正常 / 7平直已略
变量: cluster=prod-etcd (默认值) | 跳过: 2个table面板

### ⚠ 可疑曲线 (12)        ← 特征行 + ~30 个降采样点(复核归因用)
[磁盘/WAL fsync延迟] etcd-2: avg=12ms(昨日4ms,+200%) max=89ms@14:32 趋势↑156% ⚠
  点: 00:00=4.1 ... 14:32=89.1 ...
### ✓ 正常曲线摘要 (68)     ← 仅一行特征摘要
[QPS] etcd-1: avg=1.2k(+2%) 平稳 | ...
### 略过                    ← 平直线计数、失败面板清单、昨日有今日无
```

### 3.6 规模兜底(三层)

1. 可疑曲线上限 30 条(按可疑分取 top,截断注明,提示 panel_ids 聚焦);
2. 正常摘要上限 100 行,超出折叠为按 panel 计数;
3. 整体输出 32KB 硬上限(触发即截断+提示分批)。

## 4. Skill 工作流(SKILL.md 骨架)

1. 定位大盘(页面 dashboard_id / 链接 / list_dashboards 按名搜,多候选问用户);
2. 调 get_dashboard_data(时间范围按用户意图,缺省 1h 并注明;条件经 vars 传);
3. 职责约定:工具已预筛,模型做跨 panel 关联归因、指标语义与业务影响判断、"疑似周期性"复核、query_prometheus 缩窗下钻;
4. 报告格式:总体结论 → 异常清单(按严重度,带时刻+证据) → 关联分析 → 建议;
5. 边界:无可疑就明说;截断用 panel_ids 分批;非 prom 面板注明覆盖范围;
6. 交叉引用:改大盘 → n9e-modify-dashboard;建告警 → n9e-create-alert-rule。

## 5. 错误处理

| 情况 | 处理 |
|---|---|
| 大盘不存在/无 payload | 报错 |
| 整盘无可分析面板 | 报错并列 cate 分布 |
| datasource 变量解析不出 | 报错 |
| query 变量取不到值 | `.*` 兜底+输出注明 |
| 单 panel 查询失败 | 跳过+输出列明原因 |
| 昨日窗口查询失败 | 同比跳过标注,其余检测照常 |
| 空数据面板 | 计数列出 |

原则:所有降级在输出里明示,不静默吞(吸取 update_dashboard 谎报教训)。

## 6. 测试

- 统计检测纯函数单测(核心):突变/趋势/平直/离群/周期性双窗序列;
- 变量解析单测:vars 注入/默认值/label_values 兜底(mock prom.API,GetPromClient 为 ToolDeps 函数字段,可注入);
- 输出格式单测:分层渲染、三层截断;
- handler 集成测试:沿用 dashboard_test.go 模式(sqlite + board payload + mock prom);
- E2E:本机 17000 环境真实大盘验证。

## 7. 实现文件清单

| 文件 | 内容 |
|---|---|
| `aiagent/tools/defs/defs.go` | GetDashboardData 工具定义 |
| `aiagent/tools/dashboard_analyze.go` | handler:变量解析/查询编排/输出渲染 |
| `aiagent/tools/dashboard_analyze_stats.go` | 统计特征与检测(纯函数) |
| `aiagent/tools/dashboard_analyze_*_test.go` | 单测+集成测试 |
| `aiagent/skill/embedded/builtin/n9e-analyze-dashboard/SKILL.md` | skill |

复用:summarizeConfigs/varRefRe/stringVal(dashboard_update.go)、parseTimeRange/autoStep(datasource_query.go)、newDashboardTestDeps(dashboard_test.go)。
