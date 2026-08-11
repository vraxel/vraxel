import { describe, expect, it } from "vitest"
import {
  buildResourceRoutes,
  defineResource,
  perm,
  resourceHref,
  resourcePath,
  scopeOf,
} from "../resource"

const credentials = defineResource({
  module: "pki",
  name: "credentials",
  scopes: ["platform", "workspace", "namespace"],
  detailParam: "credentialId",
})

describe("scopeOf / resourcePath", () => {
  it("builds the three scope prefixes", () => {
    expect(resourcePath(credentials, {})).toBe("pki/v1/credentials")
    expect(resourcePath(credentials, { ws: "3" })).toBe("pki/v1/workspaces/3/credentials")
    expect(resourcePath(credentials, { ws: "3", ns: "7" })).toBe(
      "pki/v1/workspaces/3/namespaces/7/credentials",
    )
  })

  it("appends item segments and colon verbs", () => {
    expect(resourcePath(credentials, {}, 42)).toBe("pki/v1/credentials/42")
    expect(resourcePath(credentials, { ws: "3" }, 42, "rotate")).toBe(
      "pki/v1/workspaces/3/credentials/42/rotate",
    )
    // Backend CustomVerb form: /{parent}/{id}:{verb}
    expect(resourcePath(credentials, {}, 42, { verb: "usages" })).toBe(
      "pki/v1/credentials/42:usages",
    )
  })

  it("scopeOf resolves by presence of ws/ns", () => {
    expect(scopeOf({})).toBe("platform")
    expect(scopeOf({ ws: "1" })).toBe("workspace")
    expect(scopeOf({ ws: "1", ns: "2" })).toBe("namespace")
  })
})

describe("perm", () => {
  it("derives module:resource:verb", () => {
    expect(perm(credentials, "list")).toBe("pki:credentials:list")
    expect(perm(credentials, "deleteCollection")).toBe("pki:credentials:deleteCollection")
  })
})

describe("buildResourceRoutes", () => {
  const List = () => null
  const Detail = () => null
  it("emits list+detail for each declared scope with the module-local prefix", () => {
    const def = defineResource({
      module: "pki",
      name: "credentials",
      scopes: ["platform", "workspace", "namespace"],
      detailParam: "credentialId",
      pages: { List, Detail },
    })
    const routes = buildResourceRoutes([def])
    expect(routes.map((r) => r.path)).toEqual([
      "credentials",
      "credentials/:credentialId",
      "workspaces/:workspaceId/credentials",
      "workspaces/:workspaceId/credentials/:credentialId",
      "workspaces/:workspaceId/namespaces/:namespaceId/credentials",
      "workspaces/:workspaceId/namespaces/:namespaceId/credentials/:credentialId",
    ])
  })

  it("skips detail when no Detail page and platform-only resources stay platform-only", () => {
    const def = defineResource({
      module: "o11y",
      name: "endpoints",
      scopes: ["platform"],
      pages: { List },
    })
    expect(buildResourceRoutes([def]).map((r) => r.path)).toEqual(["endpoints"])
  })
})

describe("resourceHref", () => {
  it("builds frontend navigation paths per scope", () => {
    expect(resourceHref(credentials, {})).toBe("/pki/credentials")
    expect(resourceHref(credentials, { ws: "3" }, 9)).toBe("/pki/workspaces/3/credentials/9")
    expect(resourceHref(credentials, { ws: "3", ns: "7" })).toBe(
      "/pki/workspaces/3/namespaces/7/credentials",
    )
  })
})
