import { create } from "zustand"
import { getUserInfo, type OIDCUserInfo } from "@/core/auth/identity"
import { registerAuthBoundaryReset } from "@/core/auth/auth"

interface AuthState {
  user: OIDCUserInfo | null
  loading: boolean
  fetchUser: () => Promise<void>
  clearUser: () => void
}

let fetchPromise: Promise<void> | null = null

export const useAuthStore = create<AuthState>()((set) => ({
  user: null,
  loading: false,
  fetchUser: async () => {
    if (fetchPromise) return fetchPromise
    fetchPromise = (async () => {
      set({ loading: true })
      try {
        const user = await getUserInfo()
        set({ user, loading: false })
      } catch {
        set({ user: null, loading: false })
      } finally {
        fetchPromise = null
      }
    })()
    return fetchPromise
  },
  clearUser: () => {
    fetchPromise = null
    set({ user: null, loading: false })
  },
}))

// Wipe in-memory identity at every auth boundary (logout / 401-refresh
// failure) so user A's name never bleeds into user B's render.
registerAuthBoundaryReset(() => useAuthStore.getState().clearUser())
