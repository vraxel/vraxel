import { create } from "zustand"
import { persist } from "zustand/middleware"

interface WorkspaceState {
  currentWorkspaceId: string | null
  currentWorkspaceName: string | null
  currentNamespaceId: string | null
  setCurrentWorkspace: (id: string | null, name?: string | null) => void
  setCurrentNamespace: (id: string | null) => void
}

export const useWorkspaceStore = create<WorkspaceState>()(
  persist(
    (set) => ({
      currentWorkspaceId: null,
      currentWorkspaceName: null,
      currentNamespaceId: null,
      setCurrentWorkspace: (id, name) =>
        set({
          currentWorkspaceId: id,
          currentWorkspaceName: name ?? null,
          currentNamespaceId: null,
        }),
      setCurrentNamespace: (id) => set({ currentNamespaceId: id }),
    }),
    { name: "vraxel-workspace" },
  ),
)
