import { Check, PlugZap, TriangleAlert } from "lucide-react"
import { Link } from "react-router"
import { Button } from "@/shared/ui/button"
import { useTranslation } from "@/i18n"
import { AgentInstallPanel } from "@/modules/compute/components/agent-install-panel"
import type { Host } from "@/modules/compute/api/types"

interface Props {
  /** The row created by the previous step. Already persisted. */
  createdHost: Host
  hostsPath: string
  /** Whether the operator asked for the agent to be pushed over SSH. */
  autoInstall: boolean
  /** Why the push could not happen. Empty when it succeeded. */
  sshFailure?: string
  command: string | null
  registeredHost: Host | null
}

/**
 * Step three of the import path: give the host an agent.
 *
 * The host already exists when this renders, which is the whole point of
 * committing at the end of step two. Every outcome here is optional
 * follow-up work: skipping it leaves a perfectly valid host record, and
 * the same operation is available from the host detail page afterwards.
 * The footer button says "done" rather than "create" for that reason.
 */
export function StepAgent({
  createdHost,
  hostsPath,
  autoInstall,
  sshFailure,
  command,
  registeredHost,
}: Props) {
  const { t } = useTranslation()

  return (
    <div className="space-y-5">
      <div className="border-success/25 bg-success/10 flex items-start gap-3 rounded-lg border p-3">
        <span className="bg-success/20 text-success mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full">
          <Check className="size-3" />
        </span>
        <p className="text-sm">
          {t("compute.onboard.agent.hostCreated")}
          <Link
            to={`${hostsPath}/${createdHost.metadata.id}`}
            className="text-primary ml-1 font-medium hover:underline"
          >
            {createdHost.metadata.name}
          </Link>
          <span className="text-muted-foreground mt-1 block text-xs">
            {t("compute.onboard.agent.hostCreatedHint")}
          </span>
        </p>
      </div>

      {!autoInstall ? (
        <div className="border-border-subtle rounded-xl border p-4">
          <div className="flex items-start gap-3">
            <PlugZap className="text-muted-foreground mt-0.5 size-4 shrink-0" />
            <div className="space-y-1">
              <p className="text-sm font-medium">{t("compute.onboard.agent.skipped")}</p>
              <p className="text-muted-foreground text-sm">
                {t("compute.onboard.agent.skippedHint")}
              </p>
            </div>
          </div>
          <Button asChild size="sm" variant="outline" className="mt-3">
            <Link to={`${hostsPath}/${createdHost.metadata.id}`}>
              {t("compute.onboard.install.viewHost")}
            </Link>
          </Button>
        </div>
      ) : (
        <>
          {sshFailure && (
            <div className="border-warning/25 bg-warning/10 flex items-start gap-3 rounded-lg border p-3">
              <TriangleAlert className="text-warning mt-0.5 size-4 shrink-0" />
              <p className="text-sm">
                {t("compute.onboard.agent.sshFailed")}
                <span className="text-muted-foreground mt-1 block text-xs">{sshFailure}</span>
              </p>
            </div>
          )}
          <AgentInstallPanel
            command={command}
            hostsPath={hostsPath}
            boundHostName={createdHost.metadata.name}
            registeredHost={registeredHost}
            attaching
          />
        </>
      )}
    </div>
  )
}
