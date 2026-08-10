import { useCallback, useEffect, useState } from "react"
import { Link, Navigate, useSearchParams } from "react-router"
import { Plus, Pencil, Trash2 } from "lucide-react"
import { useForm, useWatch } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Checkbox } from "@/shared/ui/checkbox"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/shared/ui/form"
import {
  listRoles,
  createRole,
  updateRole,
  getRole,
  listAllPermissions,
  rolesApi,
} from "@/modules/iam/api/rbac"
import { rolesDef } from "@/modules/iam/defs"
import { useQueryClient } from "@tanstack/react-query"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import { useApiMutation } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { Permission, Role } from "@/modules/iam/api/types"
import { useTranslation, findBuiltinRoleNamesMatching } from "@/i18n"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { PermissionSelector } from "@/modules/iam/components/permission-selector"
import { FormDialog } from "@/frameworks/form/form-dialog"
import { usePermission } from "@/core/permission/use-permission"
import { usePermissionStore } from "@/core/permission/permission-store"

const SCOPE_VARIANT: Record<string, "default" | "secondary" | "outline"> = {
  platform: "default",
  workspace: "secondary",
  namespace: "outline",
}

export default function RoleListPage() {
  const { t, locale } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const permissionsLoaded = usePermissionStore((s) => s.permissions) !== null
  const canCreate = hasPermission("iam:roles:create")
  const canUpdate = hasPermission("iam:roles:update")
  const canDelete = hasPermission("iam:roles:delete")
  const [permissions, setPermissions] = useState<Permission[]>([])

  // dialogs
  const [createOpen, setCreateOpen] = useState(false)
  const [editRole, setEditRole] = useState<Role | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Role | null>(null)
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false)

  // Builtin roles are stored under stable English names; extra_names lets
  // the backend also match rows whose translated display label matches the
  // search text. Derived from the same URL param useListQuery reads.
  const [sp] = useSearchParams()
  const search = sp.get("q") ?? ""
  const extraNames = search ? findBuiltinRoleNamesMatching(search, locale) : []

  const query = useListQuery<Role>({
    def: rolesDef,
    api: rolesApi,
    scope: {},
    filterKeys: ["scope", "builtin"],
    extraParams: extraNames.length > 0 ? { extra_names: extraNames.join(",") } : undefined,
  })
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(rolesDef) })

  const deleteMutation = useApiMutation({
    mutationFn: (id: string) => rolesApi.delete({}, id),
    invalidates: [qk.resource(rolesDef)],
    onSuccess: () => {
      toast.success(t("action.deleteSuccess"))
      setDeleteTarget(null)
    },
    onError: (err) => showApiError(err, t, "role.title"),
  })
  const batchDeleteMutation = useApiMutation({
    mutationFn: (ids: string[]) => rolesApi.deleteCollection({}, ids),
    invalidates: [qk.resource(rolesDef)],
    onSuccess: () => {
      toast.success(t("action.deleteSuccess"))
      setBatchDeleteOpen(false)
      query.clearSelection()
    },
    onError: (err) => showApiError(err, t, "role.title"),
  })

  const loadPermissions = useCallback(async () => {
    if (permissions.length > 0) return
    try {
      const items = await listAllPermissions()
      setPermissions(items)
    } catch {
      // silently ignore — permission selector will show empty
    }
  }, [permissions.length])

  const handleCreate = async () => {
    await loadPermissions()
    setCreateOpen(true)
  }

  const handleEdit = async (role: Role) => {
    await loadPermissions()
    setEditRole(role)
  }

  // only non-builtin roles can be selected for batch delete
  const selectableRows = query.rows.filter((r) => !r.spec.builtin)
  const selectColumn: ColumnDef<Role> = {
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
        onCheckedChange={() => query.toggleAll(selectableRows.map((r) => r.metadata.id))}
      />
    ),
    cell: (role) => (
      <Checkbox
        checked={query.selected.has(role.metadata.id)}
        onCheckedChange={() => query.toggleOne(role.metadata.id)}
        disabled={!!role.spec.builtin}
      />
    ),
  }

  const columns: ColumnDef<Role>[] = [
    ...(canDelete ? [selectColumn] : []),
    {
      key: "name",
      header: t("role.name"),
      sortable: true,
      truncate: true,
      cell: (role) => (
        <Link to={`/iam/roles/${role.metadata.id}`} className="font-medium hover:underline">
          {role.spec.name}
        </Link>
      ),
    },
    {
      key: "display_name",
      header: t("common.displayName"),
      sortable: true,
      truncate: true,
      cell: (role) => t(`role.${role.spec.name}`, { defaultValue: role.spec.displayName || "-" }),
    },
    {
      key: "scope",
      header: t("role.scope"),
      filter: [
        { value: "all", label: t("common.all") },
        { value: "platform", label: t("role.scope.platform") },
        { value: "workspace", label: t("role.scope.workspace") },
        { value: "namespace", label: t("role.scope.namespace") },
      ],
      cell: (role) => (
        <Badge variant={SCOPE_VARIANT[role.spec.scope] ?? "outline"}>
          {t(`role.scope.${role.spec.scope}`)}
        </Badge>
      ),
    },
    {
      key: "builtin",
      header: t("role.builtin"),
      filter: [
        { value: "all", label: t("common.all") },
        { value: "true", label: t("role.builtin") },
        { value: "false", label: t("role.custom") },
      ],
      cell: (role) => (
        <Badge variant={role.spec.builtin ? "secondary" : "outline"}>
          {role.spec.builtin ? t("role.builtin") : t("role.custom")}
        </Badge>
      ),
    },
    {
      key: "description",
      header: t("common.description"),
      truncate: true,
      className: "text-sm",
      cell: (role) =>
        t(`role.desc.${role.spec.name}`, { defaultValue: role.spec.description || "-" }),
    },
    {
      key: "rules",
      header: t("role.rules"),
      className: "text-muted-foreground text-sm",
      cell: (role) =>
        t("role.rulesCount", { count: role.spec.ruleCount ?? role.spec.rules?.length ?? 0 }),
    },
    {
      key: "created_at",
      header: t("common.created"),
      sortable: true,
      className: "text-muted-foreground text-sm whitespace-nowrap",
      cell: (role) => new Date(role.metadata.createdAt).toLocaleString(),
    },
  ]

  if (permissionsLoaded && !hasPermission("iam:roles:list")) {
    return <Navigate to="/" replace />
  }

  return (
    <ResourceListPage
      query={query}
      columns={columns}
      titleKey="role.title"
      subtitle={t("role.manage", { count: query.totalCount })}
      searchPlaceholderKey="role.searchPlaceholder"
      selectable={false}
      emptyKey="role.noData"
      createButton={
        canCreate && (
          <Button onClick={handleCreate}>
            <Plus className="mr-2 h-4 w-4" />
            {t("role.create")}
          </Button>
        )
      }
      batchActions={
        canDelete && (
          <Button variant="destructive" size="sm" onClick={() => setBatchDeleteOpen(true)}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("role.batchDelete")} ({query.selected.size})
          </Button>
        )
      }
      rowActions={(role) => (
        <>
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
              title={role.spec.builtin ? t("role.builtinCannotDelete") : t("common.delete")}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          )}
        </>
      )}
    >
      {/* create dialog */}
      <RoleFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        permissions={permissions}
        onSuccess={invalidate}
      />

      {/* edit dialog */}
      <RoleFormDialog
        open={!!editRole}
        onOpenChange={(v) => {
          if (!v) setEditRole(null)
        }}
        role={editRole ?? undefined}
        permissions={permissions}
        onSuccess={invalidate}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onOpenChange={(v) => {
          if (!v) setDeleteTarget(null)
        }}
        title={t("common.delete")}
        description={t("role.deleteConfirm", { name: deleteTarget?.spec.name ?? "" })}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.metadata.id)
        }}
        confirmText={t("common.delete")}
      />

      <ConfirmDialog
        open={batchDeleteOpen}
        onOpenChange={setBatchDeleteOpen}
        title={t("role.batchDelete")}
        description={t("role.batchDeleteConfirm", { count: query.selected.size })}
        onConfirm={() => batchDeleteMutation.mutate(Array.from(query.selected))}
        confirmText={t("common.delete")}
      />
    </ResourceListPage>
  )
}

// --- Role Create/Edit Form Dialog ---

interface RoleFormValues {
  name: string
  displayName: string
  description: string
  scope: "platform" | "workspace" | "namespace"
  rules: string[]
}

function RoleFormDialog({
  open,
  onOpenChange,
  role,
  permissions,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  role?: Role
  permissions: Permission[]
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const isEdit = !!role
  const [loading, setLoading] = useState(false)
  const [rawFullRole, setFullRole] = useState<Role | null>(null)
  // Only trust the fetched detail when it belongs to the role being
  // edited; otherwise (create mode, or an edit whose fetch hasn't landed
  // yet) fall back to the list row. This replaces the effect's synchronous
  // setFullRole(null) reset and closes the stale-detail window when
  // switching straight from editing role A to role B.
  const fullRole =
    role && rawFullRole && rawFullRole.metadata.id === role.metadata.id ? rawFullRole : null

  const roleFormSchema = z.object({
    name: isEdit
      ? z.string()
      : z
          .string()
          .min(3, t("role.validation.name.format"))
          .max(50, t("role.validation.name.format"))
          .regex(/^[a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9]$/, t("role.validation.name.format")),
    displayName: z
      .string()
      .max(128, t("api.validation.maxLength", { max: 128 }))
      .optional(),
    description: z
      .string()
      .max(1000, t("api.validation.maxLength", { max: 1000 }))
      .optional(),
    scope: z.enum(["platform", "workspace", "namespace"]),
    rules: z.array(z.string()).min(1, t("role.validation.rules.required")),
  })

  const form = useForm<RoleFormValues>({
    resolver: zodResolver(roleFormSchema) as never,
    mode: "onBlur",
    defaultValues: {
      name: "",
      displayName: "",
      description: "",
      scope: "platform",
      rules: [],
    },
  })

  // Fetch full role data (with rules) when editing — list API only returns ruleCount
  useEffect(() => {
    if (open && role) {
      const fetchFull = async () => {
        try {
          const r = await getRole(role.metadata.id)
          setFullRole(r)
          form.reset({
            name: r.spec.name,
            displayName: r.spec.displayName ?? "",
            description: r.spec.description ?? "",
            scope: r.spec.scope,
            rules: r.spec.rules ?? [],
          })
        } catch {
          /* fall back to list data */
        }
      }
      fetchFull()
    } else if (open) {
      form.reset({
        name: "",
        displayName: "",
        description: "",
        scope: "platform",
        rules: [],
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, role])

  const checkUniqueness = async (value: string) => {
    if (!value) return
    try {
      const data = await listRoles({ page: 1, pageSize: 1, search: value })
      const exists = data.items?.some((r) => {
        if (isEdit && r.metadata.id === role?.metadata.id) return false
        return r.spec.name === value
      })
      if (exists) form.setError("name", { message: t("role.validation.name.taken") })
    } catch {
      /* backend will enforce */
    }
  }

  const onSubmit = async (values: RoleFormValues) => {
    setLoading(true)
    try {
      if (isEdit) {
        const editRole = fullRole ?? role
        await updateRole(editRole.metadata.id, {
          metadata: editRole.metadata,
          spec: {
            ...editRole.spec,
            displayName: values.displayName || undefined,
            description: values.description || undefined,
            rules: values.rules,
          },
        })
        toast.success(t("action.updateSuccess"))
      } else {
        await createRole({
          metadata: {} as Role["metadata"],
          spec: {
            name: values.name,
            displayName: values.displayName || undefined,
            description: values.description || undefined,
            scope: values.scope,
            rules: values.rules,
          } as Role["spec"],
        })
        toast.success(t("action.createSuccess"))
      }
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "role", "role.title")
    } finally {
      setLoading(false)
    }
  }

  const selectedRules = useWatch({ control: form.control, name: "rules" })

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? t("role.edit") : t("role.create")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:!max-w-none !w-auto min-w-[800px]"
      bodyClassName="grid grid-cols-1 md:grid-cols-3 gap-6 min-h-0 flex-1"
    >
      {/* Left: basic fields */}
      <div className="col-span-1 -mx-1 space-y-4 overflow-y-auto px-1">
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t("role.name")}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  disabled={isEdit}
                  onBlur={async (e) => {
                    field.onBlur()
                    if (isEdit || !e.target.value) return
                    const valid = await form.trigger("name")
                    if (valid) checkUniqueness(e.target.value)
                  }}
                />
              </FormControl>
              {!isEdit && <FormDescription>{t("role.validation.name.hint")}</FormDescription>}
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="scope"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t("role.scope")}</FormLabel>
              <Select name={field.name} value={field.value} onValueChange={field.onChange} disabled>
                <FormControl>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="platform">{t("role.scope.platform")}</SelectItem>
                </SelectContent>
              </Select>
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
                <Textarea {...field} rows={3} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
      {/* Right: permission selector */}
      <FormField
        control={form.control}
        name="rules"
        render={() => (
          <FormItem className="col-span-2 flex min-h-0 flex-col">
            <div className="text-sm font-medium">
              {t("role.rules")}
              <span className="text-destructive ml-0.5">*</span>
              {selectedRules.length > 0 && (
                <span className="text-muted-foreground ml-2 font-normal">
                  ({t("role.rulesCount", { count: selectedRules.length })})
                </span>
              )}
            </div>
            <PermissionSelector
              permissions={permissions}
              value={selectedRules}
              onChange={(rules) => form.setValue("rules", rules, { shouldValidate: true })}
              scope="platform"
            />
            <FormMessage />
          </FormItem>
        )}
      />
    </FormDialog>
  )
}
