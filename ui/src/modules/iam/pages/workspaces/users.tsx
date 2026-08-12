import { useEffect, useMemo, useState } from "react"
import { formatDateTime } from "@/shared/lib/format"
import { NameCell } from "@/frameworks/list/name-cell"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient, keepPreviousData } from "@tanstack/react-query"
import { useParams, Navigate } from "react-router"
import { Plus, UserMinus, Search, Filter, Loader2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { EmptyState } from "@/shared/components/empty-state"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { RowActionsCell, RowActionsHead } from "@/shared/components/row-actions"
import { TruncateCell } from "@/shared/components/truncate-cell"
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
  listWorkspaceUsers,
  addWorkspaceUsers,
  removeWorkspaceUsers,
} from "@/modules/iam/api/users"
import { listWorkspaceRoles } from "@/modules/iam/api/rbac"
import type { ListParams } from "@/core/api/types"
import type { User } from "@/modules/iam/api/types"
import { showApiError } from "@/core/api/client"
import { useTranslation } from "@/i18n"
import { usePermission } from "@/core/permission/use-permission"
import { usePermissionStore } from "@/core/permission/permission-store"
import { useListState } from "@/frameworks/list/use-list-state"
import { SortIcon } from "@/shared/components/sort-icon"
import { Pagination } from "@/shared/components/pagination"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"

export default function WorkspaceUsersPage() {
  const workspaceId = useParams().workspaceId!
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
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
      "workspace-users",
      workspaceId,
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
      return listWorkspaceUsers(workspaceId, params)
    },
    placeholderData: keepPreviousData,
  })
  const members = useMemo(() => membersQuery.data?.items ?? [], [membersQuery.data])
  const loading = membersQuery.isPending
  const totalCount = membersQuery.data?.totalCount ?? 0
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["iam", "workspace-users", workspaceId] })
    // Adding members consumes available=true candidates; drop that cache too
    // or a reopen within staleTime re-offers the just-added user (C11).
    void qc.invalidateQueries({ queryKey: ["iam", "workspace-add-users", workspaceId] })
  }

  const [addOpen, setAddOpen] = useState(false)
  const [removeTarget, setRemoveTarget] = useState<User | null>(null)
  const [batchRemoveOpen, setBatchRemoveOpen] = useState(false)

  useEffect(() => {
    setPage(1)
    clearSelection()
  }, [search, statusFilter, pageSize, setPage, clearSelection])

  if (permissionsLoaded && !hasPermission("iam:users:list", { workspaceId })) {
    return <Navigate to="/" replace />
  }

  const handleRemove = async () => {
    if (!removeTarget) return
    try {
      const result = await removeWorkspaceUsers(workspaceId, [removeTarget.metadata.id])
      if (result.failedCount > 0) {
        toast.error(t("api.error.cannotRemoveOwner"))
      } else {
        toast.success(t("workspace.memberRemoved"))
      }
      setRemoveTarget(null)
      refresh()
    } catch (err) {
      showApiError(err, t, "user.title")
    }
  }

  const handleBatchRemove = async () => {
    try {
      const result = await removeWorkspaceUsers(workspaceId, Array.from(selected))
      if (result.failedCount > 0) {
        toast.warning(
          t("workspace.memberPartialRemoved", {
            success: result.successCount,
            failed: result.failedCount,
          }),
        )
      } else {
        toast.success(t("workspace.memberRemoved"))
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
          <h1 className="text-xl font-semibold tracking-tight">{t("workspace.members")}</h1>
          <p className="text-muted-foreground text-sm">
            {t("workspace.membersManage", { count: totalCount })}
          </p>
        </div>
        {hasPermission("iam:users:create", { workspaceId }) && (
          <Button onClick={() => setAddOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("workspace.addMember")}
          </Button>
        )}
      </div>
      <div className="mb-4 flex items-center gap-3">
        <div className="relative max-w-xs flex-1">
          <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
          <Input
            name="workspace-user-search"
            placeholder={t("user.searchPlaceholder")}
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        {selected.size > 0 && hasPermission("iam:users:deleteCollection", { workspaceId }) && (
          <Button variant="destructive" size="sm" onClick={() => setBatchRemoveOpen(true)}>
            <UserMinus className="mr-2 h-4 w-4" />
            {t("workspace.removeMember")} ({selected.size})
          </Button>
        )}
      </div>

      {/* table */}
      <div className="border-border-subtle overflow-hidden rounded-xl border shadow-sm">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">
                {hasPermission("iam:users:deleteCollection", { workspaceId }) && (
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
                )}
              </TableHead>
              <TableHead
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("username")}
              >
                {t("user.username")}
                <SortIcon field="username" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("email")}
              >
                {t("user.email")}
                <SortIcon field="email" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("phone")}
              >
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
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("created_at")}
              >
                {t("common.created")}
                <SortIcon field="created_at" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              <TableHead
                className="hover:text-foreground cursor-pointer transition-colors select-none"
                onClick={() => handleSort("updated_at")}
              >
                {t("common.updated")}
                <SortIcon field="updated_at" sortBy={sortBy} sortOrder={sortOrder} />
              </TableHead>
              {hasPermission("iam:users:deleteCollection", { workspaceId }) && (
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
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={10} className="p-0 whitespace-normal">
                  <EmptyState title={t("workspace.noMembers")} />
                </TableCell>
              </TableRow>
            ) : (
              members.map((m) => (
                <TableRow key={m.metadata.id}>
                  <TableCell>
                    {hasPermission("iam:users:deleteCollection", { workspaceId }) && (
                      <Checkbox
                        checked={selected.has(m.metadata.id)}
                        onCheckedChange={() => toggleOne(m.metadata.id)}
                      />
                    )}
                  </TableCell>
                  <TableCell>
                    <NameCell
                      to={`/iam/workspaces/${workspaceId}/users/${m.metadata.id}`}
                      displayName={m.spec.displayName}
                      name={m.spec.username}
                    />
                  </TableCell>
                  <TruncateCell>{m.spec.email}</TruncateCell>
                  <TruncateCell>{m.spec.phone || "-"}</TruncateCell>
                  <TableCell>
                    {m.spec.roles && m.spec.roles.length > 0 ? (
                      <div className="flex flex-wrap gap-1">
                        {m.spec.roles.map((r) => (
                          <Badge key={r} variant="secondary">
                            {t(`role.${r}`, { defaultValue: r })}
                          </Badge>
                        ))}
                      </div>
                    ) : (
                      "-"
                    )}
                  </TableCell>
                  <TableCell>
                    <Badge variant={m.spec.status === "active" ? "default" : "secondary"}>
                      {m.spec.status === "active" ? t("common.active") : t("common.inactive")}
                    </Badge>
                  </TableCell>
                  <TruncateCell className="text-muted-foreground text-sm">
                    {formatDateTime(m.metadata.createdAt)}
                  </TruncateCell>
                  <TruncateCell className="text-muted-foreground text-sm">
                    {formatDateTime(m.metadata.updatedAt)}
                  </TruncateCell>
                  {hasPermission("iam:users:deleteCollection", { workspaceId }) && (
                    <RowActionsCell size="sm">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="text-destructive hover:text-destructive h-8 w-8"
                        onClick={() => setRemoveTarget(m)}
                        title={t("workspace.removeMember")}
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
        onSuccess={handleAddSuccess}
      />

      {/* remove confirm */}
      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(v) => {
          if (!v) setRemoveTarget(null)
        }}
        title={t("workspace.removeMember")}
        description={t("workspace.removeMemberConfirm", {
          name: removeTarget?.spec.username ?? "",
        })}
        onConfirm={handleRemove}
        confirmText={t("common.confirm")}
      />

      {/* batch remove confirm */}
      <ConfirmDialog
        open={batchRemoveOpen}
        onOpenChange={setBatchRemoveOpen}
        title={t("workspace.removeMember")}
        description={t("workspace.batchRemoveMemberConfirm", { count: selected.size })}
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
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspaceId: string
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const [selectedRoleId, setSelectedRoleId] = useState("")
  const [defaultRoleId, setDefaultRoleId] = useState("")
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [submitting, setSubmitting] = useState(false)
  const [searchQuery, setSearchQuery] = useState("")

  // available=true asks the backend for platform users not yet in this
  // workspace (workspace admins lack iam:users:list:platform; the
  // workspace-scope endpoint runs under iam:users:list:workspace).
  const dataQuery = useApiQuery({
    queryKey: ["iam", "workspace-add-users", workspaceId],
    queryFn: async () => {
      const [userData, roleData] = await Promise.all([
        listWorkspaceUsers(workspaceId, { pageSize: 100, available: "true" }),
        listWorkspaceRoles(workspaceId, { pageSize: 100 }),
      ])
      return { users: userData.items ?? [], roles: roleData.items ?? [] }
    },
    enabled: open,
  })
  const allUsers = useMemo(() => dataQuery.data?.users ?? [], [dataQuery.data])
  const roles = dataQuery.data?.roles ?? []
  const loading = open && dataQuery.isPending

  // Reset selection + default the role to workspace-viewer when the dialog
  // opens (or once its data lands). Adjust-during-render so the reset is
  // painted in the same pass, not one render late.
  const [prevResetKey, setPrevResetKey] = useState<unknown[]>([open, dataQuery.data])
  if (prevResetKey[0] !== open || prevResetKey[1] !== dataQuery.data) {
    setPrevResetKey([open, dataQuery.data])
    if (open) {
      setSelectedIds(new Set())
      setSearchQuery("")
      const viewer = (dataQuery.data?.roles ?? []).find((r) => r.spec.name === "workspace-viewer")
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
      await addWorkspaceUsers(workspaceId, userIds, roleId)
      toast.success(t("workspace.memberAdded"))
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
          <DialogTitle>{t("workspace.addMember")}</DialogTitle>
          <DialogDescription>{t("workspace.addMemberDesc")}</DialogDescription>
        </DialogHeader>
        <div className="-mx-1 min-h-0 flex-1 overflow-y-auto px-1">
          <div>
            <p className="mb-2 text-sm font-medium">{t("rolebinding.selectRole")}</p>
            <div className="max-h-[120px] overflow-auto rounded-lg border">
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
                name="workspace-rolebinding-user-search"
                placeholder={t("user.searchPlaceholder")}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9"
              />
            </div>
            <div className="max-h-[200px] overflow-auto rounded-lg border">
              {loading ? (
                <div className="space-y-2 p-4">
                  {Array.from({ length: 3 }).map((_, i) => (
                    <Skeleton key={i} className="h-8 w-full" />
                  ))}
                </div>
              ) : filteredUsers.length === 0 ? (
                <p className="text-muted-foreground p-4 text-center text-sm">
                  {searchQuery ? t("common.noSearchResults") : t("workspace.noAvailableUsers")}
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
            {submitting && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
            {t("workspace.addMember")} {selectedIds.size > 0 && `(${selectedIds.size})`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
