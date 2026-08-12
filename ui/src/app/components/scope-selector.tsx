import { useEffect } from "react"
import { useNavigate, useLocation } from "react-router"
import { Building2, FolderKanban } from "lucide-react"
import {
  Select,
  SelectContent,
  SelectEmpty,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/ui/select"
import { TruncateText } from "@/shared/components/truncate-text"
import { useScopeStore } from "@/core/scope/scope-store"
import { usePermissionStore } from "@/core/permission/permission-store"
import { checkPermission, getFirstPermittedPath } from "@/core/permission/use-permission"
import {
  detectResource,
  buildScopedPath,
  getResourcePermission,
  getScopeLevel,
  isResourceAtScope,
} from "@/core/registry/nav-config"
import { listWorkspaces } from "@/modules/iam/api/workspaces"
import { listNamespaces, listWorkspaceNamespaces } from "@/modules/iam/api/namespaces"
import { useApiQuery } from "@/core/query/hooks"
import { useTranslation } from "@/i18n"
import type { Namespace, Workspace } from "@/modules/iam/api/types"

const ALL = "__all__"
/** Re-fetch scope data every 5 minutes to detect membership changes made by others. */
const POLL_INTERVAL_MS = 5 * 60 * 1000

export function ScopeSelector() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const workspaceId = useScopeStore((s) => s.workspaceId)
  const namespaceId = useScopeStore((s) => s.namespaceId)
  const version = useScopeStore((s) => s.version)
  const invalidate = useScopeStore((s) => s.invalidate)

  const permissions = usePermissionStore((s) => s.permissions)
  const hasPlatformScope = permissions?.isPlatformAdmin || (permissions?.platform?.length ?? 0) > 0
  // Workspace-level users (e.g. workspace-viewer) should see "All namespaces" within their workspace
  const hasWorkspaceScope = !!(
    workspaceId && (permissions?.workspaces?.[workspaceId]?.permissions?.length ?? 0) > 0
  )

  // Periodic polling: bump the scope store's version to re-fetch scope data
  // everywhere it is keyed (membership changes made by other users).
  useEffect(() => {
    const timer = setInterval(() => invalidate(), POLL_INTERVAL_MS)
    return () => clearInterval(timer)
  }, [invalidate])

  const wsQuery = useApiQuery<Workspace[]>({
    queryKey: ["scope-selector", "workspaces", version],
    queryFn: async () => (await listWorkspaces({ pageSize: 100 })).items ?? [],
    placeholderData: (prev) => prev, // keep previous data across key changes / errors
    meta: { skipGlobalError: true }, // selector polls; failures must not toast
  })
  const workspaces = wsQuery.data ?? []

  // Stale workspace detection: if current workspace was removed, redirect to first permitted path
  // Non-platform users with no workspace selected: auto-select
  useEffect(() => {
    if (workspaces.length === 0) return
    if (workspaceId && !workspaces.some((ws) => ws.metadata.id === workspaceId)) {
      // Current workspace no longer accessible — redirect
      const newWsId = workspaces[0]?.metadata.id ?? null
      if (permissions) {
        navigate(getFirstPermittedPath(permissions, newWsId, null))
      } else {
        const resource = detectResource(location.pathname)
        navigate(buildScopedPath(resource, newWsId, null))
      }
    } else if (!hasPlatformScope && !workspaceId) {
      // Non-platform user with no workspace — auto-select first
      const firstWsId = workspaces[0].metadata.id
      const resource = detectResource(location.pathname)
      navigate(buildScopedPath(resource, firstWsId, null))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasPlatformScope, workspaces, workspaceId])

  // Workspace-role users (with iam:namespaces:list) see all namespaces via
  // the scoped list API. Namespace-only members fall back to the platform
  // list, whose backend AccessFilter already narrows it to namespaces they
  // can reach; the client then keeps only the active workspace's.
  const canListNamespaces = permissions
    ? checkPermission(permissions, "iam:namespaces:list", workspaceId ? { workspaceId } : undefined)
    : false

  const nsQuery = useApiQuery<Namespace[]>({
    // Keyed by scope + the store's invalidation version (bumped every
    // POLL_INTERVAL_MS and by the 403 handler in core/api/client.ts).
    queryKey: ["scope-selector", "namespaces", workspaceId ?? "", canListNamespaces, version],
    queryFn: async () => {
      if (!canListNamespaces) {
        if (!workspaceId) return []
        const data = await listNamespaces({ pageSize: 100 })
        return (data.items ?? []).filter((ns) => ns.spec.workspaceId === workspaceId)
      }
      const data = workspaceId
        ? await listWorkspaceNamespaces(workspaceId, { pageSize: 100 })
        : await listNamespaces({ pageSize: 100 })
      return data.items ?? []
    },
    placeholderData: (prev) => prev, // keep previous data across key changes / errors
    meta: { skipGlobalError: true }, // selector polls; failures must not toast
  })
  const namespaces = nsQuery.data ?? []

  // Stale namespace detection + auto-select for non-platform users
  useEffect(() => {
    if (
      namespaceId &&
      namespaces.length > 0 &&
      !namespaces.some((ns) => ns.metadata.id === namespaceId)
    ) {
      // Current namespace no longer accessible — redirect
      const newNsId = namespaces[0]?.metadata.id ?? null
      if (permissions) {
        navigate(getFirstPermittedPath(permissions, workspaceId, newNsId))
      } else {
        const resource = detectResource(location.pathname)
        navigate(buildScopedPath(resource, workspaceId, newNsId))
      }
    } else if (!hasPlatformScope && !hasWorkspaceScope && namespaces.length === 1 && !namespaceId) {
      // Non-platform user with single namespace — auto-select
      const nsId = namespaces[0].metadata.id
      const resource = detectResource(location.pathname)
      navigate(buildScopedPath(resource, workspaceId, nsId))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasPlatformScope, hasWorkspaceScope, namespaces, namespaceId])

  return (
    <div className="space-y-0.5 overflow-hidden px-1">
      <Select
        value={workspaceId ?? (hasPlatformScope ? ALL : "")}
        onValueChange={(v) => {
          const wsId = v === ALL ? null : v
          const resource = detectResource(location.pathname)
          const targetScope = getScopeLevel(wsId, null)
          const available = resource ? isResourceAtScope(resource, targetScope) : false
          const permCode = available && resource ? getResourcePermission(resource) : undefined
          const scope = wsId ? { workspaceId: wsId } : undefined
          // Navigate only; root-layout's useLayoutEffect syncs scope store from URL,
          // avoiding stale requests from the old page seeing the new scope before unmounting.
          if (
            available &&
            permCode &&
            permissions &&
            checkPermission(permissions, permCode, scope)
          ) {
            navigate(buildScopedPath(resource, wsId, null))
          } else if (permissions) {
            navigate(getFirstPermittedPath(permissions, wsId, null))
          } else {
            navigate(buildScopedPath(resource, wsId, null))
          }
        }}
        onOpenChange={(open) => {
          if (open) void wsQuery.refetch()
        }}
      >
        <SelectTrigger
          size="sm"
          className="hover:bg-accent h-8 w-full min-w-0 gap-1.5 overflow-hidden rounded-md border-0 bg-transparent px-2 text-xs shadow-none focus-visible:ring-0"
        >
          <Building2 className="text-muted-foreground h-3.5 w-3.5 shrink-0" />
          {(() => {
            const selectedWs = workspaces.find((ws) => ws.metadata.id === workspaceId)
            const wsLabel = (ws: Workspace) => ws.spec.displayName || ws.metadata.name
            return (
              <SelectValue placeholder={t("scope.selectWorkspace")}>
                {workspaceId === ALL
                  ? t("scope.allWorkspaces")
                  : selectedWs && <TruncateText>{wsLabel(selectedWs)}</TruncateText>}
              </SelectValue>
            )
          })()}
        </SelectTrigger>
        <SelectContent className="max-w-64">
          {hasPlatformScope && <SelectItem value={ALL}>{t("scope.allWorkspaces")}</SelectItem>}
          {workspaces.length === 0 && !hasPlatformScope ? (
            <SelectEmpty>{t("common.noOptions")}</SelectEmpty>
          ) : (
            workspaces.map((ws) => (
              <SelectItem key={ws.metadata.id} value={ws.metadata.id}>
                <TruncateText>{ws.spec.displayName || ws.metadata.name}</TruncateText>
              </SelectItem>
            ))
          )}
        </SelectContent>
      </Select>
      <Select
        value={namespaceId ?? (hasPlatformScope || hasWorkspaceScope ? ALL : "")}
        onValueChange={(v) => {
          const nsId = v === ALL ? null : v
          const resource = detectResource(location.pathname)
          const targetScope = getScopeLevel(workspaceId, nsId)
          const available = resource ? isResourceAtScope(resource, targetScope) : false
          const permCode = available && resource ? getResourcePermission(resource) : undefined
          const scope =
            nsId && workspaceId
              ? { workspaceId, namespaceId: nsId }
              : workspaceId
                ? { workspaceId }
                : undefined
          if (
            available &&
            permCode &&
            permissions &&
            checkPermission(permissions, permCode, scope)
          ) {
            navigate(buildScopedPath(resource, workspaceId, nsId))
          } else if (permissions) {
            navigate(getFirstPermittedPath(permissions, workspaceId, nsId))
          } else {
            navigate(buildScopedPath(resource, workspaceId, nsId))
          }
        }}
        onOpenChange={(open) => {
          if (open) void nsQuery.refetch()
        }}
        disabled={!workspaceId}
      >
        <SelectTrigger
          size="sm"
          className="hover:bg-accent h-8 w-full min-w-0 gap-1.5 overflow-hidden rounded-md border-0 bg-transparent px-2 text-xs shadow-none focus-visible:ring-0"
        >
          <FolderKanban className="text-muted-foreground h-3.5 w-3.5 shrink-0" />
          {(() => {
            const nsLabel = (ns: Namespace) =>
              ns.metadata.name.endsWith("-default") &&
              (!ns.spec.displayName || ns.spec.displayName === "Default")
                ? t("namespace.builtinDefault")
                : ns.spec.displayName || ns.metadata.name
            const selectedNs = namespaces.find((ns) => ns.metadata.id === namespaceId)
            return (
              <SelectValue placeholder={t("scope.selectNamespace")}>
                {namespaceId === ALL
                  ? t("scope.allNamespaces")
                  : selectedNs && <TruncateText>{nsLabel(selectedNs)}</TruncateText>}
              </SelectValue>
            )
          })()}
        </SelectTrigger>
        <SelectContent className="max-w-64">
          {(hasPlatformScope || hasWorkspaceScope) && (
            <SelectItem value={ALL}>{t("scope.allNamespaces")}</SelectItem>
          )}
          {namespaces.length === 0 && !(hasPlatformScope || hasWorkspaceScope) ? (
            <SelectEmpty>{t("common.noOptions")}</SelectEmpty>
          ) : (
            namespaces.map((ns) => (
              <SelectItem key={ns.metadata.id} value={ns.metadata.id}>
                <TruncateText>
                  {ns.metadata.name.endsWith("-default") &&
                  (!ns.spec.displayName || ns.spec.displayName === "Default")
                    ? t("namespace.builtinDefault")
                    : ns.spec.displayName || ns.metadata.name}
                </TruncateText>
              </SelectItem>
            ))
          )}
        </SelectContent>
      </Select>
    </div>
  )
}
