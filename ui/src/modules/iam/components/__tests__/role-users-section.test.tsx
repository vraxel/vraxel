import { describe, expect, it, vi } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"

// Assert against i18n keys, not the localized text, so wording changes
// don't break these behavior tests.
vi.mock("@/i18n", () => ({
  useTranslation: () => ({ t: (k: string) => k }),
  translate: (k: string) => k,
}))
import { MemoryRouter } from "react-router"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { TooltipProvider } from "@/shared/ui/tooltip"
import { RoleUsersSection, type RoleUsersConfig } from "../role-users-section"
import type { RoleBindingList } from "@/modules/iam/api/types"

function binding(id: string, userId: string, username: string, isOwner = false) {
  return {
    apiVersion: "iam/v1",
    kind: "RoleBinding",
    metadata: {
      id,
      name: id,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    },
    spec: { userId, username, roleId: "1", scope: "platform" as const, isOwner },
  }
}

function renderSection(
  bindings: ReturnType<typeof binding>[],
  overrides: Partial<RoleUsersConfig> = {},
) {
  const list: RoleBindingList = {
    apiVersion: "iam/v1",
    kind: "RoleBindingList",
    items: bindings,
    totalCount: bindings.length,
  }
  const config: RoleUsersConfig = {
    roleId: "1",
    detailPrefix: "/iam",
    listBindings: vi.fn(async () => list),
    listCandidates: vi.fn(async () => ({
      apiVersion: "iam/v1",
      kind: "UserList",
      items: [],
      totalCount: 0,
    })),
    assign: vi.fn(async () => ({ successCount: 0, failedCount: 0 })),
    revoke: vi.fn(async () => {}),
    canAssign: true,
    canRevoke: true,
    cacheKey: ["platform", "1"],
    ...overrides,
  }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <MemoryRouter>
          <RoleUsersSection config={config} />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  )
}

describe("RoleUsersSection", () => {
  it("lists users with this role, linking each to the user detail", async () => {
    renderSection([binding("b1", "10", "alice"), binding("b2", "20", "bob")])
    expect(await screen.findByText("alice")).toBeInTheDocument()
    expect(screen.getByText("bob")).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "alice" })).toHaveAttribute("href", "/iam/users/10")
  })

  it("empty state when the role has no users", async () => {
    renderSection([])
    expect(await screen.findByText("role.noUsers")).toBeInTheDocument()
  })

  it("disables revoke for the owner binding", async () => {
    renderSection([binding("b1", "10", "alice", true)])
    await screen.findByText("alice")
    const revoke = screen.getByRole("button", { name: "rolebinding.ownerLocked" })
    expect(revoke).toBeDisabled()
  })

  it("hides revoke entirely without the delete permission", async () => {
    renderSection([binding("b1", "10", "alice")], { canRevoke: false })
    await screen.findByText("alice")
    expect(screen.queryByRole("button", { name: "rolebinding.revoke" })).toBeNull()
  })

  it("hides the assign button without the create permission", async () => {
    renderSection([], { canAssign: false })
    await waitFor(() => expect(screen.getByText("role.noUsers")).toBeInTheDocument())
    expect(screen.queryByText("role.assignUsers")).toBeNull()
  })
})
