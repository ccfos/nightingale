---
name: categraf-deploy-guide
description: Answers "how do I deploy the categraf collector". Trigger scenarios: the user asks "how to install categraf / how to deploy categraf / run categraf with Docker / install categraf on Windows / how to register categraf as a system service / report categraf to Nightingale / how to write categraf config.toml / how to verify that categraf is collecting data". Covers binary + systemd, Docker, Windows, K8s tips, key configuration, and common verification commands. This skill is instructional/guidance-oriented, calls no tools, and outputs ready-to-paste commands and configuration snippets.
tags:
  - internal
---

# Categraf Deployment Guide

## Scope

**Enter this skill**:
- "how to install / how to deploy / how to start categraf"
- "run categraf on Docker / Kubernetes / Windows / Linux"
- "how to register categraf as a systemd service / start on boot"
- "report categraf to Nightingale / how to write config.toml / what to put in the writer URL"
- "how to verify that categraf is collecting data"
- "categraf is installed but Nightingale can't see the machine" → **do not enter this skill**, route to `host-onboard-diagnose`

**Do not enter this skill**:
- Diagnosing onboarding failures after install → `host-onboard-diagnose`
- Abnormal metrics after onboarding → `host-health-diagnose`
- Configuring collection for a specific input plugin (mysql / redis / nginx / snmp, etc.) → guide the user to read the comments under `conf/input.<name>/`; this skill only covers "get it installed + get it connected"

## One-Sentence Principle

**categraf deployment = three actions**: get the binary → correctly write the `[[writers]]` section of `config.toml` → start it and verify. Everything else is a variant of these three steps (containerization, systemd management, Windows service, etc.).

## Deployment Methods Quick Reference

| Scenario | Recommended method | Notes |
|---|---|---|
| Linux physical machine / VM | **binary + systemd** | v0.3.35+ has a built-in `--install` one-liner to register |
| Linux but no root | binary + `--user --install` | supported in v0.4.5+ |
| Container / orchestration environment | **Docker** or K8s DaemonSet | collecting host metrics requires mounting `/proc /sys` |
| Windows | `categraf.exe --win-service-install` | or run in the background with `win_run.bat` |
| macOS (development/debugging) | run the binary directly | not recommended for production |

Operating system support: Linux kernel 2.6.32+ / Windows 10+ or Server 2008+ / macOS 10.15+.

## Quickest Path: One-Click Install

If the nightingale server is v9+ and bundles categraf, a single command on the target
host does everything below (download, unpack, rewrite the reporting address, register
and start the systemd service):

```bash
curl -sSfL 'http://N9E_HOST:17000/api/n9e/agents/categraf/install.sh' | sudo bash -s -- --server 'http://N9E_HOST:17000'
```

The address is filled in by the server, so nothing has to be edited by hand. The same
command is available from the machine list page ("Install Categraf"), which also waits
for the host to report. Useful flags: `--force` (reinstall/upgrade), `--dir <path>`,
`--auth user:pass` (when the agent API requires basic auth). Note the `bash -s --`
form: `| sudo bash --force` silently drops the flag.

If the server is older, does not bundle the package, or the host is not Linux, fall
back to the manual steps below.

## Step 1: Get the Binary

Download URL: https://github.com/flashcatcloud/categraf/releases

Package naming convention: `categraf-{version}-{os}-{arch}.tar.gz`, for example `categraf-v0.3.35-linux-amd64.tar.gz`. **Confirm the architecture first** (`uname -m`: choose amd64 for x86_64, arm64 for aarch64).

Directory structure after extraction:

```
categraf/
├── categraf              # main program
└── conf/
    ├── config.toml       # main config (hostname / writers / heartbeat)
    └── input.*/          # sub-config directories for the various collection plugins
```

## Step 2: Correctly Write `conf/config.toml`

Minimal working configuration (reporting to Nightingale n9e):

```toml
[global]
hostname = ""                  # leave empty to let categraf auto-detect the hostname; specify explicitly when multiple machines share a name
interval = 15                  # global collection frequency (seconds)
providers = ["local"]

[global.labels]
# region = "shanghai"          # global extra labels, optional
# env = "prod"

[[writers]]
url = "http://N9E_HOST:17000/prometheus/v1/write"
basic_auth_user = ""
basic_auth_pass = ""
timeout = 5000
dial_timeout = 2500
max_idle_conns_per_host = 100

[heartbeat]
enable = true                  # must be enabled, otherwise this machine won't appear in the Nightingale machine list
url = "http://N9E_HOST:17000/v1/n9e/heartbeat"
interval = 10
```

**Key fields**:
- `[[writers]].url`: n9e's remote write receiving endpoint; the path is fixed at `/prometheus/v1/write`, and the default port is 17000.
- `[heartbeat].enable=true` + `[heartbeat].url`: heartbeat reporting, which lets the "machine list" see this machine; **if missing → installed but invisible**.
- `[global].hostname`: leave empty to use the system hostname; if the `hostname` command in a container/cloned machine returns a duplicated name like `localhost.localdomain`, **you must specify it explicitly**, otherwise different machines will overwrite each other.
- `[global].labels`: global tags; setting good distinguishing dimensions (region / env / idc) makes building dashboards and alerts much easier later.

⚠️ **Do not set `omit_hostname` to true** — it strips the `ident` label, with the result that the machine list shows OS/agent_version all as unknown (a common pitfall).

## Step 3: Start and Manage

### Option A: Linux + systemd (v0.3.35 and above, recommended)

```bash
cd /path/to/categraf
sudo ./categraf --install      # register as a system service
sudo ./categraf --start        # start
sudo ./categraf --status       # check status
sudo ./categraf --stop         # stop
```

Non-root user (v0.4.5+):

```bash
./categraf --user --install
./categraf --user --start
```

systemd resource limits (edit `/etc/systemd/system/categraf.service`):

```ini
[Service]
CPUQuota=200%
MemoryLimit=1G
```

After editing, run `systemctl daemon-reload && systemctl restart categraf`.

### Option B: Docker

Minimal (collects only in-container/network metrics):

```bash
docker run -td \
  --name categraf \
  --restart=always \
  -e TZ=Asia/Shanghai \
  -v /home/flashcat/categraf/conf/:/etc/categraf/conf \
  flashcatcloud/categraf:latest
```

**Collecting host CPU/memory/disk** (the most common case; you must mount the host's `/proc` `/sys` and use host networking):

```bash
docker run -td \
  --name categraf \
  --restart=always \
  --network=host \
  -e TZ=Asia/Shanghai \
  -e HOST_PROC=/hostfs/proc \
  -e HOST_SYS=/hostfs/sys \
  -e HOST_MOUNT_PREFIX=/hostfs \
  -v /home/flashcat/categraf/conf/:/etc/categraf/conf \
  -v /proc:/hostfs/proc:ro \
  -v /sys:/hostfs/sys:ro \
  flashcatcloud/categraf:latest
```

Resource limits: append `--cpus=2 --memory=1g`.

### Option C: Windows

Register as a service:

```cmd
categraf.exe --win-service-install
categraf.exe --win-service-start
categraf.exe --win-service-stop
categraf.exe --win-service-uninstall
```

Or run in the background with a script:

```cmd
win_run.bat start
win_run.bat stop
```

Windows does not support `kill -HUP` reload; after changing the config, use `--win-service-stop` + `--win-service-start`.

### Option D: Kubernetes

On K8s you typically use a DaemonSet (one per Node to collect host metrics) + a ConfigMap (to distribute `config.toml` uniformly). The official release package includes a `k8s/` directory with sample YAML; just run `kubectl apply -f k8s/`. **The two things you must change**:
1. The `[[writers]].url` and `[heartbeat].url` in the ConfigMap should point to your n9e;
2. If you use multiple clusters, add `cluster = "xxx"` to `[global.labels]` to distinguish them.

## Step 4: Verify

### 4.1 Single-plugin debugging (does not actually report, prints once)

```bash
./categraf --test --inputs cpu              # test just one
./categraf --test --inputs cpu:mem:disk     # use colons to separate multiple
```

If metric lines appear on stdout, local collection is OK; if it errors out (failed to connect to mysql / insufficient permissions, etc.), get past this step before starting the service.

### 4.2 Reload config (without restarting the process)

```bash
kill -HUP `pidof categraf`   # Linux-only, not supported on Windows
```

### 4.3 Verify on the Nightingale side

- Open n9e in a browser → **Infrastructure / Machine List**; you should be able to see this `ident`, with OS/CPU/agent_version all not unknown.
- If it doesn't appear → route to `host-onboard-diagnose` and run the 5-stage troubleshooting; don't guess blindly here.

### 4.4 Self-upgrade (v0.3.36+)

```bash
./categraf --update --update_url https://github.com/flashcatcloud/categraf/releases/download/vX.Y.Z/categraf-vX.Y.Z-linux-amd64.tar.gz

# On an intranet, point --update_url at nightingale's own bundled package instead:
./categraf --update --update_url 'http://N9E_HOST:17000/api/n9e/agents/categraf/download?arch=amd64'
```

## Common Pitfalls (one-liner version)

| Symptom | Cause | Fix |
|---|---|---|
| Not visible in the machine list | `[heartbeat].enable=false` or `url` not changed | edit the toml, restart |
| Machine list shows OS=unknown | `omit_hostname=true` or categraf version < v0.2.35 | turn off omit_hostname, upgrade the version |
| Multiple machines overwrite each other | duplicated hostname (localhost / VM clone) | explicitly set `[global].hostname` or an ident shell |
| Can't collect host CPU in Docker | `/proc /sys` not mounted and `HOST_*` env not set | use the "collect host" command above |
| Writing to prom returns 499 / queue full | n9e backend ingest queue is full | tune n9e write concurrency, or reduce the categraf interval |
| TLS x509 unknown authority | a self-signed certificate is used without installing the CA | add `insecure_skip_verify = true` to the writer section (test only), or install the CA |
| BasicAuth 401 | n9e has BasicAuth enabled but categraf is not configured | fill in `basic_auth_user/pass` |

## Output Style

- **Clarify before giving a solution**: when the user hasn't stated the OS / deployment form, first ask "Do you want to install it on a Linux physical machine, Docker, or K8s? What is the n9e access URL?"
- **Give a ready-to-paste command for every suggestion** — don't write "tweak the config", write "change `[[writers]].url` in `config.toml` to `http://your-n9e:17000/prometheus/v1/write`".
- **Configuration examples must include placeholder-replacement notes** (such as `N9E_HOST`) so the user knows what to change.
- Reply in the user's language (Chinese for Chinese users, English for English users).
- After deployment, **always remind them to run `--test --inputs cpu` once** and "take a look at the Nightingale machine list", otherwise there's no way to know right then whether the install is working.
- If the user says "I finished installing but can't see the machine" → don't give more deployment instructions; route them directly to `host-onboard-diagnose`.
