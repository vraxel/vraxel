import { create } from "zustand"
import { getUserPermissions, type UserPermissionsSpec } from "@/core/auth/identity"
import { registerAuthBoundaryReset } from "@/core/auth/auth"

interface PermissionState {
  permissions: UserPermissionsSpec | null
  loading: boolean
  fetchPermissions: (userId: string) => Promise<void>
  clearPermissions: () => void
}

let fetchPromise: Promise<void> | null = null

export const usePermissionStore = create<PermissionState>()((set) => ({
  permissions: null,
  loading: false,
  fetchPermissions: async (userId: string) => {
    if (fetchPromise) return fetchPromise
    fetchPromise = (async () => {
      set({ loading: true })
      try {
        const data = await getUserPermissions(userId)
        set({ permissions: data.spec, loading: false })
      } catch {
        set({
          permissions: { isPlatformAdmin: false, platform: [], workspaces: {}, namespaces: {} },
          loading: false,
        })
      } finally {
        fetchPromise = null
      }
    })()
    return fetchPromise
  },
  clearPermissions: () => {
    fetchPromise = null
    set({ permissions: null, loading: false })
  },
}))

// Wipe in-memory permissions at every auth boundary (logout / 401-refresh
// failure) so user A's permission set never bleeds into user B's render.
registerAuthBoundaryReset(() => usePermissionStore.getState().clearPermissions())
