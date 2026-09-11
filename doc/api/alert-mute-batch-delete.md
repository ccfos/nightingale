# Batch Cleanup of Expired Mute Rules API

Cleans up leftover, already-expired alert mute rules (AlertMute) in one shot. A typical use case: after running for a long time the system has accumulated a large number of temporary mutes and you want to clear them out, either per business group or globally.

The API runs asynchronously: the server returns a success response immediately and then deletes the matching rows in batches in the background until nothing matches any more.

## Endpoint

```
DELETE /api/n9e/alert-mutes
```

- Permissions: `auth` + `admin`; administrators only.
- Content-Type: `application/json`.

## Request parameters

| Field | Type | Required | Description |
|------|------|------|------|
| timestamp | int64 | Yes | Unix timestamp in seconds. Only mute rules with `create_at < timestamp` are deleted. Read it as "delete expired mutes created before this point in time". |
| group_ids | int64[] | No | List of business group IDs. When provided, only mute rules under these business groups are cleaned up; omitting it or passing an empty array means all business groups. |

## Deletion criteria

The background batch job only deletes rows that satisfy **all** of the following:

1. `etime > 0`: the mute has an end time (permanent mutes, where `etime = 0`, are never cleaned up).
2. `etime < now`: the end time has already passed, i.e. the mute has expired.
3. `create_at < timestamp`: it was created before the `timestamp` in the request.
4. If `group_ids` was provided, `group_id IN (group_ids)` is appended as well.

In other words: this API **only cleans up expired mutes**. It never touches permanent mutes or mutes that are still in effect.

## Response

It returns as soon as the request has been accepted and the background job has been started:

```json
{
  "dat": "Alert mutes deletion started",
  "err": ""
}
```

The background job deletes in batches of 1000 rows, sleeping 100 ms between batches, until a batch returns fewer than 1000 rows. On a DB error it aborts and writes to the server log.

If a required parameter is missing, it returns 400 synchronously:

```json
{
  "err": "timestamp parameter is required"
}
```

## Examples

Clean up expired mutes under two business groups that were created more than 30 days ago:

```bash
NOW=$(date +%s)
THRESHOLD=$((NOW - 30*24*3600))

curl -X DELETE 'http://<n9e-host>/api/n9e/alert-mutes' \
  -H 'Content-Type: application/json' \
  -H 'Cookie: <admin-session>' \
  -d "{\"timestamp\": ${THRESHOLD}, \"group_ids\": [1, 2]}"
```

Globally clean up expired mutes older than 30 days across all business groups:

```bash
curl -X DELETE 'http://<n9e-host>/api/n9e/alert-mutes' \
  -H 'Content-Type: application/json' \
  -H 'Cookie: <admin-session>' \
  -d "{\"timestamp\": ${THRESHOLD}}"
```

## Notes

- The API is **asynchronous**; getting back `Alert mutes deletion started` does not mean the cleanup has finished. To confirm the outcome, search the server log for `Successfully deleted alert mutes` / `Failed to delete alert mutes`, or call the list API again and check the remaining count.
- The cleanup is irreversible. Make sure `timestamp` and `group_ids` cover the intended range before calling it.
- Permanent mutes (`etime = 0`) and mutes that expire in the future are never deleted, so the caller does not need any extra safety filtering.
