import { useState } from "react"
import { toast } from "sonner"
import { TriangleAlert } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog"
import { Button } from "@/shared/ui/button"
import { Skeleton } from "@/shared/ui/skeleton"
import { useApiQuery } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { showApiError } from "@/core/api/client"
import { useTranslation } from "@/i18n"
import type { ScopeRef } from "@/core/registry/resource"
import { hostImageSiblingsApi, mergeHost } from "@/modules/compute/api/hosts"
import { hostsDef } from "@/modules/compute/defs"
import type { Host } from "@/modules/compute/api/types"
import { AgentStatusBadge } from "./agent-status-badge"

/**
 * Fold a duplicate host record into the record it duplicates.
 *
 * The dialog leads with the evidence rather than a verdict, because the
 * server genuinely does not know: a machine whose hardware was replaced
 * and a machine cloned from a powered-off template produce exactly the
 * same signals. Only the operator knows which happened to this machine,
 * so the job here is to show them what we saw and let them decide -- not
 * to recommend one and have them click past it.
 */
export function HostMergeDialog({
  host,
  scope,
  onClose,
  onMerged,
}: {
  /** The record the operator is looking at; null while closed. */
  host: Host | null
  scope: ScopeRef
  onClose: () => void
  onMerged: () => void
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const open = !!host

  // Asked of the server, not filtered client-side: sameness is
  // /etc/machine-id, which is deliberately not a field on the host -- the
  // id means nothing to an operator, and matching on anything they CAN
  // see (a hostname, say) would offer to merge unrelated machines.
  const query = useApiQuery({
    queryKey: qk.sub(hostsDef, scope, host?.metadata.id ?? "", "image-siblings"),
    queryFn: () => hostImageSiblingsApi.list(scope, host!.metadata.id),
    enabled: open,
  })
  const siblings = query.data?.items ?? []

  const merge = async (survivor: Host) => {
    if (!host) return
    setBusy(true)
    try {
      // The record the operator picks survives; the one they were
      // looking at is absorbed into it.
      await mergeHost(scope, survivor.metadata.id, { sourceHostId: host.metadata.id })
      toast.success(t("compute.host.mergeSuccess"))
      onClose()
      onMerged()
    } catch (err) {
      showApiError(err, t, "compute.host.title")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v && !busy) onClose()
      }}
    >
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("compute.host.merge")}</DialogTitle>
          <DialogDescription>{t("compute.host.mergeHint")}</DialogDescription>
        </DialogHeader>

        <div className="border-warning/25 bg-warning/10 flex items-start gap-3 rounded-lg border p-3 text-sm">
          <TriangleAlert className="text-warning mt-0.5 size-4 shrink-0" />
          <p>{t("compute.host.mergeEvidence")}</p>
        </div>

        <div className="space-y-2">
          <p className="text-sm font-medium">{t("compute.host.mergeInto")}</p>
          {query.isPending ? (
            <Skeleton className="h-16 w-full rounded-lg" />
          ) : siblings.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t("compute.host.mergeNoCandidates")}</p>
          ) : (
            siblings.map((s) => (
              <div
                key={s.metadata.id}
                className="border-border-subtle flex items-center gap-3 rounded-lg border p-3"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {s.spec.displayName || s.metadata.name}
                    </span>
                    <AgentStatusBadge
                      status={s.spec.agentStatus}
                      conflictAt={s.spec.agentConflictAt}
                    />
                  </div>
                  <p className="text-muted-foreground truncate font-mono text-xs">
                    {s.spec.reportedPrimaryIp || "-"}
                  </p>
                </div>
                <Button size="sm" variant="outline" disabled={busy} onClick={() => merge(s)}>
                  {t("compute.host.mergeConfirm")}
                </Button>
              </div>
            ))
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={onClose}>
            {t("common.cancel")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
