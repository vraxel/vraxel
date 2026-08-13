import { useState } from "react"
import { Link } from "react-router"
import { KeyRound, Plus } from "lucide-react"
import { formatDateTime } from "@/shared/lib/format"
import { Button } from "@/shared/ui/button"
import { useTranslation } from "@/i18n"
import { useListQuery } from "@/frameworks/list/use-list-query"
import { NameCell } from "@/frameworks/list/name-cell"
import { ResourceListPage, type ColumnDef } from "@/frameworks/list/resource-list-page"
import type { ScopeRef } from "@/core/registry/resource"
import { hostsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { hostsDef } from "@/modules/compute/defs"
import { AgentStatusBadge } from "@/modules/compute/components/agent-status-badge"
import { PendingTokensSheet } from "@/modules/compute/components/pending-tokens-sheet"

export default function HostListPage() {
  const { t } = useTranslation()
  const [tokensOpen, setTokensOpen] = useState(false)

  const scope: ScopeRef = {}
  const query = useListQuery<Host>({
    def: hostsDef,
    api: hostsApi,
    scope,
    filterKeys: ["agentStatus"],
  })

  const columns: ColumnDef<Host>[] = [
    {
      key: "name",
      header: t("common.name"),
      sortable: true,
      cell: (h) => (
        <NameCell
          to={`/compute/hosts/${h.metadata.id}`}
          displayName={h.spec.displayName}
          name={h.metadata.name}
        />
      ),
    },
    {
      key: "agentStatus",
      header: t("compute.host.agentStatus"),
      // The one status column. hosts.status is deliberately absent: see
      // docs/agent/onboarding-design.md section 5.
      filter: [
        { value: "all", label: t("common.all") },
        { value: "online", label: t("compute.agent.online") },
        { value: "offline", label: t("compute.agent.offline") },
      ],
      cell: (h) => (
        <AgentStatusBadge status={h.spec.agentStatus} conflictAt={h.spec.agentConflictAt} />
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
      key: "agentVersion",
      header: t("compute.host.agentVersion"),
      cell: (h) => (
        <span className="text-muted-foreground text-xs">{h.spec.agentVersion || "-"}</span>
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

  const onboardButton = (
    <Button asChild>
      <Link to="/compute/hosts/onboard">
        <Plus className="size-4" />
        {t("compute.host.onboard")}
      </Link>
    </Button>
  )

  return (
    <>
      <ResourceListPage
        query={query}
        columns={columns}
        titleKey="compute.host.title"
        subtitle={t("compute.host.subtitle")}
        searchPlaceholderKey="compute.host.searchPlaceholder"
        // The framework renders the empty state itself and reuses
        // createButton as its call to action, so onboarding stays the one
        // thing offered on an empty table.
        emptyKey="compute.host.empty"
        selectable={false}
        createButton={
          <div className="flex items-center gap-2">
            {/* Pending tokens live behind a secondary affordance: they are
                an operational loose end, not a resource an operator
                manages day to day. But they must be reachable -- a token
                that leaked before it was redeemed has to be revocable. */}
            <Button variant="outline" onClick={() => setTokensOpen(true)}>
              <KeyRound className="size-4" />
              {t("compute.token.pending")}
            </Button>
            {onboardButton}
          </div>
        }
      />
      <PendingTokensSheet open={tokensOpen} onOpenChange={setTokensOpen} />
    </>
  )
}
