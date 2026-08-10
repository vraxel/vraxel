// Pure pagination helpers, kept in a sibling .ts (not pagination.tsx)
// so the component file only exports components and react-refresh
// fast-refresh stays intact.

const SIBLING = 2
const SHOW_ALL_THRESHOLD = 7

export type PageItem = number | "ellipsis-l" | "ellipsis-r"

// Boundaries (1, total) are always shown. Window auto-extends near edges so the
// total visible page count stays roughly constant. No ellipsis when window is
// directly adjacent to a boundary.
export function buildPageItems(current: number, total: number): PageItem[] {
  if (total <= 0) return []
  if (total <= SHOW_ALL_THRESHOLD) {
    return Array.from({ length: total }, (_, i) => i + 1)
  }

  const target = SIBLING * 2 + 1
  let left = Math.max(2, current - SIBLING)
  let right = Math.min(total - 1, current + SIBLING)

  if (right - left + 1 < target) {
    if (current - SIBLING < 2) {
      right = Math.min(total - 1, left + target - 1)
    } else if (current + SIBLING > total - 1) {
      left = Math.max(2, right - target + 1)
    }
  }

  const items: PageItem[] = [1]
  if (left > 2) items.push("ellipsis-l")
  for (let i = left; i <= right; i++) items.push(i)
  if (right < total - 1) items.push("ellipsis-r")
  items.push(total)
  return items
}
