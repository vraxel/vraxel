import { useRef, useState, useCallback, useLayoutEffect } from "react"
import {
  Tooltip, TooltipTrigger, TooltipContent,
} from "@/shared/ui/tooltip"

/**
 * A text element that shows a tooltip when its inner text is truncated.
 * Tooltip only appears if the content actually overflows.
 *
 * Use this for non-table contexts (detail page info cards, stat cards, etc).
 * For table cells, use `TruncateCell` instead.
 *
 * Parent must have `min-w-0` for `truncate` to take effect inside flex/grid layouts.
 *
 * Set `lines` to enable multi-line clamping (e.g. `lines={3}` for `line-clamp-3`).
 * Without `lines`, single-line `truncate` is used.
 */
export function TruncateText({
  children,
  text,
  className,
  lines,
}: {
  children: React.ReactNode
  /** Plain text for the tooltip (defaults to children if not set) */
  text?: string
  className?: string
  /** Number of lines before clamping. Omit for single-line truncate. */
  lines?: number
}) {
  const spanRef = useRef<HTMLSpanElement>(null)
  const [truncated, setTruncated] = useState(false)
  const [open, setOpen] = useState(false)

  useLayoutEffect(() => {
    const el = spanRef.current
    if (!el) return
    const measure = () => {
      const overflows = lines
        ? el.scrollHeight > el.clientHeight
        : el.scrollWidth > el.clientWidth
      setTruncated(overflows)
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [children, text, lines])

  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next && truncated)
  }, [truncated])

  const clampClass = lines ? `line-clamp-${lines} break-all` : "truncate"

  return (
    <Tooltip open={open} onOpenChange={handleOpenChange}>
      <TooltipTrigger asChild>
        <span
          ref={spanRef}
          // pointer-events is inherited; Radix's SelectValue sets it to "none"
          // so trigger clicks pass through to the underlying button, which also
          // hides the inner span from hover events. Force auto here so the
          // tooltip can still open on hover; click events bubble regardless of
          // pointer-events, so the trigger button still opens on click.
          className={`block pointer-events-auto ${clampClass} ${className ?? ""}`}
        >
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-xs break-words whitespace-normal">
        {text ?? children}
      </TooltipContent>
    </Tooltip>
  )
}
