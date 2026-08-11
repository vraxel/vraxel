import { Card, CardContent } from "@/shared/ui/card"
import { Skeleton } from "@/shared/ui/skeleton"

interface OverviewCardProps {
  label: string
  icon: React.ComponentType<{ className?: string }>
  value: number | string | null
  loading?: boolean
  onClick?: () => void
}

export function OverviewCard({ label, icon: Icon, value, loading, onClick }: OverviewCardProps) {
  return (
    <Card
      className={onClick ? "hover:bg-muted/50 cursor-pointer transition-colors" : undefined}
      onClick={onClick}
    >
      <CardContent className="flex items-center gap-4 p-4">
        <div className="bg-primary/10 flex h-10 w-10 items-center justify-center rounded-lg">
          <Icon className="text-primary h-5 w-5" />
        </div>
        <div>
          {loading ? (
            <Skeleton className="mb-1 h-7 w-12" />
          ) : (
            <p className="text-2xl font-bold">{value ?? "-"}</p>
          )}
          <p className="text-muted-foreground text-sm">{label}</p>
        </div>
      </CardContent>
    </Card>
  )
}
