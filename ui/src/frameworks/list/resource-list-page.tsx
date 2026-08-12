import type { ReactNode } from "react"
import { Link } from "react-router"
import { AlertCircle, Search } from "lucide-react"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table"
import { Input } from "@/shared/ui/input"
import { Button } from "@/shared/ui/button"
import { Checkbox } from "@/shared/ui/checkbox"
import { Skeleton } from "@/shared/ui/skeleton"
import { SortIcon } from "@/shared/components/sort-icon"
import { FilterTableHead, type FilterOption } from "@/shared/components/filter-table-head"
import { TruncateCell } from "@/shared/components/truncate-cell"
import { EmptyState } from "@/shared/components/empty-state"
import { Pagination } from "@/shared/components/pagination"
import { cn } from "@/shared/lib/utils"
import { useTranslation } from "@/i18n"
import type { ListQuery, ListRow } from "./use-list-query"

// Placeholder bars all stretched to w-full read as a grey wall, which is the
// one thing a skeleton must not do -- real text has ragged right edges.
// Indexed by (row + column) so the raggedness is deterministic, not random.
const SKELETON_WIDTHS = ["w-full", "w-3/4", "w-1/2", "w-5/6", "w-2/3"]

export interface ColumnDef<T> {
  /** Sort field name and react key. */
  key: string
  header: ReactNode
  sortable?: boolean
  /** When present the header shows a filter dropdown bound to `filterKey`. */
  filter?: FilterOption[]
  filterKey?: string
  cell: (row: T) => ReactNode
  /** Wrap the cell in TruncateCell (overflow tooltip). */
  truncate?: boolean
  className?: string
  headClassName?: string
}

export interface ResourceListPageProps<T extends ListRow> {
  query: ListQuery<T>
  columns: ColumnDef<T>[]
  titleKey: string
  subtitle?: ReactNode
  searchPlaceholderKey?: string
  /** Rendered in the header (page owns the permission check). */
  createButton?: ReactNode
  /** Rendered when there is a selection (page owns permission + wiring). */
  batchActions?: ReactNode
  /** Extra controls in the toolbar (rendered after the search box). */
  toolbarExtra?: ReactNode
  rowActions?: (row: T) => ReactNode
  rowHref?: (row: T) => string
  /** Show the multi-select checkbox column. */
  selectable?: boolean
  /** Extra classes per row (e.g. highlight a just-onboarded row). */
  rowClassName?: (row: T) => string | undefined
  emptyKey?: string
  /** Dialogs etc. rendered after the table. */
  children?: ReactNode
}

/**
 * Configuration-driven list page (frameworks/list). Owns the cross-page
 * boilerplate -- header, search, sortable/filterable header cells,
 * skeleton rows, empty AND error states (kept separate, plan.md 1.9),
 * the select-all column, and pagination -- so a migrated page is columns +
 * filters + row/batch actions, not 300+ lines of copied structure.
 *
 * The page owns the ListQuery (via useListQuery) so it can reach rows,
 * scope and selection for its own mutations; the framework only renders.
 */
export function ResourceListPage<T extends ListRow>({
  query,
  columns,
  titleKey,
  subtitle,
  searchPlaceholderKey,
  createButton,
  batchActions,
  toolbarExtra,
  rowActions,
  rowHref,
  selectable = true,
  rowClassName,
  emptyKey,
  children,
}: ResourceListPageProps<T>) {
  const { t } = useTranslation()
  const { rows, totalCount, isPending, error } = query

  const colCount = columns.length + (selectable ? 1 : 0) + (rowActions ? 1 : 0)
  const allSelected = rows.length > 0 && query.selected.size === rows.length
  const someSelected = query.selected.size > 0 && !allSelected

  return (
    <div className="p-6">
      <div className="mb-6 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold tracking-tight">{t(titleKey)}</h1>
          {subtitle != null && <p className="text-muted-foreground mt-1 text-sm">{subtitle}</p>}
        </div>
        {createButton}
      </div>

      <div className="mb-4 flex items-center gap-3">
        <div className="relative max-w-xs flex-1">
          <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
          <Input
            name="list-search"
            placeholder={searchPlaceholderKey ? t(searchPlaceholderKey) : t("common.search")}
            value={query.searchInput}
            onChange={(e) => query.setSearchInput(e.target.value)}
            className="pl-9"
          />
        </div>
        {toolbarExtra}
        {query.selected.size > 0 && batchActions}
      </div>

      {/* overflow-hidden so the muted table header clips to the rounded corners */}
      <div className="overflow-hidden rounded-xl border shadow-sm">
        <Table>
          <TableHeader>
            <TableRow>
              {selectable && (
                <TableHead className="w-10">
                  <Checkbox
                    checked={allSelected ? true : someSelected ? "indeterminate" : false}
                    onCheckedChange={() => query.toggleAll(rows.map((r) => r.metadata.id))}
                    aria-label="select all"
                  />
                </TableHead>
              )}
              {columns.map((col) => {
                // FilterTableHead renders its own "unset" item, so pull an
                // { value: "all" } entry out of the column's options and pass
                // its (often custom, e.g. "所有区域") label as allLabel --
                // avoids the double "全部" and preserves the custom wording.
                const allOpt = col.filter?.find((o) => o.value === "all")
                const opts = col.filter?.filter((o) => o.value !== "all") ?? []
                return col.filter ? (
                  <FilterTableHead
                    key={col.key}
                    field={col.key}
                    sortBy={query.sortBy}
                    sortOrder={query.sortOrder}
                    onSort={query.handleSort}
                    filterValue={query.filters[col.filterKey ?? col.key] ?? "all"}
                    onFilterChange={(v) => query.setFilter(col.filterKey ?? col.key, v)}
                    options={opts}
                    allLabel={allOpt?.label}
                    hideSort={!col.sortable}
                    className={col.headClassName}
                  >
                    {col.header}
                  </FilterTableHead>
                ) : (
                  <TableHead key={col.key} className={col.headClassName}>
                    {col.sortable ? (
                      <button
                        type="button"
                        className="group/sort hover:text-foreground focus-visible:ring-ring/40 inline-flex cursor-pointer items-center rounded-sm transition-colors outline-none select-none focus-visible:ring-2"
                        onClick={() => query.handleSort(col.key)}
                      >
                        {col.header}
                        <SortIcon
                          field={col.key}
                          sortBy={query.sortBy}
                          sortOrder={query.sortOrder}
                        />
                      </button>
                    ) : (
                      col.header
                    )}
                  </TableHead>
                )
              })}
              {rowActions && <TableHead className="w-0" />}
            </TableRow>
          </TableHeader>
          <TableBody>
            {isPending ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i} className="hover:bg-transparent">
                  {Array.from({ length: colCount }).map((__, j) => (
                    <TableCell key={j}>
                      <Skeleton
                        className={cn("h-4", SKELETON_WIDTHS[(i + j) % SKELETON_WIDTHS.length])}
                      />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : error && rows.length === 0 ? (
              // Only surface the error row when there is nothing to show:
              // a failed background refetch keeps the cached rows (TQ v5
              // sets error but retains data) and the global toast already
              // reported it -- wiping a loaded table for a transient 502
              // was review finding C5.
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={colCount} className="p-0 whitespace-normal">
                  <EmptyState
                    icon={AlertCircle}
                    title={t("common.loadError")}
                    action={
                      <Button variant="outline" size="sm" onClick={() => query.refetch()}>
                        {t("common.retry")}
                      </Button>
                    }
                  />
                </TableCell>
              </TableRow>
            ) : rows.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={colCount} className="p-0 whitespace-normal">
                  {/* The header's create button doubles as the empty-state CTA:
                      when the table is empty it is the only thing to do here. */}
                  <EmptyState
                    title={emptyKey ? t(emptyKey) : t("common.noData")}
                    action={createButton}
                  />
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => {
                const id = row.metadata.id
                return (
                  <TableRow
                    key={id}
                    // Without this the only sign a row is selected is its own
                    // checkbox; TableRow already styles data-state=selected.
                    data-state={query.selected.has(id) ? "selected" : undefined}
                    className={
                      [rowHref ? "cursor-pointer" : "", rowClassName?.(row) ?? ""]
                        .filter(Boolean)
                        .join(" ") || undefined
                    }
                  >
                    {selectable && (
                      <TableCell className="w-10" onClick={(e) => e.stopPropagation()}>
                        <Checkbox
                          checked={query.selected.has(id)}
                          onCheckedChange={() => query.toggleOne(id)}
                          aria-label="select row"
                        />
                      </TableCell>
                    )}
                    {columns.map((col) => {
                      const content = col.cell(row)
                      const wrapped = rowHref ? (
                        <Link to={rowHref(row)} className="block">
                          {content}
                        </Link>
                      ) : (
                        content
                      )
                      return col.truncate ? (
                        <TruncateCell key={col.key} className={col.className}>
                          {wrapped}
                        </TruncateCell>
                      ) : (
                        <TableCell key={col.key} className={col.className}>
                          {wrapped}
                        </TableCell>
                      )
                    })}
                    {rowActions && (
                      <TableCell className="w-0" onClick={(e) => e.stopPropagation()}>
                        {rowActions(row)}
                      </TableCell>
                    )}
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>

      <Pagination
        page={query.page}
        pageSize={query.pageSize}
        totalCount={totalCount}
        onPageChange={query.setPage}
        onPageSizeChange={query.setPageSize}
      />

      {children}
    </div>
  )
}
