/**
 * Build a scope-aware WebSocket URL for a module resource.
 *
 * Mirrors the REST path shape the apiserver registers:
 *   /api/{module}/v1/[workspaces/{ws}/[namespaces/{ns}/]]{resource}
 */
export function buildTaskWsUrl(
  apiModule: string,
  resourcePath: string,
  scopeWorkspaceId?: string,
  scopeNamespaceId?: string,
  extraQuery?: Record<string, string>,
): string {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:"
  let scopePath = ""
  if (scopeWorkspaceId && scopeNamespaceId) {
    scopePath = `workspaces/${scopeWorkspaceId}/namespaces/${scopeNamespaceId}/`
  } else if (scopeWorkspaceId) {
    scopePath = `workspaces/${scopeWorkspaceId}/`
  }
  let url = `${protocol}//${location.host}/api/${apiModule}/v1/${scopePath}${resourcePath}`
  if (extraQuery) {
    const qs = new URLSearchParams(extraQuery).toString()
    if (qs) url += `?${qs}`
  }
  return url
}
