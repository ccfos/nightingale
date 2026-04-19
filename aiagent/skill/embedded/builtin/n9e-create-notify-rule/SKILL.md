---
name: n9e-create-notify-rule
description: 在夜莺(n9e)环境中创建通知规则。当用户要求创建通知规则、添加通知策略、配置告警通知方式、设置通知渠道时使用。
---

# 夜莺(n9e) 创建通知规则

在夜莺监控平台上创建通知规则，用于定义告警事件的通知方式、通知对象、通知时段和过滤条件。通知规则可被多条告警规则关联引用。

---

## 前置条件

用户需要提供：
- **n9e 地址**：如 `http://<n9e-host>:<port>`
- **用户名/密码**：如 `<username>/<password>`
- **通知内容描述**：如 "当一级告警触发时，通过邮件通知运维团队"

如果用户未提供以上信息，使用 AskUserQuestion 工具询问。

---

## 执行步骤

### 第一步：登录获取 Token

```
POST /api/n9e/auth/login
Content-Type: application/json
Body: {"username":"<用户名>","password":"<密码>"}
```

从响应中提取 `dat.access_token`，后续请求都带上 `Authorization: Bearer <token>`。

### 第二步：查询团队（用户组）列表

通知规则需要关联用户组，先获取可用的用户组列表：

```
GET /api/n9e/user-groups
Authorization: Bearer <token>
```

将返回的用户组列表通过 **AskUserQuestion** 工具展示给用户，让用户选择要关联的用户组。

### 第三步：查询通知渠道和消息模板

获取可用的通知渠道列表：

```
GET /api/n9e/notify-channel-configs
Authorization: Bearer <token>
```

将返回的通知渠道列表通过 **AskUserQuestion** 工具展示给用户，让用户选择要使用的通知渠道。

选定渠道后，获取该渠道关联的消息模板列表：

```
GET /api/n9e/message-templates?channel_id=<channel_id>
Authorization: Bearer <token>
```

如果有可用模板，让用户选择或使用默认模板（列表中的第一个）。

### 第四步：构建通知规则并创建

根据用户需求构建通知规则 payload 并调用创建 API（payload 必须是**数组**格式）：

```
POST /api/n9e/notify-rules
Authorization: Bearer <token>
Content-Type: application/json
Body: [<通知规则对象>]
```

### 第五步：验证

```
GET /api/n9e/notify-rule/<rule_id>
Authorization: Bearer <token>
```

向用户输出创建结果摘要，包括规则名称、关联的通知渠道和用户组。

---

## 通知规则 Payload 结构

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
      "time_ranges": [
        {
          "week": [0, 1, 2, 3, 4, 5, 6],
          "start": "00:00",
          "end": "00:00"
        }
      ],
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
| `template_id` | int | 否 | 消息模板 ID，不填则使用渠道默认模板 |
| `params` | object | 否 | 渠道参数，不同渠道类型结构不同，详见下方说明 |
| `severities` | int[] | 是 | 适用的告警级别，`[1, 2, 3]` 表示全部级别 |
| `time_ranges` | array | 否 | 通知生效时段列表 |
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

定义通知规则的生效时间窗口：

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

可通过 API 获取可用的事件标签 key：

```
GET /api/n9e/event-tagkeys
Authorization: Bearer <token>
```

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
      "time_ranges": [
        {
          "week": [0, 1, 2, 3, 4, 5, 6],
          "start": "00:00",
          "end": "00:00"
        }
      ],
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
      "time_ranges": [
        {
          "week": [0, 1, 2, 3, 4, 5, 6],
          "start": "00:00",
          "end": "00:00"
        }
      ],
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
      "time_ranges": [
        {
          "week": [0, 1, 2, 3, 4, 5, 6],
          "start": "00:00",
          "end": "00:00"
        }
      ],
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
      "time_ranges": [
        {
          "week": [0, 1, 2, 3, 4, 5, 6],
          "start": "00:00",
          "end": "00:00"
        }
      ],
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

1. **创建 API 接收数组**：即使只创建一条规则，payload 也必须是数组格式 `[{...}]`
2. **channel_id 必须大于 0**：每条 notify_config 必须指定有效的通知渠道 ID
3. **user_group_ids 不能为空**：至少关联一个用户组
4. **severities 不能为空**：至少指定一个告警级别，`[1, 2, 3]` 表示所有级别
5. **全天生效**：`time_ranges` 的 `start` 和 `end` 都设为 `"00:00"` 且 `week` 设为 `[0,1,2,3,4,5,6]` 表示全天候生效
6. **多个 attribute 是 AND 关系**：事件必须同时匹配所有属性条件
7. **in/not in 的 value 用空格分隔**：如 `"1 2 3"`，不要用逗号
8. **template_id 可选**：如果不指定，系统会使用渠道的默认模板
9. **通知规则可被告警规则引用**：创建完成后可在告警规则中通过 `notify_rule_ids` 关联
