import { useEffect, useState } from "react"
import { Link, useParams } from "react-router"
import { Pencil, Plus, Trash2 } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { formatDateTime } from "@/shared/lib/format"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import { useTranslation } from "@/i18n"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { NameCell } from "@/frameworks/list/name-cell"
import { StatusFilter } from "@/frameworks/list/status-filter"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { FormDialog } from "@/frameworks/form/form-dialog"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { useQueryClient } from "@tanstack/react-query"
import { useApiMutation } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { handleFormApiError, showApiError } from "@/core/api/client"
import { usePermission } from "@/core/permission/use-permission"
import { buildPermScope, buildScopedPath } from "@/core/registry/nav-config"
import type { ScopeRef } from "@/core/registry/resource"
import { hostsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { hostsDef } from "@/modules/compute/defs"
import { AgentStatusBadge } from "@/modules/compute/components/agent-status-badge"
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
    filterKeys: ["agentStatus"],
  })
  // Agents come and go without anyone touching this page, and a machine
  // running the install script adds itself to this list.
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
      key: "ip",
      header: t("compute.host.ip"),
      cell: (h) => <span className="font-mono text-xs">{h.spec.reportedPrimaryIp || "-"}</span>,
    },
    {
      key: "os",
      header: t("compute.host.os"),
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
      toolbarExtra={
        <StatusFilter
          value={query.filters.agentStatus ?? "all"}
          onChange={(v) => query.setFilter("agentStatus", v)}
          allLabel={t("compute.host.agentStatusAll")}
          placeholder={t("compute.host.agentStatus")}
          options={[
            { value: "online", label: t("compute.agent.online") },
            { value: "offline", label: t("compute.agent.offline") },
          ]}
        />
      }
      createButton={
        <Button asChild>
          <Link to={hostPath("onboard")}>
            <Plus className="size-4" />
            {t("compute.host.create")}
          </Link>
        </Button>
      }
      rowActions={(h) => (
        <>
          {canUpdate && (
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={() => setEditTarget(h)}
              title={t("common.edit")}
            >
              <Pencil className="h-3.5 w-3.5" />
            </Button>
          )}
          {canDelete && (
            <Button
              variant="ghost"
              size="icon"
              className="text-destructive hover:text-destructive h-8 w-8"
              onClick={() => setDeleteTarget(h)}
              title={t("common.delete")}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          )}
        </>
      )}
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
        description={t("compute.host.deleteConfirm", { name: deleteTarget?.metadata.name ?? "" })}
        onConfirm={() => {
          if (deleteTarget) return deleteMutation.mutateAsync(deleteTarget.metadata.id)
        }}
        confirmText={t("common.delete")}
      />
    </ResourceListPage>
  )
}

// ===== Host Edit Dialog =====

interface HostEditFormValues {
  displayName: string
  description: string
}

function HostEditDialog({
  host,
  scope,
  onClose,
  onSuccess,
}: {
  host: Host | null
  scope: ScopeRef
  onClose: () => void
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)

  const schema = z.object({
    displayName: z
      .string()
      .max(128, t("api.validation.maxLength", { max: 128 }))
      .optional(),
    description: z
      .string()
      .max(1000, t("api.validation.maxLength", { max: 1000 }))
      .optional(),
  })

  const form = useForm<HostEditFormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: { displayName: "", description: "" },
  })

  useEffect(() => {
    if (host) {
      form.reset({
        displayName: host.spec.displayName ?? "",
        description: host.spec.description ?? "",
      })
    }
  }, [host, form])

  const onSubmit = async (values: HostEditFormValues) => {
    if (!host) return
    setLoading(true)
    try {
      await hostsApi.update(scope, host.metadata.id, {
        spec: {
          displayName: values.displayName,
          description: values.description,
        },
      })
      toast.success(t("action.updateSuccess"))
      onClose()
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "host", "compute.host.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={!!host}
      onOpenChange={(v) => {
        if (!v) onClose()
      }}
      title={t("compute.host.edit")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:max-w-lg"
    >
      <FormField
        control={form.control}
        name="displayName"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("common.displayName")}</FormLabel>
            <FormControl>
              <Input {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="description"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("common.description")}</FormLabel>
            <FormControl>
              <Textarea rows={3} {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </FormDialog>
  )
}
