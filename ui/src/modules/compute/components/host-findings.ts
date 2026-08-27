import type { Host } from "@/modules/compute/api/types"

/**
 * What is wrong with this host, grouped by the tab that shows it.
 *
 * Tabs hide things. That is what they are for, and it is also how a host
 * with a read-only root filesystem ends up looking clean: the header
 * carries three banners, all of them about the AGENT's identity (image
 * group, credential conflict, foreign machine), and nothing about the
 * machine's own health. An operator has to open all six tabs to learn
 * there is nothing to find.
 *
 * So every finding a tab can show gets counted here and shown ON the tab.
 * The rule for what belongs: it must be derivable from the host record
 * the page already has. The processes and accounts tabs fetch on mount --
 * deliberately, for cost and for permissions -- so their findings (a
 * failed unit, an sshd that permits root with a password) cannot be
 * counted without defeating that, and those two tabs carry no badge.
 *
 * Not counted, though the data is here: cpuMitigations reporting
 * "vulnerable". Nearly every machine in a fleet is vulnerable to
 * something the kernel has decided not to mitigate by default, so it
 * would badge every host permanently, and a badge that is always lit
 * says nothing. It stays a detail inside the kernel card, which is where
 * somebody auditing mitigations will look for it.
 */
export type TabFinding = {
  count: number
  /** Something is broken now, as opposed to something worth a look. */
  hot: boolean
}

export type HostFindings = Partial<
  Record<"overview" | "network" | "storage" | "metrics", TabFinding>
>

// The same threshold the storage tab paints red at, so a badge and the
// row it points to cannot disagree.
const hotPct = 90

function pct(used?: number, total?: number): number | undefined {
  if (typeof used !== "number" || typeof total !== "number" || total <= 0) return undefined
  return (used / total) * 100
}

export function hostFindings(host: Host): HostFindings {
  const s = host.spec
  const out: HostFindings = {}

  // An unsynced clock is not cosmetic: every timestamp this page shows
  // comes from the agent, so a machine that has stopped disciplining its
  // clock makes our own judgement of what is fresh unreliable. Warn, not
  // hot -- nothing is down.
  if (s.clockSync === "unsynced") out.overview = { count: 1, hot: false }

  // Half duplex on a server link is a negotiation failure, and it reads
  // as unexplained latency in every application on the box.
  const halfDuplex = (s.nics ?? []).filter((n) => n.duplex === "half").length
  if (halfDuplex > 0) out.network = { count: halfDuplex, hot: false }

  // Read-only is the kernel giving up on a disk with services still
  // writing to it. Inodes are counted beside bytes because they run out
  // independently: writes fail while the byte gauge still reads half
  // empty.
  const badFs = (s.filesystems ?? []).filter(
    (f) =>
      f.readOnly ||
      (pct(f.usedBytes, f.sizeBytes) ?? 0) >= hotPct ||
      (pct(f.inodesUsed, f.inodesTotal) ?? 0) >= hotPct,
  ).length
  if (badFs > 0) out.storage = { count: badFs, hot: true }

  // Already computed by the server on every list and get -- a scalar
  // subquery over host_alert_states, which holds only live problems --
  // and until now read by nothing.
  if ((s.alertsFiring ?? 0) > 0) out.metrics = { count: s.alertsFiring ?? 0, hot: true }

  return out
}
