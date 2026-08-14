import { Link, useParams } from "react-router"
import { ArrowLeft, Trash2 } from "lucide-react"
import { formatDateTime } from "@/shared/lib/format"
import { Button } from "@/shared/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Skeleton } from "@/shared/ui/skeleton"
import { useApiQuery } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { useTranslation } from "@/i18n"
import { hostsApi } from "@/modules/compute/api/hosts"
import { hostsDef } from "@/modules/compute/defs"
import { buildScopedPath } from "@/core/registry/nav-config"
import type { ScopeRef } from "@/core/registry/resource"
import { AgentStatusBadge } from "@/modules/compute/components/agent-status-badge"

export default function HostDetailPage() {
  const { hostId, workspaceId, namespaceId } = useParams()
  const { t } = useTranslation()
  const scope: ScopeRef = { ws: workspaceId, ns: namespaceId }
  const listPath = buildScopedPath("hosts", workspaceId ?? null, namespaceId ?? null)

  const query = useApiQuery({
    queryKey: qk.detail(hostsDef, scope, hostId ?? ""),
    queryFn: () => hostsApi.get(scope, hostId!),
    enabled: !!hostId,
  })
  const host = query.data ?? null

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
          {/* No "revoke agent token" here. Every registration already
              bumps token_version, so re-running install-agent.sh rotates
              the credential and leaves the host working -- which is what
              an operator wants after a leak. A bare revoke would leave a
              registered host permanently offline with no way back except
              walking to the machine. */}
          <Button variant="outline" size="sm">
            <Trash2 className="size-4" />
            {t("common.delete")}
          </Button>
        </div>
      </div>

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
              <Field label={t("common.createdBy")} value={host.spec.createdByName} />
              <Field label={t("common.created")} value={formatDateTime(host.metadata.createdAt)} />
            </dl>
            {/* Everything above the description line is reported by the
                agent, not entered by anyone. Saying so once removes the
                question of why none of it is editable. */}
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
