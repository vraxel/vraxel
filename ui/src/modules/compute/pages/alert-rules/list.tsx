import { useState } from "react"
import { useParams } from "react-router"
import { EllipsisVertical, Pencil, Plus, Trash2 } from "lucide-react"
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
import { useListQuery } from "@/frameworks/list/use-list-query"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { useQueryClient } from "@tanstack/react-query"
import { useApiMutation } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { showApiError } from "@/core/api/client"
import { usePermission } from "@/core/permission/use-permission"
import { buildPermScope } from "@/core/registry/nav-config"
import type { ScopeRef } from "@/core/registry/resource"
import type { HostAlertRule } from "@/generated/compute"
import { ALERT_METRICS, alertRulesApi } from "@/modules/compute/api/alert-rules"
import { hostAlertRulesDef } from "@/modules/compute/defs"
import { AlertRuleDialog } from "@/modules/compute/components/alert-rule-dialog"

const SEVERITY_VARIANT: Record<string, "outline" | "secondary" | "destructive"> = {
  info: "outline",
  warning: "secondary",
  critical: "destructive",
}

export default function AlertRuleListPage() {
  const { t } = useTranslation()
  const { hasPermission } = usePermission()

  const { workspaceId, namespaceId } = useParams()
  const scope: ScopeRef = { ws: workspaceId, ns: namespaceId }
  const permScope = buildPermScope(workspaceId, namespaceId)

  const canCreate = hasPermission("compute:host-alert-rules:create", permScope)
  const canUpdate = hasPermission("compute:host-alert-rules:update", permScope)
  const canDelete = hasPermission("compute:host-alert-rules:delete", permScope)

  const qc = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<HostAlertRule | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<HostAlertRule | null>(null)

  const query = useListQuery<HostAlertRule>({
    def: hostAlertRulesDef,
    api: alertRulesApi,
    scope,
    filterKeys: ["metric", "severity"],
    // firingCount moves as alerts fire and resolve; same freshness story
    // as the host list's utilisation columns.
    refetchIntervalMs: 30_000,
  })

  const deleteMutation = useApiMutation({
    mutationFn: (id: string) => alertRulesApi.delete(scope, id),
    invalidates: [qk.resource(hostAlertRulesDef)],
    onSuccess: () => {
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
    },
    onError: (err) => showApiError(err, t, "compute.alertRule.title"),
  })

  const condition = (r: HostAlertRule) =>
    `${t(`compute.alertRule.metrics.${r.spec.metric}`)} ${t(`compute.alertRule.ops.${r.spec.op}`)} ${r.spec.threshold}`

  const columns: ColumnDef<HostAlertRule>[] = [
    {
      key: "name",
      header: t("common.name"),
      sortable: true,
      cell: (r) => (
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate">{r.metadata.name}</span>
          {r.spec.enabled === false && (
            <Badge variant="outline">{t("compute.alertRule.disabled")}</Badge>
          )}
        </div>
      ),
    },
    {
      key: "metric",
      header: t("compute.alertRule.condition"),
      sortable: true,
      filterKey: "metric",
      filter: [
        { value: "all", label: t("compute.alertRule.metricAll") },
        ...ALERT_METRICS.map((m) => ({ value: m, label: t(`compute.alertRule.metrics.${m}`) })),
      ],
      cell: (r) => (
        <div className="min-w-0">
          <div className="truncate text-sm">{condition(r)}</div>
          <div className="text-muted-foreground text-xs">
            {t("compute.alertRule.forHint", { seconds: r.spec.forSeconds ?? 60 })}
          </div>
        </div>
      ),
    },
    {
      key: "severity",
      header: t("compute.alertRule.severity"),
      sortable: true,
      filterKey: "severity",
      filter: [
        { value: "all", label: t("compute.alertRule.severityAll") },
        { value: "info", label: t("compute.alertRule.severities.info") },
        { value: "warning", label: t("compute.alertRule.severities.warning") },
        { value: "critical", label: t("compute.alertRule.severities.critical") },
      ],
      cell: (r) => (
        <Badge variant={SEVERITY_VARIANT[r.spec.severity] ?? "outline"}>
          {t(`compute.alertRule.severities.${r.spec.severity}`)}
        </Badge>
      ),
    },
    {
      key: "firing",
      header: t("compute.alertRule.firing"),
      cell: (r) =>
        r.spec.firingCount ? (
          <Badge variant="destructive">{r.spec.firingCount}</Badge>
        ) : (
          <span className="text-muted-foreground text-sm">-</span>
        ),
    },
    {
      key: "createdAt",
      header: t("common.created"),
      sortable: true,
      sortKey: "created_at",
      cell: (r) => (
        <span className="text-muted-foreground text-sm">
          {formatDateTime(r.metadata.createdAt)}
        </span>
      ),
    },
    {
      key: "createdBy",
      header: t("common.createdBy"),
      sortable: true,
      sortKey: "created_by",
      cell: (r) => <span className="text-sm">{r.spec.createdByName || "-"}</span>,
    },
  ]

  return (
    <ResourceListPage
      query={query}
      columns={columns}
      titleKey="compute.alertRule.title"
      subtitle={t("compute.alertRule.subtitle")}
      searchPlaceholderKey="compute.alertRule.searchPlaceholder"
      emptyKey="compute.alertRule.empty"
      selectable={false}
      createButton={
        canCreate ? (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            {t("compute.alertRule.create")}
          </Button>
        ) : undefined
      }
      rowActions={
        canUpdate || canDelete
          ? (r) => (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" className="h-8 w-8">
                    <EllipsisVertical className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {canUpdate && (
                    <DropdownMenuItem onClick={() => setEditTarget(r)}>
                      <Pencil className="mr-2 h-4 w-4" />
                      {t("common.edit")}
                    </DropdownMenuItem>
                  )}
                  {canUpdate && canDelete && <DropdownMenuSeparator />}
                  {canDelete && (
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => setDeleteTarget(r)}
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
      <AlertRuleDialog
        rule={editTarget}
        createOpen={createOpen}
        scope={scope}
        onClose={() => {
          setCreateOpen(false)
          setEditTarget(null)
        }}
        onSuccess={() => qc.invalidateQueries({ queryKey: qk.resource(hostAlertRulesDef) })}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        description={t("compute.alertRule.deleteConfirm", {
          name: deleteTarget?.metadata.name ?? "",
        })}
        onConfirm={() => {
          if (deleteTarget) return deleteMutation.mutateAsync(deleteTarget.metadata.id)
        }}
        confirmText={t("common.delete")}
      />
    </ResourceListPage>
  )
}
