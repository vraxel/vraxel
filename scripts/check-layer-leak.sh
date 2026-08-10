#!/usr/bin/env bash
# Layer rules for pkg/apis (see pkg/apis/ARCHITECTURE.md).
#
# Minimal Model 2 onion: top-level -> business -> store -> pkg/db.
# No reverse arrows, no cross-arrows.
#
# Rule A (infra-leak): pgx / pgtype / pgconn / pgxpool and the pkg/db
# subtree (pkg/db/generated, pkg/db/pgerrors, pkg/db/sqlnull) may appear
# only in pkg/apis/<mod>/store/*.go. Exempt (narrow): pkg/apis/install.go and
# pkg/apis/<mod>/install.go may import "vraxel.io/vraxel/pkg/db" (the handle
# package only, not its generated subpackages) for the
# *db.DB handle type.
# Business / handler / types / storage code never imports pgx or
# pkg/db: they receive domain types from the module's own store
# sub-package.
#
# Rule B (cross-module store): a file not under pkg/apis/<B>/ must
# not import pkg/apis/<B>/store for any module B. Same-module imports
# (the business layer wiring in its own store) are always allowed.
# pkg/apis/install.go is never allowed to import any <mod>/store at
# all, since it sits at pkg/apis/ (not under any <mod>/).
#
# Rule D (store reverse): pkg/apis/<mod>/store/*.go must not import
# pkg/apis/<mod> (the same module's business package). The store
# sub-package is self-contained: domain row/input types and store
# interfaces live in <mod>/store/, not in <mod>.
#
# Rule E (no raw SQL): pkg/apis/<mod>/store/*.go must not embed SQL
# DML as string literals. All SQL goes through sqlc queries in
# pkg/db/query/*.sql and is invoked via s.Q().XxxMethod(). Lines may
# opt out with a trailing "// lint:allow-raw-sql" marker for legitimate
# non-DML (e.g. pg_notify / advisory locks), but DML (SELECT / INSERT /
# UPDATE / DELETE / WITH) belongs in sqlc.
#
# The lib/list package (Query / Result[T] / Pagination / FilterStr /
# ...) is pure-Go value types, importable from every layer.
#
# Modes:
#   full   - scan the whole tree (default; used by `make lint-layers`
#            and the pre-commit hook).
#   staged - only check files whose diff ADDS a forbidden import.
#            Retained for optional use during future migrations; not
#            used by the pre-commit hook anymore.

set -e

mode="${1:-full}"

declare -a scan_files=()

# Portable line-to-array reader: macOS ships bash 3.2 which lacks
# `mapfile`, so we fall back to a `while IFS= read` loop. Both are
# null-safe for empty input.
if [ "$mode" = "staged" ]; then
  while IFS= read -r line; do
    [ -n "$line" ] && scan_files+=("$line")
  done < <(git diff --cached --name-only --diff-filter=ACMR | grep -E '^pkg/apis/.*\.go$' || true)
else
  while IFS= read -r line; do
    [ -n "$line" ] && scan_files+=("$line")
  done < <(find pkg/apis -type f -name '*.go' ! -name '*_test.go')
fi

# ---------------------------------------------------------------------------
# Rule A: pgx / pkg/db-subtree imports allowed only in <mod>/store/*.go.
# Exemption for the root "vraxel.io/vraxel/pkg/db" (handle only):
#   - pkg/apis/install.go
#   - pkg/apis/<mod>/install.go           (minimal Model 2 entry)
# The subtree imports (pkg/db/generated, etc.) are
# store-only, never exempt.
# ---------------------------------------------------------------------------

is_store_file() {
  local path="$1"
  [[ "$path" == *_test.go ]] && return 1
  [[ "$path" == */store/* ]] && return 0
  return 1
}

is_handle_exempt_file() {
  # Files allowed to import "vraxel.io/vraxel/pkg/db" (handle package only).
  local path="$1"
  [[ "$path" == "pkg/apis/install.go" ]] && return 0
  # Flat minimal-Model-2 module entry: pkg/apis/<mod>/install.go.
  if [[ "$path" =~ ^pkg/apis/[^/]+/install\.go$ ]]; then
    return 0
  fi
  return 1
}

rule_a_check() {
  local out=""
  for path in "$@"; do
    # Store files may import anything in the pkg/db subtree + pgx.
    if is_store_file "$path"; then
      continue
    fi

    # Collect every forbidden import line in this file (with -H so the
    # path prefix is preserved for downstream reporting).
    local pgx_hits
    pgx_hits=$(grep -Hn \
      -e '"github.com/jackc/pgx/v5"' \
      -e '"github.com/jackc/pgx/v5/pgtype"' \
      -e '"github.com/jackc/pgx/v5/pgconn"' \
      -e '"github.com/jackc/pgx/v5/pgxpool"' \
      -e '"vraxel.io/vraxel/pkg/db/generated"' \
      -e '"vraxel.io/vraxel/pkg/db/pgerrors"' \
      -e '"vraxel.io/vraxel/pkg/db/sqlnull"' \
      "$path" 2>/dev/null | grep -vE ':[[:space:]]*//' || true)
    [ -n "$pgx_hits" ] && out+="$pgx_hits"$'\n'

    # "vraxel.io/vraxel/pkg/db" (the handle package itself). Allowed only in
    # handle-exempt files; forbidden everywhere else outside store/.
    if ! is_handle_exempt_file "$path"; then
      local handle_hits
      handle_hits=$(grep -Hn '"vraxel.io/vraxel/pkg/db"' "$path" 2>/dev/null | grep -vE ':[[:space:]]*//' || true)
      [ -n "$handle_hits" ] && out+="$handle_hits"$'\n'
    fi
  done
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------
# Rule B: cross-module imports of pkg/apis/<B>/store.
# Only files under pkg/apis/<B>/ itself may import <B>/store.
# pkg/apis/install.go is never allowed (it sits above <B>/).
# ---------------------------------------------------------------------------

rule_b_check() {
  local raw
  raw=$(grep -rn '"vraxel.io/vraxel/pkg/apis/[^"]*/store"' "$@" 2>/dev/null | grep -vE ':[[:space:]]*//' || true)
  local out=""
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    local caller importee_mod
    caller=$(printf '%s' "$line" | cut -d: -f1)
    importee_mod=$(printf '%s' "$line" | grep -oE 'vraxel\.io/vraxel/pkg/apis/[^/"]+/store' | awk -F/ '{print $5}')

    # Same-module: allowed (importer sits somewhere under pkg/apis/<mod>/).
    if [[ "$caller" == pkg/apis/${importee_mod}/* ]]; then
      continue
    fi

    # Cross-module (or top-level assembly): forbidden.
    out+="$line"$'\n'
  done <<<"$raw"
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------
# Rule D: <mod>/store/*.go must not import <mod> (business package).
# ---------------------------------------------------------------------------

rule_d_check() {
  local out=""
  for path in "$@"; do
    is_store_file "$path" || continue
    local mod
    mod=$(printf '%s' "$path" | awk -F/ '{print $3}')
    local hits
    hits=$(grep -Hn "\"vraxel.io/vraxel/pkg/apis/${mod}\"" "$path" 2>/dev/null | grep -vE ':[[:space:]]*//' || true)
    [ -n "$hits" ] && out+="$hits"$'\n'
  done
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------
# Rule E: no raw SQL DML in pkg/apis/<mod>/store/*.go.
# Flags string literals starting with SELECT / INSERT / UPDATE / DELETE /
# WITH (case-insensitive), whether plain "..." or raw `...`. Lines with
# a trailing "// lint:allow-raw-sql" marker are exempt (for non-DML like
# pg_notify). Test files and comment lines are skipped.
# ---------------------------------------------------------------------------

rule_e_check() {
  local out=""
  for path in "$@"; do
    is_store_file "$path" || continue
    local hits
    hits=$(grep -HnE '("|`)[[:space:]]*(SELECT|INSERT|UPDATE|DELETE|WITH)[[:space:]]' "$path" 2>/dev/null \
      | grep -ivE ':[[:space:]]*(//|\*|--)' \
      | grep -v 'lint:allow-raw-sql' || true)
    [ -n "$hits" ] && out+="$hits"$'\n'
  done
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------

if [ ${#scan_files[@]} -eq 0 ]; then
  echo "layer-leak check (${mode}): OK (no files)"
  exit 0
fi

rule_a_raw=$(rule_a_check "${scan_files[@]}")
rule_b_raw=$(rule_b_check "${scan_files[@]}")
rule_d_raw=$(rule_d_check "${scan_files[@]}")
rule_e_raw=$(rule_e_check "${scan_files[@]}")

# In staged mode, narrow each rule's findings to imports that the
# current diff is ADDING (not pre-existing leaks). The pre-commit
# hook blocks new leakage only; `make lint-layers` surfaces the full
# list during migration.
filter_to_added() {
  local raw="$1"
  [ -z "$raw" ] && return 0
  local filtered=""
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    local path import_text
    path=$(printf '%s' "$line" | cut -d: -f1)
    import_text=$(printf '%s' "$line" | sed -E 's/^[^:]+:[0-9]+:[[:space:]]*//')
    if git diff --cached --unified=0 -- "$path" 2>/dev/null | grep -F "+${import_text}" | grep -vq '^+++'; then
      filtered+="$line"$'\n'
    fi
  done <<<"$raw"
  printf '%s' "$filtered"
}

if [ "$mode" = "staged" ]; then
  rule_a_raw=$(filter_to_added "$rule_a_raw")
  rule_b_raw=$(filter_to_added "$rule_b_raw")
  rule_d_raw=$(filter_to_added "$rule_d_raw")
  rule_e_raw=$(filter_to_added "$rule_e_raw")
fi

fail=0

if [ -n "$rule_a_raw" ]; then
  fail=1
  echo "ERROR (rule A): pgx types / pkg/db subtree may only appear in pkg/apis/<mod>/store/*.go."
  echo "Allowed narrow exemption: pkg/apis/install.go and pkg/apis/<mod>/install.go may import \"vraxel.io/vraxel/pkg/db\" (handle only)."
  echo "Business / handler / storage / types code receives domain types from the module's own store sub-package."
  echo "Use lib/list for Query / Result[T] / Pagination / FilterStr in handler code."
  echo "Offending lines:"
  echo -n "$rule_a_raw"
  echo ""
fi

if [ -n "$rule_b_raw" ]; then
  fail=1
  echo "ERROR (rule B): cross-module import of pkg/apis/<mod>/store is not allowed."
  echo "Expose typed interfaces from <mod>'s ModuleResult, wired via the top-level assembly."
  echo "Offending lines:"
  echo -n "$rule_b_raw"
  echo ""
fi

if [ -n "$rule_d_raw" ]; then
  fail=1
  echo "ERROR (rule D): pkg/apis/<mod>/store/*.go must not import pkg/apis/<mod> (the business package)."
  echo "Domain rows / inputs / store interfaces live in <mod>/store/types.go and <mod>/store/interfaces.go, not in the business layer."
  echo "Offending lines:"
  echo -n "$rule_d_raw"
  echo ""
fi

if [ -n "$rule_e_raw" ]; then
  fail=1
  echo "ERROR (rule E): raw SQL DML is not allowed in pkg/apis/<mod>/store/*.go."
  echo "Add the query to pkg/db/query/*.sql, run 'make sqlc-generate', and call it via s.Q().XxxMethod()."
  echo "Non-DML escape (pg_notify / advisory locks): append '// lint:allow-raw-sql' to the line."
  echo "Offending lines:"
  echo -n "$rule_e_raw"
  echo ""
fi

if [ $fail -ne 0 ]; then
  exit 1
fi

echo "layer-leak check (${mode}): OK"
