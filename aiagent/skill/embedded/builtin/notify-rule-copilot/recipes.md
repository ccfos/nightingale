# Complex semantics → NotifyConfig decomposition templates + complete examples

Map the user's natural-language routing intent to the closest template below, and give a **complete JSON draft** — don't make the user fill in field names. Replace the `<xxx_id>` placeholders with real IDs looked up via `list_notify_channels` / `list_teams`.

> **params is only illustrative for recipient-type channels**: the actual shape goes by the `contact_key`/`custom_params` returned by `list_notify_channels` (comparison table and per-medium doc links in `reference.md`). For the built-in DingTalk/WeCom/Feishu **group bot** channels, params is `{"access_token": "...", "bot_name": "<note>"}` / `{"key": "...", "bot_name": "<note>"}`, not user_ids — if the user hasn't given the token, first run `list_notify_rule_custom_params` to see whether an existing rule's value can be reused, and only if that fails ask the user and attach the doc link explaining where to get it; ask for the note (bot_name/note) in passing, and if the user doesn't give one, generate it from the rule name / routing intent, leaving nothing blank.

## Template A: tier to different channels

> "P1 alerts: phone call + DingTalk; P2/P3 alerts: DingTalk only"

```json
{
  "name": "Online tiered alert routing",
  "description": "P1: phone + DingTalk; P2/P3: DingTalk",
  "enable": true,
  "user_group_ids": [<oncall_team_id>],
  "notify_configs": [
    {
      "channel_id": <voice_channel_id>,
      "template_id": 0,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1],
      "time_ranges": []
    },
    {
      "channel_id": <dingtalk_channel_id>,
      "template_id": 0,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": []
    }
  ]
}
```

**Key decision**: write the DingTalk entry's severities as the full `[1,2,3]` (P1 is sent too, as a "paper trail" channel besides the phone call), don't write `[2,3]` and miss the DingTalk record for P1.

## Template B: different actions during working hours vs. outside working hours

> "During working hours (Mon–Fri 9 AM–6 PM) send to a DingTalk group; outside working hours, phone call + DingTalk"

```json
{
  "name": "On-call action by time window",
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

**Key decision**: weekends take the phone path 24h, so it's `week=[0,6]` all day; weekdays crossing midnight must be split into 18:00–23:59 + 00:00–09:00 two segments (you can't write 18:00–09:00, the engine looks within the same day).

## Template C: don't call on recovery

> "On alert: phone call + DingTalk; on recovery: DingTalk only, no phone call"

```json
{
  "notify_configs": [
    {
      "channel_id": <voice_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1],
      "time_ranges": [],
      "attributes": [
        { "key": "is_recovered", "func": "==", "value": "false" }
      ]
    },
    {
      "channel_id": <dingtalk_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": []
    }
  ]
}
```

**Key decision**: just add the `is_recovered == "false"` attribute to the phone entry; the DingTalk entry adds no attribute, so both alert and recovery go through. This is the standard solution.

## Template D: route to different groups by business group

> "One set of alert rules covers three business groups (zone-a/zone-b/zone-c), and each business group's alerts are pushed to the corresponding DingTalk group"

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

**Key decision**: use `attributes.group_name` rather than `label_keys`, because the business group is an ownership attribute of the alert rule, not an event label. **Note**: this will fail to match after a business group is renamed, so remind the user to sync the rule when renaming a business group; or switch to `=~` with a regex for more robustness.

## Template E: label-based canary

> "Only notify alerts with env=prod and service=payment, ignore everything else"

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

**Key decision**: the two label_keys are AND'd, so the event must carry both labels at once. If `service` does not exist in some alert events (e.g. host-unreachable alerts), this rule simply won't match — you can first use `GET /api/n9e/event-tagkeys` to see which keys events actually have.

## Template F: catch-all notification (avoid missed alerts)

> "I have N fine-grained notify rules but worry about misses, so add one more that catches all events and sends to the SRE team for a paper trail"

```json
{
  "name": "Full catch-all paper trail",
  "description": "All events go to the SRE DingTalk group, purely for a paper trail",
  "notify_configs": [
    {
      "channel_id": <sre_archive_ding_id>,
      "template_id": <minimal_template_id>,
      "params": { "user_group_ids": [<sre_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": []
    }
  ]
}
```

**Key decision**: keep one rule "with no filters at all" as a catch-all. Note that the `notify_rule_ids` on the alert-rule side must also explicitly attach this rule, otherwise it won't take effect.

---

# Complete basic examples

## Example 1: basic notify rule (notify all severities around the clock)

```json
{
  "name": "Ops team alert notification",
  "description": "Notify the ops team for alerts of all severities",
  "enable": true,
  "user_group_ids": [1],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_group_ids": [1] },
      "severities": [1, 2, 3],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    }
  ]
}
```

## Example 2: notify by severity to different channels

```json
{
  "name": "Tiered notification policy",
  "description": "Level-1 alerts notify by phone, level-2/3 alerts notify by email",
  "enable": true,
  "user_group_ids": [1, 2],
  "notify_configs": [
    {
      "channel_id": 2,
      "template_id": 3,
      "params": { "user_ids": [1, 2], "user_group_ids": [1] },
      "severities": [1],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    },
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_group_ids": [2] },
      "severities": [2, 3],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    }
  ]
}
```

## Example 3: working-hours notification + attribute filter

```json
{
  "name": "Production environment working-hours notification",
  "description": "Notify production-environment alerts only during working hours on workdays",
  "enable": true,
  "user_group_ids": [1],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_group_ids": [1] },
      "severities": [1, 2],
      "time_ranges": [
        { "week": [1, 2, 3, 4, 5], "start": "09:00", "end": "18:00" }
      ],
      "label_keys": [],
      "attributes": [
        { "key": "group_name", "func": "=~", "value": "prod-.*" }
      ]
    }
  ]
}
```

## Example 4: combined label and attribute filtering

```json
{
  "name": "API service alert notification",
  "description": "Notify unrecovered alerts related to the API service",
  "enable": true,
  "user_group_ids": [3],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_group_ids": [3] },
      "severities": [1, 2, 3],
      "time_ranges": [],
      "label_keys": [
        { "key": "service", "value": "api" }
      ],
      "attributes": [
        { "key": "is_recovered", "func": "==", "value": "false" },
        { "key": "severity", "func": "in", "value": "1 2" }
      ]
    }
  ]
}
```
