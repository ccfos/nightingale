# Per-page Quick Prompts (recommend_action) Specification

After a new chat is created, AICopilot shows "quick prompt" buttons on the empty chat panel. Their wording is determined by the `page` carried in the `/chat/new` request. **These preset prompts are hardcoded and maintained in the frontend** (they are no longer delivered by the backend `/chat/new` endpoint); this document is the shared frontend/backend specification for them.

## Design conventions

Every quick prompt is eventually sent as a single `/message/new` call. The frontend fills in the `query` field as follows:

| Field | Source | Description |
|------|------|------|
| `query.content` | The "text" column of the tables in this document | Used directly as the user message content |
| `query.action.param` | The `param` used on `/chat/new` (i.e. the chat's `page_from.param`) | The frontend passes the page-level context through to the action |
| `query.page_from` | The current chat's `page_from` | Same as in the `/chat/new` request |

> **Do not send `action.key`**: the field is deprecated and the backend no longer reads it (the frontend strips it before sending). The processing path is resolved deterministically by the backend from `content` — creation-style wording hits the `creation` keyword fast-path, everything else goes to the `general_chat` general-purpose agent.

Example (clicking the first quick prompt on the explorer page):

```json
{
  "chat_id": "...",
  "query": {
    "content": "Generate a query for host CPU usage",
    "action": {
      "param": { "datasource_type": "prometheus", "datasource_id": 1 }
    },
    "page_from": {
      "page": "explorer",
      "param": { "datasource_type": "prometheus", "datasource_id": 1 }
    }
  }
}
```

## Overview of preset prompts per page

| page | Source page | Prompt count | Suggested page `param` fields |
|------|----------|-----------|---------------------|
| `explorer` | Metrics explorer | 3 | `datasource_type`, `datasource_id` |
| `dashboards` | Dashboard list | 3 | `busi_group_id` (optional) |
| `alert_rule` | Alert rule list | 3 | `busi_group_id` (optional) |
| `alert_history` | Historical alert list | 3 | May be omitted |
| `active_alert` | Active alert list | 3 | May be omitted |
| `notify_tpl` | Message template configuration | 4 | May be omitted |
| `datasource` | Data source configuration | 3 | `datasource_type`, `datasource_id` (optional) |
| `alert_event_detail` | Alert event details | 3 | `event_id` (required); optionally `rule_id`, `target_ident`, `datasource_id` |

Any `page` value not listed here shows no quick prompts.

## Prompt text per page

Localization of the text is handled in place by the frontend according to the current UI language. The tables below list English (the default) and Simplified Chinese; translations for other languages are filled in by the frontend from its i18n resources.

### `explorer` — Metrics explorer

| # | English (default) | zh_CN |
|---|-------------|-------|
| 1 | Generate a query for host CPU usage | 帮我生成一个查询主机 CPU 使用率的语句 |
| 2 | Generate a query for memory usage | 帮我生成一个查询机器内存使用率的语句 |
| 3 | Generate a query for host disk usage | 帮我生成一个查询机器磁盘使用率的语句 |

`param` usually carries the data source of the current query editor (`datasource_type`, `datasource_id`). The backend injects it into the prompt and forwards it to the query tool layer. The generated query is embedded as a Markdown code block in the body of the `markdown` answer.

### `dashboards` — Dashboard list

| # | English (default) | zh_CN |
|---|-------------|-------|
| 1 | Create a Host machine dashboard | 帮我创建一个 Host 机器的仪表盘 |
| 2 | Create a MySQL dashboard | 帮我创建一个 MySQL 的仪表盘 |
| 3 | Create a Redis dashboard | 帮我创建一个 Redis 的仪表盘 |

> These prompts hit the backend `creation` keyword fast-path and trigger a preflight: if `param` is missing required context such as `busi_group_id`, the backend first returns a `form_select` so the user can supply it.

### `alert_rule` — Alert rule list

| # | English (default) | zh_CN |
|---|-------------|-------|
| 1 | Create a CPU usage alert rule with a threshold above 80% | 创建一条 CPU 使用率超过 80% 的告警规则 |
| 2 | Create a host down alert rule based on target heartbeat loss | 创建一条主机失联的告警规则 |
| 3 | Create a disk usage alert rule with a threshold above 85% | 创建一条机器磁盘使用率超过 85% 的告警规则 |

> Like `dashboards`, these hit the creation fast-path and go through preflight; the `create-alert-rule` skill requires both `busi_group_id` and `datasource_id`.

### `alert_history` — Historical alert list

| # | English (default) | zh_CN |
|---|-------------|-------|
| 1 | Summarize alert trends in the current filter range | 总结当前筛选范围内的告警趋势 |
| 2 | Which alert rules fired most frequently | 哪些告警规则触发最频繁 |
| 3 | Break down current alerts by severity, busi group and target | 按级别、业务组、对象拆解当前告警 |

### `active_alert` — Active alert list

| # | English (default) | zh_CN |
|---|-------------|-------|
| 1 | Summarize the distribution of currently active alerts | 总结当前活跃告警的分布情况 |
| 2 | Which rules or targets have the most active alerts | 哪些规则或对象的活跃告警最多 |
| 3 | Group current active alerts by severity and busi group | 按级别和业务组汇总当前活跃告警 |

### `notify_tpl` — Message template configuration

| # | English (default) | zh_CN |
|---|-------------|-------|
| 1 | Add hostname and severity label to the notification template | 在通知模板中加入主机名和告警级别 |
| 2 | Format trigger_value with two decimal places in the template | 把 trigger_value 保留两位小数 |
| 3 | Include a runbook link in the notification template | 在通知模板中加入排障文档链接 |
| 4 | Add alert duration and first triggered time to the template | 在模板中加入告警持续时间和首次触发时间 |

### `datasource` — Data source configuration

| # | English (default) | zh_CN |
|---|-------------|-------|
| 1 | Diagnose why datasource connection fails with an x509 certificate error | 数据源连接报 x509 证书错误，如何排查 |
| 2 | My datasource test returns 401 unauthorized, how to fix | 数据源测试连通返回 401 怎么解决 |
| 3 | Help me write the correct URL for connecting Nightingale to this datasource | 帮我写这个数据源的正确接入 URL |

### `alert_event_detail` — Alert event details

| # | English (default) | zh_CN |
|---|-------------|-------|
| 1 | Analyze the root cause of this alert event | 分析这条告警事件的根因 |
| 2 | Find similar historical alerts on the same target/rule | 查找同对象/同规则下的相似历史告警 |
| 3 | Show other active alerts on the same target around this time | 看下同一对象在这个时间点附近还有哪些活跃告警 |

> `param` must carry `event_id`; the backend injects it into the prompt context so the general-purpose agent can call tools such as `get_alert_event_detail` to read the event details. Passing `rule_id`, `target_ident`, and `datasource_id` as well reduces follow-up queries.

## Adding or changing preset prompts

1. Add or change the entry under the corresponding `page` in the frontend AICopilot preset-prompt constant table, and add the localized wording to the frontend i18n resources.
2. Update the matching section table in this file.
3. If you are adding a new `page` type, register it in the `AssistantPageType` constants in the backend `models/ai_assistant.go` as well (and update the "Overview of preset prompts per page" table above), so that `/chat/new` does not treat the new value as an unknown page.
