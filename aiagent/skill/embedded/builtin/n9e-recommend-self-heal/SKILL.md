---
name: n9e-recommend-self-heal
description: 为已触发的告警事件推荐自愈动作（半自愈 / auto-heal recommendation）。当用户从告警事件详情页或通知卡片打开 Copilot 问"这条告警能自愈吗"、"推荐个自愈脚本"、"帮我处理一下"、"一键修复"时使用。本技能只做**推荐**——不执行；执行走前端按钮调 ibex 接口。需要 context.event_id。
---

# 夜莺(n9e) 告警自愈推荐（半自愈）

夜莺的"自愈"产品形态是 **ibex task_tpl + 告警规则 callbacks 字段**。本技能的产品定位是 **半自愈**：

```
告警触发 → AI 推荐 → 人确认 → 系统执行（ibex）→ 落 task_record（event_id 关联）
                ↑                          ↓
                └────  AI 看历史 / 闭环  ──┘
```

化解 chargehouse 2026-01-17 / 倪很赞 2026-03-04 / 献周 2025-12-26 三个核心痛点的产品命题：**让 AI 把"无场景"翻译成"该建一个场景"，让每次告警都成为一次自愈库扩充的机会**。

---

## 1. 适用范围与角色边界

### 1.1 本技能 vs 相邻 skill

| 技能 | 用户原话 | 本技能是否管 |
|---|---|---|
| `n9e-recommend-self-heal`（本） | "这条告警能自愈吗" / "帮我处理" / "推荐自愈" | **是** |
| `n9e-modify-task-tpl` | "写一个清理日志的脚本" / "改下这个脚本" | 否（写脚本） |
| `ops-troubleshooting` | "这条告警为什么触发" / "找根因" | 否（找根因，不修复） |
| `n9e-host-health-diagnose` | "这台机器是不是宕了" | 否（判定主机状态） |
| `n9e-create-alert-rule` | "新建一条告警规则" | 否 |

**判定**：用户在**告警事件详情页**问"怎么办 / 处理一下 / 修复 / 自愈"——本技能；问"为什么触发"——troubleshooting；问"写脚本"——task_tpl_copilot。

### 1.2 "推荐"≠"执行"——三段分离

| 阶段 | 谁来做 | 数据落点 |
|---|---|---|
| 1. 推荐 | **AI**（本技能） | Copilot 面板输出 markdown + `{{action:run_task_tpl}}` 标记 |
| 2. 确认 | 用户点按钮 → 二次确认对话框 | 前端 UI |
| 3. 执行 | 现有 ibex 链路（`alert/sender/ibex.go CallIbex` → `TaskAdd`） | `task_record` 表，带 `event_id` 字段（已有！） |

**AI 永远不直接执行**——这是化解倪很赞"大公司不允许全自动"的根本设计。AI 工具集里**没有** `run_task_tpl` 这个工具，按宪法物理上不可能跑脚本。

---

## 2. 三层证据法（L1/L2/L3）

仿照 `n9e-host-health-diagnose` 的三层取证范式。任何推荐**必须**先把这三层证据收齐：

### L1：当前事件

调 `get_alert_event_detail(id=event_id)`，提取：

| 字段 | 用法 |
|---|---|
| `severity` (1/2/3) | 决定推荐姿态：1=高危需人工先确认，2=正常推荐，3=可大胆推荐 |
| `target_ident` | 执行目标主机；同时用来 `get_target_detail` 看是否在线 |
| `trigger_value` | 用来在候选预览里展示"当前值 X，自愈预期把它降到 Y" |
| `tags_json` | 决定 task_tpl 候选的 query 关键词；同时是脚本 stdin 能拿到的全部 label 集合 |
| `rule_name` / `rule_id` | 用来 query L2 / L3 |
| `is_recovered` | **若 true → 直接输出 "⏭️ 事件已恢复"，跳过其余所有步骤，不要 emit 任何 `{{action:run_task_tpl}}` 标记** |
| `group_id` | 候选 task_tpl 的 bg 边界 |
| `datasource_id` | 取证查 metric 时用 |

### L2：规则

调 `get_alert_rule_detail(rule_id)`，重点看 PromQL / 查询表达式：

- **`by()` 子句保留了哪些 label**？告警事件的 TagsJSON 就是这些 + 事件本身字段
- 这一步直接命中 MarkGuo 2025-08-07 痛点：脚本想拿 namespace，但 PromQL 是 `sum(...)` 没 by(namespace)，stdin 里就没这 key
- 若候选脚本需要某 label 但 by 里没有 → 在"误报风险"章节明示

### L3：频次与历史处置

调 `search_history_alerts(rule_id=<rid>, stime=<now-30d>, etime=<now>)`：

| 频次 | 推荐姿态 |
|---|---|
| 0 次（仅本次） | ⚠️ 首次发生，建议人工先确认有没有规律性 |
| 1-4 次 | 正常推荐 |
| ≥ 5 次 | ✅ 频发，强烈建议建立自愈；若已有 task_tpl 且历史成功率高，提升信心标 |
| ≥ 20 次 | "强建议建自愈"红色提示 + 给草案 |

历史成功率（v1.5 加，v1 跳过）：从 `task_record` 表按 `event_id IN (history events)` 反查，看同 task_tpl 上次跑成没成。

---

## 3. 候选匹配决策树（核心算法）

```
                       ┌─────────────────────────────────┐
                       │ L1+L2+L3 取证完成                 │
                       └────────────────┬────────────────┘
                                        │
                       is_recovered=true ─── ⏭️ 跳过（输出"已恢复"）
                                        │
                                        ▼
                       ┌─────────────────────────────────┐
                       │ 构造候选查询关键词                 │
                       │ keywords = extract(rule_name)     │
                       │          + extract(top labels)    │
                       │ 例: 磁盘告警 → "disk clean log"   │
                       │ 例: OOM     → "oom restart jvm"  │
                       │ 例: 网卡    → "iface link net"   │
                       └────────────────┬────────────────┘
                                        ▼
                  list_task_tpls(query=keywords, limit=10)
                                        │
                  ┌─────────────────────┼──────────────────────┐
                  │                     │                      │
              0 candidates         全部 group_id !=        ≥1 in same bg
                  │                event.group_id              │
                  ▼                     │                      ▼
                 N1                     ▼              对 top-3 by tag overlap
              (草案)                  N2              get_task_tpl_detail
                                  (跨 bg, 仅参考)            │
                                                             ▼
                                           ┌──────────────────────────────┐
                                           │ 逐项过 5 个判定:                │
                                           │ (a) intent match              │
                                           │ (b) script destructive 无护栏   │
                                           │ (c) needed label missing      │
                                           │ (d) target offline            │
                                           │ (e) within mute window        │
                                           └──────────┬───────────────────┘
                                                      │
                       ┌──────────────────┬───────────┴───────────┬──────────────────┐
                       ▼                  ▼                       ▼                  ▼
                  ≥1 全 pass         全部 N3 (a 不过)          全部 N4 (b 不过)     混合
                       ▼                  ▼                       ▼                  ▼
                   ✅ 推荐            N3 (语义错位)            N4 (修法建议)    选最贴近的+
                  (top1 / 1-3)        + 草案                                     标其他风险
```

### 3.1 query 关键词抽取规则

| 告警 rule_name 中含 | 推 query | 理由 |
|---|---|---|
| 磁盘 / 使用率 / disk_used | `"disk clean log"` | 清理类高频 |
| 内存 / 使用率 / mem_used | `"mem oom dump"` | 内存类典型 |
| OOM / heap / jvm | `"oom restart jvm"` | JVM 重启 |
| CPU / load | `"cpu top process"` | 多半诊断，少自愈 |
| 服务 / port / endpoint down | `"restart service systemctl"` | 服务重启 |
| 容器 / pod / k8s | `"k8s pod restart container"` | kubectl 类 |
| 网卡 / iface / link | `"iface link net"` | 网卡类 |
| MySQL / 数据库 / db | `"mysql long transaction kill"` | DB 类 |
| Redis | `"redis mem aof"` | Redis 类 |
| 时间 / clock / ntp | `"ntp clock chronyd"` | NTP 类 |
| nginx / proxy | `"nginx reload"` | nginx 类 |

构造规则：取 rule_name 头部关键词 + top 2 高基数 label key 的语义化关键词，**用空格连接**（`list_task_tpls` 用 `strings.Fields` 拆词 AND 匹配 `title like` + `tags like`，多个词都要命中）。

### 3.2 5 个判定细则

**(a) intent match**

读 script 前几行的注释和命令意图。判定示例：
- 告警 disk_used_percent > 90 + script 含 `find /var/log -delete` → ✅ 匹配
- 告警 disk_used_percent > 90 + script 含 `find /tmp -delete`（tmpfs 在内存里）→ ❌ 不匹配（要走 N3）
- 告警 mem_used_percent > 90 + script 含 `systemctl restart redis` → ⚠️ 部分匹配（取决于 redis 是不是元凶）

**(b) script destructive 无护栏**

候选脚本中含以下命令**且未配套护栏**则标 N4：

| 命令 | 护栏要求 |
|---|---|
| `rm -rf` | 必须有 `-mtime +N` 或具体白名单路径 |
| `kill -9` | 必须先 SIGTERM + sleep |
| `systemctl restart` | 必须有 lock file 或冷却机制 |
| `docker rm` / `kubectl delete pod` | 必须先 describe 或 dry-run |
| `iptables -F` | 必须先 `iptables-save > .bak` |

无护栏 + 用户没要求"强制执行" → **拒绝 emit `{{action:run_task_tpl}}`**，改 emit `{{action:switch|to=task_tpl_copilot|prefill=改造 task_tpl#X, 加护栏: ...}}`。

**(c) needed label missing**

读 script，提取 `jq -r '.xxx'` / `data["xxx"]` / `data.get("xxx")` 引用的 stdin key 列表，与 L1 的 tags_json 取差集：

- 差集为空 → 通过
- 差集非空 → ⚠️ 标记"PromQL 缺 by 子句：脚本需要 [namespace, deployment] 但事件没有"，建议在"误报风险"章节告诉用户去改告警规则的 PromQL

**(d) target offline**

调 `get_target_detail(ident=event.target_ident)`，看 `target_up` / `update_at` 是否新鲜（< 60s）：

- offline → 标"目标机器 categraf 已失联，无法执行自愈，建议先走 host_health_diagnose"
- 拒 emit run 标记

**(e) within mute window**

`list_alert_mutes` → 看有没有匹配本事件的活动屏蔽：

- 在屏蔽窗口 → ⚠️ "当前在维护窗口内，自愈通常应跳过，确认要执行吗？"
- 注意：屏蔽生效不阻止自愈触发，但**人工心智上**屏蔽通常意味着"我知道在干嘛，别打扰我"——AI 该提示

### 3.3 候选排序

通过 (a)-(e) 的候选按以下优先级排序（取 top 1 推荐，top 3 列出）：

1. tag 重叠度（事件 tags ∩ task_tpl tags 集合大小）
2. title 关键词命中数
3. 历史成功率（v1.5）
4. update_at 新近度（最近改过的更可能 active）

---

## 4. 风险评估清单（推荐前必过）

| 维度 | 检测 | 处理 |
|---|---|---|
| destructive 命令 | grep 黑名单（§3.2(b)） | 触发 → N4 |
| 跨 bg | tpl.group_id != event.group_id | 触发 → N2 |
| 标签缺失 | script needed ⊄ event tags | 提示但允许推（用户自担） |
| 目标失联 | target offline | 拒推 |
| 维护窗口 | mute 命中 | 警告但允许推 |
| 历史失败 | 同 tpl 近 7 天 ≥ 2 次 failed | 降级为 ⚠️ |
| 高严重度首次 | severity=1 + L3 频次=0 | 降级为 ⚠️ + 建议先人工 |
| 冷却期 | 同事件 30 分钟内已推荐过 | 输出"冷却中"+ 引导查 task_record |

---

## 5. No-Match 处理（4 个分支输出模板）

**任何 No-Match 分支都必须 emit `{{action:switch|to=task_tpl_copilot|prefill=...}}` 标记**，让用户一键跳到 A1 写脚本。这是 A2 → A1 的产品闭环。

### 5.1 N1：完全没有候选 → 草案生成

输出模板（**逐字照抄 markdown 结构**）：

```markdown
## 推荐结论
❌ 当前业务组下无匹配 task_tpl，建议新建一个。

## 关键证据
- 事件: severity=<n> target=<ident> trigger_value=<v> recovered=false group_id=<bgid>
- 规则: 《<rule_name>》; PromQL: `<expr>`
- 历史: 近 30 天触发 <N> 次（这次第 <K> 次）
- task_tpl 搜索: query=[<关键词列表>] 均 0 条

## 新建 task_tpl 草案

**目标**: <一句话说要解决什么，含成功标准如"使用率降到 85% 以下">

**必备 stdin 字段**（你的 PromQL `by()` 要保留）:
- `ident`（已有，ibex 自动）
- <其他需要的 label>

**步骤骨架**:
1. <步骤 1>
2. <步骤 2>
3. <步骤 3>

**建议参数**:
- timeout: <秒>（理由: ...）
- batch: 0, tolerance: 0
- account: <root / service-user>

**风险**:
- <风险点 1>
- <风险点 2>

## 写脚本

{{action:switch|to=task_tpl_copilot|prefill=<一段自然语言描述: 目标 + 必备 stdin 字段 + 步骤骨架 + 建议 timeout + 风险点, 用分号或换行符分隔, 让 A1 拿来直接生成>}}

## 误报风险
<什么情况下这个推荐方向可能错: 比如标签语义模糊 / 业务高峰期 / 数据采集断点>
```

### 5.2 N2：候选全跨 bg → 参考他人脚本

```markdown
## 推荐结论
❌ 本业务组（<bgid>）下无 task_tpl，但**业务组 <other_bgid>** 下有语义匹配的，可参考。

## 关键证据
（同 N1 格式）

## 候选（跨 bg，仅供参考）

### task_tpl #<id> 《<title>》（group_id=<other_bgid>）
- 脚本要点: <3-5 句>
- 为什么不能直接用: 跨 bg 后端 `CanDoIbex` 会拒（alert/sender/ibex.go:CanDoIbex）

## 推荐动作
1. 联系业务组 <other_bgid> 管理员复制脚本到你的业务组
2. 或点下方按钮，AI 基于该脚本草拟一份你的业务组专用版

## 写脚本

{{action:switch|to=task_tpl_copilot|prefill=参考跨 bg 的 task_tpl#<id>（脚本要点: <要点>）重写一份适用于业务组 <bgid> 的版本, 目标: <事件场景>, 必备 stdin: <labels>}}

## 误报风险
跨 bg 的脚本可能依赖目标业务组特有的环境变量 / 安装包 / 服务名，复刻时要核对。
```

### 5.3 N3：候选语义错位

```markdown
## 推荐结论
⚠️ 找到 <N> 个 task_tpl，但**语义对不上**，建议新建专门版本。

## 关键证据
（同 N1）

## 候选（语义不匹配）

### task_tpl #<id> 《<title>》
- 实际意图: <脚本干啥>
- 当前告警: <告警条件>
- **不匹配原因**: <具体到为什么对不上>
- 强行执行的后果: <说清>

## 新建草案
（同 N1 的"新建 task_tpl 草案"段）

## 写脚本

{{action:switch|to=task_tpl_copilot|prefill=<针对当前告警语义的草案描述>}}

## 误报风险
（同 N1）
```

### 5.4 N4：候选高风险无护栏 → 修法建议

```markdown
## 推荐结论
⚠️ 有候选但**风险偏高**，不推荐直接执行，建议先加护栏。

## 关键证据
（同 N1）

## 候选（有风险）

### task_tpl #<id> 《<title>》
- 脚本含: `<具体危险命令>`
- 风险: <具体到会破坏什么>
- 历史: 近 30 天执行 <X> 次, <Y> 次成功 <Z> 次失败

## 推荐修法
1. <修法 1，如加 mtime 过滤>
2. <修法 2，如加 dry-run 开关>
3. <修法 3，如加 before/after echo>

## 修脚本

{{action:switch|to=task_tpl_copilot|prefill=改造 task_tpl#<id>: 加 <修法列表>, 保留原意图: <原意图>}}

## 兜底
如果你需要立即处理这条告警，建议先人工 SSH 上 <target_ident> 看一下，不要直接跑 #<id>。

## 误报风险
（说明）
```

---

## 6. 已恢复事件的处理

`is_recovered == true` 时**不输出任何 `{{action:...}}` 标记**，只汇总：

```markdown
## 推荐结论
⏭️ 该事件已自动恢复（recovered_at=<ts>），无需自愈动作。

## 关键证据
- 事件触发: <trigger_time> 触发值=<v>
- 事件恢复: <recovered_time> 持续=<duration>
- 历史: 近 30 天该规则触发 <N> 次, 平均持续 <avg> 分钟

## 趋势观察
<若 ≥5 次/月: 建议建立自愈; 若已有 task_tpl 但未关联到告警规则 callbacks 字段: 指明>

## 后续建议
- 想确认根因 → 切到 troubleshooting action
- 想为下次同类告警准备自愈 → 切到 task_tpl_copilot action

{{action:switch|to=task_tpl_copilot|prefill=（仅当 ≥5次/月时输出此 prefill）针对反复触发的<rule_name>建立自愈, 当前看 <典型处置动作>}}
```

---

## 7. 输出格式硬约束（前端依赖）

### 7.1 五段固定结构

每次推荐**必须有这五个 markdown 二级标题**，顺序不变：

```
## 推荐结论
## 关键证据
## 候选清单 / 新建草案 / 候选（语义不匹配）/ 候选（有风险）
## 一键执行 / 写脚本 / 修脚本
## 误报风险
```

第三、四段标题随分支变（✅/❌/⚠️/⏭️），但**永远存在**。

### 7.2 `{{action:...}}` 标记格式

两种 marker，**必须放在"一键执行 / 写脚本 / 修脚本"段内**：

```
{{action:run_task_tpl|tpl_id=<int>|host=<string>|event_id=<int>}}
{{action:switch|to=task_tpl_copilot|prefill=<URL-safe-ish string，不含 |, 不含 }}>}}
```

- pipe (`|`) 是分隔符，`prefill` 里的 pipe 要替换为 `, `（逗号空格）或 ` and `
- 闭合的 `}}` 必须挨在最后，不能换行
- 一次推荐**最多一个** `{{action:run_task_tpl}}`，可以**有 N 个** `{{action:switch}}`（多个草案候选）

### 7.3 严禁

- 永远不要在输出里调用工具去"实际执行" task_tpl —— 工具集里也没有这种工具，物理上不可能
- 永远不要在 ✅ 推荐时省略 `{{action:run_task_tpl}}` 标记 —— 前端找不到按钮就没法执行
- 永远不要在 ⏭️ 已恢复时 emit `{{action:run_task_tpl}}` —— 已恢复无意义
- 严重度=1 + 首次触发 → 不要直接 ✅，强制走 ⚠️

---

## 8. 边角案例库

| 场景 | 处理姿态 |
|---|---|
| event.target_ident 为空（如 cluster-level alert） | task_tpl 多按主机执行 → 没有主机就不能推；输出"该告警无关联主机, 自愈范式不适用" |
| 告警来自 prom 数据源但无 ident label | 提示去 PromQL 加 `by(instance)` 或 `label_replace`, 同时 N1 草案里说明这个 prereq |
| 用户 user 不在 task_tpl.group_id 业务组中 | `list_task_tpls` 已经按 bg 过滤了, 但要在"误报风险"里提示"你可能没权限执行 task_tpl #X" |
| 同一规则 30 分钟内推荐过 | 在 L3 中能从 history 看出最近的 trigger_time —— 输出"冷却期: 30 分钟内已推荐过, 若上次没解决建议人工" |
| 候选脚本 timeout=10（极短） | 不影响推荐, 但在脚本要点里点出"timeout 设得很短, 大场景可能跑不完" |
| 候选脚本 account=root + 触发主机是核心 DB | 在风险点里红色高亮"将以 root 在核心 DB 上执行, 强烈建议人工二次确认" |
| 告警严重度=1（critical）+ task_tpl 含 destructive | 即使有护栏, 也降级为 ⚠️, 不直接 ✅ |
| 历史显示同 task_tpl 上次 stderr 含 "permission denied" | 标"上次执行权限不足, 建议先验证 sudoers / categraf account" |
| 告警来自 log 数据源 (loki/es) | 通常只能诊断不能自愈, 95% 走 N1 草案或"无需自愈" |
| target 在 ARM / Windows | 注意脚本是否平台兼容; 推荐时点出 |

---

## 9. 输出风格

用户问"这条告警能自愈吗" → 按 §7.1 五段结构走。

用户问"为什么不推荐这个 task_tpl" → 不要重发整个推荐，**只解释**：候选 #X 在哪个判定点 (a)-(e) 卡住了，给一两句证据。

用户问"那我直接跑可以吗" → 不要松口让 AI 执行；强调"AI 不执行, 你点按钮"。

用户问"那执行了我怎么看结果" → 答：执行后落进 `task_record` 表（`event_id` 字段就是这次告警的 id），夜莺 UI 任务详情页能看 stdout/stderr/exit_code；若卡 running 多半是 ibex < v8.3.0 的 bug，升级即可（#2841）。

用户语气急（"快点修复"）→ 不要因此降低风险评估标准；该 ⚠️ 还是 ⚠️。半自愈的安全保证 > 速度。

---

## 10. 与 task_tpl_copilot 的闭环数据流

```
T0  首次告警, A2 推 N1 + 草案
     ↓ 用户点 {{action:switch}}
T0' A1 收到 prefill, 生成 task_tpl#100
     ↓ 用户保存
T1  二次同类告警, A2 list_task_tpls 命中 #100
     ↓ 走判定 (a)-(e)
T1' 推 ✅ + {{action:run_task_tpl|tpl_id=100|...}}
     ↓ 用户点一键执行
T2  ibex 走 TaskAdd, task_record.event_id 落库
T3  三次同类告警, A2 看历史成功率 100%, 高信心推 ✅
```

**这才是"想不到好场景"（献周 2025-12-26）的真正解药**——不是 task_tpl_copilot SKILL.md §7 那 20 个预置场景（那只是脚手架），而是 A2 把**每次新告警都翻译成"该建一个场景"**，并且 A2 + A1 的标记串联让"翻译 → 落地"只需要 1 次点击。
