import type { ReactNode } from "react"
import { Link } from "react-router"
import { TruncateText } from "@/shared/components/truncate-text"

export interface NameCellProps {
  /** Detail route; the primary line becomes the link. Omit where the row
   *  has no detail page of its own (e.g. a role binding), and the cell
   *  renders as plain text. */
  to?: string
  /** Human-facing label (display name). Falls back to `name` when blank. */
  displayName?: string | null
  /** Canonical identifier (name / username), shown beneath. */
  name: string
  /** Width cap for the truncation of both lines. */
  maxWidth?: string
  /** Status badge shown beside the name, vertically centred against both
   *  lines. See the component doc for why status belongs here. */
  trailing?: ReactNode
}

/**
 * The standard identity cell for list rows: the display name as the link
 * to the detail page, with the canonical name beneath it in small muted
 * text.
 *
 * Merging the two into one column keeps the pair adjacent (they are read
 * together) and buys back a column for every resource. Both lines
 * truncate with an overflow tooltip.
 *
 * Degenerate cases collapse to a single line rather than showing a dash
 * or repeating the same string: a resource with no display name renders
 * just its name, still linked.
 *
 * `trailing` is where a row's status badge goes. Status is what an
 * operator scans a list for, so it belongs beside the name their eye is
 * already on rather than in a column of its own three headers to the
 * right -- and folding it in buys back a column. It is centred against
 * the pair of lines, because the cell is one line or two depending on
 * whether the row has a display name and a top-aligned badge would jump
 * between rows of different heights. The name's width cap tightens when
 * a badge is present so the two stay adjacent on long names.
 */
export function NameCell({ to, displayName, name, maxWidth, trailing }: NameCellProps) {
  const primary = displayName?.trim() || name
  const secondary = primary === name ? null : name
  const width = maxWidth ?? (trailing ? "max-w-[220px]" : "max-w-[304px]")
  const body = (
    <div className={`${width} min-w-0`}>
      <TruncateText text={primary}>
        {to ? (
          <Link
            to={to}
            className="text-foreground hover:text-primary font-medium transition-colors"
          >
            {primary}
          </Link>
        ) : (
          <span className="text-foreground font-medium">{primary}</span>
        )}
      </TruncateText>
      {secondary && (
        <TruncateText className="text-muted-foreground text-xs" text={secondary}>
          {secondary}
        </TruncateText>
      )}
    </div>
  )
  if (!trailing) return body
  return (
    <div className="flex min-w-0 items-center gap-2">
      {body}
      <span className="shrink-0">{trailing}</span>
    </div>
  )
}
