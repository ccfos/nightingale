---
name: alert-mute-copilot
description: 夜莺(n9e)告警屏蔽规则(alert_mute)的创建、编辑、排障一站式助手。当用户要求"创建屏蔽规则 / 屏蔽告警 / 静默告警 / 维护窗口免打扰 / 周期性屏蔽 / 每天凌晨屏蔽 / 调整或延长屏蔽 / 屏蔽不生效排查"时使用。屏蔽作用于事件评估阶段（被屏蔽的事件不落库不通知）；要配"哪些事件通知给谁"用通知规则（→ notify-rule-copilot），要查"为什么没收到通知"走告警排障（→ alert-rule-troubleshoot）。
tags:
  - internal
---

# 夜莺(n9e) 告警屏蔽规则 Copilot

屏蔽（静默）规则用于在特定时间范围内按标签条件屏蔽匹配的告警事件，典型场景：机器维护窗口、每天凌晨批处理免打扰、已知问题降噪。

## 配套资料（按需用 read_file 加载，base 填本技能名）

| 文件 | 内容 | 何时读 |
|---|---|---|
| `reference.md` | config 字段全表、两种时间模式细节、tags 操作符、完整示例 | 拼 config 拿不准字段时 |
| `troubleshooting.md` | 屏蔽不生效排查链（按引擎匹配顺序）、行为语义、坑表 | 用户说"屏蔽了还在告警"时 |
| `http-api.md` | HTTP API 路径（A2A/外部 agent 用）、preview/tryrun 验证接口 | 仅外部 A2A 场景或给用户出 curl 指令时 |

---

## 前提

你是 n9e 站内 AI 助手，运行在 n9e 进程内、已以当前用户身份认证。**直接调用内置工具操作，不要登录、不要调 HTTP API、不要用 http_fetch 打自家接口**（HTTP 流程见 `http-api.md`，那是给外部 A2A agent 的）。

---

## 心智模型

- **屏蔽发生在事件评估阶段**（`alert/process/process.go`）：命中屏蔽的事件直接丢弃——**不产生活跃/历史告警记录、不发任何通知**。所以"屏蔽生效后历史告警页还能看到旧事件"是正常的：屏蔽不清理已产生的活跃告警，只拦新事件。已触发的告警在屏蔽期间也**不会被误判为恢复**（引擎专门保了这一点）。
- **匹配条件之间是 AND**（`alert/mute/mute.go:MatchMute`），按序检查：规则启用 → 数据源 → 时间（固定/周期） → 告警级别 → 标签。任何一关不过就不屏蔽。
- **屏蔽规则按业务组隔离**：只对本业务组（`group_id`）的事件生效。
- `prod` / `cate` / `cluster` 字段**只存储展示，不参与匹配**——别指望靠它们过滤。
- 改动后**最多 9 秒生效**（内存缓存轮询周期），无需重启。

## config 核心结构

```json
{
  "note": "屏蔽规则名称/标题",
  "cause": "屏蔽原因说明",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [1, 2, 3],
  "tags": [
    {"key": "ident", "func": "==", "value": "web01"}
  ],
  "mute_time_type": 0,
  "periodic_mutes": [],
  "cluster": "0"
}
```

要点（字段全表与时间模式细节见 `reference.md`）：

- **时间不用自己算 Unix 时间戳**：固定时段（`mute_time_type=0`）把时长传工具的 `duration` 参数（如 `"2h"`/`"7d"`），`btime` 省略默认当前时间、`etime` 自动算；周期时段（`mute_time_type=1`）填 `periodic_mutes` 即可，`btime`/`etime` 可省（工具自动补，且周期匹配根本不看它们）。只有用户给**绝对起止时刻**时才自己填 `btime`/`etime`（Unix 秒，`etime > btime` 是硬校验）。
- `tags` 决定屏蔽哪些告警，多条之间 **AND**；**空数组 = 屏蔽业务组内所有告警**（无条件屏蔽，要跟用户确认这是不是本意）。
- `severities` 建议显式填，`[1,2,3]` = 全部级别。
- `datasource_ids` 空数组 = 全部数据源；`cluster` 固定 `"0"`。
- `in`/`not in` 的 value 用**空格分隔**（如 `"web01 web02"`），也可直接传数组，工具会归一。

---

## 工作流一：创建

1. **确定业务组**：用 `list_busi_groups` 拿 `group_id`。用户点名了业务组、或前端已弹业务组表单时直接用其 ID，不必再问。
2. **构建 config**：按用户描述拼 tags（屏蔽哪台机器/哪类规则/哪些标签）和时间模式；拿不准字段读 `reference.md`。
3. **调用 `create_alert_mute`**：`group_id` 传业务组（也可写在 config 里），`config` 传单个 JSON 对象字符串（不是数组）；固定时段把时长传 `duration` 参数。若没带 `group_id`，工具会自动弹业务组选择表单，用户选完会续上本次创建。
4. **回报结果**：工具返回 `{id, group_id, note, cause, btime, etime, url}`，简要汇报屏蔽对象、生效区间即可。**规则标题用站内链接展示：`[<note>](<url>)`**（url 即返回的 `/alert-mutes/edit/<id>?bgid=<group_id>`，**bgid 参数必须保留**，缺了前端页面不渲染；`note` 为空就用 `cause` 作链接文本），用户可点击直达配置页核对。

## 工作流二：编辑 / 延长 / 排障

1. 用 `list_alert_mutes` / `get_alert_mute_detail` 拿到规则 ID 和现状，确认要改什么。
2. 调用 `update_alert_mute`（**提案制**：调用即向用户展示改动清单并暂停，用户确认后系统自动落库，确认环节不经过你）：`id` 必填，`config` 只写要改的字段（**增量 patch**：未写的字段保持原值；tags/severities/datasource_ids/periodic_mutes 等数组字段提供时**整体替换**——改 tags 先从 detail 拿现有数组，在其基础上改出完整数组再传）。常见操作：
   - **延长/重设屏蔽时长**：直接传 `duration` 参数（如 `"2h"`/`"7d"`），etime 按"从现在再屏蔽这么久"重算，不用算时间戳；用户给绝对截止时刻才在 config 里填 `etime`（Unix 秒）——`etime` 与 `duration` 互斥，二选一，同传会被拒
   - **临时停用** = config 传 `{"disabled":1}`（比删除安全），恢复 = `{"disabled":0}`
   - 业务组不可改；**删除**站内没有工具，让用户在 UI（告警管理 → 告警屏蔽）操作
3. 过期的固定时段屏蔽不会自动删除，可提醒用户定期清理。
4. 用户说"屏蔽了还在告警"时，按 `troubleshooting.md` 的排查链逐关核对，**主动指出**哪一关最可能没过；确认是某关配置问题后，可直接用 `update_alert_mute` 修正。

## 输出风格

1. 创建走工具直接落库；修改是提案制——调用 `update_alert_mute` 后系统会向用户展示改动清单并等确认，所以**调用前不要自己再复述一遍改动**（避免双重确认），调用即完成本轮职责，也不要传 proposal_id/confirmed；HTTP API 仅在用户明确要 curl 时按 `http-api.md` 给命令模板，不执行。
2. 用户描述模糊时（"屏蔽一下那个告警"），先用 `list_alert_mutes` / 事件标签确认屏蔽对象，再给带真实标签值的草稿，不要凭空猜 key。
3. 无条件屏蔽（tags 空）和超长屏蔽（如 30 天）属于高危配置，无论创建还是改出来的，落库前向用户复述影响面。
4. 修改成功后同样按工作流一第 4 步的链接形式展示规则标题（`update_alert_mute` 返回值里也有 `url`）。
