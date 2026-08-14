import { Filter } from "lucide-react"
import { TableHead } from "@/shared/ui/table"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"
import { SortIcon } from "@/shared/components/sort-icon"
import { useTranslation } from "@/i18n"

export interface FilterOption {
  value: string
  label: string
}

export function FilterTableHead<T extends string = string>({
  field,
  sortBy,
  sortOrder,
  onSort,
  selected,
  onChange,
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
  selected: Set<string>
  onChange: (next: Set<string>) => void
  options: FilterOption[]
  allLabel?: string
  className?: string
  children: React.ReactNode
  hideSort?: boolean
}) {
  const { t } = useTranslation()
  const isFiltered = selected.size > 0 && selected.size < options.length

  const selectAll = () => onChange(new Set(options.map((o) => o.value)))
  const toggle = (value: string) => {
    const next = new Set(selected)
    if (next.has(value)) next.delete(value)
    else next.add(value)
    if (next.size === 0) return selectAll()
    onChange(next)
  }

  return (
    <TableHead className={className}>
      <span className="inline-flex items-center gap-1">
        <button
          type="button"
          className="group/sort hover:text-foreground focus-visible:ring-ring/40 inline-flex cursor-pointer items-center rounded-sm transition-colors outline-none select-none focus-visible:ring-2"
          onClick={() => !hideSort && onSort(field)}
        >
          {children}
          {!hideSort && <SortIcon field={field} sortBy={sortBy ?? ""} sortOrder={sortOrder} />}
        </button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="focus-visible:ring-ring/40 inline-flex cursor-pointer items-center rounded-sm outline-none focus-visible:ring-2"
              aria-label="filter"
            >
              <Filter
                className={
                  isFiltered
                    ? "fill-primary text-primary h-3 w-3"
                    : "text-muted-foreground h-3 w-3"
                }
              />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={selectAll}>
              {allLabel ?? t("common.all")}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            {options.map((opt) => (
              <DropdownMenuCheckboxItem
                key={opt.value}
                checked={selected.has(opt.value)}
                onSelect={(e) => e.preventDefault()}
                onCheckedChange={() => toggle(opt.value)}
              >
                {opt.label}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
    </TableHead>
  )
}
