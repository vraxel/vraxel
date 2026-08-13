import { useEffect, useState } from "react"
import { formatDateTime } from "@/shared/lib/format"
import { Navigate, useParams } from "react-router"
import { Plus, Pencil, Trash2, KeyRound } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Checkbox } from "@/shared/ui/checkbox"
import { Input } from "@/shared/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/shared/ui/form"
import { listUsers, createUser, updateUser, usersApi } from "@/modules/iam/api/users"
import { usersDef } from "@/modules/iam/defs"
import { useQueryClient } from "@tanstack/react-query"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { NameCell } from "@/frameworks/list/name-cell"
import { StatusFilter } from "@/frameworks/list/status-filter"
import { ActiveStatusBadge } from "@/shared/components/active-status-badge"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { useApiMutation } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { ListParams } from "@/core/api/types"
import type { User } from "@/modules/iam/api/types"
import type { ScopeRef } from "@/core/registry/resource"
import { useTranslation } from "@/i18n"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { usePermission } from "@/core/permission/use-permission"
import { usePermissionStore } from "@/core/permission/permission-store"
import { buildPermScope } from "@/core/registry/nav-config"
import { FormDialog } from "@/frameworks/form/form-dialog"
import { ResetPasswordDialog } from "@/modules/iam/components/reset-password-dialog"
import { useAuthStore } from "@/core/auth/auth-store"

export default function UserListPage() {
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const permissionsLoaded = usePermissionStore((s) => s.permissions) !== null
  const { workspaceId: scopeWorkspaceId, namespaceId: scopeNamespaceId } = useParams()
  const scope: ScopeRef = { ws: scopeWorkspaceId, ns: scopeNamespaceId }
  const permScope = buildPermScope(scopeWorkspaceId, scopeNamespaceId)
  const canCreate = hasPermission("iam:users:create", permScope)
  const canUpdate = hasPermission("iam:users:update", permScope)
  const canReset = hasPermission("iam:users:reset-password", permScope)
  const canDelete = hasPermission("iam:users:delete", permScope)
  const canBatch = hasPermission("iam:users:deleteCollection", permScope)

  const currentUserSub = useAuthStore((s) => s.user?.sub)

  // dialogs
  const [createOpen, setCreateOpen] = useState(false)
  const [editUser, setEditUser] = useState<User | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null)
  const [resetTarget, setResetTarget] = useState<User | null>(null)
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false)

  const query = useListQuery<User>({
    def: usersDef,
    api: usersApi,
    scope,
    filterKeys: ["status"],
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(usersDef) })

  const deleteMutation = useApiMutation({
    mutationFn: (id: string) => usersApi.delete(scope, id),
    invalidates: [qk.resource(usersDef)],
    onSuccess: () => {
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
    },
    onError: (err) => showApiError(err, t, "user.title"),
  })
  const batchDeleteMutation = useApiMutation({
    mutationFn: (ids: string[]) => usersApi.deleteCollection(scope, ids),
    invalidates: [qk.resource(usersDef)],
    onSuccess: () => {
      toast.success(t("action.deleteSuccess"))
      setBatchDeleteOpen(false)
      query.clearSelection()
    },
    onError: (err) => showApiError(err, t, "user.title"),
  })

  // Builtin users (admin) are not selectable; exclude them from the
  // select-all toggle and indeterminate-state calculation so the header
  // checkbox can reach a fully-checked state.
  const selectableRows = query.rows.filter((u) => !u.spec.builtin)
  const selectColumn: ColumnDef<User> = {
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
        onCheckedChange={() => query.toggleAll(selectableRows.map((u) => u.metadata.id))}
        disabled={selectableRows.length === 0}
      />
    ),
    cell: (user) =>
      !user.spec.builtin && (
        <Checkbox
          checked={query.selected.has(user.metadata.id)}
          onCheckedChange={() => query.toggleOne(user.metadata.id)}
        />
      ),
  }

  const columns: ColumnDef<User>[] = [
    ...(canBatch ? [selectColumn] : []),
    {
      key: "username",
      header: t("user.username"),
      sortable: true,
      cell: (user) => (
        <NameCell
          to={`/iam/users/${user.metadata.id}`}
          displayName={user.spec.displayName}
          name={user.spec.username}
          trailing={<ActiveStatusBadge status={user.spec.status} />}
        />
      ),
    },
    {
      key: "email",
      header: t("user.email"),
      sortable: true,
      truncate: true,
      cell: (user) => user.spec.email,
    },
    {
      key: "phone",
      header: t("common.phone"),
      sortable: true,
      truncate: true,
      cell: (user) => user.spec.phone || "-",
    },
    {
      key: "created_at",
      header: t("common.created"),
      sortable: true,
      className: "text-muted-foreground text-sm whitespace-nowrap",
      cell: (user) => formatDateTime(user.metadata.createdAt),
    },
    {
      key: "updated_at",
      header: t("common.updated"),
      sortable: true,
      className: "text-muted-foreground text-sm whitespace-nowrap",
      cell: (user) => formatDateTime(user.metadata.updatedAt),
    },
  ]

  if (permissionsLoaded && !hasPermission("iam:users:list", permScope)) {
    return <Navigate to="/" replace />
  }

  return (
    <ResourceListPage
      query={query}
      columns={columns}
      titleKey="user.title"
      subtitle={t("user.manage", { count: query.totalCount })}
      searchPlaceholderKey="user.searchPlaceholder"
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
      selectable={false}
      emptyKey="user.noData"
      createButton={
        canCreate && (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("user.create")}
          </Button>
        )
      }
      batchActions={
        canBatch && (
          <Button variant="destructive" size="sm" onClick={() => setBatchDeleteOpen(true)}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("user.batchDelete")} ({query.selected.size})
          </Button>
        )
      }
      rowActions={
        canUpdate || canReset || canDelete
          ? (user) => (
              <>
                {canUpdate && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    onClick={() => setEditUser(user)}
                    title={t("common.edit")}
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                )}
                {canReset && user.metadata.id !== currentUserSub && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8"
                    onClick={() => setResetTarget(user)}
                    title={t("user.resetPassword")}
                  >
                    <KeyRound className="h-3.5 w-3.5" />
                  </Button>
                )}
                {canDelete && !user.spec.builtin && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-destructive hover:text-destructive h-8 w-8"
                    onClick={() => setDeleteTarget(user)}
                    title={t("common.delete")}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                )}
              </>
            )
          : undefined
      }
    >
      {/* create dialog */}
      <UserFormDialog open={createOpen} onOpenChange={setCreateOpen} onSuccess={invalidate} />

      {/* edit dialog */}
      <UserFormDialog
        open={!!editUser}
        onOpenChange={(v) => {
          if (!v) setEditUser(null)
        }}
        user={editUser ?? undefined}
        onSuccess={invalidate}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        description={t("user.deleteConfirm", { name: deleteTarget?.spec.username ?? "" })}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.metadata.id)
        }}
        confirmText={t("common.delete")}
      />

      <ConfirmDialog
        open={batchDeleteOpen}
        onOpenChange={setBatchDeleteOpen}
        title={t("user.batchDelete")}
        description={t("user.batchDeleteConfirm", { count: query.selected.size })}
        onConfirm={() => batchDeleteMutation.mutate(Array.from(query.selected))}
        confirmText={t("common.delete")}
      />

      {resetTarget && (
        <ResetPasswordDialog
          open={!!resetTarget}
          onOpenChange={(v) => {
            if (!v) setResetTarget(null)
          }}
          userId={resetTarget.metadata.id}
          username={resetTarget.spec.username}
        />
      )}
    </ResourceListPage>
  )
}

// --- User Create/Edit Form Dialog ---

interface UserFormValues {
  username: string
  email: string
  displayName?: string
  phone: string
  password?: string
  status: "active" | "inactive"
}

function UserFormDialog({
  open,
  onOpenChange,
  user,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  user?: User
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const isEdit = !!user
  const [loading, setLoading] = useState(false)

  const userFormSchema = z.object({
    // Edit 模式下 username 被 disabled 且 update 路径不改写它，强约束在
    // 历史违例数据上会卡死保存按钮（#159 同款）。
    username: isEdit
      ? z.string()
      : z
          .string()
          .min(3, t("api.validation.username.format"))
          .max(50, t("api.validation.username.format"))
          .regex(/^[a-zA-Z0-9_-]+$/, t("api.validation.username.format")),
    email: z
      .email(t("api.validation.email.format"))
      .max(255, t("api.validation.maxLength", { max: 255 })),
    displayName: z
      .string()
      .max(128, t("api.validation.maxLength", { max: 128 }))
      .optional(),
    phone: z
      .string()
      .min(1, t("api.validation.required", { field: t("common.phone") }))
      .max(50, t("api.validation.maxLength", { max: 50 }))
      .regex(/^1[3-9]\d{9}$/, t("api.validation.phone.format")),
    password: isEdit
      ? z.string().optional()
      : z
          .string()
          .min(8, t("api.validation.password.length"))
          .max(72, t("api.validation.password.length"))
          .regex(/[A-Z]/, t("api.validation.password.uppercase"))
          .regex(/[a-z]/, t("api.validation.password.lowercase"))
          .regex(/[0-9]/, t("api.validation.password.digit")),
    status: z.enum(["active", "inactive"]),
  })

  const form = useForm<UserFormValues>({
    resolver: zodResolver(userFormSchema) as never,
    mode: "onBlur",
    defaultValues: {
      username: "",
      email: "",
      displayName: "",
      phone: "",
      password: "",
      status: "active",
    },
  })

  const checkUniqueness = async (field: "username" | "email" | "phone", value: string) => {
    if (!value) return
    try {
      const params: ListParams = { page: 1, pageSize: 1, [field]: value }
      const data = await listUsers(params)
      const exists = data.items?.some((u) => {
        if (isEdit && u.metadata.id === user?.metadata.id) return false
        return u.spec[field]?.toLowerCase() === value.toLowerCase()
      })
      if (exists) {
        form.setError(field, { message: t(`api.validation.${field}.taken`) })
      }
    } catch {
      // uniqueness will be enforced on submit by backend
    }
  }

  // reset form when dialog opens with user data
  useEffect(() => {
    if (open) {
      if (user) {
        form.reset({
          username: user.spec.username,
          email: user.spec.email,
          displayName: user.spec.displayName ?? "",
          phone: user.spec.phone ?? "",
          password: "",
          status: user.spec.status ?? "active",
        })
      } else {
        form.reset({
          username: "",
          email: "",
          displayName: "",
          phone: "",
          password: "",
          status: "active",
        })
      }
    }
  }, [open, user, form])

  const onSubmit = async (values: UserFormValues) => {
    setLoading(true)
    try {
      const spec = {
        username: values.username,
        email: values.email,
        displayName: values.displayName || undefined,
        phone: values.phone,
        status: values.status,
      } as User["spec"]

      if (isEdit) {
        await updateUser(user.metadata.id, {
          metadata: user.metadata,
          spec,
        })
        toast.success(t("action.updateSuccess"))
      } else {
        // include password for creation
        const createSpec: Record<string, unknown> = { ...spec }
        if (values.password) createSpec.password = values.password
        await createUser({
          metadata: {} as User["metadata"],
          spec: createSpec as unknown as User["spec"],
        })
        toast.success(t("action.createSuccess"))
      }
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
      title={isEdit ? t("user.edit") : t("user.create")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:max-w-lg"
    >
      <FormField
        control={form.control}
        name="username"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t("user.username")}</FormLabel>
            <FormControl>
              <Input
                {...field}
                disabled={isEdit}
                onBlur={async (e) => {
                  field.onBlur()
                  if (isEdit || !e.target.value) return
                  const valid = await form.trigger("username")
                  if (valid) checkUniqueness("username", e.target.value)
                }}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="email"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t("user.email")}</FormLabel>
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
            <FormLabel required>{t("common.phone")}</FormLabel>
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
      {!isEdit && (
        <FormField
          control={form.control}
          name="password"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t("common.password")}</FormLabel>
              <FormControl>
                <Input type="password" {...field} />
              </FormControl>
              <FormDescription>{t("api.validation.password.hint")}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
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
