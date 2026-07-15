---
name: modify-task-tpl
description: Helps users generate, modify, or troubleshoot Nightingale (n9e) alert self-healing scripts (task_tpl / ibex scripts). Use when the user asks to "write a self-healing script for disk cleanup / restarting a service / cleaning logs / dumping a process / reloading nginx", or asks "how does a self-healing script get the parameters passed from the alert", "what format is stdin in", "what should I set timeout to", "why is is_recovered always false", "why can't my self-healing script get the k8s namespace", "what do I do when a script stays running forever". This skill focuses on the **script body layer**—if the user wants to change alert rules, recipients, or notification templates, direct them to the corresponding skill.
tags:
  - internal
---

# Nightingale (n9e) Alert Self-Healing Script (task_tpl) Generation

Nightingale alert self-healing is the **ibex subsystem**: when an alert rule's `callbacks` field is written as `${ibex}/<task_tpl_id>`, after an alert event fires it pulls up the script of the corresponding task_tpl on the machine matching the current event's `TargetIdent` and runs it on that machine's categraf. The script receives the labels of the current alert through **stdin**.

This skill focuses on **writing/modifying the `task_tpl.script` field itself**—it does not cover creating alert rules, configuring recipients, or editing notification templates.

---

## 1. Scope: First Determine Which Layer the User Is Changing

The Nightingale alert pipeline has four layers, and each goes through a different skill:

| Layer | Entity | Key files | Handled by this skill |
|---|---|---|---|
| **Self-healing script** task_tpl | `task_tpl` table | `models/task_tpl.go`, `alert/sender/ibex.go` | **Yes** |
| Alert rule | `alert_rule` table | `models/alert_rule.go` | No (use `create-alert-rule`) |
| Notification template | `notify_tpl` table | `models/notify_tpl.go` | No (use `generate-message-template`) |
| Notification channel | `notify_channel` table | `models/notify_channel.go` | No (use `notify-channel-copilot`) |

**Judgment criteria**: the user's wording contains "script/shell/bash/python/jq/parse/execute/timeout"—this skill; contains "PromQL/threshold/trigger condition"—alert rule; contains "template/body/field rendering"—message template; contains "URL/Webhook/signature"—notification channel.

---

## 2. Data Model (What the User Can Fill In, Which Fields Nightingale Actually Uses)

### `TaskTpl` (`models/task_tpl.go:17-35`)

| Field | Type | Meaning |
|---|---|---|
| `id` | int64 | Primary key; this is what `${ibex}/<id>` in the alert rule callbacks references |
| `group_id` | int64 | Business group (permission boundary, validated by `CanDoIbex`) |
| `title` | string | Template name; at execution time it is concatenated into `<title> FH: <hostname>` and written as the task title |
| `script` | string | **The main field this skill operates on** |
| `args` | string | Command-line arguments. If the caller does not pass `args` when the alert fires and executes, the default value here is used |
| `tags` | string | Space-separated tags, used for template list filtering/classification, does not affect execution |
| `account` | string | Which user identity to run as on the target machine (e.g. `root`) |
| `batch` | int | Number of hosts running concurrently per batch. **Self-healing usually only runs on the host that triggered it, so 0 is fine** |
| `tolerance` | int | Allowed failure count within a batch. Leave at 0 for single-host self-healing scenarios |
| `timeout` | int | **Seconds**. 0 → defaults to 30; > 5 days → rejected |
| `pause` | string | Pause schedule between batches (cron style). Basically unused for self-healing |

### `TaskForm` (`models/task_tpl.go:351-365`) — the Carrier Actually Dispatched

Alert-triggered self-healing goes through `alert/sender/ibex.go CallIbex` → constructs a `TaskForm` → `TaskAdd` writes to ibex:

- `AlertTriggered: true` —— edge data centers go through Redis direct-dispatch to categraf, not relying on the central DB (`ibex.go:244-276`)
- `Stdin: <JSON string>` —— **what this skill cares about most**, explained in detail in the next section
- `Hosts: []string{event.TargetIdent}` —— executes only on the machine the alert event matched
- `Title: tpl.Title + " FH: " + host` —— easy to recognize in execution history (FH = From Host)

### CleanFields Hard Constraints (`models/task_tpl.go:137-189`)

These validations run before saving or dispatching; violations are rejected outright:

- `timeout == 0` → automatically set to 30
- `timeout > 3600*24*5` → error "longer than five days"
- `title` / `args` / `pause` / `tags` containing `str.Dangerous` characters (`` ` ``, `$()`, `&&`, etc.) → error
- `script` automatically `\r\n → \n` (resolves the CRLF problem from Windows editor copies)
- `script == ""` → error "script is required"

---

## 3. ⚠️ The Truth About stdin (the Easiest Pitfall, **Must Read**)

### 3.1 stdin Is a Flat JSON of `map[string]string`, **Not a `$event` Object**

Many users copy syntax like `{{$event.RuleName}}` from the message template docs—it **does not work at all**. `$event` is the context of the `notify_tpl` template; there is no template rendering process inside an ibex self-healing script—what it receives is just a plain JSON string.

**The construction logic of stdin** (`alert/sender/ibex.go:118-142`):

```go
tagsMap := make(map[string]string)
for _, pair := range event.TagsJSON {        // flatten event labels one by one
    k, v := splitOnce(pair, "=")
    tagsMap[k] = v
}
// inject 3 built-in keys
tagsMap["alert_severity"]      = strconv.Itoa(event.Severity)
tagsMap["alert_trigger_value"] = event.TriggerValue
tagsMap["is_recovered"]        = strconv.FormatBool(event.IsRecovered)

tags, _ := json.Marshal(tagsMap)             // serialize the whole thing into a string
in.Stdin = string(tags)
```

**Example of a real stdin payload**:

```json
{
  "ident": "host01",
  "instance": "host01:9100",
  "job": "categraf",
  "alert_severity": "2",
  "alert_trigger_value": "92.5",
  "is_recovered": "false"
}
```

Note: all values are strings (`alert_severity` is `"2"`, not `2`); to parse them into numbers in the script you have to convert them yourself.

### 3.2 Three Fields You "Want but Can't Get", and Their Solutions

| Field you want | Why you can't get it | Solution |
|---|---|---|
| Friendly names like `rule_name` / `rule_id` / `trigger_time` / `severity` | stdin only carries labels, not event metadata | In PromQL use `label_replace(..., "rule_name", "...", "...", "...")` to inject the info into a label; or switch to a callback medium (HTTP carrying the whole event JSON) |
| Recipient / notification group | Self-healing and notification rules are **two independent channels**; self_heal only looks at `alert_rule.callbacks`, not `notify_rule` | If you want recipients → go through the callback medium; stdin is not this path |
| Labels aggregated away by PromQL (e.g. k8s `namespace`/`deployment`) | `sum(...)` without `by(...)` drops labels | **The PromQL in the alert rule must be written as `sum by (instance, namespace, deployment)(...)`**; only the retained labels will enter stdin |

### 3.3 Templates for Reading stdin in Three Languages (Must Include When Generating Scripts)

**shell + jq (most common)**:

```bash
#!/bin/bash
set -euo pipefail

PAYLOAD=$(cat)                    # read all of stdin at once to avoid blocking
[ -z "$PAYLOAD" ] && { echo "FATAL: empty stdin payload"; exit 2; }

IDENT=$(echo "$PAYLOAD" | jq -r '.ident          // empty')
SEV=$(echo   "$PAYLOAD" | jq -r '.alert_severity // "3"')
VAL=$(echo   "$PAYLOAD" | jq -r '.alert_trigger_value // "0"')

[ -z "$IDENT" ] && { echo "FATAL: no ident in stdin"; exit 2; }
echo "[$(date -Iseconds)] ident=$IDENT severity=$SEV value=$VAL"
```

**shell fallback without jq** (use grep+sed, suitable for minimal containers):

```bash
PAYLOAD=$(cat)
IDENT=$(echo "$PAYLOAD" | grep -oE '"ident"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/')
```

**python** (when the processing logic is complex):

```python
#!/usr/bin/env python3
import sys, json, time

try:
    data = json.load(sys.stdin)
except json.JSONDecodeError as e:
    print(f"FATAL: bad stdin json: {e}", file=sys.stderr)
    sys.exit(2)

ident = data.get("ident", "")
sev   = int(data.get("alert_severity", "3"))
val   = float(data.get("alert_trigger_value", "0"))

if not ident:
    print("FATAL: no ident", file=sys.stderr)
    sys.exit(2)

print(f"[{time.strftime('%FT%T')}] ident={ident} sev={sev} val={val:.2f}")
```

**go** (rarely used, minimal skeleton):

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

func main() {
    var d map[string]string
    if err := json.NewDecoder(os.Stdin).Decode(&d); err != nil {
        fmt.Fprintln(os.Stderr, "FATAL:", err)
        os.Exit(2)
    }
    fmt.Println("ident:", d["ident"])
}
```

---

## 4. ⚠️ The Truth About `is_recovered` (**Don't Hard-Code Dead Code**)

**ibex self-healing does not fire on recovery events**—`alert/sender/ibex.go:39-42`:

```go
func (c *IbexCallBacker) CallBack(ctx CallBackContext) {
    ...
    if event.IsRecovered {
        logger.Infof("event_callback_ibex: event is recovered, event: %s", event.Hash)
        return                       // ← returns directly, never calls handleIbex at all
    }
    ...
}
```

**Therefore `is_recovered` in stdin is always the string `"false"`.** Writing `if [ "$IS_RECOVERED" = "true" ]; then ...` is dead code.

**The correct path**:

- If you want to "also run an action on alert recovery (send email / close ticket / notify IM)" → use **notify_rule + callback medium**, check "also notify on recovery" in the notify_rule, and process the event JSON after the callback URL receives it.
- If you want to "trigger another script on a recovery event" → same as above; **do not** try to reuse task_tpl.

A core pain point is "I also want it to trigger on recovery", and the common idea of "adding an is_recovered check" is actually a non-solution—fundamentally you need an architectural change to ibex so it accepts recovery events, or the user must switch to the callback channel.

---

## 5. timeout / batch / tolerance / pause —— Semantics of the Numeric Fields

### 5.1 timeout (seconds)

- Defaults to 30 seconds (auto-filled by `CleanFields`)
- Maximum of 5 days (anything longer errors out; by design self-healing is a short-cycle task)
- After timeout **the process is SIGKILLed**; stdout/stderr is written to the task_host table before being killed
- **No direct relationship to the alert evaluation interval**—a 60s evaluation cycle does not mean the script only has 60s to run

**Typical value reference**:

| Scenario | Suggested timeout |
|---|---|
| reload config / systemctl restart | 30 ~ 60 |
| clean logs / image cache (several GB) | 120 ~ 300 |
| jstack/jmap heap dump | 300 ~ 600 |
| yum/apt package install | 600 ~ 1800 |

### 5.2 Distinguish Three "Timeout-Class" Root Causes

| Symptom | Real root cause | Solution |
|---|---|---|
| docker-compose self-healing script times out | mistakenly treating telegraf as the ibex-agent, **the channel was never established at all** | install categraf (not telegraf), check the `categraf -test` output |
| notification script with timeout=0 is killed immediately | fe form DefaultTimeout=0 BUG | already fixed in backend CleanFields (0 → 30) |
| self-healing script execution times out | the script itself is slow (remote yum / large file copy) | increase `timeout`; consider using a p2p tool to transfer files (do not use ibex as a file distributor) |

**Do not lump them together**—when the user says "timeout", first ask about the scenario.

### 5.3 batch / tolerance / pause

- **batch**: number of hosts running concurrently per batch. Single-host self-healing triggered by an alert doesn't need it; leave at 0. Only manual batch tasks (ops scenarios) use it.
- **tolerance**: allowed failure count within a batch. Same as batch, leave at 0 for self-healing scenarios.
- **pause**: pause between batches (e.g. `00:00-08:00,17:00-23:59` means pause during these time ranges). Basically not used for self-healing—an alert can fire at any time, so you can't use pause to restrict working hours. **If you really want to restrict working hours**, do it in the alert_rule's `enable_in_bg` / `enable_stime` fields, or use the notify_rule's time window.

---

## 6. Dangerous Command List (Must Be Avoided or Guarded When Generating)

### 6.1 Blacklist (**Refuse to Generate Outright**, Even If the User Explicitly Asks)

| Command | Risk |
|---|---|
| `rm -rf /` / `rm -rf /*` / `rm -rf $UNSET_VAR/` | Wipes the entire disk |
| `mkfs.*`, `dd of=/dev/sda`, `shred /dev/sda` | Filesystem destruction |
| `shutdown`, `reboot`, `init 0`, `init 6`, `halt`, `poweroff` | Whole-machine shutdown—a self-healing script should not have this power |
| `iptables -F` / `ufw disable` / `firewall-cmd --reload` without backup | Network/security policy loss |
| `chmod -R 777 /`, `chown -R nobody:nobody /` | Permission destruction |
| `curl <non-whitelisted URL> \| sh`, `wget ... -O - \| bash` | Remote code injection |
| base64/zip/gzip-encoded embedded shell | Evades static review |
| `kubectl delete node`, `kubectl drain` without PDB check | Cluster-level impact |

### 6.2 Graylist (**Generate but Must Add Guards**)

| Command | Required accompanying guards |
|---|---|
| `systemctl restart <svc>` | lock file to prevent re-entry; record PID before/after |
| `kill -9` | first `kill -TERM` and wait 10s; then `-9`; record process info |
| `docker rm` / `docker rmi` | first `docker ps -a` to filter, forbid deleting running containers |
| `find -delete` | first dry-run (`find ... -print | head`), then add `-mtime +N` to limit scope |
| `truncate` / `> /var/log/xxx` | back up to `.bak.<ts>`; don't touch the active log (logrotate first) |
| `iptables -A` | write a backup to `/etc/iptables/rules.v4.bak.<ts>` |

### 6.3 Generic Guard Template

```bash
#!/bin/bash
set -euo pipefail

# (1) single-instance lock, to prevent concurrent triggers
LOCK=/var/run/$(basename "$0").lock
exec 9>"$LOCK"
flock -n 9 || { echo "another instance running, skip"; exit 0; }

# (2) dry-run switch (pass --dry-run in args to not actually do it)
DRY_RUN=0
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=1

# (3) before state
echo "=== BEFORE ==="
df -h /var/log

# (4) main body (switch by DRY_RUN)
if [[ $DRY_RUN -eq 1 ]]; then
    find /var/log -name "*.log" -mtime +7 -print | head -20
else
    find /var/log -name "*.log" -mtime +7 -delete
fi

# (5) after state
echo "=== AFTER ==="
df -h /var/log
```

---

## 7. Built-in Scenario Library (For When "I Can't Think of a Good Scenario" Hits)

Each scenario gives: typical alert rule name → script skeleton → suggested timeout → risk points.

### 7.1 Disk Space Category

**S1: Clean up logs older than 7 days under `/var/log`**

- Trigger: `disk_used_percent{mountpoint="/"} > 90`
- timeout: 120

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
echo "before: $(df -h /var/log | tail -1)"
find /var/log -type f -name "*.log.*" -mtime +7 -print -delete | wc -l
find /var/log -type f -name "*.gz"    -mtime +7 -print -delete | wc -l
echo "after:  $(df -h /var/log | tail -1)"
```

**S2: Clean up docker image layer cache**

- Trigger: disk full + the host has docker
- timeout: 300

```bash
#!/bin/bash
set -euo pipefail
command -v docker >/dev/null || { echo "no docker, skip"; exit 0; }
echo "before:"; df -h /var/lib/docker
docker image prune -af --filter "until=168h"     # images older than 7 days
docker builder prune -af --filter "unused-for=168h"
echo "after:"; df -h /var/lib/docker
```

**S3: Clean up files older than 30 days under `/tmp`**

- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
find /tmp -xdev -type f -mtime +30 -print -delete | wc -l
find /tmp -xdev -type d -empty -mtime +30 -delete 2>/dev/null || true
```

**S4: Shrink journalctl to 500MB**

- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
journalctl --disk-usage
journalctl --vacuum-size=500M
journalctl --disk-usage
```

### 7.2 Memory / Process Category

**S5: Find the top5 memory processes and dump them (do not auto-kill)**

- Trigger: `mem_used_percent > 90`
- timeout: 60
- ⚠️ Intentionally does not kill processes—high memory is rooted in a business problem, and auto-killing would widen the failure

```bash
#!/bin/bash
set -euo pipefail
TS=$(date +%Y%m%d-%H%M%S)
OUT=/var/log/n9e-mem-top5.$TS.txt
{
    echo "=== top5 mem proc @$TS ==="
    ps -eo pid,user,%mem,%cpu,etime,cmd --sort=-%mem | head -6
    echo; free -m
} > "$OUT"
echo "dumped to $OUT"
```

**S6: JVM OOM auto-dump + restart**

- Trigger: `process_jvm_heap_used_percent > 95`
- timeout: 600
- ⚠️ Requires the `service` label in stdin (add it to the PromQL by clause)

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
SVC=$(echo "$PAYLOAD" | jq -r '.service // empty')
[ -z "$SVC" ] && { echo "no service tag"; exit 2; }

PID=$(systemctl show -p MainPID --value "$SVC")
[ "$PID" = "0" ] && { echo "service not running"; exit 0; }

TS=$(date +%Y%m%d-%H%M%S)
DIR=/var/log/jvm-dump
mkdir -p "$DIR"

jstack "$PID" > "$DIR/jstack.$SVC.$TS.txt" || true
jmap -dump:live,format=b,file="$DIR/heap.$SVC.$TS.hprof" "$PID" || true

# lock file to prevent re-entry
LOCK=/var/run/n9e-restart-$SVC.lock
exec 9>"$LOCK"; flock -n 9 || { echo "already restarting"; exit 0; }

systemctl restart "$SVC"
sleep 5
systemctl is-active "$SVC"
```

**S7: Zombie process cleanup**

- Trigger: `zombie_process_count > 5`
- timeout: 30

```bash
#!/bin/bash
set -euo pipefail
ps -eo stat,ppid,pid,cmd | awk '$1 ~ /^Z/ {print $2}' | sort -u | while read ppid; do
    echo "send SIGCHLD to ppid $ppid"
    kill -SIGCHLD "$ppid" 2>/dev/null || true
done
```

### 7.3 Service Availability Category

**S8: Service port down → systemctl restart (with cooldown)**

- Trigger: `net_tcp_connection_count{port="3306"} < 1`
- timeout: 120

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
SVC=$(echo "$PAYLOAD" | jq -r '.service // empty')
[ -z "$SVC" ] && { echo "no service tag"; exit 2; }

# cooldown: skip if already restarted within 5 minutes
COOLDOWN=/var/run/n9e-restart-$SVC.cooldown
if [ -f "$COOLDOWN" ] && [ $(($(date +%s) - $(stat -c%Y "$COOLDOWN"))) -lt 300 ]; then
    echo "in cooldown, skip"
    exit 0
fi

systemctl restart "$SVC"
touch "$COOLDOWN"
sleep 5
systemctl is-active "$SVC"
```

**S9: Reload after nginx config change**

- Trigger: manual trigger / `file_change_count{path="/etc/nginx"} > 0`
- timeout: 30

```bash
#!/bin/bash
set -euo pipefail
nginx -t                          # validate config
nginx -s reload
sleep 1
pgrep -x nginx >/dev/null || { echo "nginx died after reload"; exit 1; }
echo "reloaded ok"
```

**S10: Only diagnose a crashloop container without restarting it (k8s)**

- Trigger: `kube_pod_container_status_restarts_total > 5`
- timeout: 60
- ⚠️ k8s already auto-restarts, so self-healing only does diagnosis + notification

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
NS=$(echo "$PAYLOAD" | jq -r '.namespace // "default"')
POD=$(echo "$PAYLOAD" | jq -r '.pod // empty')
[ -z "$POD" ] && { echo "no pod tag (check PromQL by clause)"; exit 2; }

TS=$(date +%Y%m%d-%H%M%S)
OUT=/var/log/n9e-crash-$NS-$POD.$TS.log
{
    kubectl -n "$NS" describe pod "$POD"
    echo "==="
    kubectl -n "$NS" logs "$POD" --tail=200 --previous || true
} > "$OUT"
echo "diagnosed → $OUT"
```

### 7.4 Network / System Category

**S11: NIC down → ip link set up (whitelist)**

- Trigger: `net_interface_up{iface=~"eth.+|ens.+"} == 0`
- timeout: 30

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
IFACE=$(echo "$PAYLOAD" | jq -r '.iface // empty')
[ -z "$IFACE" ] && { echo "no iface"; exit 2; }

# whitelist: only allow eth* / ens* / eno*, to avoid touching container/VPN NICs
[[ "$IFACE" =~ ^(eth|ens|eno) ]] || { echo "iface $IFACE not in whitelist"; exit 0; }

ip link show "$IFACE"
ip link set "$IFACE" up
sleep 2
ip link show "$IFACE"
```

**S12: NTP offset → restart chronyd**

- Trigger: `system_clock_offset_seconds > 60`
- timeout: 30

```bash
#!/bin/bash
set -euo pipefail
chronyc tracking | grep -E "System time|Last offset"
systemctl restart chronyd
sleep 5
chronyc makestep
chronyc tracking | grep -E "System time|Last offset"
```

**S13: DNS resolution failure → restart systemd-resolved + verify**

- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
echo "before: $(dig +short @127.0.0.53 example.com || true)"
systemctl restart systemd-resolved
sleep 2
echo "after:  $(dig +short @127.0.0.53 example.com)"
```

**S14: Recover drifted sysctl parameters**

- Trigger: `node_net_somaxconn != 65535`
- timeout: 10

```bash
#!/bin/bash
set -euo pipefail
sysctl -w net.core.somaxconn=65535
grep -q "net.core.somaxconn=65535" /etc/sysctl.d/99-n9e.conf || \
    echo "net.core.somaxconn=65535" >> /etc/sysctl.d/99-n9e.conf
```

### 7.5 Windows Scenarios (PowerShell)

**S15: Windows service down → Restart-Service**

- Trigger: `windows_service_state{state!="Running"} == 1`
- timeout: 120
- In the task_tpl, set `account` to a user with permissions; script type `script_type: powershell` (supported by categraf-win)

```powershell
$payload = [Console]::In.ReadToEnd() | ConvertFrom-Json
$svc = $payload.service
if (-not $svc) { Write-Error "no service tag"; exit 2 }

Get-Service $svc | Format-Table
Restart-Service $svc -Force
Start-Sleep -Seconds 5
$st = (Get-Service $svc).Status
Write-Output "after restart: $st"
if ($st -ne "Running") { exit 1 }
```

**S16: Windows disk cleanup (temp + Windows.old)**

- timeout: 300

```powershell
$before = (Get-PSDrive C).Free / 1GB
Get-ChildItem C:\Windows\Temp -Recurse -Force -ErrorAction SilentlyContinue |
    Where-Object { $_.LastWriteTime -lt (Get-Date).AddDays(-7) } |
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
$after = (Get-PSDrive C).Free / 1GB
Write-Output ("freed {0:N2} GB" -f ($after - $before))
```

### 7.6 In-Container Scenarios

**S17: Restart a process inside a pod via `kubectl exec`**

- Register categraf on an "ops machine" in the central data center that has a kubeconfig, and select this machine for the task_tpl to execute on
- timeout: 120

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
NS=$(echo "$PAYLOAD" | jq -r '.namespace // empty')
POD=$(echo "$PAYLOAD" | jq -r '.pod // empty')
CTN=$(echo "$PAYLOAD" | jq -r '.container // empty')

[ -z "$NS" ] || [ -z "$POD" ] || [ -z "$CTN" ] && { echo "need namespace/pod/container in labels"; exit 2; }

kubectl -n "$NS" exec "$POD" -c "$CTN" -- supervisorctl restart app || \
kubectl -n "$NS" delete pod "$POD"     # fallback recreate
```

**S18: Execute inside a container via `docker exec`**

- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
CNAME=$(echo "$PAYLOAD" | jq -r '.container_name // empty')
[ -z "$CNAME" ] && { echo "no container_name"; exit 2; }
docker inspect "$CNAME" >/dev/null || { echo "no such container"; exit 2; }
docker exec "$CNAME" sh -c 'kill -HUP 1'    # let PID 1 reload itself
```

### 7.7 Middleware Scenarios

**S19: MySQL long transaction kill (with user whitelist)**

- Trigger: `mysql_long_transaction_seconds > 600`
- timeout: 60

```bash
#!/bin/bash
set -euo pipefail
PAYLOAD=$(cat)
THRESH=600

# only kill user connections, bypass system users
mysql -N -e "
SELECT trx_mysql_thread_id
FROM information_schema.innodb_trx t
JOIN information_schema.processlist p ON p.ID=t.trx_mysql_thread_id
WHERE TIMESTAMPDIFF(SECOND, trx_started, NOW()) > $THRESH
  AND p.USER NOT IN ('root','repl','backup','event_scheduler')
" | while read tid; do
    [ -n "$tid" ] && mysql -e "KILL $tid"
done
```

**S20: Redis memory over threshold → force BGREWRITEAOF + suggest scaling up**

- timeout: 120

```bash
#!/bin/bash
set -euo pipefail
USED=$(redis-cli INFO memory | awk -F: '/^used_memory_rss:/{gsub(/\r/,""); print $2}')
echo "rss: $USED bytes"
redis-cli BGREWRITEAOF
echo "BGREWRITEAOF triggered, NOT auto-restarting redis; please scale up maxmemory or scale out"
```

---

## 8. Debugging and Troubleshooting

### 8.1 "Script Stays Running Forever"

Cross-reference issues in chronological order:

| Symptom | Root cause | Status |
|---|---|---|
| stays running because stdout never flushes | ibex < v8.3.0 feedback-not-closed-loop bug | fixed, **upgrade to v8.3.0+** |
| script has `while true; sleep` (never exits on its own) | coding problem | add a timeout, add a max loop count |
| task timed out but categraf never received the kill | network jitter | wait for the task_host table status to auto-transition to failed (depends on heartbeat) |

### 8.2 "Can't Get stdin"

Troubleshooting order:

1. **Open a manual task test**: in "Task Center" manually create a task once, enter `{"ident":"host01"}` as stdin, and see whether the script can echo it out
2. **Check the PromQL `by()` clause**: 90% of missing-label cases in self-healing are this; the event detail page shows what event.TagsJSON actually contains
3. **Check how the script reads**: `read VAR` only reads one line; use `cat` or `python json.load(sys.stdin)` to read it all at once

### 8.3 "exit code 0 but the Service Didn't Actually Recover"

- Force `set -e` (exit immediately if any sub-command fails)
- Add explicit verification after key steps: `systemctl is-active "$SVC" || exit 1`
- Output before/after to stdout so task_record is observable

### 8.4 SQL to Check Execution History (When the User Asks "Did the Task Run Successfully")

```sql
-- look at all self-healing tasks triggered by a given event
SELECT id, title, create_at, FROM_UNIXTIME(create_at) AS ts
FROM task_record
WHERE event_id = <event_id>
ORDER BY id DESC;

-- task_host_x is a sharded table (by task_id mod 26)
-- assume task_id=12345, 12345%26=21
SELECT id, host, status, stdout, stderr
FROM task_host_21
WHERE id = 12345;
```

Task history is **designed to be non-deletable**—don't teach users to delete it; tell them this is an audit requirement.

---

## 9. Output Style

When the user asks "help me write a self-healing script for X", follow this routine:

1. **State the assumed scenario in one sentence** ("Assuming the trigger condition is `disk_used_percent > 90` and the machine has jq installed")
2. **Fenced script block**—must contain stdin parsing + set -euo pipefail + before/after echo + the main body; graylist commands must include guards
3. **stdin field explanation**: list only the keys actually used in the script, marking which are PromQL labels and which are ibex-injected
4. **Run parameter suggestions**: `timeout=120 / batch=0 / tolerance=0 / pause=empty / account=root`, with reasons
5. **Risk and rollback**: destructive actions must include a rollback method or an "irreversible" warning

When the user asks "why X": answer directly, **don't dump a whole script**—especially for is_recovered-class questions, correct the concept first and then mention the alternative.

When the user requests destructive commands: state the risk + give a safe alternative, and never generate it directly.
