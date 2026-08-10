import { vi, describe, it, expect, beforeEach } from "vitest"
import type { UserPermissions } from "@/core/auth/identity"

vi.mock("@/core/auth/identity", () => ({
  getUserPermissions: vi.fn(),
}))

import { usePermissionStore } from "../permission-store"
import { getUserPermissions } from "@/core/auth/identity"

const mockedGetUserPermissions = vi.mocked(getUserPermissions)

const mockPermissionsResponse: UserPermissions = {
  apiVersion: "iam/v1",
  kind: "UserPermissions",
  spec: {
    isPlatformAdmin: false,
    platform: ["iam:users:list"],
    workspaces: {
      "ws-1": { roleNames: ["admin"], permissions: ["iam:workspaces:update"] },
    },
    namespaces: {
      "ns-1": {
        roleNames: ["viewer"],
        workspaceId: "ws-1",
        permissions: ["iam:namespaces:get"],
      },
    },
  },
}

describe("usePermissionStore", () => {
  beforeEach(() => {
    usePermissionStore.setState({ permissions: null, loading: false })
    vi.clearAllMocks()
  })

  it("has correct initial state", () => {
    const state = usePermissionStore.getState()
    expect(state.permissions).toBeNull()
    expect(state.loading).toBe(false)
  })

  it("fetchPermissions sets permissions on success", async () => {
    mockedGetUserPermissions.mockResolvedValue(mockPermissionsResponse)

    await usePermissionStore.getState().fetchPermissions("user-1")

    const state = usePermissionStore.getState()
    expect(state.permissions).toEqual(mockPermissionsResponse.spec)
    expect(state.loading).toBe(false)
    expect(mockedGetUserPermissions).toHaveBeenCalledWith("user-1")
  })

  it("fetchPermissions sets empty permissions on failure", async () => {
    mockedGetUserPermissions.mockRejectedValue(new Error("network error"))

    await usePermissionStore.getState().fetchPermissions("user-1")

    const state = usePermissionStore.getState()
    expect(state.permissions).toEqual({
      isPlatformAdmin: false,
      platform: [],
      workspaces: {},
      namespaces: {},
    })
    expect(state.loading).toBe(false)
  })

  it("fetchPermissions deduplicates concurrent calls", async () => {
    mockedGetUserPermissions.mockResolvedValue(mockPermissionsResponse)

    const p1 = usePermissionStore.getState().fetchPermissions("user-1")
    const p2 = usePermissionStore.getState().fetchPermissions("user-1")

    await Promise.all([p1, p2])

    expect(mockedGetUserPermissions).toHaveBeenCalledTimes(1)
  })

  it("clearPermissions resets permissions to null", async () => {
    mockedGetUserPermissions.mockResolvedValue(mockPermissionsResponse)
    await usePermissionStore.getState().fetchPermissions("user-1")
    expect(usePermissionStore.getState().permissions).not.toBeNull()

    usePermissionStore.getState().clearPermissions()
    expect(usePermissionStore.getState().permissions).toBeNull()
  })
})
