import { format, isValid, parseISO } from "date-fns"

/**
 * Timestamps render in a fixed `yyyy-MM-dd HH:mm` shape rather than through
 * `toLocaleString()`. Two reasons:
 *
 *  - `toLocaleString()` follows the *browser* locale, not the app's. Switching
 *    the console to English left every date rendering as `2026/8/12`.
 *  - An ops console is read by comparison: sortable, zero-padded, unambiguous
 *    beats `8/12/2026, 2:11:04 AM`.
 *
 * Seconds are dropped by default (they are noise on a created/updated column)
 * and available via `formatDateTimeSeconds` where event ordering matters.
 */
function parse(value: string | number | Date | null | undefined): Date | null {
  if (value == null || value === "") return null
  const d = typeof value === "string" ? parseISO(value) : new Date(value)
  return isValid(d) ? d : null
}

export function formatDateTime(value: string | number | Date | null | undefined): string {
  const d = parse(value)
  return d ? format(d, "yyyy-MM-dd HH:mm") : "-"
}

export function formatDateTimeSeconds(value: string | number | Date | null | undefined): string {
  const d = parse(value)
  return d ? format(d, "yyyy-MM-dd HH:mm:ss") : "-"
}
