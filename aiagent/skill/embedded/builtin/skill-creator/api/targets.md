# Monitored targets (hosts)

Machines / agents reporting to n9e. Each **target** is one host (identified by its
`ident`) that a Categraf / telegraf-style agent pushes heartbeats and metadata for.
Use these endpoints for host inventory, per-host alert context (map an alert's host to
its tags / business groups / OS / agent version), liveness/coverage checks, and agent
version audits. `/targets` lists hosts; `/targets/stats` gives you rolled-up counts.

> Gateway call: GET. Include the `/api/n9e` prefix in `path`. Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol: see `../n9e-api.md`.

## Endpoints
| Path | Purpose | `dat` shape |
|---|---|---|
| `/targets` | List/inventory of hosts, with per-host live metrics (cpu/mem/liveness) merged in from cache. | Pattern A — `{"list":[<Target>...],"total":N}` |
| `/targets/stats` | Rolled-up summary counts (alive/dead, cpu/mem utilization buckets, agent-version histogram) over your visible hosts. | object (see below) |

## Query parameters (`/targets`)
| Param | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `gids` | string (csv of business-group ids) | no | *your groups + ungrouped* | Restrict to these business groups. Empty = groups your RBAC allows **plus** hosts in no group. `0` means "ungrouped". |
| `query` | string | no | `""` | Text filter over `ident` / `host_ip` / `note` / tags / `host_tags` / `os`. Space-separated words are AND'd; a word prefixed with `-` excludes (negation). |
| `limit` | int (as string) | no | `30` | Page size. |
| `p` | int (as string) | no | `1` | Page number, starts at 1. |
| `hosts` | string (csv / space / newline separated idents) | no | — | Restrict to specific hosts. Matches `ident` **or** `host_ip`. |
| `order` | string | no | `ident` | Column to sort by (validated against real columns; falls back to `ident`). |
| `desc` | bool (as string) | no | `false` | Sort descending when `true`. |
| `datasource_ids` | string (csv) | no | — | Restrict to targets received via these datasource ids. |
| `downtime` | int seconds (as string) | no | `0` | Keep only hosts whose last heartbeat is older than this many seconds (i.e. silent longer than `downtime`). `0` = no downtime filter. |

## Response — `dat` payload
(`/targets` = Pattern A `{"list":[...],"total":N}`.) Each item is a `Target`
(`models/target.go`). "(computed)" = not a DB column; filled per-request by the handler
from Redis/cache, so it reflects live state (and can be empty/zero if the host has never
reported).

| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | Primary key of the target row. |
| `group_id` | int64 | Legacy single business-group id. Mostly `0` after the many-to-many migration — use `group_ids`/`group_objs` instead. |
| `group_objs` | array of BusiGroup | (computed) Full business-group objects this host belongs to (each has `id`, `name`, …). Source of group names. |
| `ident` | string | Unique host identifier (the agent's hostname/ident). The primary key you match alerts and other APIs on. |
| `note` | string | Free-text note / description for the host. |
| `tags` | array of string | User-attached tags, each as `key=value`. (JSON field `tags`; derived from the stored space-separated tag string.) |
| `tags_maps` | object (string→string) | (computed) Merged tag map — user tags **and** `host_tags` parsed into `{key: value}`. Convenient for lookups. |
| `update_at` | int64 | Unix seconds of the last DB update to this row (e.g. tag/note/group change). Not the heartbeat time. |
| `host_ip` | string | Host IPv4 address. |
| `agent_version` | string | Version string reported by the agent. |
| `engine_name` | string | Name of the engine/cluster instance that ingested this target. |
| `os` | string | Operating system (e.g. `linux`, `windows`). |
| `host_tags` | array of string | Tags reported by the host/agent itself, each as `key=value` (distinct from user `tags`). |
| `beat_time` | int64 | (computed) Real-time last-heartbeat unix seconds, from Redis. `0` if never seen. Basis for liveness/downtime. |
| `unixtime` | int64 | (computed) The agent's own clock (unix seconds) at last report. |
| `offset` | int64 | (computed) Clock offset (seconds) between server and agent. |
| `target_up` | float64 | (computed) Liveness: `2` = heartbeat within 60s (up), `1` = within 180s (warning), `0` = down / never seen. |
| `mem_util` | float64 | (computed) Memory utilization percent from last metadata report. |
| `cpu_num` | int | (computed) CPU core count. `-1` = host has never reported metadata (unknown). |
| `cpu_util` | float64 | (computed) CPU utilization percent from last metadata report. |
| `arch` | string | (computed) CPU architecture (e.g. `amd64`, `arm64`). |
| `remote_addr` | string | (computed) Source network address of the last report. |
| `group_ids` | array of int64 | (computed) Business-group ids this host belongs to (many-to-many). |
| `group_names` | array of string | (computed) Business-group names. Note: not populated by `/targets` (usually empty) — read names from `group_objs` instead. |

### `/targets/stats` payload
`dat` is a flat object of counts over the hosts you can see (respects the same `gids`
query param; other `/targets` filters do not apply here — it counts the whole visible set):

| Field | Type | Meaning |
|---|---|---|
| `count` | int64 | Total hosts in scope. |
| `alive_count` | int64 | Hosts with a heartbeat within the last 180s. |
| `dead_count` | int64 | Hosts with no heartbeat in the last 180s (`count - alive_count`). |
| `cpu_usage` | object (string→int64) | Histogram of CPU utilization. Keys are upper-bound buckets: `"20"`=<20%, `"40"`=20–40%, `"60"`=40–60%, `"80"`=60–80%, `"100"`=≥80%, `"-1"`=unknown (no metadata). |
| `mem_usage` | object (string→int64) | Same bucketing as `cpu_usage`, for memory utilization. |
| `versions` | object (string→int64) | Agent-version histogram: `agent_version` → host count. Empty version is counted under `"unknown"`. |

## Example
Request:
```json
{"method":"GET","path":"/api/n9e/targets","query":{"limit":"100","p":"1"}}
```
Response (trimmed):
```json
{
  "ok": true,
  "status": 200,
  "data": {
    "dat": {
      "list": [
        {
          "id": 12,
          "group_id": 0,
          "group_objs": [{"id": 2, "name": "web-team"}],
          "ident": "host-web-01",
          "note": "prod nginx",
          "tags": ["env=prod", "role=web"],
          "tags_maps": {"env": "prod", "role": "web", "region": "bj"},
          "update_at": 1719800000,
          "host_ip": "10.0.0.11",
          "agent_version": "0.3.60",
          "engine_name": "default",
          "os": "linux",
          "host_tags": ["region=bj"],
          "beat_time": 1719812345,
          "unixtime": 1719812345,
          "offset": 0,
          "target_up": 2,
          "mem_util": 42.7,
          "cpu_num": 8,
          "cpu_util": 13.5,
          "arch": "amd64",
          "remote_addr": "10.0.0.11:54312",
          "group_ids": [2],
          "group_names": []
        }
      ],
      "total": 137
    },
    "err": ""
  }
}
```

`/targets/stats` response `dat` (trimmed):
```json
{
  "count": 137,
  "alive_count": 130,
  "dead_count": 7,
  "cpu_usage": {"-1": 4, "20": 100, "40": 20, "60": 8, "80": 3, "100": 2},
  "mem_usage": {"-1": 4, "20": 60, "40": 45, "60": 20, "80": 6, "100": 2},
  "versions": {"0.3.60": 120, "0.3.58": 13, "unknown": 4}
}
```
