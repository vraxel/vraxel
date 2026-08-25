import {
  memo,
  useCallback,
  useContext,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { createPortal } from "react-dom"
import { HoverTsContext } from "./chart-hover-context"
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceLine,
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

export type ChartUnit = "pct" | "bps" | "bytes" | "sec" | "ops" | "celsius" | "plain"

function hashKey(s: string): number {
  // FNV-1a
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/**
 * Colours keyed by series identity rather than array position.
 *
 * Position is not stable: the agent omits a series that has no data in
 * the window, and VictoriaMetrics does not promise an order at all, so
 * an index-based palette re-coloured the whole network chart every time
 * a container's veth appeared or aged out. Hashing the key pins a
 * series to its colour independently of what else exists.
 *
 * Assignment walks the keys in sorted order and probes forward past
 * taken slots, so a chart whose series fit the palette still gets six
 * distinct colours -- a pure hash would happily give "load 1" and
 * "load 5" the same one. The probing does mean a hash collision among
 * the first six sorted keys can shift a colour when the set changes;
 * past the sixth key every slot is taken, so those series land on
 * their pure hash slot and never move -- which covers the many-series
 * chart the stability matters for.
 */
function assignColors(keys: string[]): Map<string, string> {
  const taken = new Array<boolean>(PALETTE.length).fill(false)
  const out = new Map<string, string>()
  for (const key of [...keys].sort()) {
    let slot = hashKey(key) % PALETTE.length
    for (let probe = 0; probe < PALETTE.length && taken[slot]; probe++) {
      slot = (slot + 1) % PALETTE.length
    }
    taken[slot] = true
    out.set(key, PALETTE[slot])
  }
  return out
}

// One scale tier per chart, picked from the data max, so the Y-axis
// ticks and every tooltip on that chart read in the same unit. Per-point
// scaling would put "900 KB/s" directly above "1.1 MB/s" and make a flat
// line look like a cliff.
type Scale = { divisor: number; label: string }
type Tier = { threshold: number; divisor: number; label: string }

const K = 1024
const BPS_TIERS: Tier[] = [
  { threshold: K ** 3, divisor: K ** 3, label: "GB/s" },
  { threshold: K ** 2, divisor: K ** 2, label: "MB/s" },
  { threshold: K, divisor: K, label: "KB/s" },
]
const BYTE_TIERS: Tier[] = [
  { threshold: K ** 4, divisor: K ** 4, label: "TiB" },
  { threshold: K ** 3, divisor: K ** 3, label: "GiB" },
  { threshold: K ** 2, divisor: K ** 2, label: "MiB" },
  { threshold: K, divisor: K, label: "KiB" },
]
// Disk service times land in the hundreds of microseconds on an SSD and
// the tens of milliseconds on a busy spindle, so seconds alone would
// draw both as zero.
const SEC_TIERS: Tier[] = [
  { threshold: 1, divisor: 1, label: "s" },
  { threshold: 0.001, divisor: 0.001, label: "ms" },
]

function tiersFor(unit: ChartUnit): { tiers: Tier[]; base: string } {
  switch (unit) {
    case "bps":
      return { tiers: BPS_TIERS, base: "B/s" }
    case "bytes":
      return { tiers: BYTE_TIERS, base: "B" }
    case "sec":
      return { tiers: SEC_TIERS, base: "us" }
    case "ops":
      return { tiers: [], base: "/s" }
    case "celsius":
      return { tiers: [], base: "C" }
    default:
      return { tiers: [], base: "" }
  }
}

function scaleFor(unit: ChartUnit, max: number): Scale {
  const { tiers, base } = tiersFor(unit)
  for (const t of tiers) {
    if (max >= t.threshold) return { divisor: t.divisor, label: t.label }
  }
  return { divisor: unit === "sec" ? 0.000001 : 1, label: base }
}

function fmtNum(v: number): string {
  return v >= 10 ? String(Math.round(v)) : v.toFixed(1)
}

// Full format for tooltips: "2.5 KB/s", "54.3%", "0.3 ms"
function formatValue(v: number, unit: ChartUnit, scale: Scale): string {
  if (unit === "pct") return `${fmtNum(v)}%`
  if (unit === "plain") return v >= 10 ? String(Math.round(v)) : v.toFixed(2)
  return `${fmtNum(v / scale.divisor)} ${scale.label}`.trimEnd()
}

// Y-axis ticks: pure number in the chart's scale ("5.6", "2.9")
function formatTick(v: number, unit: ChartUnit, scale: Scale): string {
  if (unit === "pct") return `${fmtNum(v)}%`
  if (unit === "plain") return v >= 10 ? String(Math.round(v)) : v.toFixed(2)
  return fmtNum(v / scale.divisor)
}

const Y_AXIS_WIDTH = 40
const CHART_FONT = 'var(--font-sans, "Inter", sans-serif)'

// The Y range, as [min, max].
//
// min is 0 for everything that cannot go below it, which is almost
// everything -- pinning the floor keeps a chart of small numbers from
// magnifying noise into mountains. Clock offset is the exception and the
// reason this returns a pair at all: it is signed, and a host running
// FAST is exactly as broken as one running slow, so clamping the floor
// to zero would erase half the failures this series exists to show.
function niceDomain(series: ChartSeries[], unit: ChartUnit): [number, number] {
  if (unit === "pct") return [0, 100]
  let max = 0
  let min = 0
  for (const s of series) {
    for (const v of s.values) {
      if (typeof v !== "number") continue
      if (v > max) max = v
      if (v < min) min = v
    }
  }
  return [min < 0 ? min * 1.15 : 0, max > 0 ? max * 1.15 : 1]
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

interface TipRow {
  name: string
  color: string
  /** Numeric value, kept only to sort rows; NEGATIVE_INFINITY for gaps. */
  raw: number
  value: string
}

const TIP_GAP = 12

// Chart geometry that the crosshair has to agree with. The plot area
// starts after the Y axis and ends before the right margin; vertically
// it runs from the top margin down to the X axis labels.
const CHART_MARGIN_RIGHT = 8
const CHART_MARGIN_TOP = 12
const X_AXIS_HEIGHT = 30

/**
 * The shared crosshair, drawn as a plain absolutely-positioned rule
 * rather than a recharts ReferenceLine so that moving it never re-runs
 * the chart. Its x comes from CSS calc over the plot area, so no
 * measurement or resize listener is needed.
 */
function Crosshair({ fromMs, stepSec, count }: { fromMs: number; stepSec: number; count: number }) {
  const hoverTs = useContext(HoverTsContext)
  if (hoverTs == null || count < 2) return null
  const span = (count - 1) * stepSec * 1000
  const frac = Math.min(Math.max((hoverTs - fromMs) / span, 0), 1)
  return (
    <div
      className="border-foreground/30 pointer-events-none absolute w-px border-l border-dashed"
      style={{
        left: `calc(${Y_AXIS_WIDTH}px + (100% - ${Y_AXIS_WIDTH + CHART_MARGIN_RIGHT}px) * ${frac})`,
        top: CHART_MARGIN_TOP,
        bottom: X_AXIS_HEIGHT,
      }}
    />
  )
}

/**
 * The hover tooltip, rendered into document.body so it can never
 * affect the page's layout, and positioned from its own MEASURED size
 * rather than a guessed one -- guessing is what made earlier versions
 * clip their last rows or flip when they did not need to. This is
 * Grafana's approach for chart tooltips (portal + fixed + measured
 * size); it deliberately does not use a floating-ui reference element,
 * because the anchor here is a cursor position, not a DOM node.
 */
function ChartTooltip({
  x,
  y,
  time,
  rows,
}: {
  x: number
  y: number
  time: string
  rows: TipRow[]
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ w: 0, h: 0 })

  // Layout effect, not effect: the measurement has to land before the
  // browser paints, or the first frame shows the tooltip at the
  // unclamped position.
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => {
      const r = el.getBoundingClientRect()
      setSize((prev) =>
        Math.abs(prev.w - r.width) < 1 && Math.abs(prev.h - r.height) < 1
          ? prev
          : { w: r.width, h: r.height },
      )
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const vw = window.innerWidth
  const vh = window.innerHeight
  // Never taller than the viewport; the body scrolls inside instead.
  const maxHeight = vh - TIP_GAP * 2

  // Preferred placement is below-right of the cursor. If it does not
  // fit, try the other side; if neither fits (a tooltip taller than
  // the space on both sides), pin it against the edge. Clamping rather
  // than flipping keeps the position continuous as the cursor moves,
  // so there is no jump at the threshold.
  let left = x + TIP_GAP
  let top = y + TIP_GAP
  if (size.w > 0) {
    if (left + size.w > vw - TIP_GAP) {
      const mirrored = x - TIP_GAP - size.w
      left = mirrored >= TIP_GAP ? mirrored : Math.max(TIP_GAP, vw - TIP_GAP - size.w)
    }
  }
  if (size.h > 0) {
    if (top + size.h > vh - TIP_GAP) {
      const mirrored = y - TIP_GAP - size.h
      top = mirrored >= TIP_GAP ? mirrored : Math.max(TIP_GAP, vh - TIP_GAP - size.h)
    }
  }

  return createPortal(
    <div
      ref={ref}
      className="bg-popover text-popover-foreground pointer-events-none fixed z-50 max-w-[360px] overflow-y-auto rounded-md border px-2.5 py-1.5 text-xs shadow-md"
      style={{
        left,
        top,
        maxHeight,
        // Hidden for the single frame before the size is known, so the
        // uncorrected position is never painted.
        visibility: size.h === 0 ? "hidden" : "visible",
      }}
    >
      <div className="text-muted-foreground mb-1">{time}</div>
      {rows.map((r) => (
        <div key={r.name} className="flex items-center gap-1.5">
          <span
            className="inline-block h-2 w-2 shrink-0 rounded-full"
            style={{ backgroundColor: r.color }}
          />
          <span className="min-w-0 truncate">{r.name}</span>
          <span className="ml-auto shrink-0 pl-2 font-mono tabular-nums">{r.value}</span>
        </div>
      ))}
    </div>,
    document.body,
  )
}

function MetricChartImpl({
  title,
  unit,
  series,
  fromMs,
  stepSec,
  count,
  emptyText,
  thresholds,
  onHover,
}: {
  title: string
  unit: ChartUnit
  series: ChartSeries[]
  fromMs: number
  stepSec: number
  count: number
  emptyText: string
  /** Alert thresholds to draw as horizontal guides on this chart. */
  thresholds?: { label: string; value: number }[]
  onHover?: (ts: number | null) => void
}) {
  const data = useMemo(
    () => toRows(series, fromMs, stepSec, count),
    [series, fromMs, stepSec, count],
  )
  const [minY, maxY] = niceDomain(series, unit)
  const hasData = series.some((s) => s.values.some((v) => typeof v === "number"))
  // Scaled by the larger magnitude, so a signed series picks a unit that
  // fits both ends.
  const scale = useMemo(() => scaleFor(unit, Math.max(maxY, -minY)), [unit, maxY, minY])
  const colors = useMemo(() => assignColors(series.map((s) => s.key)), [series])
  const colorOf = (key: string) => colors.get(key) ?? PALETTE[0]
  const gradPrefix = useId().replace(/:/g, "")
  // The plot area's viewport rect, needed to turn recharts' chart-local
  // tooltip coordinate into a page position. Read at tooltip time (not
  // cached) so scrolling the page does not misplace it.
  const plotRef = useRef<HTMLDivElement>(null)

  const [hidden, setHidden] = useState<Set<string>>(() => new Set())
  // Click = isolate (show only this); Ctrl/Cmd+click = toggle one.
  // If all end up hidden, restore all.
  const handleLegendClick = useCallback(
    (label: string, e: React.MouseEvent) => {
      setHidden((prev) => {
        const allLabels = series.map((s) => s.label)
        if (e.ctrlKey || e.metaKey) {
          const next = new Set(prev)
          if (next.has(label)) next.delete(label)
          else next.add(label)
          if (allLabels.every((l) => next.has(l))) return new Set()
          return next
        }
        const visibleCount = allLabels.filter((l) => !prev.has(l)).length
        if (visibleCount === 1 && !prev.has(label)) return new Set()
        return new Set(allLabels.filter((l) => l !== label))
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
          <div ref={plotRef} className="relative h-[160px] cursor-crosshair">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart
                data={data}
                margin={{ top: 12, right: 8, bottom: 0, left: 0 }}
                onMouseMove={(s) =>
                  onHover?.(typeof s?.activeLabel === "number" ? s.activeLabel : null)
                }
                onMouseLeave={() => onHover?.(null)}
              >
                <defs>
                  {series.map((s) => (
                    <linearGradient
                      key={s.key}
                      id={safeId(gradPrefix, s.key)}
                      x1="0"
                      y1="0"
                      x2="0"
                      y2="1"
                    >
                      <stop offset="0%" stopColor={colorOf(s.key)} stopOpacity={0.2} />
                      <stop offset="100%" stopColor={colorOf(s.key)} stopOpacity={0} />
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
                  tick={{ fontSize: 10, fontFamily: CHART_FONT }}
                  tickLine={false}
                  axisLine={false}
                  minTickGap={60}
                  stroke="currentColor"
                  className="text-muted-foreground"
                />
                <YAxis
                  domain={[minY, maxY]}
                  tickFormatter={(v: number) => formatTick(v, unit, scale)}
                  tick={{ fontSize: 10, fontFamily: CHART_FONT }}
                  tickLine={false}
                  axisLine={false}
                  width={Y_AXIS_WIDTH}
                  stroke="currentColor"
                  className="text-muted-foreground"
                />
                <Tooltip
                  content={({ active, payload, label, coordinate }) => {
                    if (!active || !payload?.length || !coordinate) return null
                    const rows: TipRow[] = payload
                      .filter((p) => typeof p.name === "string" && !hidden.has(p.name))
                      .map((p) => ({
                        name: String(p.name),
                        color: String(p.color),
                        raw: typeof p.value === "number" ? p.value : Number.NEGATIVE_INFINITY,
                        value:
                          typeof p.value === "number" ? formatValue(p.value, unit, scale) : "-",
                      }))
                      // Biggest first: with twenty interfaces, the ones
                      // carrying traffic must not be buried under zeroes.
                      .sort((a, b) => b.raw - a.raw)
                    if (!rows.length) return null
                    // coordinate is chart-local; the plot rect turns it
                    // into the page position the portal needs.
                    const rect = plotRef.current?.getBoundingClientRect()
                    if (!rect) return null
                    return (
                      <ChartTooltip
                        x={rect.left + (coordinate.x ?? 0)}
                        y={rect.top + (coordinate.y ?? 0)}
                        time={typeof label === "number" ? formatTime(label) : String(label)}
                        rows={rows}
                      />
                    )
                  }}
                  // No recharts cursor: the shared Crosshair overlay draws
                  // the hover line on every chart in the panel at the same
                  // timestamp, and a second line here would double it.
                  cursor={false}
                  isAnimationActive={false}
                  wrapperStyle={{ display: "none" }}
                />
                {thresholds?.map((t) => (
                  <ReferenceLine
                    key={t.label}
                    y={t.value}
                    stroke="var(--destructive)"
                    strokeOpacity={0.5}
                    strokeDasharray="4 4"
                    label={{
                      value: t.label,
                      position: "insideTopRight",
                      fontSize: 9,
                      fill: "var(--destructive)",
                    }}
                  />
                ))}
                {series.map((s) => {
                  const isHidden = hidden.has(s.label)
                  const color = colorOf(s.key)
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
                      // No animation. Recharts animates an Area by
                      // growing it from the baseline, re-interpolating
                      // every monotone segment each frame -- at 24h that
                      // is 1440 points across twenty interfaces, which
                      // stalls the range switch. It also does not read as
                      // a transition on the 30s poll: the curve replays
                      // from zero rather than easing to the new values.
                      isAnimationActive={false}
                    />
                  )
                })}
              </AreaChart>
            </ResponsiveContainer>
            <Crosshair fromMs={fromMs} stepSec={stepSec} count={count} />
          </div>

          <div className="flex min-h-[24px] flex-wrap items-start justify-center gap-x-3 gap-y-1 pt-1 text-xs">
            {series.length > 1 &&
              series.map((s) => {
                const isHidden = hidden.has(s.label)
                const color = colorOf(s.key)
                return (
                  <span
                    key={s.key}
                    className={`flex cursor-pointer items-center gap-1 select-none ${isHidden ? "line-through opacity-40" : ""}`}
                    onClick={(e) => handleLegendClick(s.label, e)}
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
