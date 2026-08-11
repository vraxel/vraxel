import { Filter } from "lucide-react"
import { TableHead } from "@/shared/ui/table"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"
import { SortIcon } from "@/shared/components/sort-icon"
import { useTranslation } from "@/i18n"

export interface FilterOption {
  value: string
  label: string
}

/**
 * FilterTableHead is a generic table-header cell with both sort
 * (click the label) and filter (click the funnel icon) controls.
 * Generic parameter T is the union of allowed sort field names;
 * pass an empty string for the field when sort isn't meaningful
 * for this column and use `hideSort` to drop the arrow.
 */
export function FilterTableHead<T extends string = string>({
  field,
  sortBy,
  sortOrder,
  onSort,
  filterValue,
  onFilterChange,
  options,
  allLabel,
  className,
  children,
  hideSort,
}: {
  field: T
  sortBy: T | null | string
  sortOrder: "asc" | "desc"
  onSort: (field: T) => void
  filterValue: string
  onFilterChange: (value: string) => void
  options: FilterOption[]
  /** Label for the built-in "unset" item; defaults to common.all. */
  allLabel?: string
  className?: string
  children: React.ReactNode
  hideSort?: boolean
}) {
  const { t } = useTranslation()
  const isFiltered = filterValue !== "all"
  return (
    <TableHead className={className}>
      <span className="inline-flex items-center gap-1">
        <button
          type="button"
          className="inline-flex cursor-pointer items-center select-none"
          onClick={() => !hideSort && onSort(field)}
        >
          {children}
          {!hideSort && <SortIcon field={field} sortBy={sortBy ?? ""} sortOrder={sortOrder} />}
        </button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button type="button" className="inline-flex items-center" aria-label="filter">
              <Filter
                className={`h-3 w-3 ${isFiltered ? "fill-primary text-primary" : "opacity-40"}`}
              />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={() => onFilterChange("all")}>
              {allLabel ?? t("common.all")}
            </DropdownMenuItem>
            {options.map((opt) => (
              <DropdownMenuItem key={opt.value} onClick={() => onFilterChange(opt.value)}>
                {opt.label}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
    </TableHead>
  )
}
