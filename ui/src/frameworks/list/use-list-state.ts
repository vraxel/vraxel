import { useCallback, useEffect, useRef, useState } from "react"

export const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export interface UseListStateOptions {
  defaultSortBy?: string
  defaultSortOrder?: "asc" | "desc"
  defaultPageSize?: number
}

export function useListState(options: UseListStateOptions = {}) {
  const {
    defaultSortBy = "created_at",
    defaultSortOrder = "desc",
    defaultPageSize = 20,
  } = options

  const [page, setPageRaw] = useState(1)
  const [pageSize, setPageSizeRaw] = useState(defaultPageSize)
  const [sortBy, setSortBy] = useState(defaultSortBy)
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">(defaultSortOrder)
  const [searchInput, setSearchInput] = useState("")
  const [search, setSearch] = useState("")
  const [selected, setSelected] = useState<Set<string>>(new Set())

  // Debounce search. Reset page=1 alongside the debounced commit so an
  // active search from page 5 doesn't keep paginating past the new
  // (smaller) result set's last page and show "empty" with no signal.
  const searchTimer = useRef<ReturnType<typeof setTimeout>>(null)
  useEffect(() => {
    searchTimer.current = setTimeout(() => {
      setSearch((prev) => {
        if (prev !== searchInput) setPageRaw(1)
        return searchInput
      })
    }, 300)
    return () => { if (searchTimer.current) clearTimeout(searchTimer.current) }
  }, [searchInput])

  const sortByRef = useRef(defaultSortBy)
  const sortOrderRef = useRef<"asc" | "desc">(defaultSortOrder)
  // 2-state toggle: clicking the currently-sorted field flips asc<->desc;
  // clicking a different field switches to it and starts at asc. The
  // previous 3-state cycle (asc -> desc -> "reset to default") collapsed
  // to a no-op whenever the clicked field was already the default sort
  // (e.g. defaultSortBy="name", clicking the "name" header): the
  // "isCurrentField && !isDefaultState" guard was false because the
  // current state IS the default, so it fell into the else branch which
  // set the same field+asc again — clicking "name" never toggled to
  // desc. Other columns worked only because they weren't the default.
  // Bug #214; affects every list page in the repo via this shared hook.
  const handleSort = useCallback((field: string) => {
    if (sortByRef.current === field) {
      const next = sortOrderRef.current === "asc" ? "desc" : "asc"
      sortOrderRef.current = next
      setSortOrder(next)
    } else {
      sortByRef.current = field
      sortOrderRef.current = "asc"
      setSortBy(field)
      setSortOrder("asc")
    }
  }, [])

  const toggleAll = useCallback((ids: string[]) => {
    setSelected((prev) => prev.size === ids.length ? new Set() : new Set(ids))
  }, [])

  const toggleOne = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }, [])

  const clearSelection = useCallback(() => setSelected(new Set()), [])

  // Drop any selected IDs that are no longer in `currentIds`. Use after a
  // refresh (e.g. single-row delete) to keep the batch-action counter honest.
  const syncSelection = useCallback((currentIds: string[]) => {
    setSelected((prev) => {
      if (prev.size === 0) return prev
      const keep = new Set<string>()
      const current = new Set(currentIds)
      for (const id of prev) if (current.has(id)) keep.add(id)
      return keep.size === prev.size ? prev : keep
    })
  }, [])

  const setPage = useCallback((p: number | ((prev: number) => number)) => {
    setPageRaw(p)
    setSelected(new Set())
  }, [])

  const setPageSize = useCallback((s: number | ((prev: number) => number)) => {
    setPageSizeRaw(s)
    setSelected(new Set())
  }, [])

  return {
    page, setPage,
    pageSize, setPageSize,
    sortBy, sortOrder, handleSort,
    searchInput, setSearchInput, search,
    selected, toggleAll, toggleOne, clearSelection, syncSelection,
  }
}

// Opt-in companion for list pages: keeps `selected` in sync with the current
// items, so single-row deletes or filter changes drop stale IDs automatically.
// Usage inside a component body:
//   const state = useListState()
//   useListSelectionSync(state.syncSelection, items)
export function useListSelectionSync<T extends { metadata: { id: string } }>(
  syncSelection: (ids: string[]) => void,
  items: T[],
) {
  useEffect(() => {
    syncSelection(items.map((it) => it.metadata.id))
  }, [syncSelection, items])
}
