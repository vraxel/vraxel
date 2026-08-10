import { describe, it, expect } from "vitest"
import { applyVerbGroupCascade } from "../permission-selector"

describe("applyVerbGroupCascade", () => {
  const credScope = [
    "pki:credentials:list",
    "pki:credentials:get",
    "pki:credentials:create",
    "pki:credentials:update",
    "pki:credentials:patch",
    "pki:credentials:delete",
    "pki:credentials:deleteCollection",
  ]

  it("adds list+get when create is newly selected", () => {
    const prev: string[] = []
    const next = ["pki:credentials:create"]
    const result = applyVerbGroupCascade(prev, next, credScope)
    expect(new Set(result)).toEqual(
      new Set(["pki:credentials:create", "pki:credentials:list", "pki:credentials:get"]),
    )
  })

  it("adds list+get when update group (update+patch) is newly selected", () => {
    const prev: string[] = []
    const next = ["pki:credentials:update", "pki:credentials:patch"]
    const result = applyVerbGroupCascade(prev, next, credScope)
    expect(new Set(result)).toEqual(
      new Set([
        "pki:credentials:update",
        "pki:credentials:patch",
        "pki:credentials:list",
        "pki:credentials:get",
      ]),
    )
  })

  it("adds list+get when delete group (delete+deleteCollection) is newly selected", () => {
    const prev: string[] = []
    const next = ["pki:credentials:delete", "pki:credentials:deleteCollection"]
    const result = applyVerbGroupCascade(prev, next, credScope)
    expect(new Set(result)).toEqual(
      new Set([
        "pki:credentials:delete",
        "pki:credentials:deleteCollection",
        "pki:credentials:list",
        "pki:credentials:get",
      ]),
    )
  })

  it("does not duplicate list/get if already selected", () => {
    const prev = ["pki:credentials:list", "pki:credentials:get"]
    const next = ["pki:credentials:list", "pki:credentials:get", "pki:credentials:create"]
    const result = applyVerbGroupCascade(prev, next, credScope)
    expect(result).toEqual([
      "pki:credentials:list",
      "pki:credentials:get",
      "pki:credentials:create",
    ])
  })

  it("removes write verbs when read group is newly unselected", () => {
    const prev = [
      "pki:credentials:list",
      "pki:credentials:get",
      "pki:credentials:create",
      "pki:credentials:update",
      "pki:credentials:delete",
    ]
    const next = ["pki:credentials:create", "pki:credentials:update", "pki:credentials:delete"]
    const result = applyVerbGroupCascade(prev, next, credScope)
    expect(result).toEqual([])
  })

  it("removes write verbs across update+patch and delete+deleteCollection when read removed", () => {
    const prev = [
      "pki:credentials:list",
      "pki:credentials:get",
      "pki:credentials:update",
      "pki:credentials:patch",
      "pki:credentials:delete",
      "pki:credentials:deleteCollection",
    ]
    const next = [
      "pki:credentials:update",
      "pki:credentials:patch",
      "pki:credentials:delete",
      "pki:credentials:deleteCollection",
    ]
    const result = applyVerbGroupCascade(prev, next, credScope)
    expect(result).toEqual([])
  })

  it("is a no-op when neither write added nor read removed", () => {
    const prev = ["pki:credentials:list"]
    const next = ["pki:credentials:list", "pki:credentials:get"]
    const result = applyVerbGroupCascade(prev, next, credScope)
    expect(result).toEqual(["pki:credentials:list", "pki:credentials:get"])
  })

  it("scopes cascade per-resource (does not leak across buckets)", () => {
    const scope = [
      "pki:credentials:list",
      "pki:credentials:get",
      "pki:credentials:create",
      "pki:labels:list",
      "pki:labels:get",
      "pki:labels:update",
    ]
    const prev: string[] = []
    const next = ["pki:credentials:create"]
    const result = applyVerbGroupCascade(prev, next, scope)
    expect(new Set(result)).toEqual(
      new Set(["pki:credentials:create", "pki:credentials:list", "pki:credentials:get"]),
    )
    expect(result).not.toContain("pki:labels:list")
    expect(result).not.toContain("pki:labels:get")
  })

  it("cascades only buckets where write was actually added at module-level toggle", () => {
    const scope = [
      "pki:credentials:list",
      "pki:credentials:get",
      "pki:credentials:create",
      "pki:labels:list",
      "pki:labels:get",
    ]
    const prev: string[] = []
    const next = ["pki:credentials:create"]
    const result = applyVerbGroupCascade(prev, next, scope)
    expect(result).not.toContain("pki:labels:list")
    expect(result).not.toContain("pki:labels:get")
  })

  it("works with wildcard scope at root level", () => {
    const wildcardScope = [
      "*:list",
      "*:get",
      "*:create",
      "*:update",
      "*:patch",
      "*:delete",
      "*:deleteCollection",
    ]
    const prev: string[] = []
    const next = ["*:create"]
    const result = applyVerbGroupCascade(prev, next, wildcardScope)
    expect(new Set(result)).toEqual(new Set(["*:create", "*:list", "*:get"]))
  })

  it("removes wildcard write verbs when wildcard read removed at root", () => {
    const wildcardScope = [
      "*:list",
      "*:get",
      "*:create",
      "*:update",
      "*:patch",
      "*:delete",
      "*:deleteCollection",
    ]
    const prev = ["*:list", "*:get", "*:create", "*:update"]
    const next = ["*:create", "*:update"]
    const result = applyVerbGroupCascade(prev, next, wildcardScope)
    expect(result).toEqual([])
  })

  it("ignores codes outside the scope (e.g., custom verbs)", () => {
    const scope = ["pki:credentials:list", "pki:credentials:get", "pki:credentials:create"]
    const prev = ["pki:credentials:rotate"]
    const next = ["pki:credentials:rotate", "pki:credentials:create"]
    const result = applyVerbGroupCascade(prev, next, scope)
    expect(new Set(result)).toEqual(
      new Set([
        "pki:credentials:rotate",
        "pki:credentials:create",
        "pki:credentials:list",
        "pki:credentials:get",
      ]),
    )
  })

  it("respects scope: when only list+create exist (no get), only adds list", () => {
    const scope = ["pki:credentials:list", "pki:credentials:create"]
    const prev: string[] = []
    const next = ["pki:credentials:create"]
    const result = applyVerbGroupCascade(prev, next, scope)
    expect(new Set(result)).toEqual(new Set(["pki:credentials:create", "pki:credentials:list"]))
  })
})
