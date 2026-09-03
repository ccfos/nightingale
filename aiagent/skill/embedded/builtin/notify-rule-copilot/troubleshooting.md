# Notify rule known-pitfalls quick lookup + test verification

## Known-pitfalls quick lookup

| Symptom | Most likely cause | Handling |
|---|---|---|
| Test send is OK, but the real alert doesn't come out | Recipient's `contact_info.<ContactKey>` is empty → `sendtos` is empty → silently not sent | Hand off to `alert-rule-troubleshoot` flow B; this skill is only responsible for having the user check the channel's `ContactKey` and the user's contact_info |
| A group-bot channel (DingTalk/WeCom/Feishu) never sends | params didn't fill in that channel's custom parameter (`access_token`/`key`), or the token is wrong — for these channels, sending depends only on the token, unrelated to recipients | Fill in params against the `custom_params` from `list_notify_channels`; for where to get the token, see the corresponding doc link in the `reference.md` quick lookup table |
| The alert rule is saved but the notification records stay empty | The alert rule isn't associated with this notify rule (`alert_rule.notify_rule_ids` is empty / still using the old-style `notify_groups`) | Alert rule list → batch update → associate the notify rule |
| A rule suddenly stops matching after a business group is renamed | `attributes.group_name == "old-name"` is hard-bound by name | Switch to `=~` with a regex, or sync-update this rule |
| `attributes` with multiple `in` values has no effect | The value was written comma-separated `"a,b,c"` | Change to **spaces**: `"a b c"` |
| Logs explode when some of multiple NotifyConfigs fail to match | Log-level issue in the current version | Suggest the user add a "catch-all NotifyConfig" (recipes.md template F) |
| One webhook is shared by N rules, and a single point of failure blocks all rules | Head-of-line blocking | Use a dedicated channel for critical paths, prompt the user to split the channel |
| A field gets cleared after editing and saving | PATCH misused, or the frontend form's normalizeValues filtered out empty time ranges | When using PUT, **GET first, then modify, then PUT**, preserving all fields |
| A cross-midnight window (e.g. 22:00–02:00) doesn't take effect | The engine doesn't roll over to the next day | Split into `22:00–23:59` + `00:00–02:00` two segments |
| `week` is written backwards (treating 1 as Sunday) | Used the Chinese convention 1=Mon instead of ISO 0=Sun | Correct it: 0=Sunday, 1=Monday … 6=Saturday |
| `is_recovered` value-type pitfall | Written as `true` (bool) instead of `"true"` (string) | The TagFilter value is a string, must use `"true"` / `"false"` |
| Wanting OR on the same label key | Not supported structurally | Switch to `attributes`' `in`, or split into multiple NotifyConfigs |
| A name with spaces fails inside `in` | A business group name containing a space gets swallowed by space-separation | Switch to regex `=~` with escaping |
| The user has no permission to see this rule | `user_group_ids` doesn't include this user's team | Add the corresponding team ID |
| Creation returns `forbidden` | The current user is in none of the `user_group_ids` teams | Add the user's own team, or have an admin do it |
| Routing by ident fails to match everything | The event labels don't contain `ident` (e.g. lost when categraf writes directly to the time-series store) | Have the user confirm the data flow; use `GET /api/n9e/event-tagkeys` to see the actual labels |
| Can't find the "switch to new version" button in the UI | The button's location has changed several times (hidden in beta14 / moved in 8.4.x) | Upgrade to 8.5.1+, and look in the **"batch update" dialog of the alert rule list** |

## Testing and verification

### Dry-run with a real event

The semantics of `POST /api/n9e/notify-rule/test` are stronger than the "test notification" button in the UI: it **matches and actually sends using a historical event ID + the NotifyConfig you pass in**:

```
Body: {"event_ids":[<history_event_id>], "notify_config":{...}}

hisEvents = AlertHisEventGetByIds(event_ids)
for each event:
    dispatch.NotifyRuleMatchCheck(notify_config, event)   ← the real matching function
SendNotifyChannelMessage(notify_config, events)           ← the real send
```

This means:

- Take a **real historical event ID** (pick one from the historical alerts), pass in the draft NotifyConfig, and you can immediately see "whether it matches" and "what the sent message looks like" — more accurate than mocking an event out of thin air in the UI.
- On failure, the returned error can distinguish whether it's a match failure (and at which step) or a channel-call failure.
- But **it cannot verify the `notify_rule_ids` association** — whether this rule has been attached by an alert rule is a separate matter; check it in the alert_rule table / on the alert rule page.

### Minimal verification checklist after editing

Every time a notify rule is modified, have the user run these 3 confirmation steps:

1. `GET /api/n9e/notify-rule/<id>` (in-app, use `get_notify_rule_detail`) to see whether the right fields were changed.
2. `POST /api/n9e/notify-rule/test` to verify matching + sending with a relevant historical event.
3. After a real alert comes out, go to `Historical alerts → details → notification records` to see whether there is a send log for this rule.
