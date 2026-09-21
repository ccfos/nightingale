# Native notification channels (Jira, ...)

Native channels are notification channels whose `request_type` equals their `ident` and whose provider builds the request itself (instead of the generic HTTP URL + body template). The first one is **Jira** (`request_type=jira`). Discord, Slack and Mattermost follow the same pattern.

Differences from `request_type=http` channels:

- Templates are rendered as **plain text** (no JSON escaping), because the provider builds the JSON payload itself.
- Success is decided per third-party API (for example Jira returns `201` / `204`), with retries on `429` / `5xx` and `Retry-After` honoured (capped at 30s per wait).
- A legacy channel with the same ident but `request_type=http` (a hand-made webhook) keeps working unchanged through the callback fallback.

## Jira channel config

`request_config.jira_request_config`:

| Field | Description |
|---|---|
| `deployment_type` | `cloud` (default). Data Center is not supported yet |
| `site_url` | Jira site URL as opened in the browser, e.g. `https://your-domain.atlassian.net` (no `/rest/api`) |
| `token_type` | `scoped` (default): scoped API token or service-account token, sent through `https://api.atlassian.com/ex/jira/{cloudId}`; `classic`: classic API token, sent to `site_url` |
| `email` | Email of the account (or service account) that owns the token |
| `api_token` | API token. Supports variable references such as `{{.jira_token}}` |
| `cloud_id` | Optional. When empty it is fetched from `{site_url}/_edge/tenant_info` |
| `proxy` / `timeout` (ms) / `retry_times` / `retry_sleep` (ms) / `insecure_skip_verify` | Network settings |

## Jira params in a notify rule

`notify_configs[].params` (all values are strings; list / map values are JSON strings):

| Key | Required | Description |
|---|---|---|
| `project_key` | yes | Project key, e.g. `OPS` |
| `issue_type` | yes | Issue type name, e.g. `Bug` |
| `on_resolve` | no | `close` (default: comment and transition to a Done status), `comment`, `none` |
| `resolve_transition` | no | Transition name or id used to close; empty = pick a transition to the Done category automatically |
| `on_repeat` | no | `none` (default) or `comment`: what to do when the alert fires again while the issue is open |
| `reopen_transition` / `reopen_duration` | no | Reopen a closed issue with this transition when the alert fires again within `reopen_duration` minutes (default 1440); otherwise a new issue is created |
| `wont_fix_resolution` | no | Issues closed with this resolution are never reopened |
| `priority_map` | no | JSON, severity → priority name, e.g. `{"1":"Highest","2":"High"}` |
| `labels` | no | JSON array of extra labels |
| `tags_as_labels` | no | `true` to add alert tags as labels (spaces become `_`, at most 20) |
| `fields` | no | JSON object of extra fields, e.g. `{"customfield_10010":"{\"value\":\"prod\"}"}`; a value that is valid JSON is sent as JSON, otherwise as text |

Issues are deduplicated with the label `eventHash=<event hash>`: repeated notifications of the same alert never create a second open issue.

The message template of a Jira channel has two fields: `title` (issue summary, max 255) and `content` (description; the rendering of the recovery event is used as the recovery comment).

## Endpoints

### POST /api/n9e/notify-channel-config/check

Checks the credentials and permissions of a (possibly unsaved) channel config, item by item. Permission: same as `/notify-channel-config/test`.

Request:

```json
{ "config": { "ident": "jira", "request_type": "jira", "request_config": { "jira_request_config": { "site_url": "https://x.atlassian.net", "email": "bot@example.com", "api_token": "..." } } } }
```

Response `dat`:

```json
[
  { "name": "Cloud ID", "ok": true, "required": true, "skipped": false, "message": "a436116f-..." },
  { "name": "Credentials", "ok": true, "required": true, "skipped": false, "message": "" },
  { "name": "Account", "ok": true, "required": false, "skipped": false, "message": "Bot bot@example.com" },
  { "name": "Add comments", "ok": false, "required": false, "skipped": false, "message": "" }
]
```

`name` and hint messages are translated according to `X-Language`. Only a failed item with `required=true` means the config does not work.

### Jira dropdown data (saved channel)

Permission: logged-in user (same as the PagerDuty service list).

| Endpoint | Response `dat` |
|---|---|
| `GET /api/n9e/jira-project-list/:id` | `[{"id":"10000","key":"OPS","name":"Operations"}]` |
| `GET /api/n9e/jira-issue-type-list/:id?project=OPS` | `[{"id":"10001","name":"Bug","subtask":false}]` (sub-tasks excluded) |
| `GET /api/n9e/jira-issue-type-check/:id?project=OPS&issue_type=Bug` | `{"missing_permissions":["ADD_COMMENTS"],"required_fields":[{"fieldId":"customfield_10010","name":"Environment","required":true,"hasDefaultValue":false}]}` |
| `GET /api/n9e/jira-priority-list/:id` | `[{"id":"1","name":"Highest"}]` |

### Test endpoints

Both `POST /api/n9e/notify-channel-config/test` and `POST /api/n9e/notify-rule/test` accept `"with_recovery": true`: the events are sent once as firing and then once as recovered, which verifies the recovery flow (for Jira: comment and close).

For native channels the channel test response carries `detail` with the provider's action summary, e.g. `created OPS-12 https://x.atlassian.net/browse/OPS-12`. Each test send uses a one-off nonce in the Jira dedup key, so every test creates its own issue.
