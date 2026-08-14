import { Badge } from "@/shared/ui/badge"
import { cn } from "@/shared/lib/utils"
import { useTranslation } from "@/i18n"

interface Props {
  /** "online" / "offline" as the server sends them, or undefined when no
   *  agent has ever been bound to this host. Typed as a plain string
   *  because that is what the wire carries (Go has no union to generate
   *  from); host_agents.status is CHECK-constrained to the two values,
   *  so the mapping below is exhaustive in practice. */
  status?: string
  /** Set while two live agent processes claim this host's identity. */
  conflictAt?: string
  className?: string
}

/**
 * The host's primary status.
 *
 * Three states, not two. A host imported by hand has never had an agent,
 * which is a different fact from an agent that is not answering: one is
 * waiting for an install, the other for a machine to come back. Calling
 * both "offline" sends the operator looking for a process that was never
 * there.
 *
 * Conflict outranks all of them: while it is set the gateway refuses
 * every channel for this host, including the one that looks like the
 * original, so reporting "offline" would be true but useless -- the
 * operator needs to know the host is unmanageable and why.
 */
export function AgentStatusBadge({ status, conflictAt, className }: Props) {
  const { t } = useTranslation()

  if (conflictAt) {
    return (
      <Badge variant="destructive" title={t("compute.agent.conflictHint")} className={className}>
        {t("compute.agent.conflict")}
      </Badge>
    )
  }
  if (!status) {
    return (
      <Badge
        variant="outline"
        title={t("compute.agent.notInstalledHint")}
        className={cn("text-muted-foreground", className)}
      >
        {t("compute.agent.notInstalled")}
      </Badge>
    )
  }
  return status === "online" ? (
    <Badge variant="success" className={className}>
      {t("compute.agent.online")}
    </Badge>
  ) : (
    <Badge variant="secondary" className={cn("text-muted-foreground", className)}>
      {t("compute.agent.offline")}
    </Badge>
  )
}
