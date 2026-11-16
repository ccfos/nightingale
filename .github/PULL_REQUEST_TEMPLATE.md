<!--
Thanks for contributing! Please fill in EVERY section below.
A bot checks this description. PRs that change code without a linked issue or without
the evidence below are CLOSED AUTOMATICALLY; fixing the description reopens them.

Read CONTRIBUTING.md first:
- Open an issue BEFORE sending a PR. If you have fewer than 3 merged PRs in this repository,
  wait for a maintainer to respond on the issue before sending code.
- Every PR must be verified against a RUNNING Nightingale instance by a human, not only by reading code or running unit tests.
- AI-assisted PRs are welcome, but you must disclose it below and you are still responsible for the evidence.
- Keep it small and focused: with fewer than 3 merged PRs you can have 2 open PRs at a time, each up to 10 files / 500 changed lines
  (docs and tests not counted). Agree on anything larger in the issue first.
- Only PRs that touch nothing but docs / tests / CI files are exempt, whatever type is ticked below.
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
Usage: `Fixes #<issue number>`. Every PR that changes code MUST link an issue of this repository.
-->
Fixes #

**Evidence (REQUIRED for every PR that changes code)**:
<!--
=== Bug fix — all three are required ===
1. Full reproduction path: version, datasource/config involved, exact steps, expected vs actual behavior.
2. Screenshot showing the problem BEFORE the fix (for logs or API output, a screenshot of the terminal).
3. Screenshot showing the same path AFTER the fix.

=== New feature — required ===
1. Screenshot(s) of the new page / form / behavior running in Nightingale.
2. How you tested it (steps, datasource, config).

=== Refactor / chore that changes code — required ===
1. Screenshot(s) of the affected path still working on a running instance.

Paste or drag the images into this box on github.com. External image links and images
committed to a branch are NOT accepted.
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
