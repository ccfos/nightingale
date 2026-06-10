# 通知规则 config 字段全表

数据模型 `models/notify_rule.go`：`NotifyRule` + `NotifyConfig[]`。

## 基础字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 通知规则名称。前端列表展示用它，内部没有 ID 引用关系，不要硬绑死字面值 |
| `description` | string | 否 | 强烈建议写明"这条规则的路由意图"，如 `P1 全天 + P2/P3 工作时间通知运维`。规则一多（上百条）没 description 就只能靠 name 猜 |
| `enable` | bool | 否 | 默认 `true`。`enable=false` 整条规则不参与匹配但还在列表里——临时禁用比删除安全（删了告警规则的 `notify_rule_ids` 关联会失效） |
| `user_group_ids` | int[] | 是 | **授权团队，不是接收人**：这些团队的成员能看到/编辑/引用这条规则（`center/router/router_notify_rule.go` 列表过滤与编辑校验）。空 + 非 admin → 只有 admin 看得到；创建时至少挂一个团队，否则后面非 admin 改不了 |
| `notify_configs` | array | 是 | 路由表，至少 1 条；建议 ≤5 条，要写 10 条多半应拆成 2-3 条规则 |
| `pipeline_configs` | array | 否 | 关联事件处理 pipeline：`[{"pipeline_id": 5, "enable": true}]`。事件命中本规则后按顺序走 EventDrop/Callback/EventUpdate/Relabel/AISummary 等处理器（v8 把"事件处理"挪到了通知规则齿轮里）。pipeline 内容不在本 skill 范围 |

## notify_configs 每条路由

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `channel_id` | int | 是 | 通知媒介 ID，必须 >0 且该媒介 `enable=true`。用 `list_notify_channels` 拿真实 ID。同一媒介可被多条 NotifyConfig（甚至跨规则）复用——注意"队头阻塞"：共用 webhook 挂了会阻塞所有规则，关键链路建议独立媒介 |
| `template_id` | int | 否 | 0 或不填 = 工具自动补该渠道默认模板（weight 最小的，与前端选完渠道自动选第一个模板一致）。模板与渠道**强绑定**（模板字段名取决于渠道 RequestConfig），换渠道后老 template_id 一般要重选。指定特定模板用 `list_message_templates`（`notify_channel_ident` 传渠道 ident）。flashduty/pagerduty 不需要模板 |
| `params` | object | 否 | 接收人参数，按渠道类型不同，见下方 |
| `severities` | int[] | 是 | 适用级别。`[1,2,3]` 全级别；空数组 = 什么都不匹配，规则白配 |
| `time_ranges` | array | 否 | 适用时段。**留空 `[]` 或不填 = 全部时段生效**；多个窗口之间 OR |
| `label_keys` | array | 否 | 按事件标签过滤，多条 **AND** |
| `attributes` | array | 否 | 按事件属性过滤，多条 **AND** |

### severity 告警级别

| 值 | 含义 |
|---|---|
| 1 | 一级报警 (Critical) |
| 2 | 二级报警 (Warning) |
| 3 | 三级报警 (Info) |

## params 渠道参数

### 标准用户类渠道（dingtalk/feishu/wecom/email/sms/voice…）

```json
{ "user_ids": [1, 2, 3], "user_group_ids": [1, 2] }
```

- `user_ids` 与 `user_group_ids` 是 **OR** 关系，命中的用户去重后取联系方式。
- 具体取哪个联系字段（phone/email/dingtalk_userid…）由渠道 `ParamConfig.UserInfo.ContactKey` 决定——用户 `contact_info` 里这个字段空着就**静默不发**（"测试 OK 真实告警没发出"最常见根因）。
- 群机器人类（钉钉/企微/飞书群 webhook）`ContactKey` 通常留空，此时 `params` 只决定"在卡片里 @ 谁"，不影响是否发送。

### Flashduty 渠道

```json
{ "ids": ["flashduty_channel_id_1"] }
```

### PagerDuty 渠道

```json
{
  "pagerduty_integration_ids": ["service_id-integration_id"],
  "pagerduty_integration_keys": ["integration_key"]
}
```

### 自定义 Webhook 渠道

params 的 key 由渠道配置中的 `param_config.custom.params` 定义，值为字符串类型。复制其他规则的自定义参数可用 `GET /api/n9e/notify-rule-custom-params?notify_channel_id=<id>`。

## time_ranges 适用时段

```json
{ "week": [0, 1, 2, 3, 4, 5, 6], "start": "00:00", "end": "00:00" }
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `week` | int[] | 生效星期。**0=周日**, 1=周一, ..., 6=周六（国际惯例，中国用户常误写 1=周一~7=周日，注意纠正） |
| `start` | string | 每天生效开始时间 `HH:mm` |
| `end` | string | 每天生效结束时间 `HH:mm` |

- `start` 和 `end` 都为 `"00:00"` 表示全天 24 小时。
- 一条 NotifyConfig 可挂多个 time_ranges，之间 **OR**。
- **跨午夜（如 22:00–02:00）要拆两段**：`{start:"22:00", end:"23:59"}` + `{start:"00:00", end:"02:00"}`，引擎不会自动跨天。
- 全部时段生效就留空 `[]`，别塞全天条目。

## label_keys 标签过滤

事件标签来自时序数据（PromQL 的 label）+ 告警规则附加标签 + 规则名等。

```json
{ "key": "service", "value": "api" }
```

- 多条 label_keys 之间 **AND**（`alert/dispatch/dispatch.go:NotifyRuleMatchCheck`）。
- **同一个 key 不能写多次取 OR**（结构限制）；要 OR 用 `attributes` 的 `in`，或拆多条 NotifyConfig。
- 可选 key 列表：`GET /api/n9e/event-tagkeys`；不确定事件有哪些标签时在回复里向用户确认。
- 事件标签里没有 `ident`（如 categraf 直写时序库丢失）时按 ident 路由会全部失配，要让用户确认数据流。

## attributes 属性过滤

属性 = 事件元数据，**不是用户自定义标签**。支持的 key 固定：

| key | 含义 | 支持操作符 | value 说明 |
|---|---|---|---|
| `group_name` | 告警规则所属业务组名 | `==` `!=` `=~` `!~` `in` `not in` | 业务组名称（**按名绑定，业务组改名后失联**） |
| `cluster` | 数据源名 | `==` `!=` `=~` `!~` `in` `not in` | 数据源名称 |
| `is_recovered` | 是否已恢复 | `==` | `"true"` / `"false"`（字符串，不是 bool） |
| `rule_id` | 告警规则 ID | `==` `!=` `in` `not in` | 数字字符串 |
| `severity` | 告警级别 | `==` `!=` `in` `not in` | `"1"` / `"2"` / `"3"` |
| `target_group` | 监控对象（主机）所属业务组 | `in` `not in` `=~` `!~` | 业务组 **ID**（不是 name） |

### func 操作符

| func | 含义 | value 写法 |
|---|---|---|
| `==` | 精确匹配 | `"production"` |
| `!=` | 不等于 | `"test"` |
| `=~` | 正则匹配 | `"prod-.*"` |
| `!~` | 正则不匹配 | `"test-.*"` |
| `in` | 在列表中 | **空格分隔**：`"prod-01 prod-02 prod-03"`，不要用逗号 |
| `not in` | 不在列表中 | **空格分隔**：`"test-01 test-02"` |
