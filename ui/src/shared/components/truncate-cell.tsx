import { useRef, useState, useCallback, useLayoutEffect } from "react"
import { TableCell } from "@/shared/ui/table"
import {
  Tooltip, TooltipTrigger, TooltipContent,
} from "@/shared/ui/tooltip"

/**
 * A TableCell that shows a tooltip when the inner text is truncated.
 * Tooltip only appears if the content actually overflows.
 * Use `text` prop when children contain interactive elements (e.g. Link)
 * so the tooltip shows plain text instead of a clickable element.
 */
export function TruncateCell({
  children,
  text,
  className,
  maxWidth = "max-w-[200px]",
}: {
  children: React.ReactNode
  /** Plain text for the tooltip (defaults to children if not set) */
  text?: string
  className?: string
  maxWidth?: string
}) {
  const spanRef = useRef<HTMLSpanElement>(null)
  const [truncated, setTruncated] = useState(false)
  const [open, setOpen] = useState(false)

  // Measure after layout so scrollWidth/clientWidth reflect the final DOM;
  // update on window resize so column-width changes stay in sync.
  useLayoutEffect(() => {
    const el = spanRef.current
    if (!el) return
    const measure = () => setTruncated(el.scrollWidth > el.clientWidth)
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [children, text])

  const handleOpenChange = useCallback((next: boolean) => {
    setOpen(next && truncated)
  }, [truncated])

  // The max-width must live on the inner block span, not on the <td>:
  // auto table-layout ignores a max-width on <td>, so a capped block span
  // is the only thing honoured across layout modes.
  return (
    <TableCell>
      <Tooltip open={open} onOpenChange={handleOpenChange}>
        <TooltipTrigger asChild>
          <span ref={spanRef} className={`block truncate ${maxWidth} ${className ?? ""}`}>
            {children}
          </span>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs break-words whitespace-normal">
          {text ?? children}
        </TooltipContent>
      </Tooltip>
    </TableCell>
  )
}
