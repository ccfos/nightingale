---
name: n9e-create-notify-rule
description: 在夜莺(n9e)环境中创建通知规则。当用户要求创建通知规则、添加通知策略、配置告警通知方式、设置通知渠道时使用。
tags:
  - internal
---

# 夜莺(n9e) 创建通知规则

在夜莺监控平台上创建通知规则，用于定义告警事件的通知方式、通知对象、通知时段和过滤条件。通知规则可被多条告警规则关联引用。

---

## 前提

你是 n9e 站内 AI 助手，运行在 n9e 进程内、已以当前用户身份认证。**直接调用内置工具创建，不要登录、不要调 HTTP API、不要用 http_fetch 打自家接口。**

---

## 执行步骤

### 第一步：确定接收通知的团队（用户组）

通知规则关联到一个或多个团队(用户组)。用 `list_teams` 列出可选团队，拿到 `user_group_ids`。
> 用户在对话里点名了团队、或前端已弹出团队选择表单时，直接用其 ID，不必再问。

### 第二步：确定通知媒介和消息模板

每条 `notify_config` 需要一个通知媒介 `channel_id` 和一个消息模板 `template_id`：
- 用 `list_notify_channels` 列出已启用的通知媒介，拿到 `channel_id` 和 `ident`（如 dingtalk/email/tx-sms）。
- 按用户描述（钉钉 / 邮件 / 电话 / 短信…）匹配出对应的 `channel_id`；匹配不到就在回复里把候选列给用户让其选。
- `template_id`（消息模板）**可不填**：`create_notify_rule` 会按 `channel_id` 反查渠道、自动补该渠道的默认模板（该渠道下 weight 最小的那个，与前端选完渠道自动选第一个模板一致）。
  - 仅当用户点名了某个具体模板时，才用 `list_message_templates`（`notify_channel_ident` 传渠道 `ident`）取其 `id` 填进 `template_id`。
  - flashduty / pagerduty 渠道本就不需要模板。

### 第三步：构建 config 并调用 create_notify_rule

按下文「config 结构」把规则拼成一个 JSON 对象，调用 `create_notify_rule` 工具，`config` 参数传这段 JSON 字符串。
- 若没带 `user_group_ids`，工具会自动弹出团队选择表单，用户选完会续上本次创建，无需你重试登录之类。

### 第四步：回报结果

工具返回 `{id, name, user_group_ids, notify_configs_count}`。据此向用户简要汇报创建结果（规则名、关联团队、通知配置条数）即可。

---

## config 结构

```json
{
  "name": "通知规则名称",
  "description": "规则描述",
  "enable": true,
  "user_group_ids": [1, 2],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": {
        "user_ids": [1, 2],
        "user_group_ids": [1]
      },
      "severities": [1, 2, 3],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    }
  ]
}
```

---

## 字段说明

### 基础字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 通知规则名称 |
| `description` | string | 否 | 规则描述说明 |
| `enable` | bool | 否 | 是否启用，默认 `true` |
| `user_group_ids` | int[] | 是 | 关联的用户组（团队）ID 列表 |
| `notify_configs` | array | 是 | 通知配置列表，可配置多条 |

### notify_configs 通知配置

每条通知配置定义一个通知渠道及其过滤条件：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `channel_id` | int | 是 | 通知渠道 ID，必须大于 0 |
| `template_id` | int | 否 | 消息模板 ID。**不填则工具自动补该渠道的默认模板**（该渠道下 weight 最小的）；要指定特定模板时用 `list_message_templates` 按渠道 `ident` 取真实 id |
| `params` | object | 否 | 渠道参数，不同渠道类型结构不同，详见下方说明 |
| `severities` | int[] | 是 | 适用的告警级别，`[1, 2, 3]` 表示全部级别 |
| `time_ranges` | array | 否 | 通知生效时段；**留空 `[]` 或不填 = 全部时段生效**，仅在需要限制时段时才填 |
| `label_keys` | array | 否 | 按事件标签过滤 |
| `attributes` | array | 否 | 按事件属性过滤 |

### severity 告警级别

| 值 | 含义 |
|---|---|
| 1 | 一级报警 (Critical) |
| 2 | 二级报警 (Warning) |
| 3 | 三级报警 (Info) |

### params 渠道参数

params 结构取决于通知渠道类型：

#### 用户通知类渠道（邮件、短信、电话、即时通讯等）

```json
{
  "user_ids": [1, 2, 3],
  "user_group_ids": [1, 2]
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `user_ids` | int[] | 通知的用户 ID 列表 |
| `user_group_ids` | int[] | 通知的用户组 ID 列表 |

#### Flashduty 渠道

```json
{
  "ids": ["flashduty_channel_id_1"]
}
```

#### PagerDuty 渠道

```json
{
  "pagerduty_integration_ids": ["service_id-integration_id"],
  "pagerduty_integration_keys": ["integration_key"]
}
```

#### 自定义 Webhook 渠道

params 的 key 由渠道配置中的 `param_config.custom.params` 定义，值为字符串类型。

### time_ranges 通知时段

**留空数组 `[]` 或不配置 `time_ranges` 即表示全部时段生效**，这是最常见的默认，无需填任何条目。只有当用户明确要"仅在某些时段才发通知"时，才配置生效时间窗口：

```json
{
  "week": [0, 1, 2, 3, 4, 5, 6],
  "start": "00:00",
  "end": "00:00"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `week` | int[] | 生效的星期。0=周日, 1=周一, ..., 6=周六 |
| `start` | string | 每天生效开始时间，格式 `HH:mm` |
| `end` | string | 每天生效结束时间，格式 `HH:mm` |

- `start` 和 `end` 都设为 `"00:00"` 表示全天 24 小时生效
- `week` 设为 `[0,1,2,3,4,5,6]` 表示每天生效
- 可配置多组 time_ranges 实现多个时间窗口

### label_keys 标签过滤

按告警事件的标签进行过滤，只有匹配的事件才会通过该通知配置发送：

```json
{
  "key": "标签名",
  "value": "标签值"
}
```

标签 key 来自告警事件自身的标签（如 `service`、`region`），按用户描述填即可；不确定有哪些标签时，可在回复里向用户确认。

### attributes 属性过滤

按告警事件的属性进行过滤：

```json
{
  "key": "属性名",
  "func": "匹配操作符",
  "value": "匹配值"
}
```

**多个 attribute 之间是 AND 关系**，事件必须同时匹配所有条件。

#### 支持的属性 key 及操作符

| key | 含义 | 支持的操作符 | value 说明 |
|---|---|---|---|
| `group_name` | 业务组名称 | `==`, `!=`, `=~`, `!~`, `in`, `not in` | 业务组名称 |
| `cluster` | 数据源 | `==`, `!=`, `=~`, `!~`, `in`, `not in` | 数据源名称 |
| `is_recovered` | 是否已恢复 | `==` | `"true"` 或 `"false"` |
| `rule_id` | 告警规则 ID | `==`, `!=`, `in`, `not in` | 告警规则 ID |
| `severity` | 告警级别 | `==`, `!=`, `in`, `not in` | `"1"`, `"2"`, `"3"` |
| `target_group` | 监控对象业务组 | `in`, `not in`, `=~`, `!~` | 业务组 ID |

#### func 操作符说明

| func 操作符 | 含义 | value 示例 |
|---|---|---|
| `==` | 精确匹配 | `"production"` |
| `!=` | 不等于 | `"test"` |
| `=~` | 正则匹配 | `"prod-.*"` |
| `!~` | 正则不匹配 | `"test-.*"` |
| `in` | 在列表中（空格分隔） | `"prod-01 prod-02 prod-03"` |
| `not in` | 不在列表中（空格分隔） | `"test-01 test-02"` |

---

## 完整示例

### 示例一：基础通知规则（全天候通知所有级别告警）

```json
{
  "name": "运维团队告警通知",
  "description": "所有级别的告警都通知运维团队",
  "enable": true,
  "user_group_ids": [1],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": {
        "user_group_ids": [1]
      },
      "severities": [1, 2, 3],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    }
  ]
}
```

### 示例二：按级别分渠道通知

```json
{
  "name": "分级通知策略",
  "description": "一级告警电话通知，二三级告警邮件通知",
  "enable": true,
  "user_group_ids": [1, 2],
  "notify_configs": [
    {
      "channel_id": 2,
      "template_id": 3,
      "params": {
        "user_ids": [1, 2],
        "user_group_ids": [1]
      },
      "severities": [1],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    },
    {
      "channel_id": 1,
      "template_id": 1,
      "params": {
        "user_group_ids": [2]
      },
      "severities": [2, 3],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    }
  ]
}
```

### 示例三：工作时间通知 + 属性过滤

```json
{
  "name": "生产环境工作时间通知",
  "description": "仅工作日工作时间通知生产环境的告警",
  "enable": true,
  "user_group_ids": [1],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": {
        "user_group_ids": [1]
      },
      "severities": [1, 2],
      "time_ranges": [
        {
          "week": [1, 2, 3, 4, 5],
          "start": "09:00",
          "end": "18:00"
        }
      ],
      "label_keys": [],
      "attributes": [
        {
          "key": "group_name",
          "func": "=~",
          "value": "prod-.*"
        }
      ]
    }
  ]
}
```

### 示例四：按标签和属性联合过滤

```json
{
  "name": "API服务告警通知",
  "description": "通知 API 服务相关的未恢复告警",
  "enable": true,
  "user_group_ids": [3],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": {
        "user_group_ids": [3]
      },
      "severities": [1, 2, 3],
      "time_ranges": [],
      "label_keys": [
        {
          "key": "service",
          "value": "api"
        }
      ],
      "attributes": [
        {
          "key": "is_recovered",
          "func": "==",
          "value": "false"
        },
        {
          "key": "severity",
          "func": "in",
          "value": "1 2"
        }
      ]
    }
  ]
}
```

---

## 关键注意事项

1. **config 是单个 JSON 对象**：`create_notify_rule` 的 `config` 传一个规则对象（不是数组）；一次建一条
2. **channel_id 必须大于 0**：每条 notify_config 必须指定有效的通知渠道 ID（用 list_notify_channels 拿真实 ID，别猜）
3. **user_group_ids 不能为空**：至少关联一个用户组
4. **severities 不能为空**：至少指定一个告警级别，`[1, 2, 3]` 表示所有级别
5. **全部时段无需填**：`time_ranges` 留空 `[]`（或不填）即表示全部时段生效，别再塞全天 `00:00-00:00` 条目；只有要限制时段时才配置 `time_ranges`
6. **多个 attribute 是 AND 关系**：事件必须同时匹配所有属性条件
7. **in/not in 的 value 用空格分隔**：如 `"1 2 3"`，不要用逗号
8. **template_id 可不填**：缺省时 `create_notify_rule` 会按渠道反查、自动补该渠道默认模板（weight 最小的），不会再出现 template_id=0 发不出通知的问题；只有要指定特定模板才用 `list_message_templates` 取真实 id
9. **通知规则可被告警规则引用**：创建完成后可在告警规则中通过 `notify_rule_ids` 关联
