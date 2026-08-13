import { useState } from "react"
import { Link } from "react-router"
import { KeyRound, Plus } from "lucide-react"
import { formatDateTime } from "@/shared/lib/format"
import { Button } from "@/shared/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"
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
      // Status rides in the name cell rather than a column of its own.
      // It is the first thing an operator looks for, so it belongs beside
      // the name their eye is already on, and folding it in buys back a
      // column for data that has nowhere else to go. The name's width cap
      // is tightened from the default so the badge stays adjacent instead
      // of drifting right on long names.
      cell: (h) => (
        <div className="flex min-w-0 items-start gap-2">
          <NameCell
            to={`/compute/hosts/${h.metadata.id}`}
            displayName={h.spec.displayName}
            name={h.metadata.name}
            maxWidth="max-w-[220px]"
          />
          <AgentStatusBadge
            status={h.spec.agentStatus}
            conflictAt={h.spec.agentConflictAt}
            className="mt-0.5 shrink-0"
          />
        </div>
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
        // The status filter follows its column into the toolbar: left on
        // the name header it would read as filtering by name.
        toolbarExtra={
          <Select
            value={query.filters.agentStatus ?? "all"}
            onValueChange={(v) => query.setFilter("agentStatus", v)}
          >
            <SelectTrigger className="h-9 w-40">
              <SelectValue placeholder={t("compute.host.agentStatus")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("compute.host.agentStatusAll")}</SelectItem>
              <SelectItem value="online">{t("compute.agent.online")}</SelectItem>
              <SelectItem value="offline">{t("compute.agent.offline")}</SelectItem>
            </SelectContent>
          </Select>
        }
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
            <Button asChild>
              <Link to="/compute/hosts/onboard">
                <Plus className="size-4" />
                {t("compute.host.create")}
              </Link>
            </Button>
          </div>
        }
      />
      <PendingTokensSheet open={tokensOpen} onOpenChange={setTokensOpen} />
    </>
  )
}
