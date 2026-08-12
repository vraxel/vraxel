import { useEffect, useMemo, useState } from "react"
import { NameCell } from "@/frameworks/list/name-cell"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { useParams } from "react-router"
import { Plus, Pencil, Trash2, Search, Filter } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { useScopeStore } from "@/core/scope/scope-store"
import { Button } from "@/shared/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { RowActionsCell, RowActionsHead } from "@/shared/components/row-actions"
import { Badge } from "@/shared/ui/badge"
import { Checkbox } from "@/shared/ui/checkbox"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"

import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import {
  listWorkspaceNamespaces,
  createWorkspaceNamespace,
  updateNamespace,
  deleteNamespace,
  deleteNamespaces,
  listNamespaces,
} from "@/modules/iam/api/namespaces"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { ListParams } from "@/core/api/types"
import type { Namespace } from "@/modules/iam/api/types"
import { useTranslation } from "@/i18n"
import { useListState, useListSelectionSync } from "@/frameworks/list/use-list-state"
import { usePermission } from "@/core/permission/use-permission"
import { SortIcon } from "@/shared/components/sort-icon"
import { EmptyState } from "@/shared/components/empty-state"
import { Pagination } from "@/shared/components/pagination"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { TruncateCell } from "@/shared/components/truncate-cell"
import { FormDialog } from "@/frameworks/form/form-dialog"

export default function WorkspaceNamespacesPage() {
  const workspaceId = useParams().workspaceId!
  const { t } = useTranslation()
  const {
    page,
    setPage,
    pageSize,
    setPageSize,
    sortBy,
    sortOrder,
    handleSort,
    searchInput,
    setSearchInput,
    search,
    selected,
    toggleAll,
    toggleOne,
    clearSelection,
    syncSelection,
  } = useListState()
  const { hasPermission } = usePermission()
  const permScope = { workspaceId }
  const [statusFilter, setStatusFilter] = useState("all")
  const [visibilityFilter, setVisibilityFilter] = useState("all")
  const qc = useQueryClient()
  const namespacesQuery = useApiQuery({
    queryKey: [
      "iam",
      "workspace-namespaces",
      workspaceId,
      page,
      pageSize,
      sortBy,
      sortOrder,
      search,
      statusFilter,
      visibilityFilter,
    ],
    queryFn: () => {
      const params: ListParams = { page, pageSize, sortBy, sortOrder }
      if (search) params.search = search
      if (statusFilter !== "all") params.status = statusFilter
      if (visibilityFilter !== "all") params.visibility = visibilityFilter
      return listWorkspaceNamespaces(workspaceId, params)
    },
    placeholderData: keepPreviousData,
  })
  const namespaces = useMemo(() => namespacesQuery.data?.items ?? [], [namespacesQuery.data])
  const refresh = () =>
    qc.invalidateQueries({ queryKey: ["iam", "workspace-namespaces", workspaceId] })
  useListSelectionSync(syncSelection, namespaces)
  const loading = namespacesQuery.isPending
  const totalCount = namespacesQuery.data?.totalCount ?? 0

  const [createOpen, setCreateOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<Namespace | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Namespace | null>(null)
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false)

  useEffect(() => {
    setPage(1)
    clearSelection()
  }, [search, statusFilter, visibilityFilter, pageSize, setPage, clearSelection])

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteNamespace(deleteTarget.metadata.id)
      useScopeStore.getState().invalidate()
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
      refresh()
    } catch (err) {
      showApiError(err, t, "namespace.title")
    }
  }

  const handleBatchDelete = async () => {
    try {
      await deleteNamespaces(Array.from(selected))
      useScopeStore.getState().invalidate()
      toast.success(t("action.deleteSuccess"))
      setBatchDeleteOpen(false)
      clearSelection()
      refresh()
    } catch (err) {
      showApiError(err, t, "namespace.title")
    }
  }

  const handleCreateSuccess = () => {
    refresh()
  }

  const handleEditSuccess = () => {
    refresh()
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("namespace.title")}</h1>
          <p className="text-muted-foreground text-sm">
            {t("namespace.manage", { count: totalCount })}
          </p>
        </div>
        {hasPermission("iam:namespaces:create", { workspaceId }) && (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("namespace.create")}
          </Button>
        )}
      </div>
      <div className="mb-4 flex items-center gap-3">
        <div className="relative max-w-xs flex-1">
          <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
          <Input
            name="workspace-namespace-search"
            placeholder={t("namespace.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        {selected.size > 0 && hasPermission("iam:namespaces:deleteCollection", permScope) && (
          <Button variant="destructive" size="sm" onClick={() => setBatchDeleteOpen(true)}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("namespace.batchDelete")} ({selected.size})
          </Button>
        )}
      </div>

      {/* table */}
      <div className="overflow-hidden rounded-xl border shadow-sm">
        <Table>
          <TableHeader>
            <TableRow>
              {hasPermission("iam:namespaces:deleteCollection", permScope) && (
                <TableHead className="w-10">
                  <Checkbox
                    checked={
                      selected.size === 0 || namespaces.length === 0
                        ? false
                        : selected.size === namespaces.length
                          ? true
                          : "indeterminate"
                    }
                    onCheckedChange={() => toggleAll(namespaces.map((ns) => ns.metadata.id))}
                  />
                </TableHead>
              )}
              <TableHead className="hover:text-foreground cursor-pointer transition-colors select-none" onClick={() => handleSort("name")}>
                {t("common.name")}
                <SortIcon field="name" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("description")}
              >
                {t("common.description")}
                <SortIcon field="description" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead>{t("namespace.owner")}</TableHead>
              <TableHead>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button className="inline-flex items-center gap-1 select-none">
                      {t("namespace.visibility")}
                      <Filter
                        className={`h-3 w-3 ${visibilityFilter !== "all" ? "text-primary" : "opacity-40"}`}
                      />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start">
                    <DropdownMenuItem onClick={() => setVisibilityFilter("all")}>
                      {t("common.all")}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setVisibilityFilter("public")}>
                      {t("namespace.visibility.public")}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setVisibilityFilter("private")}>
                      {t("namespace.visibility.private")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableHead>
              <TableHead>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button className="inline-flex items-center gap-1 select-none">
                      {t("common.status")}
                      <Filter
                        className={`h-3 w-3 ${statusFilter !== "all" ? "text-primary" : "opacity-40"}`}
                      />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start">
                    <DropdownMenuItem onClick={() => setStatusFilter("all")}>
                      {t("common.all")}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setStatusFilter("active")}>
                      {t("common.active")}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setStatusFilter("inactive")}>
                      {t("common.inactive")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableHead>
              <TableHead
                className="cursor-pointer text-center select-none"
                onClick={() => handleSort("member_count")}
              >
                {t("namespace.memberCount")}
                <SortIcon field="member_count" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("created_at")}
              >
                {t("common.created")}
                <SortIcon field="created_at" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead>{t("common.createdBy")}</TableHead>
              <RowActionsHead>{t("common.actions")}</RowActionsHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 11 }).map((_, j) => (
                    <TableCell key={j}>
                      <Skeleton className="h-4 w-16" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : namespaces.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={11} className="p-0 whitespace-normal">
                  <EmptyState title={t("namespace.noData")} />
                </TableCell>
              </TableRow>
            ) : (
              namespaces.map((ns) => (
                <TableRow key={ns.metadata.id}>
                  {hasPermission("iam:namespaces:deleteCollection", permScope) && (
                    <TableCell>
                      <Checkbox
                        checked={selected.has(ns.metadata.id)}
                        onCheckedChange={() => toggleOne(ns.metadata.id)}
                      />
                    </TableCell>
                  )}
                  <TableCell>
                    <NameCell
                      to={`/iam/workspaces/${workspaceId}/namespaces/${ns.metadata.id}`}
                      displayName={
                        ns.spec.displayName ||
                        (ns.metadata.name.endsWith("-default") ? t("namespace.builtinDefault") : "")
                      }
                      name={ns.metadata.name}
                    />
                  </TableCell>
                  {(() => {
                    const descText =
                      ns.spec.description ||
                      (ns.metadata.name.endsWith("-default")
                        ? t("namespace.builtinDefaultDesc", { name: ns.spec.workspaceName || "" })
                        : "-")
                    return (
                      <TruncateCell text={descText} className="text-muted-foreground text-sm">
                        {descText}
                      </TruncateCell>
                    )
                  })()}
                  <TableCell className="text-sm">{ns.spec.ownerName || ns.spec.ownerId}</TableCell>
                  <TableCell>
                    <Badge variant={ns.spec.visibility === "public" ? "default" : "secondary"}>
                      {ns.spec.visibility === "public"
                        ? t("namespace.visibility.public")
                        : t("namespace.visibility.private")}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge variant={ns.spec.status === "active" ? "default" : "secondary"}>
                      {ns.spec.status === "active" ? t("common.active") : t("common.inactive")}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-center">
                    {ns.spec.memberCount ?? 0}/{ns.spec.maxMembers || "∞"}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm whitespace-nowrap">
                    {new Date(ns.metadata.createdAt).toLocaleString()}
                  </TableCell>
                  <TruncateCell text={ns.spec.createdByName || "-"} className="text-sm">
                    {ns.spec.createdByName || "-"}
                  </TruncateCell>
                  <RowActionsCell>
                    {hasPermission("iam:namespaces:update", {
                      workspaceId,
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
                      workspaceId,
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
                  </RowActionsCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* pagination */}
      <Pagination
        totalCount={totalCount}
        page={page}
        pageSize={pageSize}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
      />

      {/* create dialog */}
      <NamespaceFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        workspaceId={workspaceId}
        onSuccess={handleCreateSuccess}
      />

      {/* edit dialog */}
      <NamespaceFormDialog
        open={!!editTarget}
        onOpenChange={(v) => {
          if (!v) setEditTarget(null)
        }}
        workspaceId={workspaceId}
        namespace={editTarget ?? undefined}
        onSuccess={handleEditSuccess}
      />

      {/* delete confirm */}
      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        description={t("namespace.deleteConfirm", { name: deleteTarget?.metadata.name ?? "" })}
        onConfirm={handleDelete}
        confirmText={t("common.delete")}
      />

      {/* batch delete confirm */}
      <ConfirmDialog
        open={batchDeleteOpen}
        onOpenChange={setBatchDeleteOpen}
        title={t("namespace.batchDelete")}
        description={t("namespace.batchDeleteConfirm", { count: selected.size })}
        onConfirm={handleBatchDelete}
        confirmText={t("common.delete")}
      />
    </div>
  )
}

// ===== Namespace Form Dialog =====

interface NamespaceFormValues {
  name: string
  displayName: string
  description: string
  visibility: "public" | "private"
  status: "active" | "inactive"
  maxMembers: number
}

function NamespaceFormDialog({
  open,
  onOpenChange,
  workspaceId,
  namespace,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspaceId: string
  namespace?: Namespace
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const isEdit = !!namespace
  const [loading, setLoading] = useState(false)

  const schema = z.object({
    // Edit 模式下 name 被 disabled 且 update 路径不改写它，强约束在历史
    // 违例数据上会卡死保存按钮（#159 同款）。
    name: isEdit
      ? z.string()
      : z
          .string()
          .min(3, t("namespace.validation.name.format"))
          .max(50, t("namespace.validation.name.format"))
          .regex(
            /^[a-zA-Z0-9][a-zA-Z0-9_-]{1,48}[a-zA-Z0-9]$/,
            t("namespace.validation.name.format"),
          ),
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
          displayName: namespace.spec.displayName ?? "",
          description: namespace.spec.description ?? "",
          visibility: namespace.spec.visibility ?? "public",
          status: namespace.spec.status ?? "active",
          maxMembers: namespace.spec.maxMembers ?? 0,
        })
      } else {
        form.reset({
          name: "",
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
      const exists = data.items?.some((n) => n.metadata.name === value)
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
        await createWorkspaceNamespace(workspaceId, {
          metadata: { name: values.name } as Namespace["metadata"],
          spec: {
            displayName: values.displayName,
            description: values.description,
            visibility: values.visibility,
            status: values.status,
            maxMembers: values.maxMembers,
            workspaceId,
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
            <FormLabel>{t("common.name")}</FormLabel>
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
            <FormLabel>{t("namespace.visibility")}</FormLabel>
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
            <FormLabel>{t("common.status")}</FormLabel>
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
            <FormLabel>{t("namespace.maxMembers")}</FormLabel>
            <FormControl>
              <Input
                type="number"
                min={0}
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
