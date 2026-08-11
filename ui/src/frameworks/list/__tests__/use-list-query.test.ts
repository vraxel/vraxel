import { describe, expect, it, vi } from "vitest"
import { renderHook, act, waitFor } from "@testing-library/react"
import { createElement, type ReactNode } from "react"
import { MemoryRouter } from "react-router"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { useListQuery } from "../use-list-query"
import { defineResource } from "@/core/registry/resource"

const def = defineResource({ module: "pki", name: "credentials", scopes: ["platform"] })

function wrapper(initialEntries: string[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: ReactNode }) =>
    createElement(
      MemoryRouter,
      { initialEntries },
      createElement(QueryClientProvider, { client: qc }, children),
    )
}

interface Row {
  metadata: { id: string }
}
function fakeApi(seen: { params?: object }[]) {
  return {
    list: vi.fn(async (_s: unknown, params?: object) => {
      seen.push({ params })
      return { items: [{ metadata: { id: "1" } }] as Row[], totalCount: 1 }
    }),
  }
}

describe("useListQuery", () => {
  it("reads page/size/sort/search/filters from the URL", async () => {
    const seen: { params?: object }[] = []
    const api = fakeApi(seen)
    const { result } = renderHook(
      () => useListQuery<Row>({ def, api, scope: {}, filterKeys: ["status"] }),
      { wrapper: wrapper(["/?page=3&pageSize=50&sortBy=name&sortOrder=asc&q=db&status=active"]) },
    )
    await waitFor(() => expect(result.current.rows.length).toBe(1))
    expect(result.current.page).toBe(3)
    expect(result.current.pageSize).toBe(50)
    expect(result.current.sortBy).toBe("name")
    expect(result.current.sortOrder).toBe("asc")
    expect(result.current.search).toBe("db")
    expect(result.current.filters.status).toBe("active")
    expect(seen.at(-1)?.params).toMatchObject({
      page: 3,
      pageSize: 50,
      sortBy: "name",
      sortOrder: "asc",
      search: "db",
      status: "active",
    })
  })

  it("changing a filter resets page to 1 (single place -- no double request race)", async () => {
    const seen: { params?: object }[] = []
    const api = fakeApi(seen)
    const { result } = renderHook(
      () => useListQuery<Row>({ def, api, scope: {}, filterKeys: ["status"] }),
      { wrapper: wrapper(["/?page=5"]) },
    )
    await waitFor(() => expect(result.current.rows.length).toBe(1))
    act(() => result.current.setFilter("status", "active"))
    await waitFor(() => expect(result.current.page).toBe(1))
    expect(result.current.filters.status).toBe("active")
  })

  it("sort toggles asc<->desc on the same field, resets to asc on a new field", async () => {
    const api = fakeApi([])
    const { result } = renderHook(
      () =>
        useListQuery<Row>({
          def,
          api,
          scope: {},
          defaultSortBy: "created_at",
          defaultSortOrder: "desc",
        }),
      { wrapper: wrapper(["/?sortBy=name&sortOrder=asc"]) },
    )
    await waitFor(() => expect(result.current.rows.length).toBe(1))
    act(() => result.current.handleSort("name"))
    await waitFor(() => expect(result.current.sortOrder).toBe("desc"))
    act(() => result.current.handleSort("username"))
    await waitFor(() => expect(result.current.sortBy).toBe("username"))
    expect(result.current.sortOrder).toBe("asc")
  })

  it("selection toggles and clears; omits empty filter params", async () => {
    const seen: { params?: object }[] = []
    const api = fakeApi(seen)
    const { result } = renderHook(
      () => useListQuery<Row>({ def, api, scope: {}, filterKeys: ["status"] }),
      { wrapper: wrapper(["/"]) },
    )
    await waitFor(() => expect(result.current.rows.length).toBe(1))
    expect(seen.at(-1)?.params).not.toHaveProperty("status")
    act(() => result.current.toggleOne("1"))
    expect(result.current.selected.has("1")).toBe(true)
    act(() => result.current.clearSelection())
    expect(result.current.selected.size).toBe(0)
  })
})
