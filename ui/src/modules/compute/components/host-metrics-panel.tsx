import { useMemo, useState } from "react"
import { keepPreviousData } from "@tanstack/react-query"
import { formatDateTime } from "@/shared/lib/format"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Button } from "@/shared/ui/button"
import { Skeleton } from "@/shared/ui/skeleton"
import { translate, useTranslation } from "@/i18n"
import { useApiQuery } from "@/core/query/hooks"
import type { ScopeRef } from "@/core/registry/resource"
import type { HostMetrics } from "@/generated/compute"
import { hostMetricsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { MetricChart, type ChartSeries, type ChartUnit } from "./metric-chart"
import { HoverTsContext } from "./chart-hover-context"

// The window presets. Steps follow what the agent's ring can answer --
// 15s exists for the last hour only -- and every preset stays under the
// shared point budget.
const WINDOWS = [
  { key: "1h", ms: 3_600_000, stepSec: 15 },
  { key: "6h", ms: 21_600_000, stepSec: 60 },
  { key: "24h", ms: 86_400_000, stepSec: 60 },
] as const

type WindowKey = (typeof WINDOWS)[number]["key"]

// The device dimension for network series labels. Module-level so the
// charts memo does not close over a per-render function.
const dev = (_: string, labels?: Record<string, string>) => labels?.device ?? "-"

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
 * The six utilisation charts on a host's detail page, read on demand
 * from the host's own agent (the server keeps no metric history). The
 * window slides on every poll: from/to are computed inside the fetch, so
 * a chart left open keeps showing "the last hour", not the hour that was
 * last when it opened.
 */
export function HostMetricsPanel({ host, scope }: { host: Host; scope: ScopeRef }) {
  const { t, locale } = useTranslation()
  const [win, setWin] = useState<WindowKey>("1h")
  const preset = WINDOWS.find((w) => w.key === win) ?? WINDOWS[0]
  // Shared across all six charts, so hovering one shows the crosshair at
  // the same instant on the others -- which is how a CPU spike and the
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
    // No faster than a bucket completes, and never faster than 30s.
    //
    // Every poll refetches the whole window to learn about its tail: at
    // 6h, two polls 30s apart differ in 6 of 361 buckets (measured --
    // 3.8% of the points), because only the trailing buckets are still
    // filling. Polling twice per bucket buys a partially-formed tail,
    // which is not what a six-hour trend is read for, and each one is
    // work for the managed machine's own agent since the server keeps no
    // history.
    //
    // The 30s floor is the other half: at 1h the step is the agent's own
    // 15s sample period, and matching it would double the load on the
    // default view to shave 15s off a chart nobody reads that closely.
    refetchInterval: Math.max(preset.stepSec * 1000, 30_000),
    // The interval already skips fetching while the tab is hidden --
    // that is the library default, not something to restate here. What
    // is NOT the default is coming back: refetchOnWindowFocus is false
    // globally (core/query/client.ts), so returning to the tab left the
    // charts on the window the operator walked away from until the next
    // tick fired. A metrics panel that reads as live has to catch up the
    // moment it is looked at.
    refetchOnWindowFocus: true,
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
  const charts = useMemo<{ id: string; title: string; unit: ChartUnit; series: ChartSeries[] }[]>(
    () => [
      {
        id: "cpu",
        title: translate("compute.host.cpu"),
        unit: "pct",
        series: pick(res, ["cpu.used_pct"], () => translate("compute.host.metrics.used")),
      },
      {
        id: "mem",
        title: translate("compute.host.memory"),
        unit: "pct",
        series: pick(res, ["mem.used_pct", "swap.used_pct"], (name) =>
          name === "swap.used_pct" ? "swap" : translate("compute.host.metrics.used"),
        ),
      },
      {
        id: "load",
        title: translate("compute.host.metrics.load"),
        unit: "plain",
        series: pick(res, ["load.1", "load.5", "load.15"], (name) =>
          name.replace("load.", "load "),
        ),
      },
      {
        id: "fs",
        title: translate("compute.host.metrics.filesystem"),
        unit: "pct",
        series: pick(res, ["fs.used_pct"], (_, labels) => labels?.mountpoint ?? "-"),
      },
      {
        id: "disk",
        title: translate("compute.host.metrics.diskIO"),
        unit: "bps",
        series: pick(
          res,
          ["disk.read_bps", "disk.write_bps"],
          (name, labels) =>
            `${labels?.device ?? "-"} ${name === "disk.read_bps" ? translate("compute.host.metrics.read") : translate("compute.host.metrics.write")}`,
        ),
      },
      {
        id: "net",
        title: translate("compute.host.metrics.network"),
        unit: "bps",
        series: pick(
          res,
          ["net.rx_bps", "net.tx_bps"],
          (name, labels) => `${dev(name, labels)} ${name === "net.rx_bps" ? "rx" : "tx"}`,
        ),
      },
    ],
    // translate() reads the locale from the store at call time, so the
    // rule cannot see that these labels depend on it; drop locale and
    // the titles would stay in the old language until the next poll.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [res, locale],
  )

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base">{t("compute.host.metrics.title")}</CardTitle>
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
      </CardHeader>
      <CardContent>
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
