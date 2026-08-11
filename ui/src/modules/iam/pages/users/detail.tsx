import { useEffect, useId, useMemo, useState } from "react"
import { useParams, useNavigate, Link } from "react-router"
import { Pencil, Trash2, Search, Filter, KeyRound } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { TruncateText } from "@/shared/components/truncate-text"
import { TruncateCell } from "@/shared/components/truncate-cell"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"

import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import {
  updateUser,
  listUsers,
  listUserWorkspaces,
  listUserNamespaces,
  usersApi,
} from "@/modules/iam/api/users"
import { listUserRoleBindings } from "@/modules/iam/api/rbac"
import { usersDef } from "@/modules/iam/defs"
import { qk } from "@/core/query/keys"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient } from "@tanstack/react-query"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { ListParams } from "@/core/api/types"
import type { User } from "@/modules/iam/api/types"
import { useTranslation } from "@/i18n"
import { useListState } from "@/frameworks/list/use-list-state"
import { SortIcon } from "@/shared/components/sort-icon"
import { Pagination } from "@/shared/components/pagination"
import { usePermission } from "@/core/permission/use-permission"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { FormDialog } from "@/frameworks/form/form-dialog"
import { ResetPasswordDialog } from "@/modules/iam/components/reset-password-dialog"
import { useAuthStore } from "@/core/auth/auth-store"

export default function UserDetailPage() {
  const { userId } = useParams()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const currentUserSub = useAuthStore((s) => s.user?.sub)
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [resetOpen, setResetOpen] = useState(false)

  const detailQuery = useApiQuery({
    queryKey: qk.detail(usersDef, {}, userId ?? ""),
    queryFn: () => usersApi.get({}, userId!),
    enabled: !!userId,
  })
  const user = detailQuery.data ?? null
  const loading = detailQuery.isPending
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(usersDef) })

  const handleDelete = async () => {
    if (!user) return
    try {
      await usersApi.delete({}, user.metadata.id)
      qc.invalidateQueries({ queryKey: qk.resource(usersDef) })
      toast.success(t("action.deleteSuccess"))
      navigate("/iam/users")
    } catch (err) {
      showApiError(err, t, "user.title")
    }
  }

  if (loading) {
    return (
      <div className="space-y-4 p-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-32 w-full" />
      </div>
    )
  }

  if (!user) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">{t("user.notFound")}</p>
      </div>
    )
  }

  return (
    <div className="p-6">
      {/* header */}
      <div className="mb-6 flex items-center justify-between gap-4">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <h1 className="min-w-0 flex-1 text-2xl font-bold">
            <TruncateText>{user.spec.username}</TruncateText>
          </h1>
          <Badge variant={user.spec.status === "active" ? "default" : "secondary"}>
            {user.spec.status === "active" ? t("common.active") : t("common.inactive")}
          </Badge>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {hasPermission("iam:users:update") && (
            <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
              <Pencil className="mr-2 h-4 w-4" />
              {t("common.edit")}
            </Button>
          )}
          {hasPermission("iam:users:reset-password") && user.metadata.id !== currentUserSub && (
            <Button variant="outline" size="sm" onClick={() => setResetOpen(true)}>
              <KeyRound className="mr-2 h-4 w-4" />
              {t("user.resetPassword")}
            </Button>
          )}
          {hasPermission("iam:users:delete") && !user.spec.builtin && (
            <Button variant="destructive" size="sm" onClick={() => setDeleteOpen(true)}>
              <Trash2 className="mr-2 h-4 w-4" />
              {t("common.delete")}
            </Button>
          )}
        </div>
      </div>

      <div className="space-y-6">
        {/* user info card */}
        <Card>
          <CardHeader>
            <CardTitle>{t("user.details")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-x-8 gap-y-4 text-sm md:grid-cols-2">
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("user.username")}</span>
                <p className="font-medium">
                  <TruncateText>{user.spec.username}</TruncateText>
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("common.displayName")}</span>
                <p className="font-medium">
                  <TruncateText>{user.spec.displayName || "-"}</TruncateText>
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("user.email")}</span>
                <p className="font-medium">
                  <TruncateText>{user.spec.email}</TruncateText>
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("common.phone")}</span>
                <p className="font-medium">
                  <TruncateText>{user.spec.phone || "-"}</TruncateText>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("common.status")}</span>
                <p>
                  <Badge variant={user.spec.status === "active" ? "default" : "secondary"}>
                    {user.spec.status === "active" ? t("common.active") : t("common.inactive")}
                  </Badge>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("common.created")}</span>
                <p className="font-medium">{new Date(user.metadata.createdAt).toLocaleString()}</p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("common.updated")}</span>
                <p className="font-medium">{new Date(user.metadata.updatedAt).toLocaleString()}</p>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* associated workspaces */}
        <UserWorkspacesCard userId={user.metadata.id} />

        {/* associated namespaces */}
        <UserNamespacesCard userId={user.metadata.id} />

        {/* role bindings */}
        <UserRoleBindingsCard userId={user.metadata.id} />
      </div>

      {/* edit dialog */}
      <EditUserDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        user={user}
        onSuccess={invalidate}
      />

      {/* delete confirm */}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("common.delete")}
        description={t("user.deleteConfirm", { name: user.spec.username })}
        onConfirm={handleDelete}
        confirmText={t("common.delete")}
      />

      {/* reset password */}
      <ResetPasswordDialog
        open={resetOpen}
        onOpenChange={setResetOpen}
        userId={user.metadata.id}
        username={user.spec.username}
      />
    </div>
  )
}

// ===== Joined Workspaces Card =====

function UserWorkspacesCard({ userId }: { userId: string }) {
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
  } = useListState({ defaultSortBy: "joined_at", defaultSortOrder: "desc", defaultPageSize: 10 })
  const [statusFilter, setStatusFilter] = useState("all")

  const params: ListParams = { page, pageSize, sortBy, sortOrder }
  if (search) params.search = search
  if (statusFilter !== "all") params.status = statusFilter
  const listQuery = useApiQuery({
    queryKey: qk.sub(usersDef, {}, userId, "workspaces", params),
    queryFn: () => listUserWorkspaces(userId, params),
  })
  const workspaces = listQuery.data?.items ?? []
  const totalCount = listQuery.data?.totalCount ?? 0
  const loading = listQuery.isPending

  useEffect(() => {
    setPage(1)
  }, [search, statusFilter, pageSize, setPage])

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("user.workspaces")}</CardTitle>
      </CardHeader>
      <CardContent>
        {/* toolbar */}
        <div className="mb-4 flex items-center gap-3">
          <div className="relative max-w-xs">
            <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
            <Input
              name="user-workspaces-search"
              placeholder={t("common.search")}
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className="pl-9"
            />
          </div>
        </div>

        {/* table */}
        <div className="border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("name")}
                >
                  {t("common.name")}
                  <SortIcon field="name" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("display_name")}
                >
                  {t("common.displayName")}
                  <SortIcon field="display_name" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead>{t("workspace.owner")}</TableHead>
                <TableHead
                  className="cursor-pointer text-center select-none"
                  onClick={() => handleSort("namespace_count")}
                >
                  {t("workspace.namespaceCount")}
                  <SortIcon field="namespace_count" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead
                  className="cursor-pointer text-center select-none"
                  onClick={() => handleSort("member_count")}
                >
                  {t("workspace.memberCount")}
                  <SortIcon field="member_count" sortBy={sortBy} sortOrder={sortOrder} />
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
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("role_name")}
                >
                  {t("user.role")}
                  <SortIcon field="role_name" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("joined_at")}
                >
                  {t("user.joinedAt")}
                  <SortIcon field="joined_at" sortBy={sortBy} sortOrder={sortOrder} />
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
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 10 }).map((_, j) => (
                      <TableCell key={j}>
                        <Skeleton className="h-4 w-16" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : workspaces.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={10} className="text-muted-foreground py-10 text-center">
                    {t("user.noWorkspaces")}
                  </TableCell>
                </TableRow>
              ) : (
                workspaces.map((ws) => (
                  <TableRow key={ws.metadata.id}>
                    <TableCell className="font-medium">
                      <Link
                        to={`/iam/workspaces/${ws.metadata.id}`}
                        className="text-primary hover:underline"
                      >
                        {ws.metadata.name}
                      </Link>
                    </TableCell>
                    <TruncateCell>{ws.spec.displayName || "-"}</TruncateCell>
                    <TruncateCell>{ws.spec.ownerName || "-"}</TruncateCell>
                    <TableCell className="text-center">{ws.spec.namespaceCount ?? 0}</TableCell>
                    <TableCell className="text-center">{ws.spec.memberCount ?? 0}</TableCell>
                    <TableCell>
                      <Badge variant={ws.spec.status === "active" ? "default" : "secondary"}>
                        {ws.spec.status === "active" ? t("common.active") : t("common.inactive")}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {ws.spec.roles && ws.spec.roles.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {ws.spec.roles.map((r, idx) => (
                            <Badge key={r} variant="outline">
                              {t(`role.${r}`, {
                                defaultValue: ws.spec.roleDisplayNames?.[idx] || r,
                              })}
                            </Badge>
                          ))}
                        </div>
                      ) : (
                        "-"
                      )}
                    </TableCell>
                    <TruncateCell className="text-muted-foreground text-sm">
                      {ws.spec.joinedAt ? new Date(ws.spec.joinedAt).toLocaleString() : "-"}
                    </TruncateCell>
                    <TruncateCell className="text-muted-foreground text-sm">
                      {new Date(ws.metadata.createdAt).toLocaleString()}
                    </TruncateCell>
                    <TruncateCell className="text-muted-foreground text-sm">
                      {new Date(ws.metadata.updatedAt).toLocaleString()}
                    </TruncateCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        {/* pagination */}
        {totalCount > 0 && (
          <Pagination
            page={page}
            pageSize={pageSize}
            totalCount={totalCount}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        )}
      </CardContent>
    </Card>
  )
}

// ===== Joined Namespaces Card =====

function UserNamespacesCard({ userId }: { userId: string }) {
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
  } = useListState({ defaultSortBy: "joined_at", defaultSortOrder: "desc", defaultPageSize: 10 })
  const [statusFilter, setStatusFilter] = useState("all")

  const params: ListParams = { page, pageSize, sortBy, sortOrder }
  if (search) params.search = search
  if (statusFilter !== "all") params.status = statusFilter
  const listQuery = useApiQuery({
    queryKey: qk.sub(usersDef, {}, userId, "namespaces", params),
    queryFn: () => listUserNamespaces(userId, params),
  })
  const namespaces = listQuery.data?.items ?? []
  const totalCount = listQuery.data?.totalCount ?? 0
  const loading = listQuery.isPending

  useEffect(() => {
    setPage(1)
  }, [search, statusFilter, pageSize, setPage])

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("user.namespaceRefs")}</CardTitle>
      </CardHeader>
      <CardContent>
        {/* toolbar */}
        <div className="mb-4 flex items-center gap-3">
          <div className="relative max-w-xs">
            <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
            <Input
              name="user-namespaces-search"
              placeholder={t("common.search")}
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className="pl-9"
            />
          </div>
        </div>

        {/* table */}
        <div className="border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("name")}
                >
                  {t("common.name")}
                  <SortIcon field="name" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("display_name")}
                >
                  {t("common.displayName")}
                  <SortIcon field="display_name" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead>{t("namespace.workspaceName")}</TableHead>
                <TableHead>{t("namespace.owner")}</TableHead>
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
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("role_name")}
                >
                  {t("user.role")}
                  <SortIcon field="role_name" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("joined_at")}
                >
                  {t("user.joinedAt")}
                  <SortIcon field="joined_at" sortBy={sortBy} sortOrder={sortOrder} />
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
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 10 }).map((_, j) => (
                      <TableCell key={j}>
                        <Skeleton className="h-4 w-16" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : namespaces.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={10} className="text-muted-foreground py-10 text-center">
                    {t("user.noNamespaceRefs")}
                  </TableCell>
                </TableRow>
              ) : (
                namespaces.map((ns) => (
                  <TableRow key={ns.metadata.id}>
                    <TableCell className="font-medium">
                      <Link
                        to={`/iam/namespaces/${ns.metadata.id}`}
                        className="text-primary hover:underline"
                      >
                        {ns.metadata.name}
                      </Link>
                    </TableCell>
                    <TruncateCell>
                      {ns.metadata.name.endsWith("-default") && ns.spec.displayName === "Default"
                        ? t("namespace.builtinDefault")
                        : ns.spec.displayName || "-"}
                    </TruncateCell>
                    <TableCell>
                      {ns.spec.workspaceName ? (
                        <Link
                          to={`/iam/workspaces/${ns.spec.workspaceId}`}
                          className="text-primary hover:underline"
                        >
                          {ns.spec.workspaceName}
                        </Link>
                      ) : (
                        "-"
                      )}
                    </TableCell>
                    <TruncateCell>{ns.spec.ownerName || "-"}</TruncateCell>
                    <TableCell>
                      <Badge variant={ns.spec.status === "active" ? "default" : "secondary"}>
                        {ns.spec.status === "active" ? t("common.active") : t("common.inactive")}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-center">{ns.spec.memberCount ?? 0}</TableCell>
                    <TableCell>
                      {ns.spec.roles && ns.spec.roles.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {ns.spec.roles.map((r, idx) => (
                            <Badge key={r} variant="outline">
                              {t(`role.${r}`, {
                                defaultValue: ns.spec.roleDisplayNames?.[idx] || r,
                              })}
                            </Badge>
                          ))}
                        </div>
                      ) : (
                        "-"
                      )}
                    </TableCell>
                    <TruncateCell className="text-muted-foreground text-sm">
                      {ns.spec.joinedAt ? new Date(ns.spec.joinedAt).toLocaleString() : "-"}
                    </TruncateCell>
                    <TruncateCell className="text-muted-foreground text-sm">
                      {new Date(ns.metadata.createdAt).toLocaleString()}
                    </TruncateCell>
                    <TruncateCell className="text-muted-foreground text-sm">
                      {new Date(ns.metadata.updatedAt).toLocaleString()}
                    </TruncateCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>

        {/* pagination */}
        {totalCount > 0 && (
          <Pagination
            page={page}
            pageSize={pageSize}
            totalCount={totalCount}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        )}
      </CardContent>
    </Card>
  )
}

// ===== User Role Bindings Card =====

function UserRoleBindingsCard({ userId }: { userId: string }) {
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
  } = useListState({ defaultSortBy: "created_at", defaultSortOrder: "desc", defaultPageSize: 10 })
  const [scopeFilter, setScopeFilter] = useState("all")

  const params: ListParams = { page: 1, pageSize: 100, sortBy, sortOrder }
  if (scopeFilter !== "all") params.scope = scopeFilter
  const listQuery = useApiQuery({
    queryKey: qk.sub(usersDef, {}, userId, "rolebindings", params),
    queryFn: () => listUserRoleBindings(userId, params),
  })
  const allBindings = listQuery.data?.items ?? []
  const loading = listQuery.isPending

  useEffect(() => {
    setPage(1)
  }, [search, scopeFilter, pageSize, setPage])

  // Client-side filtering: match against translated role name, English role name, and display name
  const filtered = useMemo(() => {
    if (!search) return allBindings
    const q = search.toLowerCase()
    return allBindings.filter((b) => {
      const roleName = b.spec.roleName || ""
      const roleDisplayName = b.spec.roleDisplayName || ""
      const translated = t(`role.${roleName}`, { defaultValue: roleDisplayName || roleName })
      return (
        roleName.toLowerCase().includes(q) ||
        roleDisplayName.toLowerCase().includes(q) ||
        translated.toLowerCase().includes(q)
      )
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allBindings, search])

  // Client-side pagination
  const totalCount = filtered.length
  const bindings = useMemo(() => {
    const start = (page - 1) * pageSize
    return filtered.slice(start, start + pageSize)
  }, [filtered, page, pageSize])

  const scopeLabel = (scope: string) => {
    if (scope === "platform") return t("rolebinding.scope.platform")
    if (scope === "workspace") return t("rolebinding.scope.workspace")
    return t("rolebinding.scope.namespace")
  }

  const scopeVariant = (scope: string): "default" | "secondary" | "outline" => {
    if (scope === "platform") return "default"
    if (scope === "workspace") return "secondary"
    return "outline"
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("user.rolebindings")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="mb-4 flex items-center gap-3">
          <div className="relative max-w-xs">
            <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
            <Input
              name="user-rolebindings-search"
              placeholder={t("common.search")}
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className="pl-9"
            />
          </div>
        </div>
        <div className="border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("role_name")}
                >
                  {t("role.title")}
                  <SortIcon field="role_name" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <button className="inline-flex items-center gap-1 select-none">
                        {t("rolebinding.scope")}
                        <Filter
                          className={`h-3 w-3 ${scopeFilter !== "all" ? "text-primary" : "opacity-40"}`}
                        />
                      </button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start">
                      <DropdownMenuItem onClick={() => setScopeFilter("all")}>
                        {t("common.all")}
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => setScopeFilter("platform")}>
                        {t("rolebinding.scope.platform")}
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => setScopeFilter("workspace")}>
                        {t("rolebinding.scope.workspace")}
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => setScopeFilter("namespace")}>
                        {t("rolebinding.scope.namespace")}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableHead>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("scope_target")}
                >
                  {t("rolebinding.scopeTarget")}
                  <SortIcon field="scope_target" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead
                  className="cursor-pointer select-none"
                  onClick={() => handleSort("created_at")}
                >
                  {t("common.created")}
                  <SortIcon field="created_at" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 4 }).map((_, j) => (
                      <TableCell key={j}>
                        <Skeleton className="h-4 w-16" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : bindings.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="text-muted-foreground py-8 text-center">
                    {t("user.noRolebindings")}
                  </TableCell>
                </TableRow>
              ) : (
                bindings.map((b) => (
                  <TableRow key={b.metadata.id}>
                    <TableCell>
                      <Badge variant="secondary">
                        {t(`role.${b.spec.roleName}`, {
                          defaultValue: b.spec.roleDisplayName || b.spec.roleName || "",
                        })}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={scopeVariant(b.spec.scope)}>{scopeLabel(b.spec.scope)}</Badge>
                    </TableCell>
                    <TableCell className="text-sm">
                      {b.spec.scope === "platform" ? (
                        t("rolebinding.scope.platform")
                      ) : b.spec.scope === "namespace" ? (
                        <span>
                          {b.spec.workspaceName && (
                            <Link
                              to={`/iam/workspaces/${b.spec.workspaceId}`}
                              className="text-primary hover:underline"
                            >
                              {b.spec.workspaceName}
                            </Link>
                          )}
                          {b.spec.workspaceName && b.spec.namespaceName && " / "}
                          {b.spec.namespaceName && (
                            <Link
                              to={`/iam/workspaces/${b.spec.workspaceId}/namespaces/${b.spec.namespaceId}`}
                              className="text-primary hover:underline"
                            >
                              {b.spec.namespaceName}
                            </Link>
                          )}
                        </span>
                      ) : b.spec.workspaceName ? (
                        <Link
                          to={`/iam/workspaces/${b.spec.workspaceId}`}
                          className="text-primary hover:underline"
                        >
                          {b.spec.workspaceName}
                        </Link>
                      ) : (
                        "-"
                      )}
                    </TableCell>
                    <TruncateCell className="text-muted-foreground text-sm">
                      {new Date(b.metadata.createdAt).toLocaleString()}
                    </TruncateCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
        {totalCount > 0 && (
          <Pagination
            page={page}
            pageSize={pageSize}
            totalCount={totalCount}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        )}
      </CardContent>
    </Card>
  )
}

// ===== Edit User Dialog =====

function EditUserDialog({
  open,
  onOpenChange,
  user,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: User
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const userNameId = useId()
  const [loading, setLoading] = useState(false)

  const schema = z.object({
    email: z.email(t("api.validation.email.format")),
    displayName: z.string().optional(),
    phone: z
      .string()
      .min(1, t("api.validation.required", { field: t("common.phone") }))
      .regex(/^1[3-9]\d{9}$/, t("api.validation.phone.format")),
    status: z.enum(["active", "inactive"]),
  })

  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: {
      email: user.spec.email,
      displayName: user.spec.displayName ?? "",
      phone: user.spec.phone ?? "",
      status: user.spec.status ?? "active",
    },
  })

  useEffect(() => {
    if (open) {
      form.reset({
        email: user.spec.email,
        displayName: user.spec.displayName ?? "",
        phone: user.spec.phone ?? "",
        status: user.spec.status ?? "active",
      })
    }
  }, [open, user, form])

  const checkUniqueness = async (field: "email" | "phone", value: string) => {
    if (!value) return
    try {
      const data = await listUsers({ page: 1, pageSize: 1, [field]: value })
      const exists = data.items?.some((u) => {
        if (u.metadata.id === user.metadata.id) return false
        return u.spec[field]?.toLowerCase() === value.toLowerCase()
      })
      if (exists) form.setError(field, { message: t(`api.validation.${field}.taken`) })
    } catch {
      /* backend will enforce */
    }
  }

  const onSubmit = async (values: FormValues) => {
    setLoading(true)
    try {
      await updateUser(user.metadata.id, {
        metadata: user.metadata,
        spec: {
          ...user.spec,
          email: values.email,
          displayName: values.displayName,
          phone: values.phone,
          status: values.status,
        },
      })
      toast.success(t("action.updateSuccess"))
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "user", "user.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("user.edit")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:max-w-lg"
    >
      <div>
        <label htmlFor={userNameId} className="text-sm font-medium">
          {t("user.username")}
        </label>
        <Input
          id={userNameId}
          name="user-name"
          value={user.spec.username}
          disabled
          className="mt-1"
        />
      </div>
      <FormField
        control={form.control}
        name="email"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("user.email")}</FormLabel>
            <FormControl>
              <Input
                type="email"
                {...field}
                onBlur={async (e) => {
                  field.onBlur()
                  if (!e.target.value) return
                  const valid = await form.trigger("email")
                  if (valid) checkUniqueness("email", e.target.value)
                }}
              />
            </FormControl>
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
        name="phone"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("common.phone")}</FormLabel>
            <FormControl>
              <Input
                {...field}
                onBlur={async (e) => {
                  field.onBlur()
                  if (!e.target.value) return
                  const valid = await form.trigger("phone")
                  if (valid) checkUniqueness("phone", e.target.value)
                }}
              />
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
    </FormDialog>
  )
}
