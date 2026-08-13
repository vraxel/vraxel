import { useEffect, useState } from "react"
import { formatDateTime } from "@/shared/lib/format"
import { Plus, Pencil, Trash2 } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { useScopeStore } from "@/core/scope/scope-store"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"

import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import {
  listWorkspaces,
  createWorkspace,
  updateWorkspace,
  workspacesApi,
} from "@/modules/iam/api/workspaces"
import { workspacesDef } from "@/modules/iam/defs"
import { useQueryClient } from "@tanstack/react-query"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { NameCell } from "@/frameworks/list/name-cell"
import { StatusFilter } from "@/frameworks/list/status-filter"
import { ActiveStatusBadge } from "@/shared/components/active-status-badge"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { useApiMutation } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { Workspace } from "@/modules/iam/api/types"
import { useTranslation } from "@/i18n"
import { usePermission } from "@/core/permission/use-permission"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { FormDialog } from "@/frameworks/form/form-dialog"

export default function WorkspaceListPage() {
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const canCreate = hasPermission("iam:workspaces:create")
  const canBatch = hasPermission("iam:workspaces:deleteCollection")

  const [createOpen, setCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<Workspace | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Workspace | null>(null)
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false)

  const query = useListQuery<Workspace>({
    def: workspacesDef,
    api: workspacesApi,
    scope: {},
    filterKeys: ["status"],
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(workspacesDef) })

  const deleteMutation = useApiMutation({
    mutationFn: (id: string) => workspacesApi.delete({}, id),
    invalidates: [qk.resource(workspacesDef)],
    onSuccess: () => {
      useScopeStore.getState().invalidate()
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
    },
    onError: (err) => showApiError(err, t, "workspace.title"),
  })
  const batchDeleteMutation = useApiMutation({
    mutationFn: (ids: string[]) => workspacesApi.deleteCollection({}, ids),
    invalidates: [qk.resource(workspacesDef)],
    onSuccess: () => {
      useScopeStore.getState().invalidate()
      toast.success(t("action.deleteSuccess"))
      setBatchDeleteOpen(false)
      query.clearSelection()
    },
    onError: (err) => showApiError(err, t, "workspace.title"),
  })

  const columns: ColumnDef<Workspace>[] = [
    {
      key: "name",
      header: t("common.name"),
      sortable: true,
      cell: (ws) => (
        <NameCell
          to={`/iam/workspaces/${ws.metadata.id}`}
          displayName={ws.spec.displayName}
          name={ws.metadata.name}
          trailing={<ActiveStatusBadge status={ws.spec.status} />}
        />
      ),
    },
    {
      key: "description",
      header: t("common.description"),
      truncate: true,
      className: "text-muted-foreground text-sm",
      cell: (ws) => ws.spec.description || "-",
    },
    {
      key: "owner",
      header: t("workspace.owner"),
      truncate: true,
      className: "text-sm",
      cell: (ws) => ws.spec.ownerName || ws.spec.ownerId,
    },
    {
      key: "namespace_count",
      header: t("workspace.namespaceCount"),
      sortable: true,
      headClassName: "text-center",
      className: "text-center",
      cell: (ws) => ws.spec.namespaceCount ?? 0,
    },
    {
      key: "member_count",
      header: t("workspace.memberCount"),
      sortable: true,
      headClassName: "text-center",
      className: "text-center",
      cell: (ws) => ws.spec.memberCount ?? 0,
    },
    {
      key: "created_at",
      header: t("common.created"),
      sortable: true,
      className: "text-muted-foreground text-sm whitespace-nowrap",
      cell: (ws) => formatDateTime(ws.metadata.createdAt),
    },
    {
      key: "createdBy",
      header: t("common.createdBy"),
      truncate: true,
      className: "text-sm",
      cell: (ws) => ws.spec.createdByName || "-",
    },
    {
      key: "updated_at",
      header: t("common.updated"),
      sortable: true,
      className: "text-muted-foreground text-sm whitespace-nowrap",
      cell: (ws) => formatDateTime(ws.metadata.updatedAt),
    },
  ]

  return (
    <ResourceListPage
      query={query}
      columns={columns}
      titleKey="workspace.title"
      subtitle={t("workspace.manage", { count: query.totalCount })}
      searchPlaceholderKey="workspace.searchPlaceholder"
      // The status filter follows its column into the toolbar: left on
      // the name header it would read as filtering by name.
      toolbarExtra={
        <StatusFilter
          value={query.filters.status ?? "all"}
          onChange={(v) => query.setFilter("status", v)}
          options={[
            { value: "active", label: t("common.active") },
            { value: "inactive", label: t("common.inactive") },
          ]}
        />
      }
      selectable={canBatch}
      emptyKey="workspace.noData"
      createButton={
        canCreate && (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("workspace.create")}
          </Button>
        )
      }
      batchActions={
        canBatch && (
          <Button variant="destructive" size="sm" onClick={() => setBatchDeleteOpen(true)}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("workspace.batchDelete")} ({query.selected.size})
          </Button>
        )
      }
      rowActions={(ws) => (
        <>
          {hasPermission("iam:workspaces:update", { workspaceId: ws.metadata.id }) && (
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={() => setEditTarget(ws)}
              title={t("common.edit")}
            >
              <Pencil className="h-3.5 w-3.5" />
            </Button>
          )}
          {hasPermission("iam:workspaces:delete", { workspaceId: ws.metadata.id }) && (
            <Button
              variant="ghost"
              size="icon"
              className="text-destructive hover:text-destructive h-8 w-8"
              onClick={() => setDeleteTarget(ws)}
              title={t("common.delete")}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          )}
        </>
      )}
    >
      {/* create dialog */}
      <WorkspaceFormDialog open={createOpen} onOpenChange={setCreateOpen} onSuccess={invalidate} />

      {/* edit dialog */}
      <WorkspaceFormDialog
        open={!!editTarget}
        onOpenChange={(v) => {
          if (!v) setEditTarget(null)
        }}
        workspace={editTarget ?? undefined}
        onSuccess={invalidate}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        description={t("workspace.deleteConfirm", { name: deleteTarget?.metadata.name ?? "" })}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.metadata.id)
        }}
        confirmText={t("common.delete")}
      />

      <ConfirmDialog
        open={batchDeleteOpen}
        onOpenChange={setBatchDeleteOpen}
        title={t("workspace.batchDelete")}
        description={t("workspace.batchDeleteConfirm", { count: query.selected.size })}
        onConfirm={() => batchDeleteMutation.mutate(Array.from(query.selected))}
        confirmText={t("common.delete")}
      />
    </ResourceListPage>
  )
}

// ===== Workspace Form Dialog =====

interface WorkspaceFormValues {
  name: string
  displayName: string
  description: string
  status: "active" | "inactive"
}

function WorkspaceFormDialog({
  open,
  onOpenChange,
  workspace,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspace?: Workspace
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const isEdit = !!workspace
  const [loading, setLoading] = useState(false)

  const schema = z.object({
    // Edit 模式下 name 被 disabled 且 update 路径不改写它，强约束在历史
    // 违例数据上会卡死保存按钮（#159 同款）。
    name: isEdit
      ? z.string()
      : z
          .string()
          .min(3, t("workspace.validation.name.format"))
          .max(50, t("workspace.validation.name.format"))
          .regex(/^[a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9]$/, t("workspace.validation.name.format")),
    displayName: z
      .string()
      .max(128, t("api.validation.maxLength", { max: 128 }))
      .optional(),
    description: z
      .string()
      .max(1000, t("api.validation.maxLength", { max: 1000 }))
      .optional(),
    status: z.enum(["active", "inactive"]),
  })

  const form = useForm<WorkspaceFormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: { name: "", displayName: "", description: "", status: "active" },
  })

  useEffect(() => {
    if (open) {
      if (workspace) {
        form.reset({
          name: workspace.metadata.name,
          displayName: workspace.spec.displayName ?? "",
          description: workspace.spec.description ?? "",
          status: workspace.spec.status ?? "active",
        })
      } else {
        form.reset({ name: "", displayName: "", description: "", status: "active" })
      }
    }
  }, [open, workspace, form])

  const checkUniqueness = async (value: string) => {
    if (!value || isEdit) return
    try {
      const data = await listWorkspaces({ page: 1, pageSize: 1, search: value })
      const exists = data.items?.some((w) => w.metadata.name === value)
      if (exists) form.setError("name", { message: t("workspace.validation.name.taken") })
    } catch {
      /* backend will enforce */
    }
  }

  const onSubmit = async (values: WorkspaceFormValues) => {
    setLoading(true)
    try {
      if (isEdit) {
        await updateWorkspace(workspace.metadata.id, {
          metadata: workspace.metadata,
          spec: {
            ...workspace.spec,
            displayName: values.displayName,
            description: values.description,
            status: values.status,
          },
        })
        toast.success(t("action.updateSuccess"))
      } else {
        await createWorkspace({
          metadata: { name: values.name } as Workspace["metadata"],
          spec: {
            displayName: values.displayName,
            description: values.description,
            status: values.status,
          } as Workspace["spec"],
        })
        toast.success(t("action.createSuccess"))
      }
      useScopeStore.getState().invalidate()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "workspace", "workspace.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? t("workspace.edit") : t("workspace.create")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:max-w-lg"
    >
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t("common.name")}</FormLabel>
            <FormControl>
              <Input
                {...field}
                disabled={isEdit}
                placeholder="my-workspace"
                onBlur={async (e) => {
                  field.onBlur()
                  if (!e.target.value) return
                  const valid = await form.trigger("name")
                  if (valid) checkUniqueness(e.target.value)
                }}
              />
            </FormControl>
            {!isEdit && (
              <p className="text-muted-foreground text-xs">{t("workspace.validation.name.hint")}</p>
            )}
            <FormMessage />
          </FormItem>
        )}
      />
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
      <FormField
        control={form.control}
        name="status"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t("common.status")}</FormLabel>
            <Select name={field.name} value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="active">{t("common.active")}</SelectItem>
                <SelectItem value="inactive">{t("common.inactive")}</SelectItem>
              </SelectContent>
            </Select>
            <FormMessage />
          </FormItem>
        )}
      />
    </FormDialog>
  )
}
