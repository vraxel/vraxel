import * as React from "react"

import { cn } from "@/shared/lib/utils"

function Progress({
  value = 0,
  indicatorClassName,
  className,
  ...props
}: React.ComponentProps<"div"> & {
  value?: number
  indicatorClassName?: string
}) {
  return (
    <div
      data-slot="progress"
      className={cn("h-2 w-full rounded-full bg-muted overflow-hidden", className)}
      {...props}
    >
      <div
        data-slot="progress-indicator"
        className={cn("h-full rounded-full bg-primary transition-all", indicatorClassName)}
        style={{ width: `${Math.min(Math.max(value, 0), 100)}%` }}
      />
    </div>
  )
}

export { Progress }
