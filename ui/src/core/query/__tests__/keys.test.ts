import { describe, expect, it } from "vitest"
import { qk } from "../keys"
import { defineResource } from "@/core/registry/resource"

const def = defineResource({ module: "pki", name: "credentials", scopes: ["platform"] })

describe("qk", () => {
  it("scope segments are always present so prefixes never alias across scopes", () => {
    expect(qk.list(def, {}, { page: 1 })).toEqual([
      "pki",
      "credentials",
      "",
      "",
      "list",
      { page: 1 },
    ])
    expect(qk.list(def, { ws: "3" })).toEqual(["pki", "credentials", "3", "", "list", {}])
    expect(qk.detail(def, { ws: "3", ns: "7" }, 5)).toEqual([
      "pki",
      "credentials",
      "3",
      "7",
      "detail",
      "5",
    ])
  })

  it("resource prefix matches list/detail keys for invalidation", () => {
    const prefix = qk.resource(def)
    const list = qk.list(def, { ws: "1" })
    expect(list.slice(0, prefix.length)).toEqual([...prefix])
  })

  it("sub keys extend the detail key", () => {
    const detail = qk.detail(def, {}, 5)
    const sub = qk.sub(def, {}, 5, "usages", { page: 2 })
    expect(sub.slice(0, detail.length)).toEqual([...detail])
    expect(sub.slice(detail.length)).toEqual(["usages", { page: 2 }])
  })
})
