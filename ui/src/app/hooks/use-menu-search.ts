import { useCallback, useMemo, useState } from "react"

export interface MenuSearchItem {
  /** Unique key = nav link `to`. */
  key: string
  /** Lowercased searchable strings (zh label, en label, pinyin initials). */
  tokens: string[]
}

/**
 * Browser-Ctrl+F-style quick search over the sidebar menu: nothing is hidden,
 * matching items are marked, and the caller scrolls `currentKey` into view.
 * `items` is expected in visual (top-to-bottom) order.
 */
export function useMenuSearch(items: MenuSearchItem[]) {
  const [query, setQuery] = useState("")
  const [index, setIndex] = useState(0)

  const matches = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return [] as string[]
    return items.filter((it) => it.tokens.some((tok) => tok.includes(q))).map((it) => it.key)
  }, [items, query])

  // Every new query restarts navigation at the first match
  // (adjust-during-render; an effect would paint the stale index first).
  const [prevQuery, setPrevQuery] = useState(query)
  if (prevQuery !== query) {
    setPrevQuery(query)
    setIndex(0)
  }

  const total = matches.length
  const currentIndex = total === 0 ? -1 : Math.min(index, total - 1)
  const currentKey = currentIndex >= 0 ? matches[currentIndex] : null
  const matchSet = useMemo(() => new Set(matches), [matches])

  const next = useCallback(() => {
    setIndex((i) => (total === 0 ? 0 : (Math.min(i, total - 1) + 1) % total))
  }, [total])
  const prev = useCallback(() => {
    setIndex((i) => (total === 0 ? 0 : (Math.min(i, total - 1) - 1 + total) % total))
  }, [total])
  const clear = useCallback(() => {
    setQuery("")
    setIndex(0)
  }, [])

  return { query, setQuery, matchSet, currentKey, currentIndex, total, next, prev, clear }
}
