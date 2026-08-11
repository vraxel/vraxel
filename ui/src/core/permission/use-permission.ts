import { useCallback } from "react"
import { usePermissionStore } from "@/core/permission/permission-store"
import { NAV_ITEMS, buildScopedPath, getScopeLevel } from "@/core/registry/nav-config"
import type { UserPermissionsSpec } from "@/core/auth/identity"

/**
 * Check if user has a specific permission at the given scope, using raw perms data.
 * This is a static version of hasPermission for use outside of React components.
 */
export function checkPermission(
  perms: UserPermissionsSpec,
  code: string,
  scope?: { workspaceId?: string; namespaceId?: string },
): boolean {
  if (perms.isPlatformAdmin) return true
  if (perms.platform?.includes(code)) return true

  if (scope?.namespaceId) {
    const nsPerms = perms.namespaces?.[scope.namespaceId]
    const wsId = scope.workspaceId || nsPerms?.workspaceId
    if (wsId) {
      const wsPerms = perms.workspaces?.[wsId]
      if (wsPerms?.permissions?.includes(code)) return true
    }
    if (nsPerms?.permissions?.includes(code)) return true
    return false
  }

  if (scope?.workspaceId) {
    const wsPerms = perms.workspaces?.[scope.workspaceId]
    if (wsPerms?.permissions?.includes(code)) return true
    return false
  }

  return false
}

/**
 * Compute the best landing page based on the user's permissions.
 * Finds the first nav item the user can access, checking platform → workspace → namespace scope.
 */
export function getDefaultPath(perms: UserPermissionsSpec): string {
  // Platform scope
  if (perms.isPlatformAdmin || (perms.platform?.length ?? 0) > 0) {
    for (const item of NAV_ITEMS) {
      if (item.hideFromNav) continue
      if (checkPermission(perms, item.permission)) {
        return buildScopedPath(item.resource, null, null)
      }
    }
  }

  // Workspace scope
  const wsIds = Object.keys(perms.workspaces ?? {})
  if (wsIds.length > 0) {
    const wsId = wsIds[0]
    const wsScope = { workspaceId: wsId }
    for (const item of NAV_ITEMS) {
      if (item.hideFromNav) continue
      if (checkPermission(perms, item.permission, wsScope)) {
        return buildScopedPath(item.resource, wsId, null)
      }
    }
  }

  // Namespace scope
  const nsEntries = Object.entries(perms.namespaces ?? {})
  if (nsEntries.length > 0) {
    const [nsId, nsPerms] = nsEntries[0]
    const nsScope = { workspaceId: nsPerms.workspaceId, namespaceId: nsId }
    for (const item of NAV_ITEMS) {
      if (item.hideFromNav) continue
      if (checkPermission(perms, item.permission, nsScope)) {
        return buildScopedPath(item.resource, nsPerms.workspaceId, nsId)
      }
    }
  }

  return "/error?status=403"
}

/**
 * Find the first permitted navigation path for a specific scope.
 * Used by scope-selector to redirect when the current resource is not accessible in the new scope.
 */
export function getFirstPermittedPath(
  perms: UserPermissionsSpec,
  wsId: string | null,
  nsId: string | null,
): string {
  const scope =
    nsId && wsId
      ? { workspaceId: wsId, namespaceId: nsId }
      : wsId
        ? { workspaceId: wsId }
        : undefined
  const scopeLevel = getScopeLevel(wsId, nsId)
  for (const item of NAV_ITEMS) {
    if (!item.scopes.includes(scopeLevel)) continue
    if (item.hideFromNav) continue
    if (checkPermission(perms, item.permission, scope)) {
      return buildScopedPath(item.resource, wsId, nsId)
    }
  }
  // Fallback: when workspace is selected but no namespace specified,
  // check namespace-scoped permissions within this workspace.
  // This handles users who only have namespace-level access (e.g. added directly to a namespace).
  if (wsId && !nsId) {
    for (const [entryNsId, nsPerms] of Object.entries(perms.namespaces ?? {})) {
      if (nsPerms.workspaceId !== wsId) continue
      const nsScope = { workspaceId: wsId, namespaceId: entryNsId }
      for (const item of NAV_ITEMS) {
        if (item.hideFromNav) continue
        if (checkPermission(perms, item.permission, nsScope)) {
          return buildScopedPath(item.resource, wsId, entryNsId)
        }
      }
    }
  }
  return "/error?status=403"
}

export function usePermission() {
  const permissions = usePermissionStore((s) => s.permissions)

  const isPlatformAdmin = permissions?.isPlatformAdmin ?? false

  const hasPermission = useCallback(
    (code: string, scope?: { workspaceId?: string; namespaceId?: string }): boolean => {
      if (!permissions) return false
      return checkPermission(permissions, code, scope)
    },
    [permissions],
  )

  return { hasPermission, isPlatformAdmin }
}
