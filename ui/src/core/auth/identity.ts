import type { TypeMeta } from "@/generated/meta"
import { api, apiRequest } from "@/core/api/client"

// Identity + authorization payloads the framework itself depends on:
// the session shell (root layout, route guards, nav visibility) reads
// them before any module page mounts. They live in core -- not in the
// iam module -- so lower layers never have to import upwards.

export interface OIDCUserInfo {
  sub: string
  name?: string
  email?: string
  phone_number?: string
}

export interface WorkspaceScopePerms {
  roleNames: string[]
  permissions: string[]
}

export interface NamespaceScopePerms {
  roleNames: string[]
  workspaceId: string
  permissions: string[]
}

export interface UserPermissionsSpec {
  isPlatformAdmin: boolean
  platform: string[]
  workspaces: Record<string, WorkspaceScopePerms>
  namespaces: Record<string, NamespaceScopePerms>
}

export interface UserPermissions extends TypeMeta {
  spec: UserPermissionsSpec
}

/** Current session identity, straight from the OIDC provider. */
export async function getUserInfo(): Promise<OIDCUserInfo> {
  const res = await fetch("/oidc/userinfo", { credentials: "include" })
  if (!res.ok) {
    throw new Error("Failed to fetch user info")
  }
  return res.json()
}

/** The effective permission set for one user across all three scopes. */
export async function getUserPermissions(userId: string): Promise<UserPermissions> {
  return apiRequest(api.get(`/api/iam/v1/users/${userId}/permissions`).json())
}
