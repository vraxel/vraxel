import { useMemo, useState } from "react"
import { keepPreviousData } from "@tanstack/react-query"
import { RefreshCw } from "lucide-react"
import { formatDateTime } from "@/shared/lib/format"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Button } from "@/shared/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"
import { Skeleton } from "@/shared/ui/skeleton"
import { translate, useTranslation } from "@/i18n"
import { useApiQuery } from "@/core/query/hooks"
import type { ScopeRef } from "@/core/registry/resource"
import type { HostMetrics } from "@/generated/compute"
import { hostMetricsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { Tabs, TabsList, TabsTrigger } from "@/shared/ui/tabs"
import { MetricChart, type ChartSeries, type ChartUnit } from "./metric-chart"
import { HoverTsContext } from "./chart-hover-context"
import { chartsFor, GROUP_KEYS, type GroupKey } from "./host-metrics-charts"

// The window presets. Steps follow what the agent's ring can answer --
// 15s exists for the last hour only -- and every preset stays under the
// shared point budget.
const WINDOWS = [
  { key: "1h", ms: 3_600_000, stepSec: 15 },
  { key: "6h", ms: 21_600_000, stepSec: 60 },
  { key: "24h", ms: 86_400_000, stepSec: 60 },
] as const

type WindowKey = (typeof WINDOWS)[number]["key"]

// Refresh cadences the operator can pick. "auto" is the default and the
// only one that reasons about the window: no faster than a bucket
// completes, never faster than 30s (see the query below). The explicit
// values exist for the two cases auto cannot serve -- watching a change
// land right now, and holding a picture still to read it.
const REFRESH_OPTIONS = [
  { key: "auto", ms: null },
  { key: "off", ms: 0 },
  { key: "15s", ms: 15_000 },
  { key: "30s", ms: 30_000 },
  { key: "1m", ms: 60_000 },
  { key: "5m", ms: 300_000 },
] as const

type RefreshKey = (typeof REFRESH_OPTIONS)[number]["key"]

// One chart's worth of the response: series picked by name, labelled by
// their distinguishing dimension.
function pick(
  res: HostMetrics | undefined,
  names: string[],
  label: (name: string, labels?: Record<string, string>) => string,
): ChartSeries[] {
  if (!res) return []
  const out: ChartSeries[] = []
  for (const s of res.series) {
    if (!names.includes(s.name)) continue
    out.push({
      key: s.name + JSON.stringify(s.labels ?? {}),
      label: label(s.name, s.labels),
      values: s.values,
    })
  }
  return out
}

/**
 * The utilisation charts on a host's detail page, read on demand
 * from the host's own agent (the server keeps no metric history). The
 * window slides on every poll: from/to are computed inside the fetch, so
 * a chart left open keeps showing "the last hour", not the hour that was
 * last when it opened.
 */
export function HostMetricsPanel({ host, scope }: { host: Host; scope: ScopeRef }) {
  const { t, locale } = useTranslation()
  const [win, setWin] = useState<WindowKey>("1h")
  const preset = WINDOWS.find((w) => w.key === win) ?? WINDOWS[0]
  const [refreshKey, setRefreshKey] = useState<RefreshKey>("auto")
  const [group, setGroup] = useState<GroupKey>("overview")
  const refreshMs = REFRESH_OPTIONS.find((r) => r.key === refreshKey)?.ms ?? null
  // Shared across the visible charts, so hovering one shows the crosshair
  // at the same instant on the others -- which is how a CPU spike and the
  // disk spike that caused it get lined up by eye.
  const [hoverTs, setHoverTs] = useState<number | null>(null)

  const online = host.spec.agentStatus === "online"

  const query = useApiQuery({
    queryKey: ["host-metrics", host.metadata.id, scope.ws, scope.ns, win],
    queryFn: () => {
      const now = Date.now()
      return hostMetricsApi(scope, host.metadata.id, {
        from_ms: now - preset.ms,
        to_ms: now,
        step_sec: preset.stepSec,
      })
    },
    // Only while the agent is up: an offline host has no ring to answer
    // from, and the panel says so instead of polling into an error.
    enabled: online,
    // "auto" (null) derives the cadence: no faster than a bucket
    // completes, and never faster than 30s. Anything else is the
    // operator's explicit choice, with 0 meaning paused -- which
    // TanStack spells `false`.
    //
    // Every poll refetches the whole window to learn about its tail: at
    // 6h, two polls 30s apart differ in 6 of 361 buckets (measured --
    // 3.8% of the points), because only the trailing buckets are still
    // filling. Polling twice per bucket buys a partially-formed tail,
    // which is not what a six-hour trend is read for, and each one is
    // work for the managed machine's own agent since the server keeps no
    // history. The 30s floor is the other half: at 1h the step is the
    // agent's own 15s sample period, and matching it would double the
    // load on the default view to shave 15s off a chart nobody reads
    // that closely.
    refetchInterval:
      refreshMs === null ? Math.max(preset.stepSec * 1000, 30_000) : refreshMs || false,
    // The interval already skips fetching while the tab is hidden --
    // that is the library default, not something to restate here. What
    // is NOT the default is coming back: refetchOnWindowFocus is false
    // globally (core/query/client.ts), so returning to the tab left the
    // charts on the window the operator walked away from until the next
    // tick fired. A metrics panel that reads as live has to catch up the
    // moment it is looked at.
    //
    // Paused is the exception: someone who turned refreshing off wants
    // the picture to hold still, and having it move the instant they
    // click back into the window is the opposite of that.
    refetchOnWindowFocus: refreshMs !== 0,
    // Switching range mints a new key, and without this the charts fall
    // back to skeletons: six recharts instances torn down and rebuilt,
    // losing whatever series the operator had isolated. Keeping the
    // previous window on screen swaps the data in place instead.
    placeholderData: keepPreviousData,
    // Failures render inline on the card. Without this, polling against
    // a briefly unreachable agent raises a toast per attempt -- a
    // sustained storm for as long as the page stays open.
    meta: { skipGlobalError: true },
  })

  const res = query.data
  const empty = t("compute.host.metrics.noData")

  // Memoised on the response: without this, every mousemove would run
  // pick() six times over the whole series list, which was half of what
  // made the shared crosshair stutter. Labels go through the non-hook
  // translator so the memo does not depend on `t`, which is rebuilt on
  // every render; `locale` is what actually changes them.
  // Only the visible group's charts are built. The response carries
  // every series either way (one query answers the whole page), but
  // pick() walks it once per chart, and doing that for all 29 on every
  // poll was measurable on the crosshair.
  const charts = useMemo<{ id: string; title: string; unit: ChartUnit; series: ChartSeries[] }[]>(
    () =>
      chartsFor(group).map((c) => ({
        id: c.id,
        title: translate(c.titleKey),
        unit: c.unit,
        series: pick(res, c.names, c.label),
      })),
    // translate() reads the locale from the store at call time, so the
    // rule cannot see that these labels depend on it; drop locale and
    // the titles would stay in the old language until the next poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [res, locale, group],
  )

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base">{t("compute.host.metrics.title")}</CardTitle>
        <div className="flex items-center gap-2">
          <div className="flex gap-1">
            {WINDOWS.map((w) => (
              <Button
                key={w.key}
                size="sm"
                variant={w.key === win ? "secondary" : "ghost"}
                onClick={() => setWin(w.key)}
              >
                {w.key}
              </Button>
            ))}
          </div>
          <Select value={refreshKey} onValueChange={(v) => setRefreshKey(v as RefreshKey)}>
            <SelectTrigger
              size="sm"
              className="w-28"
              aria-label={t("compute.host.metrics.refresh")}
            >
              <RefreshCw className={`size-3.5 ${query.isFetching ? "animate-spin" : ""}`} />
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              {REFRESH_OPTIONS.map((r) => (
                <SelectItem key={r.key} value={r.key}>
                  {r.key === "auto" || r.key === "off"
                    ? t(`compute.host.metrics.refresh.${r.key}`)
                    : r.key}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardHeader>
      <CardContent>
        {/* Sub-tabs inside the card, not a second row of page tabs: the
            window and refresh controls in the header apply to all of
            them, and moving the grouping up a level would separate a
            chart from the controls that shape it. */}
        <Tabs value={group} onValueChange={(v) => setGroup(v as GroupKey)} className="mb-4">
          <TabsList>
            {GROUP_KEYS.map((k) => (
              <TabsTrigger key={k} value={k}>
                {t(`compute.host.metrics.group.${k}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        {!online ? (
          <div className="text-muted-foreground py-8 text-center text-sm">
            {t("compute.host.metrics.offline")}
            {host.spec.metricsSampledAt
              ? ` (${t("compute.host.metrics.lastSample")} ${formatDateTime(host.spec.metricsSampledAt)})`
              : ""}
          </div>
        ) : query.isError ? (
          <div className="text-destructive py-8 text-center text-sm">
            {query.error instanceof Error ? query.error.message : empty}
          </div>
        ) : !res ? (
          <div className="grid gap-3 md:grid-cols-2">
            {charts.map((c) => (
              <Skeleton key={c.id} className="h-[180px]" />
            ))}
          </div>
        ) : (
          <HoverTsContext.Provider value={hoverTs}>
            <div
              className={`grid gap-3 transition-opacity md:grid-cols-2 ${
                query.isPlaceholderData ? "opacity-50" : ""
              }`}
            >
              {charts.map((c) => (
                <MetricChart
                  key={c.id}
                  title={c.title}
                  unit={c.unit}
                  series={c.series}
                  fromMs={res.fromMs}
                  stepSec={res.stepSec}
                  count={res.count}
                  emptyText={empty}
                  onHover={setHoverTs}
                />
              ))}
            </div>
          </HoverTsContext.Provider>
        )}
      </CardContent>
    </Card>
  )
}
