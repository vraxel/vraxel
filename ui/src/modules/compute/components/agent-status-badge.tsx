import { Badge } from "@/shared/ui/badge"
import { useTranslation } from "@/i18n"

interface Props {
  status?: "online" | "offline"
  /** Set while two live agent processes claim this host's identity. */
  conflictAt?: string
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
