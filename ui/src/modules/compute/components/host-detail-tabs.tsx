import type { ReactNode } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Badge } from "@/shared/ui/badge"
import { Progress } from "@/shared/ui/progress"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { formatDateTime } from "@/shared/lib/format"
import { useTranslation } from "@/i18n"
import {
  HostCpuCell,
  HostDiskCell,
  HostMemCell,
} from "@/modules/compute/components/host-metrics-cells"
import type { Host } from "@/modules/compute/api/types"

// The detail page's data tabs. Split out of detail.tsx, which owns the
// header, the banners and the dialogs; these are pure views over one
// host object and take no callbacks.

/** One label/value pair. Empty renders "-" rather than collapsing, so a
 *  field that has not been reported keeps its place and its label.
 *
 *  wide takes two of the four columns, for values that would truncate in
 *  one (a CPU model, a UUID); full takes the row, for free text. */
function Field({
  label,
  value,
  mono,
  wide,
  full,
}: {
  label: string
  value?: ReactNode
  mono?: boolean
  wide?: boolean
  full?: boolean
}) {
  const empty = value === undefined || value === null || value === ""
  const span = full ? "col-span-2 lg:col-span-4" : wide ? "col-span-2" : ""
  return (
    <div className={`min-w-0 ${span}`}>
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd
        className={`truncate ${mono ? "font-mono text-xs" : "text-sm"}`}
        title={typeof value === "string" ? value : undefined}
      >
        {empty ? "-" : value}
      </dd>
    </div>
  )
}

/**
 * A full-width card of label/value pairs, four across on a wide screen.
 *
 * Full width and stacked rather than two cards side by side, which is
 * what this page did first: a CSS grid gives its cells a common row
 * height, so the shorter card inherits the taller one's and the
 * difference becomes dead space inside it (measured at 148px between an
 * 11-field card and a 5-field one). Stacking removes the coupling
 * entirely -- each card is as tall as its own content, whatever gets
 * added to its neighbour later -- and four columns is what pays for the
 * width, since these values are "amd64", "4", "6.00".
 */
function InfoCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-3 lg:grid-cols-4">{children}</dl>
      </CardContent>
    </Card>
  )
}

// Byte formatting for inventory figures, which span a CD-ROM and a
// 20 TiB array. Binary units throughout, matching df/free and the rest of
// the platform.
const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"]
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

// Reuses the list's thresholds so a filesystem that reads "warning" here
// is the one that would tint the list's disk gauge.
const warnPct = 75
const hotPct = 90

// Text only -- bars are magnitude, never status (see shared/ui/progress).
function textTone(pct: number): string | undefined {
  if (pct >= hotPct) return "text-destructive"
  if (pct >= warnPct) return "text-warning"
  return undefined
}

/**
 * Overview: how loaded the machine is, then everything it IS.
 *
 * Every scalar field on one tab, because they fit: four across on a
 * wide screen this is three cards and about one screen, and splitting
 * them earlier bought a second tab click in exchange for a panel that
 * used 349px of an 830px viewport. The asset fields below are gated on
 * bare metal, so a guest's card is shorter still. Tabs earn their place
 * here only for
 * the inventory TABLES, which a real server can fill with a dozen disks
 * and twenty mounts, and for the metrics panel, which is expensive to
 * mount.
 */
export function HostOverviewTab({ host }: { host: Host }) {
  const { t } = useTranslation()
  const s = host.spec

  // 4 sockets x 1 core x 1 thread is a different machine from 1 x 4 x 1,
  // and cpuCores (a count of logical CPUs) cannot tell them apart. Shown
  // as the product so the arithmetic is visible rather than asserted.
  const topology =
    s.cpuSockets && s.cpuCoresPerSocket
      ? t("compute.host.cpuTopologyValue", {
          sockets: s.cpuSockets,
          cores: s.cpuCoresPerSocket,
          threads: s.cpuThreadsPerCore || 1,
        })
      : undefined

  // A serial is an asset identifier on real hardware. On a guest it is a
  // re-encoding of the SMBIOS UUID that identifies nothing an operator
  // can look up, so it is hidden rather than shown as a plausible-looking
  // asset tag.
  const physical = s.virtualization === "physical"

  return (
    <div className="space-y-4">
      {/* The same three gauges the list sorts on, at the top of the page
          an operator opened to answer "how is this host doing". Same
          component as the list cells, so the reading and its colour
          cannot drift between the two views. */}
      <div className="grid gap-4 sm:grid-cols-3">
        <GaugeCard label={t("compute.host.cpu")}>
          <HostCpuCell spec={s} wide />
        </GaugeCard>
        <GaugeCard label={t("compute.host.memory")}>
          <HostMemCell spec={s} wide />
        </GaugeCard>
        <GaugeCard label={t("compute.host.disk")}>
          <HostDiskCell spec={s} wide />
        </GaugeCard>
      </div>

      <InfoCard title={t("compute.host.basicInfo")}>
        <Field label={t("compute.host.ip")} value={s.reportedPrimaryIp} mono />
        <Field label={t("compute.host.hostname")} value={s.hostname} />
        <Field label={t("compute.host.os")} value={s.os} wide />
        <Field label={t("compute.host.kernel")} value={s.kernelVersion} mono />
        <Field label={t("compute.host.arch")} value={s.arch} />
        {/* Where on the network this machine sits, next to the address it
            sits at. osId / osVersionId are deliberately not shown: they
            are the same facts as os, split so a query can compare them,
            and a row reading "debian" under one reading "Debian GNU/Linux
            13" is a second spelling with nothing new in it. */}
        <Field label={t("compute.host.defaultGateway")} value={s.defaultGateway} mono />
        {/* Beside the timezone, because they are the two halves of "what
            time does this machine think it is" -- and a clock nobody
            disciplined makes every other reading on this page unreliable,
            including our own judgement of whether they are fresh. */}
        <Field label={t("compute.host.timezone")} value={s.timezone} />
        <Field
          label={t("compute.host.clockSync")}
          value={s.clockSync ? <ClockSyncBadge value={s.clockSync} /> : undefined}
        />
        <Field
          label={t("compute.host.origin")}
          value={
            s.origin === "agent" ? t("compute.host.originAgent") : t("compute.host.originManual")
          }
        />
        <Field label={t("compute.host.bootAt")} value={formatDateTime(s.bootAt)} />
        <Field label={t("common.createdBy")} value={s.createdByName} />
        <Field label={t("common.created")} value={formatDateTime(host.metadata.createdAt)} />
        <Field label={t("common.description")} value={s.description} full />
        {/* What an ssh client pins. Shown because the event worth catching
            is these CHANGING: a rebuilt or replaced machine has new ones,
            and every operator's client refuses to connect until somebody
            decides that was expected. */}
        {(s.sshHostKeys?.length ?? 0) > 0 && (
          <Field
            label={t("compute.host.sshHostKeys")}
            full
            value={
              <div className="space-y-0.5">
                {s.sshHostKeys?.map((k) => (
                  <div key={k.fingerprint} className="truncate font-mono text-xs">
                    <span className="text-muted-foreground">{k.type}</span> {k.fingerprint}
                  </div>
                ))}
              </div>
            }
          />
        )}
      </InfoCard>

      {(s.kernelCmdline || (s.cpuMitigations?.length ?? 0) > 0) && (
        <InfoCard title={t("compute.host.kernelSection")}>
          <Field label={t("compute.host.kernelCmdline")} value={s.kernelCmdline} mono full />
          {(s.cpuMitigations?.length ?? 0) > 0 && (
            <Field
              label={t("compute.host.cpuMitigations")}
              full
              value={<Mitigations items={s.cpuMitigations ?? []} />}
            />
          )}
        </InfoCard>
      )}

      <InfoCard title={t("compute.host.hardware")}>
        <Field label={t("compute.host.cpuModel")} value={s.cpuModel} wide />
        <Field
          label={t("compute.host.logicalCpus")}
          value={s.cpuCores ? `${s.cpuCores}` : undefined}
        />
        <Field label={t("compute.host.cpuTopology")} value={topology} />
        <Field
          label={t("compute.host.memoryTotal")}
          value={s.memoryMb ? bytes(s.memoryMb * 1024 * 1024) : undefined}
        />
        <Field
          label={t("compute.host.virtualization")}
          value={s.virtualization ? <VirtBadge value={s.virtualization} /> : undefined}
        />
        <Field label={t("compute.host.systemVendor")} value={s.systemVendor} />
        <Field label={t("compute.host.productName")} value={s.productName} />
        <Field label={t("compute.host.biosVersion")} value={s.biosVersion} />
        <Field label={t("compute.host.biosDate")} value={s.biosDate} />
        <Field label={t("compute.host.boardName")} value={s.boardName} wide />
        {/* The asset-management fields, and they only mean anything on
            hardware. A guest's serial re-encodes its SMBIOS UUID, its
            chassis type is whatever "Other" maps to, and its board serial
            belongs to a board that does not exist -- three plausible
            looking values that identify nothing an operator can look up. */}
        {physical && (
          <>
            <Field label={t("compute.host.serialNumber")} value={s.serialNumber} wide mono />
            <Field label={t("compute.host.assetTag")} value={s.assetTag} mono />
            <Field
              label={t("compute.host.chassisType")}
              value={s.chassisType ? <ChassisLabel value={s.chassisType} /> : undefined}
            />
            <Field label={t("compute.host.boardSerial")} value={s.boardSerial} wide mono />
          </>
        )}
      </InfoCard>

      <InfoCard title={t("compute.host.agentSession")}>
        <Field label={t("compute.host.agentVersion")} value={s.agentVersion} />
        <Field label={t("compute.host.agentId")} value={s.agentId} wide mono />
        <Field label={t("compute.host.connectedAt")} value={formatDateTime(s.agentConnectedAt)} />
        <Field label={t("compute.host.lastSeenAt")} value={formatDateTime(s.agentLastSeenAt)} />
        <Field
          label={t("compute.host.factsReportedAt")}
          value={formatDateTime(s.factsReportedAt)}
        />
      </InfoCard>
    </div>
  )
}

function GaugeCard({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Card>
      <CardContent className="space-y-2 p-4">
        <p className="text-muted-foreground text-sm">{label}</p>
        {children}
      </CardContent>
    </Card>
  )
}

function ClockSyncBadge({ value }: { value: string }) {
  const { t } = useTranslation()
  if (value !== "unsynced") {
    return <Badge variant="secondary">{t("compute.host.clockSynced")}</Badge>
  }
  // Destructive because it invalidates readings elsewhere on this page
  // rather than being a fact about the machine's hardware.
  return <Badge variant="destructive">{t("compute.host.clockUnsynced")}</Badge>
}

/**
 * The kernel's verdict on each hardware vulnerability it tracks.
 *
 * Summarised, not listed: seventeen lines of kernel prose is the whole
 * card on a host where the answer is "nothing to do". The entries that
 * are NOT "Not affected" are the ones with something to say, so those are
 * printed and the rest become a count.
 *
 * Statuses are shown verbatim. "Mitigation: PTE Inversion", "Vulnerable"
 * and "Unknown: Dependent on hypervisor status" are three different
 * situations, and the third one -- a guest that cannot see its host's
 * microcode -- is exactly the case a normalised badge would turn into a
 * claim it has no basis for.
 */
function Mitigations({ items }: { items: { name: string; status: string }[] }) {
  const { t } = useTranslation()
  const notAffected = items.filter((m) => m.status === "Not affected")
  const rest = items.filter((m) => m.status !== "Not affected")
  const vulnerable = rest.filter((m) => m.status.startsWith("Vulnerable"))

  return (
    <div className="space-y-1">
      <div className="text-xs">
        {vulnerable.length > 0 ? (
          <span className="text-destructive font-medium">
            {t("compute.host.mitigationVulnerable", { count: vulnerable.length })}
          </span>
        ) : (
          <span className="text-muted-foreground">
            {t("compute.host.mitigationClear", { count: notAffected.length })}
          </span>
        )}
      </div>
      {rest.map((m) => (
        <div key={m.name} className="flex min-w-0 gap-2 font-mono text-xs">
          <span className="text-muted-foreground w-52 shrink-0 truncate">{m.name}</span>
          <span
            className={`truncate ${m.status.startsWith("Vulnerable") ? "text-destructive" : ""}`}
            title={m.status}
          >
            {m.status}
          </span>
        </div>
      ))}
    </div>
  )
}

function VirtBadge({ value }: { value: string }) {
  const { t } = useTranslation()
  // Spelled out rather than built as `compute.host.virt.${value}`: the
  // key type is a union of the literal keys, so an interpolated one only
  // compiles behind a cast -- which is the compiler being asked to stop
  // checking exactly where a value from the wire meets a key set.
  const labels: Record<string, string> = {
    physical: t("compute.host.virt.physical"),
    vmware: t("compute.host.virt.vmware"),
    kvm: t("compute.host.virt.kvm"),
    qemu: t("compute.host.virt.qemu"),
    xen: t("compute.host.virt.xen"),
    hyperv: t("compute.host.virt.hyperv"),
    virtualbox: t("compute.host.virt.virtualbox"),
    parallels: t("compute.host.virt.parallels"),
    bhyve: t("compute.host.virt.bhyve"),
    unknown: t("compute.host.virt.unknown"),
  }
  // Bare metal is the noteworthy state in a fleet that is mostly guests,
  // and "unknown" means the detection saw a hypervisor but could not name
  // it -- both worth a glance, so neither gets the quiet variant.
  const variant =
    value === "physical" ? "default" : value === "unknown" ? "destructive" : "secondary"
  return <Badge variant={variant}>{labels[value] ?? value}</Badge>
}

// Spelled out for the same reason as VirtBadge's map: an interpolated key
// only compiles behind a cast, and a cast here is the compiler being told
// to stop checking exactly where a wire value meets a key set.
function ChassisLabel({ value }: { value: string }) {
  const { t } = useTranslation()
  const labels: Record<string, string> = {
    desktop: t("compute.host.chassis.desktop"),
    tower: t("compute.host.chassis.tower"),
    laptop: t("compute.host.chassis.laptop"),
    server: t("compute.host.chassis.server"),
    rack: t("compute.host.chassis.rack"),
    blade: t("compute.host.chassis.blade"),
  }
  return <>{labels[value] ?? value}</>
}

/** Network: the machine's own interfaces, container plumbing excluded. */
export function HostNetworkTab({ host }: { host: Host }) {
  const { t } = useTranslation()
  const nics = host.spec.nics ?? []
  const dns = host.spec.dns

  if (nics.length === 0 && !dns) return <EmptyFacts />

  return (
    <div className="space-y-4">
      {/* Above the interfaces, not below: a host that resolves differently
          from its neighbours produces an outage that looks like an
          application fault for hours, and the addresses on the card below
          are no help in finding it. */}
      {dns && (
        <InfoCard title={t("compute.host.dns")}>
          <Field label={t("compute.host.dnsServers")} value={dns.servers?.join(", ")} mono wide />
          <Field label={t("compute.host.dnsSearch")} value={dns.search?.join(", ")} mono wide />
        </InfoCard>
      )}
      {nics.length > 0 && <NICTable host={host} />}
    </div>
  )
}

function NICTable({ host }: { host: Host }) {
  const { t } = useTranslation()
  const nics = host.spec.nics ?? []
  return (
    <Card>
      <CardContent className="pt-6">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("compute.host.nic.name")}</TableHead>
              <TableHead>{t("compute.host.nic.address")}</TableHead>
              <TableHead>{t("compute.host.nic.mac")}</TableHead>
              <TableHead>{t("compute.host.nic.driver")}</TableHead>
              <TableHead className="text-right">{t("compute.host.nic.speed")}</TableHead>
              <TableHead className="text-right">{t("compute.host.nic.mtu")}</TableHead>
              <TableHead>{t("compute.host.nic.state")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {nics.map((n) => (
              <TableRow key={n.name}>
                <TableCell>
                  <div className="font-mono text-xs">{n.name}</div>
                  {/* The kind badge only when it is NOT a physical port: on
                      an ordinary machine every row would carry the same
                      word, and the whole reason these rows exist is that
                      bond / bridge / vlan are the unusual ones. */}
                  {n.kind && n.kind !== "physical" && (
                    <Badge variant="secondary" className="mt-1">
                      <NicKindLabel value={n.kind} />
                    </Badge>
                  )}
                  {n.master && (
                    <div className="text-muted-foreground mt-1 text-xs">
                      {t("compute.host.nic.enslavedTo", { master: n.master })}
                    </div>
                  )}
                </TableCell>
                <TableCell className="font-mono text-xs">
                  {[...(n.ipv4 ?? []), ...(n.ipv6 ?? [])].map((ip) => (
                    <div key={ip} className="truncate">
                      {ip}
                    </div>
                  ))}
                  {!n.ipv4?.length && !n.ipv6?.length && "-"}
                </TableCell>
                <TableCell className="font-mono text-xs">{n.mac || "-"}</TableCell>
                <TableCell className="text-muted-foreground font-mono text-xs">
                  {n.driver || "-"}
                </TableCell>
                <TableCell className="text-right">
                  <div className="text-sm tabular-nums">
                    {n.speedMbps ? `${n.speedMbps} Mb/s` : "-"}
                  </div>
                  {/* Half duplex on a server link is a negotiation failure,
                      so it is called out; full is the expected case and is
                      left quiet. */}
                  {n.duplex === "half" && (
                    <div className="text-warning text-xs">{t("compute.host.nic.duplexHalf")}</div>
                  )}
                  {n.duplex === "full" && (
                    <div className="text-muted-foreground text-xs">
                      {t("compute.host.nic.duplexFull")}
                    </div>
                  )}
                </TableCell>
                <TableCell className="text-right text-sm tabular-nums">{n.mtu || "-"}</TableCell>
                <TableCell>
                  <Badge variant={n.state === "up" ? "outline" : "secondary"}>
                    {n.state || "-"}
                  </Badge>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function NicKindLabel({ value }: { value: string }) {
  const { t } = useTranslation()
  const labels: Record<string, string> = {
    bond: t("compute.host.nic.kind.bond"),
    bridge: t("compute.host.nic.kind.bridge"),
    vlan: t("compute.host.nic.kind.vlan"),
  }
  return <>{labels[value] ?? value}</>
}

// Inode counts, which run to millions and are not bytes. Decimal steps,
// because an inode table is a count and nobody thinks of it in 1024s.
function count(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${Math.round(n / 1_000)}K`
  return `${n}`
}

/** Storage: the disks the machine owns, and what is mounted off them. */
export function HostStorageTab({ host }: { host: Host }) {
  const { t } = useTranslation()
  const disks = host.spec.blockDevices ?? []
  const fs = host.spec.filesystems ?? []
  const swaps = host.spec.swaps ?? []

  if (disks.length === 0 && fs.length === 0) return <EmptyFacts />

  return (
    <div className="space-y-4">
      {disks.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("compute.host.blockDevices")}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("compute.host.disk.name")}</TableHead>
                  <TableHead>{t("compute.host.disk.vendor")}</TableHead>
                  <TableHead>{t("compute.host.disk.model")}</TableHead>
                  <TableHead>{t("compute.host.disk.serial")}</TableHead>
                  <TableHead>{t("compute.host.disk.media")}</TableHead>
                  <TableHead className="text-right">{t("compute.host.disk.size")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {disks.map((d) => (
                  <TableRow key={d.name}>
                    <TableCell className="font-mono text-xs">{d.name}</TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {d.vendor || "-"}
                    </TableCell>
                    <TableCell className="text-sm">{d.model || "-"}</TableCell>
                    {/* A SCSI wwid runs to 40-odd characters and is read
                        by matching, not by remembering, so it truncates
                        with the whole value on hover. */}
                    <TableCell
                      className="text-muted-foreground max-w-40 truncate font-mono text-xs"
                      title={d.serial}
                    >
                      {d.serial || "-"}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {t(d.rotational ? "compute.host.disk.hdd" : "compute.host.disk.ssd")}
                    </TableCell>
                    <TableCell className="text-right text-sm tabular-nums">
                      {bytes(d.sizeBytes)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      {fs.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("compute.host.filesystems")}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("compute.host.fs.mount")}</TableHead>
                  <TableHead>{t("compute.host.fs.device")}</TableHead>
                  <TableHead>{t("compute.host.fs.type")}</TableHead>
                  <TableHead className="w-48">{t("compute.host.fs.usage")}</TableHead>
                  <TableHead className="text-right">{t("compute.host.fs.inodes")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {fs.map((f) => {
                  const pct = f.sizeBytes ? ((f.usedBytes ?? 0) / f.sizeBytes) * 100 : undefined
                  // btrfs and friends allocate inodes on demand and report
                  // no total, which is "no ceiling" rather than "none
                  // left" -- so the cell says nothing instead of 100%.
                  const inodePct = f.inodesTotal
                    ? ((f.inodesUsed ?? 0) / f.inodesTotal) * 100
                    : undefined
                  return (
                    <TableRow key={f.mount}>
                      <TableCell>
                        <div className="font-mono text-xs">{f.mount}</div>
                        {/* Not a setting. ext4 and xfs mount with
                            errors=remount-ro, so a local filesystem that
                            has gone read-only is a disk the kernel gave up
                            on with services still writing to it. */}
                        {f.readOnly && (
                          <Badge
                            variant="destructive"
                            className="mt-1"
                            title={t("compute.host.fs.readOnlyHint")}
                          >
                            {t("compute.host.fs.readOnly")}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="font-mono text-xs">{f.device || "-"}</TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {f.fstype || "-"}
                      </TableCell>
                      <TableCell>
                        <div className="space-y-1">
                          <div className="flex items-baseline gap-1 text-sm tabular-nums">
                            <span>
                              {bytes(f.usedBytes)} / {bytes(f.sizeBytes)}
                            </span>
                            {pct !== undefined && (
                              <span
                                className={`text-xs ${textTone(pct) ?? "text-muted-foreground"}`}
                              >
                                ({Math.round(pct)}%)
                              </span>
                            )}
                          </div>
                          {/* An unreadable size (a statfs that failed) is not
                              an empty filesystem. Drawn as a dimmed rail
                              rather than a full-colour 0% bar, which reads as
                              "this mount is empty" -- the same distinction the
                              list's gauge makes. */}
                          <Progress
                            value={pct ?? 0}
                            className={pct === undefined ? "bg-muted/50 h-1.5" : "h-1.5"}
                          />
                        </div>
                      </TableCell>
                      {/* Text, not a second bar: running out of inodes is
                          the rare failure, so it needs to be readable when
                          somebody looks and to shout when it matters,
                          which colour does without another row of height. */}
                      <TableCell className="text-right text-sm tabular-nums">
                        {inodePct === undefined ? (
                          "-"
                        ) : (
                          <>
                            <div className={textTone(inodePct)}>
                              {Math.round(inodePct)}%
                            </div>
                            <div className="text-muted-foreground text-xs">
                              {count(f.inodesUsed ?? 0)} / {count(f.inodesTotal ?? 0)}
                            </div>
                          </>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      {/* Configuration only. How much swap is in use is a metric -- it
          changes every sample, and this inventory is sent only when its
          content changes, so carrying usage here would defeat that. */}
      {swaps.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("compute.host.swap")}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("compute.host.swap.device")}</TableHead>
                  <TableHead>{t("compute.host.swap.kind")}</TableHead>
                  <TableHead className="text-right">{t("compute.host.swap.size")}</TableHead>
                  {/* Equal priorities stripe, different ones are a
                      fallback chain: the same list of devices means two
                      different things without this column. */}
                  <TableHead className="text-right">{t("compute.host.swap.priority")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {swaps.map((sw) => (
                  <TableRow key={sw.device}>
                    <TableCell className="font-mono text-xs">{sw.device}</TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {sw.kind || "-"}
                    </TableCell>
                    <TableCell className="text-right text-sm tabular-nums">
                      {bytes(sw.sizeBytes)}
                    </TableCell>
                    <TableCell className="text-right text-sm tabular-nums">
                      {sw.priority ?? "-"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

// Says WHY the tab is empty, and names the case waiting does not fix.
//
// An agent older than this feature connects, heartbeats and looks
// perfectly healthy while never sending an inventory -- so "the agent
// will report it once it connects" would be a promise the page cannot
// keep, and an operator would wait on a host that is already doing
// everything it is going to do.
function EmptyFacts() {
  const { t } = useTranslation()
  return <div className="text-muted-foreground p-6 text-sm">{t("compute.host.noFacts")}</div>
}
