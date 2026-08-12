import { useEffect, useMemo } from "react"
import { useParams } from "react-router"
import { Search } from "lucide-react"
import { EmptyState } from "@/shared/components/empty-state"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { TruncateText } from "@/shared/components/truncate-text"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { getWorkspaceUser, getNamespaceUser } from "@/modules/iam/api/users"
import {
  listUserRoleBindings,
  listWorkspaceUserRoleBindings,
  listNamespaceUserRoleBindings,
} from "@/modules/iam/api/rbac"
import type { ListParams } from "@/core/api/types"
import { useTranslation } from "@/i18n"
import { useApiQuery } from "@/core/query/hooks"
import { keepPreviousData } from "@tanstack/react-query"
import { useListState } from "@/frameworks/list/use-list-state"
import { SortIcon } from "@/shared/components/sort-icon"
import { Pagination } from "@/shared/components/pagination"

export default function ScopedUserDetailPage() {
  const { workspaceId, namespaceId, userId } = useParams()
  const { t } = useTranslation()
  const userQuery = useApiQuery({
    queryKey: ["iam", "user-detail", workspaceId, namespaceId ?? "", userId],
    queryFn: () =>
      namespaceId
        ? getNamespaceUser(workspaceId!, namespaceId, userId!)
        : getWorkspaceUser(workspaceId!, userId!),
    enabled: !!userId,
  })
  const user = userQuery.data ?? null
  const loading = userQuery.isPending

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
      <div className="mb-6 flex items-center gap-3">
        <h1 className="min-w-0 flex-1 text-xl font-semibold tracking-tight">
          <TruncateText>{user.spec.username}</TruncateText>
        </h1>
        <Badge variant={user.spec.status === "active" ? "default" : "secondary"}>
          {user.spec.status === "active" ? t("common.active") : t("common.inactive")}
        </Badge>
      </div>

      {/* user info card */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>{t("user.details")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-x-8 gap-y-4 text-sm md:grid-cols-2">
            <div className="min-w-0">
              <span className="text-muted-foreground mb-1 block text-xs">{t("user.username")}</span>
              <p className="font-medium">
                <TruncateText>{user.spec.username}</TruncateText>
              </p>
            </div>
            <div className="min-w-0">
              <span className="text-muted-foreground mb-1 block text-xs">
                {t("common.displayName")}
              </span>
              <p className="font-medium">
                <TruncateText>{user.spec.displayName || "-"}</TruncateText>
              </p>
            </div>
            <div className="min-w-0">
              <span className="text-muted-foreground mb-1 block text-xs">{t("user.email")}</span>
              <p className="font-medium">
                <TruncateText>{user.spec.email}</TruncateText>
              </p>
            </div>
            <div className="min-w-0">
              <span className="text-muted-foreground mb-1 block text-xs">{t("common.phone")}</span>
              <p className="font-medium">
                <TruncateText>{user.spec.phone || "-"}</TruncateText>
              </p>
            </div>
            <div>
              <span className="text-muted-foreground mb-1 block text-xs">{t("common.status")}</span>
              <p>
                <Badge variant={user.spec.status === "active" ? "default" : "secondary"}>
                  {user.spec.status === "active" ? t("common.active") : t("common.inactive")}
                </Badge>
              </p>
            </div>
            <div>
              <span className="text-muted-foreground mb-1 block text-xs">
                {t("common.created")}
              </span>
              <p className="font-medium">{new Date(user.metadata.createdAt).toLocaleString()}</p>
            </div>
            <div>
              <span className="text-muted-foreground mb-1 block text-xs">
                {t("common.updated")}
              </span>
              <p className="font-medium">{new Date(user.metadata.updatedAt).toLocaleString()}</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* role bindings card */}
      <ScopedRoleBindingsCard
        userId={userId!}
        workspaceId={workspaceId!}
        namespaceId={namespaceId}
      />
    </div>
  )
}

// ===== Scoped Role Bindings Card =====

function ScopedRoleBindingsCard({
  userId,
  workspaceId,
  namespaceId,
}: {
  userId: string
  workspaceId: string
  namespaceId?: string
}) {
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
  const bindingsQuery = useApiQuery({
    queryKey: [
      "iam",
      "user-rolebindings",
      workspaceId ?? "",
      namespaceId ?? "",
      userId,
      sortBy,
      sortOrder,
    ],
    queryFn: () => {
      // Bug #134: hit the workspace/namespace-scoped verb so the permission
      // check lands at scope-level iam:users:get (which a project / workspace
      // member has) instead of platform-level.
      const params: ListParams = { page: 1, pageSize: 100, sortBy, sortOrder }
      return namespaceId
        ? listNamespaceUserRoleBindings(workspaceId, namespaceId, userId, params)
        : workspaceId
          ? listWorkspaceUserRoleBindings(workspaceId, userId, params)
          : listUserRoleBindings(userId, { ...params })
    },
    placeholderData: keepPreviousData,
  })
  const allBindings = useMemo(() => bindingsQuery.data?.items ?? [], [bindingsQuery.data])
  const loading = bindingsQuery.isPending

  useEffect(() => {
    setPage(1)
  }, [search, pageSize, setPage])

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
              name="scoped-user-rolebindings-search"
              placeholder={t("common.search")}
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className="pl-9"
            />
          </div>
        </div>
        <div className="overflow-hidden rounded-xl border shadow-sm">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead
                  className="hover:text-foreground cursor-pointer transition-colors select-none"
                  onClick={() => handleSort("role_name")}
                >
                  {t("role.title")}
                  <SortIcon field="role_name" sortBy={sortBy} sortOrder={sortOrder} />
                </TableHead>
                <TableHead>{t("rolebinding.scope")}</TableHead>
                <TableHead
                  className="hover:text-foreground cursor-pointer transition-colors select-none"
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
                    {Array.from({ length: 3 }).map((_, j) => (
                      <TableCell key={j}>
                        <Skeleton className="h-4 w-16" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : bindings.length === 0 ? (
                <TableRow className="hover:bg-transparent">
                  <TableCell colSpan={3} className="p-0 whitespace-normal">
                    <EmptyState title={t("user.noRolebindings")} />
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
                    <TableCell className="text-muted-foreground text-sm whitespace-nowrap">
                      {new Date(b.metadata.createdAt).toLocaleString()}
                    </TableCell>
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
