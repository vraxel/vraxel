import type { Host } from "@/modules/compute/api/types"

// Cells for the host list's utilisation columns, fed entirely by the
// list response itself -- the values ride the agent's heartbeat into
// one row per host, so rendering them costs no extra request and works
// across server instances.
//
// Staleness is judged from the snapshot's own timestamp (the AGENT's
// clock, by design: a host with a broken clock should read as stale,
// not as fresh numbers that are wrong). Values older than a few
// heartbeats grey out instead of disappearing -- the last reading of a
// machine that just went dark is exactly what the operator wants to
// see, as long as it stops looking current.
const staleAfterMs = 60_000

function isStale(sampledAt?: string): boolean {
  if (!sampledAt) return true
  return Date.now() - Date.parse(sampledAt) > staleAfterMs
}

// A missing value renders as "-", never as 0: a host whose agent has
// not reported is not a host at 0% CPU.
function UtilValue({ value, stale }: { value?: number; stale: boolean }) {
  if (typeof value !== "number") {
    return <span className="text-muted-foreground text-sm">-</span>
  }
  const tone = stale ? "text-muted-foreground/60" : value >= 90 ? "text-destructive" : ""
  return <span className={`text-sm tabular-nums ${tone}`}>{Math.round(value)}%</span>
}

type Pt = { x: number; y: number }

function sparkSmooth(points: Pt[], yMin: number, yMax: number): string {
  if (points.length < 2) return ""
  const c = (v: number) => Math.min(Math.max(v, yMin), yMax)
  let d = `M ${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`
  for (let i = 0; i < points.length - 1; i++) {
    const prev = points[Math.max(0, i - 1)]
    const cur = points[i]
    const next = points[i + 1]
    const after = points[Math.min(points.length - 1, i + 2)]
    const cp1x = cur.x + (next.x - prev.x) / 6
    const cp1y = c(cur.y + (next.y - prev.y) / 6)
    const cp2x = next.x - (after.x - cur.x) / 6
    const cp2y = c(next.y - (after.y - cur.y) / 6)
    d += ` C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${next.x.toFixed(1)} ${next.y.toFixed(1)}`
  }
  return d
}

// The 24h CPU sparkline. Null buckets are gaps in the line, not zeroes:
// an agent that started an hour ago has 23 hours of honest nothing.
function CpuSparkline({ trend, stale }: { trend?: (number | undefined)[]; stale: boolean }) {
  if (!trend || trend.length < 2) return null
  const w = 72
  const h = 16
  const segments: { line: string; area: string }[] = []
  let current: Pt[] = []
  const baseline = h
  trend.forEach((v, i) => {
    if (typeof v !== "number") {
      if (current.length >= 2) {
        const line = sparkSmooth(current, 0, h)
        const first = current[0]
        const last = current[current.length - 1]
        segments.push({
          line,
          area: `${line} L ${last.x.toFixed(1)} ${baseline} L ${first.x.toFixed(1)} ${baseline} Z`,
        })
      }
      current = []
      return
    }
    const x = (i / (trend.length - 1)) * w
    const y = h - 1 - (Math.min(Math.max(v, 0), 100) / 100) * (h - 2)
    current.push({ x, y })
  })
  if (current.length >= 2) {
    const line = sparkSmooth(current, 0, h)
    const first = current[0]
    const last = current[current.length - 1]
    segments.push({
      line,
      area: `${line} L ${last.x.toFixed(1)} ${baseline} L ${first.x.toFixed(1)} ${baseline} Z`,
    })
  }
  if (segments.length === 0) return null
  return (
    <svg
      width={w}
      height={h}
      viewBox={`0 0 ${w} ${h}`}
      className={stale ? "text-muted-foreground/40" : "text-muted-foreground"}
      aria-hidden="true"
    >
      {segments.map((seg, i) => (
        <g key={i}>
          <path d={seg.area} fill="currentColor" fillOpacity="0.1" />
          <path d={seg.line} fill="none" stroke="currentColor" strokeWidth="1" />
        </g>
      ))}
    </svg>
  )
}

export function HostCpuCell({ spec }: { spec: Host["spec"] }) {
  const stale = isStale(spec.metricsSampledAt)
  return (
    <div className="flex items-center gap-2">
      <UtilValue value={spec.cpuUsedPct} stale={stale} />
      <CpuSparkline trend={spec.cpuTrend} stale={stale} />
    </div>
  )
}

export function HostMemCell({ spec }: { spec: Host["spec"] }) {
  return <UtilValue value={spec.memUsedPct} stale={isStale(spec.metricsSampledAt)} />
}

export function HostDiskCell({ spec }: { spec: Host["spec"] }) {
  const stale = isStale(spec.metricsSampledAt)
  if (typeof spec.diskUsedPct !== "number") {
    return <span className="text-muted-foreground text-sm">-</span>
  }
  // The percentage names its mountpoint because it is the FULLEST real
  // filesystem, not necessarily /; "92%" alone would send the operator
  // to the wrong disk.
  return (
    <div className="min-w-0">
      <UtilValue value={spec.diskUsedPct} stale={stale} />
      <div className="text-muted-foreground truncate font-mono text-xs">
        {spec.diskUsedPath || "-"}
      </div>
    </div>
  )
}
