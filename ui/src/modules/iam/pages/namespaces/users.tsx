import { useEffect, useMemo, useState } from "react"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { useParams, Link, Navigate } from "react-router"
import { Plus, UserMinus, Search, Filter } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { RowActionsCell, RowActionsHead } from "@/shared/components/row-actions"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog"
import { Checkbox } from "@/shared/ui/checkbox"
import {
  listNamespaceUsers,
  addNamespaceUsers,
  removeNamespaceUsers,
} from "@/modules/iam/api/users"
import { listNamespaceRoles } from "@/modules/iam/api/rbac"
import type { ListParams } from "@/core/api/types"
import type { User } from "@/modules/iam/api/types"
import { showApiError } from "@/core/api/client"
import { useTranslation } from "@/i18n"
import { useListState } from "@/frameworks/list/use-list-state"
import { usePermission } from "@/core/permission/use-permission"
import { usePermissionStore } from "@/core/permission/permission-store"
import { SortIcon } from "@/shared/components/sort-icon"
import { Pagination } from "@/shared/components/pagination"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"

export default function NamespaceUsersPage() {
  const workspaceId = useParams().workspaceId!
  const namespaceId = useParams().namespaceId!
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const usersBasePath = `/iam/workspaces/${workspaceId}/namespaces/${namespaceId}/users`
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
  } = useListState()

  const permissionsLoaded = usePermissionStore((s) => s.permissions) !== null

  const [statusFilter, setStatusFilter] = useState("all")
  const qc = useQueryClient()
  const membersQuery = useApiQuery({
    queryKey: [
      "iam",
      "namespace-users",
      workspaceId,
      namespaceId,
      page,
      pageSize,
      sortBy,
      sortOrder,
      search,
      statusFilter,
    ],
    queryFn: () => {
      const params: ListParams = { page, pageSize, sortBy, sortOrder }
      if (search) params.search = search
      if (statusFilter !== "all") params.status = statusFilter
      return listNamespaceUsers(workspaceId, namespaceId, params)
    },
    placeholderData: keepPreviousData,
  })
  const members = useMemo(() => membersQuery.data?.items ?? [], [membersQuery.data])
  const loading = membersQuery.isPending
  const totalCount = membersQuery.data?.totalCount ?? 0
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["iam", "namespace-users", workspaceId, namespaceId] })
    // Adding members consumes available=true candidates; drop that cache too (C11).
    void qc.invalidateQueries({
      queryKey: ["iam", "namespace-add-users", workspaceId, namespaceId],
    })
  }

  const [addOpen, setAddOpen] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<User | null>(null)
  const [batchRemoveOpen, setBatchRemoveOpen] = useState(false)

  useEffect(() => {
    setPage(1)
    clearSelection()
  }, [search, statusFilter, pageSize, setPage, clearSelection])

  if (permissionsLoaded && !hasPermission("iam:users:list", { workspaceId, namespaceId })) {
    return <Navigate to="/" replace />
  }

  const handleRemove = async () => {
    if (!removeTarget) return
    try {
      const result = await removeNamespaceUsers(workspaceId, namespaceId, [
        removeTarget.metadata.id,
      ])
      if (result.failedCount > 0) {
        toast.error(t("api.error.cannotRemoveOwner"))
      } else {
        toast.success(t("namespace.memberRemoved"))
      }
      setRemoveTarget(null)
      refresh()
    } catch (err) {
      showApiError(err, t, "user.title")
    }
  }

  const handleBatchRemove = async () => {
    try {
      const result = await removeNamespaceUsers(workspaceId, namespaceId, Array.from(selected))
      if (result.failedCount > 0) {
        toast.warning(
          t("namespace.memberPartialRemoved", {
            success: result.successCount,
            failed: result.failedCount,
          }),
        )
      } else {
        toast.success(t("namespace.memberRemoved"))
      }
      setBatchRemoveOpen(false)
      clearSelection()
      refresh()
    } catch (err) {
      showApiError(err, t, "user.title")
    }
  }

  const handleAddSuccess = () => {
    refresh()
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("namespace.members")}</h1>
          <p className="text-muted-foreground text-sm">
            {t("namespace.membersManage", { count: totalCount })}
          </p>
        </div>
        {hasPermission("iam:users:create", { workspaceId, namespaceId }) && (
          <Button onClick={() => setAddOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("namespace.addMember")}
          </Button>
        )}
      </div>
      <div className="mb-4 flex items-center gap-3">
        <div className="relative max-w-xs flex-1">
          <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
          <Input
            name="namespace-user-search"
            placeholder={t("user.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        {selected.size > 0 &&
          hasPermission("iam:users:deleteCollection", { workspaceId, namespaceId }) && (
            <Button variant="destructive" size="sm" onClick={() => setBatchRemoveOpen(true)}>
              <UserMinus className="mr-2 h-4 w-4" />
              {t("namespace.removeMember")} ({selected.size})
            </Button>
          )}
      </div>

      {/* table */}
      <div className="border">
        <Table>
          <TableHeader>
            <TableRow>
              {hasPermission("iam:users:deleteCollection", { workspaceId, namespaceId }) && (
                <TableHead className="w-10">
                  <Checkbox
                    checked={
                      selected.size === 0 || members.length === 0
                        ? false
                        : selected.size === members.length
                          ? true
                          : "indeterminate"
                    }
                    onCheckedChange={() => toggleAll(members.map((m) => m.metadata.id))}
                  />
                </TableHead>
              )}
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort("username")}
              >
                {t("user.username")}
                <SortIcon field="username" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead className="cursor-pointer select-none" onClick={() => handleSort("email")}>
                {t("user.email")}
                <SortIcon field="email" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort("display_name")}
              >
                {t("common.displayName")}
                <SortIcon field="display_name" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead className="cursor-pointer select-none" onClick={() => handleSort("phone")}>
                {t("common.phone")}
                <SortIcon field="phone" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead>{t("user.role")}</TableHead>
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
                className="cursor-pointer select-none"
                onClick={() => handleSort("created_at")}
              >
                {t("common.created")}
                <SortIcon field="created_at" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead
                className="cursor-pointer select-none"
                onClick={() => handleSort("updated_at")}
              >
                {t("common.updated")}
                <SortIcon field="updated_at" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              {hasPermission("iam:users:deleteCollection", { workspaceId, namespaceId }) && (
                <RowActionsHead size="sm">{t("common.actions")}</RowActionsHead>
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 10 }).map((_, j) => (
                    <TableCell key={j}>
                      <Skeleton className="h-4 w-20" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : members.length === 0 ? (
              <TableRow>
                <TableCell colSpan={10} className="text-muted-foreground py-8 text-center">
                  {t("namespace.noMembers")}
                </TableCell>
              </TableRow>
            ) : (
              members.map((m) => (
                <TableRow key={m.metadata.id}>
                  {hasPermission("iam:users:deleteCollection", { workspaceId, namespaceId }) && (
                    <TableCell>
                      <Checkbox
                        checked={selected.has(m.metadata.id)}
                        onCheckedChange={() => toggleOne(m.metadata.id)}
                      />
                    </TableCell>
                  )}
                  <TableCell className="font-medium">
                    <Link to={`${usersBasePath}/${m.metadata.id}`} className="hover:underline">
                      {m.spec.username}
                    </Link>
                  </TableCell>
                  <TableCell>{m.spec.email}</TableCell>
                  <TableCell>{m.spec.displayName || "-"}</TableCell>
                  <TableCell>{m.spec.phone || "-"}</TableCell>
                  <TableCell>
                    {m.spec.role ? t(`role.${m.spec.role}`, { defaultValue: m.spec.role }) : "-"}
                  </TableCell>
                  <TableCell>
                    <Badge variant={m.spec.status === "active" ? "default" : "secondary"}>
                      {m.spec.status === "active" ? t("common.active") : t("common.inactive")}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm whitespace-nowrap">
                    {new Date(m.metadata.createdAt).toLocaleString()}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm whitespace-nowrap">
                    {new Date(m.metadata.updatedAt).toLocaleString()}
                  </TableCell>
                  {hasPermission("iam:users:deleteCollection", { workspaceId, namespaceId }) && (
                    <RowActionsCell size="sm">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="text-destructive hover:text-destructive h-8 w-8"
                        onClick={() => setRemoveTarget(m)}
                        title={t("namespace.removeMember")}
                      >
                        <UserMinus className="h-3.5 w-3.5" />
                      </Button>
                    </RowActionsCell>
                  )}
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

      {/* add member dialog */}
      <AddMemberDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        workspaceId={workspaceId}
        namespaceId={namespaceId}
        onSuccess={handleAddSuccess}
      />

      {/* remove confirm */}
      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(v) => {
          if (!v) setRemoveTarget(null)
        }}
        title={t("namespace.removeMember")}
        description={t("namespace.removeMemberConfirm", {
          name: removeTarget?.spec.username ?? "",
        })}
        onConfirm={handleRemove}
        confirmText={t("common.confirm")}
      />

      {/* batch remove confirm */}
      <ConfirmDialog
        open={batchRemoveOpen}
        onOpenChange={setBatchRemoveOpen}
        title={t("namespace.removeMember")}
        description={t("namespace.batchRemoveMemberConfirm", { count: selected.size })}
        onConfirm={handleBatchRemove}
        confirmText={t("common.confirm")}
      />
    </div>
  )
}

// ===== Add Member Dialog =====

function AddMemberDialog({
  open,
  onOpenChange,
  workspaceId,
  namespaceId,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspaceId: string
  namespaceId: string
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const [selectedRoleId, setSelectedRoleId] = useState("")
  const [defaultRoleId, setDefaultRoleId] = useState("")
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [submitting, setSubmitting] = useState(false)
  const [searchQuery, setSearchQuery] = useState("")

  // available=true returns platform users not yet in this namespace (avoids
  // the platform-level listUsers which namespace admins lack permission for).
  const dataQuery = useApiQuery({
    queryKey: ["iam", "namespace-add-users", workspaceId, namespaceId],
    queryFn: async () => {
      const [userData, roleData] = await Promise.all([
        listNamespaceUsers(workspaceId, namespaceId, { pageSize: 100, available: "true" }),
        listNamespaceRoles(workspaceId, namespaceId, { pageSize: 100 }),
      ])
      return { users: userData.items ?? [], roles: roleData.items ?? [] }
    },
    enabled: open,
  })
  const allUsers = useMemo(() => dataQuery.data?.users ?? [], [dataQuery.data])
  const roles = dataQuery.data?.roles ?? []
  const loading = open && dataQuery.isPending

  // Reset selection + default the role to namespace-viewer when the dialog
  // opens (or once its data lands). Adjust-during-render so the reset is
  // painted in the same pass, not one render late.
  const [prevResetKey, setPrevResetKey] = useState<unknown[]>([open, dataQuery.data])
  if (prevResetKey[0] !== open || prevResetKey[1] !== dataQuery.data) {
    setPrevResetKey([open, dataQuery.data])
    if (open) {
      setSelectedIds(new Set())
      setSearchQuery("")
      const viewer = (dataQuery.data?.roles ?? []).find((r) => r.spec.name === "namespace-viewer")
      setSelectedRoleId(viewer?.metadata.id ?? "")
      setDefaultRoleId(viewer?.metadata.id ?? "")
    }
  }

  // Backend already filtered out current members; keep the variable name for
  // a smaller diff with the search/filter code below.
  const availableUsers = allUsers

  const filteredUsers = searchQuery
    ? availableUsers.filter((u) => {
        const q = searchQuery.toLowerCase()
        return (
          u.spec.username.toLowerCase().includes(q) ||
          u.spec.email?.toLowerCase().includes(q) ||
          u.spec.displayName?.toLowerCase().includes(q) ||
          u.spec.phone?.includes(q)
        )
      })
    : availableUsers

  const handleToggle = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const handleSubmit = async () => {
    if (selectedIds.size === 0) return
    setSubmitting(true)
    try {
      const userIds = Array.from(selectedIds)
      const roleId = selectedRoleId && selectedRoleId !== defaultRoleId ? selectedRoleId : undefined
      await addNamespaceUsers(workspaceId, namespaceId, userIds, roleId)
      toast.success(t("namespace.memberAdded"))
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      showApiError(err, t, "user.title")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[85vh] flex-col overflow-hidden"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t("namespace.addMember")}</DialogTitle>
          <DialogDescription>{t("namespace.addMemberDesc")}</DialogDescription>
        </DialogHeader>
        <div className="-mx-1 min-h-0 flex-1 overflow-y-auto px-1">
          <div>
            <p className="mb-2 text-sm font-medium">{t("rolebinding.selectRole")}</p>
            <div className="max-h-[120px] overflow-auto border">
              {loading ? (
                <div className="space-y-2 p-4">
                  {Array.from({ length: 2 }).map((_, i) => (
                    <Skeleton key={i} className="h-8 w-full" />
                  ))}
                </div>
              ) : (
                roles.map((role) => (
                  <label
                    key={role.metadata.id}
                    className={`hover:bg-muted/50 flex cursor-pointer items-center gap-3 px-4 py-2 ${selectedRoleId === role.metadata.id ? "bg-muted" : ""}`}
                  >
                    <Checkbox
                      checked={selectedRoleId === role.metadata.id}
                      onCheckedChange={() => setSelectedRoleId(role.metadata.id)}
                    />
                    <div className="flex-1">
                      <p className="text-sm font-medium">
                        {t(`role.${role.spec.name}`, {
                          defaultValue: role.spec.displayName || role.spec.name,
                        })}
                      </p>
                      <p className="text-muted-foreground text-xs">
                        {t(`role.desc.${role.spec.name}`, {
                          defaultValue: role.spec.description || "",
                        }) || "-"}
                      </p>
                    </div>
                  </label>
                ))
              )}
            </div>
          </div>
          <div>
            <p className="mb-2 text-sm font-medium">{t("rolebinding.selectUser")}</p>
            <div className="relative mb-2">
              <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
              <Input
                name="namespace-rolebinding-user-search"
                placeholder={t("user.searchPlaceholder")}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9"
              />
            </div>
            <div className="max-h-[200px] overflow-auto border">
              {loading ? (
                <div className="space-y-2 p-4">
                  {Array.from({ length: 3 }).map((_, i) => (
                    <Skeleton key={i} className="h-8 w-full" />
                  ))}
                </div>
              ) : filteredUsers.length === 0 ? (
                <p className="text-muted-foreground p-4 text-center text-sm">
                  {searchQuery ? t("common.noSearchResults") : t("namespace.noAvailableUsers")}
                </p>
              ) : (
                filteredUsers.map((user) => {
                  const isInactive = user.spec.status === "inactive"
                  return (
                    <label
                      key={user.metadata.id}
                      className={`hover:bg-muted/50 flex cursor-pointer items-center gap-3 px-4 py-2 ${isInactive ? "opacity-50" : ""}`}
                    >
                      <Checkbox
                        checked={selectedIds.has(user.metadata.id)}
                        onCheckedChange={() => handleToggle(user.metadata.id)}
                      />
                      <div className="flex-1">
                        <p className="text-sm font-medium">
                          {user.spec.username}
                          {isInactive && (
                            <Badge variant="secondary" className="ml-2 text-[10px]">
                              {t("common.inactive")}
                            </Badge>
                          )}
                        </p>
                        <p className="text-muted-foreground text-xs">
                          {user.spec.displayName || user.spec.email}
                        </p>
                      </div>
                    </label>
                  )
                })
              )}
            </div>
          </div>
        </div>
        <DialogFooter className="mt-6 shrink-0 border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={selectedIds.size === 0 || !selectedRoleId || submitting}
          >
            {submitting ? "..." : t("namespace.addMember")}{" "}
            {selectedIds.size > 0 && `(${selectedIds.size})`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
