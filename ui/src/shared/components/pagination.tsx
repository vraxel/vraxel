import { useState } from "react"
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/shared/ui/select"
import { useTranslation } from "@/i18n"
import { PAGE_SIZE_OPTIONS } from "@/frameworks/list/use-list-state"
import { buildPageItems } from "@/shared/components/pagination-utils"

interface PaginationProps {
  totalCount: number
  page: number
  pageSize: number
  onPageChange: (page: number) => void
  onPageSizeChange: (size: number) => void
}

export function Pagination({ totalCount, page, pageSize, onPageChange, onPageSizeChange }: PaginationProps) {
  const { t } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize))
  const [jumpInput, setJumpInput] = useState("")

  if (totalCount === 0) return null

  const pageItems = buildPageItems(page, totalPages)
  const pageUnit = t("common.pageUnit")

  const commitJump = () => {
    if (!jumpInput) return
    const target = Math.min(Math.max(1, Number(jumpInput)), totalPages)
    if (target !== page) onPageChange(target)
    setJumpInput("")
  }

  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-4">
      <div className="flex items-center gap-4">
        <p className="text-muted-foreground text-sm">{t("common.total", { count: totalCount })}</p>
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground text-sm">{t("common.pageSize")}</span>
          <Select value={String(pageSize)} onValueChange={(v) => onPageSizeChange(Number(v))}>
            <SelectTrigger className="h-8 w-[70px]"><SelectValue /></SelectTrigger>
            <SelectContent>{PAGE_SIZE_OPTIONS.map((s) => <SelectItem key={s} value={String(s)}>{s}</SelectItem>)}</SelectContent>
          </Select>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-1">
        <Button variant="outline" size="icon-sm" disabled={page <= 1} onClick={() => onPageChange(1)} aria-label={t("common.firstPage")}>
          <ChevronsLeft />
        </Button>
        <Button variant="outline" size="icon-sm" disabled={page <= 1} onClick={() => onPageChange(page - 1)} aria-label={t("common.previous")}>
          <ChevronLeft />
        </Button>
        {pageItems.map((item) => {
          if (item === "ellipsis-l" || item === "ellipsis-r") {
            return (
              <span key={item} aria-hidden="true" className="text-muted-foreground flex h-8 w-8 select-none items-center justify-center text-sm">...</span>
            )
          }
          const isCurrent = item === page
          return (
            <Button
              key={item}
              variant={isCurrent ? "default" : "outline"}
              size="icon-sm"
              aria-current={isCurrent ? "page" : undefined}
              aria-label={t("common.gotoPage", { page: item })}
              onClick={() => { if (!isCurrent) onPageChange(item) }}
            >
              {item}
            </Button>
          )
        })}
        <Button variant="outline" size="icon-sm" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)} aria-label={t("common.next")}>
          <ChevronRight />
        </Button>
        <Button variant="outline" size="icon-sm" disabled={page >= totalPages} onClick={() => onPageChange(totalPages)} aria-label={t("common.lastPage")}>
          <ChevronsRight />
        </Button>
        {totalPages > 1 && (
          <div className="ml-2 flex items-center gap-2">
            <span className="text-muted-foreground text-sm">{t("common.jumpTo")}</span>
            <Input
              name="jump-page"
              className="h-8 w-12 px-2 text-center"
              inputMode="numeric"
              pattern="[0-9]*"
              aria-label={t("common.jumpToPageInput")}
              value={jumpInput}
              onChange={(e) => setJumpInput(e.target.value.replace(/[^0-9]/g, ""))}
              onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); commitJump() } }}
              onBlur={commitJump}
            />
            {pageUnit && <span className="text-muted-foreground text-sm">{pageUnit}</span>}
          </div>
        )}
      </div>
    </div>
  )
}
