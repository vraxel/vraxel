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

// Above this, the reading and its gauge turn destructive. One threshold,
// matching what the numbers used to do on their own; the alert rules own
// the per-host thresholds and a list cell cannot cheaply know them.
const hotPct = 90

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
  const hot = known && value >= hotPct

  return (
    <div className="w-28 space-y-1">
      <div className="flex items-baseline justify-between gap-2">
        <span
          className={`text-sm tabular-nums ${
            !known
              ? "text-muted-foreground"
              : stale
                ? "text-muted-foreground/60"
                : hot
                  ? "text-destructive"
                  : ""
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
        indicatorClassName={
          stale ? "bg-muted-foreground/40" : hot ? "bg-destructive" : "bg-primary"
        }
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
