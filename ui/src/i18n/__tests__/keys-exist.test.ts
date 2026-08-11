// Every dotted string literal in src that LOOKS like an i18n key must
// exist in the zh-CN catalog, and every template-literal key prefix must
// match at least one key. Catches both directions the parity test cannot:
// a typo'd t("user.craete") in code, and an over-pruned catalog that
// deleted a key the code still references.
import { describe, expect, it } from "vitest"
import { readFileSync, readdirSync, statSync } from "node:fs"
import { join, resolve } from "node:path"
import zhCN from "../locales/zh-CN"

const SRC = resolve(__dirname, "../..")
const KEYS = new Set(Object.keys(zhCN))

// Dotted literals that are not i18n keys. Extend deliberately; a growing
// list is a smell that the key-looking-literal heuristic needs work.
const NON_KEY_LITERALS = new Set([
  "created_at", // sort field, no dot -- listed defensively
])

// First segments that can never be an i18n namespace (file paths, wire
// fields, media types leak through the dotted-literal heuristic).
const NON_KEY_PREFIXES = ["metadata.", "spec.", "vraxel.", "node:", "application.", "assets."]

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      if (name === "locales" || name === "__tests__" || name === "generated") continue
      walk(p, out)
    } else if (/\.(ts|tsx)$/.test(name) && !/\.test\.tsx?$/.test(name)) {
      out.push(p)
    }
  }
  return out
}

const LITERAL_RE = /["'`]([a-z][A-Za-z0-9]*(?:\.[A-Za-z0-9_-]+)+)["'`]/g
const TEMPLATE_PREFIX_RE = /`([a-z][A-Za-z0-9.]*\.)\$\{/g

describe("i18n key references", () => {
  const files = walk(SRC)
  const missing: string[] = []
  const deadPrefixes: string[] = []

  for (const f of files) {
    const text = readFileSync(f, "utf8")
    for (const [, key] of text.matchAll(LITERAL_RE)) {
      if (KEYS.has(key) || NON_KEY_LITERALS.has(key)) continue
      if (NON_KEY_PREFIXES.some((p) => key.startsWith(p))) continue
      // Only flag literals whose first segment is an existing namespace:
      // "user.craete" (namespace `user.` exists -> typo) is flagged;
      // "some.random.id" (unknown namespace) is not treated as a key.
      const ns = key.slice(0, key.indexOf(".") + 1)
      const nsExists = [...KEYS].some((k) => k.startsWith(ns))
      if (nsExists) missing.push(`${f.slice(SRC.length + 1)}: "${key}"`)
    }
    for (const [, prefix] of text.matchAll(TEMPLATE_PREFIX_RE)) {
      if (![...KEYS].some((k) => k.startsWith(prefix))) {
        deadPrefixes.push(`${f.slice(SRC.length + 1)}: \`${prefix}\${...}\``)
      }
    }
  }

  it("every key-looking literal resolves in the zh-CN catalog", () => {
    expect(missing).toEqual([])
  })

  it("every template-literal key prefix matches at least one key", () => {
    expect(deadPrefixes).toEqual([])
  })
})
