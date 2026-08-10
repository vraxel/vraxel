import { ArrowUpDown, ArrowUp, ArrowDown } from "lucide-react"

export function SortIcon({ field, sortBy, sortOrder }: {
  field: string
  sortBy: string
  sortOrder: "asc" | "desc"
}) {
  if (sortBy !== field) return <ArrowUpDown className="ml-1 inline h-3 w-3 opacity-40" />
  return sortOrder === "asc"
    ? <ArrowUp className="ml-1 inline h-3 w-3" />
    : <ArrowDown className="ml-1 inline h-3 w-3" />
}
