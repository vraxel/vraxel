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
      <ArrowUpDown className="text-muted-foreground/40 ml-1 inline h-3 w-3" />
    )
  }
  return sortOrder === "asc" ? (
    <ArrowUp className="text-primary ml-1 inline h-3 w-3" />
  ) : (
    <ArrowDown className="text-primary ml-1 inline h-3 w-3" />
  )
}
