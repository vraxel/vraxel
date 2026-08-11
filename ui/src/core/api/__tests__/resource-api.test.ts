import { beforeEach, describe, expect, it, vi } from "vitest"
import { defineResource } from "@/core/registry/resource"

interface Call {
  method: string
  url: string
  opts?: { searchParams?: URLSearchParams; json?: unknown; timeout?: number }
}
const calls: Call[] = []

vi.mock("../client", () => {
  const mk = (method: string) => (url: string, opts?: Call["opts"]) => {
    calls.push({ method, url, opts })
    // Mirror real backend delete/action semantics: 204 with an empty
    // body (text ""), so jsonOrEmpty's empty-body path is exercised.
    return { json: async () => ({ mocked: true }), text: async () => "" }
  }
  return {
    api: {
      get: mk("get"),
      post: mk("post"),
      put: mk("put"),
      patch: mk("patch"),
      delete: mk("delete"),
    },
    apiRequest: (p: Promise<unknown>) => p,
  }
})

import { defineAction, defineResourceApi, defineSubApi, defineVerb } from "../resource-api"

const hostsDef = defineResource({
  module: "compute",
  name: "hosts",
  scopes: ["platform", "workspace", "namespace"],
  detailParam: "hostId",
})

describe("defineResourceApi", () => {
  beforeEach(() => calls.splice(0))

  it("builds scope-collapsed CRUD urls", async () => {
    const hosts = defineResourceApi(hostsDef)
    await hosts.list({ ws: "3" }, { page: 2, pageSize: 20 })
    await hosts.get({}, 7)
    await hosts.create({ ws: "3", ns: "9" }, { name: "n1" })
    await hosts.delete({}, 7, { preserveVM: true })
    expect(calls.map((c) => [c.method, c.url])).toEqual([
      ["get", "compute/v1/workspaces/3/hosts"],
      ["get", "compute/v1/hosts/7"],
      ["post", "compute/v1/workspaces/3/namespaces/9/hosts"],
      ["delete", "compute/v1/hosts/7"],
    ])
    expect(calls[0].opts?.searchParams?.toString()).toBe("page=2&pageSize=20")
    expect(calls[3].opts?.searchParams?.toString()).toBe("preserveVM=true")
  })

  it("applies per-verb options (delete timeout)", async () => {
    const hosts = defineResourceApi(hostsDef, { delete: { timeout: 90_000 } })
    await hosts.delete({ ws: "1" }, 5)
    expect(calls[0].opts?.timeout).toBe(90_000)
  })

  // Backend single delete replies 204 with an empty body; .json() on ky
  // would JSON.parse("") and throw. delete/action must tolerate it.
  it("delete and action tolerate a 204 empty body", async () => {
    const hosts = defineResourceApi(hostsDef)
    await expect(hosts.delete({}, 7)).resolves.toBeUndefined()
    const reboot = defineAction(hostsDef, "reboot")
    await expect(reboot({ ws: "3" }, 7)).resolves.toBeUndefined()
  })
})

describe("action / verb / sub", () => {
  beforeEach(() => calls.splice(0))

  it("defineAction posts to /{id}/{action}", async () => {
    const reboot = defineAction(hostsDef, "reboot")
    await reboot({ ws: "3" }, 7)
    expect(calls[0]).toMatchObject({
      method: "post",
      url: "compute/v1/workspaces/3/hosts/7/reboot",
    })
  })

  it("defineVerb gets the colon form /{id}:{verb}", async () => {
    const templates = defineVerb(hostsDef, "templates")
    await templates({}, 7, { refresh: true })
    expect(calls[0].url).toBe("compute/v1/hosts/7:templates")
    expect(calls[0].opts?.searchParams?.toString()).toBe("refresh=true")
  })

  it("defineSubApi nests under the parent item", async () => {
    const nics = defineSubApi(hostsDef, "nics")
    await nics.list({ ws: "3", ns: "9" }, 7)
    await nics.delete({}, 7, 2)
    expect(calls.map((c) => [c.method, c.url])).toEqual([
      ["get", "compute/v1/workspaces/3/namespaces/9/hosts/7/nics"],
      ["delete", "compute/v1/hosts/7/nics/2"],
    ])
  })
})
