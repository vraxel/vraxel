import { iamApi } from "./client"
import { apiRequest } from "@/core/api/client"
import type { ListParams } from "@/core/api/types"
import type { Workspace, WorkspaceList } from "./types"

export async function listWorkspaces(params?: ListParams): Promise<WorkspaceList> {
  return apiRequest(iamApi.get("workspaces", { searchParams: params as Record<string, string> }).json())
}

export async function getWorkspace(id: string): Promise<Workspace> {
  return apiRequest(iamApi.get(`workspaces/${id}`).json())
}

export async function createWorkspace(
  data: Pick<Workspace, "metadata" | "spec">,
): Promise<Workspace> {
  return apiRequest(iamApi.post("workspaces", { json: data }).json())
}

export async function updateWorkspace(
  id: string,
  data: Pick<Workspace, "metadata" | "spec">,
): Promise<Workspace> {
  return apiRequest(iamApi.put(`workspaces/${id}`, { json: data }).json())
}

export async function deleteWorkspace(id: string): Promise<void> {
  await apiRequest(iamApi.delete(`workspaces/${id}`) as Promise<unknown>)
}

export async function deleteWorkspaces(ids: string[]): Promise<void> {
  await apiRequest(iamApi.delete("workspaces", { json: { ids } }).json())
}

import { defineResourceApi } from "@/core/api/resource-api"
import { workspacesDef } from "../defs"

export const workspacesApi = defineResourceApi<
  Workspace, WorkspaceList, ListParams, Pick<Workspace, "metadata" | "spec">, Pick<Workspace, "metadata" | "spec">
>(workspacesDef)
