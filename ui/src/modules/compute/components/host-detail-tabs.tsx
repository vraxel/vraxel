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
function barTone(pct: number): string {
  if (pct >= hotPct) return "bg-destructive"
  if (pct >= warnPct) return "bg-warning"
  return "bg-primary"
}

/**
 * Overview: how loaded the machine is, then everything it IS.
 *
 * All 25 scalar fields on one tab, because they fit: four across on a
 * wide screen this is three cards and about one screen, and splitting
 * them earlier bought a second tab click in exchange for a panel that
 * used 349px of an 830px viewport. Tabs earn their place here only for
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
        <Field label={t("compute.host.timezone")} value={s.timezone} />
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
      </InfoCard>

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
        {physical && (
          <Field label={t("compute.host.serialNumber")} value={s.serialNumber} wide mono />
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

/** Network: the machine's own interfaces, container plumbing excluded. */
export function HostNetworkTab({ host }: { host: Host }) {
  const { t } = useTranslation()
  const nics = host.spec.nics ?? []

  if (nics.length === 0) return <EmptyFacts />

  return (
    <Card>
      <CardContent className="pt-6">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("compute.host.nic.name")}</TableHead>
              <TableHead>{t("compute.host.nic.address")}</TableHead>
              <TableHead>{t("compute.host.nic.mac")}</TableHead>
              <TableHead className="text-right">{t("compute.host.nic.speed")}</TableHead>
              <TableHead className="text-right">{t("compute.host.nic.mtu")}</TableHead>
              <TableHead>{t("compute.host.nic.state")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {nics.map((n) => (
              <TableRow key={n.name}>
                <TableCell className="font-mono text-xs">{n.name}</TableCell>
                <TableCell className="font-mono text-xs">
                  {[...(n.ipv4 ?? []), ...(n.ipv6 ?? [])].map((ip) => (
                    <div key={ip} className="truncate">
                      {ip}
                    </div>
                  ))}
                  {!n.ipv4?.length && !n.ipv6?.length && "-"}
                </TableCell>
                <TableCell className="font-mono text-xs">{n.mac || "-"}</TableCell>
                <TableCell className="text-right text-sm tabular-nums">
                  {n.speedMbps ? `${n.speedMbps} Mb/s` : "-"}
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

/** Storage: the disks the machine owns, and what is mounted off them. */
export function HostStorageTab({ host }: { host: Host }) {
  const { t } = useTranslation()
  const disks = host.spec.blockDevices ?? []
  const fs = host.spec.filesystems ?? []

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
                  <TableHead>{t("compute.host.disk.model")}</TableHead>
                  <TableHead>{t("compute.host.disk.media")}</TableHead>
                  <TableHead className="text-right">{t("compute.host.disk.size")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {disks.map((d) => (
                  <TableRow key={d.name}>
                    <TableCell className="font-mono text-xs">{d.name}</TableCell>
                    <TableCell className="text-sm">{d.model || "-"}</TableCell>
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
                </TableRow>
              </TableHeader>
              <TableBody>
                {fs.map((f) => {
                  const pct = f.sizeBytes ? ((f.usedBytes ?? 0) / f.sizeBytes) * 100 : undefined
                  return (
                    <TableRow key={f.mount}>
                      <TableCell className="font-mono text-xs">{f.mount}</TableCell>
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
                              <span className="text-muted-foreground text-xs">
                                ({Math.round(pct)}%)
                              </span>
                            )}
                          </div>
                          <Progress
                            value={pct ?? 0}
                            className="h-1.5"
                            indicatorClassName={barTone(pct ?? 0)}
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

// Says WHY the tab is empty. An agent reports its inventory once per
// session, so the gap between a host coming online and this arriving is
// real -- and a host that has never had an agent will never fill it.
function EmptyFacts() {
  const { t } = useTranslation()
  return <div className="text-muted-foreground p-6 text-sm">{t("compute.host.noFacts")}</div>
}
