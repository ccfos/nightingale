# n9e-cli Design Decisions

A command-line tool that lets AI agents such as Cursor, Codex, and Claude work efficiently with Nightingale alert rules and alert events.

## 1. Choosing an approach: CLI first, MCP as a complement

### 1.1 Why the CLI

| Aspect | CLI | MCP |
|------|-----|-----|
| Token cost | Consumed on demand, only when a command runs | The tool schemas are always injected into the context and cost tokens even when unused |
| Token efficiency | 4–32× cheaper than MCP (measured in the industry) | Schema bloat is severe; 15 tools cost roughly 3000+ tokens |
| Reliability | 100% (a stateless binary, no connection issues) | ~72% (TCP timeouts, dropped connections, and so on) |
| Progressive disclosure | The agent discovers commands by running `--help` on demand | Every tool definition is loaded into the system prompt at once |
| Composability | Pipes, jq, and xargs work naturally | Locked inside the JSON-RPC protocol |

### 1.2 Where MCP is still valuable

- Environments with no shell access (agents that can only call APIs)
- Multi-tenant authentication scenarios
- When structured audit logs are required
- When 50+ tools need dynamic discovery

### 1.3 A hybrid strategy

The CLI and the MCP server (the existing [n9e-mcp-server](https://github.com/n9e/n9e-mcp-server)) share the same underlying API-calling library and serve as entry points for different scenarios.

### 1.4 A three-layer granularity model

The API, the CLI, and MCP serve different consumers under different design constraints, so their granularity must differ:

```
Consumer        Granularity   Design constraint
──────────────────────────────────────────────────────────────────────────
MCP Tool        Coarsest      Each tool schema costs 150-800 tokens and is always
                              resident in the context
                              → keep the tool count low (5-12) and make each one count
──────────────────────────────────────────────────────────────────────────
CLI Command     Medium        Discovered on demand via --help, tokens spent only on call
                              → more commands are fine (15-25), each self-contained
──────────────────────────────────────────────────────────────────────────
HTTP API        Finest        The foundation everything else depends on: frontend,
                              backend, CLI, MCP
                              → RESTful, one endpoint per resource, atomic (30-50)
```

#### The same scenario at each granularity

Scenario: look at the critical active alerts of a business group over the last hour, then create a mute rule for one of them.

**API layer (4 calls, atomic operations)**:

```
GET  /api/n9e/busi-groups/mine                                  → list the business groups
GET  /api/n9e/alert-cur-events/list?gid=1&severity=1&hours=1   → query active alerts
GET  /api/n9e/alert-cur-event/42                                → get the alert details
POST /api/n9e/busi-group/1/alert-mutes                          → create the mute rule
```

**CLI layer (2 calls, operation oriented)**:

```bash
n9e-cli alert active list --group-id 1 --severity 1 --hours 1 --json
# --group-name infra works too; the CLI resolves name→ID automatically

n9e-cli mute create --group-id 1 --json < mute.json
```

**MCP layer (1 call, task oriented)**:

```json
{
  "tool": "investigate_alerts",
  "params": {
    "group": "infrastructure",
    "severity": "critical",
    "hours": 1,
    "action": "list_and_summarize"
  }
}
```

#### Granularity comparison

| Aspect | API | CLI | MCP |
|------|-----|-----|-----|
| Design philosophy | Resource oriented | Operation oriented | Task oriented |
| Count | Many (30-50 endpoints) | Medium (15-25 commands) | Few (5-12 tools) |
| What one call accomplishes | One atomic operation | One user intent | One complete scenario |
| ID resolution | The caller's responsibility | Automatic name→ID resolution | Resolved internally |
| Aggregation | None | Light (common combinations) | Heavy (multi-step orchestration) |
| Output control | A fixed JSON schema | Flexible via `--json`/`--quiet`/flags | Returns a summary the agent can reason over directly |
| Pagination | `limit`+`offset` parameters | A `--limit` flag with a sensible default | Handled internally, returns the top N |
| Token cost | Not directly involved | On demand (only when called) | Schema always resident (150-800 tokens/tool) |

#### Design principles per layer

**API (finest granularity, unchanged)**:

Nightingale's existing API granularity is right — RESTful and atomic. It is the foundation layer and needs no changes for the CLI or MCP.

**CLI (medium granularity, ~20 commands)**:

- Most commands map 1:1 to an API, keeping them simple and predictable
- A few high-frequency scenarios aggregate (name→ID resolution, batch operations, import/export)
- Variants are covered by combining flags rather than by adding commands

```bash
# 1:1 with the API (when the agent needs a precise operation)
n9e-cli alert rule get --id 42 --json

# "Smarter" than the API (aggregation plus convenient parameters)
n9e-cli alert active list --group-name infra --severity critical --json

# A composite command (one command = several API calls)
n9e-cli alert rule import --file rules.yaml --group-name infra --dry-run --json
```

**MCP (coarsest granularity, ~8 tools)**:

- Each tool schema costs 150-800 tokens, and past about 20 tools an agent's selection accuracy drops noticeably
- Use an `action` parameter to distinguish CRUD within one tool instead of splitting it into four
- Trim return values semantically — return a summary the agent can reason over directly, not raw JSON
- The industry recommends 5-12 tools per server

```
# Recommended (task oriented, ~8 tools):
query_alerts        → unified query over active and historical alerts, with built-in filtering and summarization
manage_alert_rules  → list / view / create / update rules (distinguished by the action parameter)
manage_mutes        → full CRUD for mute rules
query_targets       → query monitored targets and their status
search              → cross-resource search (for when the agent is not sure what it is looking for)
get_overview        → system overview (alert statistics, severity distribution, and so on)
```

#### How the three layers relate

```
┌──────────────────────────────────────────────────────┐
│  MCP Tool (task layer)                               │
│  ~8 coarse-grained tools for AI conversations        │
│  Each tool orchestrates several CLI/API calls        │
├──────────────────────────────────────────────────────┤
│  CLI Command (operation layer)                       │
│  ~20 medium-grained commands for the shell           │
│  Each command may aggregate 1-3 API calls            │
├──────────────────────────────────────────────────────┤
│  HTTP API (resource layer)                           │
│  ~40 fine-grained endpoints for every client         │
│  Atomic operations, RESTful                          │
└──────────────────────────────────────────────────────┘
```

The CLI and MCP both call the same HTTP API; they are facade layers on top of it, aggregating to different degrees for their respective consumers. Neither touches the database directly.

## 2. Design principles

### 2.1 Layered subcommands with a noun-verb hierarchy

It follows the `noun verb` pattern (like `docker container ls` and `gh pr create`), so an agent can discover the tree with `--help`:

```
n9e-cli --help          → see all resources (nouns)
n9e-cli alert --help    → see the sub-resources under alert
n9e-cli alert rule --help → see every operation on rule (verbs)
```

### 2.2 Structured output

- Every command supports `--json` output
- JSON goes to stdout; logs and progress go to stderr
- Fields are flattened, avoiding deep nesting
- Types are consistent: timestamps are always Unix epoch or ISO 8601
- Streaming output uses NDJSON (one JSON object per line)

### 2.3 Exit codes as control flow

```
0 = success
1 = generic error
2 = argument error (incorrect usage)
3 = resource not found
4 = insufficient permissions
5 = conflict (the resource already exists)
```

Combined with structured error output:

```json
{"error": "not_found", "message": "alert rule 42 not found", "suggestion": "run n9e-cli alert rule list --group-id 1"}
```

### 2.4 Help text as documentation

The `--help` of every command must include:

- Clearly marked required / optional parameters
- At least two realistic examples
- A mention of the `--json` flag
- A concise one-line description

```
n9e-cli alert rule list --help
List alert rules for a business group.

Usage:
  n9e-cli alert rule list [flags]

Flags:
  --group-id int     Business group ID (required)
  --disabled         Only show disabled rules
  --json             Output as JSON
  --limit int        Max results (default: 20)
  --offset int       Pagination offset

Examples:
  n9e-cli alert rule list --group-id 1 --json
  n9e-cli alert rule list --group-id 1 --disabled --limit 10 --json
```

### 2.5 AI-agent-friendly features

- `--dry-run`: preview the change and emit a structured diff
- `--yes` / `--force`: skip confirmation prompts (agents cannot answer interactively)
- `--quiet`: print bare values only, suitable for pipes
- `--limit` / `--offset`: pagination, so a single call never returns every record
- Idempotent operations: `create --if-not-exists`, or `apply` semantics
- Automatic non-interactive terminal detection: when stdin is not a TTY, skip the confirmation or fail

### 2.6 Composability

```bash
# Agents naturally compose these commands
n9e-cli alert active list --json | jq '.[] | select(.severity == 1)'

# Built-in filtering saves tokens
n9e-cli alert active list --severity 1 --json

# Batch operations reduce the number of calls
n9e-cli alert rule delete --ids 1,2,3 --yes
```

### 2.7 Actionable error messages

An error message must contain:
- An error code / type (a parseable string such as `"image_not_found"`)
- The failing input (echo the parameters back)
- A suggested next step
- A distinction between transient and permanent errors

### 2.8 Resource identifier strategy

#### Current state

Every core Nightingale model uses a MySQL `int64` auto-increment primary key, and the human-readable identifier fields are uneven:

| Model | Primary key | Has a Name? | Name uniqueness |
|------|------|-----------|-------------|
| AlertRule | `int64` auto-increment | Yes | `(group_id, name)` unique at the application layer, no DB constraint |
| AlertCurEvent | `int64` (taken from HisEvent.Id) | No (there is a RuleName snapshot) | — |
| AlertHisEvent | `int64` auto-increment | No (there is a RuleName snapshot) | — |
| Dashboard | `int64` auto-increment | Yes | `(group_id, name)` DB UNIQUE |
| AlertMute | `int64` auto-increment | **No** (only a Note) | — |
| AlertSubscribe | `int64` auto-increment | Yes | No uniqueness constraint |
| EventPipeline | `int64` auto-increment | Yes | No uniqueness constraint |
| BusiGroup | `int64` auto-increment | Yes | **Globally DB UNIQUE** |

In addition, AlertCurEvent and AlertHisEvent have a `hash` field (`rule_id + vector_key`) that can locate events with the same rule and the same label combination.

#### Decision: no DB schema change; resolve intelligently in the CLI layer

**Adding a UUID / slug column to every table is not recommended**, because:
- Nightingale is a mature project with many production users, and schema changes carry migration cost
- Names not being unique within a business group is a reasonable design (different teams may have rules with the same name)
- The frontend, the API, and the MCP server would all have to adapt

**A "smart translation layer" in the CLI is recommended instead**, following what `gh` (the GitHub CLI) and `kubectl` do:

#### Parameter acceptance rules

Every get/update/delete command accepts both `--id` and `--name`:

```bash
# Precise: by ID (for agents and scripts)
n9e-cli alert rule get --id 42

# Friendly: by name (for humans and agents)
n9e-cli alert rule get --name "CPU Usage Alert"

# Disambiguated: name plus business group scope
n9e-cli alert rule get --name "CPU Alert" --group-name "Infrastructure"
```

#### Automatic business group name resolution

Every command that needs a `group_id` accepts both `--group-id` and `--group-name`:

```bash
# The traditional way: look up the group ID first
n9e-cli alert rule list --group-id 1 --json

# Automatic CLI resolution: --group-name → look up BusiGroup internally → obtain the ID
n9e-cli alert rule list --group-name "Infrastructure" --json
```

BusiGroup.Name is globally DB UNIQUE, so name→ID resolution is safe.

#### Name matching logic

```
Resolution priority: --id > --name
Matching rules:
  exactly one match  → return the result
  multiple matches   → return the list plus a hint to use --id or add --group-name to disambiguate
  no match           → exit code 3 plus a suggestion to use the list command
```

#### Identifying alert events

AlertCurEvent already has a `hash` field, and the CLI supports both:

```bash
n9e-cli alert active get --id 12345        # by event ID
n9e-cli alert active get --hash "abc123"   # by hash (events of the same rule with the same labels share a hash)
```

#### Models without a Name (AlertMute)

AlertMute only has a `note` field, which is unsuitable as an identifier. The CLI strategy:

- Make the list command filter well (`--group-name`, `--active`, `--severity`)
- The typical agent flow is to list first and extract an ID from the output for the follow-up operation
- CLI output always includes the `id` field so the agent can extract it

#### Always include the ID in the output

The JSON output of every list/get command must contain the `id` field first, so an agent can pull an ID out of a list result and use it for a subsequent get/update/delete:

```json
[
  {"id": 42, "name": "CPU Usage Alert", "group_id": 1, "severity": 1, "disabled": 0},
  {"id": 43, "name": "Memory Usage Alert", "group_id": 1, "severity": 2, "disabled": 0}
]
```

## 3. Command tree

```
n9e-cli
├── alert
│   ├── rule
│   │   ├── list          # list alert rules
│   │   ├── get           # get rule details
│   │   ├── create        # create a rule
│   │   ├── update        # update a rule
│   │   └── delete        # delete a rule
│   ├── active
│   │   ├── list          # list active alert events
│   │   └── get           # get active alert details
│   └── history
│       ├── list          # query historical alerts
│       └── get           # historical alert details
├── mute
│   ├── list              # list mute rules
│   ├── get               # mute rule details
│   ├── create            # create a mute rule
│   ├── update            # update a mute rule
│   └── delete            # delete a mute rule
├── subscribe
│   ├── list              # list alert subscriptions
│   ├── get               # subscription details
│   ├── create            # create a subscription
│   └── update            # update a subscription
├── notify
│   ├── rule
│   │   ├── list          # list notification rules
│   │   └── get           # notification rule details
│   └── channel
│       └── list          # list notification channels
├── target
│   ├── list              # list monitored targets
│   └── get               # monitored target details
├── datasource
│   └── list              # list data sources
├── event-pipeline
│   ├── list              # list event pipelines
│   ├── get               # pipeline details
│   └── runs              # view execution records
└── busi-group
    └── list              # list business groups
```

## 4. The corresponding Nightingale API routes

Under the hood the CLI calls Nightingale's existing HTTP APIs:

| CLI command | Nightingale API route | Handler file |
|----------|---------------|--------------|
| `alert rule list` | `GET /api/n9e/busi-group/:id/alert-rules` | `router_alert_rule.go` |
| `alert rule get` | `GET /api/n9e/alert-rule/:arid` | `router_alert_rule.go` |
| `alert active list` | `GET /api/n9e/alert-cur-events/list` | `router_alert_cur_event.go` |
| `alert active get` | `GET /api/n9e/alert-cur-event/:eid` | `router_alert_cur_event.go` |
| `alert history list` | `GET /api/n9e/alert-his-events/list` | `router_alert_his_event.go` |
| `alert history get` | `GET /api/n9e/alert-his-event/:eid` | `router_alert_his_event.go` |
| `mute list` | `GET /api/n9e/busi-group/:id/alert-mutes` | `router_mute.go` |
| `subscribe list` | `GET /api/n9e/busi-group/:id/alert-subscribes` | `router_alert_subscribe.go` |
| `notify rule list` | `GET /api/n9e/busi-group/:id/notify-rules` | `router_notify_rule.go` |
| `target list` | `GET /api/n9e/targets` | `router_target.go` |
| `datasource list` | `GET /api/n9e/datasource/list` | `router_datasource.go` |
| `busi-group list` | `GET /api/n9e/busi-groups/mine` | `router_busi_group.go` |

The handler files live under `center/router/`.

## 5. Token optimization strategy

1. **On-demand disclosure in the CLI**: the agent only calls `--help` when it needs to (~200-500 tokens), unlike MCP which preloads every schema (~3000+ tokens per turn)
2. **An AGENTS.md guide file**: place a roughly 800-token cookbook at the project root describing the core usage. A hand-written guide works better than an LLM-generated one and cuts inference cost by about 20%
3. **Flat JSON**: fewer tokens to parse
4. **Built-in pagination**: `--limit` defaults to 20 records, so huge result sets are never returned
5. **Built-in filtering**: parameters such as `--severity` and `--status` save the agent from a second pass through jq

## 6. Technology choices

- **Framework**: [Cobra](https://github.com/spf13/cobra) (used by kubectl, gh, and docker)
- **Configuration**: [Viper](https://github.com/spf13/viper) (API address, token, and so on)
- **Authentication**: the `N9E_API_URL` and `N9E_TOKEN` environment variables plus the `~/.n9e-cli.yaml` configuration file
- **Language**: Go (the same as the main Nightingale project, so type definitions can be reused)

## 7. Configuration management

```yaml
# ~/.n9e-cli.yaml
api_url: http://localhost:17000
token: your-api-token
default_output: json
```

Environment variables can override it:

```bash
export N9E_API_URL=http://localhost:17000
export N9E_TOKEN=your-api-token
```

Precedence: command-line flags > environment variables > configuration file

## 8. References

- [Writing CLI Tools That AI Agents Actually Want to Use](https://dev.to/uenyioha/writing-cli-tools-that-ai-agents-actually-want-to-use-39no)
- [MCP vs CLI: Benchmarking AI Agent Cost & Reliability](https://www.scalekit.com/blog/mcp-vs-cli-use)
- [MCP vs CLI for AI Agents: I Measured the Same Tool Both Ways](https://afrozeamjad.com/writing/mcp-vs-cli-token-benchmark/)
- [Building CLI for Agents](https://docs.hiroleague.com/ai-coding-bible/building-cli-for-agents)
- [Your MCP server is a monolith. Here's how to fix it](https://www.channel.tel/blog/mcp-server-monolith-fix-tool-scoping) — MCP tool granularity and the 5-12 tools per server principle
- [How MCP Tool Definitions Inflate Your AI Agent Token Costs](https://docs.bswen.com/blog/2026-04-24-mcp-token-overhead/) — measured data behind the 150-800 tokens per tool schema
- [MCP Token Optimization: 4 Approaches Compared](https://stackone.com/blog/mcp-token-optimization/)
- [How Granular Should You Design APIs?](https://nordicapis.com/how-granular-should-you-design-apis/) — API granularity design principles
- [Levels of API granularity](https://world.hey.com/boriseetgerink/levels-of-api-granularity-48e0967d) — the Resource / Aggregate / Facade three-layer model
- [n9e-mcp-server](https://github.com/n9e/n9e-mcp-server)
