import type { ReactNode } from "react"
import { Inbox } from "lucide-react"
import { cn } from "@/shared/lib/utils"

interface EmptyStateProps {
  icon?: React.ComponentType<{ className?: string }>
  title: ReactNode
  description?: ReactNode
  /** Primary affordance -- usually the same create button as the page header. */
  action?: ReactNode
  className?: string
}

export function EmptyState({
  icon: Icon = Inbox,
  title,
  description,
  action,
  className,
}: EmptyStateProps) {
  return (
    <div className={cn("flex flex-col items-center gap-3 px-6 py-14 text-center", className)}>
      <div className="bg-muted text-muted-foreground flex size-11 items-center justify-center rounded-full">
        <Icon className="size-5" />
      </div>
      <div className="space-y-1">
        <p className="text-foreground text-sm font-medium">{title}</p>
        {description != null && (
          <p className="text-muted-foreground max-w-sm text-sm">{description}</p>
        )}
      </div>
      {action != null && <div className="mt-1">{action}</div>}
    </div>
  )
}
