import { describe, it, expect, vi, beforeEach } from "vitest"

// Keep the real ApiError (the handler uses instanceof); replace only
// showApiError so we can assert whether the global handler surfaced the error.
vi.mock("@/core/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/core/api/client")>()
  return { ...actual, showApiError: vi.fn() }
})

import { ApiError, showApiError } from "@/core/api/client"
import type { StatusResponse } from "@/core/api/types"
import { queryClient } from "../client"

const onError = queryClient.getQueryCache().config.onError!
// The handler only reads .meta and .queryHash off the query. A unique
// queryHash per case avoids the module-level cooldown Map cross-contaminating
// tests (each distinct query gets its own cooldown slot).
const q = (queryHash: string, meta?: Record<string, unknown>) =>
  ({ meta, queryHash }) as unknown as Parameters<typeof onError>[1]
const apiErr = (status: number) =>
  new ApiError({ status, reason: "x", message: "boom" } as StatusResponse)

describe("queryClient global error handler", () => {
  beforeEach(() => vi.clearAllMocks())

  it("surfaces a generic ApiError (500)", () => {
    onError(apiErr(500), q("k-500"))
    expect(showApiError).toHaveBeenCalledTimes(1)
  })

  it("surfaces a non-ApiError", () => {
    onError(new Error("boom"), q("k-nonapi"))
    expect(showApiError).toHaveBeenCalledTimes(1)
  })

  it("skips 404 (detail pages render their own not-found UI)", () => {
    onError(apiErr(404), q("k-404"))
    expect(showApiError).not.toHaveBeenCalled()
  })

  it("skips 401 (auth layer handles refresh / redirect)", () => {
    onError(apiErr(401), q("k-401"))
    expect(showApiError).not.toHaveBeenCalled()
  })

  it("skips when a query opts out via meta.skipGlobalError", () => {
    onError(apiErr(500), q("k-optout", { skipGlobalError: true }))
    expect(showApiError).not.toHaveBeenCalled()
  })

  it("forwards meta.errorResourceKey to showApiError", () => {
    onError(apiErr(500), q("k-reskey", { errorResourceKey: "user.title" }))
    expect(showApiError).toHaveBeenCalledWith(expect.anything(), expect.any(Function), "user.title")
  })

  it("throttles repeated failures of the same query (no toast spam on polling)", () => {
    onError(apiErr(500), q("k-poll"))
    onError(apiErr(500), q("k-poll"))
    onError(apiErr(503), q("k-poll"))
    expect(showApiError).toHaveBeenCalledTimes(1)
  })
})
