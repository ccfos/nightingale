# Full table of notify rule config fields

Data model `models/notify_rule.go`: `NotifyRule` + `NotifyConfig[]`.

## Basic fields

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Notify rule name. The frontend list displays it; internally there is no ID reference relationship, so don't hard-bind to the literal value |
| `description` | string | No | Strongly recommended to state "this rule's routing intent", e.g. `P1 all day + P2/P3 notify ops during working hours`. Once there are many rules (hundreds), without a description you can only guess from the name |
| `enable` | bool | No | Defaults to `true`. `enable=false` means the whole rule does not participate in matching but stays in the list — temporarily disabling is safer than deleting (deleting breaks the `notify_rule_ids` association in alert rules) |
| `user_group_ids` | int[] | Yes | **Authorized teams, not recipients**: members of these teams can see/edit/reference this rule (`center/router/router_notify_rule.go` list filtering and edit validation). Empty + non-admin → only admins can see it; attach at least one team at creation, otherwise non-admins can't edit it later |
| `notify_configs` | array | Yes | The routing table, at least 1 entry; recommended ≤5, and if you find yourself writing 10 you should probably split into 2-3 rules |
| `pipeline_configs` | array | No | Associated event-processing pipelines: `[{"pipeline_id": 5, "enable": true}]`. After an event matches this rule, it runs through processors such as EventDrop/Callback/EventUpdate/Relabel/AISummary in order (v8 moved "event processing" into the notify rule's gear menu). Pipeline contents are out of scope for this skill |

## notify_configs (per route)

| Field | Type | Required | Description |
|---|---|---|---|
| `channel_id` | int | Yes | Notification medium ID, must be >0 and the medium must have `enable=true`. Use `list_notify_channels` to get the real ID. The same medium can be reused by multiple NotifyConfigs (even across rules) — beware "head-of-line blocking": if a shared webhook goes down it blocks all rules, so use a dedicated medium for critical paths |
| `template_id` | int | No | 0 or omitted = the tool auto-fills that channel's default template (the one with the smallest weight, matching how the frontend auto-selects the first template after you pick a channel). Templates are **strongly bound** to channels (template field names depend on the channel RequestConfig), so an old template_id generally must be reselected after switching channels. To specify a particular template use `list_message_templates` (pass the channel ident as `notify_channel_ident`). flashduty/pagerduty do not need a template |
| `params` | object | No | Recipient parameters, varying by channel type; see below |
| `severities` | int[] | Yes | Applicable severities. `[1,2,3]` = all severities; an empty array = matches nothing, the rule is configured for nothing |
| `time_ranges` | array | No | Applicable time windows. **Empty `[]` or omitted = effective at all times**; multiple windows are OR'd |
| `label_keys` | array | No | Filter by event labels, multiple entries **AND'd** |
| `attributes` | array | No | Filter by event attributes, multiple entries **AND'd** |

### severity (alert level)

| Value | Meaning |
|---|---|
| 1 | Level-1 alert (Critical) |
| 2 | Level-2 alert (Warning) |
| 3 | Level-3 alert (Info) |

## params (channel parameters)

**The shape of params is determined by the chosen medium; the information different media require from the user differs completely.** The three fields `contact_key` / `custom_params` / `request_type` returned by `list_notify_channels` are the basis for the decision — match them up:

| Decision | params shape | What the user must provide |
|---|---|---|
| `contact_key` is non-empty (email/SMS/phone…) | `{"user_ids":[...], "user_group_ids":[...]}` | Who the recipients are (people or teams) |
| `custom_params` is non-empty (DingTalk/WeCom/Feishu group bots, callback, telegram, custom webhook…) | `{"<key>": "<value>"}` fill a string per key | The value of each key, e.g. the group bot's access_token |
| `request_type=flashduty` | `{"ids": [<collaboration-space channel_id>]}` | The FlashDuty collaboration-space ID (if omitted, the integration's default space is used) |
| `request_type=pagerduty` | See PagerDuty below | Pick the target Service (in the tool scenario, have the user provide the integration key) |

The first two rows are **not mutually exclusive**: a user's custom channel may have both `contact_key` and `custom_params` non-empty, in which case custom_params decides whether the message is sent (**required**), while `user_ids`/`user_group_ids` only decide who to @ in the group (optional) — don't miss the token by matching only on the first row.

**If information is missing, stop and ask the user — do not leave it blank or fabricate it** — and when asking, attach the corresponding official doc from "Medium parameter quick lookup and doc links" below, telling the user where to obtain the value (e.g. "DingTalk group → group settings → smart group assistant → the string after `access_token=` in the bot Webhook URL").

### User-recipient type (contact_key non-empty)

```json
{ "user_ids": [1, 2, 3], "user_group_ids": [1, 2] }
```

- `user_ids` and `user_group_ids` are an **OR** relationship; the matched users are deduplicated and their contact methods are taken.
- Which specific contact field is taken (phone/email/dingtalk_userid…) is exactly `contact_key` — if that field is blank in the user's `contact_info`, it is **silently not sent** (the most common root cause of "test was OK but the real alert wasn't sent"); have the user confirm the recipient has filled in that field in the personal center / user management.

### Custom-parameter type (custom_params non-empty)

The keys of params correspond one-to-one with the keys returned by `custom_params`, the values are all strings, supplied by the user. The built-in group-bot channels are all of this type:

```json
{ "access_token": "xxxx", "bot_name": "Nightingale Alerts" }
```

- Whether the message is sent depends only on these parameters (e.g. whether the access_token is correct), unrelated to recipients; if the medium has a "contact method" configured (contact_key also non-empty), you can additionally add `user_ids` to decide who to **@** in the group.
- token/key/url must come from the user; **note parameters** like `bot_name` / `note` are asked about in passing when requesting the token ("which group / what purpose is this bot, give me a note") — if the user does not provide one, **do not press them and do not leave it blank**, but auto-generate one from the notify rule's configuration (using the rule name / routing intent, e.g. "Nightingale Alerts - Core Service P1"); once there are many rules, without notes you can't tell which token maps to which group.
- **Reuse already-filled values**: use `list_notify_rule_custom_params(notify_channel_id)` to look up parameter values that other rules have filled in for this medium (grouped by value, with the names of the rules using them attached). When the user says "send to the same DingTalk group as rule X" or doesn't bring a token, check here first; if it matches, reuse it directly without making the user dig out the Webhook; if nothing is found or the user explicitly says it's a new group, ask the user for it.

### Flashduty channel

```json
{ "ids": [123] }
```

### PagerDuty channel

```json
{
  "pagerduty_integration_ids": ["service_id-integration_id"],
  "pagerduty_integration_keys": ["integration_key"]
}
```

### Medium parameter quick lookup and doc links

When asking the user for parameters, attach the corresponding link (clickable, external). The URL prefix is uniformly `https://flashcat.cloud/docs/content/flashcat-monitor/nightingale-v9/usage/alert-notify/notify-channel/`; the table below only lists the final segment:

| Medium ident | What the user must provide in the notify rule | Doc final segment |
|---|---|---|
| `dingtalk` DingTalk group bot | `access_token`: the string after `access_token=` in the bot Webhook URL (group settings → smart group assistant → bot) | `dingtalk/` |
| `wecom` WeCom group bot | `key`: the string after `key=` in the group bot Webhook URL | `wecom/` |
| `feishucard` Feishu group card | `access_token`: the final segment of the group bot Webhook URL | `feishucard/` |
| `larkcard` Lark group card (international edition) | `access_token`: same as above | `larkcard/` |
| `email` Email | Recipients (the user must already have an email configured) | `email/` |
| `ali-sms` / `tx-sms` SMS | Recipients (the user must already have a phone number configured) | `ali-sms/` `tx-sms/` |
| `ali-voice` / `tx-voice` Phone | Recipients (the user must already have a phone number configured) | `ali-voice/` `tx-voice/` |
| `flashduty` FlashDuty | Collaboration-space ID (if omitted, the integration's default space is used) | `flashduty/` |
| `pagerduty` PagerDuty | Target Service (frontend dropdown selection; the API scenario needs the integration key) | `pagerduty/` |
| `callback` Callback | `callback_url`: an HTTP(S) address reachable by Nightingale | `callback/` |
| `telegram` Telegram | Bot Token + Chat ID | `telegram/` |
| `jsm_alert` JSM Alert | API Key (generated by the JSM API integration) | `jsm-alert/` |
| `script` Script | Usually no parameters needed (the script is in the medium) | `script/` |

For media a user built themselves (idents not in the table above), go by the keys returned by `custom_params` and ask about each one; when you can't figure out a key's meaning, give the user the notification-medium overview doc: `.../notify-channel/` (i.e. the prefix itself).

## time_ranges (applicable time windows)

```json
{ "week": [0, 1, 2, 3, 4, 5, 6], "start": "00:00", "end": "00:00" }
```

| Field | Type | Description |
|---|---|---|
| `week` | int[] | Effective days of week. **0=Sunday**, 1=Monday, ..., 6=Saturday (international convention; Chinese users often mistakenly write 1=Monday~7=Sunday, watch for and correct this) |
| `start` | string | Daily start time of effectiveness `HH:mm` |
| `end` | string | Daily end time of effectiveness `HH:mm` |

- Both `start` and `end` being `"00:00"` means the full 24 hours of the day.
- One NotifyConfig can have multiple time_ranges, **OR'd** between them.
- **Crossing midnight (e.g. 22:00–02:00) must be split into two segments**: `{start:"22:00", end:"23:59"}` + `{start:"00:00", end:"02:00"}`; the engine does not roll over to the next day automatically.
- For effectiveness at all times, leave it empty `[]`; don't stuff in an all-day entry.

## label_keys (label filtering)

Event labels come from time-series data (PromQL labels) + alert-rule additional labels + the rule name, etc.

```json
{ "key": "service", "value": "api" }
```

- Multiple label_keys are **AND'd** (`alert/dispatch/dispatch.go:NotifyRuleMatchCheck`).
- **The same key cannot be written multiple times to OR them** (structural limitation); for OR use `attributes`' `in`, or split into multiple NotifyConfigs.
- List of selectable keys: `GET /api/n9e/event-tagkeys`; when unsure which labels an event has, confirm with the user in your reply.
- When the event labels do not contain `ident` (e.g. lost when categraf writes directly to the time-series store), routing by ident will fail to match everything, so have the user confirm the data flow.

## attributes (attribute filtering)

Attributes = event metadata, **not user-defined labels**. The supported keys are fixed:

| key | Meaning | Supported operators | value notes |
|---|---|---|---|
| `group_name` | Name of the business group the alert rule belongs to | `==` `!=` `=~` `!~` `in` `not in` | Business group name (**bound by name, breaks after a business group is renamed**) |
| `cluster` | Data source name | `==` `!=` `=~` `!~` `in` `not in` | Data source name |
| `is_recovered` | Whether it has recovered | `==` | `"true"` / `"false"` (string, not bool) |
| `rule_id` | Alert rule ID | `==` `!=` `in` `not in` | Numeric string |
| `severity` | Alert level | `==` `!=` `in` `not in` | `"1"` / `"2"` / `"3"` |
| `target_group` | Business group the monitored object (host) belongs to | `in` `not in` `=~` `!~` | Business group **ID** (not name) |

### func (operators)

| func | Meaning | How to write value |
|---|---|---|
| `==` | Exact match | `"production"` |
| `!=` | Not equal | `"test"` |
| `=~` | Regex match | `"prod-.*"` |
| `!~` | Regex non-match | `"test-.*"` |
| `in` | In the list | **Space-separated**: `"prod-01 prod-02 prod-03"`, do not use commas |
| `not in` | Not in the list | **Space-separated**: `"test-01 test-02"` |
