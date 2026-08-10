import { iamApi } from "./client"
import { apiRequest } from "@/core/api/client"
import type { ListParams, StatusResponse } from "@/core/api/types"
import type { ChangePasswordRequest, NamespaceList, ResetPasswordRequest, User, UserList, WorkspaceList } from "./types"

export async function listUsers(params?: ListParams): Promise<UserList> {
  return apiRequest(iamApi.get("users", { searchParams: params as Record<string, string> }).json())
}

export async function getUser(id: string): Promise<User> {
  return apiRequest(iamApi.get(`users/${id}`).json())
}

export async function createUser(data: Pick<User, "metadata" | "spec">): Promise<User> {
  return apiRequest(iamApi.post("users", { json: data }).json())
}

export async function updateUser(
  id: string,
  data: Pick<User, "metadata" | "spec">,
): Promise<User> {
  return apiRequest(iamApi.put(`users/${id}`, { json: data }).json())
}

export async function deleteUser(id: string): Promise<void> {
  await apiRequest(iamApi.delete(`users/${id}`) as Promise<unknown>)
}

export async function deleteUsers(ids: string[]): Promise<void> {
  await apiRequest(iamApi.delete("users", { json: { ids } }).json())
}

export async function getWorkspaceUser(workspaceId: string, userId: string): Promise<User> {
  return apiRequest(iamApi.get(`workspaces/${workspaceId}/users/${userId}`).json())
}

export async function getNamespaceUser(workspaceId: string, namespaceId: string, userId: string): Promise<User> {
  return apiRequest(iamApi.get(`workspaces/${workspaceId}/namespaces/${namespaceId}/users/${userId}`).json())
}

export async function listWorkspaceUsers(
  workspaceId: string,
  params?: ListParams,
): Promise<UserList> {
  return apiRequest(
    iamApi.get(`workspaces/${workspaceId}/users`, { searchParams: params as Record<string, string> }).json(),
  )
}

export async function addWorkspaceUsers(workspaceId: string, ids: string[], roleId?: string): Promise<void> {
  const body: { ids: string[]; roleId?: string } = { ids }
  if (roleId) body.roleId = roleId
  await apiRequest(iamApi.post(`workspaces/${workspaceId}/users`, { json: body }).json())
}

export async function removeWorkspaceUsers(workspaceId: string, ids: string[]): Promise<{ successCount: number; failedCount: number }> {
  return apiRequest(iamApi.delete(`workspaces/${workspaceId}/users`, { json: { ids } }).json())
}

export async function listNamespaceUsers(
  workspaceId: string,
  namespaceId: string,
  params?: ListParams,
): Promise<UserList> {
  return apiRequest(
    iamApi.get(`workspaces/${workspaceId}/namespaces/${namespaceId}/users`, { searchParams: params as Record<string, string> }).json(),
  )
}

export async function addNamespaceUsers(workspaceId: string, namespaceId: string, ids: string[], roleId?: string): Promise<void> {
  const body: { ids: string[]; roleId?: string } = { ids }
  if (roleId) body.roleId = roleId
  await apiRequest(iamApi.post(`workspaces/${workspaceId}/namespaces/${namespaceId}/users`, { json: body }).json())
}

export async function removeNamespaceUsers(workspaceId: string, namespaceId: string, ids: string[]): Promise<{ successCount: number; failedCount: number }> {
  return apiRequest(iamApi.delete(`workspaces/${workspaceId}/namespaces/${namespaceId}/users`, { json: { ids } }).json())
}

export async function listUserWorkspaces(
  userId: string,
  params?: ListParams,
): Promise<WorkspaceList> {
  return apiRequest(
    iamApi.get(`users/${userId}/workspaces`, { searchParams: params as Record<string, string> }).json(),
  )
}

export async function listUserNamespaces(
  userId: string,
  params?: ListParams,
): Promise<NamespaceList> {
  return apiRequest(
    iamApi.get(`users/${userId}/namespaces`, { searchParams: params as Record<string, string> }).json(),
  )
}

export async function changePassword(
  userId: string,
  data: ChangePasswordRequest,
): Promise<StatusResponse> {
  return apiRequest(iamApi.post(`users/${userId}/change-password`, { json: data }).json())
}

export async function resetPassword(
  userId: string,
  data: ResetPasswordRequest,
): Promise<StatusResponse> {
  return apiRequest(iamApi.post(`users/${userId}/reset-password`, { json: data }).json())
}

export { getUserInfo } from "@/core/auth/identity"

import { defineResourceApi } from "@/core/api/resource-api"
import { usersDef } from "../defs"

export const usersApi = defineResourceApi<
  User, UserList, ListParams, Pick<User, "metadata" | "spec">, Pick<User, "metadata" | "spec">
>(usersDef)
