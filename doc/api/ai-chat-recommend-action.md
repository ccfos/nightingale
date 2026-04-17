# 各页面快捷提问（recommend_action）规格

新会话创建后，AICopilot 在空会话面板上展示「快捷提问」按钮，文案根据 `/chat/new` 请求里携带的 `page` 决定。**这些预置提示词由前端硬编码维护**（不再由后端 `/chat/new` 接口下发），本文档作为前后端共同的提示词规格。

> 接口请求/响应字段、整体交互时序见 [ai-chat.md](./ai-chat.md)。

## 设计约定

每条快捷提问最终会作为一次 `/message/new` 调用发出，前端按下表填充 `query` 字段：

| 字段 | 来源 | 说明 |
|------|------|------|
| `query.content` | 本文档表格里的「文案」列 | 直接作为用户消息内容 |
| `query.action.key` | 各页面章节标注的 `action.key` | 决定后端的处理路径（`AssistantActionKey`） |
| `query.action.param` | `/chat/new` 时使用的 `param`（即会话的 `page_from.param`） | 前端把页面级上下文透传到 action |
| `query.page_from` | 当前会话的 `page_from` | 与 `/chat/new` 请求一致 |

示例（在 explorer 页面点击第 1 条快捷提问）：

```json
{
  "chat_id": "...",
  "query": {
    "content": "帮我生成一个查询主机 CPU 使用率的语句",
    "action": {
      "key": "query_generator",
      "param": { "datasource_type": "prometheus", "datasource_id": 1 }
    },
    "page_from": {
      "page": "explorer",
      "param": { "datasource_type": "prometheus", "datasource_id": 1 }
    }
  }
}
```

## 页面预置提示词总览

| page | 来源页面 | action.key | 提示词条数 | 页面 param 建议字段 |
|------|----------|------------|-----------|---------------------|
| `explorer` | 时序数据探索 | `query_generator` | 3 | `datasource_type`, `datasource_id` |
| `dashboards` | 仪表盘列表 | `creation` | 3 | `busi_group_id`（可选） |
| `alert_rule` | 告警规则列表 | `creation` | 3 | `busi_group_id`（可选） |
| `alert_history` | 历史告警列表 | `alert_query` | 3 | 可省略 |
| `active_alert` | 活跃告警列表 | `alert_query` | 3 | 可省略 |
| `notify_tpl` | 消息模板配置 | `notify_template_generator` | 4 | 可省略 |
| `datasource` | 数据源配置 | `datasource_diagnose` | 3 | `datasource_type`, `datasource_id`（可选） |

未列出的 `page` 值不展示快捷提问。

## 各页面的提示词内容

文案多语言由前端按当前 UI 语种就地切换；下表列出英文（缺省）与简体中文，其他语种的翻译由前端根据 i18n 资源补齐。

### `explorer` — 时序数据探索

action.key = `query_generator`

| # | 英文（缺省） | zh_CN |
|---|-------------|-------|
| 1 | Generate a query for host CPU usage | 帮我生成一个查询主机 CPU 使用率的语句 |
| 2 | Generate a query for memory usage | 帮我生成一个查询机器内存使用率的语句 |
| 3 | Generate a query for host disk usage | 帮我生成一个查询机器磁盘使用率的语句 |

`param` 通常携带当前查询编辑器的数据源（`datasource_type`, `datasource_id`），后端 `query_generator` 据此选择目标 LLM/Skill。

### `dashboards` — 仪表盘列表

action.key = `creation`

| # | 英文（缺省） | zh_CN |
|---|-------------|-------|
| 1 | Create a Host machine dashboard | 帮我创建一个 Host 机器的仪表盘 |
| 2 | Create a MySQL dashboard | 帮我创建一个 MySQL 的仪表盘 |
| 3 | Create a Redis dashboard | 帮我创建一个 Redis 的仪表盘 |

> `creation` 操作会触发 preflight：若 `param` 中缺少 `busi_group_id` 等必填上下文，后端会先返回 `form_select` 让用户补齐。详见 ai-chat.md 中「创建类操作的 preflight 流程」。

### `alert_rule` — 告警规则列表

action.key = `creation`

| # | 英文（缺省） | zh_CN |
|---|-------------|-------|
| 1 | Create a CPU usage alert rule with a threshold above 80% | 创建一条 CPU 使用率超过 80% 的告警规则 |
| 2 | Create a host down alert rule based on target heartbeat loss | 创建一条主机失联的告警规则 |
| 3 | Create a disk usage alert rule with a threshold above 85% | 创建一条机器磁盘使用率超过 85% 的告警规则 |

> 与 `dashboards` 一样会走 preflight；`n9e-create-alert-rule` skill 同时要求 `busi_group_id` 与 `datasource_id`。

### `alert_history` — 历史告警列表

action.key = `alert_query`

| # | 英文（缺省） | zh_CN |
|---|-------------|-------|
| 1 | Summarize alert trends in the current filter range | 总结当前筛选范围内的告警趋势 |
| 2 | Which alert rules fired most frequently | 哪些告警规则触发最频繁 |
| 3 | Break down current alerts by severity, busi group and target | 按级别、业务组、对象拆解当前告警 |

### `active_alert` — 活跃告警列表

action.key = `alert_query`

| # | 英文（缺省） | zh_CN |
|---|-------------|-------|
| 1 | Summarize the distribution of currently active alerts | 总结当前活跃告警的分布情况 |
| 2 | Which rules or targets have the most active alerts | 哪些规则或对象的活跃告警最多 |
| 3 | Group current active alerts by severity and busi group | 按级别和业务组汇总当前活跃告警 |

### `notify_tpl` — 消息模板配置

action.key = `notify_template_generator`

| # | 英文（缺省） | zh_CN |
|---|-------------|-------|
| 1 | Add hostname and severity label to the notification template | 在通知模板中加入主机名和告警级别 |
| 2 | Format trigger_value with two decimal places in the template | 把 trigger_value 保留两位小数 |
| 3 | Include a runbook link in the notification template | 在通知模板中加入排障文档链接 |
| 4 | Add alert duration and first triggered time to the template | 在模板中加入告警持续时间和首次触发时间 |

### `datasource` — 数据源配置

action.key = `datasource_diagnose`

| # | 英文（缺省） | zh_CN |
|---|-------------|-------|
| 1 | Diagnose why datasource connection fails with an x509 certificate error | 数据源连接报 x509 证书错误，如何排查 |
| 2 | My datasource test returns 401 unauthorized, how to fix | 数据源测试连通返回 401 怎么解决 |
| 3 | Help me write the correct URL for connecting Nightingale to this datasource | 帮我写这个数据源的正确接入 URL |

## 新增/修改预置提示词

1. 在前端 AICopilot 的预置提示词常量表里新增/修改对应 `page` 下的条目，文案在前端 i18n 资源中补齐多语种翻译。
2. 同步更新本文件里对应章节的表格。
3. 如新增的是新 `page` 类型，需要在后端 `models/ai_assistant.go` 的 `AssistantPageType` 常量、以及 ai-chat.md 的 `page` 枚举值表里同步登记，确保 `/chat/new` 接收到新值时不会被当作未知 page。
