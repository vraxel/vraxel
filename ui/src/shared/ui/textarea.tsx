import * as React from "react"

import { cn } from "@/shared/lib/utils"

function Textarea({ className, ref, ...props }: React.ComponentProps<"textarea">) {
  // Same dev guard as <Input>: a <textarea> without name or id triggers the
  // browser autofill warning. RHF {...field} supplies name; orphan cases
  // are inline editors and detail-page read-only fields.
  const innerRef = React.useRef<HTMLTextAreaElement | null>(null)
  React.useEffect(() => {
    if (import.meta.env.PROD) return
    const el = innerRef.current
    if (!el) return
    if (!el.name && !el.id) {
      throw new Error(
        `<Textarea> mounted with neither name nor id. Add name="..." (preferred) ` +
          `or id="..." to silence the browser autofill warning. ` +
          `Element: ${el.outerHTML.slice(0, 200)}`,
      )
    }
  })

  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "min-h-16 w-full min-w-0 rounded-md border border-input bg-card px-3 py-2 text-base shadow-xs transition-[color,box-shadow,border-color] outline-none placeholder:text-faint focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/25 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 md:text-sm",
        className
      )}
      {...props}
      ref={(node) => {
        innerRef.current = node
        if (typeof ref === "function") ref(node)
        else if (ref) (ref as React.MutableRefObject<HTMLTextAreaElement | null>).current = node
      }}
    />
  )
}

export { Textarea }
