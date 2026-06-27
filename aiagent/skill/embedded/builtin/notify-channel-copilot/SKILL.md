---
name: notify-channel-copilot
description: Helps users modify, create, or troubleshoot Nightingale (n9e) notify channels (notify_channel). Use it when the user asks to change the URL, request body, signature, headers, proxy, TLS, @-mentions, or recipient fields of channels such as DingTalk/Feishu/WeCom/email/SMS/voice/Webhook, or asks "how do I integrate platform X" or "why can't it send / why am I getting 9499 / Bad Request". This skill focuses on **the channel-layer configuration**—if the user is changing "message content/fields/rendering", switch to generate-message-template instead.
tags:
  - internal
---

# Nightingale (n9e) Notify Channel Modification

## Scope (first determine which layer the user is changing)

The Nightingale notification pipeline has three layers, each with different pain points:

| Layer | Entity | Key files | Does this skill cover it |
|---|---|---|---|
| **Channel** Notify Channel | `notify_channel` table, `NotifyChannelConfig` | `models/notify_channel.go`, `alert/sender/provider/*.go` | **Yes** |
| Message Template Notify Template | `notify_tpl` table | `models/notify_tpl.go` | No (use `generate-message-template`) |
| Notify Rule | `notify_rule` table | `models/notify_rule.go` | No |

**How to decide**:
- The user's wording mentions "URL/Webhook address/request header/timeout/proxy/signature/secret key/AppID/AppSecret/CorpID/integration"—**channel layer**, stay in this skill.
- The user's wording mentions "template/body/field/variable/rendering/title/card color"—**template layer**, switch to `generate-message-template`.
- The user's wording mentions "who to send to/recipient/subscription/filter/label matching"—**rule layer**, out of scope for this skill.

---

## Data model `NotifyChannelConfig`

`models/notify_channel.go`:

```go
type NotifyChannelConfig struct {
    ID          int64
    Name        string                // display name
    Ident       string                // channel type (the key that routes to a provider, see table below)
    Description string
    Enable      bool

    ParamConfig   *NotifyParamConfig   // user parameters: contact_key + custom params
    RequestType   string               // http | smtp | script | flashduty | pagerduty
    RequestConfig *RequestConfig       // pick the matching sub-struct by ident/request_type

    Weight int
}
```

`RequestConfig` is a union: only one field is filled depending on the channel type:

| Field | Applicable ident |
|---|---|
| `HTTPRequestConfig` | all channels that use a pure HTTP webhook (dingtalk, feishu, wecom, telegram, slackwebhook, callback, …) |
| `SMTPRequestConfig` | `email` |
| `ScriptRequestConfig` | `script` |
| `FlashDutyRequestConfig` | `flashduty` |
| `PagerDutyRequestConfig` | `pagerduty` |
| `DingtalkAppRequestConfig` | `dingtalkapp` (DingTalk app, currently not registered, see `provider/init.go`) |
| `FeishuAppRequestConfig` | `feishuapp` |
| `WecomAppRequestConfig` | `wecomapp` |

---

## Built-in ident overview (`models/user.go` + `alert/sender/provider/init.go`)

| Ident | Provider | Request type | One-liner |
|---|---|---|---|
| `dingtalk` | DingtalkProvider | HTTP | DingTalk group robot webhook |
| `wecom` | WecomProvider | HTTP | WeCom group robot webhook |
| `feishu` | simpleHTTPProvider | HTTP | Feishu group robot markdown (early) |
| `feishucard` | FeishuCardProvider | HTTP | Feishu message card (supports color switching/@-mentions) |
| `lark` | simpleHTTPProvider | HTTP | Lark (international Feishu) markdown |
| `larkcard` | LarkCardProvider | HTTP | Lark card |
| `feishuapp` | FeishuAppProvider | HTTP (App) | Feishu app robot (DM/group) |
| `wecomapp` | WecomAppProvider | HTTP (App) | WeCom self-built app |
| `telegram` | simpleHTTPProvider | HTTP | Telegram Bot |
| `discord` | simpleHTTPProvider | HTTP | Discord webhook |
| `slackbot` / `slackwebhook` | simpleHTTPProvider | HTTP | Slack |
| `mattermostbot` / `mattermostwebhook` | simpleHTTPProvider | HTTP | Mattermost |
| `jira` / `jsm_alert` | simpleHTTPProvider | HTTP | Jira / JSM ticket-type |
| `email` | EmailProvider | SMTP | Email |
| `tx-sms` | TencentSmsProvider | HTTP | Tencent Cloud SMS |
| `tx-voice` | TencentVoiceProvider | HTTP | Tencent Cloud voice |
| `ali-sms` | AliyunSmsProvider | HTTP | Alibaba Cloud SMS |
| `ali-voice` | AliyunVoiceProvider | HTTP | Alibaba Cloud voice |
| `pagerduty` | PagerDutyProvider | HTTP | PagerDuty |
| `flashduty` | FlashDutyProvider | HTTP | Flashduty integration |
| `script` | ScriptProvider | Script | shell/python script (compatible with the legacy notify.py) |
| `callback` | CallbackProvider | HTTP | generic HTTP callback |

**Registration mechanism**: `Resolve()` in `provider/registry.go`: first does an exact lookup by `Ident` → if not found, falls back by `RequestType` to a generic provider (`callback`/`script`/`email`/`flashduty`/`pagerduty`). So **a custom ident (e.g. `my-internal-webhook`) works as long as `request_type=http`, falling back to callback**.

---

## `HTTPRequestConfig` field details

```go
type HTTPRequestConfig struct {
    URL           string                 // full URL, may contain {{ ... }} template variables
    Method        string                 // GET | POST | PUT
    Headers       map[string]string      // header values may also contain template variables
    Proxy         string                 // like http://proxy:port, empty means direct connection
    Timeout       int                    // milliseconds, default 10000
    Concurrency   int                    // concurrency, default 5
    RetryTimes    int                    // default 3
    RetryInterval int                    // milliseconds, default 100
    TLS           *TLSConfig             // {Enable, CertFile, KeyFile, CAFile, SkipVerify}
    Request       RequestDetail          // {Parameters: query string, Form, Body}
}
```

**The Body field is a string**; inside it you reference the event using Go template syntax, and **the actual data fields follow the field dictionary from `generate-message-template`** (`$event.RuleName`, `$labels.ident`, `timeformat`, `unescaped`, etc.). The Body is rendered through `html/template`, so `<` and `&` get escaped—a JSON Body is usually unaffected, but **when writing HTML tags in the template you need `{{unescaped "<b>..."}}`**.

**URL / Headers / Parameters are rendered through templates the same way** (`alert/sender/provider/http_common.go:113-142 replaceVariables`). Key details:
- Only when it contains `{{` does it go through `html/template`; otherwise it is kept verbatim (`needsTemplateRendering` filters it first).
- The context is shared with the Body—the 6 variables from the next section.

Examples:
- Route the URL by severity level: `http://bot/notify?level={{$event.Severity}}`
- Inject auth into a header: `Authorization: Bearer {{$params.token}}`
- Concatenate all recipients' phone numbers into the query:
  `?ats={{range $i,$s := $sendtos}}{{if $i}},{{end}}{{$s}}{{end}}`

---

## Template context — variables you can use

All HTTP-type providers share `alert/sender/provider/http_common.go:SendHTTPRequest`. When rendering Body / URL / Headers / Parameters, these 6 variables are injected uniformly (line 34-44):

| Variable | Source | Typical use |
|---|---|---|
| `$event` | `events[0]`, the first `AlertCurEvent` of this batch | `{{$event.RuleName}}` / `{{$event.Severity}}` / `{{$event.TriggerValue}}` |
| `$events` | `[]*AlertCurEvent`, the whole batch | callback default template `{{ jsonMarshal $events }}` |
| `$sendtos` | `[]string`, resolved from each recipient's `contact_info` by the channel's `ContactKey` | `{{range $sendtos}}...{{end}}`, `{{ jsonMarshal $sendtos }}` |
| `$sendto` | `sendtos[0]`, present only when `len(sendtos)>0` | single-recipient rendering |
| `$params` | the channel's `ParamConfig.Custom.Params` | SMS template's `SignName` / `TemplateId` |
| `$tpl` | custom fields attached in the notify template | used when decoupling the template from the channel |

In addition, `$event.NotifyUsersObj` is also valid: a `[]*User` array containing the complete User objects of all recipients in this batch (`models/alert_cur_event.go:65` + `alert/dispatch/dispatch.go:971`). It is a field left over from the v6 notify.py era (`gorm:"-"`, runtime-only), and is still populated in v8. The difference from `$sendtos`:

- `$sendtos`: a plain string array, already resolved by ContactKey, **preferred for simple scenarios**.
- `$event.NotifyUsersObj`: a full User object array, used when the template needs multiple fields like Phone + Email + Username at the same time, e.g. `{{range $event.NotifyUsersObj}}{{.Phone}} {{.Username}}{{end}}`.

**Where `$sendtos` comes from** (`alert/dispatch/dispatch.go:451-541 GetNotifyConfigParams`):
Take the `user_ids` + `user_group_ids` from this notify_rule's NotifyConfig → query UserCache → read the value from each user's `contact_info` JSON by the channel's `ParamConfig.UserInfo.ContactKey` → deduplicate → build `[]string`.

⚠️ Therefore:
- `ContactKey=phone` but the user's `contact_info.phone` is empty → this person doesn't make it into sendtos.
- For a custom key like `ContactKey=dingtalk_robot_token`, the user's `contact_info` JSON must also have a key of the same name. Otherwise `$sendtos` is an empty array, and `$sendto` is simply undefined.
- This is the most common root cause of "test notification works but real alerts don't go out" (see the dedicated "Test works but real alerts don't go out" section in the debugging chapter).

---

## The "required-fields map" for each channel

Below is the minimal usable configuration for each channel—when the user asks "how do I integrate X", just tell them to fill in these few fields.

### 1) DingTalk group robot `dingtalk`

- `request_type=http`, `HTTPRequestConfig`:
  - `URL`: the group robot webhook (DingTalk admin → group settings → Group Assistant → Add Robot → Custom)
  - `Method`: `POST`
  - `Headers`: `Content-Type: application/json`
  - `Body`: JSON markdown, see `provider/dingtalk_provider.go`. The `text` field must contain a **keyword** (DingTalk group robot allowlist check: the keyword you set when creating the robot in DingTalk must appear in the message text).
- **Signing vs keyword**: the built-in DingTalk provider in n9e **only supports keyword validation**. The signing mechanism requires appending `&timestamp=&sign=` to the URL query, and there is currently no native toggle for it. If the user must use signing:
  1. Use the `callback` channel and assemble the signed URL yourself (writing it in `URL` for render-time is awkward and needs a pre-script), or
  2. Switch to the `script` channel to call a script.
  3. **Recommended wording**: "For the DingTalk robot, please switch to keyword validation; the signing mechanism is not natively supported in the open-source version."
- **@-mentions**: depend on the `at.atMobiles` / `atUserIds` arrays in the message body. The built-in DingTalk provider's request body is assembled by the message template, so **in the template use `{{batchContactsAts}}` or range over `$event.NotifyUsersObj` yourself to get `.Phone`**.

### 2) WeCom group robot `wecom`

- `URL`: the group robot webhook
- `Method`: `POST`, `Content-Type: application/json`
- **Limitations**: the group robot markdown does not support `<font color>`; @-mentions rely on `mentioned_mobile_list`/`mentioned_list`; **the new version of WeCom has retired group robots**, so newly created groups no longer have a webhook address—at that point you can only switch to the `wecomapp` self-built app.

### 3) WeCom self-built app `wecomapp`

- `request_type=http`, but uses `WecomAppRequestConfig`:
  - `CorpID`, `CorpSecret`, `AgentID` (obtained from the WeCom admin console)
  - `Proxy` / `Timeout` / `RetryTimes` / `RetrySleep`
- Uses `provider/wecomapp_provider.go`, which automatically manages access_token refresh.
- **Recipient field**: the user's `contact_info.wecom_userid` (filled in user management).

### 4) Feishu group robot (markdown) `feishu`

- Uses `simpleHTTPProvider`, template-driven. Set `HTTPRequestConfig.URL` to the group robot webhook, and `Body` to a JSON template.
- **Signature validation**: a Feishu group robot secret, like DingTalk signing, falls under "timestamp + signature embedded in the message body". **The built-in feishu provider in n9e likewise has no automatic signing.** Solutions:
  1. When creating the group robot, **do not check "Signature validation"**, and use "Custom keyword" or "IP allowlist" instead.
  2. If signing is mandatory → use a custom `script` channel.
- **Backslash Bad Request 9499**: the Feishu webhook receives JSON, and a backslash `\` is a JSON escape character. Label values like the Windows path `D:\foo` or `device="D:"` will break the JSON if they go directly into the body. **In the template use `{{$labels.path | jsonMarshal}}` to convert the string into a valid JSON string** (with quotes), or use `label_replace` at the PromQL/rule source to strip out the `\`.

### 5) Feishu card `feishucard` / Lark card `larkcard`

- Uses `FeishuCardProvider` / `LarkCardProvider`, sending Feishu v2 message cards.
- Configuration items are the same as `feishu` (URL + optional secret). `Body` is a complete `interactive` card JSON.
- **Card color switching**: a Feishu card only recognizes enumerated colors (`red / orange / yellow / green / turquoise / blue / indigo / purple / carmine / grey`); writing hex is invalid. The color is written in the `header.template` field and rendered by the template based on `IsRecovered / Severity`.
- **@-mentions**: use `<at email=...></at>` or `<at id=...></at>`; the template uses `{{batchContactsAtsInFeishuEmail $event.NotifyUsersObj}}` or `{{batchContactsAtsInFeishuId ...}}`.

### 6) Feishu self-built app `feishuapp`

- Uses `FeishuAppRequestConfig`:
  - `AppID`, `AppSecret`
  - `ReceiveIDType`: `open_id` / `user_id` / `email` / `chat_id` (determines which `contact_info.feishu_*` field is used)
- Uses `provider/feishuapp_provider.go`, which automatically manages tenant_access_token.

### 7) Email `email`

- `request_type=smtp`, `SMTPRequestConfig`:
  - `Host`, `Port`, `Username`, `Password`, `From`
  - `InsecureSkipVerify`: set to true for self-signed certificates
  - `Batch`: how many recipients to pack in at most per send (to avoid exceeding the SMTP server's per-send recipient limit)
- **Email subject template**: stored separately in the `notify_tpl` table, with ident `mailsubject` (see the `EmailSubject` constant). The fix path for "the subject contains all labels and leaks information" is to change this template, not touch the channel.
- **HTML vs plain text**: the email body template uses `text/template` (no escaping), so you can write HTML tags directly. Other IM-type channels use `html/template` and need `unescaped` as a fallback.

### 8) SMS/voice (Tencent Cloud / Alibaba Cloud) `tx-sms` / `tx-voice` / `ali-sms` / `ali-voice`

- Common structure: uses `HTTPRequestConfig`, but **the real credentials are filled via the custom parameters in `ParamConfig.Custom.Params`** (SecretId / SecretKey / SDKAppId / TemplateId / SignName, etc.).
- **Template-variable-missing error**: "test notification shows the template variable is missing a corresponding parameter value"—the order/count of SMS template parameters must be **strictly consistent** with the template approved in the provider's console.
  - Investigation path: ① provider console → find the TemplateId → see how many `${1}` `${2}` the template content has; ② the `params` array in the n9e template must be filled according to this count; ③ the field names/order must align.
- **Chinese garbled text (voice/callback)**: n9e encodes the body as UTF-8 by default, but some voice provider interfaces require GBK or url-encoded Chinese—try `{{$event.RuleName | escape}}` in the parameters, or transcode in the script channel.

### 9) PagerDuty `pagerduty`

- `PagerDutyRequestConfig`: `Proxy`, `ApiKey` (account-level API Key, not a routing key), `Timeout`, `RetryTimes`, `RetrySleep`.
- Uses the PagerDuty Events API v2. **Don't mistakenly fill the ApiKey with an Integration Key** (a common pitfall).

### 10) Flashduty `flashduty`

- `FlashDutyRequestConfig`: `IntegrationUrl` (one URL per integration), `Proxy`, `Timeout`, `RetryTimes`, `RetrySleep`.
- The integration entry point provided by Flashduty; the template is handled by the Flashduty backend, so on the n9e side it is almost a "pass-through of the events array".

### 11) Script `script`

- `ScriptRequestConfig`:
  - `ScriptType`: `python` / `shell`
  - `Script`: the script content (written to a temp file at runtime and then executed)
  - `Path`: you can also directly give the path of an existing script
  - `Timeout`: milliseconds
- **The alert data is passed in via stdin as JSON**—read stdin in the script and parse it.
- **Historical baggage**: the v6-era `notify.py` is exactly this fallback path—any weird IM/system requirement (custom signing, private protocol, complex @ logic) can ultimately be covered by script.

### 12) Callback `callback` (generic HTTP)

- Any scenario of "hit an HTTP endpoint and send the event JSON over" uses this.
- The default template of `HTTPRequestConfig.Body` is `{{ jsonMarshal $events }}` (note it is `$events`, plural—the whole batch is passed over).
- **Custom ident fallback also goes through callback**: an ident like `my-custom-webhook` works as long as `request_type=http` (see `Registry.Resolve`).

⚠️ **The most common pitfall for users upgrading from v6/v7**: `$sendtos` is already auto-injected into the template context (`http_common.go:36`), but Callback's **default Body template `{{ jsonMarshal $events }}` only outputs events** and does not reference sendtos. A downstream jenkins / auto-dial / self-healing script that wants "the recipient contact info of this notification" won't get it. The fix is to explicitly add it to the Body:

```json
{
  "events": {{ jsonMarshal $events }},
  "sendtos": {{ jsonMarshal $sendtos }}
}
```

It's not that the context lacks it—it's that the default template doesn't reference it.

---

## Three paths to modify a notify channel

### Path A: UI (recommended)
- Path: `System Config → Notification Config → Notify Channel` → select channel → Edit
- Applies to: 90% of scenarios. Change the URL, change the timeout, change the body, change the headers, add custom parameters.
- One **important pitfall**: once created, the "channel type" (i.e. `Ident`) **cannot be modified** in the UI. To switch the type you can only delete and recreate.

### Path B: HTTP API
- `POST /api/n9e/notify-channel` (create), `PUT /api/n9e/notify-channel/:id` (update), `DELETE /api/n9e/notify-channel/:id`
- Look at `center/router/router_notify_channel.go` for the exact paths and request body formats.
- Applies to: bulk changes, migration, provisioning config in CI.

### Path C: edit the DB directly (last resort)
- Table `notify_channel`, where `request_config` is a JSON field.
- Note: ① after editing, the server must reload (n9e pulls every 9 seconds, no restart needed); ② a bad JSON edit will cause Verify to fail and make the whole channel unusable—before editing, `mysqldump -t notify_channel > backup.sql`.

---

## Debugging and troubleshooting

### Check whether the channel actually sent a request

Two layers of evidence, from coarse to fine.

**Layer 1: the `notification_record` table**—one record per channel call, regardless of success or failure:

```sql
SELECT id, target, channel, status, error_message, send_time, details
FROM notification_record
WHERE channel = '<channel ident>'
ORDER BY id DESC LIMIT 20;
```

- `status=success`: it was sent; how the peer handles it is unrelated to n9e.
- `status=failure`: `error_message` usually carries the peer's HTTP status code or an error description.
- **The `details` field is `varchar(2048)`** (`models/notification_record.go:22`), so a long body gets truncated—at that point go to layer 2.

**Layer 2: the center service log**—the real log formats in `alert/sender/provider/http_common.go`:

| Level | Location | grep keyword | Content |
|---|---|---|---|
| Info | line 55 | `url:` | the rendered URL / Headers / Parameters |
| Error | line 63/69/80 | `send_http: failed` | on failure, the url + request_body + error, **visible at info level** |
| Debug | line 78 | `send http request:` | the full req + resp + response body |
| Debug | line 213 | `URL:` | the full `URL, Method, Headers, params, Body` (including template-rendered content) |

Steps:
1. Failure troubleshooting **does not require enabling debug**; just `grep 'send_http: failed' n9e-center.log` to get the url + body + error.
2. To see the full request body when sending succeeds but the content looks wrong → change the center log level to `debug` (default info), then `grep -E 'send http request:|^.*URL:' n9e-center.log`.
3. Debug logs are not affected by the 2048-byte truncation of `notification_record.details`.

### Quick reference for common errors

| Symptom | Most likely cause | Investigation |
|---|---|---|
| Feishu `{"code":9499,"msg":"Bad Request"}` | the body JSON is invalid, most likely a backslash/unescaped quote | use `{{... \| jsonMarshal}}` for all label values in the template, or confirm all quotes are escaped |
| DingTalk "keyword does not match" | the DingTalk robot has keyword validation enabled, but the keyword doesn't appear in the text | always include the keyword in the rule name/alert title, or add the robot to the allowlist |
| DingTalk/Feishu "timestamp is invalid" / "sign not match" | signature validation is enabled but n9e didn't sign automatically | switch to keyword/IP allowlist, or use the script channel |
| Email `tls: handshake failure` | the SMTP server certificate verification failed | `InsecureSkipVerify: true` or change the port (587 STARTTLS / 465 SSL) |
| `connect: i/o timeout` | the network is unreachable or a proxy is needed | fill `HTTPRequestConfig.Proxy` with the proxy address, and confirm the machine can resolve the webhook domain |
| SMS "template variable is missing a corresponding parameter value" | the count of `${1}` in the Alibaba Cloud/Tencent Cloud SMS template doesn't match the params array in n9e | compare against the template content in the provider console and fill in the missing ones in order |
| Saving a custom ident shows `unsupported channel` | the ident is not registered and request_type is not in the fallback table | `RequestType` must be one of `http/script/smtp/flashduty/pagerduty` |
| A channel tests OK but real alerts don't go out | sendtos is empty for real alerts (the recipient's contact_info is missing the field) / the notify_rule didn't select this channel | see the dedicated "Test works but real alerts don't go out" section below |

### Test works but real alerts don't go out

A common problem. The root cause is **the source of sendtos differs between a test and a real alert**:

- **The test button**: `POST /notify-rule/test` → the tester fills the recipients directly in the UI form, and sendtos comes from the values filled in the form. As long as the channel's own URL/Body/Headers are fine, it can send.
- **Real alerts**: go through `alert/dispatch/dispatch.go GetNotifyConfigParams`, taking the notify_rule's `user_ids` / `user_group_ids` → query users → read the field from `contact_info` by `ContactKey` → build sendtos. **A gap anywhere makes sendtos empty**, and this path silently sends nothing.

Three investigation steps:

1. Look at the recipient scope of this notify_rule config:
   ```sql
   SELECT user_ids, user_group_ids FROM notify_rule WHERE id=<id>;
   ```
2. Pull out these users and check whether the field corresponding to ContactKey is empty:
   ```sql
   SELECT id, username, contact_info FROM users WHERE id IN (...);
   ```
   When ContactKey is `phone`, look at `contact_info.phone`; for a custom key (e.g. `dingtalk_robot_token`), read the value under the same name.
3. For the business-group / team case, also query the team membership table:
   ```sql
   SELECT user_id FROM team_user WHERE team_id=<id>;
   ```

If any layer is empty → sendtos is empty → real alerts don't send on this path, but the test sends because the form filled in the values.

### End-to-end verification steps

1. **The "Test" button on the channel page**: the backend is `POST /notify-rule/test` (`center/router/router_notify_rule.go:142, 172-264`), which directly calls `Provider.Notify` to **actually send one message**—the recipient/title/body filled in the form really go through the webhook out to the group.
   Note this step **does not invoke the sendtos resolution logic**; it sends whatever is filled in the form. If it can send, it means this channel's URL / Headers / Body template / network / credentials / signature are all fine; if it can't send, the channel's own pipeline has a problem.
   (There is **no** `Provider.Check` method in the code; don't mislead the user in docs or speech into looking for it.)
2. Test passes but still can't send → go to the "Test works but real alerts don't go out" section above to investigate sendtos.
3. Real alerts go out but the content is wrong → go to layer 2 of the "Check whether the channel actually sent a request" section above, enable debug log, and capture the full request body.

---

## The standard procedure for "adding/copying a channel"

When the user asks "I want to integrate Slack / Feishu with signing / an internal HTTP system", give them this template:

1. **Choose the ident**:
   - Common public platforms (Slack/Discord/Telegram/Lark/Jira) → use the built-in ident.
   - A private system/self-built HTTP service → pick any ident (e.g. `my-internal-bot`), `request_type=http` is enough.
   - Complex signing/state machine/encoding → pick your own ident, `request_type=script`.

2. **Fill in `RequestConfig`**: HTTP-type channels fill `HTTPRequestConfig`, at minimum `URL` + `Method` + `Headers.Content-Type` + `Body`.

3. **The minimal skeleton of the `Body` template** (using DingTalk markdown as an example):

   ```json
   {
     "msgtype": "markdown",
     "markdown": {
       "title": "{{$event.RuleName}}",
       "text": "#### {{if $event.IsRecovered}}Recovered{{else}}Alerting{{end}}: {{$event.RuleName}}\n- Object: {{$event.TargetIdent}}\n- Trigger value: {{$event.TriggerValue}}\n- Time: {{timeformat $event.TriggerTime}}"
     },
     "at": {
       "atMobiles": [{{range $i, $u := $event.NotifyUsersObj}}{{if $i}},{{end}}"{{$u.Phone}}"{{end}}],
       "isAtAll": false
     }
   }
   ```

4. **The recipient field** `ParamConfig.UserInfo.ContactKey`:
   - DingTalk group robot / Feishu group robot → leave empty (group-level, doesn't pick a person)
   - DingTalk app / Feishu app / WeCom app → use `dingtalk_userid` / `feishu_userid` / `wecom_userid`
   - Email → `email`
   - SMS/voice → `phone`
   - A fully custom contact (e.g. `slack_user_id`) → pick your own key name and fill the value into the `contact_info` JSON in the `user` table.

5. **Test → Save → select it in the notify rule.**

---

## Output style

When the user asks "how do I change X", answer using this routine:

1. In one sentence, point out **which layer to change** (channel/template/rule). If the user is actually asking about the template/rule, first redirect them to the correct skill.
2. Give **field-level instructions**: touch a precise path like `notify_channel.request_config.http_request_config.headers`, not a vague "go change it in the admin console".
3. If a built-in ident can be used, **prefer the built-in** (feishucard is better than a hand-written feishu webhook).
4. For known pitfalls like signing/special encoding/backslashes, **say directly that you've "hit this before"** and give the solution—don't make the user go through trial and error.
5. Throughout, **never change the database or call the API on the user's behalf**—only tell them which field to change and how to verify.
