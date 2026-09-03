# Subscription rule HTTP API (for external A2A agents / when providing the user with curl commands)

> The in-app AI assistant **must not** use these endpoints (use the built-in FC tools). Authentication: `Authorization: Bearer <token>`. Route definitions are in `center/router/router.go`.

| Operation | Method | Path | Notes |
|---|---|---|---|
| List (across business groups) | `GET` | `/api/n9e/busi-groups/alert-subscribes` | Subscriptions under the business groups visible to the current user |
| List (single business group) | `GET` | `/api/n9e/busi-group/:id/alert-subscribes` | |
| Detail | `GET` | `/api/n9e/alert-subscribe/:sid` | |
| Create | `POST` | `/api/n9e/busi-group/:id/alert-subscribes` | Body is a **single** AlertSubscribe JSON object; group_id is taken from the URL |
| Update | `PUT` | `/api/n9e/busi-group/:id/alert-subscribes` | Body is an **array** `[{...}]` (opposite of create); updated per an explicit field list, but it's still recommended to GET the detail first, modify the complete object, then PUT |
| Delete | `DELETE` | `/api/n9e/busi-group/:id/alert-subscribes` | Body: `{"ids":[1,2,3]}` |
| Tryrun | `POST` | `/api/n9e/alert-subscribe/alert-subscribes-tryrun` | Body: `{"event_id":<historical event ID>,"config":{...subscription draft...}}`; validates the match gate by gate, and the new version even runs the notification rule's real test send — **after editing, tryrun first, then save** |

Permissions: create/update/delete require business group read-write permission (bgrw) + the corresponding `/alert-subscribes/*` menu permission; listing requires only read-only.

Directly editing the DB (last resort): table `alert_subscribe`, where `tags`/`busi_groups`/`webhooks`/`extra_config`/`notify_rule_ids` etc. are JSON/serialized fields; the in-memory cache auto-reloads in ~9s, no restart needed; back up before changing.
