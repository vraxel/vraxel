#!/usr/bin/env bash
# Pre-commit sanity in seconds, not the full `make check`.
#
# Two things, in this order, because they fail for different reasons:
#
#   1. Gate integrity -- is the safety net itself still there? This is cheap and
#      it goes first because a broken gate is invisible: in Aug 2026 ~340 lint
#      violations accumulated across this codebase's sibling repos while CI
#      stayed green, because `eslint .` exits 0 on warnings. A gate that stopped
#      gating looks exactly like a gate with nothing to report.
#
#   2. Change correctness -- will what I just wrote pass? Scoped to changed
#      files where the tool allows it.
#
# What this does NOT do: Go tests, full-repo lint. Those live in `make check`
# and belong to CI. Run this before committing; run `make check` before pushing
# anything you care about.
set -uo pipefail
cd "$(dirname "$0")/.."

RED=$'\033[31m'; GRN=$'\033[32m'; YEL=$'\033[33m'; DIM=$'\033[2m'; OFF=$'\033[0m'
fail=0
pass() { printf '  %sok%s   %s\n' "$GRN" "$OFF" "$1"; }
bad()  { printf '  %sFAIL%s %s\n' "$RED" "$OFF" "$1"; [ -n "${2:-}" ] && printf '       %s%s%s\n' "$DIM" "$2" "$OFF"; fail=1; }
warn() { printf '  %swarn%s %s\n' "$YEL" "$OFF" "$1"; [ -n "${2:-}" ] && printf '       %s%s%s\n' "$DIM" "$2" "$OFF"; }

FILES="$(git diff --cached --name-only --diff-filter=ACMR)"
# Nothing staged -> fall back to the working tree, so this is useful mid-edit.
# Untracked files are listed explicitly: `git diff` does not report them, and a
# brand-new file is exactly the one you most want checked.
if [ -z "$FILES" ]; then
  FILES="$(git diff --name-only --diff-filter=ACMR; git ls-files --others --exclude-standard)"
fi
FILES="$(echo "$FILES" | grep -v '^$' | sort -u)"

echo
echo "gate integrity"

# The rules we decided must never go quiet are still at error, as ESLint
# resolves them. Catches 'warn', 'off', and dropped-from-config alike.
if out=$(node ./scripts/check-lint-config.mjs 2>&1); then
  pass "pinned lint rules at error"
else
  bad "pinned lint rules" "$out"
fi

# --max-warnings 0 is what makes 'warn' and 'error' the same to CI. Without it
# a downgraded rule is reported and ignored.
for pkg in $(node -e '
  const fs=require("fs")
  const y=fs.readFileSync("pnpm-workspace.yaml","utf8")
  for (const l of y.split("\n")) { const m=/^\s*-\s*["\x27]?([^"\x27\s]+)/.exec(l); if (m && fs.existsSync(m[1]+"/package.json")) console.log(m[1]) }
'); do
  lint=$(node -e "console.log(require('./$pkg/package.json').scripts?.lint ?? '')")
  case "$lint" in
    "")             bad "$pkg has no lint script" "every workspace package must be linted" ;;
    *--max-warnings\ 0*) pass "$pkg lint gated (--max-warnings 0)" ;;
    *)              bad "$pkg lint is not gated" "add --max-warnings 0, or warnings pass CI silently" ;;
  esac
  tc=$(node -e "console.log(require('./$pkg/package.json').scripts?.typecheck ?? '')")
  [ -n "$tc" ] || bad "$pkg has no typecheck script" "pnpm -r typecheck would skip it"
done

# The hook only protects whoever installed it.
if [ "$(git config core.hooksPath || true)" = ".githooks" ]; then
  pass "pre-commit hook installed"
else
  warn "pre-commit hook not installed" "run: make setup-hooks"
fi

echo
echo "changed files"

if [ -z "$FILES" ]; then
  echo "  (nothing changed)"
else
  GO=$(echo "$FILES" | grep -E '\.go$' || true)
  TS=$(echo "$FILES" | grep -E '\.tsx?$' || true)

  if [ -n "$GO" ]; then
    unformatted=$(echo "$GO" | xargs gofmt -l -s 2>/dev/null || true)
    if [ -n "$unformatted" ]; then bad "gofmt" "$(echo "$unformatted" | tr '\n' ' ')  -- run: make fmt"; else pass "gofmt"; fi
    # -o a temp dir: a plain `go build` on a main package drops the binary in
    # the repo root. Compilation is what we want, not the artifact.
    tmp=$(mktemp -d)
    if out=$(go build -o "$tmp/" ./... 2>&1); then pass "go build"; else bad "go build" "$out"; fi
    rm -rf "$tmp"
    # Only meaningful when pkg/apis moved; cheap enough to always run.
    if out=$(./scripts/check-layer-leak.sh 2>&1); then pass "layer guard"; else bad "layer guard" "$out"; fi
  fi

  if [ -n "$TS" ]; then
    # tsc is project-wide (project references); no way to scope it.
    if out=$(pnpm -r typecheck 2>&1); then pass "typecheck"; else bad "typecheck" "$(echo "$out" | tail -20)"; fi
    # eslint scopes fine, so lint only what changed -- grouped by the workspace
    # package that owns each file rather than assuming one, which is the same
    # hardcoding that left anvil-ui ungated in the sibling repo.
    for pkg in $(echo "$TS" | cut -d/ -f1 | sort -u); do
      [ -f "$pkg/package.json" ] || continue
      rel=$(echo "$TS" | grep "^$pkg/" | sed "s|^$pkg/||")
      if out=$(cd "$pkg" && npx eslint --max-warnings 0 $rel 2>&1); then
        pass "eslint $pkg (changed files)"
      else
        bad "eslint $pkg" "$(echo "$out" | tail -20)"
      fi
    done
  fi

  # Generated artifacts are committed; a stale one is a silent drift.
  if echo "$FILES" | grep -qE '^pkg/db/.*\.sql$|^pkg/apis/.*types.*\.go$'; then
    warn "SQL or API types changed" "run: make generate, and commit the result"
  fi
  # 8-digit YYYYMMDD collides when two migrations land the same day.
  for f in $(echo "$FILES" | grep -E '^migrations?/.*\.sql$' || true); do
    base=$(basename "$f")
    echo "$base" | grep -qE '^[0-9]{14}_' || bad "migration $base" "must be YYYYMMDDHHMMSS -- create it with: make new-migration NAME=..."
  done
fi

echo
if [ "$fail" -ne 0 ]; then
  printf '%sfailed%s -- fix the above, or run `make check` for the full gate\n\n' "$RED" "$OFF"
  exit 1
fi
printf '%spassed%s %s(quick pass -- `make check` still owns Go tests and full lint)%s\n\n' "$GRN" "$OFF" "$DIM" "$OFF"
