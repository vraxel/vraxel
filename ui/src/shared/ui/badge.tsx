import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { Slot } from "radix-ui"

import { cn } from "@/shared/lib/utils"

const badgeVariants = cva(
  "inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-md border border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap transition-[color,box-shadow] focus-visible:ring-[3px] focus-visible:ring-ring/40 aria-invalid:border-destructive aria-invalid:ring-destructive/20 [&>svg]:pointer-events-none [&>svg]:size-3",
  {
    variants: {
      // Status badges read as tinted chips (soft background + saturated text),
      // not as solid blocks -- a table full of solid pills fights the data.
      variant: {
        default:
          "border-primary/15 bg-primary-subtle text-primary [a&]:hover:bg-primary-subtle/70",
        secondary:
          "bg-secondary text-secondary-foreground [a&]:hover:bg-secondary/70",
        destructive:
          "border-destructive/15 bg-destructive/10 text-destructive focus-visible:ring-destructive/30 [a&]:hover:bg-destructive/15",
        warning:
          "border-warning/20 bg-warning/12 text-warning [a&]:hover:bg-warning/20",
        success:
          "border-success/20 bg-success/12 text-success [a&]:hover:bg-success/20",
        solid: "bg-primary text-primary-foreground [a&]:hover:bg-primary/90",
        outline:
          "border-border text-foreground [a&]:hover:bg-accent [a&]:hover:text-accent-foreground",
        ghost: "[a&]:hover:bg-accent [a&]:hover:text-accent-foreground",
        link: "text-primary underline-offset-4 [a&]:hover:underline",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
)

function Badge({
  className,
  variant = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"span"> &
  VariantProps<typeof badgeVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : "span"

  return (
    <Comp
      data-slot="badge"
      data-variant={variant}
      className={cn(badgeVariants({ variant }), className)}
      {...props}
    />
  )
}

export { Badge, badgeVariants }
