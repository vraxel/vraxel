import { describe, it, expect, beforeEach } from "vitest"
import { renderHook } from "@testing-library/react"
import { usePermission, getDefaultPath, getFirstPermittedPath } from "../use-permission"
import { usePermissionStore } from "@/core/permission/permission-store"
import type { UserPermissionsSpec } from "@/core/auth/identity"

function setPermissions(perms: UserPermissionsSpec | null) {
  usePermissionStore.setState({ permissions: perms })
}

const basePerms: UserPermissionsSpec = {
  isPlatformAdmin: false,
  platform: [],
  workspaces: {},
  namespaces: {},
}

describe("usePermission", () => {
  beforeEach(() => {
    setPermissions(null)
  })

  // --- isPlatformAdmin ---

  describe("isPlatformAdmin", () => {
    it("returns false when permissions is null", () => {
      const { result } = renderHook(() => usePermission())
      expect(result.current.isPlatformAdmin).toBe(false)
    })

    it("returns true when isPlatformAdmin is true", () => {
      setPermissions({ ...basePerms, isPlatformAdmin: true })
      const { result } = renderHook(() => usePermission())
      expect(result.current.isPlatformAdmin).toBe(true)
    })

    it("returns false when isPlatformAdmin is false", () => {
      setPermissions({ ...basePerms, isPlatformAdmin: false })
      const { result } = renderHook(() => usePermission())
      expect(result.current.isPlatformAdmin).toBe(false)
    })
  })

  // --- hasPermission (no scope) ---

  describe("hasPermission without scope", () => {
    it("returns false when permissions is null", () => {
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:users:list")).toBe(false)
    })

    it("returns true for any code when isPlatformAdmin", () => {
      setPermissions({ ...basePerms, isPlatformAdmin: true })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("anything")).toBe(true)
    })

    it("returns true when platform includes code", () => {
      setPermissions({ ...basePerms, platform: ["iam:users:list"] })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:users:list")).toBe(true)
    })

    it("returns false when platform does not include code", () => {
      setPermissions({ ...basePerms, platform: ["iam:users:list"] })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:users:delete")).toBe(false)
    })
  })

  // --- hasPermission (workspaceId scope) ---

  describe("hasPermission with workspaceId scope", () => {
    it("returns true when workspace permissions include code", () => {
      setPermissions({
        ...basePerms,
        workspaces: {
          "ws-1": { roleNames: ["admin"], permissions: ["iam:workspaces:update"] },
        },
      })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:workspaces:update", { workspaceId: "ws-1" })).toBe(
        true,
      )
    })

    it("returns false when workspace permissions do not include code", () => {
      setPermissions({
        ...basePerms,
        workspaces: {
          "ws-1": { roleNames: ["viewer"], permissions: ["iam:workspaces:get"] },
        },
      })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:workspaces:delete", { workspaceId: "ws-1" })).toBe(
        false,
      )
    })

    it("returns true when isPlatformAdmin regardless of scope", () => {
      setPermissions({ ...basePerms, isPlatformAdmin: true })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("anything", { workspaceId: "ws-1" })).toBe(true)
    })

    it("returns true when platform permission covers workspace scope", () => {
      setPermissions({ ...basePerms, platform: ["iam:workspaces:update"] })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:workspaces:update", { workspaceId: "ws-1" })).toBe(
        true,
      )
    })
  })

  // --- hasPermission (namespaceId scope) ---

  describe("hasPermission with namespaceId scope", () => {
    it("returns true when namespace permissions include code", () => {
      setPermissions({
        ...basePerms,
        namespaces: {
          "ns-1": {
            roleNames: ["editor"],
            workspaceId: "ws-1",
            permissions: ["iam:namespaces:update"],
          },
        },
        workspaces: {
          "ws-1": { roleNames: ["viewer"], permissions: ["iam:workspaces:get"] },
        },
      })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:namespaces:update", { namespaceId: "ns-1" })).toBe(
        true,
      )
    })

    it("returns true when parent workspace permissions include code (inheritance)", () => {
      setPermissions({
        ...basePerms,
        namespaces: {
          "ns-1": {
            roleNames: ["viewer"],
            workspaceId: "ws-1",
            permissions: ["iam:namespaces:get"],
          },
        },
        workspaces: {
          "ws-1": { roleNames: ["admin"], permissions: ["iam:namespaces:update"] },
        },
      })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:namespaces:update", { namespaceId: "ns-1" })).toBe(
        true,
      )
    })

    it("returns false when neither namespace nor parent workspace include code", () => {
      setPermissions({
        ...basePerms,
        namespaces: {
          "ns-1": {
            roleNames: ["viewer"],
            workspaceId: "ws-1",
            permissions: ["iam:namespaces:get"],
          },
        },
        workspaces: {
          "ws-1": { roleNames: ["viewer"], permissions: ["iam:workspaces:get"] },
        },
      })
      const { result } = renderHook(() => usePermission())
      expect(result.current.hasPermission("iam:namespaces:delete", { namespaceId: "ns-1" })).toBe(
        false,
      )
    })
  })
})

// --- Bug #72: Console icon must not leak across projects ---
//
// Two-workspace user with a different permission in each. Inside ws-1 the
// icon must land on a ws-1 route, not on the route ws-2 happens to grant.
describe("getFirstPermittedPath vs getDefaultPath - cross-project leak", () => {
  const twoProjectPerms: UserPermissionsSpec = {
    isPlatformAdmin: false,
    platform: [],
    workspaces: {
      "ws-1": { roleNames: ["user-viewer"], permissions: ["iam:users:list"] },
      "ws-2": { roleNames: ["ns-viewer"], permissions: ["iam:namespaces:list"] },
    },
    namespaces: {},
  }

  it("getDefaultPath leaks: returns ws-1's users regardless of current scope", () => {
    // Pins the exact buggy behavior the icon used to have. getDefaultPath
    // takes no scope argument, so it iterates Object.keys(workspaces) in
    // insertion order, picks ws-1, and returns its first permitted nav item
    // (users). A user actively inside ws-2 still gets teleported to ws-1 --
    // that is the cross-project leak.
    expect(getDefaultPath(twoProjectPerms)).toBe("/iam/workspaces/ws-1/users")
  })

  it("getFirstPermittedPath honors current scope: ws-1 lands on users", () => {
    const target = getFirstPermittedPath(twoProjectPerms, "ws-1", null)
    expect(target).toBe("/iam/workspaces/ws-1/users")
  })

  it("getFirstPermittedPath honors current scope: ws-2 lands on namespaces", () => {
    const target = getFirstPermittedPath(twoProjectPerms, "ws-2", null)
    expect(target).toBe("/iam/workspaces/ws-2/namespaces")
  })

  it("getFirstPermittedPath in single-project no-overview case still finds first allowed page", () => {
    // Reporter's other case: only one workspace, and the permission it grants
    // is not the first nav item -> should still land on the permitted page.
    const singleProject: UserPermissionsSpec = {
      isPlatformAdmin: false,
      platform: [],
      workspaces: {
        "ws-1": { roleNames: ["user-viewer"], permissions: ["iam:users:list"] },
      },
      namespaces: {},
    }
    expect(getFirstPermittedPath(singleProject, "ws-1", null)).toBe("/iam/workspaces/ws-1/users")
  })

  it("getFirstPermittedPath returns 403 when no in-scope permission exists", () => {
    const noPerms: UserPermissionsSpec = {
      isPlatformAdmin: false,
      platform: [],
      workspaces: { "ws-1": { roleNames: [], permissions: [] } },
      namespaces: {},
    }
    expect(getFirstPermittedPath(noPerms, "ws-1", null)).toBe("/error?status=403")
  })
})
