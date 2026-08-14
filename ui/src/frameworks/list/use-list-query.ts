import { keepPreviousData } from "@tanstack/react-query"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useSearchParams } from "react-router"
import { useApiQuery } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import type { ResourceDef, ScopeRef } from "@/core/registry/resource"
import type { ListParams } from "@/core/api/types"

export const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export interface ListRow {
  metadata: { id: string }
}

// Structural, so any module's List type (which extends TypeMeta with its
// own item type) satisfies it without a nominal match.
interface ListApi<T> {
  list: (s: ScopeRef, params?: ListParams) => Promise<{ items: T[]; totalCount: number }>
}

/** A filter key, optionally with dependent keys to clear when it changes. */
export type FilterKeySpec = string | { key: string; resets?: readonly string[] }

export interface UseListQueryOptions<T> {
  def: ResourceDef
  api: ListApi<T>
  scope: ScopeRef
  /**
   * Filter param keys; each is read from / written to the URL as
   * "all"=unset. A `{ key, resets }` entry clears the `resets` keys when
   * `key` changes -- for cascading filters (e.g. region -> site -> rack),
   * without moving them out of the column headers.
   */
  filterKeys?: readonly FilterKeySpec[]
  defaultSortBy?: string
  defaultSortOrder?: "asc" | "desc"
  defaultPageSize?: number
  /** Fixed params merged into every request (never in the URL). */
  extraParams?: Record<string, string | number | boolean | undefined>
  /** When false, the list query does not run (e.g. gated behind a permission). */
  enabled?: boolean
}

/**
 * The list-page data engine (frameworks/list). Merges URL-backed list
 * state (page/size/sort/search/filters -- survives refresh, back-nav and
 * link sharing, plan.md D11) with a TanStack Query fetch keyed by scope +
 * params (qk.list), so a stale response can never clobber a newer one.
 *
 * The "reset page to 1 when a filter/search changes" rule lives here, in
 * ONE place (setFilter / the debounced search commit), which is what
 * structurally removes the pre-refactor double-request race (analysis.md
 * 根因 B): the page no longer runs a second effect that re-derives page
 * after the first request already fired.
 *
 * Selection is deliberately NOT in the URL (transient, per-view).
 */
export function useListQuery<T extends ListRow>(opts: UseListQueryOptions<T>) {
  const {
    def,
    api,
    scope,
    filterKeys: filterSpecs = [],
    defaultSortBy = "created_at",
    defaultSortOrder = "desc",
    defaultPageSize = 20,
    extraParams,
    enabled = true,
  } = opts
  const filterKeys = filterSpecs.map((f) => (typeof f === "string" ? f : f.key))
  // filterSpecs is a fresh array literal on every caller render; key the
  // memo on its JSON form so the cascade map (and thus setFilter) stays
  // referentially stable for a given spec shape.
  const specsJson = JSON.stringify(filterSpecs)
  const resetsByKey = useMemo(() => {
    const specs: readonly FilterKeySpec[] = JSON.parse(specsJson)
    const map: Record<string, readonly string[]> = {}
    for (const f of specs) if (typeof f !== "string" && f.resets) map[f.key] = f.resets
    return map
  }, [specsJson])
  const [sp, setSp] = useSearchParams()

  const page = Number(sp.get("page") ?? 1) || 1
  const pageSize = Number(sp.get("page_size") ?? defaultPageSize) || defaultPageSize
  const sortBy = sp.get("sort_by") ?? defaultSortBy
  const sortOrder = (sp.get("sort_order") as "asc" | "desc" | null) ?? defaultSortOrder
  const search = sp.get("q") ?? ""
  const filters: Record<string, Set<string>> = {}
  for (const k of filterKeys) {
    const raw = sp.get(k)
    filters[k] = raw ? new Set(raw.split(",")) : new Set()
  }

  // Search input is local + immediate; the URL commits after a 300ms
  // debounce, resetting page there (single place).
  const [searchInput, setSearchInput] = useState(search)
  const searchTimer = useRef<ReturnType<typeof setTimeout>>(null)
  const commitSearch = useCallback(
    (value: string) => {
      setSp(
        (prev) => {
          const next = new URLSearchParams(prev)
          if (value) next.set("q", value)
          else next.delete("q")
          next.set("page", "1")
          return next
        },
        { replace: true },
      )
    },
    [setSp],
  )
  useEffect(() => {
    searchTimer.current = setTimeout(() => {
      if (searchInput !== search) commitSearch(searchInput)
    }, 300)
    return () => {
      if (searchTimer.current) clearTimeout(searchTimer.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchInput])
  // External URL changes (browser back/forward, sidebar links) must flow back
  // into the input; otherwise the box keeps showing a stale term while the
  // list is actually unfiltered. Adjust-during-render (not an effect) so the
  // correction lands in the same render pass; after a normal commit
  // search === searchInput, so this branch is skipped while typing.
  const [prevSearch, setPrevSearch] = useState(search)
  if (prevSearch !== search) {
    setPrevSearch(search)
    setSearchInput(search)
  }

  const patchParams = useCallback(
    (patch: Record<string, string | null>, resetPage = false) => {
      setSp(
        (prev) => {
          const next = new URLSearchParams(prev)
          for (const [k, v] of Object.entries(patch)) {
            if (v === null || v === "") next.delete(k)
            else next.set(k, v)
          }
          if (resetPage) next.set("page", "1")
          return next
        },
        { replace: true },
      )
    },
    [setSp],
  )

  const setPage = useCallback((p: number) => patchParams({ page: String(p) }), [patchParams])
  const setPageSize = useCallback(
    (s: number) => patchParams({ page_size: String(s) }, true),
    [patchParams],
  )
  const handleSort = useCallback(
    (field: string) => {
      const nextOrder = sortBy === field && sortOrder === "asc" ? "desc" : "asc"
      // Clicking a new field starts at asc; clicking the current flips.
      patchParams({ sort_by: field, sort_order: sortBy === field ? nextOrder : "asc" })
    },
    [sortBy, sortOrder, patchParams],
  )
  const setFilter = useCallback(
    (key: string, value: Set<string>) => {
      const csv = Array.from(value).sort().join(",")
      const patch: Record<string, string | null> = { [key]: csv || null }
      for (const dep of resetsByKey[key] ?? []) patch[dep] = null
      patchParams(patch, true)
    },
    [patchParams, resetsByKey],
  )

  // Selection (transient).
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const toggleAll = useCallback(
    (ids: string[]) => {
      setSelected((prev) => (prev.size === ids.length ? new Set() : new Set(ids)))
    },
    [setSelected],
  )
  const toggleOne = useCallback(
    (id: string) => {
      setSelected((prev) => {
        const next = new Set(prev)
        if (next.has(id)) next.delete(id)
        else next.add(id)
        return next
      })
    },
    [setSelected],
  )
  const clearSelection = useCallback(() => setSelected(new Set()), [setSelected])

  const params: ListParams = { page, page_size: pageSize, sort_by: sortBy, sort_order: sortOrder }
  if (search) params.search = search
  for (const k of filterKeys) {
    const s = filters[k]
    if (s.size > 0) (params as Record<string, unknown>)[k] = Array.from(s).sort().join(",")
  }
  if (extraParams)
    for (const [k, v] of Object.entries(extraParams))
      if (v !== undefined) (params as Record<string, unknown>)[k] = v

  const query = useApiQuery({
    queryKey: qk.list(def, scope, params),
    queryFn: () => api.list(scope, params),
    enabled,
    placeholderData: keepPreviousData,
  })
  const rows = useMemo(() => query.data?.items ?? [], [query.data])
  const totalCount = query.data?.totalCount ?? 0

  // Drop selected ids no longer present after a refetch. Adjusted during
  // render (keyed on the response identity) instead of an effect, so the
  // pruned selection is what this very render paints.
  const [prevData, setPrevData] = useState(query.data)
  if (prevData !== query.data) {
    setPrevData(query.data)
    setSelected((prev) => {
      if (prev.size === 0) return prev
      const present = new Set(rows.map((r) => r.metadata.id))
      const keep = new Set<string>()
      for (const id of prev) if (present.has(id)) keep.add(id)
      return keep.size === prev.size ? prev : keep
    })
  }

  return {
    rows,
    totalCount,
    isPending: query.isPending,
    isFetching: query.isFetching,
    error: query.error,
    refetch: query.refetch,
    page,
    setPage,
    pageSize,
    setPageSize,
    sortBy,
    sortOrder,
    handleSort,
    search,
    searchInput,
    setSearchInput,
    filters,
    setFilter,
    selected,
    toggleAll,
    toggleOne,
    clearSelection,
    params,
    scope,
  }
}

export type ListQuery<T extends ListRow> = ReturnType<typeof useListQuery<T>>
