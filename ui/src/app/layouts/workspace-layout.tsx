import { Outlet, useParams } from "react-router"
import { useEffect } from "react"
import { useWorkspaceStore } from "@/core/scope/workspace-store"

export default function WorkspaceLayout() {
  const { workspaceId } = useParams()
  const setCurrentWorkspace = useWorkspaceStore((s) => s.setCurrentWorkspace)

  useEffect(() => {
    if (workspaceId) {
      setCurrentWorkspace(workspaceId)
    }
  }, [workspaceId, setCurrentWorkspace])

  return <Outlet />
}
