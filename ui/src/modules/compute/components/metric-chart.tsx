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

// Every chart uses the same Y-axis width so plot areas align across
// all panels in the 2-column grid. 64px accommodates the widest label
// (bps: "999 MB/s"); pct and plain waste a few pixels but the visual
// consistency is worth it.
const Y_AXIS_WIDTH = 64

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
  const gradPrefix = useId().replace(/:/g, "")

  const [hidden, setHidden] = useState<Set<string>>(() => new Set())
  const handleLegendClick = useCallback((label: string) => {
    setHidden((prev) => {
      const next = new Set(prev)
      if (next.has(label)) next.delete(label)
      else next.add(label)
      return next
    })
  }, [])

  return (
    <div className="bg-muted/30 rounded-lg border p-3">
      <div className="mb-3 text-sm font-medium">{title}</div>

      {!hasData ? (
        <div className="text-muted-foreground flex h-[160px] items-center justify-center text-xs">
          {emptyText}
        </div>
      ) : (
        <>
          <ResponsiveContainer width="100%" height={160}>
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
                tickFormatter={(v: number) => formatUnit(v, unit)}
                tick={{ fontSize: 10 }}
                tickLine={false}
                axisLine={false}
                width={Y_AXIS_WIDTH}
                stroke="currentColor"
                className="text-muted-foreground"
              />
              <Tooltip
                labelFormatter={(v) => (typeof v === "number" ? formatTime(v) : String(v))}
                formatter={(v, name) => {
                  if (typeof name === "string" && hidden.has(name)) return [null, null]
                  return [typeof v === "number" ? formatUnit(v, unit) : "-", name]
                }}
                contentStyle={{
                  fontSize: 12,
                  borderRadius: 6,
                  border: "1px solid var(--border)",
                  background: "var(--popover)",
                  color: "var(--popover-foreground)",
                }}
                cursor={{ stroke: "currentColor", strokeOpacity: 0.2 }}
                isAnimationActive={false}
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
