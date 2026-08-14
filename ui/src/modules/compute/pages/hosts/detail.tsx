import { useState } from "react"
import { Link, useNavigate, useParams } from "react-router"
import { ArrowLeft, Pencil, PlugZap, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { formatDateTime } from "@/shared/lib/format"
import { Button } from "@/shared/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Skeleton } from "@/shared/ui/skeleton"
import { useApiQuery } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { useQueryClient } from "@tanstack/react-query"
import { showApiError } from "@/core/api/client"
import { useTranslation } from "@/i18n"
import { usePermission } from "@/core/permission/use-permission"
import { buildPermScope, buildScopedPath } from "@/core/registry/nav-config"
import type { ScopeRef } from "@/core/registry/resource"
import { hostsApi } from "@/modules/compute/api/hosts"
import { hostsDef } from "@/modules/compute/defs"
import { AgentStatusBadge } from "@/modules/compute/components/agent-status-badge"
import { HostEditDialog } from "@/modules/compute/components/host-edit-dialog"
import { AgentInstallDialog } from "@/modules/compute/components/agent-install-dialog"
import { HostMergeDialog } from "@/modules/compute/components/host-merge-dialog"
import { useHostWatch } from "@/modules/compute/use-host-watch"
import { ConfirmDialog } from "@/shared/components/confirm-dialog"

export default function HostDetailPage() {
  const { hostId, workspaceId, namespaceId } = useParams()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { hasPermission } = usePermission()
  const qc = useQueryClient()
  const scope: ScopeRef = { ws: workspaceId, ns: namespaceId }
  const permScope = buildPermScope(workspaceId, namespaceId)
  const listPath = buildScopedPath("hosts", workspaceId ?? null, namespaceId ?? null)

  const canUpdate = hasPermission("compute:hosts:update", permScope)
  const canDelete = hasPermission("compute:hosts:delete", permScope)
  // Installing an agent means minting a join token, which the API gates
  // on compute:hosts:create -- a token is the power to bring a machine
  // into this scope.
  const canInstallAgent = hasPermission("compute:hosts:create", permScope)

  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [installOpen, setInstallOpen] = useState(false)
  const [mergeOpen, setMergeOpen] = useState(false)

  const query = useApiQuery({
    queryKey: qk.detail(hostsDef, scope, hostId ?? ""),
    queryFn: () => hostsApi.get(scope, hostId!),
    enabled: !!hostId,
  })
  const host = query.data ?? null
  useHostWatch(scope)

  const handleDelete = async () => {
    if (!host) return
    try {
      await hostsApi.delete(scope, host.metadata.id)
      qc.invalidateQueries({ queryKey: qk.resource(hostsDef) })
      toast.success(t("action.deleteSuccess"))
      navigate(listPath)
    } catch (err) {
      showApiError(err, t, "compute.host.title")
    }
  }

  if (query.isPending) {
    return (
      <div className="space-y-4 p-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-48 w-full rounded-xl" />
      </div>
    )
  }
  if (!host) {
    return <div className="text-muted-foreground p-6 text-sm">{t("common.loadError")}</div>
  }

  return (
    <div className="p-6">
      <div className="mb-6 flex items-start gap-3">
        <Button asChild variant="ghost" size="icon" aria-label={t("compute.host.title")}>
          <Link to={listPath}>
            <ArrowLeft className="size-4" />
          </Link>
        </Button>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-3">
            <h1 className="truncate text-xl font-semibold tracking-tight">
              {host.spec.displayName || host.metadata.name}
            </h1>
            <AgentStatusBadge
              status={host.spec.agentStatus}
              conflictAt={host.spec.agentConflictAt}
            />
          </div>
          <p className="text-muted-foreground mt-0.5 text-sm">{host.metadata.name}</p>
        </div>
        <div className="flex items-center gap-2">
          {canInstallAgent && (
            <Button variant="outline" size="sm" onClick={() => setInstallOpen(true)}>
              <PlugZap className="size-4" />
              {t(host.spec.agentId ? "compute.host.reinstallAgent" : "compute.host.installAgent")}
            </Button>
          )}
          {canUpdate && (
            <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
              <Pencil className="size-4" />
              {t("common.edit")}
            </Button>
          )}
          {canDelete && (
            <Button variant="outline" size="sm" onClick={() => setDeleteOpen(true)}>
              <Trash2 className="size-4" />
              {t("common.delete")}
            </Button>
          )}
        </div>
      </div>

      {(host.spec.imageGroupSize ?? 0) > 1 && (
        <div className="border-warning/25 bg-warning/10 mb-6 flex items-start justify-between gap-3 rounded-lg border p-3 text-sm">
          <div>
            <p className="font-medium">{t("compute.host.imageGroup")}</p>
            <p className="text-muted-foreground mt-1 text-xs">
              {t("compute.host.imageGroupHint", { count: host.spec.imageGroupSize ?? 0 })}
            </p>
          </div>
          {canDelete && (
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() => setMergeOpen(true)}
            >
              {t("compute.host.merge")}
            </Button>
          )}
        </div>
      )}

      {host.spec.agentConflictAt && (
        <div className="border-destructive/25 bg-destructive/10 mb-6 rounded-lg border p-3 text-sm">
          <p className="font-medium">{t("compute.agent.conflict")}</p>
          <p className="text-muted-foreground mt-1 text-xs">{t("compute.agent.conflictHint")}</p>
        </div>
      )}

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("compute.host.basicInfo")}</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
              <Field label={t("compute.host.ip")} value={host.spec.reportedPrimaryIp} mono />
              <Field label={t("compute.host.hostname")} value={host.spec.hostname} />
              <Field label={t("compute.host.os")} value={host.spec.os} />
              <Field label={t("compute.host.arch")} value={host.spec.arch} />
              <Field
                label={t("compute.host.cpu")}
                value={`${host.spec.cpuCores ?? 0} ${t("compute.host.cores")}`}
              />
              <Field
                label={t("compute.host.memory")}
                value={`${Math.round((host.spec.memoryMb ?? 0) / 1024)} GiB`}
              />
              <Field label={t("compute.host.disk")} value={`${host.spec.diskGb ?? 0} GiB`} />
              <Field label={t("common.description")} value={host.spec.description} />
              <Field
                label={t("compute.host.origin")}
                value={
                  host.spec.origin === "agent"
                    ? t("compute.host.originAgent")
                    : t("compute.host.originManual")
                }
              />
              <Field label={t("common.createdBy")} value={host.spec.createdByName} />
              <Field label={t("common.created")} value={formatDateTime(host.metadata.createdAt)} />
            </dl>
            <p className="text-muted-foreground mt-4 text-xs">{t("compute.host.reportedNote")}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("compute.host.agentSession")}</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
              <Field label={t("compute.host.agentVersion")} value={host.spec.agentVersion} />
              <Field label={t("compute.host.agentId")} value={host.spec.agentId} mono />
              <Field
                label={t("compute.host.connectedAt")}
                value={formatDateTime(host.spec.agentConnectedAt)}
              />
              <Field
                label={t("compute.host.lastSeenAt")}
                value={formatDateTime(host.spec.agentLastSeenAt)}
              />
            </dl>
          </CardContent>
        </Card>
      </div>

      <HostEditDialog
        host={editOpen ? host : null}
        scope={scope}
        onClose={() => setEditOpen(false)}
        onSuccess={() =>
          qc.invalidateQueries({ queryKey: qk.detail(hostsDef, scope, hostId ?? "") })
        }
      />

      <AgentInstallDialog
        host={installOpen ? host : null}
        scope={scope}
        onClose={() => setInstallOpen(false)}
      />

      <HostMergeDialog
        host={mergeOpen ? host : null}
        scope={scope}
        onClose={() => setMergeOpen(false)}
        onMerged={() => navigate(listPath)}
      />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("common.delete")}
        // Deleting the row does not stop the machine: its agent keeps
        // dialling in with a credential nothing will honour again. Said
        // here because this is the last moment anyone is in a position to
        // do something about it -- afterwards there is no host page left
        // to say it on, and the only trace is a 401 in the server log.
        description={
          t("compute.host.deleteConfirm", { name: host.metadata.name }) +
          (host.spec.agentId ? `\n\n${t("compute.host.deleteAgentWarning")}` : "")
        }
        onConfirm={handleDelete}
        confirmText={t("common.delete")}
      />
    </div>
  )
}

function Field({ label, value, mono }: { label: string; value?: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className={`truncate ${mono ? "font-mono text-xs" : ""}`} title={value}>
        {value || "-"}
      </dd>
    </div>
  )
}
