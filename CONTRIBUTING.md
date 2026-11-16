# Contributing to Nightingale

Thanks for helping improve Nightingale. Review time is limited, so pull requests have to arrive discussed, verified and small enough to review. A bot enforces the rules below and **closes pull requests that do not meet them**; fixing the pull request reopens it.

## 1. Open an issue first

Every pull request that changes code must link an issue of this repository in its description (`Fixes #123`).

- If no issue exists, [open one](https://github.com/ccfos/nightingale/issues/new/choose) and describe the bug or the change you propose.
- If you have **fewer than 3 merged pull requests** in this repository, wait until a maintainer has replied to or labeled the issue before sending code. This avoids work on changes that will not be accepted or that need a different approach.
- Issues declined by maintainers (`not planned`, `wontfix`, `invalid`) cannot be used.

## 2. Verify the change on a running instance

Every pull request that changes code must be run on a real Nightingale instance by a human. Reading the code or passing unit tests is not enough. The description must contain:

- **Bug fix**: the full reproduction steps and at least two screenshots, one of the problem before the fix and one of the same path after the fix. For logs or API output, take a screenshot of the terminal.
- **New feature**: screenshot(s) of the new page or behavior, and how you tested it.
- **Refactor / chore**: screenshot(s) of the affected path still working.
- The ticked checkbox "I ran this change on a real Nightingale instance myself".

Upload the screenshots by pasting or dragging them into the description on github.com. External image links and images committed to a branch are not accepted.

AI-assisted pull requests are welcome. Disclose the tool in the checklist; the requirements above apply all the same.

## 3. Keep pull requests small and few

Files under documentation and tests do not count towards the size.

| | Open pull requests at a time | Size of one pull request |
|---|---|---|
| Fewer than 3 merged pull requests | 2 | 10 files, 500 changed lines |
| 3 or more merged pull requests | no limit | 50 files, 10000 changed lines |

A larger change, such as a new datasource, needs its scope agreed in the issue first. A maintainer then lifts the size limit for that pull request with the `skip-size-limit` label.

## What is exempt

Pull requests that touch only documentation (`*.md`, `doc/`), tests (`*_test.go`, `testdata/`) or CI files (`.github/`) are not checked. The bot decides this from the changed files, not from the PR type you tick.

## If the bot closes your pull request

It adds a `needs-issue`, `needs-evidence` and/or `over-limit` label and a comment listing what is missing.

1. Resolve the listed points. For `needs-issue`, continue the discussion in the issue until a maintainer responds.
2. Edit the pull request description. Any edit re-runs the check, and the pull request is reopened once it passes.
3. Do not force-push or delete the branch while the pull request is closed: GitHub cannot reopen it afterwards and you would have to open a new one.

Maintainers can waive the whole check with the `skip-evidence-check` label, or only the size limit with `skip-size-limit`.

## Before you push

```bash
go build ./...
go test ./...

# when touching integrations/**
go run ./cmd/integrations-alert-action check
go run ./cmd/integrations-i18n check -scope all
```

A schema change must land in the gorm struct and in `docker/initsql/a-n9e.sql`, `docker/sqlite.sql` and `docker/migratesql/migrate.sql`.

Commit messages and code comments are written in English. Please follow the [Code of Conduct](CODE_OF_CONDUCT.md); report security issues through the [security policy](SECURITY.md) instead of a public issue.
