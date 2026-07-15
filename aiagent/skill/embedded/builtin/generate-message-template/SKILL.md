---
name: generate-message-template
description: Generate or modify Nightingale (n9e) alert notification message templates. Use when the user asks to write a notification template, change the message format, add hostname/recovery value/severity, or create DingTalk/Feishu/Lark/email/SMS/voice templates.
tags:
  - internal
---

# Nightingale (n9e) Notification Message Template Generation

Nightingale message templates use Go `text/template` / `html/template` syntax (email uses `text/template`, others use `html/template` and then escape). Users edit them on the "Notification Management → Message Templates" page; once saved they are referenced by notification rules, and when an alert fires the variables are substituted with rendered data and sent to each channel.

This skill focuses on **writing/modifying the template fragment itself**, and does not cover creating notification rules or channel configuration.

---

## Render Context

The `renderData` passed in when the backend executes:

| key | type | description |
|---|---|---|
| `.events` | `[]*AlertCurEvent` | The list of alert events in the current batch, usually just 1 |
| `.domain` | `string` | The n9e site URL, used to build redirect links |

**Automatically injected shorthands** (no need to write by hand):

```gotemplate
{{ $events := .events }}
{{ $event := index $events 0 }}
{{ $labels := $event.TagsMap }}
{{ $value := $event.TriggerValue }}
```

So inside the template you can directly use `$event.xxx`, `$labels.agent_hostname`, and `$value`.

---

## Available `$event` Fields (`*AlertCurEvent`)

### Common Fields
| field | type | description |
|---|---|---|
| `.Id` | int64 | Alert event ID (used to build the redirect link) |
| `.RuleId` | int64 | Alert rule ID |
| `.RuleName` | string | Rule name |
| `.RuleNote` | string | Rule note |
| `.Severity` | int | Alert severity: 1=Critical, 2=Warning, 3=Info |
| `.PromQl` | string | Alert trigger expression |
| `.RuleAlgo` | string | Rule algorithm type |
| `.TriggerTime` | int64 | Trigger time, unix seconds |
| `.TriggerValue` | string | Metric value at trigger time (already a string) |
| `.FirstTriggerTime` | int64 | First abnormal time (for consecutive alerts) |
| `.LastEvalTime` | int64 | Most recent evaluation time, **used as the recovery time on recovery** |
| `.IsRecovered` | bool | Whether it has recovered |
| `.NotifyCurNumber` | int | Which notification number this send is |
| `.TargetIdent` | string | Monitored object (usually agent_hostname) |
| `.TargetNote` | string | Object note |
| `.GroupId` / `.GroupName` | int64 / string | Business group |
| `.Cluster` | string | Data source cluster name |
| `.Cate` | string | Data source type (prometheus / host / mysql / ...) |
| `.RunbookUrl` | string | Runbook link |

### Labels / Annotations / Trigger Value Objects
| field | type | description |
|---|---|---|
| `.TagsMap` | `map[string]string` | Event labels, preferred: `{{$labels.agent_hostname}}` |
| `.TagsJSON` | `[]string` | A string array like `["k=v", ...]` (commonly used in early templates) |
| `.AnnotationsJSON` | `map[string]string` | Annotations, commonly `.AnnotationsJSON.recovery_value` |
| `.TriggerValuesJson.ValuesWithUnit` | map | Trigger values with units (multi-value scenarios) |

### Notification Objects
| field | type | description |
|---|---|---|
| `.NotifyUsersObj` | `[]*User` | The user objects to notify this time (with Username/Nickname/Phone/Email) |
| `.NotifyGroupsObj` | `[]*UserGroup` | Associated user groups |

> User objects are commonly used in DingTalk/Feishu/Lark templates to build @-mentions; see the `ats` family of helpers below.

---

## Available Helper Functions (`tplx.TemplateFuncMap`)

### Time
- `timeformat <unix>` — defaults to `"2006-01-02 15:04:05"`; pass a second argument to override: `{{timeformat $event.TriggerTime "15:04:05"}}`
- `timestamp` — current time string
- `now.Unix` — current unix seconds (`now` is a Go template builtin)
- `humanizeDuration <sec>` — `"3m15s"`
- `humanizeDurationInterface <interface>` — same as above but accepts an interface
- `toTime <unix>` — returns a `time.Time`, can be chained
- `parseDuration <"5m">` — returns a `time.Duration`

### Numbers / Formatting
- `formatDecimal <v> <n>` — keep n decimal places (`{{formatDecimal $event.TriggerValue 2}}`)
- `humanize <v>` — K/M/G units (base 1000, SI)
- `humanize1024 <v>` — Ki/Mi/Gi (base 1024)
- `humanizePercentage <v>` / `humanizePercentageH <v>` — percentage
- `add / sub / mul / div <a> <b>` — arithmetic
- `printf "%.2f" <v>` — Go `fmt.Sprintf`

### Strings
- `toUpper / toLower / title` — case conversion
- `contains <s> <sub>` — substring
- `match <regex> <s>` — regex match (bool)
- `reReplaceAll <regex> <repl> <s>` — regex replace
- `split <s> <sep>` / `join <slice> <sep>`
- `stripPort <host:port>` / `stripDomain <host.domain>`
- `b64enc` / `b64dec` — base64

### Links / Escaping
- `escape <s>` — URL path escape
- `unescaped <s>` — output raw HTML (no escaping)
- `safeHtml <s>` — same as above
- `urlconvert <s>`

### Labels / Trigger Values
- `label <key> <labelMap>` / `value <key> <m>` / `strvalue <v>`
- `first <slice>` — take the first
- `tagsMapToStr <map>` — join labels into `k=v,k=v`
- `sortByLabel <items> <key>` — sort by label

### @-mention (DingTalk/Feishu/Lark only)
- `ats <users> <platform>` — generate an at fragment
- `batchContactsAts <contacts> <platform>` — batch at
- `batchContactsAtsInFeishuEmail <contacts>` / `batchContactsAtsInFeishuId <contacts>` — Feishu only
- `batchContactsJoinComma <contacts>` / `batchContactsJsonMarshal <contacts>`
- `mappingAndJoin <map> <kvSep> <itemSep>`

### Others
- `jsonMarshal <v>` — serialize to a JSON string
- `mapDifference <a> <b>` — set difference

---

## Syntax Differences Across Channels (`notify_channel_ident`)

A message template is bound to a specific channel; the channel ident determines the text engine and escaping behavior:

| Ident | Engine | Description |
|---|---|---|
| `email` | `text/template` | No re-escaping; HTML templates can write tags directly |
| `slackwebhook` / `slackbot` | `html/template` | After rendering, `"` / `\n` are escaped and wrapped into `template.HTML`; usually written in Markdown |
| Others (`dingtalk` / `feishu` / `feishucard` / `larkcard` / `wecom` / `tx-sms` / `ali-voice` …) | `html/template` | After rendering, `"` `\n` `\r` are JSON-string-escaped, suitable for stuffing directly into a webhook payload |

> Important: `html/template` automatically escapes `<` `>` `&`. **Where you don't want escaping, use `{{unescaped "…"}}` or `{{safeHtml .X}}`**, otherwise DingTalk will render a literal `&lt;`.

---

## Template Writing Workflow

1. **Confirm the channel ident**: when the user says "DingTalk" it is `dingtalk`, "Feishu card" is `feishucard`, "email" is `email`. Different channels differ significantly in style.
2. **Decide whether to render the recovery state**: by default you should split into two branches on `$event.IsRecovered`. Only SMS/voice may omit it.
3. **Pick fields**:
   - For labels, prefer `$labels.<key>` rather than `$event.TagsJSON`.
   - For the trigger value use `$event.TriggerValue`; for two decimal places use `{{formatDecimal $event.TriggerValue 2}}`.
   - Always pass timestamps through `timeformat`.
   - Build the redirect link as `{{.domain}}/share/alert-his-events/{{$event.Id}}`.
4. **Severity display**:
   - Number: `{{$event.Severity}}`
   - Worded: `{{if eq $event.Severity 1}}Critical{{else if eq $event.Severity 2}}Warning{{else}}Info{{end}}`
   - English: `Critical / Warning / Info`
5. **@-mention (DingTalk/Feishu)**:
   - DingTalk `@everyone`: add a line `@all` at the end of the template (DingTalk matches by splitting on spaces). To at an exact phone number: `{{range $event.NotifyUsersObj}}@{{.Phone}} {{end}}`.
   - Feishu card: use `{{batchContactsAtsInFeishuEmail $event.NotifyUsersObj}}` to build `<at email=...></at>` fragments.
6. **Output**: wrap the template body in a markdown ``` ```gotemplate ``` code block, and at the end provide a **variable/function explanation** with each non-trivial variable/function on its own line.

---

## Output Format

When replying to the user:

1. One or two sentences of introduction explaining what the template is for and which channel it fits.
2. ```gotemplate
   <template content>
   ```
3. `**Variable explanation**`: list the non-trivial fields and functions used in the template (such as `$event.TargetIdent`, `formatDecimal`, `timeformat`), one per line.
4. If the user's request is ambiguous (for example "add the hostname" — is it `target_ident` or some tag), **state directly in the introduction what assumption you made**, rather than asking back.

The language follows the user's input (use Chinese if the input is Chinese).

---

## Built-in Reference Templates (Starting Points for Customization)

### DingTalk markdown (full version)

```gotemplate
#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$duration := sub now.Unix $event.FirstTriggerTime}}{{if $event.IsRecovered}}{{$duration = sub $event.LastEvalTime $event.FirstTriggerTime}}{{end}}
- **Alert Level**: S{{$event.Severity}}
{{- if $event.RuleNote}}
- **Rule Note**: {{$event.RuleNote}}
{{- end}}
{{- if $event.TargetIdent}}
- **Target**: {{$event.TargetIdent}}
{{- end}}
{{- if not $event.IsRecovered}}
- **Trigger Value**: {{$event.TriggerValue}}
- **Trigger Time**: {{timeformat $event.TriggerTime}}
- **Duration**: {{humanizeDurationInterface $duration}}
{{- else}}
- **Recovery Time**: {{timeformat $event.LastEvalTime}}
- **Duration**: {{humanizeDurationInterface $duration}}
{{- end}}
- **Event Tags**:
{{- range $k, $v := $labels}}
{{- if ne $k "rulename"}}
    - {{$k}}: {{$v}}
{{- end}}
{{- end}}
[Event Detail]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [Mute 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}})
```

### Feishu card (concise version)

```gotemplate
{{- if $event.IsRecovered}}
**Severity/State:** S{{$event.Severity}} Recovered
**Alert Name:** {{$event.RuleName}}
**Event Tags:** {{$event.TagsJSON}}
**Recovery Time:** {{timeformat $event.LastEvalTime}}
{{- else}}
**Severity/State:** S{{$event.Severity}} Triggered
**Alert Name:** {{$event.RuleName}}
**Event Tags:** {{$event.TagsJSON}}
**Trigger Time:** {{timeformat $event.TriggerTime}}
**Trigger Value:** {{$event.TriggerValue}}
{{- if $event.RuleNote}}
**Alert Description:** {{$event.RuleNote}}
{{- end}}
{{- end}}
```

### SMS / Voice (minimal)

```gotemplate
Severity/State: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}} Rule: {{$event.RuleName}} Target: {{$event.TargetIdent}}
```

---

## Typical Customization Scenarios

### 1) "Add the hostname to the DingTalk template"

```gotemplate
- **Host**: {{$event.TargetIdent}}
{{- if $labels.ip}}
- **IP**: {{$labels.ip}}
{{- end}}
```

> Note: `target_ident` is usually the hostname; if you need the IP, prefer the labels `ip`, `instance`, `host`.

### 2) "Keep trigger_value to two decimal places"

```gotemplate
- **Trigger Value**: {{formatDecimal $event.TriggerValue 2}}
```

> `TriggerValue` is itself a string; `formatDecimal` first converts it to a float and then formats it, and returns non-numeric values unchanged.

### 3) "@-mention the alert recipients at the end of the DingTalk template"

```gotemplate
...template body...

{{- range $event.NotifyUsersObj}}@{{.Phone}} {{end}}
```

> DingTalk identifies @-mentioned users by "space + phone number". Or uniformly use `@all`.

### 4) "Show the recovery value on recovery"

```gotemplate
{{- if $event.IsRecovered}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **Recovery Value**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **Recovery Time**: {{timeformat $event.LastEvalTime}}
{{- end}}
```

> The recovery value is written into `AnnotationsJSON.recovery_value` by the alert engine on recovery, and **only exists when there is a recovery value**, so guard it with `if`.

### 5) "Use Chinese for the alert severity"

```gotemplate
- **Severity**: {{if eq $event.Severity 1}}Critical{{else if eq $event.Severity 2}}Warning{{else}}Info{{end}}
```

### 6) "Only send alerts for production-environment machines"

The template itself does not do filtering — filtering should go into the `attributes` / `label_keys` of the **notification rule**. The template is only responsible for display. If the user asks about this, point it out.

### 7) "Color by state/severity"

DingTalk/email use `<font color>`, Feishu cards use the `template` field to fill in a color keyword:

```gotemplate
{{/* DingTalk markdown: prepend emoji + color to the title */}}
#### {{if $event.IsRecovered}}<font color="#008800">✅ {{$event.RuleName}}</font>{{else}}<font color="#FF0000">🚨 {{$event.RuleName}}</font>{{end}}

{{/* The template field for Feishu feishucard (determines the card header color block) */}}
{{if $event.IsRecovered}}turquoise{{else}}{{if eq $event.Severity 1}}red{{else if eq $event.Severity 2}}orange{{else}}grey{{end}}{{end}}
```

> Feishu cards only recognize enumerated colors: `red / orange / yellow / green / turquoise / blue / indigo / purple / carmine / grey`; hex color codes are ignored.

### 8) "Show the alert duration"

```gotemplate
{{$duration := sub now.Unix $event.FirstTriggerTime}}
{{- if $event.IsRecovered}}{{$duration = sub $event.LastEvalTime $event.FirstTriggerTime}}{{end}}
- **Duration**: {{humanizeDurationInterface $duration}}
```

> Triggered state: `current time - FirstTriggerTime`; recovered state: `LastEvalTime - FirstTriggerTime`. `humanizeDurationInterface` outputs a human-readable form like `1h3m5s`.

### 9) "Embed a tag value in a URL, avoiding spaces/special characters breaking the link"

```gotemplate
[View Dashboard]({{.domain}}/dashboards/123?ident={{urlquery $event.TargetIdent}}&host={{urlquery (index $labels "host")}})
[Mute 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}})
```

> Stuffing `{{$event.TargetIdent}}` directly into a URL will, when it contains spaces, Chinese, or `&`, get double-escaped on the IM side (typical symptom: `&` becomes `&amp;` and the link won't open). **Wrap every variable that goes into a URL query in `urlquery`.**

### 10) "Tag keys containing hyphens/dots/Chinese — use index to fetch"

```gotemplate
- **Application**: {{index $labels "app-name"}}
- **K8s Cluster**: {{index $labels "k8s.io/cluster"}}
- **Business Dashboard**: {{index $event.AnnotationsJSON "dashboard_url"}}
```

> `$labels.app-name` will be parsed by Go template as "`app` minus `name`" and is bound to fail. As long as a key **is not pure alphanumerics plus underscore**, always fetch it with `index`; the same goes for Annotations.

### 11) "Fallback for abnormal values (+Inf / NaN)"

```gotemplate
- **Trigger Value**: {{if or (eq $event.TriggerValue "+Inf") (eq $event.TriggerValue "NaN")}}N/A{{else}}{{formatDecimal $event.TriggerValue 2}}{{end}}
```

> A `/0` in PromQL returns `+Inf`, and an aggregation over missing data returns `NaN`. Rendered directly into Feishu/DingTalk they appear as literal `+Inf`, which looks like a bug.

### 12) "In Edge mode the event Id=0 — degrade the redirect link"

```gotemplate
{{if gt $event.Id 0}}
[Event Detail]({{.domain}}/share/alert-his-events/{{$event.Id}})
{{else}}
(Edge-mode event — view details on the central server)
{{end}}
```

> Known issue: in Edge mode the event Id is written asynchronously, so at render time it is 0, and the redirect link would land on `/alert-his-events/0` (404). Add a `gt $event.Id 0` guard at the template layer.

### 13) "Merge and display multiple alerts"

```gotemplate
{{range $i, $e := .events}}
{{- if $i}}
---
{{end -}}
- Rule: {{$e.RuleName}}
- Target: {{$e.TargetIdent}}
- Trigger Value: {{$e.TriggerValue}}
- Time: {{timeformat $e.TriggerTime}}
{{end}}
```

> In the default scenario one notification has exactly one event, and the template renders with `$event = index .events 0`. When one notification aggregates multiple events (subscription aggregation, batch send) you must `range .events` to expand them.

---

## Key Considerations

1. **`html/template` HTML-escapes**. Tags such as `<font>` and `<at>` in DingTalk/Feishu/Lark **must be wrapped in `unescaped` or placed in content where Go template recognizes them itself**. Email uses `text/template` and is not subject to this.
2. **`TriggerValue` is a string**: to compare magnitudes directly use `parseDuration`/custom logic; for normal display-only purposes leave it as is.
3. **Do not manually write headers like `{{$events := .events}}`** — the system injects them automatically.
4. **Don't drop the recovery branch**: rules with `NotifyRecovered=1` reuse the same template to send the recovery message.
5. **Hyphens/dots in tag keys**: `$labels.agent_hostname` works, but `$labels.app-name` does not — use `index $labels "app-name"`.
6. **All time fields are unix seconds** (`int64`); don't use `{{$event.TriggerTime}}` directly as text — use `timeformat` or `toTime`.
7. **Merging multiple alerts**: `.events` may have multiple entries; the default DingTalk/Feishu templates only render entry 0 (`$event`); if the user wants a batch display, use `{{range .events}}`.
8. **Use `.domain` for links**: don't hardcode `http://localhost:17000`, otherwise switching environments breaks it.

---

## Common Mistakes

- ❌ `{{$event.TriggerValue | printf "%.2f"}}` — for `printf`, the first argument is the format, and `TriggerValue` is a string.
- ✅ `{{formatDecimal $event.TriggerValue 2}}`

- ❌ Displaying `{{$event.TriggerTime}}` directly — it outputs unix seconds.
- ✅ `{{timeformat $event.TriggerTime}}`

- ❌ Writing `<font color="red">` in DingTalk without escaping — `html/template` will escape it into `&lt;font&gt;`.
- ✅ Put it inside a conditional branch (Go template's `{{if}}` branch literals do not get the inner tags HTML-escaped), or wrap the whole thing in `{{unescaped "…"}}`. Nightingale's official DingTalk template writes `<font>` directly and it works, because the outer area is a plain-text section (not inside an attribute value) — just follow the official sample.

- ❌ Forgetting to split the recovery branch — the trigger time in a recovery notification is misleading.
- ✅ Wrap all dynamic content first with `{{if $event.IsRecovered}}…{{else}}…{{end}}`.

- ❌ `{{if .IsRecoverd}}` — a missing `e` in the spelling. Go template **silently takes the else branch** for a nonexistent field (without erroring), so the consequence is that the recovery notification sends the triggered-state copy, and it is very hard to track down.
- ✅ `{{if $event.IsRecovered}}`. Similar spelling traps: `Resoverd`, `Recoverd`, `recovered` (lowercase) all fail.

- ❌ `{{if lt $value 10}}` or `{{if gt $event.TriggerValue 80}}` — `TriggerValue` is a string; comparing with `lt`/`gt` against a number will report `incompatible types`, or be interpreted as a string lexicographic comparison and yield a wrong result. The current version **has no built-in `toFloat` helper**. Put numeric threshold checks back into the alert rule's condition expression, and keep the template layer for display only.

- ❌ Building a URL with `?ident={{$event.TargetIdent}}` directly — a TargetIdent containing spaces/Chinese/`&` will break the link.
- ✅ `?ident={{urlquery $event.TargetIdent}}`, **pass every variable that goes into a URL query through `urlquery`**.

- ❌ `{{$labels.app-name}}` or `{{$labels.k8s.io/cluster}}` — keys containing hyphens/dots/slashes fail to parse.
- ✅ `{{index $labels "app-name"}}`, `{{index $labels "k8s.io/cluster"}}`.
