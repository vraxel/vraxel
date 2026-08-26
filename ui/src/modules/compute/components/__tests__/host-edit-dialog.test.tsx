import { vi, describe, it, expect } from "vitest"
import { fireEvent, render, screen } from "@testing-library/react"

vi.mock("@/modules/compute/api/hosts", () => ({
  hostsApi: { update: vi.fn() },
}))
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

import { HostEditDialog } from "../host-edit-dialog"
import type { Host } from "@/modules/compute/api/types"

// A fresh object each call, exactly like a refetch: same host, new
// identity, and one field the poll would have updated underneath.
function host(id: string, cpuUsedPct: number): Host {
  return {
    apiVersion: "compute/v1",
    metadata: { id, name: "lcp" },
    spec: { displayName: "lcp", description: "", cpuUsedPct },
  } as unknown as Host
}

function nameInput(): HTMLInputElement {
  return screen.getByLabelText(/^显示名称/, { selector: "input" }) as HTMLInputElement
}

// React-controlled inputs need the native setter so the value tracker sees it.
function setValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set
  setter?.call(input, value)
  fireEvent.input(input)
}

describe("HostEditDialog", () => {
  // The detail page behind this dialog refetches every 15s to keep its
  // utilisation gauges live, and every response is a new object. Seeding
  // the form off that object wiped whatever was typed in it, on a timer.
  it("keeps what was typed when the host object is replaced by a refetch", () => {
    const props = { scope: {}, onClose: vi.fn(), onSuccess: vi.fn() } as never
    const { rerender } = render(<HostEditDialog host={host("24", 1)} {...props} />)

    setValue(nameInput(), "renamed by hand")
    expect(nameInput().value).toBe("renamed by hand")

    rerender(<HostEditDialog host={host("24", 42)} {...props} />)
    expect(nameInput().value).toBe("renamed by hand")
  })

  // Closing passes null and reopening passes the host again, which is the
  // only signal the dialog gets that it has a new subject.
  it("reseeds from the host when it is reopened", () => {
    const props = { scope: {}, onClose: vi.fn(), onSuccess: vi.fn() } as never
    const { rerender } = render(<HostEditDialog host={host("24", 1)} {...props} />)

    setValue(nameInput(), "abandoned edit")
    rerender(<HostEditDialog host={null} {...props} />)
    rerender(<HostEditDialog host={host("24", 1)} {...props} />)

    expect(nameInput().value).toBe("lcp")
  })
})
