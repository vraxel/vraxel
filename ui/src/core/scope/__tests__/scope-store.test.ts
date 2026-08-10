import { describe, it, expect, beforeEach } from "vitest"
import { useScopeStore } from "../scope-store"

describe("useScopeStore", () => {
  beforeEach(() => {
    useScopeStore.setState({ workspaceId: null, namespaceId: null })
  })

  it("has correct initial state", () => {
    const state = useScopeStore.getState()
    expect(state.workspaceId).toBeNull()
    expect(state.namespaceId).toBeNull()
  })

  it("setScope updates both workspaceId and namespaceId", () => {
    useScopeStore.getState().setScope("ws-1", "ns-1")
    expect(useScopeStore.getState().workspaceId).toBe("ws-1")
    expect(useScopeStore.getState().namespaceId).toBe("ns-1")
  })

  it("setScope with null namespaceId clears namespace only", () => {
    useScopeStore.setState({ workspaceId: "ws-1", namespaceId: "ns-1" })
    useScopeStore.getState().setScope("ws-2", null)
    expect(useScopeStore.getState().workspaceId).toBe("ws-2")
    expect(useScopeStore.getState().namespaceId).toBeNull()
  })

  it("setScope with null for both clears everything", () => {
    useScopeStore.setState({ workspaceId: "ws-1", namespaceId: "ns-1" })
    useScopeStore.getState().setScope(null, null)
    expect(useScopeStore.getState().workspaceId).toBeNull()
    expect(useScopeStore.getState().namespaceId).toBeNull()
  })

  it("invalidate increments version", () => {
    const v0 = useScopeStore.getState().version
    useScopeStore.getState().invalidate()
    expect(useScopeStore.getState().version).toBe(v0 + 1)
  })
})
