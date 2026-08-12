import { ArrowUpDown, ArrowUp, ArrowDown } from "lucide-react"

/**
 * The neutral (unsorted) arrow stays invisible until the header row is
 * hovered or the trigger is focused: a table with eight sortable columns
 * otherwise carries eight permanent grey arrows competing with the data.
 * The active direction is always visible -- it is state, not an affordance.
 */
export function SortIcon({
  field,
  sortBy,
  sortOrder,
}: {
  field: string
  sortBy: string
  sortOrder: "asc" | "desc"
}) {
  if (sortBy !== field) {
    return (
      <ArrowUpDown className="ml-1 inline h-3 w-3 opacity-0 transition-opacity group-hover/thead:opacity-40 group-focus-visible/sort:opacity-60" />
    )
  }
  return sortOrder === "asc" ? (
    <ArrowUp className="text-primary ml-1 inline h-3 w-3" />
  ) : (
    <ArrowDown className="text-primary ml-1 inline h-3 w-3" />
  )
}
