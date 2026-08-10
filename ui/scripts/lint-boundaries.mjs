#!/usr/bin/env node
// Frontend module/layer boundary lint -- the counterpart of the
// backend's `make lint-layers`. Chained into `pnpm lint`.
//
// Rules (current tree; the W1 directory move updates the path roots):
//   R1  src/modules/<A>/** must not import @/modules/<B>/** (A != B).
//       Cross-module reuse goes through a shared layer or the owning
//       module's explicit public exports, not deep page paths.
//   R2  src/{shared,frameworks,core}/** must not import @/modules/** (layer
//       inversion: lower layers must not depend on modules).
//   R3  src/shared/ui/** (shadcn primitives; eslint-ignored) must not
//       import @/core, @/modules or @/frameworks -- keep primitives pure.
//
// Existing violations are frozen in ALLOWLIST below and may only
// shrink (ratchet): a violation not listed fails the build, and a
// listed entry that no longer matches also fails ("stale entry") so
// the list cannot silently rot. Do NOT add new entries; fix the
// import direction instead (see docs/frontend-refactor/plan.md).
import { readdirSync, readFileSync, statSync } from "node:fs"
import { join, dirname, resolve, relative, sep } from "node:path"

const SRC = resolve(process.cwd(), "src")

// Entries are "<file> -> <import>" with src-relative posix paths.
// Empty on purpose: this tree starts with zero boundary debt. Never add
// an entry -- fix the import direction instead.
const ALLOWLIST = new Set([
])

const walk = (dir) =>
  readdirSync(dir).flatMap((f) => {
    const p = join(dir, f)
    return statSync(p).isDirectory() ? walk(p) : [p]
  })

const toPosix = (p) => p.split(sep).join("/")

// import/export-from + bare side-effect imports. Dynamic import() is
// banned repo-wide (root CLAUDE.md) so it is not scanned here.
const IMPORT_RE = /(?:\bimport\b|\bexport\b)[^"'`;]*?\bfrom\s*"([^"]+)"|\bimport\s*"([^"]+)"/g

// Resolve an import specifier to a src-relative posix path, or null
// for external packages.
function resolveSpec(fileAbs, spec) {
  if (spec.startsWith("@/")) return spec.slice(2)
  if (spec.startsWith(".")) {
    const abs = resolve(dirname(fileAbs), spec)
    const rel = relative(SRC, abs)
    if (rel.startsWith("..")) return null
    return toPosix(rel)
  }
  return null
}

function pageModule(rel) {
  return rel.startsWith("modules/") ? rel.split("/")[1] : null
}

const files = walk(SRC).filter(
  (p) => /\.(ts|tsx)$/.test(p) && !/\.test\.(ts|tsx)$/.test(p) && !p.includes(`${sep}__tests__${sep}`),
)

const violations = []
for (const fileAbs of files) {
  const fileRel = toPosix(relative(SRC, fileAbs))
  const src = readFileSync(fileAbs, "utf8")
  for (const m of src.matchAll(IMPORT_RE)) {
    const spec = m[1] ?? m[2]
    const target = resolveSpec(fileAbs, spec)
    if (!target) continue
    const line = src.slice(0, m.index).split("\n").length

    let rule = null
    if (target.startsWith("modules/")) {
      if (/^(shared|frameworks|core)\//.test(fileRel)) rule = "R2 lower layers must not import modules"
      else {
        const from = pageModule(fileRel)
        const to = pageModule(target)
        if (from && to && from !== to) rule = "R1 cross-module page import"
      }
    }
    if (fileRel.startsWith("shared/ui/") && /^(core|modules|frameworks)(\/|$)/.test(target)) {
      rule = "R3 ui primitives must stay pure"
    }
    if (rule) violations.push({ key: `${fileRel} -> ${target}`, line, rule, fileRel })
  }
}

const seen = new Set()
let failed = false
for (const v of violations) {
  seen.add(v.key)
  if (!ALLOWLIST.has(v.key)) {
    console.error(`boundary: ${v.fileRel}:${v.line} ${v.rule}: ${v.key}`)
    failed = true
  }
}
for (const entry of ALLOWLIST) {
  if (!seen.has(entry)) {
    console.error(`boundary: stale allowlist entry (violation fixed -- delete the line): ${entry}`)
    failed = true
  }
}

if (failed) {
  console.error(`\nlint-boundaries: FAILED (${violations.filter((v) => !ALLOWLIST.has(v.key)).length} new, ${[...ALLOWLIST].filter((e) => !seen.has(e)).length} stale; allowlist=${ALLOWLIST.size})`)
  process.exit(1)
}
console.log(`lint-boundaries: OK (${violations.length} allowlisted legacy violations, 0 new)`)
