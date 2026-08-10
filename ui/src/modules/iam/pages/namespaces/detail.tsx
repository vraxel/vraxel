import { useEffect, useId, useState } from "react"
import { useParams, useNavigate } from "react-router"
import { Pencil, Trash2, Users, ShieldCheck } from "lucide-react"
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
import { FormDialog } from "@/frameworks/form/form-dialog"
import { updateNamespace, namespacesApi } from "@/modules/iam/api/namespaces"
import { namespacesDef } from "@/modules/iam/defs"
import { qk } from "@/core/query/keys"
import { useApiQuery } from "@/core/query/hooks"
import { useQueryClient } from "@tanstack/react-query"
import { handleFormApiError, showApiError } from "@/core/api/client"
import type { Namespace } from "@/modules/iam/api/types"
import { useTranslation } from "@/i18n"
import { usePermission } from "@/core/permission/use-permission"

export default function NamespaceDetailPage() {
  const { namespaceId, workspaceId } = useParams()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const detailQuery = useApiQuery({
    queryKey: qk.detail(namespacesDef, {}, namespaceId ?? ""),
    queryFn: () => namespacesApi.get({}, namespaceId!),
    enabled: !!namespaceId,
  })
  const namespace = detailQuery.data ?? null
  const loading = detailQuery.isPending
  const invalidate = () => qc.invalidateQueries({ queryKey: qk.resource(namespacesDef) })

  const handleDelete = async () => {
    if (!namespace) return
    try {
      await namespacesApi.delete({}, namespace.metadata.id)
      useScopeStore.getState().invalidate()
      qc.invalidateQueries({ queryKey: qk.resource(namespacesDef) })
      toast.success(t("action.deleteSuccess"))
      navigate(workspaceId ? `/iam/workspaces/${workspaceId}/namespaces` : "/iam/namespaces")
    } catch (err) {
      showApiError(err, t, "namespace.title")
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

  if (!namespace) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">{t("namespace.notFound")}</p>
      </div>
    )
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between gap-4">
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <h1 className="min-w-0 flex-1 text-2xl font-bold">
            <TruncateText>{namespace.metadata.name}</TruncateText>
          </h1>
          <Badge variant={namespace.spec.status === "active" ? "default" : "secondary"}>
            {namespace.spec.status === "active" ? t("common.active") : t("common.inactive")}
          </Badge>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {hasPermission("iam:namespaces:update", {
            workspaceId: workspaceId || namespace.spec.workspaceId,
            namespaceId: namespace.metadata.id,
          }) && (
            <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
              <Pencil className="mr-2 h-4 w-4" />
              {t("common.edit")}
            </Button>
          )}
          {hasPermission("iam:namespaces:delete", {
            workspaceId: workspaceId || namespace.spec.workspaceId,
            namespaceId: namespace.metadata.id,
          }) && (
            <Button variant="destructive" size="sm" onClick={() => setDeleteOpen(true)}>
              <Trash2 className="mr-2 h-4 w-4" />
              {t("common.delete")}
            </Button>
          )}
        </div>
      </div>

      {/* Overview content */}
      <div className="space-y-6">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <Card
            className="hover:bg-muted/50 cursor-pointer transition-colors"
            onClick={() =>
              navigate(
                `/iam/workspaces/${namespace.spec.workspaceId}/namespaces/${namespace.metadata.id}/users`,
              )
            }
          >
            <CardContent className="flex items-center gap-4 p-4">
              <div className="bg-primary/10 flex h-10 w-10 items-center justify-center rounded-lg">
                <Users className="text-primary h-5 w-5" />
              </div>
              <div>
                <p className="text-2xl font-bold">
                  {namespace.spec.memberCount ?? 0}
                  <span className="text-muted-foreground text-base font-normal">
                    /{namespace.spec.maxMembers || "\u221E"}
                  </span>
                </p>
                <p className="text-muted-foreground text-sm">{t("namespace.members")}</p>
              </div>
            </CardContent>
          </Card>
          <Card
            className="hover:bg-muted/50 cursor-pointer transition-colors"
            onClick={() =>
              navigate(
                `/iam/workspaces/${namespace.spec.workspaceId}/namespaces/${namespace.metadata.id}/rolebindings`,
              )
            }
          >
            <CardContent className="flex items-center gap-4 p-4">
              <div className="bg-primary/10 flex h-10 w-10 items-center justify-center rounded-lg">
                <ShieldCheck className="text-primary h-5 w-5" />
              </div>
              <div>
                <p className="text-2xl font-bold">{namespace.spec.roleBindingCount ?? 0}</p>
                <p className="text-muted-foreground text-sm">{t("rolebinding.title")}</p>
              </div>
            </CardContent>
          </Card>
        </div>
        <Card>
          <CardHeader>
            <CardTitle>{t("namespace.details")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 gap-x-8 gap-y-4 text-sm md:grid-cols-2">
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("common.name")}</span>
                <p className="font-medium">
                  <TruncateText>{namespace.metadata.name}</TruncateText>
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("common.displayName")}</span>
                <p className="font-medium">
                  <TruncateText>
                    {namespace.spec.displayName ||
                      (namespace.metadata.name.endsWith("-default")
                        ? t("namespace.builtinDefault")
                        : "-")}
                  </TruncateText>
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("namespace.owner")}</span>
                <p className="font-medium">
                  <TruncateText>{namespace.spec.ownerName || namespace.spec.ownerId}</TruncateText>
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("namespace.workspaceName")}</span>
                <p className="font-medium">
                  <TruncateText>
                    {namespace.spec.workspaceName || namespace.spec.workspaceId}
                  </TruncateText>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("namespace.visibility")}</span>
                <p>
                  <Badge variant={namespace.spec.visibility === "public" ? "default" : "secondary"}>
                    {namespace.spec.visibility === "public"
                      ? t("namespace.visibility.public")
                      : t("namespace.visibility.private")}
                  </Badge>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("common.status")}</span>
                <p>
                  <Badge variant={namespace.spec.status === "active" ? "default" : "secondary"}>
                    {namespace.spec.status === "active" ? t("common.active") : t("common.inactive")}
                  </Badge>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("namespace.maxMembers")}</span>
                <p className="font-medium">{namespace.spec.maxMembers || "\u221E"}</p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("namespace.memberCount")}</span>
                <p className="font-medium">
                  {namespace.spec.memberCount ?? 0}/{namespace.spec.maxMembers || "\u221E"}
                </p>
              </div>
              <div className="col-span-2 min-w-0">
                <span className="text-muted-foreground">{t("common.description")}</span>
                <p className="font-medium">
                  <TruncateText lines={3}>
                    {namespace.spec.description ||
                      (namespace.metadata.name.endsWith("-default")
                        ? t("namespace.builtinDefaultDesc", {
                            name: namespace.spec.workspaceName || "",
                          })
                        : "-")}
                  </TruncateText>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("common.created")}</span>
                <p className="font-medium">
                  {new Date(namespace.metadata.createdAt).toLocaleString()}
                </p>
              </div>
              <div className="min-w-0">
                <span className="text-muted-foreground">{t("common.createdBy")}</span>
                <p className="font-medium">
                  <TruncateText>{namespace.spec.createdByName || "-"}</TruncateText>
                </p>
              </div>
              <div>
                <span className="text-muted-foreground">{t("common.updated")}</span>
                <p className="font-medium">
                  {new Date(namespace.metadata.updatedAt).toLocaleString()}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* edit dialog */}
      <EditNamespaceDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        namespace={namespace}
        onSuccess={invalidate}
      />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("common.delete")}
        description={t("namespace.deleteConfirm", { name: namespace.metadata.name })}
        onConfirm={handleDelete}
        confirmText={t("common.delete")}
      />
    </div>
  )
}

// ===== Edit Namespace Dialog =====

function EditNamespaceDialog({
  open,
  onOpenChange,
  namespace,
  onSuccess,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  namespace: Namespace
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const nsNameId = useId()
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
    visibility: z.enum(["public", "private"]),
    maxMembers: z
      .number()
      .min(0, t("namespace.validation.maxMembers"))
      .max(1000000, t("namespace.validation.maxMembers"))
      .refine((v) => Number.isInteger(v), t("namespace.validation.maxMembers"))
      .optional(),
    status: z.enum(["active", "inactive"]),
  })

  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: {
      displayName: namespace.spec.displayName ?? "",
      description: namespace.spec.description ?? "",
      visibility: namespace.spec.visibility ?? "public",
      maxMembers: namespace.spec.maxMembers ?? 0,
      status: namespace.spec.status ?? "active",
    },
  })

  useEffect(() => {
    if (open) {
      form.reset({
        displayName: namespace.spec.displayName ?? "",
        description: namespace.spec.description ?? "",
        visibility: namespace.spec.visibility ?? "public",
        maxMembers: namespace.spec.maxMembers ?? 0,
        status: namespace.spec.status ?? "active",
      })
    }
  }, [open, namespace, form])

  const onSubmit = async (values: FormValues) => {
    setLoading(true)
    try {
      await updateNamespace(namespace.metadata.id, {
        metadata: namespace.metadata,
        spec: {
          ...namespace.spec,
          displayName: values.displayName,
          description: values.description,
          visibility: values.visibility,
          maxMembers: values.maxMembers,
          status: values.status,
        },
      })
      useScopeStore.getState().invalidate()
      toast.success(t("action.updateSuccess"))
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "namespace", "namespace.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("namespace.edit")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:max-w-lg"
    >
      <div>
        <label htmlFor={nsNameId} className="text-sm font-medium">
          {t("common.name")}
        </label>
        <Input
          id={nsNameId}
          name="ns-name"
          value={namespace.metadata.name}
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
        name="visibility"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("namespace.visibility")}</FormLabel>
            <Select name={field.name} value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="public">{t("namespace.visibility.public")}</SelectItem>
                <SelectItem value="private">{t("namespace.visibility.private")}</SelectItem>
              </SelectContent>
            </Select>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="maxMembers"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("namespace.maxMembers")}</FormLabel>
            <FormControl>
              <Input
                type="number"
                min={0}
                max={1000000}
                {...field}
                onChange={(e) =>
                  field.onChange(e.target.value ? Number(e.target.value) : undefined)
                }
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
