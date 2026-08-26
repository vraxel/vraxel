import type { ReactNode } from "react"
import { Search } from "lucide-react"
import { Input } from "@/shared/ui/input"
import { TableHead } from "@/shared/ui/table"
import { SortIcon } from "@/shared/components/sort-icon"
import type { TableView } from "@/modules/compute/components/host-table-view"

// The two controls the detail page's in-memory tables share.
//
// Split from the hook and comparators next door because a .tsx file that
// exports both components and plain functions breaks Fast Refresh -- the
// rule says so, and the fix is a second file rather than an exemption.

/** A sortable column header, visually identical to the list page's. */
export function SortHead<T>({
  view,
  field,
  children,
  className,
}: {
  view: TableView<T>
  field: string
  children: ReactNode
  className?: string
}) {
  return (
    <TableHead className={className}>
      <button
        type="button"
        className="group/sort hover:text-foreground focus-visible:ring-ring/40 inline-flex cursor-pointer items-center rounded-sm transition-colors outline-none select-none focus-visible:ring-2"
        onClick={() => view.toggleSort(field)}
      >
        {children}
        <SortIcon field={field} sortBy={view.sortBy} sortOrder={view.sortDir} />
      </button>
    </TableHead>
  )
}

/** The search box these tables share.
 *
 *  name is required, not decorative: the shared Input warns without one,
 *  because a nameless field is what the browser offers to autofill with
 *  somebody's address. */
export function TableSearch<T>({
  view,
  placeholder,
  name,
}: {
  view: TableView<T>
  placeholder: string
  name: string
}) {
  return (
    <div className="relative w-64">
      <Search className="text-muted-foreground absolute top-1/2 left-2.5 h-3.5 w-3.5 -translate-y-1/2" />
      <Input
        name={name}
        value={view.query}
        onChange={(e) => view.setQuery(e.target.value)}
        placeholder={placeholder}
        className="h-8 pl-8 text-sm"
        autoComplete="off"
      />
    </div>
  )
}
