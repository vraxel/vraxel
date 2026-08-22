import { useState } from "react"
import { Link, useParams } from "react-router"
import {
  ArrowUpDown,
  EllipsisVertical,
  Pencil,
  Plus,
  ScrollText,
  SquareTerminal,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"
import { formatDateTime } from "@/shared/lib/format"
import { Badge } from "@/shared/ui/badge"
import { Button } from "@/shared/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"
import { useTranslation } from "@/i18n"
import { SortIcon } from "@/shared/components/sort-icon"
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
import {
  HostCpuCell,
  HostDiskCell,
  HostMemCell,
} from "@/modules/compute/components/host-metrics-cells"
import { HostEditDialog } from "@/modules/compute/components/host-edit-dialog"
import { HostLogsDialog } from "@/modules/compute/components/host-logs-dialog"
import { HostTerminalDialog } from "@/modules/compute/components/host-terminal-dialog"
import { useHostWatch } from "@/modules/compute/use-host-watch"

export default function HostListPage() {
  const { t } = useTranslation()
  const { hasPermission } = usePermission()

  const { workspaceId, namespaceId } = useParams()
  const scope: ScopeRef = { ws: workspaceId, ns: namespaceId }
  const permScope = buildPermScope(workspaceId, namespaceId)

  const canUpdate = hasPermission("compute:hosts:update", permScope)
  const canDelete = hasPermission("compute:hosts:delete", permScope)
  const canOpenTerminal = hasPermission("compute:hosts:terminal", permScope)
  const canViewLogs = hasPermission("compute:hosts:logs", permScope)

  const qc = useQueryClient()

  const [editTarget, setEditTarget] = useState<Host | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Host | null>(null)
  const [terminalTarget, setTerminalTarget] = useState<Host | null>(null)
  const [logsTarget, setLogsTarget] = useState<Host | null>(null)

  const query = useListQuery<Host>({
    def: hostsDef,
    api: hostsApi,
    scope,
    filterKeys: ["agent_status", "scope", "origin"],
    // The utilisation columns change every heartbeat, and the watch
    // stream deliberately says nothing about that (state transitions
    // only, so per-beat churn cannot flood watchers). Two beats is a
    // reasonable freshness for numbers a human is glancing at.
    refetchIntervalMs: 30_000,
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

  // Capacity sorts. The spec column merged into the utilisation gauges,
  // so these three have no header to hang off -- and "find the biggest
  // machines" is a real question the list must still answer. They go
  // through the same handleSort as a header (click again to flip
  // direction), so a sort set here and a sort set there are one state.
  const specSorts = [
    { field: "cpu_cores", label: t("compute.host.sortCores") },
    { field: "memory_mb", label: t("compute.host.sortMemoryTotal") },
    { field: "disk_gb", label: t("compute.host.sortDiskTotal") },
  ]
  const activeSpecSort = specSorts.find((s) => s.field === query.sortBy)

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
            <span className="flex items-center gap-1">
              <AgentStatusBadge
                status={h.spec.agentStatus}
                conflictAt={h.spec.agentConflictAt}
                foreignMachineAt={h.spec.agentForeignMachineAt}
              />
              {(h.spec.alertsFiring ?? 0) > 0 && (
                <Badge variant="destructive">
                  {t("compute.alertRule.firingBadge", { count: h.spec.alertsFiring ?? 0 })}
                </Badge>
              )}
            </span>
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
      key: "origin",
      header: t("compute.host.origin"),
      sortable: true,
      filter: [
        { value: "all", label: t("compute.host.originAll") },
        { value: "agent", label: t("compute.host.originAgent") },
        { value: "manual", label: t("compute.host.originManual") },
      ],
      // Deliberately not connectivityMode, which looks the same on most
      // rows and means something else: origin is how the record came to
      // exist and never changes, connectivity is how we reach the host
      // today and does. An imported host that later installs an agent
      // stays "manual" here.
      cell: (h) => (
        <span className="text-sm">
          {h.spec.origin === "agent"
            ? t("compute.host.originAgent")
            : t("compute.host.originManual")}
        </span>
      ),
    },
    {
      key: "cpu",
      header: t("compute.host.cpu"),
      sortable: true,
      sortKey: "cpu_used_pct",
      cell: (h) => <HostCpuCell spec={h.spec} />,
    },
    {
      key: "memory",
      header: t("compute.host.memory"),
      sortable: true,
      sortKey: "mem_used_pct",
      cell: (h) => <HostMemCell spec={h.spec} />,
    },
    {
      key: "disk",
      header: t("compute.host.disk"),
      sortable: true,
      sortKey: "disk_used_pct",
      cell: (h) => <HostDiskCell spec={h.spec} />,
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
      sortable: true,
      sortKey: "created_by",
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
      toolbarExtra={
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" className="h-9">
              <ArrowUpDown className="size-4" />
              {activeSpecSort?.label ?? t("compute.host.sortSpec")}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {specSorts.map((s) => (
              <DropdownMenuItem key={s.field} onClick={() => query.handleSort(s.field)}>
                {s.label}
                {/* Only the active item carries an arrow: a neutral one on
                    every row would read as three unsorted affordances. */}
                {query.sortBy === s.field && (
                  <span className="ml-auto">
                    <SortIcon field={s.field} sortBy={query.sortBy} sortOrder={query.sortOrder} />
                  </span>
                )}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      }
      createButton={
        <Button asChild>
          <Link to={hostPath("onboard")}>
            <Plus className="size-4" />
            {t("compute.host.create")}
          </Link>
        </Button>
      }
      rowActions={
        canUpdate || canDelete || canOpenTerminal || canViewLogs
          ? (h) => (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" className="h-8 w-8">
                    <EllipsisVertical className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {canOpenTerminal && (
                    <DropdownMenuItem
                      // A terminal needs a live agent to carry it, and the
                      // list already knows which hosts have one.
                      disabled={h.spec.agentStatus !== "online"}
                      onClick={() => setTerminalTarget(h)}
                    >
                      <SquareTerminal className="mr-2 h-4 w-4" />
                      {t("compute.host.terminal.open")}
                    </DropdownMenuItem>
                  )}
                  {canViewLogs && (
                    <DropdownMenuItem
                      // Logs ride the same agent channel as the terminal.
                      disabled={h.spec.agentStatus !== "online"}
                      onClick={() => setLogsTarget(h)}
                    >
                      <ScrollText className="mr-2 h-4 w-4" />
                      {t("compute.host.logs.open")}
                    </DropdownMenuItem>
                  )}
                  {(canOpenTerminal || canViewLogs) && (canUpdate || canDelete) && (
                    <DropdownMenuSeparator />
                  )}
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
      {terminalTarget && (
        <HostTerminalDialog
          host={terminalTarget}
          scope={scope}
          open
          onOpenChange={(o) => !o && setTerminalTarget(null)}
        />
      )}

      {logsTarget && (
        <HostLogsDialog
          host={logsTarget}
          scope={scope}
          open
          onOpenChange={(o) => !o && setLogsTarget(null)}
        />
      )}

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
