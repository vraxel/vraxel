// Regression: the login page must survive an effect running twice.
// StrictMode double-invokes effects in dev, and the flow marker used to
// be CONSUMED here -- the second run saw no marker, called
// startAuthFlow(), and the fresh request_id re-triggered the same
// effect, refreshing the page forever.
import { describe, expect, it, vi, beforeEach } from "vitest"
import { render } from "@testing-library/react"
import { StrictMode } from "react"
import { MemoryRouter } from "react-router"

// vi.mock is hoisted above the imports, so the factory cannot close over
// a normal top-level const -- vi.hoisted lifts the spy with it.
const { startAuthFlow } = vi.hoisted(() => ({ startAuthFlow: vi.fn() }))
vi.mock("@/core/auth/auth", () => ({
  startAuthFlow,
  loginWithCredentials: vi.fn(),
  fetchAuthConfig: vi.fn().mockResolvedValue({ selfRegistration: false, socialProviders: [] }),
  socialLoginUrl: vi.fn(),
}))

import LoginPage from "../login"

function renderAt(url: string) {
  return render(
    <StrictMode>
      <MemoryRouter initialEntries={[url]}>
        <LoginPage />
      </MemoryRouter>
    </StrictMode>,
  )
}

describe("login page auth-flow guard", () => {
  beforeEach(() => {
    startAuthFlow.mockClear()
    sessionStorage.clear()
  })

  it("does not restart the flow when this session initiated it", () => {
    sessionStorage.setItem("oidc_flow_pending", "1")
    renderAt("/login?request_id=abc")
    expect(startAuthFlow).not.toHaveBeenCalled()
  })

  it("leaves the marker in place across re-renders (idempotent read)", () => {
    sessionStorage.setItem("oidc_flow_pending", "1")
    const { rerender } = renderAt("/login?request_id=abc")
    rerender(
      <StrictMode>
        <MemoryRouter initialEntries={["/login?request_id=abc"]}>
          <LoginPage />
        </MemoryRouter>
      </StrictMode>,
    )
    expect(sessionStorage.getItem("oidc_flow_pending")).toBe("1")
    expect(startAuthFlow).not.toHaveBeenCalled()
  })

  it("starts a flow for a stale request_id (no marker)", () => {
    renderAt("/login?request_id=stale")
    expect(startAuthFlow).toHaveBeenCalled()
  })

  it("starts a flow when the URL carries no request_id", () => {
    renderAt("/login")
    expect(startAuthFlow).toHaveBeenCalled()
  })
})
