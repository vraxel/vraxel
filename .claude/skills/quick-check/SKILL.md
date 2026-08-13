---
name: quick-check
description: Fast pre-commit check for this repo -- verifies the lint/type gates are still intact, then checks only what changed. Use before committing, or when asked to "check", "verify", "跑一下检查", or after finishing an edit. Much faster than `make check`, which owns Go tests and full-repo lint.
---

# quick-check

```bash
./scripts/quick-check.sh
```

Seconds, not minutes. Run it after finishing an edit and before committing.

## What it checks, and why in that order

**1. Gate integrity** — is the safety net still there?

This runs first because a broken gate is invisible. In Aug 2026 ~340 lint
violations accumulated across this codebase's sibling repos while CI stayed
green the whole time: `eslint .` exits 0 on warnings, and the rules had been
pinned to `'warn'` "until the migration lands". **A gate that stopped gating
looks exactly like a gate with nothing to report.**

- pinned lint rules still resolve to `error` (`scripts/check-lint-config.mjs`)
- every workspace package has `lint` with `--max-warnings 0`, and a `typecheck`
- the pre-commit hook is installed

**2. Change correctness** — will what I just wrote pass?

Scoped to changed files where the tool allows it. Falls back to unstaged files
when nothing is staged, so it is useful mid-edit.

- Go: `gofmt -s`, `go build`, layer guard
- TS: `pnpm -r typecheck` (project-wide, tsc cannot be scoped) + `eslint` on
  the changed files only, grouped by owning workspace package
- reminds you to `make generate` when SQL / API types changed
- rejects a migration filename that is not `YYYYMMDDHHMMSS_` (8-digit dates
  collide when two land the same day)

## What it does NOT do

Go tests and full-repo lint. Those are `make check`, and they are the gate CI
runs. **Passing quick-check is not permission to skip `make check` before you
push anything you care about.**

## Reading the output

- `FAIL` on a gate-integrity line means the safety net itself regressed — treat
  it as more urgent than a failing test. Someone downgraded a rule, dropped a
  package's lint script, or turned a rule off. Fix the gate, do not work around
  it.
- `FAIL` on a changed-files line is an ordinary failure; the fix hint is on the
  line below.
- `warn` is advisory (hook not installed, generated artifacts may be stale).

## If a rule genuinely needs an exception

Put `// eslint-disable-next-line <rule>` on the offending line with the reason
above it. Never downgrade the rule in the config: the disable is greppable,
shows up in review, and dies with the code it excuses. A blanket downgrade is
none of those, which is the whole reason this skill exists.
