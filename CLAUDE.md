# CLAUDE.md

Nightingale (n9e) is an open-source alerting/monitoring platform (Go, gin + gorm). This file lists only what cannot be learned quickly by reading the code.

## Cross-repo constraints

- The commercial fork `flashcatcloud/n9e-plus` imports this module, so **exported signatures it calls (e.g. `alert.Start`, `centerrt.New`, `BuildNotifyContext`, `center.Initialize`-style entry points) are effectively public API**. Add fields/setters instead of changing parameter lists (see comments in `center/center.go` and `alert/alert.go`).
- The frontend is a separate repo (`n9e/fe`), embedded as a statik bundle. `front/statik/statik.go` is committed, so `go build ./...` works without running `fe.sh`.

## Commands

```bash
go build ./...                       # ~1 min cold; also verifies tests compile
go test ./...                        # in-memory sqlite (github.com/glebarez/sqlite), no external services
./n9e --configs etc.local            # etc.local* are git-ignored per-developer configs; etc/ is the shipped default

# CI gates, run when touching integrations/**
go run ./cmd/integrations-alert-action check     # every builtin alert rule needs a non-empty `action` annotation
go run ./cmd/integrations-alert-action update    # tighten integrations/alert_action_baseline.json after fixing rules
go run ./cmd/integrations-i18n check -scope all  # en_US rendering must contain no Chinese
```

There is no lint config; the two integration checks are the only code gates in CI.

## Rules that span several files

- **Center vs edge/alert.** `n9e` (center) owns the DB; `n9e-edge` and `n9e-alert` have none. Model functions branch on `ctx.IsCenter`: gorm directly, or HTTP to the center via `pkg/poster` against `/v1/n9e/...`. A new model read path that edge/alert needs must implement both branches, and the center must expose the matching `/v1/n9e` route.
- **memsto caches.** Alert evaluation, dispatch and routers read from `memsto/` caches, never the DB on the hot path. Adding a cached table means: model `Stat()` + `GetsAll()` (both ctx branches), a cache in `memsto`, and threading it through `alert.Start`. Change detection uses `count/max(update_at)`; a row count alone misses "one created, one deleted" between two syncs, so tables without `update_at` whose rows are never edited (e.g. `user_token`) compare `count + max(id)`.
- **Caches change only through their periodic sync.** Don't patch a cache from an API handler to make a change "take effect immediately": it only covers the instance that served the request, so behind a load balancer the same token or config works on one node and fails on another, and the next sync's full replace can undo it. Accept the sync delay and fix the sync's change detection instead. The one exception is the Skill Gateway's `UserTokenCache.Inject`, whose token must authenticate within the same request.
- **Schema.** `models/migrate/migrate.go` runs AutoMigrate on center startup, and `docker/initsql/a-n9e.sql`, `docker/sqlite.sql`, `docker/migratesql/migrate.sql` are maintained by hand. A new column/table must land in the gorm struct and all three SQL files. MySQL/PostgreSQL/SQLite differences (text sizes, unique index limits) are handled explicitly in `MigrateTables`; long `string` fields need an explicit `gorm:"type:..."` or AutoMigrate drifts or fails.
- **Models for the frontend.** Nullable JSON arrays must be normalized to `[]` on the way out (see `models/alert_rule_null_compat_test.go`).
- **Alert pipeline order** (`alert/`): `eval` (query, produce `AlertCurEvent`) → `process` (mute, relabel, dedupe against `alert_cur_event`) → `queue` → `dispatch` (subscribe, notify rules, event pipelines) → `sender`.
- **Event pipeline processors** self-register via `models.RegisterProcessor` in their package `init`; a new processor package must also be blank-imported in `alert/pipeline/pipeline.go`.

## Conventions

- Prefer the simplest code that is correct. Enforce ownership inside the write itself (`Where("id = ? and username = ?", ...)` and check `RowsAffected`) instead of reading the row first and then writing.
- New user-facing backend strings go through `pkg/i18nx`.
- Commit messages and new code comments are in English, even though existing comments are often Chinese.
- Tests that need a DB open `sqlite.Open(":memory:")` and call the relevant `AutoMigrate`, following the existing `*_test.go` in the same package; don't add a shared harness.
- A new endpoint family gets a doc in `doc/api/*.md`.
