import { useEffect, useId, useMemo, useState } from "react"
import { useParams, useNavigate } from "react-router"
import { Pencil, Trash2 } from "lucide-react"
import { useForm, useWatch } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"

import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { TruncateText } from "@/shared/components/truncate-text"
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import {
  updateRole,
  listAllPermissions,
  rolesApi,
  listRoleBindings,
  createRoleBindings,
  deleteRoleBinding,
} from "@/modules/iam/api/rbac"
import { listUsers } from "@/modules/iam/api/users"
import { rolesDef, permissionsDef } from "@/modules/iam/defs"
import { qk } from "@/core/query/keys"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient } from "@tanstack/react-query"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { Permission, Role } from "@/modules/iam/api/types"
import { useTranslation } from "@/i18n"
import { usePermission } from "@/core/permission/use-permission"
import { PermissionSelector, patternCovers } from "@/modules/iam/components/permission-selector"
import { FormDialog } from "@/frameworks/form/form-dialog"
import { RoleUsersSection } from "@/modules/iam/components/role-users-section"

const SCOPE_VARIANT: Record<string, "default" | "secondary" | "outline"> = {
  platform: "default",
  workspace: "secondary",
  namespace: "outline",
}

export default function RoleDetailPage() {
  const { roleId } = useParams()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const detailQuery = useApiQuery({
    queryKey: qk.detail(rolesDef, {}, roleId ?? ""),
    queryFn: () => rolesApi.get({}, roleId!),
    enabled: !!roleId,
  })
  const permsQuery = useApiQuery({
    queryKey: qk.list(permissionsDef, {}, { all: true }),
    queryFn: () => listAllPermissions(),
  })
  const role = detailQuery.data ?? null
  const permissions = permsQuery.data ?? []
  const loading = detailQuery.isPending || permsQuery.isPending
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(rolesDef) })

  const handleEdit = () => {
    setEditOpen(true)
  }

  const handleDelete = async () => {
    if (!role) return
    try {
      await rolesApi.delete({}, role.metadata.id)
      qc.invalidateQueries({ queryKey: qk.resource(rolesDef) })
      toast.success(t("action.deleteSuccess"))
      navigate("/iam/roles")
    } catch (err) {
      showApiError(err, t)
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

  if (!role) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">{t("role.notFound")}</p>
      </div>
    )
  }

  return (
    <div className="p-6">
      {/* header */}
      <div className="mb-6 flex items-center justify-between gap-4">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <h1 className="min-w-0 flex-1 text-xl font-semibold tracking-tight">
            <TruncateText>{role.spec.name}</TruncateText>
          </h1>
          <Badge variant={SCOPE_VARIANT[role.spec.scope] ?? "outline"}>
            {t(`role.scope.${role.spec.scope}`)}
          </Badge>
          <Badge variant={role.spec.builtin ? "secondary" : "outline"}>
            {role.spec.builtin ? t("role.builtin") : t("role.custom")}
          </Badge>
        </div>
        {!role.spec.builtin &&
          (hasPermission("iam:roles:update") || hasPermission("iam:roles:delete")) && (
            <div className="flex shrink-0 items-center gap-2">
              {hasPermission("iam:roles:update") && (
                <Button variant="outline" size="sm" onClick={handleEdit}>
                  <Pencil className="mr-2 h-4 w-4" />
                  {t("common.edit")}
                </Button>
              )}
              {hasPermission("iam:roles:delete") && (
                <Button variant="destructive" size="sm" onClick={() => setDeleteOpen(true)}>
                  <Trash2 className="mr-2 h-4 w-4" />
                  {t("common.delete")}
                </Button>
              )}
            </div>
          )}
      </div>

      {/* role info card */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>{t("role.details")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-x-8 gap-y-4 text-sm md:grid-cols-2">
            <div className="min-w-0">
              <span className="text-muted-foreground">{t("role.name")}</span>
              <p className="font-medium">
                <TruncateText>{role.spec.name}</TruncateText>
              </p>
            </div>
            <div className="min-w-0">
              <span className="text-muted-foreground">{t("common.displayName")}</span>
              <p className="font-medium">
                <TruncateText>
                  {t(`role.${role.spec.name}`, { defaultValue: role.spec.displayName || "-" })}
                </TruncateText>
              </p>
            </div>
            <div>
              <span className="text-muted-foreground">{t("role.scope")}</span>
              <p>
                <Badge variant={SCOPE_VARIANT[role.spec.scope] ?? "outline"}>
                  {t(`role.scope.${role.spec.scope}`)}
                </Badge>
              </p>
            </div>
            <div>
              <span className="text-muted-foreground">{t("role.builtin")}</span>
              <p>
                <Badge variant={role.spec.builtin ? "secondary" : "outline"}>
                  {role.spec.builtin ? t("role.builtin") : t("role.custom")}
                </Badge>
              </p>
            </div>
            <div className="col-span-2 min-w-0">
              <span className="text-muted-foreground">{t("common.description")}</span>
              <p className="font-medium">
                <TruncateText lines={3}>
                  {t(`role.desc.${role.spec.name}`, { defaultValue: role.spec.description || "-" })}
                </TruncateText>
              </p>
            </div>
            <div>
              <span className="text-muted-foreground">{t("common.created")}</span>
              <p className="font-medium">{new Date(role.metadata.createdAt).toLocaleString()}</p>
            </div>
            <div>
              <span className="text-muted-foreground">{t("common.updated")}</span>
              <p className="font-medium">{new Date(role.metadata.updatedAt).toLocaleString()}</p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* permission rules card */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle>
            {t("role.rules")}
            <span className="text-muted-foreground ml-2 text-sm font-normal">
              ({t("role.rulesCount", { count: role.spec.rules?.length ?? 0 })})
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {!role.spec.rules || role.spec.rules.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t("role.noPermissions")}</p>
          ) : (
            <div
              className="grid grid-cols-1 gap-6 md:grid-cols-2"
              style={{ height: "min(600px, 60vh)" }}
            >
              <div className="overflow-y-auto border rounded-lg">
                <PermissionSelector permissions={permissions} value={role.spec.rules} readOnly />
              </div>
              <div className="overflow-y-auto border rounded-lg">
                <MatchedRulesList rules={role.spec.rules} permissions={permissions} />
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* users with this role */}
      {hasPermission("iam:rolebindings:list") && (
        <RoleUsersSection
          config={{
            roleId: role.metadata.id,
            detailPrefix: "/iam",
            listBindings: (params) => listRoleBindings(params),
            listCandidates: (params) => listUsers(params),
            assign: (ids) => createRoleBindings(ids, role.metadata.id),
            revoke: (id) => deleteRoleBinding(id),
            canAssign: hasPermission("iam:rolebindings:create"),
            canRevoke: hasPermission("iam:rolebindings:delete"),
            cacheKey: ["platform", role.metadata.id],
          }}
        />
      )}

      {/* edit dialog */}
      {!role.spec.builtin && (
        <EditRoleDialog
          open={editOpen}
          onOpenChange={setEditOpen}
          role={role}
          permissions={permissions}
          onSuccess={invalidate}
        />
      )}

      {/* delete confirm */}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("common.delete")}
        description={t("role.deleteConfirm", { name: role.metadata.name })}
        onConfirm={handleDelete}
        confirmText={t("common.delete")}
      />
    </div>
  )
}

// ===== Matched Rules List =====

function MatchedRulesList({ rules, permissions }: { rules: string[]; permissions: Permission[] }) {
  const { t } = useTranslation()

  const ruleDetails = useMemo(() => {
    return rules.map((rule) => {
      const all = rule.includes("*")
        ? permissions.filter((p) => patternCovers(rule, p.spec.code))
        : permissions.filter((p) => p.spec.code === rule)
      // Deduplicate by code — same code may exist at multiple scopes
      const seen = new Set<string>()
      const matched = all.filter((p) => {
        if (seen.has(p.spec.code)) return false
        seen.add(p.spec.code)
        return true
      })
      return { rule, matched }
    })
  }, [rules, permissions])

  return (
    <div>
      {ruleDetails.map(({ rule, matched }) => (
        <div key={rule} className="border-b last:border-b-0">
          <div className="bg-muted/30 flex items-center justify-between px-3 py-1.5">
            <code className="text-xs font-medium">{rule}</code>
            <span className="text-muted-foreground text-xs">
              {t("role.matchCount", { count: matched.length })}
            </span>
          </div>
          {matched.length > 0 && (
            <div className="px-3 py-1">
              {matched.map((p) => (
                <p key={p.spec.code} className="text-muted-foreground py-0.5 text-xs">
                  <TruncateText>
                    {t(`perm.${p.spec.code}`, { defaultValue: p.spec.description || p.spec.code })}
                  </TruncateText>
                </p>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

// ===== Edit Role Dialog =====

function EditRoleDialog({
  open,
  onOpenChange,
  role,
  permissions,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  role: Role
  permissions: Permission[]
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const roleNameId = useId()
  const roleScopeId = useId()
  const [loading, setLoading] = useState(false)

  const schema = z.object({
    displayName: z
      .string()
      .max(128, t("api.validation.maxLength", { max: 128 }))
      .optional(),
    description: z
      .string()
      .max(1000, t("api.validation.maxLength", { max: 1000 }))
      .optional(),
    rules: z.array(z.string()).min(1, t("role.validation.rules.required")),
  })

  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: {
      displayName: role.spec.displayName ?? "",
      description: role.spec.description ?? "",
      rules: role.spec.rules ?? [],
    },
  })

  useEffect(() => {
    if (open) {
      form.reset({
        displayName: role.spec.displayName ?? "",
        description: role.spec.description ?? "",
        rules: role.spec.rules ?? [],
      })
    }
  }, [open, role, form])

  const selectedRules = useWatch({ control: form.control, name: "rules" })

  const onSubmit = async (values: FormValues) => {
    setLoading(true)
    try {
      await updateRole(role.metadata.id, {
        metadata: role.metadata,
        spec: {
          ...role.spec,
          displayName: values.displayName || undefined,
          description: values.description || undefined,
          rules: values.rules,
        },
      })
      toast.success(t("action.updateSuccess"))
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "role", "role.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("role.edit")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:!max-w-none !w-auto min-w-[800px]"
      bodyClassName="grid grid-cols-1 md:grid-cols-3 gap-6 min-h-0 flex-1"
    >
      {/* Left: basic fields */}
      <div className="col-span-1 -mx-1 space-y-4 overflow-y-auto px-1">
        <div>
          <label htmlFor={roleNameId} className="text-sm font-medium">
            {t("role.name")}
          </label>
          <Input
            id={roleNameId}
            name="role-name"
            value={role.spec.name}
            disabled
            className="mt-1"
          />
        </div>
        <div>
          <label htmlFor={roleScopeId} className="text-sm font-medium">
            {t("role.scope")}
          </label>
          <Input
            id={roleScopeId}
            name="role-scope"
            value={t(`role.scope.${role.spec.scope}`)}
            disabled
            className="mt-1"
          />
        </div>
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
              scope={role.spec.scope as "platform" | "workspace" | "namespace"}
            />
            <FormMessage />
          </FormItem>
        )}
      />
    </FormDialog>
  )
}
