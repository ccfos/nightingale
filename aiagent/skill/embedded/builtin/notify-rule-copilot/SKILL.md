---
name: notify-rule-copilot
description: One-stop assistant for creating, editing, copying, and troubleshooting Nightingale (n9e) notify rules (notify_rule). Use it when the user asks to "create a notify rule / add a notification policy / configure how alerts are delivered / edit and adjust a notify rule / tiered notification / route by business group or label / take different actions during vs. outside working hours / don't call on recovery / fix a rule that isn't matching" — it is especially good at decomposing natural-language routing requirements into a correct NotifyConfig array. This skill focuses on the routing layer of notify rules: it does not touch the notification media themselves (→ notify-channel-copilot), does not touch message templates (→ generate-message-template), and does not investigate "why nothing was sent" (→ alert-rule-troubleshoot flow B).
tags:
  - internal
---

# Nightingale (n9e) Notify Rule Copilot

A notify rule defines how alert events are delivered: the notification method, recipients, time windows, and filter conditions. It can be referenced by multiple alert rules. This skill covers the full lifecycle of notify rules: **creating, editing, copying, routing decomposition, and troubleshooting**.

## Companion materials (load on demand with read_file; set `base` to this skill's name)

| File | Contents | When to read |
|---|---|---|
| `reference.md` | Full table of config fields: notify_configs / the shape of `params` per channel and the user input each requires / quick lookup of official doc links per medium / time_ranges / label_keys / attributes operators | When you are unsure about a field while assembling config; when you need to ask the user for channel params (access_token, etc.) and must attach a doc link |
| `recipes.md` | 6 complex routing decomposition templates (tiered / time-windowed / no phone call on recovery / route by business group / label-based canary / catch-all) + 4 complete basic examples | When the requirement is composite routing |
| `troubleshooting.md` | Quick lookup table of known pitfalls + test/verification (dry-run) checklist | When a rule does not take effect / behaves abnormally after editing |
| `http-api.md` | HTTP API paths (for A2A / external agents), the GET→modify→PUT editing pattern, direct DB edits | Only for external A2A scenarios, or when producing curl commands for the user |

---

## Scope (first determine which layer the user is changing)

Nightingale's notification pipeline is a three-layer abstraction, with one skill per layer — **do not cross wires**:

| Layer | Entity | Which skill |
|---|---|---|
| **Medium** Notify Channel | `notify_channel` table | `notify-channel-copilot` |
| **Template** Message Template | `message_template` table | `generate-message-template` |
| **Rule** Notify Rule | `notify_rule` table | **this skill** |

How to judge:

- The wording mentions "URL / Webhook address / request headers / proxy / signature / AppID / AppSecret / how to integrate X" — **medium layer**, hand off to `notify-channel-copilot`.
- The wording mentions "template / message body / fields / card color / who to @ / `{{ ... }}` variables" — **template layer**, hand off to `generate-message-template`.
- The wording mentions "who to send to / which channel for which severity / working hours / route by business group / filter by label / applicable attributes / edit an existing rule" — **rule layer**, this skill.
- The wording mentions "an event already fired but no notification was received, help me find out why" — **post-hoc diagnosis**, hand off to `alert-rule-troubleshoot` flow B. This skill is only responsible for "pulling up the correct rule configuration", not for replaying logs.

---

## Prerequisites

You are the in-app n9e AI assistant, running inside the n9e process and already authenticated as the current user. **Call the built-in tools to operate directly — do not log in, do not call the HTTP API, and do not use http_fetch to hit your own endpoints** (the HTTP flow in `http-api.md` is for external A2A agents).

---

## Mental model: NotifyRule + NotifyConfig

One `NotifyRule` is like a folder holding N `NotifyConfig`s, where **each NotifyConfig is an independent "route"** — with its own severity/time/label/attribute filters, going through its own medium, using its own template, sent to its own recipients. Multiple NotifyConfigs are **OR'd in parallel**: whichever an event matches is the route it takes, and it can match several at once and be sent through all of them.

This is exactly why "for P1, send DingTalk + phone call during working hours, and only phone call outside working hours" must be split into 3 NotifyConfigs (see `recipes.md` template B).

The two most easily misunderstood fields:

- `user_group_ids` is the **authorized teams, not the recipients** — it determines who can see/edit/reference this rule; recipients are determined by each NotifyConfig's `params.user_ids` / `params.user_group_ids`. Empty `user_group_ids` + a non-admin user → only admins can see it.
- A template only does "content rendering", not "routing decisions" — when a user wants to "if-else on severity inside the template", guide them to **split the NotifyConfig** instead.

## Core config structure

```json
{
  "name": "notify rule name",
  "description": "It is recommended to state the routing intent, e.g.: P1 all day + P2/P3 notify ops during working hours",
  "enable": true,
  "user_group_ids": [1, 2],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_ids": [1, 2], "user_group_ids": [1] },
      "severities": [1, 2, 3],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    }
  ]
}
```

Key points (full field table and per-channel `params` shapes are in `reference.md`):

- `user_group_ids` must not be empty; `notify_configs` needs at least 1 entry, with a recommended ≤5 per rule (beyond that, split into multiple rules).
- `channel_id` must be >0; use `list_notify_channels` to get real IDs, don't guess.
- The shape of `params` varies by medium: recipient-type channels fill `user_ids`/`user_group_ids`, group-bot-type channels fill `access_token` and other custom keys, and flashduty fills collaboration-space `ids` — go by the `contact_key`/`custom_params` returned by `list_notify_channels`, and **ask the user for missing information and attach the doc link** (quick lookup table in `reference.md`).
- `template_id` **can be omitted**: the tool automatically fills in that channel's default template (the one with the smallest weight); only when the user names a specific template do you use `list_message_templates` (filtered by channel `ident`) to fetch the real id. The flashduty/pagerduty channels do not need a template at all.
- `severities` must not be empty; `[1,2,3]` = all severities; to make "P1 take one route, P2/P3 take another" you must split the NotifyConfig.
- `time_ranges` left empty `[]` = effective at all times; **don't stuff in an all-day 00:00-00:00 entry**; multiple windows are OR'd; crossing midnight must be split into two segments; `week` uses 0=Sunday.
- `label_keys` / `attributes` are all **AND'd** across entries; the value of `in`/`not in` is **space-separated**.

---

## Workflow 1: Create

1. **Determine the team (user group) that receives notifications**: use `list_teams` to get `user_group_ids`. If the user named a team, or the frontend already popped a team form, use those IDs directly without asking again.
2. **Determine the notification medium and template**: use `list_notify_channels` to list enabled media, and match `channel_id` by the user's description (DingTalk/email/phone/SMS…); if you can't match, present the candidates for the user to pick. `template_id` is usually omitted per the rule above.
3. **Determine the channel params — different media require different user input**: judge the params shape for the medium from the `contact_key` / `custom_params` / `request_type` returned by `list_notify_channels` (see the "params channel parameters" comparison table in `reference.md`): for recipient-type channels (email/SMS/phone) ask "who to send to"; for group-bot-type channels (DingTalk/WeCom/Feishu) you must obtain the `access_token`/`key`; flashduty needs the collaboration-space ID; callback needs the URL. When the user has not supplied a custom param, **first run `list_notify_rule_custom_params` to look up values already filled in by existing rules** — if the user says "the same group as rule X", match by rule name and reuse it directly; if nothing is found or the user explicitly says it's a new group, **stop and ask the user — do not leave it blank or fabricate it**, and when asking attach the medium's official doc link from the `reference.md` quick lookup table, telling the user where to obtain the value. When asking for a token, also ask the user for a note (`bot_name`/`note`: which group, what purpose); if the user does not provide a note, **do not press them** — auto-generate one from the rule name / routing intent and fill it in, leaving nothing blank.
4. **Assemble config and call `create_notify_rule`**: for composite routing requirements, first decompose the NotifyConfig against `recipes.md`; pass a single JSON object as `config` (not an array), creating one rule per call. If `user_group_ids` is not included, the tool automatically pops a team-selection form, and after the user finishes selecting it resumes this creation.
5. **Report the result**: the tool returns `{id, name, user_group_ids, notify_configs_count, url}`; briefly report the rule name, associated team, and number of notify configs. **Display the rule name as an in-app link: `[<name>](<url>)`** (the url is the returned `/notification-rules/edit/<id>`), so the user can click straight to the config page to verify. After creation, you can associate it in an alert rule via `notify_rule_ids`.

## Workflow 2: Edit / Copy / Troubleshoot

1. Use `list_notify_rules` / `get_notify_rule_detail` to get the rule ID and the complete JSON of the existing config.
2. Call `update_notify_rule` (**proposal-based**: calling it shows the user a change list and pauses; after the user confirms, the system commits automatically — the confirmation step does not go through you): `id` is required, and `config` writes only the fields to change (**incremental patch**: fields not written keep their original values). Distinguish two kinds of edits:
   - **Top-level scalars** (enable/disable, rename, change description): config passes only that one field, e.g. `{"enable":false}`
   - **`notify_configs` / `user_group_ids` are wholesale replacements, not appends**: even to change a single value like `notify_configs[1].attributes[0].func`, you must build the **complete array** based on the current state from detail and pass that, otherwise the notify configs you did not mention will be dropped
3. A non-admin can only edit rules belonging to their own teams; when changing `user_group_ids`, the new list must still include a team the user belongs to. When a newly added notify config lacks `template_id`, the tool auto-fills that channel's default template (same as creation). **When adding a NotifyConfig or switching media, re-determine the params per Workflow 1 step 3** — different media have different params shapes (an old channel's `access_token` becomes `key` when moved to WeCom); ask the user for missing information and attach the doc link from the `reference.md` quick lookup table, and don't reuse old params or leave them blank.
4. Copying a rule = get the detail → rename / change the differing fields → `create_notify_rule` to create a new one.
5. When the user writes a config that will hit a pitfall (binding business group by name / using a comma in `in` / not splitting a cross-midnight window / writing `week` backwards…), **proactively correct it** against `troubleshooting.md`.
6. After editing, suggest the user dry-run to verify before rolling out: take a real historical event ID and run `POST /api/n9e/notify-rule/test` (see `troubleshooting.md`).

## Output style

1. The first sentence pins down the layer (only take it on if it's the rule layer, otherwise hand off to the corresponding skill — don't do another skill's job).
2. Map composite routing requirements directly to the closest template in `recipes.md`, give a **complete JSON draft**, and don't make the user fill in field names themselves.
3. Creation goes straight to the database via the tool; modification is proposal-based — after you call `update_notify_rule` the system shows the user a change list and waits for confirmation, so **do not restate the changes yourself before calling** (avoid double confirmation), and do not pass proposal_id/confirmed. Exception: when changing the whole `notify_configs` group as a replacement, the system text can only show the whole JSON group (truncated when too long), so before calling you may use one sentence to point out the actual field path being changed (e.g. "only changing `notify_configs[1].severities` to [1,2]"); only when the user explicitly asks for curl do you give a command template per `http-api.md`, without executing it.
4. After a successful modification, display the rule name in the same link form as Workflow 1 step 5 (the `update_notify_rule` return value also contains `url`).
