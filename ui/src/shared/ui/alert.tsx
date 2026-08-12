import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/shared/lib/utils"

const alertVariants = cva(
  "rounded-md border p-3 text-sm",
  {
    variants: {
      variant: {
        destructive: "border-destructive/50 bg-destructive/10",
        warning: "border-warning/40 bg-warning/10",
        info: "border-primary/30 bg-primary-subtle",
      },
    },
    defaultVariants: {
      variant: "destructive",
    },
  }
)

function Alert({
  className,
  variant,
  ...props
}: React.ComponentProps<"div"> & VariantProps<typeof alertVariants>) {
  return (
    <div
      data-slot="alert"
      role="alert"
      className={cn(alertVariants({ variant }), className)}
      {...props}
    />
  )
}

export { Alert, alertVariants }
