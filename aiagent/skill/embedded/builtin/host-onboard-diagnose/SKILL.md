---
name: host-onboard-diagnose
description: Diagnose onboarding failures where "categraf is installed/running but the host does not show up in the Nightingale host list, or shows unknown / has no metrics". Triggers when the user asks "why doesn't my newly installed host appear", "all the OS values in the host list are unknown", "I installed 3 collectors via Helm but only see 1", "the agent won't register", or "categraf is installed but the host doesn't show". **Mutually exclusive** with host-health-diagnose: that one handles "was onboarded before, now lost contact", while this skill handles "never got onboarded at all". Core stance: **a missing host is not a single cause, but rather one segment of the onboarding pipeline being broken**. Looking only at heartbeat.enable and telling the user to change categraf is a common pitfall (many users change it and still can't see the host, because the problem is in omit_hostname / ident shell / TLS / token / edge redis / multi-cluster routing).
max_iterations: 18
builtin_tools:
  - probe_target_onboard_status
  - list_targets
  - get_target_detail
  - query_prometheus
  - query_host_metrics_window
  - list_datasources
tags:
  - internal
---

# Host Onboarding Failure Diagnosis (host-onboard-diagnose)

## Scope

**Enter this skill**:
- "My newly installed categraf host doesn't show up in Nightingale"
- "The agent is installed and running, but the host doesn't appear in the host list"
- "For this host in the list, the OS / CPU / version are all unknown"
- "I deployed 3 categraf hosts via Helm, but the platform only sees 1"
- "I installed the agent on Windows but it won't register"
- "The host disappeared right after I changed its hostname" (if categraf is still running)

**Do NOT enter this skill**:
- Was visible before, recently lost contact → `host-health-diagnose`
- ident duplicate / want to clean up residue after renaming → `host-ident-cleanup` (to be built)
- Want to change alert rules / mutes → `creation` / `create-alert-rule`
- Looking into why an alert didn't fire → `alert-rule-troubleshoot`

## One-Sentence Principle

**A missing host ≠ a single cause.** The onboarding pipeline has 5 segments, and each segment getting stuck looks different. Looking at just one segment and telling the user to change config is a common pitfall. **Gather evidence first, then localize segment by segment, and finally give fix commands.**

## The 5 Segments of the Onboarding Pipeline

```
[1] categraf local process    Is it present / is the config correct / is heartbeat.enable on
        │
[2] heartbeat report HTTP      Can it reach /v1/n9e/heartbeat (network / TLS / BasicAuth)
        │
[3] server / edge receive      token / version compatibility / hostname duplicate check
        │
[4] target table persistence   Is this ident in the DB, is the meta in redis
        │
[5] Redis + metric stream       Can the time-series store find samples for this ident
```

## First Action: Call `probe_target_onboard_status`

This is the **only** diagnostic entry tool in this skill; it returns the footprint of all 5 segments in one shot. **Always call it first**, then decide the next step.

Key fields returned:
- `in_target_db` + `target.os` + `target.agent_version` → evidence for segments 3/4
- `in_redis_beat` + `redis_meta.hostname` + `redis_meta.remote_addr` → evidence for segment 4
- `in_prom_target_up` + `target_up_last` + `prom_metrics_hit` → evidence for segment 5
- `likely_segment` + `likely_causes` → **diagnosis already aggregated at the tool layer**; **do not bypass it and re-derive it yourself**

If the user did not provide an ident, first call `list_targets` and let the user pick, or filter out candidates by OS=unknown / empty agent_version (the "unassigned / partially onboarded" view).

## Decision Table (branch by likely_segment)

| likely_segment | Meaning | Preferred fix action |
|---|---|---|
| `segment_1_or_2` | This host is in none of DB / redis / prom | On the target host: `systemctl status categraf` → `journalctl -u categraf --since "5 min ago"` → check whether it reports `connection refused` / `x509` / `401` |
| `segment_3` | target exists but OS/agent_version empty | Check categraf's `config.toml`: `[heartbeat] enable=true` and `omit_hostname=false`; version ≥ v0.2.35 |
| `segment_4` | target persisted but no data in redis | Check whether n9e/edge has redis configured; in edge mode the `[Redis]` of `edge.toml`; whether n9e and n9e-edge versions are consistent |
| `segment_5` | redis has the beat but prom can't query it | Check whether categraf `[[writers]]` is configured; whether the datasource is correct in a multi-cluster deployment; whether the ident contains special characters like `()` `[]` `*` |
| `ok` | Onboarding is normal | If the user still insists "I can't see it", guide them to refresh the page / check business-group filtering / check browser cache |

## Three Variant Queries for Segment 5 (use `query_prometheus`)

When `likely_segment=segment_5`, you **must** run the following 3 queries with `query_prometheus` to confirm whether it is an ident label problem or truly no data:

```promql
# 1. Standard ident exact match (most common)
target_up{ident="<ident>"}

# 2. Fuzzy match (use when ident has an IP prefix / alias)
target_up{ident=~".*<host>.*"}

# 3. Extreme fallback: did it land on the instance label (snmp / custom tag scenario)
{instance=~".*<host>.*"}
```

All three return no data → the data stream truly never reached prom; go back and check categraf writers / TLS / n9e ingest queue.
Only (2) or (3) has data → **ident label problem** (special characters / global_labels override / snmp agent_host_tag misuse); guide the user to `host-ident-cleanup` (to be built) or to fix the categraf config.

## Output Template (strongly enforced)

The Final Answer uses Markdown, in the user's language. **Four sections**:

```
## Conclusion
<one sentence: stuck at segment X: xxxx (or: onboarding is normal, the reason you can't see it is yyy)>

## Onboarding Pipeline Evidence
- Segment 1/2 (categraf local/HTTP): <not gathered / inferred anomaly: xxx>
- Segment 3 (server receive): target in_db=true, os=unknown, agent_version="" → heartbeat metadata not persisted
- Segment 4 (target persistence + redis): target update_at=2026-05-14 10:23:11 but no heartbeat in redis
- Segment 5 (Prom): target_up has no data / prom_metrics_hit=0

## Fix Commands
1. Run on the target host:
   `grep -E 'heartbeat|omit_hostname' /etc/categraf/conf/config.toml`
   Expected: in the heartbeat section enable=true, omit_hostname=false. If either doesn't match, change it and `systemctl restart categraf`.
2. ...
3. ...

## Self-Verification Steps
<give the user 1-2 commands for "if you want to confirm it's fixed, you can verify like this", for example:
- Within 30s after restarting categraf, refresh the host list back in the platform; the OS/CPU fields should be non-unknown
- `curl -s http://<n9e>:17000/api/n9e/self-metrics | grep <ident>`>
```

## Anti-Patterns (do NOT do these)

- ❌ Telling the user to change `heartbeat.enable` directly without calling `probe_target_onboard_status`. **Gather evidence first.** Many users already have heartbeat enabled; the problem is in omit_hostname / TLS / version.
- ❌ Saying "categraf isn't installed" just because `in_target_db=false`. You must look at the redis segment and the segment_1_or_2 causes to judge holistically whether it's a network problem or a process problem — the recommended actions are completely different.
- ❌ When segment 3 is stuck, only telling the user to change heartbeat without mentioning omit_hostname / version. Those two are just as common.
- ❌ When segment 5 is stuck, telling the user to change writers without running the 3 variant PromQL queries. An ident label problem also gets stuck at segment 5.
- ❌ Not giving concrete commands in the output, only saying "check the categraf config". Every recommendation must be something the user can paste and run directly.

## Known Failure Modes Per Segment

- **Segment 1/2**: categraf can't reach center, connection refused, TLS unknown authority, self-signed certificate, BasicAuth invalid, ams token mismatch, Helm multi-node only 1 seen, Windows, Win2008 not supported
- **Segment 3**: heartbeat enable=false, unknown fields / omit_hostname=true, categraf version too low, v6 requires v0.2.35+, identity shell fails to get IP, hostname duplicate
- **Segment 4**: edge redis nil, n9e and n9e-edge version mismatch, CenterApi missing, host not visible in the center under edge deployment
- **Segment 5**: ident with parentheses not found in dashboards, host=* bug, snmp ident conflict, omit_hostname=true causing the ident label to be lost, wrong datasource in multi-cluster, write queue full 499, global.labels override

## Output Style

- Don't be coy. The conclusion goes in the first line of the first section.
- Evidence must give concrete field values; don't write "looks normal".
- Fix commands must be paste-and-run; avoid fluff like "go check it".
- Answer in the user's language (Chinese for Chinese users, English for English users).
- If `likely_segment=ok` but the user insists the host doesn't appear, prompt them to check: business-group filtering (the host is present but hidden by business group in the frontend), browser cache, and the visible-business-group permissions of the logged-in user.
