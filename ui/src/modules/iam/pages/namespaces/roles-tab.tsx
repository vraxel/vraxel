import { useEffect, useMemo, useState } from "react"
import { formatDateTime } from "@/shared/lib/format"
import { NameCell } from "@/frameworks/list/name-cell"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { useParams, Navigate } from "react-router"
import { Plus, Pencil, Trash2, Search } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { RowActionsCell, RowActionsHead } from "@/shared/components/row-actions"
import { Checkbox } from "@/shared/ui/checkbox"
import {
  listNamespaceRoles,
  deleteNamespaceRole,
  deleteNamespaceRoles,
  listAllPermissions,
} from "@/modules/iam/api/rbac"
import type { ListParams } from "@/core/api/types"
import type { Role } from "@/modules/iam/api/types"
import { showApiError } from "@/core/api/client"
import { useTranslation, findBuiltinRoleNamesMatching } from "@/i18n"
import { useListState, useListSelectionSync } from "@/frameworks/list/use-list-state"
import { usePermission } from "@/core/permission/use-permission"
import { usePermissionStore } from "@/core/permission/permission-store"
import { SortIcon } from "@/shared/components/sort-icon"
import { EmptyState } from "@/shared/components/empty-state"
import { Pagination } from "@/shared/components/pagination"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { ScopedRoleFormDialog } from "@/modules/iam/components/scoped-role-form-dialog"
import { TruncateCell } from "@/shared/components/truncate-cell"

export default function NamespaceRolesTab() {
  const workspaceId = useParams().workspaceId!
  const namespaceId = useParams().namespaceId!
  const { t, locale } = useTranslation()
  const rolesBasePath = `/iam/workspaces/${workspaceId}/namespaces/${namespaceId}/roles`
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

  const permissionsLoaded = usePermissionStore((s) => s.permissions) !== null

  const qc = useQueryClient()
  const rolesQuery = useApiQuery({
    queryKey: [
      "iam",
      "namespace-roles",
      workspaceId,
      namespaceId,
      page,
      pageSize,
      sortBy,
      sortOrder,
      search,
      locale,
    ],
    queryFn: () => {
      const params: ListParams = { page, pageSize, sortBy, sortOrder }
      if (search) {
        params.search = search
        const extra = findBuiltinRoleNamesMatching(search, locale)
        if (extra.length > 0) params.extra_names = extra.join(",")
      }
      return listNamespaceRoles(workspaceId, namespaceId, params)
    },
    placeholderData: keepPreviousData,
  })
  const roles = useMemo(() => rolesQuery.data?.items ?? [], [rolesQuery.data])
  const refresh = () =>
    qc.invalidateQueries({ queryKey: ["iam", "namespace-roles", workspaceId, namespaceId] })
  useListSelectionSync(syncSelection, roles)
  const loading = rolesQuery.isPending
  const totalCount = rolesQuery.data?.totalCount ?? 0
  const permissionsQuery = useApiQuery({
    queryKey: ["iam", "all-permissions"],
    queryFn: () => listAllPermissions(),
    meta: { skipGlobalError: true },
  })
  const permissions = permissionsQuery.data ?? []
  const [createOpen, setCreateOpen] = useState(false)
  const [editRole, setEditRole] = useState<Role | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Role | null>(null)
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false)

  useEffect(() => {
    setPage(1)
    clearSelection()
  }, [search, pageSize, setPage, clearSelection])

  if (permissionsLoaded && !hasPermission("iam:roles:list", { workspaceId, namespaceId })) {
    return <Navigate to="/" replace />
  }

  const canCreate = hasPermission("iam:roles:create", { workspaceId, namespaceId })
  const canUpdate = hasPermission("iam:roles:update", { workspaceId, namespaceId })
  const canDelete = hasPermission("iam:roles:delete", { workspaceId, namespaceId })

  const handleCreate = () => {
    setCreateOpen(true)
  }

  const handleEdit = (role: Role) => {
    setEditRole(role)
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteNamespaceRole(workspaceId, namespaceId, deleteTarget.metadata.id)
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
      refresh()
    } catch (err) {
      showApiError(err, t, "role.title")
    }
  }

  const handleBatchDelete = async () => {
    try {
      await deleteNamespaceRoles(workspaceId, namespaceId, Array.from(selected))
      toast.success(t("action.deleteSuccess"))
      setBatchDeleteOpen(false)
      clearSelection()
      refresh()
    } catch (err) {
      showApiError(err, t, "role.title")
    }
  }

  const selectableRoles = roles.filter((r) => !r.spec.builtin)
  const selectableIds = selectableRoles.map((r) => r.metadata.id)

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("role.title")}</h1>
          <p className="text-muted-foreground text-sm">{t("role.manage", { count: totalCount })}</p>
        </div>
        {canCreate && (
          <Button onClick={handleCreate}>
            <Plus className="mr-2 h-4 w-4" />
            {t("role.create")}
          </Button>
        )}
      </div>
      <div className="mb-4 flex items-center gap-3">
        <div className="relative max-w-xs flex-1">
          <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
          <Input
            name="namespace-role-search"
            placeholder={t("role.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        {canDelete && selected.size > 0 && (
          <Button variant="destructive" size="sm" onClick={() => setBatchDeleteOpen(true)}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("role.batchDelete")} ({selected.size})
          </Button>
        )}
      </div>

      <div className="border-border-subtle overflow-hidden rounded-xl border shadow-sm">
        <Table>
          <TableHeader>
            <TableRow>
              {canDelete && (
                <TableHead className="w-10">
                  <Checkbox
                    checked={
                      selected.size === 0 || selectableIds.length === 0
                        ? false
                        : selected.size === selectableIds.length
                          ? true
                          : "indeterminate"
                    }
                    onCheckedChange={() => toggleAll(selectableIds)}
                  />
                </TableHead>
              )}
              <TableHead
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("name")}
              >
                {t("common.name")}
                <SortIcon field="name" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead>{t("role.builtin")}</TableHead>
              <TableHead>{t("common.description")}</TableHead>
              <TableHead>{t("role.rules")}</TableHead>
              <TableHead
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("created_at")}
              >
                {t("common.created")}
                <SortIcon field="created_at" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              {(canUpdate || canDelete) && <RowActionsHead>{t("common.actions")}</RowActionsHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({
                    length: 6 + (canDelete ? 1 : 0) + (canUpdate || canDelete ? 1 : 0),
                  }).map((_, j) => (
                    <TableCell key={j}>
                      <Skeleton className="h-4 w-20" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : roles.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell
                  colSpan={6 + (canDelete ? 1 : 0) + (canUpdate || canDelete ? 1 : 0)}
                  className="p-0 whitespace-normal"
                >
                  <EmptyState title={t("role.noData")} />
                </TableCell>
              </TableRow>
            ) : (
              roles.map((role) => (
                <TableRow key={role.metadata.id}>
                  {canDelete && (
                    <TableCell>
                      <Checkbox
                        checked={selected.has(role.metadata.id)}
                        onCheckedChange={() => toggleOne(role.metadata.id)}
                        disabled={!!role.spec.builtin}
                      />
                    </TableCell>
                  )}
                  <TableCell>
                    <NameCell
                      to={`${rolesBasePath}/${role.metadata.id}`}
                      displayName={t(`role.${role.spec.name}`, {
                        defaultValue: role.spec.displayName ?? "",
                      })}
                      name={role.spec.name}
                    />
                  </TableCell>
                  <TableCell>
                    <Badge variant={role.spec.builtin ? "secondary" : "outline"}>
                      {role.spec.builtin ? t("role.builtin") : t("role.custom")}
                    </Badge>
                  </TableCell>
                  <TruncateCell>
                    {t(`role.desc.${role.spec.name}`, {
                      defaultValue: role.spec.description || "-",
                    })}
                  </TruncateCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {t("role.rulesCount", {
                      count: role.spec.ruleCount ?? role.spec.rules?.length ?? 0,
                    })}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm whitespace-nowrap">
                    {formatDateTime(role.metadata.createdAt)}
                  </TableCell>
                  {(canUpdate || canDelete) && (
                    <RowActionsCell>
                      {canUpdate && (
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8"
                          onClick={() => handleEdit(role)}
                          disabled={!!role.spec.builtin}
                          title={role.spec.builtin ? t("role.builtinCannotEdit") : t("common.edit")}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      {canDelete && (
                        <Button
                          variant="ghost"
                          size="icon"
                          className="text-destructive hover:text-destructive h-8 w-8"
                          onClick={() => setDeleteTarget(role)}
                          disabled={!!role.spec.builtin}
                          title={
                            role.spec.builtin ? t("role.builtinCannotDelete") : t("common.delete")
                          }
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      )}
                    </RowActionsCell>
                  )}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <Pagination
        totalCount={totalCount}
        page={page}
        pageSize={pageSize}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
      />

      <ScopedRoleFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        scope="namespace"
        scopeId={namespaceId}
        workspaceId={workspaceId}
        permissions={permissions}
        onSuccess={refresh}
      />

      <ScopedRoleFormDialog
        open={!!editRole}
        onOpenChange={(v) => {
          if (!v) setEditRole(null)
        }}
        scope="namespace"
        scopeId={namespaceId}
        workspaceId={workspaceId}
        role={editRole ?? undefined}
        permissions={permissions}
        onSuccess={refresh}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        description={t("role.deleteConfirm", { name: deleteTarget?.spec.name ?? "" })}
        onConfirm={handleDelete}
        confirmText={t("common.delete")}
      />

      <ConfirmDialog
        open={batchDeleteOpen}
        onOpenChange={setBatchDeleteOpen}
        title={t("role.batchDelete")}
        description={t("role.batchDeleteConfirm", { count: selected.size })}
        onConfirm={handleBatchDelete}
        confirmText={t("common.delete")}
      />
    </div>
  )
}
