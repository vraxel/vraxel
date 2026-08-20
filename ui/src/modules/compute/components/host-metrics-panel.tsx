import { useState } from "react"
import { formatDateTime } from "@/shared/lib/format"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Button } from "@/shared/ui/button"
import { Skeleton } from "@/shared/ui/skeleton"
import { useTranslation } from "@/i18n"
import { useApiQuery } from "@/core/query/hooks"
import type { ScopeRef } from "@/core/registry/resource"
import type { HostMetrics } from "@/generated/compute"
import { hostMetricsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { MetricChart, type ChartSeries, type ChartUnit } from "./metric-chart"

// The window presets. Steps follow what the agent's ring can answer --
// 15s exists for the last hour only -- and every preset stays under the
// shared point budget.
const WINDOWS = [
  { key: "1h", ms: 3_600_000, stepSec: 15 },
  { key: "6h", ms: 21_600_000, stepSec: 60 },
  { key: "24h", ms: 86_400_000, stepSec: 60 },
] as const

type WindowKey = (typeof WINDOWS)[number]["key"]

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
  const { t } = useTranslation()
  const [win, setWin] = useState<WindowKey>("1h")
  const preset = WINDOWS.find((w) => w.key === win) ?? WINDOWS[0]

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
    refetchInterval: 30_000,
  })

  const res = query.data
  const empty = t("compute.host.metrics.noData")
  const dev = (_: string, labels?: Record<string, string>) => labels?.device ?? "-"

  const charts: { title: string; unit: ChartUnit; series: ChartSeries[] }[] = [
    {
      title: t("compute.host.cpu"),
      unit: "pct",
      series: pick(res, ["cpu.used_pct"], () => t("compute.host.metrics.used")),
    },
    {
      title: t("compute.host.memory"),
      unit: "pct",
      series: pick(res, ["mem.used_pct", "swap.used_pct"], (name) =>
        name === "swap.used_pct" ? "swap" : t("compute.host.metrics.used"),
      ),
    },
    {
      title: t("compute.host.metrics.load"),
      unit: "plain",
      series: pick(res, ["load.1", "load.5", "load.15"], (name) => name.replace("load.", "load ")),
    },
    {
      title: t("compute.host.metrics.filesystem"),
      unit: "pct",
      series: pick(res, ["fs.used_pct"], (_, labels) => labels?.mountpoint ?? "-"),
    },
    {
      title: t("compute.host.metrics.diskIO"),
      unit: "bps",
      series: pick(
        res,
        ["disk.read_bps", "disk.write_bps"],
        (name, labels) =>
          `${labels?.device ?? "-"} ${name === "disk.read_bps" ? t("compute.host.metrics.read") : t("compute.host.metrics.write")}`,
      ),
    },
    {
      title: t("compute.host.metrics.network"),
      unit: "bps",
      series: pick(
        res,
        ["net.rx_bps", "net.tx_bps"],
        (name, labels) => `${dev(name, labels)} ${name === "net.rx_bps" ? "rx" : "tx"}`,
      ),
    },
  ]

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
          // Not an empty grid of axes: the honest state is a sentence.
          // The list row's last snapshot (greyed) is the freshest data
          // that exists for this host.
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
              <Skeleton key={c.title} className="h-[180px]" />
            ))}
          </div>
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {charts.map((c) => (
              <MetricChart
                key={c.title}
                title={c.title}
                unit={c.unit}
                series={c.series}
                fromMs={res.fromMs}
                stepSec={res.stepSec}
                count={res.count}
                emptyText={empty}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
