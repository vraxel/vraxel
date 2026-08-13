import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"
import { useTranslation } from "@/i18n"

export interface StatusFilterOption {
  value: string
  label: string
}

interface Props {
  /** Current value, "all" when unset. */
  value: string
  onChange: (value: string) => void
  options: StatusFilterOption[]
  /** Label for the "no filter" entry; defaults to common.all. */
  allLabel?: string
  placeholder?: string
  className?: string
}

/**
 * The toolbar dropdown a status filter moves to once its column has been
 * folded into the name cell.
 *
 * It cannot stay on the name column's header: FilterTableHead renders the
 * filter next to that header's text, so a status filter there would read
 * as filtering by name. The toolbar is where a filter with no column of
 * its own belongs.
 */
export function StatusFilter({
  value,
  onChange,
  options,
  allLabel,
  placeholder,
  className = "h-9 w-40",
}: Props) {
  const { t } = useTranslation()
  return (
    <Select value={value || "all"} onValueChange={onChange}>
      <SelectTrigger className={className}>
        <SelectValue placeholder={placeholder ?? t("common.status")} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{allLabel ?? t("common.all")}</SelectItem>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
