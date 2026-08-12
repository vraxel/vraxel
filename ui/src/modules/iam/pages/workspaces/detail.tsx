import { useEffect, useId, useState } from "react"
import { formatDateTime } from "@/shared/lib/format"
import { useParams, useNavigate } from "react-router"
import { Pencil, Trash2, FolderKanban, Users } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { useScopeStore } from "@/core/scope/scope-store"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"

import { ConfirmDialog } from "@/shared/components/confirm-dialog"
import { TruncateText } from "@/shared/components/truncate-text"
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import { updateWorkspace, workspacesApi } from "@/modules/iam/api/workspaces"
import { workspacesDef } from "@/modules/iam/defs"
import { qk } from "@/core/query/keys"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient } from "@tanstack/react-query"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { Workspace } from "@/modules/iam/api/types"
import { OverviewCard } from "@/shared/components/overview-card"
import { FormDialog } from "@/frameworks/form/form-dialog"
import { useTranslation } from "@/i18n"
import { usePermission } from "@/core/permission/use-permission"
import { useWorkspaceStore } from "@/core/scope/workspace-store"

export default function WorkspaceDetailPage() {
  const { workspaceId } = useParams()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const setCurrentWorkspace = useWorkspaceStore((s) => s.setCurrentWorkspace)
  const qc = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const detailQuery = useApiQuery({
    queryKey: qk.detail(workspacesDef, {}, workspaceId ?? ""),
    queryFn: () => workspacesApi.get({}, workspaceId!),
    enabled: !!workspaceId,
  })
  const workspace = detailQuery.data ?? null
  const loading = detailQuery.isPending
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(workspacesDef) })

  // Keep the workspace store in sync with the loaded workspace (nav /
  // breadcrumb read the current name from it).
  useEffect(() => {
    if (workspaceId && detailQuery.data)
      setCurrentWorkspace(workspaceId, detailQuery.data.metadata.name)
  }, [workspaceId, detailQuery.data, setCurrentWorkspace])

  const handleDelete = async () => {
    if (!workspace) return
    try {
      await workspacesApi.delete({}, workspace.metadata.id)
      useScopeStore.getState().invalidate()
      qc.invalidateQueries({ queryKey: qk.resource(workspacesDef) })
      toast.success(t("action.deleteSuccess"))
      navigate("/iam/workspaces")
    } catch (err) {
      showApiError(err, t, "workspace.title")
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

  if (!workspace) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">{t("workspace.notFound")}</p>
      </div>
    )
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between gap-4">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <h1 className="min-w-0 flex-1 text-xl font-semibold tracking-tight">
            <TruncateText>{workspace.metadata.name}</TruncateText>
          </h1>
          <Badge variant={workspace.spec.status === "active" ? "default" : "secondary"}>
            {workspace.spec.status === "active" ? t("common.active") : t("common.inactive")}
          </Badge>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {hasPermission("iam:workspaces:update", { workspaceId }) && (
            <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
              <Pencil className="mr-2 h-4 w-4" />
              {t("common.edit")}
            </Button>
          )}
          {hasPermission("iam:workspaces:delete", { workspaceId }) && (
            <Button variant="destructive" size="sm" onClick={() => setDeleteOpen(true)}>
              <Trash2 className="mr-2 h-4 w-4" />
              {t("common.delete")}
            </Button>
          )}
        </div>
      </div>

      {/* Overview content */}
      <div className="space-y-6">
        <div className="grid grid-cols-2 gap-4">
          <OverviewCard
            label={t("workspace.namespaces")}
            icon={FolderKanban}
            value={workspace.spec.namespaceCount ?? 0}
            onClick={() => navigate("namespaces")}
          />
          <OverviewCard
            label={t("workspace.members")}
            icon={Users}
            value={workspace.spec.memberCount ?? 0}
            onClick={() => navigate("users")}
          />
        </div>
        <Card>
          <CardHeader>
            <CardTitle>{t("workspace.details")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-x-8 gap-y-4 text-sm md:grid-cols-2">
              <div className="min-w-0">
                <span className="text-muted-foreground mb-1 block text-xs">{t("common.name")}</span>
                <p className="font-medium">
                  <TruncateText>{workspace.metadata.name}</TruncateText>
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground mb-1 block text-xs">
                  {t("common.displayName")}
                </span>
                <p className="font-medium">
                  <TruncateText>{workspace.spec.displayName || "-"}</TruncateText>
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground mb-1 block text-xs">
                  {t("workspace.owner")}
                </span>
                <p className="font-medium">
                  <TruncateText>{workspace.spec.ownerName || workspace.spec.ownerId}</TruncateText>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground mb-1 block text-xs">
                  {t("common.status")}
                </span>
                <p>
                  <Badge variant={workspace.spec.status === "active" ? "default" : "secondary"}>
                    {workspace.spec.status === "active" ? t("common.active") : t("common.inactive")}
                  </Badge>
                </p>
              </div>
              <div className="col-span-2 min-w-0">
                <span className="text-muted-foreground mb-1 block text-xs">
                  {t("common.description")}
                </span>
                <p className="font-medium">
                  <TruncateText lines={3}>{workspace.spec.description || "-"}</TruncateText>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground mb-1 block text-xs">
                  {t("common.created")}
                </span>
                <p className="font-medium">
                  {formatDateTime(workspace.metadata.createdAt)}
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground mb-1 block text-xs">
                  {t("common.createdBy")}
                </span>
                <p className="font-medium">
                  <TruncateText>{workspace.spec.createdByName || "-"}</TruncateText>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground mb-1 block text-xs">
                  {t("common.updated")}
                </span>
                <p className="font-medium">
                  {formatDateTime(workspace.metadata.updatedAt)}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* edit dialog */}
      <EditWorkspaceDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        workspace={workspace}
        onSuccess={invalidate}
      />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("common.delete")}
        description={t("workspace.deleteConfirm", { name: workspace.metadata.name })}
        onConfirm={handleDelete}
        confirmText={t("common.delete")}
      />
    </div>
  )
}

// ===== Edit Workspace Dialog =====

function EditWorkspaceDialog({
  open,
  onOpenChange,
  workspace,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspace: Workspace
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const wsNameId = useId()
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
    status: z.enum(["active", "inactive"]),
  })

  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: {
      displayName: workspace.spec.displayName ?? "",
      description: workspace.spec.description ?? "",
      status: workspace.spec.status ?? "active",
    },
  })

  useEffect(() => {
    if (open) {
      form.reset({
        displayName: workspace.spec.displayName ?? "",
        description: workspace.spec.description ?? "",
        status: workspace.spec.status ?? "active",
      })
    }
  }, [open, workspace, form])

  const onSubmit = async (values: FormValues) => {
    setLoading(true)
    try {
      await updateWorkspace(workspace.metadata.id, {
        metadata: workspace.metadata,
        spec: {
          ...workspace.spec,
          displayName: values.displayName,
          description: values.description,
          status: values.status,
        },
      })
      useScopeStore.getState().invalidate()
      toast.success(t("action.updateSuccess"))
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "workspace", "workspace.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("workspace.edit")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:max-w-lg"
    >
      <div>
        <label htmlFor={wsNameId} className="text-sm font-medium">
          {t("common.name")}
        </label>
        <Input
          id={wsNameId}
          name="ws-name"
          value={workspace.metadata.name}
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
              <Textarea rows={3} {...field} />
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
