import { iamApi } from "./client"
import { apiRequest } from "@/core/api/client"
import type { ListParams } from "@/core/api/types"
import type { Permission, PermissionList, Role, RoleBinding, RoleBindingList, RoleList, TransferOwnershipRequest } from "./types"

// --- Permissions ---
export async function listPermissions(params?: ListParams): Promise<PermissionList> {
  return apiRequest(iamApi.get("permissions", { searchParams: params as Record<string, string> }).json())
}

export async function listAllPermissions(): Promise<Permission[]> {
  const pageSize = 100
  let page = 1
  const allItems: Permission[] = []
  let totalCount: number
  do {
    const data = await listPermissions({ page, pageSize } as ListParams)
    allItems.push(...(data.items ?? []))
    totalCount = data.totalCount
    page++
  } while (allItems.length < totalCount)
  return allItems
}

// --- Platform Roles ---
export async function listRoles(params?: ListParams): Promise<RoleList> {
  return apiRequest(iamApi.get("roles", { searchParams: params as Record<string, string> }).json())
}
export async function getRole(id: string): Promise<Role> {
  return apiRequest(iamApi.get(`roles/${id}`).json())
}
export async function createRole(data: Pick<Role, "metadata" | "spec">): Promise<Role> {
  return apiRequest(iamApi.post("roles", { json: data }).json())
}
export async function updateRole(id: string, data: Pick<Role, "metadata" | "spec">): Promise<Role> {
  return apiRequest(iamApi.put(`roles/${id}`, { json: data }).json())
}
export async function deleteRole(id: string): Promise<void> {
  await apiRequest(iamApi.delete(`roles/${id}`) as Promise<unknown>)
}
export async function deleteRoles(ids: string[]): Promise<void> {
  await apiRequest(iamApi.delete("roles", { json: { ids } }).json())
}

// --- Scoped Roles ---
export async function listWorkspaceRoles(
  workspaceId: string,
  params?: ListParams,
): Promise<RoleList> {
  return apiRequest(
    iamApi.get(`workspaces/${workspaceId}/roles`, { searchParams: params as Record<string, string> }).json(),
  )
}
export async function getWorkspaceRole(workspaceId: string, roleId: string): Promise<Role> {
  return apiRequest(iamApi.get(`workspaces/${workspaceId}/roles/${roleId}`).json())
}
export async function createWorkspaceRole(
  workspaceId: string,
  data: Pick<Role, "metadata" | "spec">,
): Promise<Role> {
  return apiRequest(iamApi.post(`workspaces/${workspaceId}/roles`, { json: data }).json())
}
export async function updateWorkspaceRole(
  workspaceId: string,
  roleId: string,
  data: Pick<Role, "metadata" | "spec">,
): Promise<Role> {
  return apiRequest(iamApi.put(`workspaces/${workspaceId}/roles/${roleId}`, { json: data }).json())
}
export async function deleteWorkspaceRole(workspaceId: string, roleId: string): Promise<void> {
  await apiRequest(iamApi.delete(`workspaces/${workspaceId}/roles/${roleId}`) as Promise<unknown>)
}
export async function deleteWorkspaceRoles(workspaceId: string, ids: string[]): Promise<void> {
  await apiRequest(iamApi.delete(`workspaces/${workspaceId}/roles`, { json: { ids } }).json())
}
export async function listNamespaceRoles(
  workspaceId: string,
  namespaceId: string,
  params?: ListParams,
): Promise<RoleList> {
  return apiRequest(
    iamApi.get(`workspaces/${workspaceId}/namespaces/${namespaceId}/roles`, { searchParams: params as Record<string, string> }).json(),
  )
}
export async function getNamespaceRole(workspaceId: string, namespaceId: string, roleId: string): Promise<Role> {
  return apiRequest(iamApi.get(`workspaces/${workspaceId}/namespaces/${namespaceId}/roles/${roleId}`).json())
}
export async function createNamespaceRole(
  workspaceId: string,
  namespaceId: string,
  data: Pick<Role, "metadata" | "spec">,
): Promise<Role> {
  return apiRequest(iamApi.post(`workspaces/${workspaceId}/namespaces/${namespaceId}/roles`, { json: data }).json())
}
export async function updateNamespaceRole(
  workspaceId: string,
  namespaceId: string,
  roleId: string,
  data: Pick<Role, "metadata" | "spec">,
): Promise<Role> {
  return apiRequest(iamApi.put(`workspaces/${workspaceId}/namespaces/${namespaceId}/roles/${roleId}`, { json: data }).json())
}
export async function deleteNamespaceRole(workspaceId: string, namespaceId: string, roleId: string): Promise<void> {
  await apiRequest(iamApi.delete(`workspaces/${workspaceId}/namespaces/${namespaceId}/roles/${roleId}`) as Promise<unknown>)
}
export async function deleteNamespaceRoles(workspaceId: string, namespaceId: string, ids: string[]): Promise<void> {
  await apiRequest(iamApi.delete(`workspaces/${workspaceId}/namespaces/${namespaceId}/roles`, { json: { ids } }).json())
}

// --- Platform RoleBindings ---
export async function listRoleBindings(params?: ListParams): Promise<RoleBindingList> {
  return apiRequest(
    iamApi.get("rolebindings", { searchParams: params as Record<string, string> }).json(),
  )
}
export async function createRoleBinding(data: Pick<RoleBinding, "spec">): Promise<RoleBinding> {
  return apiRequest(iamApi.post("rolebindings", { json: data }).json())
}
export async function deleteRoleBinding(id: string): Promise<void> {
  await apiRequest(iamApi.delete(`rolebindings/${id}`) as Promise<unknown>)
}
export async function deleteRoleBindings(ids: string[]): Promise<void> {
  await apiRequest(iamApi.delete("rolebindings", { json: { ids } }).json())
}

// --- Workspace RoleBindings ---
export async function listWorkspaceRoleBindings(
  workspaceId: string,
  params?: ListParams,
): Promise<RoleBindingList> {
  return apiRequest(
    iamApi
      .get(`workspaces/${workspaceId}/rolebindings`, {
        searchParams: params as Record<string, string>,
      })
      .json(),
  )
}
export async function createWorkspaceRoleBinding(
  workspaceId: string,
  data: Pick<RoleBinding, "spec">,
): Promise<RoleBinding> {
  return apiRequest(iamApi.post(`workspaces/${workspaceId}/rolebindings`, { json: data }).json())
}
export async function deleteWorkspaceRoleBinding(
  workspaceId: string,
  id: string,
): Promise<void> {
  await apiRequest(iamApi.delete(`workspaces/${workspaceId}/rolebindings/${id}`) as Promise<unknown>)
}
export async function deleteWorkspaceRoleBindings(
  workspaceId: string,
  ids: string[],
): Promise<void> {
  await apiRequest(iamApi.delete(`workspaces/${workspaceId}/rolebindings`, { json: { ids } }).json())
}

// --- Namespace RoleBindings ---
export async function listNamespaceRoleBindings(
  workspaceId: string,
  namespaceId: string,
  params?: ListParams,
): Promise<RoleBindingList> {
  return apiRequest(
    iamApi
      .get(`workspaces/${workspaceId}/namespaces/${namespaceId}/rolebindings`, {
        searchParams: params as Record<string, string>,
      })
      .json(),
  )
}
export async function createNamespaceRoleBinding(
  workspaceId: string,
  namespaceId: string,
  data: Pick<RoleBinding, "spec">,
): Promise<RoleBinding> {
  return apiRequest(
    iamApi
      .post(`workspaces/${workspaceId}/namespaces/${namespaceId}/rolebindings`, { json: data })
      .json(),
  )
}
export async function deleteNamespaceRoleBinding(
  workspaceId: string,
  namespaceId: string,
  id: string,
): Promise<void> {
  await apiRequest(
    iamApi.delete(`workspaces/${workspaceId}/namespaces/${namespaceId}/rolebindings/${id}`) as Promise<unknown>,
  )
}
export async function deleteNamespaceRoleBindings(
  workspaceId: string,
  namespaceId: string,
  ids: string[],
): Promise<void> {
  await apiRequest(
    iamApi.delete(`workspaces/${workspaceId}/namespaces/${namespaceId}/rolebindings`, { json: { ids } }).json(),
  )
}

// --- User Permission & RoleBinding Verbs ---
export { getUserPermissions } from "@/core/auth/identity"
export async function listUserRoleBindings(
  userId: string,
  params?: ListParams,
): Promise<RoleBindingList> {
  return apiRequest(
    iamApi
      .get(`users/${userId}/rolebindings`, { searchParams: params as Record<string, string> })
      .json(),
  )
}
export async function listWorkspaceUserRoleBindings(
  workspaceId: string,
  userId: string,
  params?: ListParams,
): Promise<RoleBindingList> {
  return apiRequest(
    iamApi
      .get(`workspaces/${workspaceId}/users/${userId}/rolebindings`, {
        searchParams: params as Record<string, string>,
      })
      .json(),
  )
}
export async function listNamespaceUserRoleBindings(
  workspaceId: string,
  namespaceId: string,
  userId: string,
  params?: ListParams,
): Promise<RoleBindingList> {
  return apiRequest(
    iamApi
      .get(`workspaces/${workspaceId}/namespaces/${namespaceId}/users/${userId}/rolebindings`, {
        searchParams: params as Record<string, string>,
      })
      .json(),
  )
}

// --- Transfer Ownership ---
export async function transferWorkspaceOwnership(
  workspaceId: string,
  data: TransferOwnershipRequest,
): Promise<void> {
  await apiRequest(
    iamApi.post(`workspaces/${workspaceId}/transfer-ownership`, { json: data }).json(),
  )
}
export async function transferNamespaceOwnership(
  workspaceId: string,
  namespaceId: string,
  data: TransferOwnershipRequest,
): Promise<void> {
  await apiRequest(
    iamApi
      .post(`workspaces/${workspaceId}/namespaces/${namespaceId}/transfer-ownership`, {
        json: data,
      })
      .json(),
  )
}

import { defineResourceApi } from "@/core/api/resource-api"
import { rolesDef, rolebindingsDef } from "../defs"

export const rolesApi = defineResourceApi<
  Role, RoleList, ListParams, Pick<Role, "metadata" | "spec">, Pick<Role, "metadata" | "spec">
>(rolesDef)

export const rolebindingsApi = defineResourceApi<
  RoleBinding, RoleBindingList, ListParams, Pick<RoleBinding, "spec">, Pick<RoleBinding, "spec">
>(rolebindingsDef)
