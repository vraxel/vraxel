// A role binding has no detail page; the row's two meaningful targets
// are the user and the role. Each scope's list only returns bindings of
// that scope, so the view scope决定 prefix. Pin all three.
import { describe, expect, it } from "vitest"
import { scopedDetailPrefix } from "../rolebinding-list-view"

describe("rolebinding detail prefixes", () => {
  it("platform", () => {
    expect(scopedDetailPrefix(undefined, undefined)).toBe("/iam")
  })
  it("workspace", () => {
    expect(scopedDetailPrefix("7", undefined)).toBe("/iam/workspaces/7")
  })
  it("namespace", () => {
    expect(scopedDetailPrefix("7", "9")).toBe("/iam/workspaces/7/namespaces/9")
  })
  it("ignores a namespace with no workspace (not a reachable route)", () => {
    expect(scopedDetailPrefix(undefined, "9")).toBe("/iam")
  })
})
