import { Badge } from "@/shared/ui/badge"
import { cn } from "@/shared/lib/utils"
import { useTranslation } from "@/i18n"

interface Props {
  status?: "online" | "offline"
  /** Set while two live agent processes claim this host's identity. */
  conflictAt?: string
}

/**
 * The list's status indicator, sized to sit inside the name cell.
 *
 * A dot rather than a chip because it shares the cell with the name:
 * status is what an operator scans a host list for, so it has to be on
 * the line their eye is already on, and a full badge there would push the
 * name around as its text changes length. The state is still readable --
 * the dot carries a title, and the toolbar filter names all three.
 *
 * A contended identity is the exception and keeps its chip: it is rare,
 * it means the host is unmanageable, and a red dot alone would read as
 * "offline" to anyone who has not met it before.
 */
export function AgentStatusDot({ status, conflictAt }: Props) {
  const { t } = useTranslation()
  const label = conflictAt
    ? t("compute.agent.conflict")
    : status === "online"
      ? t("compute.agent.online")
      : t("compute.agent.offline")
  return (
    <span
      title={label}
      aria-label={label}
      className={cn(
        "size-2 shrink-0 rounded-full",
        conflictAt
          ? "bg-destructive"
          : status === "online"
            ? "bg-success"
            : "bg-muted-foreground/40",
      )}
    />
  )
}

/**
 * The host list's primary status.
 *
 * Conflict outranks online/offline: while it is set the gateway refuses
 * every channel for this host, including the one that looks like the
 * original, so reporting "offline" would be true but useless -- the
 * operator needs to know the host is unmanageable and why.
 */
export function AgentStatusBadge({ status, conflictAt }: Props) {
  const { t } = useTranslation()

  if (conflictAt) {
    return (
      <Badge variant="destructive" title={t("compute.agent.conflictHint")}>
        {t("compute.agent.conflict")}
      </Badge>
    )
  }
  return status === "online" ? (
    <Badge variant="success">{t("compute.agent.online")}</Badge>
  ) : (
    <Badge variant="secondary">{t("compute.agent.offline")}</Badge>
  )
}
