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
 * One utilisation gauge: the amount, the percentage it works out to, and
 * a bar. Fixed width so the three columns line up as one block the eye
 * can scan down.
 *
 * The amount leads because it is the answer to "how big is this host",
 * which a percentage alone cannot give: 90% of 2 GiB and 90% of 512 GiB
 * are different problems. The percentage is the signal, so it carries
 * the colour; the amount stays neutral, being an inventory fact.
 *
 * A host with no reading keeps its amount and drops the percentage, over
 * a dimmed rail -- never a 0% that would read as an idle machine. The
 * rail stays so rows with and without a reading keep the same height;
 * ragged row heights were what made this column group look broken.
 */
function UtilGauge({
  amount,
  value,
  stale,
}: {
  /** Omitted when nothing can honestly be paired with the percentage,
   *  which then becomes the primary text instead of a suffix. */
  amount?: ReactNode
  value?: number
  stale: boolean
}) {
  const known = typeof value === "number"
  const tone = known && !stale ? toneFor(value) : null
  const pctTone = tone ? tone.text : "text-muted-foreground/60"

  return (
    <div className="w-36 space-y-1">
      <div className="flex items-baseline gap-1">
        {amount === undefined ? (
          <span className={`text-sm tabular-nums ${known ? pctTone : "text-muted-foreground"}`}>
            {known ? `${Math.round(value)}%` : "-"}
          </span>
        ) : (
          <>
            <span className="truncate text-sm tabular-nums">{amount}</span>
            {known && (
              <span className={`text-xs tabular-nums ${pctTone}`}>({Math.round(value)}%)</span>
            )}
          </>
        )}
      </div>
      <Progress
        value={known ? value : 0}
        className={known ? "h-1.5" : "bg-muted/50 h-1.5"}
        indicatorClassName={tone ? tone.bar : "bg-muted-foreground/40"}
      />
    </div>
  )
}

// GiB with one decimal. Not fmtStorageBytes: that switches to MiB below
// 1 GiB, and a used/total pair whose halves carry different units reads
// as two unrelated numbers.
const gib = (mb: number) => (mb / 1024).toFixed(1)
const gibBytes = (b: number) => (b / (1024 * 1024 * 1024)).toFixed(1)

export function HostCpuCell({ spec }: { spec: Host["spec"] }) {
  const { t } = useTranslation()
  // Cores, not "cores in use": a percentage of a core is a number no
  // operator acts on, and the count is what makes the percentage mean
  // something.
  return (
    <UtilGauge
      amount={spec.cpuCores ? `${spec.cpuCores} ${t("compute.host.cores")}` : "-"}
      value={spec.cpuUsedPct}
      stale={isStale(spec.metricsSampledAt)}
    />
  )
}

export function HostMemCell({ spec }: { spec: Host["spec"] }) {
  const total = spec.memoryMb
  const pct = spec.memUsedPct
  // Used is derived, not reported: memUsedPct is 1 - MemAvailable/MemTotal
  // and memoryMb is that same MemTotal from registration, so the product
  // is the used figure rather than an estimate of one.
  const amount = !total
    ? "-"
    : typeof pct === "number"
      ? `${gib((total * pct) / 100)} / ${gib(total)} GiB`
      : `${gib(total)} GiB`
  return <UtilGauge amount={amount} value={pct} stale={isStale(spec.metricsSampledAt)} />
}

export function HostDiskCell({ spec }: { spec: Host["spec"] }) {
  const used = spec.diskUsedBytes
  const total = spec.diskTotalBytes
  const stale = isStale(spec.metricsSampledAt)
  // The agent sums used/total across the same real filesystems that
  // decide diskUsedPct (each device counted once), so with the pair in
  // hand the percentage shown is the host's overall disk pressure rather
  // than its single fullest mount.
  //
  // Without it, fall back to diskUsedPct. An agent older than those two
  // fields still reports that one, and it is the whole disk column for
  // such a host -- dropping it left the cell reading "-" on a machine
  // that was reporting its disk perfectly well.
  const hasPair = typeof used === "number" && typeof total === "number" && total > 0
  const pct = hasPair ? (used / total) * 100 : spec.diskUsedPct
  const amount = hasPair ? `${gibBytes(used)} / ${gibBytes(total)} GiB` : undefined
  return <UtilGauge amount={amount} value={pct} stale={stale} />
}
