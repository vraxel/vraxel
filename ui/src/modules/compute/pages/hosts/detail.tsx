import { useState } from "react"
import { Link, useNavigate, useParams } from "react-router"
import { ArrowLeft, Pencil, PlugZap, ScrollText, SquareTerminal, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { Skeleton } from "@/shared/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs"
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
import { HostMetricsPanel } from "@/modules/compute/components/host-metrics-panel"
import {
  HostNetworkTab,
  HostOverviewTab,
  HostStorageTab,
} from "@/modules/compute/components/host-detail-tabs"
import { HostAccountsTab, HostProcessesTab } from "@/modules/compute/components/host-runtime-tabs"
import { AgentInstallDialog } from "@/modules/compute/components/agent-install-dialog"
import { HostMergeDialog } from "@/modules/compute/components/host-merge-dialog"
import { HostLogsDialog } from "@/modules/compute/components/host-logs-dialog"
import { HostTerminalDialog } from "@/modules/compute/components/host-terminal-dialog"
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
  const canOpenTerminal = hasPermission("compute:hosts:terminal", permScope)
  const canViewLogs = hasPermission("compute:hosts:logs", permScope)
  // Installing an agent means minting a join token, which the API gates
  // on compute:hosts:create -- a token is the power to bring a machine
  // into this scope.
  const canInstallAgent = hasPermission("compute:hosts:create", permScope)

  const [editOpen, setEditOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [installOpen, setInstallOpen] = useState(false)
  const [mergeOpen, setMergeOpen] = useState(false)
  const [terminalOpen, setTerminalOpen] = useState(false)
  const [logsOpen, setLogsOpen] = useState(false)

  const query = useApiQuery({
    queryKey: qk.detail(hostsDef, scope, hostId ?? ""),
    queryFn: () => hostsApi.get(scope, hostId!),
    enabled: !!hostId,
    // This response carries live utilisation, not just the host's record,
    // and useHostWatch cannot keep it fresh: that socket delivers state
    // CHANGES (an agent going offline, a host appearing), and a machine
    // quietly heartbeating produces none. Without a timer the page held
    // its first response forever -- the numbers aged silently and the
    // gauges greyed out after a minute, because the gauge judges a
    // reading's age against the wall clock and was telling the truth
    // about data this page had stopped refreshing.
    //
    // 15s is the agent's heartbeat, the rate at which these numbers can
    // actually change. It also keeps the worst-case age (one heartbeat
    // plus one interval) inside the 60s the gauge greys at.
    refetchInterval: 15_000,
    // The interval pauses while the tab is hidden, so coming back to a
    // page left open in another window would otherwise show minute-old
    // numbers until the next tick. Overrides the global default, which is
    // right for records that only change when somebody edits them.
    refetchOnWindowFocus: true,
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
              foreignMachineAt={host.spec.agentForeignMachineAt}
            />
          </div>
          <p className="text-muted-foreground mt-0.5 text-sm">{host.metadata.name}</p>
        </div>
        <div className="flex items-center gap-2">
          {canOpenTerminal && (
            <Button
              variant="outline"
              size="sm"
              // A terminal needs a live agent to carry it. Disabled rather
              // than hidden: the button is where an operator expects it,
              // and its tooltip says what is missing.
              disabled={host.spec.agentStatus !== "online"}
              title={
                host.spec.agentStatus === "online"
                  ? undefined
                  : t("compute.host.terminal.needsAgent")
              }
              onClick={() => setTerminalOpen(true)}
            >
              <SquareTerminal className="size-4" />
              {t("compute.host.terminal.open")}
            </Button>
          )}
          {canViewLogs && (
            <Button
              variant="outline"
              size="sm"
              // Logs ride the same agent channel as the terminal.
              disabled={host.spec.agentStatus !== "online"}
              title={
                host.spec.agentStatus === "online" ? undefined : t("compute.host.logs.needsAgent")
              }
              onClick={() => setLogsOpen(true)}
            >
              <ScrollText className="size-4" />
              {t("compute.host.logs.open")}
            </Button>
          )}
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

      {/* Ranked above online/offline for the same reason the conflict
          banner is: the host reads offline, and this is the reason. */}
      {host.spec.agentForeignMachineAt && (
        <div className="border-destructive/25 bg-destructive/10 mb-6 rounded-lg border p-3 text-sm">
          <p className="font-medium">{t("compute.agent.foreignMachine")}</p>
          <p className="text-muted-foreground mt-1 text-xs">
            {t("compute.agent.foreignMachineHint")}
          </p>
          {host.spec.agentForeignMachineUuid && (
            <p className="text-muted-foreground mt-1 font-mono text-xs">
              {host.spec.agentForeignMachineUuid}
            </p>
          )}
        </div>
      )}

      {/* A tab earns its place by holding something that would otherwise
          crowd the page, not by naming a category. The scalar fields all
          fit on one screen together, so they share the overview; the
          inventory TABLES get their own, because a real server fills them
          with a dozen disks and twenty mounts; and metrics gets one
          because mounting it starts a polling query and a chart per
          series. */}
      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">{t("compute.host.tab.overview")}</TabsTrigger>
          <TabsTrigger value="network">{t("compute.host.tab.network")}</TabsTrigger>
          <TabsTrigger value="storage">{t("compute.host.tab.storage")}</TabsTrigger>
          <TabsTrigger value="processes">{t("compute.host.tab.processes")}</TabsTrigger>
          <TabsTrigger value="accounts">{t("compute.host.tab.accounts")}</TabsTrigger>
          <TabsTrigger value="metrics">{t("compute.host.tab.metrics")}</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4">
          <HostOverviewTab host={host} />
          <p className="text-muted-foreground mt-4 text-xs">{t("compute.host.reportedNote")}</p>
        </TabsContent>
        <TabsContent value="network" className="mt-4">
          <HostNetworkTab host={host} />
        </TabsContent>
        <TabsContent value="storage" className="mt-4">
          <HostStorageTab host={host} />
        </TabsContent>
        {/* Both fetch, unlike the two tabs above, which read lists already
            on the host object. Mounted only while selected so a page
            opened to read the hostname does not pay for two requests --
            and so a viewer without compute:hosts:accounts:list only sees
            that tab fail when they choose to open it. */}
        <TabsContent value="processes" className="mt-4">
          <HostProcessesTab host={host} scope={scope} />
        </TabsContent>
        <TabsContent value="accounts" className="mt-4">
          <HostAccountsTab host={host} scope={scope} />
        </TabsContent>
        {/* Mounted only while selected: the panel owns a polling query
            and a chart per series, and paying for those on a page opened
            to read the hostname is what the tabs are here to avoid. */}
        <TabsContent value="metrics" className="mt-4">
          <HostMetricsPanel host={host} scope={scope} />
        </TabsContent>
      </Tabs>

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

      <HostTerminalDialog
        host={host}
        scope={scope}
        open={terminalOpen}
        onOpenChange={setTerminalOpen}
      />

      <HostLogsDialog host={host} scope={scope} open={logsOpen} onOpenChange={setLogsOpen} />

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
