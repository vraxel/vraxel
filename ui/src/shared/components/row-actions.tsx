import { TableCell, TableHead } from "@/shared/ui/table"
import { cn } from "@/shared/lib/utils"

// Width sizing under `table-auto`:
//   `<Table>` uses `table-layout: auto`, so cell `w-N` is a width hint:
//   the column settles at `w-N` when content fits, otherwise the column
//   auto-expands to its natural width. Sizes are calibrated to 32px
//   icon buttons (`h-8 w-8`) with `gap-1` (4px) between and `p-2` (8px)
//   cell padding on each side, picked so 1-4 buttons fit without
//   triggering auto-expansion.
//
//   - sm: w-20  (80px)  ~ 1 icon button       (1*32 + 0 + 2*8 = 48,  padding-rich)
//   - md: w-40 (160px)  ~ 2-4 icon buttons    (4*32 + 3*4 + 2*8 = 156)
//
// Adding a 5th action: do NOT widen further -- it starts to steal space
// from data columns AND auto-expands the column. Refactor into an
// overflow `<DropdownMenu>` (e.g. show first 3 inline + "..." for
// the rest).

type Size = "sm" | "md"

const sizeClass: Record<Size, string> = {
  sm: "w-20",
  md: "w-40",
}

// Sticky-right so the actions column stays visible while the table scrolls
// horizontally. Sticky cells need a fully opaque background in every row
// state, including hover, to mask cells passing beneath them -- TableRow's
// own hover is `bg-primary-subtle/60`, and a translucent sticky cell would
// let the underlying scrolled column bleed through, so the sticky cell uses
// the same tint at full opacity. The left-edge shadow hints that content is
// scrolled underneath.
const stickyHead = "sticky right-0 z-20 bg-card shadow-[-4px_0_6px_-4px_rgb(0_0_0/0.08)]"
// Ghost-button hover overrides: the default ghost variant uses `hover:bg-accent`,
// but in this theme `accent` and `muted` are the same color, so on hovered rows
// (cell becomes `bg-muted`) the button hover is invisible. Inject a stronger
// hover bg here so every action cell renders a visible hover affordance:
//   - destructive ghost (.text-destructive) -> pink
//   - other ghost -> neutral
//
// Exported for pages that need a custom-width sticky cell and can't use
// `RowActionsCell` directly (e.g. `dev/issues/list.tsx` needs `w-[280px]`).
export const rowActionStickyCell =
  "sticky right-0 z-10 bg-card group-hover/row:bg-primary-subtle group-data-[state=selected]/row:bg-primary-subtle shadow-[-4px_0_6px_-4px_rgb(0_0_0/0.08)] " +
  "[&_[data-variant=ghost]:hover]:bg-foreground/10 [&_[data-variant=ghost].text-destructive:hover]:bg-destructive/10"

interface Props {
  children: React.ReactNode
  className?: string
  size?: Size
}

export function RowActionsHead({ children, className, size = "md" }: Props) {
  return <TableHead className={cn(sizeClass[size], stickyHead, className)}>{children}</TableHead>
}

export function RowActionsCell({ children, className, size = "md" }: Props) {
  return (
    <TableCell className={cn(sizeClass[size], rowActionStickyCell, className)}>
      <div className="flex items-center gap-1">{children}</div>
    </TableCell>
  )
}
