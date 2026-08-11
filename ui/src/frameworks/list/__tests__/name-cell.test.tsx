import { describe, expect, it } from "vitest"
import { render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router"
import { TooltipProvider } from "@/shared/ui/tooltip"
import { NameCell } from "../name-cell"

// Both lines truncate through TruncateText, whose Radix tooltip needs a
// provider -- RootLayout supplies one in the app.
function renderCell(props: Parameters<typeof NameCell>[0]) {
  return render(
    <TooltipProvider>
      <MemoryRouter>
        <NameCell {...props} />
      </MemoryRouter>
    </TooltipProvider>,
  )
}

describe("NameCell", () => {
  it("links the display name and shows the name beneath it", () => {
    renderCell({ to: "/iam/roles/1", displayName: "平台管理员", name: "platform-admin" })
    const link = screen.getByRole("link", { name: "平台管理员" })
    expect(link).toHaveAttribute("href", "/iam/roles/1")
    expect(screen.getByText("platform-admin")).toBeInTheDocument()
  })

  // Degenerate cases must collapse to one line: a dash or a repeated
  // string would be noise in every row that lacks a display name.
  it("falls back to the name, still linked, when the display name is blank", () => {
    renderCell({ to: "/iam/roles/1", displayName: "   ", name: "platform-admin" })
    expect(screen.getByRole("link", { name: "platform-admin" })).toBeInTheDocument()
    expect(screen.getAllByText("platform-admin")).toHaveLength(1)
  })

  it("does not repeat the name when it equals the display name", () => {
    renderCell({ to: "/iam/roles/1", displayName: "platform-admin", name: "platform-admin" })
    expect(screen.getAllByText("platform-admin")).toHaveLength(1)
  })

  it("renders plain text when no detail route is given", () => {
    renderCell({ displayName: "Admin", name: "admin" })
    expect(screen.queryByRole("link")).toBeNull()
    expect(screen.getByText("Admin")).toBeInTheDocument()
    expect(screen.getByText("admin")).toBeInTheDocument()
  })
})
