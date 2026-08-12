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
 */
export function NameCell({ to, displayName, name, maxWidth = "max-w-[240px]" }: NameCellProps) {
  const primary = displayName?.trim() || name
  const secondary = primary === name ? null : name
  return (
    <div className={`${maxWidth} min-w-0`}>
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
}
