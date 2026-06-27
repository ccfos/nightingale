# Notify rule HTTP API (for external A2A agents / when producing curl commands for the user)

> The in-app AI assistant should **not** use these endpoints (use the built-in FC tools); this file is for external agents operating programmatically, or for the in-app assistant to reference when explaining to a user "how to change it themselves with curl". Authentication: `Authorization: Bearer <token>`.

## Path A: HTTP API (programmable)

| Operation | Method | Path | Notes |
|---|---|---|---|
| List | `GET` | `/api/n9e/notify-rules` | Returns only the rules under the current user's authorized teams |
| Detail | `GET` | `/api/n9e/notify-rule/<id>` | |
| Create | `POST` | `/api/n9e/notify-rules` | **Body must be an array**, even when creating just 1: `[{...}]` |
| Update | `PUT` | `/api/n9e/notify-rule/<id>` | Body is a single object and **replaces wholesale** — you must GET first, modify, then PUT |
| Delete | `POST` | `/api/n9e/notify-rules/del` | Body: `{"ids":[1,2,3]}` |
| Test send | `POST` | `/api/n9e/notify-rule/test` | Body: `{"event_ids":[<history_event_id>], "notify_config":{...}}` |
| Fetch custom webhook params | `GET` | `/api/n9e/notify-rule/custom-params?notify_channel_id=<id>` | Used to copy another rule's custom parameters |
| Available media list | `GET` | `/api/n9e/notify-channel-configs` | Get channel_id |
| Template list | `GET` | `/api/n9e/message-templates?channel_id=<id>` | Get template_id |
| Event label keys | `GET` | `/api/n9e/event-tagkeys` | Selectable keys for label_keys |

**The correct way to edit**:

```text
1. GET /api/n9e/notify-rule/<id>      → get the complete NotifyRule JSON
2. Modify a field locally (e.g. notify_configs[1].severities = [1])
3. PUT /api/n9e/notify-rule/<id>      → submit the whole thing back
```

**Do not attempt a partial PATCH update** — PUT goes through `Update(...).Select("*")` (`models/notify_rule.go`), and fields not passed will be cleared.

## Path B: UI

- Entry: `Alert management → Notify rules`
- Suitable for: users unfamiliar with the API, with few fields, who aren't comfortable with the JSON structure.

## Path C: directly modify the DB (last resort)

- Table `notify_rule`; `notify_configs` / `pipeline_configs` / `user_group_ids` are all JSON fields.
- n9e has a `NotifyRuleCache` in memory; after a change, the cache layer auto-reloads it within ~9s, no restart needed.
- Before changing, run `mysqldump -t notify_rule > backup.sql`.
