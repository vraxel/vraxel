import type { ReactNode } from "react"
import { Progress } from "@/shared/ui/progress"
import { useTranslation } from "@/i18n"
import type { Host } from "@/modules/compute/api/types"

// Cells for the host list's utilisation columns, fed entirely by the
// list response itself -- the values ride the agent's heartbeat into
// one row per host, so rendering them costs no extra request and works
// across server instances.
//
// All three columns share one shape: the reading, what it is a fraction
// OF, and a gauge. The capacity is the host's spec (cores, RAM) rather
// than a column of its own, because "10%" and "8 GiB" answer one
// question together and separately answer neither -- 10% of a 4 GiB box
// and of a 512 GiB box are different facts. Disk names its mountpoint in
// that slot for the same reason: the percentage is the FULLEST real
// filesystem, not necessarily /.
//
// Staleness is judged from the snapshot's own timestamp (the AGENT's
// clock, by design: a host with a broken clock should read as stale,
// not as fresh numbers that are wrong). Values older than a few
// heartbeats grey out instead of disappearing -- the last reading of a
// machine that just went dark is exactly what the operator wants to
// see, as long as it stops looking current.
const staleAfterMs = 60_000

// Two thresholds, shared by the reading and its gauge so the colour is
// one signal rather than two that can disagree. These are fixed and
// generic on purpose: the alert rules own the per-host thresholds that
// actually page someone, and a list cell cannot cheaply know them --
// this is the "worth a second look while scanning" tier, not an alert.
const warnPct = 75
const hotPct = 90

// The tone for a live reading. Stale and unknown are handled by the
// caller: both mean "do not read this as the current colour".
function toneFor(value: number): { text: string; bar: string } {
  if (value >= hotPct) return { text: "text-destructive", bar: "bg-destructive" }
  if (value >= warnPct) return { text: "text-warning", bar: "bg-warning" }
  return { text: "", bar: "bg-primary" }
}

function isStale(sampledAt?: string): boolean {
  if (!sampledAt) return true
  return Date.now() - Date.parse(sampledAt) > staleAfterMs
}

/**
 * One utilisation gauge: reading, capacity, bar. Fixed width so the
 * three columns line up as one block the eye can scan down.
 *
 * A missing value renders as "-" over an empty rail, never as 0%: a host
 * whose agent has not reported is not a host at 0% CPU. The rail stays
 * (dimmed) so rows with and without a reading keep the same height --
 * ragged row heights were what made this column group look broken.
 */
function UtilGauge({
  value,
  capacity,
  capacityMono,
  stale,
}: {
  value?: number
  capacity: ReactNode
  capacityMono?: boolean
  stale: boolean
}) {
  const known = typeof value === "number"
  const tone = known && !stale ? toneFor(value) : null

  return (
    <div className="w-28 space-y-1">
      <div className="flex items-baseline justify-between gap-2">
        <span
          className={`text-sm tabular-nums ${
            tone ? tone.text : known ? "text-muted-foreground/60" : "text-muted-foreground"
          }`}
        >
          {known ? `${Math.round(value)}%` : "-"}
        </span>
        <span
          className={`text-muted-foreground truncate text-xs ${capacityMono ? "font-mono" : ""}`}
        >
          {capacity}
        </span>
      </div>
      <Progress
        value={known ? value : 0}
        className={known ? "h-1.5" : "bg-muted/50 h-1.5"}
        indicatorClassName={tone ? tone.bar : "bg-muted-foreground/40"}
      />
    </div>
  )
}

export function HostCpuCell({ spec }: { spec: Host["spec"] }) {
  const { t } = useTranslation()
  return (
    <UtilGauge
      value={spec.cpuUsedPct}
      capacity={spec.cpuCores ? `${spec.cpuCores} ${t("compute.host.cores")}` : "-"}
      stale={isStale(spec.metricsSampledAt)}
    />
  )
}

export function HostMemCell({ spec }: { spec: Host["spec"] }) {
  return (
    <UtilGauge
      value={spec.memUsedPct}
      capacity={spec.memoryMb ? `${Math.round(spec.memoryMb / 1024)} GiB` : "-"}
      stale={isStale(spec.metricsSampledAt)}
    />
  )
}

export function HostDiskCell({ spec }: { spec: Host["spec"] }) {
  return (
    <UtilGauge
      value={spec.diskUsedPct}
      capacity={spec.diskUsedPath || "-"}
      capacityMono
      stale={isStale(spec.metricsSampledAt)}
    />
  )
}
