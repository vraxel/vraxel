import * as React from "react"

import { cn } from "@/shared/lib/utils"

// The bar has no colour prop on purpose. A filled bar is a MAGNITUDE, and
// repainting it was the only thing the prop was ever used for -- to say
// "over threshold" or "this reading is stale". Both are states, and a bar
// that changes colour states them without labelling them: three grey
// gauges under a header reading 在线 were reported as a rendering fault
// twice before the prop was removed. States belong on the text beside the
// bar, where they can be coloured AND named.
function Progress({
  value = 0,
  className,
  ...props
}: React.ComponentProps<"div"> & { value?: number }) {
  return (
    <div
      data-slot="progress"
      className={cn("h-2 w-full rounded-full bg-muted overflow-hidden", className)}
      {...props}
    >
      <div
        data-slot="progress-indicator"
        className="h-full rounded-full bg-primary transition-all"
        style={{ width: `${Math.min(Math.max(value, 0), 100)}%` }}
      />
    </div>
  )
}

export { Progress }
