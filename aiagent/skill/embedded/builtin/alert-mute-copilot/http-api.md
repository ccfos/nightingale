# Mute rule HTTP API (for external A2A agents / when giving the user curl commands)

> The in-app AI assistant should **not** use these endpoints (use the built-in FC tools). Authentication: `Authorization: Bearer <token>`. Route definitions are in `center/router/router.go`.

| Operation | Method | Path | Notes |
|---|---|---|---|
| List (across business groups) | `GET` | `/api/n9e/busi-groups/alert-mutes` | Mutes visible to the current user; supports `query` search, pagination, and `expired=1` to query expired ones |
| List (single business group) | `GET` | `/api/n9e/busi-group/:id/alert-mutes` | |
| Detail | `GET` | `/api/n9e/busi-group/:id/alert-mute/:amid` | |
| Create | `POST` | `/api/n9e/busi-group/:id/alert-mutes` | Body is a **single** AlertMute JSON object |
| Update | `PUT` | `/api/n9e/busi-group/:id/alert-mute/:amid` | Single-object body, **replaced wholesale**—GET first, then modify, then PUT |
| Bulk field change | `PUT` | `/api/n9e/busi-group/:id/alert-mutes/fields` | Body: `{"ids":[...],"fields":{...}}`, suitable for bulk disable/extend |
| Delete | `DELETE` | `/api/n9e/busi-group/:id/alert-mutes` | Body: `{"ids":[1,2,3]}` |
| Preview hit events | `POST` | `/api/n9e/busi-group/:id/alert-mutes/preview` | Preview the active events a mute draft would hit; **run this before creating a large-scope mute** |
| Trial run | `POST` | `/api/n9e/alert-mute-tryrun` | Do a match trial run with a rule draft |
| Bulk cleanup of expired mutes | `DELETE` | `/api/n9e/alert-mutes` | **Requires admin**; asynchronously cleans up expired fixed-period mutes in the background (periodic mutes are not cleaned up) |

Permissions: create/update/delete require business-group read-write permission (bgrw) + the corresponding `/alert-mutes/*` menu permission; listing only requires read-only.

Directly modifying the DB (last resort): table `alert_mute`; `tags`/`periodic_mutes`/`severities`/`datasource_ids` are JSON/serialized fields; the in-memory cache reloads automatically in ~9s, no restart needed; back up before changing.
