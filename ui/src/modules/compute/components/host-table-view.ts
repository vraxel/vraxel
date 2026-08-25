import { useMemo, useState } from "react"

// Client-side search and sort for the detail page's in-memory tables.
//
// Deliberately not ResourceListPage, which is the right tool for a paged
// server resource and the wrong one here: these lists arrive whole inside
// one object (a real machine reports ~30 workloads and ~30 accounts), so
// there is no page to fetch, no sort_by to send and no URL state worth
// owning. Filtering thirty rows in the browser is immediate; a round trip
// per keystroke would not be. The sort affordance is the list page's, so
// the two read the same even though nothing is shared underneath.

export type SortDir = "asc" | "desc"

export interface TableView<T> {
  rows: T[]
  query: string
  setQuery: (v: string) => void
  sortBy: string
  sortDir: SortDir
  toggleSort: (field: string) => void
  /** Rows before filtering, for a "showing N of M" line. */
  total: number
}

/**
 * Searches with a lowercased substring match over whatever `haystack`
 * joins together.
 *
 * Substring rather than fuzzy: an operator searching a process table
 * types "nginx" or ":443" and means it. Fuzzy matching would put
 * "containerd" in the results for "cnd" and, worse, would rank an exact
 * match below something else -- on a list where every row is already
 * visible, that is a way to hide the row somebody was looking at.
 */
export function useTableView<T>(
  all: T[],
  haystack: (row: T) => string,
  comparators: Record<string, (a: T, b: T) => number>,
  initial: { by: string; dir: SortDir },
): TableView<T> {
  const [query, setQuery] = useState("")
  const [sortBy, setSortBy] = useState(initial.by)
  const [sortDir, setSortDir] = useState<SortDir>(initial.dir)

  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const filtered = needle ? all.filter((r) => haystack(r).toLowerCase().includes(needle)) : all
    const cmp = comparators[sortBy]
    if (!cmp) return filtered
    // Copy before sorting: the source array belongs to the query cache,
    // and sorting it in place would mutate what react-query handed us.
    const sorted = [...filtered].sort(cmp)
    return sortDir === "asc" ? sorted : sorted.reverse()
    // haystack and comparators are defined inline by the callers and so
    // are new on every render; depending on them would rebuild this on
    // every keystroke of an unrelated input.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [all, query, sortBy, sortDir])

  return {
    rows,
    query,
    setQuery,
    sortBy,
    sortDir,
    // Re-picking the active field flips direction, which is what every
    // table in the product does; picking a new one starts ascending.
    toggleSort: (field: string) => {
      if (field === sortBy) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"))
        return
      }
      setSortBy(field)
      setSortDir("asc")
    },
    total: all.length,
  }
}

/** Sorts undefined and empty last whichever direction is active. */
export function byText<T>(get: (row: T) => string | undefined) {
  return (a: T, b: T) => (get(a) ?? "").localeCompare(get(b) ?? "")
}

export function byNumber<T>(get: (row: T) => number | undefined) {
  return (a: T, b: T) => (get(a) ?? 0) - (get(b) ?? 0)
}
