import { translate, type TranslateKey } from "@/i18n"
import type { ChartUnit } from "./metric-chart"

// What each chart is made of, kept out of the panel so that file stays
// about fetching and layout.
//
// A definition names the series to pick and how to label each line. The
// labels are built at call time through the non-hook translator, because
// the panel memoises the whole set on the response and would otherwise
// hold last language's strings until the next poll.
export interface ChartDef {
  id: string
  /** Which sub-tab it belongs to. */
  group: GroupKey
  titleKey: TranslateKey
  unit: ChartUnit
  names: string[]
  /** Line label. Given the series name and its labels. */
  label: (name: string, labels?: Record<string, string>) => string
}

export const GROUP_KEYS = ["overview", "cpu", "memory", "disk", "network", "system"] as const
export type GroupKey = (typeof GROUP_KEYS)[number]

// Label builders. Module-level so the panel's memo does not close over
// per-render functions.
//
// Every one that produces text resolves it INSIDE the returned closure.
// Resolving at module load would freeze the labels in whatever language
// the app started in, which is the same trap the panel's memo comment
// describes -- and here it would never self-correct, because a module
// body runs once.
const dev = (_: string, l?: Record<string, string>) => l?.device ?? "-"
const mount = (_: string, l?: Record<string, string>) => l?.mountpoint ?? "-"
const one = (key: TranslateKey) => () => translate(key)
// A device carries two lines (read and write, rx and tx); the direction
// has to be in the label or the legend is a list of identical names.
const devDir =
  (firstName: string, first: TranslateKey, second: TranslateKey) =>
  (name: string, l?: Record<string, string>) =>
    `${l?.device ?? "-"} ${translate(name === firstName ? first : second)}`
// Same, for directions that are not translated (rx / tx are the wire's
// own words and are not localised anywhere else on this page either).
const devRxTx = (rxName: string) => (name: string, l?: Record<string, string>) =>
  `${l?.device ?? "-"} ${name === rxName ? "rx" : "tx"}`

/**
 * Every chart, in display order within its group.
 *
 * The set is deliberately not node_exporter's full surface. Each one
 * earns its place by answering a question the others cannot: throughput
 * cannot tell you a device is out of IOPS, utilisation cannot tell you
 * it is slow, and a percentage of memory cannot tell you how much of it
 * is reclaimable cache. Charts that only restate a neighbour (guest CPU
 * time, page tables, slab detail) are left out -- on a page read while
 * something is wrong, a chart that adds nothing costs attention.
 */
export const CHARTS: ChartDef[] = [
  // --- CPU ---
  {
    id: "cpu.used",
    group: "cpu",
    titleKey: "compute.host.cpu",
    unit: "pct",
    names: ["cpu.used_pct"],
    label: one("compute.host.metrics.used"),
  },
  {
    id: "cpu.modes",
    group: "cpu",
    titleKey: "compute.host.metrics.cpuByMode",
    unit: "pct",
    names: ["cpu.mode_pct"],
    // steal is why this chart is worth a slot on a fleet of guests: it is
    // the hypervisor taking CPU away, and total usage cannot show it.
    label: (_, l) => l?.mode ?? "-",
  },
  {
    id: "load",
    group: "cpu",
    titleKey: "compute.host.metrics.load",
    unit: "plain",
    names: ["load.1", "load.5", "load.15"],
    label: (name) => name.replace("load.", "load "),
  },
  {
    id: "psi.cpu",
    group: "cpu",
    titleKey: "compute.host.metrics.psiCpu",
    unit: "pct",
    names: ["psi.cpu_pct"],
    label: one("compute.host.metrics.stalled"),
  },
  {
    id: "cpu.switches",
    group: "cpu",
    titleKey: "compute.host.metrics.switches",
    unit: "ops",
    names: ["sys.ctx_switches", "sys.interrupts"],
    label: (name) =>
      name === "sys.ctx_switches"
        ? translate("compute.host.metrics.ctxSwitches")
        : translate("compute.host.metrics.interrupts"),
  },

  // --- memory ---
  {
    id: "mem.used",
    group: "memory",
    titleKey: "compute.host.memory",
    unit: "pct",
    names: ["mem.used_pct", "swap.used_pct"],
    label: (name) => (name === "swap.used_pct" ? "swap" : translate("compute.host.metrics.used")),
  },
  {
    id: "mem.detail",
    group: "memory",
    titleKey: "compute.host.metrics.memDetail",
    unit: "bytes",
    names: ["mem.used_bytes", "mem.buffers_bytes", "mem.cached_bytes", "mem.free_bytes"],
    label: (name) => name.replace("mem.", "").replace("_bytes", ""),
  },
  {
    id: "swap.io",
    group: "memory",
    titleKey: "compute.host.metrics.swapIO",
    unit: "ops",
    names: ["swap.in_pps", "swap.out_pps"],
    label: (name) => (name === "swap.in_pps" ? "in" : "out"),
  },
  {
    id: "mem.faults",
    group: "memory",
    titleKey: "compute.host.metrics.pageFaults",
    unit: "ops",
    names: ["mem.page_faults"],
    label: (_, l) => l?.kind ?? "-",
  },
  {
    id: "mem.oom",
    group: "memory",
    titleKey: "compute.host.metrics.oomKills",
    unit: "plain",
    names: ["mem.oom_kills"],
    label: one("compute.host.metrics.oomKills"),
  },
  {
    id: "psi.mem",
    group: "memory",
    titleKey: "compute.host.metrics.psiMem",
    unit: "pct",
    names: ["psi.mem_pct"],
    label: one("compute.host.metrics.stalled"),
  },

  // --- disk ---
  {
    id: "fs.used",
    group: "disk",
    titleKey: "compute.host.metrics.filesystem",
    unit: "pct",
    names: ["fs.used_pct"],
    label: mount,
  },
  {
    id: "fs.inodes",
    group: "disk",
    titleKey: "compute.host.metrics.inodes",
    unit: "pct",
    names: ["fs.inodes_used_pct"],
    label: mount,
  },
  {
    id: "disk.io",
    group: "disk",
    titleKey: "compute.host.metrics.diskIO",
    unit: "bps",
    names: ["disk.read_bps", "disk.write_bps"],
    label: devDir("disk.read_bps", "compute.host.metrics.read", "compute.host.metrics.write"),
  },
  {
    id: "disk.iops",
    group: "disk",
    titleKey: "compute.host.metrics.iops",
    unit: "ops",
    names: ["disk.read_iops", "disk.write_iops"],
    label: devDir("disk.read_iops", "compute.host.metrics.read", "compute.host.metrics.write"),
  },
  {
    id: "disk.wait",
    group: "disk",
    titleKey: "compute.host.metrics.diskLatency",
    unit: "sec",
    names: ["disk.read_wait_sec", "disk.write_wait_sec"],
    label: devDir("disk.read_wait_sec", "compute.host.metrics.read", "compute.host.metrics.write"),
  },
  {
    id: "disk.util",
    group: "disk",
    titleKey: "compute.host.metrics.diskUtil",
    unit: "pct",
    names: ["disk.util_pct"],
    label: dev,
  },
  {
    id: "psi.io",
    group: "disk",
    titleKey: "compute.host.metrics.psiIO",
    unit: "pct",
    names: ["psi.io_pct"],
    label: one("compute.host.metrics.stalled"),
  },

  // --- network ---
  {
    id: "net.bps",
    group: "network",
    titleKey: "compute.host.metrics.network",
    unit: "bps",
    names: ["net.rx_bps", "net.tx_bps"],
    label: devRxTx("net.rx_bps"),
  },
  {
    id: "net.pps",
    group: "network",
    titleKey: "compute.host.metrics.packets",
    unit: "ops",
    names: ["net.rx_pps", "net.tx_pps"],
    label: devRxTx("net.rx_pps"),
  },
  {
    id: "net.errs",
    group: "network",
    titleKey: "compute.host.metrics.netErrors",
    unit: "ops",
    names: ["net.rx_errs", "net.tx_errs", "net.rx_drops", "net.tx_drops"],
    label: (name, l) => `${l?.device ?? "-"} ${name.replace("net.", "").replace("_", " ")}`,
  },
  {
    id: "tcp.retrans",
    group: "network",
    titleKey: "compute.host.metrics.tcpRetrans",
    unit: "ops",
    names: ["tcp.retrans"],
    label: one("compute.host.metrics.tcpRetrans"),
  },
  {
    id: "sockets",
    group: "network",
    titleKey: "compute.host.metrics.sockets",
    unit: "plain",
    names: ["tcp.in_use", "sockets.used"],
    label: (name) => (name === "tcp.in_use" ? "TCP" : translate("compute.host.metrics.socketsAll")),
  },
  {
    id: "conntrack",
    group: "network",
    titleKey: "compute.host.metrics.conntrack",
    unit: "pct",
    names: ["conntrack.used_pct"],
    label: one("compute.host.metrics.used"),
  },

  // --- system ---
  {
    id: "sys.procs",
    group: "system",
    titleKey: "compute.host.metrics.processes",
    unit: "plain",
    names: ["sys.procs_running", "sys.procs_blocked"],
    label: (name) =>
      name === "sys.procs_running"
        ? translate("compute.host.metrics.running")
        : translate("compute.host.metrics.blocked"),
  },
  {
    id: "sys.fd",
    group: "system",
    titleKey: "compute.host.metrics.fileDescriptors",
    unit: "pct",
    names: ["sys.fd_used_pct"],
    label: one("compute.host.metrics.used"),
  },
  {
    id: "sys.uptime",
    group: "system",
    titleKey: "compute.host.metrics.uptime",
    unit: "plain",
    names: ["sys.uptime_sec"],
    // Seconds divided into days at the pick site would need a transform
    // the pipeline does not have; the tooltip's raw seconds is exact and
    // a restart is visible as the cliff regardless of the unit.
    label: one("compute.host.metrics.uptime"),
  },
  {
    id: "sys.clock",
    group: "system",
    titleKey: "compute.host.metrics.clock",
    unit: "sec",
    names: ["sys.time_drift_sec"],
    label: one("compute.host.metrics.drift"),
  },
  {
    id: "sys.temp",
    group: "system",
    titleKey: "compute.host.metrics.temperature",
    unit: "celsius",
    names: ["sys.temp_celsius"],
    label: (_, l) => [l?.chip, l?.sensor].filter(Boolean).join(" ") || "-",
  },
]

// The default group: the charts an operator opens the page for. Not a
// summary of each group -- a shortlist, so the first screen answers "is
// this host healthy" without a click.
//
// Labelled "key metrics" rather than "overview" because the page already
// has an Overview TAB, and two controls with the same word one level
// apart is a reader guessing which is which.
const OVERVIEW_IDS = new Set(["cpu.used", "load", "mem.used", "fs.used", "disk.io", "net.bps"])

export function chartsFor(group: GroupKey): ChartDef[] {
  if (group === "overview") return CHARTS.filter((c) => OVERVIEW_IDS.has(c.id))
  return CHARTS.filter((c) => c.group === group)
}
