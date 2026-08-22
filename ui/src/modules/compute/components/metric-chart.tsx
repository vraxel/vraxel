import { useId, useMemo, useRef, useState } from "react"
import { useAnimatedSeries } from "@/modules/compute/use-animated-series"

// A line chart over the fixed grid HostMetrics answers with. Hand-rolled
// SVG with cubic Bezier smoothing (Catmull-Rom control points, same
// algorithm as OpenSurge/trafficChart.ts) and gradient area fill.
//
// Null values are gaps in the line, never zeroes: a bucket the agent
// holds nothing for (it was down, the disk was not mounted yet) must not
// draw as an idle machine.

export interface ChartSeries {
  key: string
  label: string
  values: (number | null | undefined)[]
}

const PALETTE = ["#0ea5e9", "#10b981", "#f59e0b", "#8b5cf6", "#f43f5e", "#06b6d4"]

const W = 600
const H = 160
const PAD_L = 44
const PAD_R = 8
const PAD_T = 8
const PAD_B = 20

export type ChartUnit = "pct" | "bps" | "plain"

function formatUnit(v: number, unit: ChartUnit): string {
  switch (unit) {
    case "pct":
      return `${v >= 10 ? Math.round(v) : v.toFixed(1)}%`
    case "bps": {
      const units = ["B/s", "KB/s", "MB/s", "GB/s"]
      let x = v
      let i = 0
      while (x >= 1024 && i < units.length - 1) {
        x /= 1024
        i++
      }
      return `${x >= 10 || i === 0 ? Math.round(x) : x.toFixed(1)} ${units[i]}`
    }
    default:
      return v >= 10 ? String(Math.round(v)) : v.toFixed(2)
  }
}

function niceMax(series: ChartSeries[], unit: ChartUnit): number {
  if (unit === "pct") return 100
  let max = 0
  for (const s of series) {
    for (const v of s.values) {
      if (typeof v === "number" && v > max) max = v
    }
  }
  return max > 0 ? max * 1.15 : 1
}

// --- Smooth path generation (Catmull-Rom cubic Bezier) ---

type Pt = { x: number; y: number }

function fmt(n: number): string {
  return n.toFixed(1)
}

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(Math.max(v, lo), hi)
}

function smoothPath(points: Pt[], yMin: number, yMax: number): string {
  if (points.length === 0) return ""
  if (points.length === 1) return `M ${fmt(points[0].x)} ${fmt(points[0].y)}`
  let d = `M ${fmt(points[0].x)} ${fmt(points[0].y)}`
  for (let i = 0; i < points.length - 1; i++) {
    const prev = points[Math.max(0, i - 1)]
    const cur = points[i]
    const next = points[i + 1]
    const after = points[Math.min(points.length - 1, i + 2)]
    const cp1x = cur.x + (next.x - prev.x) / 6
    const cp1y = clamp(cur.y + (next.y - prev.y) / 6, yMin, yMax)
    const cp2x = next.x - (after.x - cur.x) / 6
    const cp2y = clamp(next.y - (after.y - cur.y) / 6, yMin, yMax)
    d += ` C ${fmt(cp1x)} ${fmt(cp1y)}, ${fmt(cp2x)} ${fmt(cp2y)}, ${fmt(next.x)} ${fmt(next.y)}`
  }
  return d
}

function buildSegmentPaths(
  points: Pt[],
  baseline: number,
  yMin: number,
  yMax: number,
): { linePath: string; areaPath: string } {
  const linePath = smoothPath(points, yMin, yMax)
  if (points.length < 2) return { linePath, areaPath: "" }
  const first = points[0]
  const last = points[points.length - 1]
  const areaPath = `${linePath} L ${fmt(last.x)} ${fmt(baseline)} L ${fmt(first.x)} ${fmt(baseline)} Z`
  return { linePath, areaPath }
}

export function MetricChart({
  title,
  unit,
  series,
  fromMs,
  stepSec,
  count,
  emptyText,
}: {
  title: string
  unit: ChartUnit
  series: ChartSeries[]
  fromMs: number
  stepSec: number
  count: number
  emptyText: string
}) {
  const boxRef = useRef<HTMLDivElement>(null)
  const [hover, setHover] = useState<number | null>(null)
  const gradientId = useId().replace(/:/g, "")

  // Flatten all series values into one array for a single animation
  // hook call (hooks cannot be called inside a loop), then slice back.
  const flat = useMemo(() => series.flatMap((s) => s.values), [series])
  const animKey = `${fromMs}:${count}:${series.map((s) => s.key).join(",")}`
  const animatedFlat = useAnimatedSeries(flat, animKey)
  const animatedSeries = useMemo(() => {
    const result: ChartSeries[] = []
    let offset = 0
    for (const s of series) {
      result.push({ ...s, values: animatedFlat.slice(offset, offset + s.values.length) })
      offset += s.values.length
    }
    return result
  }, [series, animatedFlat])

  const innerW = W - PAD_L - PAD_R
  const innerH = H - PAD_T - PAD_B
  const max = niceMax(animatedSeries, unit)
  const xAt = (i: number) => PAD_L + (count > 1 ? (i / (count - 1)) * innerW : 0)
  const yAt = (v: number) => PAD_T + innerH - (Math.min(v, max) / max) * innerH
  const baseline = PAD_T + innerH

  const seriesPaths = animatedSeries.map((s) => {
    const segments: { linePath: string; areaPath: string }[] = []
    let current: Pt[] = []
    s.values.forEach((v, i) => {
      if (typeof v !== "number") {
        if (current.length >= 2)
          segments.push(buildSegmentPaths(current, baseline, PAD_T, baseline))
        current = []
        return
      }
      current.push({ x: xAt(i), y: yAt(v) })
    })
    if (current.length >= 2) segments.push(buildSegmentPaths(current, baseline, PAD_T, baseline))
    return segments
  })
  const hasData = seriesPaths.some((p) => p.length > 0)

  const timeAt = (i: number) => new Date(fromMs + i * stepSec * 1000)
  const timeLabel = (i: number) =>
    timeAt(i).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })

  const onMove = (e: React.MouseEvent) => {
    const box = boxRef.current?.getBoundingClientRect()
    if (!box || count < 2) return
    const fx = ((e.clientX - box.left) / box.width) * W
    const i = Math.round(((fx - PAD_L) / innerW) * (count - 1))
    setHover(i >= 0 && i < count ? i : null)
  }

  const gridYs = [0, 0.5, 1]

  return (
    <div className="rounded-lg border p-3">
      <div className="mb-1 flex items-baseline justify-between">
        <span className="text-sm font-medium">{title}</span>
        {series.length > 1 && (
          <span className="flex flex-wrap justify-end gap-x-3 text-xs">
            {series.map((s, i) => (
              <span key={s.key} className="text-muted-foreground inline-flex items-center gap-1">
                <span
                  className="inline-block h-0.5 w-3 rounded-full"
                  style={{ backgroundColor: PALETTE[i % PALETTE.length] }}
                />
                {s.label}
              </span>
            ))}
          </span>
        )}
      </div>

      <div
        ref={boxRef}
        className="relative"
        onMouseMove={onMove}
        onMouseLeave={() => setHover(null)}
      >
        <svg viewBox={`0 0 ${W} ${H}`} className="block w-full" role="img" aria-label={title}>
          <defs>
            {series.map((s, i) => (
              <linearGradient key={s.key} id={`${gradientId}-${i}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0" stopColor={PALETTE[i % PALETTE.length]} stopOpacity="0.2" />
                <stop offset="1" stopColor={PALETTE[i % PALETTE.length]} stopOpacity="0" />
              </linearGradient>
            ))}
          </defs>

          {gridYs.map((g) => {
            const gy = PAD_T + innerH - g * innerH
            return (
              <g key={g} className="text-muted-foreground/60">
                <line
                  x1={PAD_L}
                  x2={W - PAD_R}
                  y1={gy}
                  y2={gy}
                  stroke="currentColor"
                  strokeOpacity="0.25"
                  strokeDasharray="3 3"
                />
                <text x={PAD_L - 6} y={gy + 3} textAnchor="end" fontSize="9" fill="currentColor">
                  {formatUnit(g * max, unit)}
                </text>
              </g>
            )
          })}
          {count > 1 &&
            [0, 0.5, 1].map((g) => (
              <text
                key={g}
                x={PAD_L + g * innerW}
                y={H - 6}
                textAnchor={g === 0 ? "start" : g === 1 ? "end" : "middle"}
                fontSize="9"
                fill="currentColor"
                className="text-muted-foreground/60"
              >
                {timeLabel(Math.round(g * (count - 1)))}
              </text>
            ))}

          {seriesPaths.map((segments, i) =>
            segments.map(
              (seg, j) =>
                seg.areaPath && (
                  <path
                    key={`area-${series[i].key}-${j}`}
                    d={seg.areaPath}
                    fill={`url(#${gradientId}-${i})`}
                  />
                ),
            ),
          )}

          {seriesPaths.map((segments, i) =>
            segments.map((seg, j) => (
              <path
                key={`line-${series[i].key}-${j}`}
                d={seg.linePath}
                fill="none"
                stroke={PALETTE[i % PALETTE.length]}
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
                vectorEffect="non-scaling-stroke"
              />
            )),
          )}

          {hover !== null && hasData && (
            <line
              x1={xAt(hover)}
              x2={xAt(hover)}
              y1={PAD_T}
              y2={baseline}
              stroke="currentColor"
              strokeOpacity="0.35"
            />
          )}
        </svg>

        {!hasData && (
          <div className="text-muted-foreground absolute inset-0 flex items-center justify-center text-xs">
            {emptyText}
          </div>
        )}

        {hover !== null && hasData && (
          <div
            className="bg-popover text-popover-foreground pointer-events-none absolute top-1 z-10 rounded-md border px-2 py-1 text-xs shadow-md"
            style={
              hover < count / 2
                ? { left: `${(xAt(hover) / W) * 100}%`, marginLeft: 8 }
                : { right: `${100 - (xAt(hover) / W) * 100}%`, marginRight: 8 }
            }
          >
            <div className="text-muted-foreground">{timeLabel(hover)}</div>
            {series.map((s, i) => {
              const v = s.values[hover]
              return (
                <div key={s.key} className="flex items-center gap-1.5">
                  <span
                    className="inline-block h-0.5 w-3 rounded-full"
                    style={{ backgroundColor: PALETTE[i % PALETTE.length] }}
                  />
                  <span>{s.label}</span>
                  <span className="ml-auto pl-2 font-mono">
                    {typeof v === "number" ? formatUnit(v, unit) : "-"}
                  </span>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
