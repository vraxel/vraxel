import { Filter } from "lucide-react"
import { Button } from "@/shared/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"
import { useTranslation } from "@/i18n"

export interface StatusFilterOption {
  value: string
  label: string
}

interface Props {
  selected: Set<string>
  onChange: (next: Set<string>) => void
  options: StatusFilterOption[]
  allLabel?: string
  className?: string
}

export function StatusFilter({ selected, onChange, options, allLabel, className }: Props) {
  const { t } = useTranslation()
  const allValues = options.map((o) => o.value)
  const effective = selected.size === 0 ? new Set(allValues) : selected
  const isFiltered = effective.size < options.length

  const selectAll = () => onChange(new Set())
  const toggle = (value: string) => {
    const next = new Set(effective)
    if (next.has(value)) next.delete(value)
    else next.add(value)
    if (next.size === 0 || next.size === options.length) return selectAll()
    onChange(next)
  }

  const label = isFiltered
    ? options
        .filter((o) => effective.has(o.value))
        .map((o) => o.label)
        .join(", ")
    : (allLabel ?? t("common.all"))

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" className={className}>
          <Filter
            className={
              isFiltered ? "fill-primary text-primary h-3 w-3" : "text-muted-foreground h-3 w-3"
            }
          />
          <span className="max-w-32 truncate">{label}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuItem onClick={selectAll}>{allLabel ?? t("common.all")}</DropdownMenuItem>
        <DropdownMenuSeparator />
        {options.map((opt) => (
          <DropdownMenuCheckboxItem
            key={opt.value}
            checked={effective.has(opt.value)}
            onSelect={(e) => e.preventDefault()}
            onCheckedChange={() => toggle(opt.value)}
          >
            {opt.label}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
