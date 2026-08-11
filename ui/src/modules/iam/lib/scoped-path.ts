/**
 * Route prefix for the user / role a role binding points at. A binding has
 * no detail page of its own, and each scope's list only returns bindings
 * of that scope (the SQL pins WHERE rb.scope = ...), so the view's scope
 * is the row's scope. A namespace without a workspace is not a reachable
 * route, so it degrades to the platform prefix.
 */
export function scopedDetailPrefix(workspaceId?: string, namespaceId?: string): string {
  if (workspaceId && namespaceId) return `/iam/workspaces/${workspaceId}/namespaces/${namespaceId}`
  if (workspaceId) return `/iam/workspaces/${workspaceId}`
  return "/iam"
}
