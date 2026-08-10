import { useEffect, useMemo, useState } from "react"
import { useApiQuery } from "@/core/query/hooks"
import { Link, useParams } from "react-router"
import { Plus, Pencil, Trash2 } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import {
  Select,
  SelectContent,
  SelectEmpty,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/ui/select"

import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import {
  listNamespaces,
  createNamespace,
  updateNamespace,
  namespacesApi,
} from "@/modules/iam/api/namespaces"
import { namespacesDef } from "@/modules/iam/defs"
import { useQueryClient } from "@tanstack/react-query"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { useApiMutation } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { useScopeStore } from "@/core/scope/scope-store"
import { listWorkspaces } from "@/modules/iam/api/workspaces"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { Namespace } from "@/modules/iam/api/types"
import type { ScopeRef } from "@/core/registry/resource"
import { useTranslation } from "@/i18n"
import { usePermission } from "@/core/permission/use-permission"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { TruncateText } from "@/shared/components/truncate-text"
import { FormDialog } from "@/frameworks/form/form-dialog"

export default function NamespaceListPage() {
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const { workspaceId: scopeWorkspaceId } = useParams()
  const scope: ScopeRef = { ws: scopeWorkspaceId }
  const permScope = scopeWorkspaceId ? { workspaceId: scopeWorkspaceId } : undefined
  const canCreate = hasPermission("iam:namespaces:create", permScope)
  const canBatch = hasPermission("iam:namespaces:deleteCollection", permScope)

  const [createOpen, setCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<Namespace | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Namespace | null>(null)
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false)

  const query = useListQuery<Namespace>({
    def: namespacesDef,
    api: namespacesApi,
    scope,
    filterKeys: ["visibility", "status"],
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(namespacesDef) })

  const deleteMutation = useApiMutation({
    mutationFn: (id: string) => namespacesApi.delete(scope, id),
    invalidates: [qk.resource(namespacesDef)],
    onSuccess: () => {
      useScopeStore.getState().invalidate()
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
    },
    onError: (err) => showApiError(err, t, "namespace.title"),
  })
  const batchDeleteMutation = useApiMutation({
    mutationFn: (ids: string[]) => namespacesApi.deleteCollection(scope, ids),
    invalidates: [qk.resource(namespacesDef)],
    onSuccess: () => {
      useScopeStore.getState().invalidate()
      toast.success(t("action.deleteSuccess"))
      setBatchDeleteOpen(false)
      query.clearSelection()
    },
    onError: (err) => showApiError(err, t, "namespace.title"),
  })

  const columns: ColumnDef<Namespace>[] = [
    {
      key: "name",
      header: t("common.name"),
      sortable: true,
      truncate: true,
      cell: (ns) => (
        <Link to={`/iam/namespaces/${ns.metadata.id}`} className="font-medium hover:underline">
          {ns.metadata.name}
        </Link>
      ),
    },
    {
      key: "display_name",
      header: t("common.displayName"),
      sortable: true,
      truncate: true,
      cell: (ns) =>
        ns.spec.displayName ||
        (ns.metadata.name.endsWith("-default") ? t("namespace.builtinDefault") : "-"),
    },
    {
      key: "description",
      header: t("common.description"),
      sortable: true,
      truncate: true,
      className: "text-muted-foreground text-sm",
      cell: (ns) =>
        ns.spec.description ||
        (ns.metadata.name.endsWith("-default")
          ? t("namespace.builtinDefaultDesc", { name: ns.spec.workspaceName || "" })
          : "-"),
    },
    {
      key: "workspaceName",
      header: t("namespace.workspaceName"),
      truncate: true,
      className: "text-sm",
      cell: (ns) => (
        <Link to={`/iam/workspaces/${ns.spec.workspaceId}`} className="hover:underline">
          {ns.spec.workspaceName || ns.spec.workspaceId}
        </Link>
      ),
    },
    {
      key: "owner",
      header: t("namespace.owner"),
      truncate: true,
      className: "text-sm",
      cell: (ns) => ns.spec.ownerName || ns.spec.ownerId,
    },
    {
      key: "visibility",
      header: t("namespace.visibility"),
      filter: [
        { value: "all", label: t("common.all") },
        { value: "public", label: t("namespace.visibility.public") },
        { value: "private", label: t("namespace.visibility.private") },
      ],
      cell: (ns) => (
        <Badge variant="outline">
          {ns.spec.visibility === "private"
            ? t("namespace.visibility.private")
            : t("namespace.visibility.public")}
        </Badge>
      ),
    },
    {
      key: "status",
      header: t("common.status"),
      filter: [
        { value: "all", label: t("common.all") },
        { value: "active", label: t("common.active") },
        { value: "inactive", label: t("common.inactive") },
      ],
      cell: (ns) => (
        <Badge variant={ns.spec.status === "active" ? "default" : "secondary"}>
          {ns.spec.status === "active" ? t("common.active") : t("common.inactive")}
        </Badge>
      ),
    },
    {
      key: "member_count",
      header: t("namespace.memberCount"),
      sortable: true,
      headClassName: "text-center",
      className: "text-center",
      cell: (ns) => (
        <>
          {ns.spec.memberCount ?? 0}/{ns.spec.maxMembers || "∞"}
        </>
      ),
    },
    {
      key: "created_at",
      header: t("common.created"),
      sortable: true,
      truncate: true,
      className: "text-muted-foreground text-sm",
      cell: (ns) => new Date(ns.metadata.createdAt).toLocaleString(),
    },
    {
      key: "createdBy",
      header: t("common.createdBy"),
      truncate: true,
      className: "text-sm",
      cell: (ns) => ns.spec.createdByName || "-",
    },
  ]

  return (
    <ResourceListPage
      query={query}
      columns={columns}
      titleKey="namespace.title"
      subtitle={t("namespace.manage", { count: query.totalCount })}
      searchPlaceholderKey="namespace.searchPlaceholder"
      selectable={canBatch}
      emptyKey="namespace.noData"
      createButton={
        canCreate && (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("namespace.create")}
          </Button>
        )
      }
      batchActions={
        canBatch && (
          <Button variant="destructive" size="sm" onClick={() => setBatchDeleteOpen(true)}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("namespace.batchDelete")} ({query.selected.size})
          </Button>
        )
      }
      rowActions={(ns) => (
        <>
          {hasPermission("iam:namespaces:update", {
            workspaceId: ns.spec.workspaceId,
            namespaceId: ns.metadata.id,
          }) && (
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={() => setEditTarget(ns)}
              title={t("common.edit")}
            >
              <Pencil className="h-3.5 w-3.5" />
            </Button>
          )}
          {hasPermission("iam:namespaces:delete", {
            workspaceId: ns.spec.workspaceId,
            namespaceId: ns.metadata.id,
          }) && (
            <Button
              variant="ghost"
              size="icon"
              className="text-destructive hover:text-destructive h-8 w-8"
              onClick={() => setDeleteTarget(ns)}
              title={t("common.delete")}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          )}
        </>
      )}
    >
      {/* create dialog */}
      <NamespaceFormDialog open={createOpen} onOpenChange={setCreateOpen} onSuccess={invalidate} />

      {/* edit dialog */}
      <NamespaceFormDialog
        open={!!editTarget}
        onOpenChange={(v) => {
          if (!v) setEditTarget(null)
        }}
        namespace={editTarget ?? undefined}
        onSuccess={invalidate}
      />

      {/* delete confirm */}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        description={t("namespace.deleteConfirm", { name: deleteTarget?.metadata.name ?? "" })}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.metadata.id)
        }}
        confirmText={t("common.delete")}
      />

      {/* batch delete confirm */}
      <ConfirmDialog
        open={batchDeleteOpen}
        onOpenChange={setBatchDeleteOpen}
        title={t("namespace.batchDelete")}
        description={t("namespace.batchDeleteConfirm", { count: query.selected.size })}
        onConfirm={() => batchDeleteMutation.mutate(Array.from(query.selected))}
        confirmText={t("common.delete")}
      />
    </ResourceListPage>
  )
}

// ===== Namespace Form Dialog =====

interface NamespaceFormValues {
  name: string
  workspaceId: string
  displayName: string
  description: string
  visibility: "public" | "private"
  status: "active" | "inactive"
  maxMembers: number
}

function NamespaceFormDialog({
  open,
  onOpenChange,
  namespace,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  namespace?: Namespace
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const isEdit = !!namespace
  const [loading, setLoading] = useState(false)
  const workspacesQuery = useApiQuery({
    queryKey: ["iam", "workspaces", "active"],
    queryFn: () => listWorkspaces({ page: 1, pageSize: 100, status: "active" }),
    enabled: open,
    meta: { skipGlobalError: true },
  })
  const workspaces = useMemo(
    () => (open ? (workspacesQuery.data?.items ?? []) : []),
    [open, workspacesQuery.data],
  )
  const workspacesLoading = open && workspacesQuery.isPending

  const schema = z.object({
    // Edit 模式下 name 被 disabled 且 update 路径不改写它，强约束在历史
    // 违例数据上会卡死保存按钮（#159 同款）。
    name: isEdit
      ? z.string()
      : z
          .string()
          .min(3, t("namespace.validation.name.format"))
          .max(50, t("namespace.validation.name.format"))
          .regex(/^[a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9]$/, t("namespace.validation.name.format")),
    workspaceId: z
      .string()
      .min(1, t("api.validation.required", { field: t("namespace.workspaceName") })),
    displayName: z
      .string()
      .max(128, t("api.validation.maxLength", { max: 128 }))
      .optional(),
    description: z
      .string()
      .max(1000, t("api.validation.maxLength", { max: 1000 }))
      .optional(),
    visibility: z.enum(["public", "private"]),
    status: z.enum(["active", "inactive"]),
    maxMembers: z.coerce
      .number()
      .min(0, t("namespace.validation.maxMembers"))
      .max(1000000, t("namespace.validation.maxMembers"))
      .refine((v) => Number.isInteger(v), t("namespace.validation.maxMembers")),
  })

  const form = useForm<NamespaceFormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: {
      name: "",
      workspaceId: "",
      displayName: "",
      description: "",
      visibility: "public",
      status: "active",
      maxMembers: 0,
    },
  })

  useEffect(() => {
    if (open) {
      if (namespace) {
        form.reset({
          name: namespace.metadata.name,
          workspaceId: namespace.spec.workspaceId ?? "",
          displayName: namespace.spec.displayName ?? "",
          description: namespace.spec.description ?? "",
          visibility: namespace.spec.visibility ?? "public",
          status: namespace.spec.status ?? "active",
          maxMembers: namespace.spec.maxMembers ?? 0,
        })
      } else {
        form.reset({
          name: "",
          workspaceId: "",
          displayName: "",
          description: "",
          visibility: "public",
          status: "active",
          maxMembers: 0,
        })
      }
    }
  }, [open, namespace, form])

  const checkUniqueness = async (value: string) => {
    if (!value || isEdit) return
    try {
      const data = await listNamespaces({ page: 1, pageSize: 1, search: value })
      const exists = data.items?.some((ns) => ns.metadata.name === value)
      if (exists) form.setError("name", { message: t("namespace.validation.name.taken") })
    } catch {
      /* backend will enforce */
    }
  }

  const onSubmit = async (values: NamespaceFormValues) => {
    setLoading(true)
    try {
      if (isEdit) {
        await updateNamespace(namespace.metadata.id, {
          metadata: namespace.metadata,
          spec: {
            ...namespace.spec,
            displayName: values.displayName,
            description: values.description,
            visibility: values.visibility,
            status: values.status,
            maxMembers: values.maxMembers,
          },
        })
        toast.success(t("action.updateSuccess"))
      } else {
        await createNamespace({
          metadata: { name: values.name } as Namespace["metadata"],
          spec: {
            workspaceId: values.workspaceId,
            displayName: values.displayName,
            description: values.description,
            visibility: values.visibility,
            status: values.status,
            maxMembers: values.maxMembers,
          } as Namespace["spec"],
        })
        toast.success(t("action.createSuccess"))
      }
      useScopeStore.getState().invalidate()
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "namespace", "namespace.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? t("namespace.edit") : t("namespace.create")}
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
                placeholder="my-namespace"
                onBlur={async (e) => {
                  field.onBlur()
                  if (!e.target.value) return
                  const valid = await form.trigger("name")
                  if (valid) checkUniqueness(e.target.value)
                }}
              />
            </FormControl>
            {!isEdit && (
              <p className="text-muted-foreground text-xs">{t("namespace.validation.name.hint")}</p>
            )}
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="workspaceId"
        render={({ field }) => {
          const selectedWs = workspaces.find((ws) => ws.metadata.id === field.value)
          const wsLabel = (ws: (typeof workspaces)[number]) =>
            ws.spec.displayName || ws.metadata.name
          return (
            <FormItem>
              <FormLabel required>{t("namespace.workspaceName")}</FormLabel>
              <Select
                name={field.name}
                value={field.value}
                onValueChange={field.onChange}
                disabled={isEdit || workspacesLoading}
              >
                <FormControl>
                  <SelectTrigger className="w-full overflow-hidden">
                    <SelectValue
                      placeholder={workspacesLoading ? "..." : t("namespace.selectWorkspace")}
                    >
                      {selectedWs && (
                        <span className="block min-w-0">
                          <TruncateText>{wsLabel(selectedWs)}</TruncateText>
                        </span>
                      )}
                    </SelectValue>
                  </SelectTrigger>
                </FormControl>
                <SelectContent className="max-w-[var(--radix-select-trigger-width)]">
                  {workspaces.length === 0 ? (
                    <SelectEmpty>{t("common.noOptions")}</SelectEmpty>
                  ) : (
                    workspaces.map((ws) => (
                      <SelectItem key={ws.metadata.id} value={ws.metadata.id}>
                        <span className="block w-full min-w-0">
                          <TruncateText>{wsLabel(ws)}</TruncateText>
                        </span>
                      </SelectItem>
                    ))
                  )}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )
        }}
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
        name="visibility"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t("namespace.visibility")}</FormLabel>
            <Select name={field.name} value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="public">{t("namespace.visibility.public")}</SelectItem>
                <SelectItem value="private">{t("namespace.visibility.private")}</SelectItem>
              </SelectContent>
            </Select>
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
      <FormField
        control={form.control}
        name="maxMembers"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t("namespace.maxMembers")}</FormLabel>
            <FormControl>
              <Input
                type="number"
                min={0}
                max={1000000}
                {...field}
                onChange={(e) =>
                  field.onChange(e.target.value === "" ? "" : Number(e.target.value))
                }
              />
            </FormControl>
            <p className="text-muted-foreground text-xs">{t("namespace.maxMembersHint")}</p>
            <FormMessage />
          </FormItem>
        )}
      />
    </FormDialog>
  )
}
