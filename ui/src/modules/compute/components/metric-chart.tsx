import { useRef, useState } from "react"

// A line chart over the fixed grid HostMetrics answers with. Hand-rolled
// SVG rather than a chart dependency: six charts of polylines on a
// regular grid need no scales, no animation engine and no bundle weight,
// and the one interactive behaviour worth having -- reading values off a
// bucket under the cursor -- is a rect lookup.
//
// Null values are gaps in the line, never zeroes: a bucket the agent
// holds nothing for (it was down, the disk was not mounted yet) must not
// draw as an idle machine.

export interface ChartSeries {
  key: string
  label: string
  values: (number | null | undefined)[]
}

// Explicit palette instead of theme tokens: this repo defines no chart
// colours, and these mid-500s hold up on both themes.
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

// niceMax pads the data maximum so lines do not kiss the frame, with a
// floor so an all-zero chart still has a scale.
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

  const innerW = W - PAD_L - PAD_R
  const innerH = H - PAD_T - PAD_B
  const max = niceMax(series, unit)
  const x = (i: number) => PAD_L + (count > 1 ? (i / (count - 1)) * innerW : 0)
  const y = (v: number) => PAD_T + innerH - (Math.min(v, max) / max) * innerH

  const paths = series.map((s) => {
    const segments: string[] = []
    let current: string[] = []
    s.values.forEach((v, i) => {
      if (typeof v !== "number") {
        if (current.length > 1) segments.push(current.join(" "))
        current = []
        return
      }
      current.push(`${x(i).toFixed(1)},${y(v).toFixed(1)}`)
    })
    if (current.length > 1) segments.push(current.join(" "))
    return segments
  })
  const hasData = paths.some((p) => p.length > 0)

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
                  className="inline-block h-0.5 w-3"
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
          {paths.map((segments, i) =>
            segments.map((points, j) => (
              <polyline
                key={`${series[i].key}-${j}`}
                points={points}
                fill="none"
                stroke={PALETTE[i % PALETTE.length]}
                strokeWidth="1.5"
                vectorEffect="non-scaling-stroke"
              />
            )),
          )}
          {hover !== null && hasData && (
            <line
              x1={x(hover)}
              x2={x(hover)}
              y1={PAD_T}
              y2={PAD_T + innerH}
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
                ? { left: `${(x(hover) / W) * 100}%`, marginLeft: 8 }
                : { right: `${100 - (x(hover) / W) * 100}%`, marginRight: 8 }
            }
          >
            <div className="text-muted-foreground">{timeLabel(hover)}</div>
            {series.map((s, i) => {
              const v = s.values[hover]
              return (
                <div key={s.key} className="flex items-center gap-1.5">
                  <span
                    className="inline-block h-0.5 w-3"
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
