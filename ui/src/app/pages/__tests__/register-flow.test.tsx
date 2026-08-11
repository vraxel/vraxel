// The register page mirrors the login page's request_id handshake guard:
// with a live flow marker it must NOT restart the flow, and without one (stale
// or direct navigation) it must kick a fresh OIDC handshake.
import { describe, expect, it, vi, beforeEach } from "vitest"
import { render } from "@testing-library/react"
import { StrictMode } from "react"
import { MemoryRouter } from "react-router"

const { startAuthFlow } = vi.hoisted(() => ({ startAuthFlow: vi.fn() }))
vi.mock("@/core/auth/auth", () => ({
  startAuthFlow,
  registerWithCredentials: vi.fn(),
  fetchAuthConfig: vi.fn().mockResolvedValue({ selfRegistration: true, socialProviders: [] }),
  socialLoginUrl: vi.fn(),
}))

import RegisterPage from "../register"

function renderAt(url: string) {
  return render(
    <StrictMode>
      <MemoryRouter initialEntries={[url]}>
        <RegisterPage />
      </MemoryRouter>
    </StrictMode>,
  )
}

describe("register page auth-flow guard", () => {
  beforeEach(() => {
    startAuthFlow.mockClear()
    sessionStorage.clear()
  })

  it("does not restart the flow when this session initiated it", () => {
    sessionStorage.setItem("oidc_flow_pending", "1")
    renderAt("/register?request_id=abc")
    expect(startAuthFlow).not.toHaveBeenCalled()
  })

  it("starts a flow for a stale request_id (no marker)", () => {
    renderAt("/register?request_id=stale")
    expect(startAuthFlow).toHaveBeenCalled()
  })

  it("starts a flow when the URL carries no request_id", () => {
    renderAt("/register")
    expect(startAuthFlow).toHaveBeenCalled()
  })
})
