// ky's `prefix` trims the input's leading slash before joining, so an
// absolute path handed to a prefixed client silently doubles the
// prefix: api.get("/api/iam/v1/x") with prefix "/api" requests
// "/api/api/iam/v1/x". The 404 then surfaces far from its cause -- a
// failed permission fetch renders as "logged in with no permissions",
// which reads like an RBAC bug (it shipped once, exactly like that).
//
// Every call site must pass a path relative to its client's prefix.
import { describe, expect, it } from "vitest"
import { readFileSync, readdirSync, statSync } from "node:fs"
import { join, resolve } from "node:path"

const SRC = resolve(__dirname, "../../..")

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      if (name === "__tests__") continue
      walk(p, out)
    } else if (/\.(ts|tsx)$/.test(name)) {
      out.push(p)
    }
  }
  return out
}

// api.get("/x"), iamApi.post(`/x`), auditApi.delete('/x'), ...
const ABSOLUTE_CALL = /\b(?:api|[A-Za-z]+Api)\.(?:get|post|put|patch|delete|head)\(\s*[`'"]\//g

describe("prefixed API clients", () => {
  it("are never called with an absolute path", () => {
    const offenders: string[] = []
    for (const file of walk(SRC)) {
      const text = readFileSync(file, "utf8")
      for (const m of text.matchAll(ABSOLUTE_CALL)) {
        const line = text.slice(0, m.index).split("\n").length
        offenders.push(`${file.slice(SRC.length + 1)}:${line}: ${m[0].trim()}`)
      }
    }
    expect(offenders).toEqual([])
  })
})
