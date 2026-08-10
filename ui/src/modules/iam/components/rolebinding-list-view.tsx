import { useMemo, useState } from "react"
import { Plus, Trash2, Search } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Label } from "@/shared/ui/label"
import { Checkbox } from "@/shared/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog"
import type { ListParams } from "@/core/api/types"
import type { RoleBinding, RoleBindingList, RoleList, UserList } from "@/modules/iam/api/types"
import type { ScopeRef } from "@/core/registry/resource"
import { rolebindingsDef } from "@/modules/iam/defs"
import { useQueryClient } from "@tanstack/react-query"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { useApiMutation, useApiQuery } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { showApiError } from "@/core/api/client"
import { TruncateText } from "@/shared/components/truncate-text"
import { useTranslation } from "@/i18n"
import { usePermission } from "@/core/permission/use-permission"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"

/** API functions the shared view needs, parameterized by scope. */
export interface RoleBindingListConfig {
  listBindings: (params: ListParams) => Promise<RoleBindingList>
  createBinding: (data: Pick<RoleBinding, "spec">) => Promise<RoleBinding>
  deleteBinding: (id: string) => Promise<void>
  deleteBindings: (ids: string[]) => Promise<void>
  listRoles: (params?: ListParams) => Promise<RoleList>
  /**
   * Lists candidate users for a new binding. Each scope must inject an
   * implementation that runs under that scope's permissions:
   *   platform   -> listUsers (needs iam:users:list:platform)
   *   workspace  -> listWorkspaceUsers(wsId, { available: "true", ... })
   *   namespace  -> listNamespaceUsers(wsId, nsId, { available: "true", ... })
   * Without scope-specific routing, workspace/namespace admins lacking
   * platform user-list permission see an empty user dropdown.
   */
  listUsers: (params?: ListParams) => Promise<UserList>
  /** Permission codes for this scope */
  permCreate: string
  permDelete: string
  /** Scope value written into the create request */
  scope: "platform" | "workspace" | "namespace"
  /** Scope params for permission checks (e.g. { workspaceId }) */
  scopeParams?: Record<string, string>
}

export function RoleBindingListView({ config }: { config: RoleBindingListConfig }) {
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()

  const [createOpen, setCreateOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<RoleBinding | null>(null)
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false)

  const canCreate = hasPermission(config.permCreate, config.scopeParams)
  const canDelete = hasPermission(config.permDelete, config.scopeParams)

  // Query-key scope from scopeParams so each workspace/namespace tab
  // caches its own rows (the injected listBindings closure already
  // carries the scope in its URL).
  const scope: ScopeRef = {
    ws: config.scopeParams?.workspaceId,
    ns: config.scopeParams?.namespaceId,
  }
  const query = useListQuery<RoleBinding>({
    def: rolebindingsDef,
    api: { list: (_s: ScopeRef, params?: ListParams) => config.listBindings(params ?? {}) },
    scope,
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(rolebindingsDef) })

  const deleteMutation = useApiMutation({
    mutationFn: (id: string) => config.deleteBinding(id),
    invalidates: [qk.resource(rolebindingsDef)],
    onSuccess: () => {
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
    },
    onError: (err) => showApiError(err, t, "rolebinding.title"),
  })
  const batchDeleteMutation = useApiMutation({
    mutationFn: (ids: string[]) => config.deleteBindings(ids),
    invalidates: [qk.resource(rolebindingsDef)],
    onSuccess: () => {
      toast.success(t("action.deleteSuccess"))
      setBatchDeleteOpen(false)
      query.clearSelection()
    },
    onError: (err) => showApiError(err, t, "rolebinding.title"),
  })

  // Owner bindings cannot be removed; exclude them from select-all.
  const selectableRows = query.rows.filter((b) => !b.spec.isOwner)
  const selectColumn: ColumnDef<RoleBinding> = {
    key: "_select",
    headClassName: "w-10",
    header: (
      <Checkbox
        checked={
          query.selected.size === 0 || selectableRows.length === 0
            ? false
            : query.selected.size === selectableRows.length
              ? true
              : "indeterminate"
        }
        onCheckedChange={() => query.toggleAll(selectableRows.map((b) => b.metadata.id))}
      />
    ),
    cell: (binding) => (
      <Checkbox
        checked={query.selected.has(binding.metadata.id)}
        onCheckedChange={() => query.toggleOne(binding.metadata.id)}
        disabled={!!binding.spec.isOwner}
      />
    ),
  }

  const columns: ColumnDef<RoleBinding>[] = [
    ...(canDelete ? [selectColumn] : []),
    {
      key: "username",
      header: t("user.username"),
      sortable: true,
      truncate: true,
      className: "font-medium",
      cell: (binding) => binding.spec.username,
    },
    {
      key: "user_display_name",
      header: t("common.displayName"),
      sortable: true,
      truncate: true,
      cell: (binding) => binding.spec.userDisplayName || "-",
    },
    {
      key: "role_name",
      header: t("rolebinding.role"),
      sortable: true,
      className: "max-w-[150px]",
      cell: (binding) => (
        <Badge variant="secondary" className="max-w-full">
          <TruncateText>
            {t(`role.${binding.spec.roleName}`, {
              defaultValue: binding.spec.roleDisplayName || binding.spec.roleName || "",
            })}
          </TruncateText>
        </Badge>
      ),
    },
    {
      key: "role_display_name",
      header: t("rolebinding.roleDisplayName"),
      sortable: true,
      truncate: true,
      cell: (binding) => binding.spec.roleDisplayName || "-",
    },
    {
      key: "created_at",
      header: t("common.created"),
      sortable: true,
      className: "text-muted-foreground text-sm whitespace-nowrap",
      cell: (binding) => new Date(binding.metadata.createdAt).toLocaleString(),
    },
  ]

  return (
    <ResourceListPage
      query={query}
      columns={columns}
      titleKey="rolebinding.title"
      subtitle={t("rolebinding.manage", { count: query.totalCount })}
      searchPlaceholderKey="rolebinding.searchPlaceholder"
      selectable={false}
      emptyKey="rolebinding.noData"
      createButton={
        canCreate && (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("rolebinding.create")}
          </Button>
        )
      }
      batchActions={
        canDelete && (
          <Button variant="destructive" size="sm" onClick={() => setBatchDeleteOpen(true)}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("rolebinding.batchDelete")} ({query.selected.size})
          </Button>
        )
      }
      rowActions={(binding) => (
        <>
          {canDelete && (
            <Button
              variant="ghost"
              size="icon"
              className="text-destructive hover:text-destructive h-8 w-8"
              onClick={() => setDeleteTarget(binding)}
              disabled={!!binding.spec.isOwner}
              title={t("common.delete")}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          )}
        </>
      )}
    >
      <CreateRoleBindingDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onSuccess={invalidate}
        config={config}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        description={t("rolebinding.deleteConfirm", { name: deleteTarget?.spec.username ?? "" })}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.metadata.id)
        }}
        confirmText={t("common.delete")}
      />

      <ConfirmDialog
        open={batchDeleteOpen}
        onOpenChange={setBatchDeleteOpen}
        title={t("rolebinding.batchDelete")}
        description={t("rolebinding.batchDeleteConfirm", { count: query.selected.size })}
        onConfirm={() => batchDeleteMutation.mutate(Array.from(query.selected))}
        confirmText={t("common.delete")}
      />
    </ResourceListPage>
  )
}

function CreateRoleBindingDialog({
  open,
  onOpenChange,
  onSuccess,
  config,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
  config: RoleBindingListConfig
}) {
  const { t } = useTranslation()
  const [selectedUserId, setSelectedUserId] = useState("")
  const [selectedRoleId, setSelectedRoleId] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [searchQuery, setSearchQuery] = useState("")

  const dataQuery = useApiQuery({
    queryKey: [
      "iam",
      "rolebinding-candidates",
      config.scope,
      config.scopeParams?.workspaceId ?? "",
      config.scopeParams?.namespaceId ?? "",
    ],
    queryFn: async () => {
      const [userData, roleData] = await Promise.all([
        config.listUsers({ pageSize: 100 }),
        config.listRoles({ pageSize: 100 }),
      ])
      return { users: userData.items ?? [], roles: roleData.items ?? [] }
    },
    enabled: open,
  })
  const users = useMemo(() => dataQuery.data?.users ?? [], [dataQuery.data])
  const roles = dataQuery.data?.roles ?? []
  const loading = open && dataQuery.isPending

  // Reset the picker whenever the dialog opens (adjust-during-render).
  const [prevOpen, setPrevOpen] = useState(open)
  if (prevOpen !== open) {
    setPrevOpen(open)
    if (open) {
      setSelectedUserId("")
      setSelectedRoleId("")
      setSearchQuery("")
    }
  }

  const filteredUsers = searchQuery
    ? users.filter((u) => {
        const q = searchQuery.toLowerCase()
        return (
          u.spec.username.toLowerCase().includes(q) ||
          u.spec.email?.toLowerCase().includes(q) ||
          u.spec.displayName?.toLowerCase().includes(q) ||
          u.spec.phone?.includes(q)
        )
      })
    : users

  const handleSubmit = async () => {
    if (!selectedUserId || !selectedRoleId) return
    setSubmitting(true)
    try {
      await config.createBinding({
        spec: { userId: selectedUserId, roleId: selectedRoleId, scope: config.scope },
      })
      toast.success(t("action.createSuccess"))
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      showApiError(err, t, "rolebinding.title")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[85vh] max-w-lg flex-col"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{t("rolebinding.create")}</DialogTitle>
          <DialogDescription>{t("rolebinding.createDesc")}</DialogDescription>
        </DialogHeader>
        <div className="-mx-1 min-h-0 flex-1 space-y-4 overflow-y-auto px-1">
          <div>
            <Label className="text-sm font-medium">{t("rolebinding.selectRole")}</Label>
            <div className="mt-1 max-h-[150px] overflow-auto rounded-md border">
              {loading ? (
                <div className="space-y-2 p-4">
                  {Array.from({ length: 3 }).map((_, i) => (
                    <Skeleton key={i} className="h-8 w-full" />
                  ))}
                </div>
              ) : roles.length === 0 ? (
                <p className="text-muted-foreground p-4 text-center text-sm">
                  {t("rolebinding.noRoles")}
                </p>
              ) : (
                roles.map((role) => (
                  <label
                    key={role.metadata.id}
                    className={`hover:bg-muted/50 flex cursor-pointer items-center gap-3 px-4 py-2 ${selectedRoleId === role.metadata.id ? "bg-muted" : ""}`}
                  >
                    <Checkbox
                      checked={selectedRoleId === role.metadata.id}
                      onCheckedChange={() =>
                        setSelectedRoleId(
                          selectedRoleId === role.metadata.id ? "" : role.metadata.id,
                        )
                      }
                    />
                    <div className="min-w-0 flex-1">
                      <TruncateText className="text-sm font-medium">
                        {t(`role.${role.spec.name}`, {
                          defaultValue: role.spec.displayName || role.spec.name,
                        })}
                      </TruncateText>
                      <TruncateText className="text-muted-foreground text-xs">
                        {t(`role.desc.${role.spec.name}`, {
                          defaultValue: role.spec.description || "",
                        }) || "-"}
                      </TruncateText>
                    </div>
                  </label>
                ))
              )}
            </div>
          </div>
          <div>
            <Label className="text-sm font-medium">{t("rolebinding.selectUser")}</Label>
            <div className="relative mt-1">
              <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
              <Input
                name="rolebinding-user-search"
                placeholder={t("rolebinding.searchPlaceholder")}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9"
              />
            </div>
            <div className="mt-1 max-h-[200px] overflow-auto rounded-md border">
              {loading ? (
                <div className="space-y-2 p-4">
                  {Array.from({ length: 3 }).map((_, i) => (
                    <Skeleton key={i} className="h-8 w-full" />
                  ))}
                </div>
              ) : filteredUsers.length === 0 ? (
                <p className="text-muted-foreground p-4 text-center text-sm">
                  {searchQuery ? t("common.noSearchResults") : t("rolebinding.noUsers")}
                </p>
              ) : (
                filteredUsers.map((user) => (
                  <label
                    key={user.metadata.id}
                    className={`hover:bg-muted/50 flex cursor-pointer items-center gap-3 px-4 py-2 ${selectedUserId === user.metadata.id ? "bg-muted" : ""}`}
                  >
                    <Checkbox
                      checked={selectedUserId === user.metadata.id}
                      onCheckedChange={() =>
                        setSelectedUserId(
                          selectedUserId === user.metadata.id ? "" : user.metadata.id,
                        )
                      }
                    />
                    <div className="min-w-0 flex-1">
                      <TruncateText className="text-sm font-medium">
                        {user.spec.username}
                      </TruncateText>
                      <TruncateText className="text-muted-foreground text-xs">
                        {user.spec.displayName || user.spec.email}
                      </TruncateText>
                    </div>
                  </label>
                ))
              )}
            </div>
          </div>
        </div>
        <DialogFooter className="mt-6 border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!selectedUserId || !selectedRoleId || submitting}
          >
            {submitting ? "..." : t("rolebinding.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
