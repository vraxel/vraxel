import { useState } from "react"
import { Link, useParams } from "react-router"
import { EllipsisVertical, Pencil, Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { formatDateTime } from "@/shared/lib/format"
import { Button } from "@/shared/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"
import { useTranslation } from "@/i18n"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { NameCell } from "@/frameworks/list/name-cell"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { useQueryClient } from "@tanstack/react-query"
import { useApiMutation } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { showApiError } from "@/core/api/client"
import { usePermission } from "@/core/permission/use-permission"
import { buildPermScope, buildScopedPath } from "@/core/registry/nav-config"
import type { ScopeRef } from "@/core/registry/resource"
import { hostsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { hostsDef } from "@/modules/compute/defs"
import { AgentStatusBadge } from "@/modules/compute/components/agent-status-badge"
import { HostEditDialog } from "@/modules/compute/components/host-edit-dialog"
import { useHostWatch } from "@/modules/compute/use-host-watch"

export default function HostListPage() {
  const { t } = useTranslation()
  const { hasPermission } = usePermission()

  const { workspaceId, namespaceId } = useParams()
  const scope: ScopeRef = { ws: workspaceId, ns: namespaceId }
  const permScope = buildPermScope(workspaceId, namespaceId)

  const canUpdate = hasPermission("compute:hosts:update", permScope)
  const canDelete = hasPermission("compute:hosts:delete", permScope)

  const qc = useQueryClient()

  const [editTarget, setEditTarget] = useState<Host | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Host | null>(null)

  const query = useListQuery<Host>({
    def: hostsDef,
    api: hostsApi,
    scope,
    filterKeys: ["agent_status", "scope"],
  })
  useHostWatch(scope)

  const deleteMutation = useApiMutation({
    mutationFn: (id: string) => hostsApi.delete(scope, id),
    invalidates: [qk.resource(hostsDef)],
    onSuccess: () => {
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
    },
    onError: (err) => showApiError(err, t, "compute.host.title"),
  })

  const base = buildScopedPath("hosts", workspaceId ?? null, namespaceId ?? null)
  const hostPath = (suffix: string) => `${base}/${suffix}`

  const columns: ColumnDef<Host>[] = [
    {
      key: "name",
      header: t("common.name"),
      sortable: true,
      filterKey: "agent_status",
      filter: [
        { value: "all", label: t("compute.host.agentStatusAll") },
        { value: "online", label: t("compute.agent.online") },
        { value: "offline", label: t("compute.agent.offline") },
      ],
      cell: (h) => (
        <NameCell
          to={hostPath(`${h.metadata.id}`)}
          displayName={h.spec.displayName}
          name={h.metadata.name}
          trailing={
            <AgentStatusBadge status={h.spec.agentStatus} conflictAt={h.spec.agentConflictAt} />
          }
        />
      ),
    },
    {
      key: "organization",
      header: t("compute.host.organization"),
      sortable: true,
      filterKey: "scope",
      filter: [
        { value: "all", label: t("compute.host.scopeAll") },
        { value: "platform", label: t("compute.host.scopePlatform") },
        { value: "workspace", label: t("compute.host.scopeWorkspace") },
        { value: "namespace", label: t("compute.host.scopeNamespace") },
      ],
      cell: (h) => {
        if (h.spec.scope === "namespace") {
          return (
            <div className="min-w-0">
              <div className="truncate text-sm">{h.spec.namespaceName || "-"}</div>
              <div className="text-muted-foreground truncate text-xs">
                {h.spec.workspaceName || "-"}
              </div>
            </div>
          )
        }
        if (h.spec.scope === "workspace") {
          return <span className="text-sm">{h.spec.workspaceName || "-"}</span>
        }
        return (
          <span className="text-muted-foreground text-sm">{t("compute.host.scopePlatform")}</span>
        )
      },
    },
    {
      key: "ip",
      header: t("compute.host.ip"),
      sortable: true,
      cell: (h) => <span className="font-mono text-xs">{h.spec.reportedPrimaryIp || "-"}</span>,
    },
    {
      key: "os",
      header: t("compute.host.os"),
      sortable: true,
      truncate: true,
      cell: (h) => (
        <div className="min-w-0">
          <div className="truncate">{h.spec.os || "-"}</div>
          <div className="text-muted-foreground text-xs">{h.spec.arch || "-"}</div>
        </div>
      ),
    },
    {
      key: "spec",
      header: t("compute.host.spec"),
      sortable: true,
      sortKey: "cpu_cores",
      cell: (h) => (
        <span className="text-sm">
          {h.spec.cpuCores ?? 0} {t("compute.host.cores")} /{" "}
          {Math.round((h.spec.memoryMb ?? 0) / 1024)} GiB
        </span>
      ),
    },
    {
      key: "createdAt",
      header: t("common.created"),
      sortable: true,
      sortKey: "created_at",
      cell: (h) => (
        <span className="text-muted-foreground text-sm">
          {formatDateTime(h.metadata.createdAt)}
        </span>
      ),
    },
    {
      key: "createdBy",
      header: t("common.createdBy"),
      cell: (h) => <span className="text-sm">{h.spec.createdByName || "-"}</span>,
    },
  ]

  return (
    <ResourceListPage
      query={query}
      columns={columns}
      titleKey="compute.host.title"
      subtitle={t("compute.host.subtitle")}
      searchPlaceholderKey="compute.host.searchPlaceholder"
      emptyKey="compute.host.empty"
      selectable={false}
      createButton={
        <Button asChild>
          <Link to={hostPath("onboard")}>
            <Plus className="size-4" />
            {t("compute.host.create")}
          </Link>
        </Button>
      }
      rowActions={
        canUpdate || canDelete
          ? (h) => (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" className="h-8 w-8">
                    <EllipsisVertical className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {canUpdate && (
                    <DropdownMenuItem onClick={() => setEditTarget(h)}>
                      <Pencil className="mr-2 h-4 w-4" />
                      {t("common.edit")}
                    </DropdownMenuItem>
                  )}
                  {canUpdate && canDelete && <DropdownMenuSeparator />}
                  {canDelete && (
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => setDeleteTarget(h)}
                    >
                      <Trash2 className="mr-2 h-4 w-4" />
                      {t("common.delete")}
                    </DropdownMenuItem>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            )
          : undefined
      }
    >
      <HostEditDialog
        host={editTarget}
        scope={scope}
        onClose={() => setEditTarget(null)}
        onSuccess={() => qc.invalidateQueries({ queryKey: qk.resource(hostsDef) })}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        // Same warning as the detail page: deleting the row leaves the
        // machine's agent dialling in against a credential that will
        // never be honoured again.
        description={
          t("compute.host.deleteConfirm", { name: deleteTarget?.metadata.name ?? "" }) +
          (deleteTarget?.spec.agentId ? `\n\n${t("compute.host.deleteAgentWarning")}` : "")
        }
        onConfirm={() => {
          if (deleteTarget) return deleteMutation.mutateAsync(deleteTarget.metadata.id)
        }}
        confirmText={t("common.delete")}
      />
    </ResourceListPage>
  )
}
