---
name: n9e-notify-rule-copilot
description: 帮助用户在夜莺(n9e)中创建、编辑、复制、排障通知规则(notify_rule)——尤其是把"P1 工作时间发钉钉+电话、非工作时间只电话""按业务组/标签路由""分级走不同通道""恢复时不打电话"这类**自然语言需求**拆成正确的 NotifyConfig 数组。当用户要求"配通知规则 / 编辑通知规则 / 调整路由 / 改适用属性 / 分级通知 / 修复匹配不上 / 拆分接收人"时使用。本技能专注**通知规则的路由层**——不动通知媒介本身（→ n9e-notify-channel-copilot），不动消息模板（→ n9e-generate-message-template），不查"为什么没发出"（→ n9e-alert-rule-troubleshoot 流程 B）。
---

# 夜莺(n9e) 通知规则 Copilot

## 适用范围（先确定用户在改哪一层）

夜莺的通知链路是三层抽象，每层一个 skill，**不要串台**：

| 层 | 实体 | 关键文件 | 用哪个 skill |
|---|---|---|---|
| **媒介** Notify Channel | `notify_channel` 表，`NotifyChannelConfig` | `models/notify_channel.go`、`alert/sender/provider/*.go` | `n9e-notify-channel-copilot` |
| **模板** Message Template | `message_template` 表 | `models/message_template.go` | `n9e-generate-message-template` |
| **规则** Notify Rule | `notify_rule` 表，`NotifyRule` + `NotifyConfig[]` | `models/notify_rule.go`、`center/router/router_notify_rule.go` | **本 skill** |

判断口径：

- 用户原话出现"URL / Webhook 地址 / 请求头 / 代理 / 签名 / AppID / AppSecret / CorpID / 怎么接入 X"——**媒介层**，转 `n9e-notify-channel-copilot`。
- 用户原话出现"模板 / 正文 / 字段 / 卡片颜色 / @ 谁 / `{{ ... }}` 变量"——**模板层**，转 `n9e-generate-message-template`。
- 用户原话出现"**发给谁 / 什么级别走什么通道 / 工作时间 / 按业务组路由 / 按标签过滤 / 多个 NotifyConfig / 适用属性 / 编辑现有规则 / 名称改了规则失联 / 测试 OK 真实告警没发出**"——**规则层**，本 skill。
- 用户原话出现"事件已经产生但没收到通知，帮我看为什么"——**事后诊断**，转 `n9e-alert-rule-troubleshoot` 流程 B。本 skill 只负责"调出正确的规则配置"，不负责回放日志。

与现有 `n9e-create-notify-rule` 的关系：

- `n9e-create-notify-rule` 是**线性 4 步创建流程**（登录 → 查用户组 → 查渠道 → 拼 payload 创建），适合"用户已经讲清楚要什么、按部就班建一条"。
- **本 skill** 是 copilot：① 接住模糊/复杂的自然语言需求并拆成多条 NotifyConfig；② 编辑/复制/微调现有规则；③ 字段级踩坑预警；④ 引导测试发送和真实告警 diff。两个 skill 不冲突，简单创建走前者，复杂场景或编辑场景走本 skill。

---

## 数据模型 `NotifyRule` + `NotifyConfig`

`models/notify_rule.go`:

```go
type NotifyRule struct {
    ID              int64
    Name            string
    Description     string
    Enable          bool
    UserGroupIds    []int64          // 授权团队（决定谁能看/改这条规则，不等于"接收人"）
    PipelineConfigs []PipelineConfig // 关联事件处理 pipeline（齿轮里的 Pipeline/EventDrop/Callback）
    NotifyConfigs   []NotifyConfig   // ★ 真正的"路由表"，一条规则可挂 N 条
    ExtraConfig     interface{}      // plus 商业版字段（escalations 等），开源版不动
}

type NotifyConfig struct {
    ChannelID  int64                  // 走哪条媒介
    TemplateID int64                  // 用哪个消息模板
    Params     map[string]interface{} // 接收人参数（user_ids / user_group_ids / 自定义 webhook 参数）
    Type       string                 // 一般用不上，留空
    Severities []int                  // 适用级别 [1,2,3] / [1] / [2,3]
    TimeRanges []TimeRanges           // 适用时段（多个=多窗口 OR）
    LabelKeys  []TagFilter            // 适用标签（多个 AND）
    Attributes []TagFilter            // 适用属性（多个 AND）
}

type TimeRanges struct {
    Start string // "HH:mm"
    End   string // "HH:mm"，与 Start 都 "00:00" 表示全天
    Week  []int  // 0=周日,1=周一,...,6=周六；[0..6] 表示每天
}
```

**关键心智模型**：一条 `NotifyRule` 像一个文件夹，里面装 N 个 `NotifyConfig`，**每个 NotifyConfig 是一条独立的"路由"**——这条路由有自己的级别/时间/标签/属性过滤、走自己的媒介、用自己的模板、发给自己的接收人。多条 NotifyConfig 之间是**并列 OR**：一个事件命中哪条就走哪条，可以同时命中多条都发。

这就是为什么"P1 工作时间发钉钉+电话、非工作时间只电话"要拆成 3 条 NotifyConfig（见下面"复杂语义拆解"章节）。

---

## 字段地图

### 1) `name` / `description` / `enable`

- `name` 必填，但**不要硬绑死字面值**——前端列表展示用它，但内部没有 ID 引用关系。
- `description` 强烈建议写明"这条规则的路由意图"，比如 `P1 全天 + P2/P3 工作时间通知运维`。规则一多（社区有用户 100+ 条）没 description 就只能靠 name 猜，痛点很大。
- `enable=false` 时整条规则不参与匹配，但还在列表里——临时禁用比删除安全（删了告警规则的 `notify_rule_ids` 关联会失效）。

### 2) `user_group_ids` — **授权团队，不是接收人**

这是 v8 通知规则最容易误解的字段。它的语义是"**这些团队的成员能看到 / 编辑 / 引用这条规则**"——纯权限作用，不是"通知发给这些团队"。

- 接收人由每条 NotifyConfig 的 `params.user_ids` / `params.user_group_ids` 决定。
- `user_group_ids` 空 + 非 admin 用户 → 这条规则**只有 admin 看得到**（`router_notify_rule.go:128-133` 列表过滤）。
- 编辑/删除规则也要求当前用户的团队与 `user_group_ids` 有交集（`router_notify_rule.go:82-86`）。
- 创建一条新规则时至少挂一个团队，否则后面非 admin 改不了。

### 3) `notify_configs[]` — 路由表

至少 1 条；上限没有硬限，但**单条规则建议 ≤ 5 条 NotifyConfig**，超过维护成本急剧上升。如果一条规则要写 10 条 NotifyConfig，多半应该拆成 2-3 条规则。

#### 3.1 `channel_id` — 走哪条媒介

- 必须 > 0，且对应媒介在 `notify_channel` 表里 `enable=true`。
- 同一 `channel_id` 可被多条 NotifyConfig（甚至跨规则）复用——**注意 #3140 提到的"队头阻塞"风险**：同一 webhook 被多条规则共用，一旦它挂了所有规则都阻塞，关键链路建议每条用独立媒介。
- 拿可用列表：`GET /api/n9e/notify-channel-configs`。

#### 3.2 `template_id` — 用哪个消息模板

- 0 或不填 = 用该 channel 的默认模板。
- 模板与 channel 是**强绑定**关系（模板里的字段名取决于 channel 的 RequestConfig），换 channel 后老 template_id 一般要重选。
- 拿列表：`GET /api/n9e/message-templates?channel_id=<id>`。

#### 3.3 `params` — 接收人参数

按 channel 类型不同结构不同：

```jsonc
// 标准用户类（dingtalk/feishu/wecom/email/sms/voice…）
{
  "user_ids": [1, 2, 3],
  "user_group_ids": [10, 11]
}

// flashduty
{ "ids": ["channel_id_1", "channel_id_2"] }

// pagerduty
{
  "pagerduty_integration_ids": ["service_id-integration_id"],
  "pagerduty_integration_keys": ["integration_key"]
}

// 自定义 webhook（channel 的 ParamConfig.Custom.Params 定义了哪些 key）
{ "<custom_key>": "<string_value>", ... }
```

`user_ids` 和 `user_group_ids` **是 OR 关系**，所有命中的用户去重后取联系方式。具体哪个字段（phone / email / dingtalk_userid 等）由 channel 的 `ParamConfig.UserInfo.ContactKey` 决定——如果用户的 `contact_info` 里这个字段空着，就**静默不发**（这是社区"测试 OK 真实告警没发出"最常见根因，详见下面"踩坑速查"）。

群机器人类（dingtalk webhook / wecom 群机器人 / feishu webhook）`ContactKey` 通常留空，此时 `params.user_ids` / `params.user_group_ids` 只用来决定"在卡片里 @ 谁"，不影响是否发送。

#### 3.4 `severities` — 适用级别

- `[1, 2, 3]` = 全级别；`[1]` = 仅 P1；`[2, 3]` = P2 和 P3。
- 不能为空——空数组等于"什么都不匹配"，规则白配。
- **想要"P1 走一路、P2/P3 走另一路"必须拆 NotifyConfig**，不要把 `severities` 写满然后在模板里判断。

#### 3.5 `time_ranges` — 适用时段

- 一条 NotifyConfig 可挂多个 `TimeRanges`，多个之间是 **OR**（任何一个窗口命中就生效）。
- `start="00:00"` + `end="00:00"` + `week=[0,1,2,3,4,5,6]` → 7×24。这是默认值。
- 跨午夜（如 22:00–02:00）要拆成两段：`{start: "22:00", end: "23:59"}` + `{start: "00:00", end: "02:00"}`。引擎不会自动跨天。
- `week` 用国际惯例：**0=周日**。中国用户经常误写成 1=周一-7=周日，注意纠正。

#### 3.6 `label_keys` — 按事件标签过滤

事件标签来自时序数据（PromQL 的 label）+ 告警规则附加标签 + 规则名等。

```json
[
  { "key": "service",    "value": "api" },
  { "key": "env",        "value": "prod" }
]
```

- **多个 label_keys 之间是 AND**（`alert/dispatch/dispatch.go:NotifyRuleMatchCheck`）——事件必须同时带 `service=api` 和 `env=prod` 才命中。
- **同一个 key 不能写多次取 OR**（结构限制）；要 OR 就改用 `attributes` 里的 `in` 操作符，或者拆成多条 NotifyConfig。
- 拿可选 key 列表：`GET /api/n9e/event-tagkeys`。
- 注意 #2804：如果事件标签里没有 `ident`（categraf 直写时序库时丢的），按 ident 路由会全部失配——这是模型上的盲区，要让用户确认数据流。

#### 3.7 `attributes` — 按事件**属性**过滤（不是标签）

这是社区**最容易踩坑**的字段。属性 = 事件元数据，不是用户自定义标签。支持的 key 固定：

| key | 含义 | 支持操作符 | value 说明 |
|---|---|---|---|
| `group_name` | 告警规则所属业务组名 | `==` `!=` `=~` `!~` `in` `not in` | 业务组名称（**注意 #2803：按名绑定，业务组改名后失联**） |
| `cluster` | 数据源名 | `==` `!=` `=~` `!~` `in` `not in` | 数据源名称 |
| `is_recovered` | 是否已恢复 | `==` | `"true"` / `"false"` |
| `rule_id` | 告警规则 ID | `==` `!=` `in` `not in` | 数字字符串 |
| `severity` | 告警级别 | `==` `!=` `in` `not in` | `"1"` / `"2"` / `"3"` |
| `target_group` | 监控对象（主机）所属业务组 | `in` `not in` `=~` `!~` | 业务组 ID（注意是 ID 不是 name） |

操作符语义：

| func | 含义 | value 写法 |
|---|---|---|
| `==` | 精确等 | `"production"` |
| `!=` | 不等 | `"test"` |
| `=~` | 正则匹配 | `"prod-.*"` |
| `!~` | 正则不匹配 | `"test-.*"` |
| `in` | 在列表中 | **空格分隔**：`"prod-01 prod-02 prod-03"` |
| `not in` | 不在列表中 | **空格分隔**：`"test-01 test-02"` |

**多个 attribute 之间也是 AND**，跟 label_keys 一致。

#### 3.8 `pipeline_configs` — 关联事件处理 pipeline

```json
[
  { "pipeline_id": 5, "enable": true }
]
```

事件命中本规则后会按 pipeline 顺序走 EventDrop / Callback / EventUpdate / Relabel / AISummary 等处理器。**这是 v8 把"事件处理"从一级菜单挪到通知规则齿轮里**后的位置（社区林枫等多人找过）。pipeline 本身的内容不在本 skill 范围，只负责把它挂上来。

---

## 复杂语义 → NotifyConfig 拆解模板（copilot 核心价值）

这一节是 copilot 对比 `n9e-create-notify-rule` 的核心差异化能力。下面 6 个模板覆盖了社区 80% 的"我想这样路由"需求：

### 模板 A：分级走不同通道

> 「P1 告警打电话 + 钉钉，P2/P3 告警只发钉钉」

```json
{
  "name": "线上分级告警路由",
  "description": "P1: 电话 + 钉钉；P2/P3: 钉钉",
  "enable": true,
  "user_group_ids": [<oncall_team_id>],
  "notify_configs": [
    {
      "channel_id": <voice_channel_id>,
      "template_id": 0,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1],
      "time_ranges": [{ "week": [0,1,2,3,4,5,6], "start": "00:00", "end": "00:00" }]
    },
    {
      "channel_id": <dingtalk_channel_id>,
      "template_id": 0,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": [{ "week": [0,1,2,3,4,5,6], "start": "00:00", "end": "00:00" }]
    }
  ]
}
```

**关键决策**：钉钉那条 severities 写全 `[1,2,3]`（P1 也发，作为电话之外的"留痕"通道），不要写 `[2,3]` 漏掉 P1 的钉钉记录。

### 模板 B：工作时间 vs 非工作时间不同动作

> 「工作时间（周一到周五 9-18 点）发钉钉群 + @ 值班；非工作时间打电话 + 钉钉」

```json
{
  "name": "值班动作分时段",
  "user_group_ids": [<oncall_team_id>],
  "notify_configs": [
    {
      "channel_id": <dingtalk_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": [{ "week": [1,2,3,4,5], "start": "09:00", "end": "18:00" }]
    },
    {
      "channel_id": <voice_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2],
      "time_ranges": [
        { "week": [1,2,3,4,5], "start": "18:00", "end": "23:59" },
        { "week": [1,2,3,4,5], "start": "00:00", "end": "09:00" },
        { "week": [0, 6],     "start": "00:00", "end": "00:00" }
      ]
    },
    {
      "channel_id": <dingtalk_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2],
      "time_ranges": [
        { "week": [1,2,3,4,5], "start": "18:00", "end": "23:59" },
        { "week": [1,2,3,4,5], "start": "00:00", "end": "09:00" },
        { "week": [0, 6],     "start": "00:00", "end": "00:00" }
      ]
    }
  ]
}
```

**关键决策**：周末 24h 走电话路径，所以是 `week=[0,6]` 全天；工作日跨午夜要拆 18:00–23:59 + 00:00–09:00 两段（不能写 18:00–09:00，引擎当天看）。

### 模板 C：恢复时不打电话

> 「告警时打电话 + 钉钉；恢复时只发钉钉，不要打电话」

```json
{
  "notify_configs": [
    {
      "channel_id": <voice_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1],
      "time_ranges": [{ "week": [0,1,2,3,4,5,6], "start": "00:00", "end": "00:00" }],
      "attributes": [
        { "key": "is_recovered", "func": "==", "value": "false" }
      ]
    },
    {
      "channel_id": <dingtalk_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": [{ "week": [0,1,2,3,4,5,6], "start": "00:00", "end": "00:00" }]
    }
  ]
}
```

**关键决策**：电话那条加 `is_recovered == "false"` 属性即可；钉钉那条不加属性，告警和恢复都走。社区呼声 ≥ 3 次（全时施想 2025-05-15、每一段路-人人 2025-06-23、R 2026-03-09）这是标准解法。

### 模板 D：按业务组路由到不同群

> 「一套告警规则覆盖三个业务组（zone-a/zone-b/zone-c），每个业务组的告警推送到对应钉钉群」

```json
{
  "notify_configs": [
    {
      "channel_id": <ding_zone_a_id>,
      "severities": [1, 2, 3],
      "attributes": [{ "key": "group_name", "func": "==", "value": "zone-a" }]
    },
    {
      "channel_id": <ding_zone_b_id>,
      "severities": [1, 2, 3],
      "attributes": [{ "key": "group_name", "func": "==", "value": "zone-b" }]
    },
    {
      "channel_id": <ding_zone_c_id>,
      "severities": [1, 2, 3],
      "attributes": [{ "key": "group_name", "func": "==", "value": "zone-c" }]
    }
  ]
}
```

**关键决策**：用 `attributes.group_name` 而不是 `label_keys`，因为业务组是告警规则的归属属性，不是事件 label。**注意 #2803**：业务组改名后这里会失配，提醒用户业务组改名要同步规则；或者改用 `=~` 加正则更稳。

### 模板 E：按标签灰度

> 「只通知 env=prod 且 service=payment 的告警，其他全部忽略」

```json
{
  "notify_configs": [
    {
      "channel_id": <ding_channel_id>,
      "severities": [1, 2, 3],
      "label_keys": [
        { "key": "env",     "value": "prod" },
        { "key": "service", "value": "payment" }
      ]
    }
  ]
}
```

**关键决策**：两条 label_keys 是 AND，事件必须同时带这两个标签。如果 `service` 在某些告警事件里不存在（如主机失联告警），这条规则会直接不命中——可以让用户先用 `GET /api/n9e/event-tagkeys` 看下事件实际有哪些 key。

### 模板 F：兜底通知（避免漏告）

> 「我有 N 条精细通知规则，但担心漏，再来一条接所有事件发给 SRE 团队做留痕」

```json
{
  "name": "全量留痕兜底",
  "description": "所有事件都进 SRE 钉钉群，纯留痕用",
  "notify_configs": [
    {
      "channel_id": <sre_archive_ding_id>,
      "template_id": <minimal_template_id>,
      "params": { "user_group_ids": [<sre_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": [{ "week": [0,1,2,3,4,5,6], "start": "00:00", "end": "00:00" }]
    }
  ]
}
```

**关键决策**：留一条"无任何过滤"的规则做兜底——这是 #2871 用户呼声"缺省通知配置"的等价做法。要注意告警规则侧的 `notify_rule_ids` 也得显式挂上这条，否则不生效。

---

## 创建 / 编辑 / 删除 / 测试的三条路径

### 路径 A：HTTP API（首选，可程序化）

| 操作 | 方法 | 路径 | 注意 |
|---|---|---|---|
| 列表 | `GET` | `/api/n9e/notify-rules` | 仅返回当前用户授权团队下的规则 |
| 详情 | `GET` | `/api/n9e/notify-rule/<id>` | |
| 创建 | `POST` | `/api/n9e/notify-rules` | **Body 必须是数组**，即使只建 1 条：`[{...}]` |
| 更新 | `PUT` | `/api/n9e/notify-rule/<id>` | Body 是单对象，会**整体替换**——必须先 GET 再改再 PUT |
| 删除 | `POST` | `/api/n9e/notify-rules/del` | Body: `{"ids":[1,2,3]}` |
| 测试发送 | `POST` | `/api/n9e/notify-rule/test` | Body: `{"event_ids":[<history_event_id>], "notify_config":{...}}` |
| 拿自定义 webhook 参数 | `GET` | `/api/n9e/notify-rule-custom-params?notify_channel_id=<id>` | 用于复制其他规则的自定义参数 |

**编辑动作的正确姿势**：

```text
1. GET /api/n9e/notify-rule/<id>      → 拿到完整 NotifyRule JSON
2. 在本地修改某个字段（如 notify_configs[1].severities = [1]）
3. PUT /api/n9e/notify-rule/<id>      → 整体提交回去
```

**不要试图 PATCH 局部更新**——PUT 走的是 `Update(...).Select("*")`（`models/notify_rule.go:201`），未传字段会被清空。

### 路径 B：UI

- 入口：`告警管理 → 通知规则`
- 适用：用户对 API 不熟、字段少、不熟悉 JSON 结构。
- 一个**重要坑**：UI 上"切换新版"按钮位置历经多次变迁（beta14 隐藏 / 8.4.x 在右上角批量更新里），社区从 2025-03 一直问到 2025-12。如果用户找不到，先让他升到 8.5.1+，再去**告警规则列表的"批量更新"弹窗**找。

### 路径 C：直改 DB（最后手段）

- 表 `notify_rule`，`notify_configs` / `pipeline_configs` / `user_group_ids` 都是 JSON 字段。
- n9e 内存里有 `NotifyRuleCache`，改完会被缓存层在 ~9s 内自动重载，无需重启。
- 改前 `mysqldump -t notify_rule > backup.sql`。

---

## 已知坑速查

| 现象 | 大概率原因 | 处理 |
|---|---|---|
| 测试发送 OK，真实告警没出来 | 接收人 `contact_info.<ContactKey>` 为空 → `sendtos` 空 → 静默不发 | 转 `n9e-alert-rule-troubleshoot` 流程 B；本 skill 只负责让用户检查 channel 的 `ContactKey` 和 user 的 contact_info |
| 告警规则保存了但通知记录一直为空 | 告警规则没关联到这条通知规则（`alert_rule.notify_rule_ids` 为空 / 仍走老版 `notify_groups`） | 告警规则列表 → 批量更新 → 关联通知规则；社区 ≥ 25 次问 |
| 业务组改名后规则突然失配 | `attributes.group_name == "old-name"` 按名字硬绑（#2803） | 改用 `=~` 加正则，或同步改这条规则 |
| `attributes` 用 `in` 多个值无效 | value 写成逗号分隔 `"a,b,c"` | 改成**空格**：`"a b c"`（社区奥利给 2025-04-23 翻车） |
| 多个 NotifyConfig 部分匹配失败时日志暴增 | 现版本日志级别问题，#2871 紫夜尘埃 | 本 skill 范围内能做的：建议用户加一条"兜底 NotifyConfig"（模板 F） |
| 同一 webhook 被 N 条规则共用，单点宕机阻塞所有规则 | #3140 队头阻塞 | 关键链路用独立 channel，本 skill 提示用户拆 channel |
| 编辑保存后某个字段被清空 | PATCH 误用，或前端表单 normalizeValues 把空时间段过滤掉了 | 用 PUT 时**先 GET 再改再 PUT**，保留所有字段 |
| 跨午夜时段（如 22:00–02:00）不生效 | 引擎不跨天，需要拆成两段 | 拆 `22:00–23:59` + `00:00–02:00` |
| `week` 写反了（把 1 当周日） | 用了中国习惯 1=Mon 而非 ISO 0=Sun | 纠正：0=周日，1=周一 … 6=周六 |
| `is_recovered` 值类型踩坑 | 写成 `true`（bool）而不是 `"true"`（字符串） | TagFilter 的 value 字段是 string，必须用 `"true"` / `"false"` |
| 同一个 label key 想要 OR | 结构上不支持 | 改用 `attributes` 的 `in`，或拆多条 NotifyConfig |
| 名称带空格在 `in` 里失效 | #2747 业务组名含空格被空格分隔吞掉 | 改用正则 `=~` 转义 |
| 用户没权限看到这条规则 | `user_group_ids` 没包含此用户所在团队 | 加上对应团队 ID |
| 创建报 `forbidden` | 当前用户不在 `user_group_ids` 任何一个团队 | 加上自己所在团队或让 admin 操作 |

---

## 测试与验证

### 用真实事件做 dry-run

`POST /api/n9e/notify-rule/test` 的语义比 UI 上"测试通知"按钮强：它会**用历史事件 ID + 你传入的 NotifyConfig** 做匹配并真实发送，路径：

```
hisEvents = AlertHisEventGetByIds(event_ids)
for each event:
    dispatch.NotifyRuleMatchCheck(notify_config, event)   ← 真实匹配函数
SendNotifyChannelMessage(notify_config, events)           ← 真实发送
```

意味着：

- 拿一条**真实历史事件 ID**（从历史告警里挑），传入草稿 NotifyConfig，能立刻看到"会不会命中"和"发出去的样子"——这是最贴近"实际告警通道"的验证手段（比 UI 上凭空填一个 mock event 准）。
- 失败时返回的 error 信息能告诉你是匹配失败（哪一步）还是 channel 调用失败。
- 但**它仍然不能验证 `notify_rule_ids` 关联**——这条规则有没有被告警规则挂上去，是另一回事，要去 alert_rule 表 / 告警规则页面看。

### 编辑后的最小验证清单

每次修改一条通知规则，让用户走这 3 步确认：

1. `GET /api/n9e/notify-rule/<id>` 看是不是改对了字段。
2. `POST /api/n9e/notify-rule/test` 用一条相关历史事件验证匹配 + 发送。
3. 真实告警出来后到 `历史告警 → 详情 → 通知记录` 看是否有这条规则的发送日志。

---

## 输出风格

用户问"我想要 X 路由" 或 "为什么这条规则没生效" 时按这个套路答：

1. **第一句话锁定层**：判断用户是不是真在改"规则层"。如果是"为什么没发出"——转 `n9e-alert-rule-troubleshoot`；如果是"模板里少字段"——转 `n9e-generate-message-template`。**不替别人的 skill 做事**。
2. **拆解成 NotifyConfig 数组**：把用户的自然语言路由意图直接映射到上面 6 个模板里最贴近的一个，给出**完整 JSON 草稿**——不要让用户自己去填字段名。
3. **字段级精确指令**：动 `notify_configs[1].attributes[0].func` 这种路径，不是"改一下属性"。
4. **预警已知坑**：用户写出会踩 #2803 / `in` 用逗号 / 跨午夜不拆段这些常见错误时，**主动纠正**并引用一句 issue/群反馈作支撑（让用户知道这不是孤立现象）。
5. **建议先 dry-run 再保存**：拿一条历史事件 ID 用 `POST /notify-rule/test` 验证 → 没问题再正式 PUT/POST。
6. **多条 NotifyConfig 优先于复杂模板**：用户想"在模板里 if-else 判断级别"的时候，引导他**拆 NotifyConfig**，这是夜莺设计的本意。模板只做"内容渲染"，不做"路由决策"。
7. **编辑场景必须 先 GET → 改 → PUT**：不要让用户拿着脑子里的"我大概记得这条规则长什么样"直接 PUT，整体替换会丢字段。
8. **只给指令、不替用户改库或调 API**——除非用户明确说"帮我用 curl 调一下"。可以给完整的 `curl` 命令模板，但不要自己执行。
