import { memo, useCallback, useId, useMemo, useState } from "react"
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

export interface ChartSeries {
  key: string
  label: string
  values: (number | null | undefined)[]
}

const PALETTE = ["#0ea5e9", "#10b981", "#f59e0b", "#8b5cf6", "#f43f5e", "#06b6d4"]

export type ChartUnit = "pct" | "bps" | "plain"

// For bps charts, compute a single scale tier from the data max so
// the title, Y-axis ticks, and tooltip all use the same unit.
type BpsScale = { divisor: number; label: string }
const BPS_TIERS: { threshold: number; divisor: number; label: string }[] = [
  { threshold: 1024 * 1024 * 1024, divisor: 1024 * 1024 * 1024, label: "GB/s" },
  { threshold: 1024 * 1024, divisor: 1024 * 1024, label: "MB/s" },
  { threshold: 1024, divisor: 1024, label: "KB/s" },
]
function bpsScale(max: number): BpsScale {
  for (const t of BPS_TIERS) {
    if (max >= t.threshold) return { divisor: t.divisor, label: t.label }
  }
  return { divisor: 1, label: "B/s" }
}

function fmtNum(v: number): string {
  return v >= 10 ? String(Math.round(v)) : v.toFixed(1)
}

// Full format for tooltips: "2.5 KB/s", "54.3%"
function formatValue(v: number, unit: ChartUnit, scale: BpsScale): string {
  if (unit === "pct") return `${fmtNum(v)}%`
  if (unit === "bps") return `${fmtNum(v / scale.divisor)} ${scale.label}`
  return v >= 10 ? String(Math.round(v)) : v.toFixed(2)
}

// Y-axis ticks: pure number in the chart's scale ("5.6", "2.9")
function formatTick(v: number, unit: ChartUnit, scale: BpsScale): string {
  if (unit === "pct") return `${fmtNum(v)}%`
  if (unit === "bps") return fmtNum(v / scale.divisor)
  return v >= 10 ? String(Math.round(v)) : v.toFixed(2)
}

const Y_AXIS_WIDTH = 40

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

function toRows(
  series: ChartSeries[],
  fromMs: number,
  stepSec: number,
  count: number,
): Record<string, number | null>[] {
  const rows: Record<string, number | null>[] = []
  for (let i = 0; i < count; i++) {
    const row: Record<string, number | null> = { _ts: fromMs + i * stepSec * 1000 }
    for (const s of series) {
      const v = s.values[i]
      row[s.key] = typeof v === "number" ? v : null
    }
    rows.push(row)
  }
  return rows
}

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}

function safeId(prefix: string, key: string): string {
  return `${prefix}-${key.replace(/[^A-Za-z0-9_-]/g, "_")}`
}

function MetricChartImpl({
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
  const data = useMemo(
    () => toRows(series, fromMs, stepSec, count),
    [series, fromMs, stepSec, count],
  )
  const max = niceMax(series, unit)
  const hasData = series.some((s) => s.values.some((v) => typeof v === "number"))
  const scale = useMemo(() => bpsScale(max), [max])
  const gradPrefix = useId().replace(/:/g, "")

  const [hidden, setHidden] = useState<Set<string>>(() => new Set())
  // Isolate mode: click one series = show only that one; click again
  // (or click when only one is visible) = restore all. One click to
  // focus instead of N-1 clicks to hide everything else.
  const handleLegendClick = useCallback(
    (label: string) => {
      setHidden((prev) => {
        const allLabels = series.map((s) => s.label)
        const visibleCount = allLabels.filter((l) => !prev.has(l)).length
        if (visibleCount === 1 && !prev.has(label)) {
          return new Set()
        }
        const next = new Set(allLabels.filter((l) => l !== label))
        return next
      })
    },
    [series],
  )

  return (
    <div className="bg-muted/30 rounded-lg border p-3">
      <div className="mb-3 text-sm font-medium">
        {title}
        {hasData && unit === "bps" && (
          <span className="text-muted-foreground ml-1 font-normal">({scale.label})</span>
        )}
      </div>

      {!hasData ? (
        <div className="text-muted-foreground flex h-[160px] items-center justify-center text-xs">
          {emptyText}
        </div>
      ) : (
        <>
          <div className="relative h-[160px]">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={data} margin={{ top: 12, right: 8, bottom: 0, left: 0 }}>
                <defs>
                  {series.map((s, i) => (
                    <linearGradient
                      key={s.key}
                      id={safeId(gradPrefix, s.key)}
                      x1="0"
                      y1="0"
                      x2="0"
                      y2="1"
                    >
                      <stop offset="0%" stopColor={PALETTE[i % PALETTE.length]} stopOpacity={0.2} />
                      <stop offset="100%" stopColor={PALETTE[i % PALETTE.length]} stopOpacity={0} />
                    </linearGradient>
                  ))}
                </defs>
                <CartesianGrid
                  strokeDasharray="3 3"
                  stroke="currentColor"
                  strokeOpacity={0.1}
                  vertical={false}
                  className="text-foreground"
                />
                <XAxis
                  dataKey="_ts"
                  type="number"
                  domain={["dataMin", "dataMax"]}
                  tickFormatter={formatTime}
                  tick={{ fontSize: 10 }}
                  tickLine={false}
                  axisLine={false}
                  minTickGap={60}
                  stroke="currentColor"
                  className="text-muted-foreground"
                />
                <YAxis
                  domain={[0, max]}
                  tickFormatter={(v: number) => formatTick(v, unit, scale)}
                  tick={{ fontSize: 10 }}
                  tickLine={false}
                  axisLine={false}
                  width={Y_AXIS_WIDTH}
                  stroke="currentColor"
                  className="text-muted-foreground"
                />
                <Tooltip
                  content={({ active, payload, label }) => {
                    if (!active || !payload?.length) return null
                    const visible = payload
                      .filter((p) => typeof p.name === "string" && !hidden.has(p.name))
                      .slice()
                      .sort((a, b) => {
                        const va = typeof a.value === "number" ? a.value : 0
                        const vb = typeof b.value === "number" ? b.value : 0
                        return vb - va
                      })
                    if (!visible.length) return null
                    return (
                      <div className="bg-popover text-popover-foreground max-h-[200px] max-w-[360px] overflow-y-auto rounded-md border px-2.5 py-1.5 text-xs shadow-md">
                        <div className="text-muted-foreground mb-1">
                          {typeof label === "number" ? formatTime(label) : String(label)}
                        </div>
                        {visible.map((p) => (
                          <div key={String(p.name)} className="flex items-center gap-1.5">
                            <span
                              className="inline-block h-2 w-2 shrink-0 rounded-full"
                              style={{ backgroundColor: String(p.color) }}
                            />
                            <span className="min-w-0 truncate">{p.name}</span>
                            <span className="ml-auto shrink-0 pl-2 font-mono tabular-nums">
                              {typeof p.value === "number"
                                ? formatValue(p.value, unit, scale)
                                : "-"}
                            </span>
                          </div>
                        ))}
                      </div>
                    )
                  }}
                  cursor={{ stroke: "currentColor", strokeOpacity: 0.2 }}
                  isAnimationActive={false}
                  position={{ y: 0 }}
                  allowEscapeViewBox={{ x: true, y: true }}
                  wrapperStyle={{ zIndex: 20 }}
                />
                {series.map((s, i) => {
                  const isHidden = hidden.has(s.label)
                  const color = PALETTE[i % PALETTE.length]
                  return (
                    <Area
                      key={s.key}
                      dataKey={s.key}
                      name={s.label}
                      type="monotone"
                      stroke={isHidden ? "transparent" : color}
                      strokeWidth={isHidden ? 0 : 1.5}
                      fill={isHidden ? "transparent" : `url(#${safeId(gradPrefix, s.key)})`}
                      fillOpacity={isHidden ? 0 : 1}
                      dot={false}
                      activeDot={isHidden ? false : { r: 3, strokeWidth: 0, fill: color }}
                      connectNulls={false}
                      animationDuration={500}
                      animationEasing="ease-out"
                      isAnimationActive={true}
                    />
                  )
                })}
              </AreaChart>
            </ResponsiveContainer>
          </div>

          <div className="flex min-h-[24px] flex-wrap items-start justify-center gap-x-3 gap-y-1 pt-1 text-xs">
            {series.length > 1 &&
              series.map((s, i) => {
                const isHidden = hidden.has(s.label)
                const color = PALETTE[i % PALETTE.length]
                return (
                  <span
                    key={s.key}
                    className={`flex cursor-pointer items-center gap-1 select-none ${isHidden ? "line-through opacity-40" : ""}`}
                    onClick={() => handleLegendClick(s.label)}
                  >
                    <span
                      className="inline-block h-2.5 w-2.5 rounded-sm"
                      style={{ backgroundColor: color }}
                    />
                    {s.label}
                  </span>
                )
              })}
          </div>
        </>
      )}
    </div>
  )
}

export const MetricChart = memo(MetricChartImpl)
