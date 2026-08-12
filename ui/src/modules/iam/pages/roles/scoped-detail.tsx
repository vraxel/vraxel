import { useMemo, useState } from "react"
import { formatDateTime } from "@/shared/lib/format"
import { useParams, useNavigate, useSearchParams } from "react-router"
import { Pencil, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { TruncateText } from "@/shared/components/truncate-text"
import {
  getWorkspaceRole,
  deleteWorkspaceRole,
  getNamespaceRole,
  deleteNamespaceRole,
  listAllPermissions,
  listWorkspaceRoleBindings,
  listNamespaceRoleBindings,
  createWorkspaceRoleBindings,
  createNamespaceRoleBindings,
  deleteWorkspaceRoleBinding,
  deleteNamespaceRoleBinding,
} from "@/modules/iam/api/rbac"
import { listWorkspaceUsers, listNamespaceUsers } from "@/modules/iam/api/users"
import { showApiError } from "@/core/api/client"
import type { Permission } from "@/modules/iam/api/types"
import { useTranslation } from "@/i18n"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient } from "@tanstack/react-query"
import { PermissionSelector, patternCovers } from "@/modules/iam/components/permission-selector"
import { ScopedRoleFormDialog } from "@/modules/iam/components/scoped-role-form-dialog"
import { RoleUsersSection } from "@/modules/iam/components/role-users-section"
import { usePermission } from "@/core/permission/use-permission"

export default function ScopedRoleDetailPage() {
  const { workspaceId, namespaceId, roleId } = useParams()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const detailQuery = useApiQuery({
    queryKey: ["iam", "role-detail", workspaceId, namespaceId ?? "", roleId],
    queryFn: async () => {
      const [r, perms] = await Promise.all([
        namespaceId
          ? getNamespaceRole(workspaceId!, namespaceId, roleId!)
          : getWorkspaceRole(workspaceId!, roleId!),
        listAllPermissions(),
      ])
      return { role: r, permissions: perms }
    },
    enabled: !!roleId,
  })
  const role = detailQuery.data?.role ?? null
  const permissions = useMemo(() => detailQuery.data?.permissions ?? [], [detailQuery.data])
  const loading = detailQuery.isPending
  const refresh = () =>
    qc.invalidateQueries({
      queryKey: ["iam", "role-detail", workspaceId, namespaceId ?? "", roleId],
    })
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const scope = namespaceId ? "namespace" : "workspace"
  const scopeId = namespaceId ?? workspaceId!
  const scopeParams = namespaceId
    ? { workspaceId: workspaceId!, namespaceId }
    : { workspaceId: workspaceId! }

  // Tab lives in the URL so a refresh, a back-navigation from a user
  // detail page, or a shared link all land on the same tab.
  const [searchParams, setSearchParams] = useSearchParams()
  const canListBindings = hasPermission("iam:rolebindings:list", scopeParams)
  const tab = searchParams.get("tab") === "users" && canListBindings ? "users" : "overview"
  const setTab = (value: string) =>
    setSearchParams(value === "overview" ? {} : { tab: value }, { replace: true })

  // Build base path for back navigation
  const basePath = namespaceId
    ? `/iam/workspaces/${workspaceId}/namespaces/${namespaceId}/roles`
    : `/iam/workspaces/${workspaceId}/roles`

  const handleEdit = () => {
    setEditOpen(true)
  }

  const handleDelete = async () => {
    if (!role) return
    try {
      if (namespaceId) {
        await deleteNamespaceRole(workspaceId!, namespaceId, role.metadata.id)
      } else {
        await deleteWorkspaceRole(workspaceId!, role.metadata.id)
      }
      toast.success(t("action.deleteSuccess"))
      navigate(basePath)
    } catch (err) {
      showApiError(err, t, "role.title")
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
          <Badge variant={role.spec.builtin ? "secondary" : "outline"}>
            {role.spec.builtin ? t("role.builtin") : t("role.custom")}
          </Badge>
        </div>
        {!role.spec.builtin && (
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" size="sm" onClick={handleEdit}>
              <Pencil className="mr-2 h-4 w-4" />
              {t("common.edit")}
            </Button>
            <Button variant="destructive" size="sm" onClick={() => setDeleteOpen(true)}>
              <Trash2 className="mr-2 h-4 w-4" />
              {t("common.delete")}
            </Button>
          </div>
        )}
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="mb-6">
          <TabsTrigger value="overview">{t("common.overview")}</TabsTrigger>
          {canListBindings && <TabsTrigger value="users">{t("role.users")}</TabsTrigger>}
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          {/* role info card */}
          <Card>
            <CardHeader>
              <CardTitle>{t("role.details")}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-x-8 gap-y-4 text-sm md:grid-cols-2">
                <div className="min-w-0">
                  <span className="text-muted-foreground mb-1 block text-xs">{t("role.name")}</span>
                  <p className="font-medium">
                    <TruncateText>{role.spec.name}</TruncateText>
                  </p>
                </div>
                <div className="min-w-0">
                  <span className="text-muted-foreground mb-1 block text-xs">
                    {t("common.displayName")}
                  </span>
                  <p className="font-medium">
                    <TruncateText>
                      {t(`role.${role.spec.name}`, { defaultValue: role.spec.displayName || "-" })}
                    </TruncateText>
                  </p>
                </div>
                <div>
                  <span className="text-muted-foreground mb-1 block text-xs">
                    {t("role.scope")}
                  </span>
                  <p>
                    <Badge variant={scope === "workspace" ? "secondary" : "outline"}>
                      {t(`role.scope.${scope}`)}
                    </Badge>
                  </p>
                </div>
                <div>
                  <span className="text-muted-foreground mb-1 block text-xs">
                    {t("role.builtin")}
                  </span>
                  <p>
                    <Badge variant={role.spec.builtin ? "secondary" : "outline"}>
                      {role.spec.builtin ? t("role.builtin") : t("role.custom")}
                    </Badge>
                  </p>
                </div>
                <div className="col-span-2 min-w-0">
                  <span className="text-muted-foreground mb-1 block text-xs">
                    {t("common.description")}
                  </span>
                  <p className="font-medium">
                    <TruncateText lines={3}>
                      {t(`role.desc.${role.spec.name}`, {
                        defaultValue: role.spec.description || "-",
                      })}
                    </TruncateText>
                  </p>
                </div>
                <div>
                  <span className="text-muted-foreground mb-1 block text-xs">
                    {t("common.created")}
                  </span>
                  <p className="font-medium">{formatDateTime(role.metadata.createdAt)}</p>
                </div>
                <div>
                  <span className="text-muted-foreground mb-1 block text-xs">
                    {t("common.updated")}
                  </span>
                  <p className="font-medium">{formatDateTime(role.metadata.updatedAt)}</p>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* permission rules card */}
          <Card>
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
                  <div className="overflow-y-auto rounded-lg border">
                    <PermissionSelector
                      permissions={permissions}
                      value={role.spec.rules}
                      readOnly
                    />
                  </div>
                  <div className="overflow-y-auto rounded-lg border">
                    <MatchedRulesList rules={role.spec.rules} permissions={permissions} />
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {canListBindings && (
          <TabsContent value="users">
            <RoleUsersSection
              config={{
                roleId: role.metadata.id,
                detailPrefix: namespaceId
                  ? `/iam/workspaces/${workspaceId}/namespaces/${namespaceId}`
                  : `/iam/workspaces/${workspaceId}`,
                listBindings: (params) =>
                  namespaceId
                    ? listNamespaceRoleBindings(workspaceId!, namespaceId, params)
                    : listWorkspaceRoleBindings(workspaceId!, params),
                // Candidates = current members + not-yet-members. Members must be
                // included so an existing member can receive a second role; the
                // dialog itself excludes users already holding THIS role.
                listCandidates: async (params) => {
                  const list = (p?: typeof params) =>
                    namespaceId
                      ? listNamespaceUsers(workspaceId!, namespaceId, p)
                      : listWorkspaceUsers(workspaceId!, p)
                  const [members, nonMembers] = await Promise.all([
                    list(params),
                    list({ ...params, available: "true" }),
                  ])
                  const seen = new Set<string>()
                  const items = [...members.items, ...nonMembers.items].filter((u) => {
                    if (seen.has(u.metadata.id)) return false
                    seen.add(u.metadata.id)
                    return true
                  })
                  return { ...members, items, totalCount: items.length }
                },
                assign: (ids) =>
                  namespaceId
                    ? createNamespaceRoleBindings(workspaceId!, namespaceId, ids, role.metadata.id)
                    : createWorkspaceRoleBindings(workspaceId!, ids, role.metadata.id),
                revoke: (id) =>
                  namespaceId
                    ? deleteNamespaceRoleBinding(workspaceId!, namespaceId, id)
                    : deleteWorkspaceRoleBinding(workspaceId!, id),
                canAssign: hasPermission("iam:rolebindings:create", scopeParams),
                canRevoke: hasPermission("iam:rolebindings:delete", scopeParams),
                cacheKey: [scope, workspaceId, namespaceId, role.metadata.id],
              }}
            />
          </TabsContent>
        )}
      </Tabs>

      {/* edit dialog */}
      {!role.spec.builtin && (
        <ScopedRoleFormDialog
          open={editOpen}
          onOpenChange={setEditOpen}
          scope={scope}
          scopeId={scopeId}
          workspaceId={workspaceId}
          role={role}
          permissions={permissions}
          onSuccess={refresh}
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
