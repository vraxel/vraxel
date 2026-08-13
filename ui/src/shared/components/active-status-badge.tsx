import { Badge } from "@/shared/ui/badge"
import { useTranslation } from "@/i18n"

interface Props {
  status?: string
  className?: string
}

/**
 * The active / inactive chip shared by every resource whose status is
 * that binary: users, workspaces, namespaces.
 *
 * Extracted because the three had the same fifteen lines each -- badge
 * variant, both labels, the same comparison -- and they now all render it
 * inside their name cell, so a fourth copy would be a fourth place to
 * forget when the styling changes.
 */
export function ActiveStatusBadge({ status, className }: Props) {
  const { t } = useTranslation()
  const active = status === "active"
  return (
    <Badge variant={active ? "success" : "secondary"} className={className}>
      {active ? t("common.active") : t("common.inactive")}
    </Badge>
  )
}
