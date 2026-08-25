import { useQuery } from "@tanstack/react-query"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { formatDateTime } from "@/shared/lib/format"
import { useTranslation } from "@/i18n"
import { ApiError } from "@/core/api/client"
import type { ScopeRef } from "@/core/registry/resource"
import { hostAccountsApi, hostProcessesApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import type { HostAccount, HostListenPort } from "@/generated/compute"

// The two runtime-inventory tabs. Unlike the network and storage tabs,
// which read lists already on the host object, these fetch: both are
// separate endpoints, and accounts is behind a permission of its own that
// a viewer of this page may not hold.

/** Renders one listening socket the way ss does: a v6 address is
 *  bracketed, so "::" and "0.0.0.0" cannot be misread as the same
 *  binding and a port never looks glued to a colon-run. */
function portLabel(p: HostListenPort): string {
  const addr = p.addr ?? ""
  const host = addr.includes(":") ? `[${addr}]` : addr
  return `${host}:${p.port}`
}

/** Processes: what the machine runs, grouped, with what each one serves. */
export function HostProcessesTab({ host, scope }: { host: Host; scope: ScopeRef }) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ["host-processes", host.metadata.id, scope.ws, scope.ns],
    queryFn: () => hostProcessesApi(scope, host.metadata.id),
  })

  if (query.isPending) return <TableSkeleton rows={6} />
  if (query.isError) return <LoadError message={t("compute.host.runtimeLoadFailed")} />

  const groups = query.data?.groups ?? []
  if (groups.length === 0) return <EmptyRuntime />

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between">
        <CardTitle className="text-base">{t("compute.host.processes")}</CardTitle>
        <ReportedAt at={query.data?.reportedAt} />
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("compute.host.proc.name")}</TableHead>
              <TableHead>{t("compute.host.proc.user")}</TableHead>
              <TableHead className="text-right">{t("compute.host.proc.count")}</TableHead>
              <TableHead>{t("compute.host.proc.unit")}</TableHead>
              <TableHead>{t("compute.host.proc.ports")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {groups.map((g) => (
              <TableRow key={`${g.name}/${g.user ?? ""}/${g.unit ?? ""}/${g.container ?? false}`}>
                <TableCell className="font-mono text-xs">{g.name}</TableCell>
                <TableCell className="text-muted-foreground font-mono text-xs">
                  {g.user || "-"}
                </TableCell>
                {/* A count of 1 is the ordinary case and says nothing; the
                    number is only interesting when a workload is many
                    processes, so 1 stays quiet. */}
                <TableCell className="text-right text-sm tabular-nums">
                  {g.count > 1 ? g.count : ""}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs">
                  {g.container ? (
                    <Badge variant="secondary">{t("compute.host.proc.container")}</Badge>
                  ) : (
                    g.unit || "-"
                  )}
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
 *  to root. */
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

  if (query.isPending) return <TableSkeleton rows={8} />
  if (query.isError) {
    // A 403 has a message of its own: it is not a fault, it is this
    // operator not holding compute:hosts:accounts:list, and telling them
    // to retry would waste their time.
    const forbidden = query.error instanceof ApiError && query.error.status === 403
    return (
      <LoadError
        message={
          forbidden ? t("compute.host.accountsForbidden") : t("compute.host.runtimeLoadFailed")
        }
      />
    )
  }

  const users = query.data?.users ?? []
  const groups = query.data?.groups ?? []
  const sudoRules = query.data?.sudoRules ?? []
  if (users.length === 0 && groups.length === 0) return <EmptyRuntime />

  // Most of /etc/group is groups nobody is in -- 48 of 58 on an ordinary
  // Debian box -- and a row saying so carries nothing. Counted rather
  // than silently dropped, because "these are the groups" and "these are
  // the groups with anybody in them" are different claims.
  const namedGroups = groups.filter((g) => g.members?.length)
  const emptyGroups = groups.length - namedGroups.length

  // Privileged accounts first, then the ones a person could log into,
  // then everything else. A passwd file is mostly service accounts
  // nobody reads, and the two or three rows that matter would otherwise
  // sit wherever their uid put them.
  const sorted = [...users].sort((a, b) => rank(a) - rank(b) || a.uid - b.uid)

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <CardTitle className="text-base">{t("compute.host.accountUsers")}</CardTitle>
          <ReportedAt at={query.data?.reportedAt} />
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("compute.host.acct.name")}</TableHead>
                <TableHead className="text-right">{t("compute.host.acct.uid")}</TableHead>
                <TableHead>{t("compute.host.acct.shell")}</TableHead>
                <TableHead>{t("compute.host.acct.password")}</TableHead>
                <TableHead>{t("compute.host.acct.privileges")}</TableHead>
                <TableHead>{t("compute.host.acct.groups")}</TableHead>
                <TableHead className="text-right">{t("compute.host.acct.keys")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sorted.map((u) => (
                // Dimmed, not hidden. Most of a passwd file is service
                // accounts that cannot log in and hold nothing -- 26 of
                // the 29 rows on an ordinary Debian box -- and they bury
                // the two or three that matter. But an account inventory
                // whose job includes "what accounts exist here" must not
                // drop rows to make itself readable, so the split is made
                // visible instead of enforced.
                <TableRow key={u.name} className={rank(u) === 2 ? "opacity-55" : undefined}>
                  <TableCell>
                    <div className="font-mono text-xs">{u.name}</div>
                    {/* A shell that refuses a login is what makes most of
                        a passwd file uninteresting, so the rows that DO
                        accept one are marked rather than the reverse. */}
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
                  <TableCell
                    className="text-muted-foreground max-w-56 truncate text-xs"
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

      {sudoRules.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("compute.host.sudoRules")}</CardTitle>
          </CardHeader>
          <CardContent>
            {/* Verbatim, and deliberately not parsed into a table: sudo's
                grammar is real, and a half-understood rendering would give
                a confident wrong answer about who can become root. */}
            <pre className="bg-muted/50 overflow-x-auto rounded-md p-3 font-mono text-xs">
              {sudoRules.join("\n")}
            </pre>
            <p className="text-muted-foreground mt-2 text-xs">{t("compute.host.sudoRulesHint")}</p>
          </CardContent>
        </Card>
      )}

      {namedGroups.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("compute.host.accountGroups")}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("compute.host.acct.groupName")}</TableHead>
                  <TableHead className="text-right">{t("compute.host.acct.gid")}</TableHead>
                  <TableHead>{t("compute.host.acct.members")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {namedGroups.map((g) => (
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
            {emptyGroups > 0 && (
              <p className="text-muted-foreground mt-2 text-xs">
                {t("compute.host.acct.emptyGroupsHidden", { count: emptyGroups })}
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}

/** Sort key: privileged first, then anything that can log in. */
function rank(u: HostAccount): number {
  if (u.privileges?.length) return 0
  if (u.canLogin) return 1
  return 2
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
  // can be logged into with no password at all. The other three are
  // ordinary states of an ordinary machine.
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

function ReportedAt({ at }: { at?: string }) {
  const { t } = useTranslation()
  if (!at) return null
  return (
    <span className="text-muted-foreground text-xs">
      {t("compute.host.reportedAt", { at: formatDateTime(at) })}
    </span>
  )
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
