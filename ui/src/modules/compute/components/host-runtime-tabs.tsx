import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"
import { formatDateTime } from "@/shared/lib/format"
import { useTranslation } from "@/i18n"
import { ApiError } from "@/core/api/client"
import type { ScopeRef } from "@/core/registry/resource"
import { hostAccountsApi, hostProcessesApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { SortHead, TableSearch } from "@/modules/compute/components/host-table-controls"
import { byNumber, byText, useTableView } from "@/modules/compute/components/host-table-view"
import type { HostAccount, HostListenPort, HostProcessGroup } from "@/generated/compute"

// The two runtime-inventory tabs. Unlike the network and storage tabs,
// which read lists already on the host object, these fetch: both are
// separate endpoints, and accounts is behind a permission of its own that
// a viewer of this page may not hold.

// Matches the metrics panel's options, because it is the same control
// answering the same question. "auto" is 30s here rather than following a
// window preset -- there is no window, the answer is always "now".
const REFRESH_OPTIONS = [
  { key: "auto", ms: 30_000 },
  { key: "off", ms: 0 },
  { key: "15s", ms: 15_000 },
  { key: "30s", ms: 30_000 },
  { key: "1m", ms: 60_000 },
  { key: "5m", ms: 300_000 },
] as const

type RefreshKey = (typeof REFRESH_OPTIONS)[number]["key"]

/** Renders one listening socket the way ss does: a v6 address is
 *  bracketed, so "::" and "0.0.0.0" cannot be misread as the same
 *  binding and a port never looks glued to a colon-run. */
function portLabel(p: HostListenPort): string {
  const addr = p.addr ?? ""
  const host = addr.includes(":") ? `[${addr}]` : addr
  return `${host}:${p.port}`
}

const units = ["B", "KiB", "MiB", "GiB", "TiB"]
function bytes(n?: number): string {
  if (!n || n <= 0) return "-"
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v < 10 && i > 0 ? v.toFixed(1) : Math.round(v)} ${units[i]}`
}

/** Processes: what the machine runs, grouped, with what each one serves
 *  and what it is using. */
export function HostProcessesTab({ host, scope }: { host: Host; scope: ScopeRef }) {
  const { t } = useTranslation()
  const [refreshKey, setRefreshKey] = useState<RefreshKey>("auto")
  const refreshMs = REFRESH_OPTIONS.find((r) => r.key === refreshKey)?.ms ?? 30_000

  const query = useQuery({
    queryKey: ["host-processes", host.metadata.id, scope.ws, scope.ns],
    queryFn: () => hostProcessesApi(scope, host.metadata.id),
    // Every read measures cpu on the host, so this polls rather than
    // reusing a cached answer -- the numbers are the reason to look.
    refetchInterval: refreshMs || false,
    refetchOnWindowFocus: refreshMs !== 0,
    // Keeps the table on screen through a refetch instead of dropping it
    // back to skeletons every interval.
    placeholderData: (prev) => prev,
  })

  const groups = query.data?.groups ?? []
  const live = query.data?.live ?? false
  const view = useTableView<HostProcessGroup>(
    groups,
    (g) =>
      [g.name, g.user, g.unit, g.exe, ...(g.ports ?? []).map((p) => `${p.proto} ${portLabel(p)}`)]
        .filter(Boolean)
        .join(" "),
    {
      name: byText((g) => g.name),
      user: byText((g) => g.user),
      count: byNumber((g) => g.count),
      unit: byText((g) => g.unit),
      cpu: byNumber((g) => g.cpuPct),
      rss: byNumber((g) => g.rssBytes),
      started: byNumber((g) => g.startedAtMs),
    },
    // Busiest first is what somebody opening this tab is looking for.
    // With no live read every cpu is 0, so the ties fall back to the
    // order the agent sent -- which the agent itself sorts, so the table
    // is at least the same on every refresh rather than shuffling.
    { by: "cpu", dir: "desc" },
  )

  if (query.isPending) return <TableSkeleton rows={8} />
  // Only when there is nothing to show. This polls every 30s, and a
  // failed refetch keeps the last good rows (TQ v5 sets error and retains
  // data) -- replacing a loaded table with an error line because one poll
  // in a hundred timed out loses more than it reports.
  if (query.isError && groups.length === 0)
    return <LoadError message={t("compute.host.runtimeLoadFailed")} />
  if (groups.length === 0) return <EmptyRuntime />

  return (
    <Card>
      <CardHeader className="gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle className="text-base">{t("compute.host.processes")}</CardTitle>
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-xs">
              {live
                ? t("compute.host.proc.liveAt", { at: formatDateTime(query.data?.reportedAt) })
                : t("compute.host.proc.storedAt", { at: formatDateTime(query.data?.reportedAt) })}
            </span>
            <Select value={refreshKey} onValueChange={(v) => setRefreshKey(v as RefreshKey)}>
              <SelectTrigger
                size="sm"
                className="h-8 w-24"
                aria-label={t("compute.host.metrics.refresh")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent align="end">
                {REFRESH_OPTIONS.map((r) => (
                  <SelectItem key={r.key} value={r.key}>
                    {r.key === "auto"
                      ? t("compute.host.metrics.refresh.auto")
                      : r.key === "off"
                        ? t("compute.host.metrics.refresh.off")
                        : r.key}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <TableSearch
            view={view}
            name="process-search"
            placeholder={t("compute.host.proc.search")}
          />
          {view.query && (
            <span className="text-muted-foreground text-xs">
              {t("compute.host.matchCount", { shown: view.rows.length, total: view.total })}
            </span>
          )}
        </div>
        {/* Said once, where the empty column is, rather than as a dash in
            every cell: an offline host has no cpu or memory to report and
            that is a property of the host, not of each row. */}
        {!live && (
          <p className="text-muted-foreground text-xs">{t("compute.host.proc.offlineNoStats")}</p>
        )}
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <SortHead view={view} field="name">
                {t("compute.host.proc.name")}
              </SortHead>
              <SortHead view={view} field="user">
                {t("compute.host.proc.user")}
              </SortHead>
              <SortHead view={view} field="count" className="text-right">
                {t("compute.host.proc.count")}
              </SortHead>
              <SortHead view={view} field="cpu" className="text-right">
                CPU
              </SortHead>
              <SortHead view={view} field="rss" className="text-right">
                {t("compute.host.proc.memory")}
              </SortHead>
              <SortHead view={view} field="unit">
                {t("compute.host.proc.unit")}
              </SortHead>
              <SortHead view={view} field="started">
                {t("compute.host.proc.started")}
              </SortHead>
              <TableHead>{t("compute.host.proc.ports")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {view.rows.map((g) => (
              <TableRow key={`${g.name}/${g.user ?? ""}/${g.unit ?? ""}/${g.container ?? false}`}>
                <TableCell>
                  <div className="font-mono text-xs">{g.name}</div>
                  {/* The binary's path, when it says something the name
                      does not -- which is when it is not the usual one. */}
                  {g.exe && (
                    <div
                      className="text-muted-foreground max-w-56 truncate font-mono text-xs"
                      title={g.exe}
                    >
                      {g.exe}
                    </div>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground font-mono text-xs">
                  {g.user || "-"}
                </TableCell>
                <TableCell className="text-right text-sm tabular-nums">
                  {g.count > 1 ? g.count : ""}
                </TableCell>
                <TableCell className="text-right text-sm tabular-nums">
                  {live ? `${(g.cpuPct ?? 0).toFixed(1)}%` : "-"}
                </TableCell>
                <TableCell className="text-right text-sm tabular-nums">
                  {live ? bytes(g.rssBytes) : "-"}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs">
                  {g.container ? (
                    <Badge variant="secondary">{t("compute.host.proc.container")}</Badge>
                  ) : (
                    g.unit || "-"
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs whitespace-nowrap">
                  {g.startedAtMs ? formatDateTime(new Date(g.startedAtMs).toISOString()) : "-"}
                </TableCell>
                <TableCell>
                  {g.ports?.length ? (
                    <div className="flex flex-wrap gap-1">
                      {g.ports.map((p) => (
                        <Badge
                          key={`${p.proto}-${p.addr ?? ""}-${p.port}`}
                          variant="outline"
                          className="font-mono text-xs"
                        >
                          {p.proto} {portLabel(p)}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <span className="text-muted-foreground text-xs">-</span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

/** Accounts: who can use the machine, and every route each of them has
 *  to root. Split in three because they are three different tables that
 *  happen to come from one report. */
export function HostAccountsTab({ host, scope }: { host: Host; scope: ScopeRef }) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ["host-accounts", host.metadata.id, scope.ws, scope.ns],
    queryFn: () => hostAccountsApi(scope, host.metadata.id),
    // A 403 here is not a fault to retry: it means this operator does not
    // hold compute:hosts:accounts:list, and no number of attempts changes
    // that.
    retry: false,
  })

  const users = query.data?.users ?? []
  const groups = query.data?.groups ?? []
  const sudoRules = query.data?.sudoRules ?? []

  if (query.isPending) return <TableSkeleton rows={8} />
  if (query.isError) {
    // A 403 has a message of its own: it is not a fault, it is this
    // operator not holding the permission, and telling them to retry
    // would waste their time.
    const forbidden = query.error instanceof ApiError && query.error.status === 403
    return (
      <LoadError
        message={
          forbidden ? t("compute.host.accountsForbidden") : t("compute.host.runtimeLoadFailed")
        }
      />
    )
  }
  if (users.length === 0 && groups.length === 0) return <EmptyRuntime />

  return (
    <div className="space-y-4">
      <Tabs defaultValue="users">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <TabsList>
            <TabsTrigger value="users">
              {t("compute.host.accountUsers")} ({users.length})
            </TabsTrigger>
            <TabsTrigger value="groups">
              {t("compute.host.accountGroups")} ({groups.length})
            </TabsTrigger>
            <TabsTrigger value="sudo">
              {t("compute.host.sudoRules")} ({sudoRules.length})
            </TabsTrigger>
          </TabsList>
          <span className="text-muted-foreground text-xs">
            {t("compute.host.reportedAt", { at: formatDateTime(query.data?.reportedAt) })}
          </span>
        </div>

        <TabsContent value="users" className="mt-4">
          <UsersTable users={users} />
        </TabsContent>
        <TabsContent value="groups" className="mt-4">
          <GroupsTable groups={groups} />
        </TabsContent>
        <TabsContent value="sudo" className="mt-4">
          <SudoRules rules={sudoRules} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

function UsersTable({ users }: { users: HostAccount[] }) {
  const { t } = useTranslation()
  const view = useTableView<HostAccount>(
    users,
    (u) =>
      [u.name, u.fullName, u.shell, u.group, ...(u.groups ?? []), ...(u.privileges ?? [])]
        .filter(Boolean)
        .join(" "),
    {
      // Privileged first, then anything that can log in: a passwd file is
      // mostly service accounts, and the two or three rows that matter
      // would otherwise sit wherever their uid put them.
      relevance: (a, b) => rank(a) - rank(b) || a.uid - b.uid,
      name: byText((u) => u.name),
      uid: byNumber((u) => u.uid),
      shell: byText((u) => u.shell),
      password: byText((u) => u.password),
      lastLogin: byNumber((u) => u.lastLoginAtMs),
      passwordChanged: byNumber((u) => u.passwordChangedAtMs),
    },
    { by: "relevance", dir: "asc" },
  )

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <TableSearch
            view={view}
            name="account-search"
            placeholder={t("compute.host.acct.search")}
          />
          {view.query && (
            <span className="text-muted-foreground text-xs">
              {t("compute.host.matchCount", { shown: view.rows.length, total: view.total })}
            </span>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <SortHead view={view} field="name">
                {t("compute.host.acct.name")}
              </SortHead>
              <SortHead view={view} field="uid" className="text-right">
                {t("compute.host.acct.uid")}
              </SortHead>
              <SortHead view={view} field="shell">
                {t("compute.host.acct.shell")}
              </SortHead>
              <SortHead view={view} field="password">
                {t("compute.host.acct.password")}
              </SortHead>
              <TableHead>{t("compute.host.acct.privileges")}</TableHead>
              <SortHead view={view} field="lastLogin">
                {t("compute.host.acct.lastLogin")}
              </SortHead>
              <SortHead view={view} field="passwordChanged">
                {t("compute.host.acct.passwordChanged")}
              </SortHead>
              <TableHead>{t("compute.host.acct.groups")}</TableHead>
              <TableHead className="text-right">{t("compute.host.acct.keys")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {view.rows.map((u) => (
              // Dimmed, not hidden. Most of a passwd file is service
              // accounts that cannot log in and hold nothing, and they
              // bury the two or three that matter -- but an account
              // inventory whose job includes "what accounts exist here"
              // must not drop rows to make itself readable.
              <TableRow key={u.name} className={rank(u) === 2 ? "opacity-55" : undefined}>
                <TableCell>
                  <div className="font-mono text-xs">{u.name}</div>
                  {u.fullName && u.fullName !== u.name && (
                    <div className="text-muted-foreground text-xs">{u.fullName}</div>
                  )}
                  {u.canLogin && (
                    <div className="text-muted-foreground mt-0.5 text-xs">
                      {t("compute.host.acct.canLogin")}
                    </div>
                  )}
                </TableCell>
                <TableCell className="text-right text-sm tabular-nums">{u.uid}</TableCell>
                <TableCell className="text-muted-foreground font-mono text-xs">
                  {u.shell || "-"}
                </TableCell>
                <TableCell>
                  <PasswordBadge state={u.password} />
                </TableCell>
                <TableCell>
                  {u.privileges?.length ? (
                    <div className="flex flex-wrap gap-1">
                      {u.privileges.map((p) => (
                        <Badge key={p} variant="destructive">
                          <PrivilegeLabel value={p} />
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <span className="text-muted-foreground text-xs">-</span>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs whitespace-nowrap">
                  <DayOrNever ms={u.lastLoginAtMs} />
                </TableCell>
                <TableCell className="text-muted-foreground text-xs whitespace-nowrap">
                  {day(u.passwordChangedAtMs) ?? "-"}
                </TableCell>
                <TableCell
                  className="text-muted-foreground max-w-48 truncate text-xs"
                  title={u.groups?.join(", ")}
                >
                  {u.groups?.length ? u.groups.join(", ") : "-"}
                </TableCell>
                <TableCell className="text-right text-sm tabular-nums">
                  {u.sshKeys?.length ? (
                    <span title={u.sshKeys.map((k) => `${k.type} ${k.fingerprint}`).join("\n")}>
                      {u.sshKeys.length}
                    </span>
                  ) : (
                    <span className="text-muted-foreground">-</span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function GroupsTable({ groups }: { groups: { name: string; gid: number; members?: string[] }[] }) {
  const { t } = useTranslation()
  // Most of /etc/group is groups nobody is in -- 48 of 58 on an ordinary
  // Debian box -- and a row saying so carries nothing. Counted rather
  // than silently dropped: "these are the groups" and "these are the
  // groups with anybody in them" are different claims.
  const named = groups.filter((g) => g.members?.length)
  const empty = groups.length - named.length
  const view = useTableView(
    named,
    (g) => [g.name, ...(g.members ?? [])].join(" "),
    {
      name: byText<(typeof named)[number]>((g) => g.name),
      gid: byNumber<(typeof named)[number]>((g) => g.gid),
      members: byNumber<(typeof named)[number]>((g) => g.members?.length),
    },
    { by: "name", dir: "asc" },
  )

  if (named.length === 0) return <EmptyRuntime />

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <TableSearch
            view={view}
            name="group-search"
            placeholder={t("compute.host.acct.groupSearch")}
          />
          {view.query && (
            <span className="text-muted-foreground text-xs">
              {t("compute.host.matchCount", { shown: view.rows.length, total: view.total })}
            </span>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <SortHead view={view} field="name">
                {t("compute.host.acct.groupName")}
              </SortHead>
              <SortHead view={view} field="gid" className="text-right">
                {t("compute.host.acct.gid")}
              </SortHead>
              <SortHead view={view} field="members">
                {t("compute.host.acct.members")}
              </SortHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {view.rows.map((g) => (
              <TableRow key={g.name}>
                <TableCell className="font-mono text-xs">{g.name}</TableCell>
                <TableCell className="text-right text-sm tabular-nums">{g.gid}</TableCell>
                <TableCell className="text-muted-foreground text-xs">
                  {g.members?.join(", ")}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {empty > 0 && (
          <p className="text-muted-foreground mt-2 text-xs">
            {t("compute.host.acct.emptyGroupsHidden", { count: empty })}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function SudoRules({ rules }: { rules: string[] }) {
  const { t } = useTranslation()
  if (rules.length === 0) return <EmptyRuntime />
  return (
    <Card>
      <CardContent className="pt-6">
        {/* Verbatim, and deliberately not parsed into a table: sudo's
            grammar is real, and a half-understood rendering would give a
            confident wrong answer about who can become root. */}
        <pre className="bg-muted/50 overflow-x-auto rounded-md p-3 font-mono text-xs">
          {rules.join("\n")}
        </pre>
        <p className="text-muted-foreground mt-2 text-xs">{t("compute.host.sudoRulesHint")}</p>
      </CardContent>
    </Card>
  )
}

/** Sort key: privileged first, then anything that can log in. */
function rank(u: HostAccount): number {
  if (u.privileges?.length) return 0
  if (u.canLogin) return 1
  return 2
}

/** A day-resolution date, or nothing. The source is truncated to the day
 *  so printing a time would be inventing precision. */
function day(ms?: number): string | null {
  if (!ms) return null
  return new Date(ms).toISOString().slice(0, 10)
}

function DayOrNever({ ms }: { ms?: number }) {
  const { t } = useTranslation()
  const d = day(ms)
  // "Never" is not the same as "unknown": lastlog records every login, so
  // an absent entry means this account has not been used, which for a
  // person's account is the finding.
  return <>{d ?? t("compute.host.acct.neverLoggedIn")}</>
}

function PasswordBadge({ state }: { state?: string }) {
  const { t } = useTranslation()
  if (!state) return <span className="text-muted-foreground text-xs">-</span>
  const labels: Record<string, string> = {
    set: t("compute.host.acct.pw.set"),
    locked: t("compute.host.acct.pw.locked"),
    disabled: t("compute.host.acct.pw.disabled"),
    empty: t("compute.host.acct.pw.empty"),
  }
  // "empty" is the only one of the four that is a finding: the account
  // can be logged into with no password at all.
  const variant = state === "empty" ? "destructive" : "secondary"
  return <Badge variant={variant}>{labels[state] ?? state}</Badge>
}

function PrivilegeLabel({ value }: { value: string }) {
  const { t } = useTranslation()
  // Spelled out rather than interpolated into the key: the key type is a
  // union of literals, so a computed one compiles only behind a cast.
  const labels: Record<string, string> = {
    root: t("compute.host.priv.root"),
    sudo: t("compute.host.priv.sudo"),
    "docker-group": t("compute.host.priv.dockerGroup"),
    "disk-group": t("compute.host.priv.diskGroup"),
  }
  return <>{labels[value] ?? value}</>
}

function TableSkeleton({ rows }: { rows: number }) {
  return (
    <Card>
      <CardContent className="space-y-2 pt-6">
        {Array.from({ length: rows }, (_, i) => (
          <Skeleton key={i} className="h-8 w-full" />
        ))}
      </CardContent>
    </Card>
  )
}

function LoadError({ message }: { message: string }) {
  return <div className="text-muted-foreground p-6 text-sm">{message}</div>
}

// Same wording problem as the facts tabs: an agent older than this
// feature connects, heartbeats and looks perfectly healthy while never
// sending one of these, so "wait for the agent" would be a promise the
// page cannot keep.
function EmptyRuntime() {
  const { t } = useTranslation()
  return <div className="text-muted-foreground p-6 text-sm">{t("compute.host.noRuntime")}</div>
}
