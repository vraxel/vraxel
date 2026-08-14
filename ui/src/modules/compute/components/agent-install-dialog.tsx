import { useEffect, useState } from "react"
import { toast } from "sonner"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog"
import { useTranslation } from "@/i18n"
import { buildScopedPath } from "@/core/registry/nav-config"
import type { ScopeRef } from "@/core/registry/resource"
import { joinTokensApi } from "@/modules/compute/api/join-tokens"
import type { Host } from "@/modules/compute/api/types"
import { buildInstallCommand } from "@/modules/compute/install-command"
import { AgentInstallPanel } from "./agent-install-panel"

/**
 * Install (or reinstall) the agent on a host that already exists.
 *
 * One dialog for three situations the operator experiences as one thing:
 * a host imported by hand that never got an agent, a host whose agent
 * needs replacing, and a host whose credential the server no longer
 * honours. The install script always registers, so all three are the same
 * command -- which is why this mints a token and shows it rather than
 * asking the operator which case they are in.
 *
 * The token is bound to this host (targetHostId), so the machine that
 * redeems it adopts this row instead of creating a second one.
 */
export function AgentInstallDialog({
  host,
  scope,
  onClose,
}: {
  /** Null while closed, following the other host dialogs. */
  host: Host | null
  scope: ScopeRef
  onClose: () => void
}) {
  const { t } = useTranslation()
  const open = !!host
  const hostId = host?.metadata.id
  // Read out as scalars: host itself is refetched on every agent event,
  // so an effect depending on the object would mint a token per heartbeat.
  const hostName = host?.metadata.name
  const { ws, ns } = scope

  const [command, setCommand] = useState<string | null>(null)
  // The agent session that was live when this opened. Success is a NEW
  // session, not merely an online agent: reinstalling on a host whose
  // agent is already up would otherwise report "joined" the instant the
  // dialog rendered, before the operator had run anything.
  const [baseline, setBaseline] = useState<string | undefined>(undefined)
  const [openedFor, setOpenedFor] = useState<string | undefined>(undefined)
  if (open && openedFor !== hostId) {
    setOpenedFor(hostId)
    setBaseline(host?.spec.agentConnectedAt)
    setCommand(null)
  }

  useEffect(() => {
    if (!open || !hostId) return
    let live = true
    void (async () => {
      try {
        const token = await joinTokensApi.create(
          { ws, ns },
          { metadata: { name: hostName }, spec: { targetHostId: hostId } },
        )
        if (!live) return
        setCommand(buildInstallCommand(token.spec.serverUrl ?? "", token.spec.token ?? ""))
      } catch {
        if (live) toast.error(t("compute.onboard.install.mintFailed"))
      }
    })()
    return () => {
      live = false
    }
  }, [open, hostId, hostName, ws, ns, t])

  const rejoined =
    !!host && host.spec.agentStatus === "online" && host.spec.agentConnectedAt !== baseline

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose()
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t(host?.spec.agentId ? "compute.host.reinstallAgent" : "compute.host.installAgent")}
          </DialogTitle>
          <DialogDescription>{t("compute.host.installAgentHint")}</DialogDescription>
        </DialogHeader>
        <AgentInstallPanel
          command={command}
          hostsPath={buildScopedPath("hosts", ws ?? null, ns ?? null)}
          boundHostName={host?.metadata.name}
          registeredHost={rejoined ? host : null}
          attaching
        />
      </DialogContent>
    </Dialog>
  )
}
