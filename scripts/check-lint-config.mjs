#!/usr/bin/env node
// Asserts that the rules we decided must never be silenced are still at
// 'error', per workspace package, as ESLint actually resolves them.
//
// Why an assertion list and not "reject every 'off'": defineConfig flattens
// `extends`, and the shipped presets legitimately set hundreds of rules to
// 'off'. Blacklisting 'off' drowns in false positives. Asserting that a named
// list resolves to severity 2 is precise, reads as the contract it is, and
// catches all three ways a rule goes quiet -- downgraded to 'warn', set to
// 'off', or dropped from the config entirely.
//
// Mirrors lcp/scripts/check-lint-config.mjs. Keep MUST_BE_ERROR in sync with
// the sharedRules block both repos carry.
//
// This exists because `--max-warnings 0` only closes the 'warn' path. Measured:
// with react-hooks/rules-of-hooks set to 'off' and a real hooks-order violation
// in the tree, `eslint . --max-warnings 0` exits 0. That is the hole this
// closes.
import { execFileSync } from "node:child_process"
import { existsSync, readFileSync, readdirSync } from "node:fs"
import { join, dirname } from "node:path"
import { fileURLToPath } from "node:url"

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "..")

// The contract. A rule leaves this list only by deliberate decision, in a diff
// that says so.
const MUST_BE_ERROR = [
  // React Compiler family (eslint-plugin-react-hooks v7). ~340 violations
  // accumulated across these trees while they sat at 'warn'.
  "react-hooks/exhaustive-deps",
  "react-hooks/rules-of-hooks",
  "react-hooks/set-state-in-effect",
  "react-hooks/refs",
  "react-hooks/purity",
  "react-hooks/immutability",
  "react-hooks/preserve-manual-memoization",
  // Fast-refresh boundary: mixed component/non-component exports.
  "react-refresh/only-export-components",
]

// Rules that only some packages carry (they need the plugin installed).
const MUST_BE_ERROR_IF_PRESENT = [
  "unused-imports/no-unused-imports",
  "@typescript-eslint/no-unused-vars",
]

// Scope: the config as it resolves for files under src/. Path-scoped overrides
// outside src (the e2e block, whose Playwright `use` fixture false-positives
// rules-of-hooks) are deliberately not checked -- they cannot weaken app code.
function workspacePackages() {
  const yaml = readFileSync(join(repoRoot, "pnpm-workspace.yaml"), "utf8")
  const out = []
  for (const line of yaml.split("\n")) {
    const m = /^\s*-\s*["']?([^"'\s]+)["']?\s*$/.exec(line)
    if (m && existsSync(join(repoRoot, m[1], "package.json"))) out.push(m[1])
  }
  return out
}

// A representative source file for the package's main config block. Any file
// under src works; we take the first one so this does not break when a
// specific entrypoint is renamed.
function sampleFile(pkgDir) {
  const src = join(repoRoot, pkgDir, "src")
  if (!existsSync(src)) return null
  const stack = [src]
  while (stack.length) {
    const dir = stack.pop()
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      const p = join(dir, e.name)
      if (e.isDirectory()) stack.push(p)
      else if (/\.tsx?$/.test(e.name) && !e.name.endsWith(".d.ts")) return p
    }
  }
  return null
}

const severity = (v) => (Array.isArray(v) ? v[0] : v)

let failed = false
for (const pkg of workspacePackages()) {
  if (!existsSync(join(repoRoot, pkg, "eslint.config.js"))) {
    console.error(`FAIL ${pkg}: no eslint.config.js -- every workspace package must be linted`)
    failed = true
    continue
  }
  const file = sampleFile(pkg)
  if (!file) {
    console.error(`FAIL ${pkg}: no source file found to resolve the config against`)
    failed = true
    continue
  }

  let resolved
  try {
    resolved = JSON.parse(execFileSync("npx", ["eslint", "--print-config", file], {
      cwd: join(repoRoot, pkg),
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }))
  } catch {
    console.error(`FAIL ${pkg}: could not resolve the eslint config`)
    failed = true
    continue
  }

  for (const rule of MUST_BE_ERROR) {
    const sev = severity(resolved.rules?.[rule])
    if (sev !== 2 && sev !== "error") {
      console.error(`FAIL ${pkg}: ${rule} resolves to ${JSON.stringify(sev ?? "unset")}, must be error`)
      failed = true
    }
  }
  for (const rule of MUST_BE_ERROR_IF_PRESENT) {
    const raw = resolved.rules?.[rule]
    if (raw === undefined) continue
    const sev = severity(raw)
    // 0 here means the package deliberately turned it off after enabling the
    // plugin, which is the case worth catching.
    if (sev !== 2 && sev !== "error") {
      console.error(`FAIL ${pkg}: ${rule} resolves to ${JSON.stringify(sev)}, must be error`)
      failed = true
    }
  }
}

// A bare `/* eslint-disable */` silences every rule for a whole file, and
// nothing above would notice. Rule-scoped disables are fine -- they name what
// they excuse and show up in review.
for (const pkg of workspacePackages()) {
  const src = join(repoRoot, pkg, "src")
  if (!existsSync(src)) continue
  const stack = [src]
  while (stack.length) {
    const dir = stack.pop()
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      const p = join(dir, e.name)
      if (e.isDirectory()) { stack.push(p); continue }
      if (!/\.tsx?$/.test(e.name)) continue
      const text = readFileSync(p, "utf8")
      if (/\/\*\s*eslint-disable\s*\*\//.test(text)) {
        console.error(`FAIL ${p.replace(repoRoot + "/", "")}: bare /* eslint-disable */ -- disable the specific rule instead, with the reason`)
        failed = true
      }
    }
  }
}

if (failed) {
  console.error("")
  console.error("These rules are held at error on purpose. If an exception is genuinely")
  console.error("needed, put `// eslint-disable-next-line <rule>` on the offending line with")
  console.error("the reason above it. Changing this list is a decision, not a fix.")
  process.exit(1)
}
console.log(`check-lint-config: OK (${workspacePackages().length} packages, ${MUST_BE_ERROR.length} pinned rules)`)
