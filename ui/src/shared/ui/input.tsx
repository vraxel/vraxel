import * as React from "react"

import { cn } from "@/shared/lib/utils"

function Input({ className, type, ref, ...props }: React.ComponentProps<"input">) {
  // Dev guard: every <input> in the DOM should have a name or an id so the
  // browser can autofill it and stop logging "A form field element should
  // have an id or name attribute". RHF Controller's {...field} spread
  // already supplies name; orphan cases are search boxes, batch editors,
  // and disabled read-only inputs that someone forgot to label.
  //
  // Reports via console.error (not throw) so a missing name doesn't
  // white-screen the page mid-render. `pnpm lint:input` (scripts/
  // lint-input.mjs) is the build-time enforcement layer that keeps the
  // codebase clean; this runtime guard is the second net for refactors
  // that route through the JSX in unusual ways (custom wrappers, ports
  // that forget to forward `name`) -- it surfaces the violation so the
  // developer notices without taking the whole tab down.
  const innerRef = React.useRef<HTMLInputElement | null>(null)
  React.useEffect(() => {
    if (import.meta.env.PROD) return
    const el = innerRef.current
    if (!el) return
    if (!el.name && !el.id) {
      console.error(
        `<Input> mounted with neither name nor id. Add name="..." (preferred) ` +
          `or id="..." to silence the browser autofill warning. ` +
          `Element: ${el.outerHTML.slice(0, 200)}`,
      )
    }
  })

  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "h-9 w-full min-w-0 border border-input bg-transparent px-3 py-1 text-base transition-colors outline-none selection:bg-primary selection:text-primary-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/30",
        "focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
        "aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
        className
      )}
      {...props}
      ref={(node) => {
        innerRef.current = node
        if (typeof ref === "function") ref(node)
        else if (ref) (ref as React.MutableRefObject<HTMLInputElement | null>).current = node
      }}
    />
  )
}

export { Input }
