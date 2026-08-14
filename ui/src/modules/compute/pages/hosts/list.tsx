import { Link, useParams } from "react-router"
import { Plus } from "lucide-react"
import { formatDateTime } from "@/shared/lib/format"
import { Button } from "@/shared/ui/button"
import { useTranslation } from "@/i18n"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { NameCell } from "@/frameworks/list/name-cell"
import { StatusFilter } from "@/frameworks/list/status-filter"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import type { ScopeRef } from "@/core/registry/resource"
import { hostsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { hostsDef } from "@/modules/compute/defs"
import { buildScopedPath } from "@/core/registry/nav-config"
import { AgentStatusBadge } from "@/modules/compute/components/agent-status-badge"

export default function HostListPage() {
  const { t } = useTranslation()

  // Scope comes from the route, exactly as it does on every other
  // scoped list: the selector navigates to /compute/workspaces/{ws}/hosts
  // and the page must read it back, or picking a workspace would change
  // the URL and nothing else.
  const { workspaceId, namespaceId } = useParams()
  const scope: ScopeRef = { ws: workspaceId, ns: namespaceId }
  const query = useListQuery<Host>({
    def: hostsDef,
    api: hostsApi,
    scope,
    filterKeys: ["agentStatus"],
  })

  // Links stay inside the current scope; a detail page reached from a
  // workspace list must keep that workspace in its URL.
  const base = buildScopedPath("hosts", workspaceId ?? null, namespaceId ?? null)
  const hostPath = (suffix: string) => `${base}/${suffix}`

  const columns: ColumnDef<Host>[] = [
    {
      key: "name",
      header: t("common.name"),
      sortable: true,
      cell: (h) => (
        <NameCell
          to={hostPath(`${h.metadata.id}`)}
          displayName={h.spec.displayName}
          name={h.metadata.name}
          trailing={
            <AgentStatusBadge status={h.spec.agentStatus} conflictAt={h.spec.agentConflictAt} />
          }
        />
      ),
    },
    {
      key: "ip",
      header: t("compute.host.ip"),
      cell: (h) => <span className="font-mono text-xs">{h.spec.reportedPrimaryIp || "-"}</span>,
    },
    {
      key: "os",
      header: t("compute.host.os"),
      truncate: true,
      cell: (h) => (
        <div className="min-w-0">
          <div className="truncate">{h.spec.os || "-"}</div>
          <div className="text-muted-foreground text-xs">{h.spec.arch || "-"}</div>
        </div>
      ),
    },
    {
      key: "spec",
      header: t("compute.host.spec"),
      cell: (h) => (
        <span className="text-sm">
          {h.spec.cpuCores ?? 0} {t("compute.host.cores")} /{" "}
          {Math.round((h.spec.memoryMb ?? 0) / 1024)} GiB
        </span>
      ),
    },
    {
      key: "createdAt",
      header: t("common.created"),
      sortable: true,
      cell: (h) => (
        <span className="text-muted-foreground text-sm">
          {formatDateTime(h.metadata.createdAt)}
        </span>
      ),
    },
    {
      // CLAUDE.md: every listable resource shows its creator, immediately
      // right of the creation time.
      key: "createdBy",
      header: t("common.createdBy"),
      cell: (h) => <span className="text-sm">{h.spec.createdByName || "-"}</span>,
    },
  ]

  return (
    <>
      <ResourceListPage
        query={query}
        columns={columns}
        titleKey="compute.host.title"
        subtitle={t("compute.host.subtitle")}
        searchPlaceholderKey="compute.host.searchPlaceholder"
        // The framework renders the empty state itself and reuses
        // createButton as its call to action, so creating stays the one
        // thing offered on an empty table.
        emptyKey="compute.host.empty"
        selectable={false}
        toolbarExtra={
          <StatusFilter
            value={query.filters.agentStatus ?? "all"}
            onChange={(v) => query.setFilter("agentStatus", v)}
            allLabel={t("compute.host.agentStatusAll")}
            placeholder={t("compute.host.agentStatus")}
            options={[
              { value: "online", label: t("compute.agent.online") },
              { value: "offline", label: t("compute.agent.offline") },
            ]}
          />
        }
        // No pending-token affordance here. In the single-host flow a
        // token is alive for the minutes between opening the wizard and
        // the machine registering, so the list it would open is empty
        // almost always. It earns its place when batch onboarding lands
        // and one token serves a whole team -- max_uses stops being
        // pinned to 1 and there is something to watch being consumed.
        // Until then the API (GET / DELETE agent-join-tokens) is where a
        // leaked token gets cleaned up.
        createButton={
          <Button asChild>
            <Link to={hostPath("onboard")}>
              <Plus className="size-4" />
              {t("compute.host.create")}
            </Link>
          </Button>
        }
      />
    </>
  )
}
