import { useMemo } from "react"
import {
  Area,
  AreaChart,
  CartesianGrid,
  Legend,
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

// Recharts needs row-oriented data: [{time, "cpu.used_pct": 42, ...}, ...]
// Null/undefined values produce gaps via connectNulls={false} (the default).
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
  const data = useMemo(
    () => toRows(series, fromMs, stepSec, count),
    [series, fromMs, stepSec, count],
  )
  const max = niceMax(series, unit)
  const hasData = series.some((s) => s.values.some((v) => typeof v === "number"))

  return (
    <div className="rounded-lg border p-3">
      <div className="mb-1 text-sm font-medium">{title}</div>

      {!hasData ? (
        <div className="text-muted-foreground flex h-[140px] items-center justify-center text-xs">
          {emptyText}
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={160}>
          <AreaChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: -8 }}>
            <defs>
              {series.map((s, i) => (
                <linearGradient key={s.key} id={`grad-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={PALETTE[i % PALETTE.length]} stopOpacity={0.2} />
                  <stop offset="100%" stopColor={PALETTE[i % PALETTE.length]} stopOpacity={0} />
                </linearGradient>
              ))}
            </defs>
            <CartesianGrid strokeDasharray="3 3" strokeOpacity={0.15} vertical={false} />
            <XAxis
              dataKey="_ts"
              type="number"
              domain={["dataMin", "dataMax"]}
              tickFormatter={formatTime}
              tick={{ fontSize: 10 }}
              tickLine={false}
              axisLine={false}
              minTickGap={60}
            />
            <YAxis
              domain={[0, max]}
              tickFormatter={(v: number) => formatUnit(v, unit)}
              tick={{ fontSize: 10 }}
              tickLine={false}
              axisLine={false}
              width={40}
            />
            <Tooltip
              labelFormatter={(v) => (typeof v === "number" ? formatTime(v) : String(v))}
              formatter={(v) => (typeof v === "number" ? formatUnit(v, unit) : "-")}
              contentStyle={{
                fontSize: 12,
                borderRadius: 6,
                border: "1px solid var(--border)",
                background: "var(--popover)",
                color: "var(--popover-foreground)",
              }}
              isAnimationActive={false}
            />
            {series.length > 1 && (
              <Legend
                iconType="plainline"
                iconSize={12}
                wrapperStyle={{ fontSize: 11, paddingTop: 4 }}
              />
            )}
            {series.map((s, i) => (
              <Area
                key={s.key}
                dataKey={s.key}
                name={s.label}
                type="monotone"
                stroke={PALETTE[i % PALETTE.length]}
                strokeWidth={1.5}
                fill={`url(#grad-${s.key})`}
                dot={false}
                activeDot={{ r: 3, strokeWidth: 0 }}
                connectNulls={false}
                animationDuration={500}
                animationEasing="ease-out"
                isAnimationActive={true}
              />
            ))}
          </AreaChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}
