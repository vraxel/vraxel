import { describe, it, expect } from "vitest"
import { hostFindings } from "../host-findings"
import type { Host, HostSpec } from "@/modules/compute/api/types"

function host(spec: Partial<HostSpec>): Host {
  return {
    apiVersion: "compute/v1",
    metadata: { id: "1", name: "lcp" },
    spec,
  } as unknown as Host
}

describe("hostFindings", () => {
  it("finds nothing on a healthy host", () => {
    expect(
      hostFindings(
        host({
          clockSync: "synced",
          nics: [{ name: "eth0", duplex: "full" }],
          filesystems: [{ mount: "/", usedBytes: 10, sizeBytes: 100 }],
        }),
      ),
    ).toEqual({})
  })

  it("counts an unsynced clock as a warning, not a fault", () => {
    expect(hostFindings(host({ clockSync: "unsynced" })).overview).toEqual({
      count: 1,
      hot: false,
    })
  })

  it("counts each half-duplex link", () => {
    const f = hostFindings(
      host({
        nics: [
          { name: "eth0", duplex: "half" },
          { name: "eth1", duplex: "full" },
          { name: "eth2", duplex: "half" },
        ],
      }),
    )
    expect(f.network).toEqual({ count: 2, hot: false })
  })

  it("counts a filesystem that is full, read-only, or out of inodes", () => {
    const f = hostFindings(
      host({
        filesystems: [
          { mount: "/", usedBytes: 95, sizeBytes: 100 },
          { mount: "/boot", readOnly: true },
          // The case the byte gauge cannot show: half empty and unable to
          // accept another file.
          { mount: "/var", usedBytes: 50, sizeBytes: 100, inodesUsed: 99, inodesTotal: 100 },
          { mount: "/home", usedBytes: 50, sizeBytes: 100 },
        ],
      }),
    )
    expect(f.storage).toEqual({ count: 3, hot: true })
  })

  it("does not count a filesystem whose size could not be read", () => {
    // statfs failed, so usedBytes/sizeBytes are absent. That is unknown,
    // not full, and badging it would make every such mount look critical.
    expect(hostFindings(host({ filesystems: [{ mount: "/mnt/nfs" }] })).storage).toBeUndefined()
  })

  it("surfaces firing alerts, which nothing consumed before", () => {
    expect(hostFindings(host({ alertsFiring: 2 })).metrics).toEqual({ count: 2, hot: true })
  })

  it("ignores vulnerable cpu mitigations", () => {
    // Nearly every machine is vulnerable to something the kernel chose
    // not to mitigate; badging it would light the tab permanently.
    expect(
      hostFindings(host({ cpuMitigations: [{ name: "spectre_v2", status: "Vulnerable" }] })),
    ).toEqual({})
  })
})
