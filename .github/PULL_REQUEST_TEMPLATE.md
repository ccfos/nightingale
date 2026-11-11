<!--
Thanks for contributing! Please fill in EVERY section below.
PRs that skip the required evidence (screenshots / reproduction steps) will be closed without review.

Read this first:
- Open an issue and discuss BEFORE sending a PR for anything non-trivial.
- Every PR must be verified against a RUNNING Nightingale instance by a human, not only by reading code or running unit tests.
- AI-assisted PRs are welcome, but you must disclose it below and you are still responsible for the evidence.
-->

**What type of PR is this?**
<!-- Pick ONE and delete the others. -->
- [ ] Bug fix
- [ ] New feature
- [ ] Refactor / docs / chore / tests (no behavior change)

**What this PR does / why we need it**:
<!--
"Nice to have" / "You need it" is not a good reason. :)
-->

**Which issue(s) this PR fixes**:
<!--
Usage: `Fixes #<issue number>`. Bug fixes and features MUST link an issue.
-->
Fixes #

**Evidence (REQUIRED for bug fixes and features)**:
<!--
=== Bug fix — all three are required ===
1. Full reproduction path: version, datasource/config involved, exact steps, expected vs actual behavior.
2. Screenshot (or log excerpt) showing the problem BEFORE the fix.
3. Screenshot (or log excerpt) showing the same path AFTER the fix.

=== New feature — required ===
1. Screenshot(s) of the new page / form / behavior running in Nightingale.
2. How you tested it (steps, datasource, config).

Paste images directly into this box.
-->

Reproduction steps:

Before:

After (or the new feature page):

**Checklist**:
- [ ] I ran this change on a real Nightingale instance myself and attached screenshots above.
- [ ] `go build ./...` and `go test ./...` pass locally.
- [ ] If `integrations/**` changed: `go run ./cmd/integrations-alert-action check` and `go run ./cmd/integrations-i18n check -scope all` pass.
- [ ] If DB schema changed: gorm struct AND `docker/initsql/a-n9e.sql`, `docker/sqlite.sql`, `docker/migratesql/migrate.sql` are updated.
- [ ] AI tools were used to write this PR: yes / no (state which).

**Special notes for your reviewer**:
