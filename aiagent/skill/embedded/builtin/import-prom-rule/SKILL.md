---
name: import-prom-rule
description: |
  **Bulk import of a Prometheus alert rule YAML file** (create a whole set of rules at once). Dedicated to handling a remote URL or local YAML text, automatically parsing the three formats `groups` / a plain `rules` array / a single rule.
  ⚠️ **Do not use this skill for single-rule creation** — when the user describes a single alert requirement in natural language, use create-alert-rule instead.
  Triggers: import / import / bulk / URL / .yml file / .yaml file / awesome-prometheus-alerts / node-exporter.yml / prometheus rule file.
examples:
  - "Help me import https://raw.githubusercontent.com/.../node-exporter.yml"
  - "Create all the alerts in this yaml"
  - "Import the mysql file from awesome-prometheus-alerts"
  - "Bulk-create a set of redis alert rules from this file ..."
builtin_tools:
  - http_fetch
  - preview_prom_rule_yaml
  - import_prom_rule_yaml
  - list_busi_groups
  - list_datasources
tags:
  - export
---

# Skill: Import Prometheus Alert Rules

## Overview

Bulk-create Prometheus official rule YAML (in any of the three forms: with `groups`, a plain `rules` array, or a single rule) into n9e. Typical sources:

- A remote URL (e.g. awesome-prometheus-alerts on GitHub raw)
- YAML text pasted directly by the user

The workflow is a fixed four steps: **fetch YAML to a file → choose business group + datasource → preview → write to DB**.

⚠️ **Core constraint: the YAML file does not enter the LLM context**. `http_fetch` with `save_to_file=true` writes the content to a temporary file, and the subsequent `preview_prom_rule_yaml` and `import_prom_rule_yaml` both read that path via `payload_file`. The file size can range from a few KB to a few MB; letting it enter the prompt would significantly slow things down, increase cost, and make it easy for the LLM to truncate or rewrite it.

## Available Tools

| Tool | Purpose |
|---|---|
| `http_fetch` | GET a public URL. **When `save_to_file=true`, writes the body to a temporary file; the return contains only `file_path`, not the body**. Only http/https; automatically rejects intranet/loopback addresses |
| `preview_prom_rule_yaml` | Parse the YAML without writing to the DB. **Prefer the `payload_file` argument** (the file_path returned by http_fetch) to avoid large files entering the context. Returns each rule's name/severity/prom_ql etc. for the user to confirm |
| `import_prom_rule_yaml` | Parse the YAML and bulk-write to the DB by business group + datasource. **Prefer `payload_file`**. Returns each rule's id or error |
| `list_busi_groups` | List visible business groups so the user can pick a `group_id` |
| `list_datasources` | List datasources, filtered by `plugin_type=prometheus` |

## Execution Steps

### Step 1: Fetch the YAML to a temporary file

**User gave a URL** — always include `save_to_file=true`:

```
http_fetch(url="https://.../node-exporter.yml", save_to_file=true)
```

The return is:
```json
{"status_code":200,"content_type":"text/plain","size":8731,"truncated":false,
 "file_path":"/var/folders/.../n9e-aiagent-fetch-xxx.yml"}
```

Remember the `file_path`; the last two steps both need it.

**User pasted YAML text directly** — skip http_fetch and pass the text directly as the `payload` argument of the subsequent tools (in this case the YAML is already in the context anyway, so there is no point writing it to disk).

**Error handling**:
- `truncated=true` means it exceeds `max_bytes` (default 1 MiB). Call `http_fetch(url=..., save_to_file=true, max_bytes=8388608)` again (8 MiB is the hard upper limit).
- `status_code` not 2xx: tell the user the status code and ask whether to try another URL.
- Errors like "blocked: ... resolves to non-public address": the user gave an intranet address; ask the user to paste the YAML content directly.

### Step 2: Choose the business group and datasource

Call concurrently (do not serialize):
- `list_busi_groups()` to list visible business groups. Prefer the one with `is_default=true`; if there is only one, use it directly; if there is more than one, **use AskUserQuestion to let the user choose**
- `list_datasources(plugin_type="prometheus")` to list Prom datasources. If there is only one, use it directly; if more than one, let the user choose

If the user's request already specifies a business group name / datasource name, match by name and use it directly without asking again.

### Step 3: Preview

```
preview_prom_rule_yaml(payload_file=<file_path from step 1>)
```

The return is `{total, items:[{name, severity, prom_ql, for_duration_sec, append_tags, annotations}]}`.

**Keep the report to the user short** — listing the first 3-5 rules plus the total is enough; do not list every rule:

> Parsed 38 rules (critical 9 / warning 27 / info 2), for example:
> - `HostHighCpuLoad` (warning, for=0s)
> - `HostOutOfMemory` (warning, for=2m)
> - `HostOomKillDetected` (warning, for=0s)
> Import all of them into the Prometheus datasource "prom-prod" of business group "default"?

### Step 4: Write to the DB

After the user confirms:

```
import_prom_rule_yaml(
  group_id=<from step 2>,
  datasource_ids="[<ds id from step 2>]",
  payload_file=<file_path from step 1>
)
```

`datasource_ids` must be a JSON array string, e.g. `"[1]"` or `"[1,3]"`.

The return structure:

```json
{
  "total":   38,
  "created": 36,
  "skipped": 2,        // duplicate-name rules, automatically skipped, not a failure
  "failed":  0,        // real write errors (DB/validation, etc.)
  "items": [
    {"name":"HostHighCpuLoad", "status":"skipped_duplicate"},
    {"name":"HostOutOfMemory", "status":"created", "id":177},
    {"name":"BadOne", "status":"failed", "error":"<message>"}
  ]
}
```

Each rule's `status` is one of three:
- `created` — created successfully, has an `id`
- `skipped_duplicate` — a rule with the same name already exists, **no change was made**, no retry needed
- `failed` — a real error (DB, validation, etc.); see `error`

#### Handling duplicate names (important)

⚠️ **A rule with the same name is automatically skipped (status=skipped_duplicate), not a failure**. When you see skipped > 0, **never** "retry the whole YAML" with `name_prefix` — that would create a prefixed duplicate of all the N rules that were already created successfully, doubling the data and polluting it.

The correct way to report: just tell the user which rules were skipped due to duplicate names, and let the user decide what to do:

> ✅ Imported 36 rules. Another 2 were skipped because rules with the same name already exist: HostHighCpuLoad, HostOutOfMemory.
> If you want to overwrite them, go to the alert rules page, delete the old ones, and re-import; if you want the old and new ones to coexist (e.g. for comparison testing), you can re-import with name_prefix, but be aware that **all 38 rules will be prefixed**.

Only use `name_prefix`/`name_suffix` when the user **explicitly requests coexistence** (a very rare comparison-testing scenario), and inform the user in advance that the LLM will prefix all of the rules.

#### Only real failures (status=failed) need investigation

If `status=failed` appears, look at the `error` field: common cases include datasource validation failure, field format errors, etc. These are problems with the YAML itself or the environment; name_prefix cannot fix them.

#### Cautious mode

User says "create them but don't enable them yet" → pass `disabled=1`.

### Step 5: Output the result

**Keep it short**:

> ✅ Imported 36 alert rules into business group "default".
> ⚠️ 2 were skipped because rules with the same name already exist: HostHighCpuLoad, HostOutOfMemory. To overwrite, manually delete the old rules on the alert rules page, then re-import.

Do not list all 36 rule IDs; the frontend will display them as cards. **Do not proactively suggest "retry with name_prefix" — unless the user explicitly wants coexistence.**

## How to choose between payload and payload_file

| Scenario | Which to use |
|---|---|
| A remote file fetched via http_fetch | **`payload_file`** (http_fetch with `save_to_file=true`) |
| A small snippet of YAML the user pasted directly in chat (< 50 lines) | `payload` (the YAML is already in the context anyway, so writing it to disk is pointless) |
| A large block of YAML the user pasted directly in chat (dozens of rules) | Suggest the user use a URL or upload a file; if they insist on pasting, still use `payload` |

**Pick one of the two; you cannot pass both** (the tool will error).

## severity mapping (automatic)

The tool automatically maps the `labels.severity` string to n9e's number:

| Prom labels.severity | n9e severity |
|---|---|
| critical / error / fatal / page / sev1 | 1 |
| warning / warn / sev2 | 2 |
| info / notice / sev3 | 3 |
| other / missing | 2 (defaults to warning) |

No manual conversion by the LLM is needed.

## Handling of other labels

All labels other than `severity` are automatically assembled into n9e's `append_tags` (format `k=v`, spaces are stripped).

## Common Pitfalls

1. **`datasource_ids` must be a JSON array string**: pass `"[1]"`, not `"1"` or `1`.
2. **`http_fetch` cannot fetch from the intranet**: a `http://10.x.x.x/...` URL given by the user will be rejected outright. Ask the user to use a public URL or paste the content directly.
3. **`payload_file` must be one written by http_fetch**: the tool validates that the path is under `os.TempDir()` and starts with `n9e-aiagent-fetch-`. An arbitrary path cannot be used.
4. **The `expr` in the YAML is stored verbatim**: n9e stores the original expression directly (including thresholds); do not have the LLM split the `>`/`<` and the threshold.
5. **The `for` field is converted to seconds**: `5m` → 300, `1h` → 3600. If the original YAML has no `for`, n9e uses 0 seconds (triggers immediately).
6. **Do not query `list_alert_rules` to detect duplicate names**. Call import directly and decide whether to retry with a prefix based on the returned error.

## Example Conversation

**User:** Help me import https://raw.githubusercontent.com/samber/awesome-prometheus-alerts/refs/heads/master/dist/rules/host-and-hardware/node-exporter.yml

**Thought →** Concurrently fetch the URL (save to file) + list business groups + list datasources.

**Action 1:** `http_fetch(url="https://...", save_to_file=true)` → `{file_path: "/var/.../n9e-aiagent-fetch-abc.yml", size: 8731, ...}`
**Action 2:** `list_busi_groups()` → only default
**Action 3:** `list_datasources(plugin_type="prometheus")` → only prom-prod

**Action 4:** `preview_prom_rule_yaml(payload_file="/var/.../n9e-aiagent-fetch-abc.yml")` → 38 rules

**Final answer (preview stage):**
> Parsed 38 rules... Import them into business group "default" / datasource "prom-prod"?

**User:** Yes

**Action 5:** `import_prom_rule_yaml(group_id=1, datasource_ids="[2]", payload_file="/var/.../n9e-aiagent-fetch-abc.yml")`

**Final answer:**
> ✅ Imported 38 alert rules into "default / prom-prod".
