import { Check, Copy, Loader2, ShieldAlert } from "lucide-react"
import { toast } from "sonner"
import { useEffect, useRef, useState } from "react"
import { Link } from "react-router"
import { Button } from "@/shared/ui/button"
import { Badge } from "@/shared/ui/badge"
import { Skeleton } from "@/shared/ui/skeleton"
import { useTranslation } from "@/i18n"
import type { Host } from "@/modules/compute/api/types"

interface Props {
  /** Null while the token is still being minted. */
  command: string | null
  reservedName?: string
  /** Set once an agent has redeemed the token. */
  registeredHost: Host | null
}

export function StepInstall({ command, reservedName, registeredHost }: Props) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  // Cleared on unmount: without it the "copied" tick fires setState on a
  // page the operator already navigated away from.
  const copiedTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  useEffect(() => () => clearTimeout(copiedTimer.current), [])

  const copy = async () => {
    if (!command) return
    try {
      await navigator.clipboard.writeText(command)
      setCopied(true)
      toast.success(t("compute.onboard.install.copied"))
      clearTimeout(copiedTimer.current)
      copiedTimer.current = setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard is blocked outside a secure context; the command is on
      // screen and selectable, so this is a nuisance rather than a dead end.
      toast.error(t("compute.onboard.install.copyFailed"))
    }
  }

  return (
    <div className="space-y-5">
      <div className="border-warning/25 bg-warning/10 flex items-start gap-3 rounded-lg border p-3">
        <ShieldAlert className="text-warning mt-0.5 size-4 shrink-0" />
        <p className="text-sm">
          {t("compute.onboard.install.onceWarning")}
          {reservedName && (
            <span className="text-muted-foreground mt-1 block text-xs">
              {t("compute.onboard.install.reservedFor")}
              <span className="text-foreground ml-1 font-medium">{reservedName}</span>
            </span>
          )}
        </p>
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium">{t("compute.onboard.install.runOnHost")}</p>
          <Button size="sm" variant="outline" onClick={copy} disabled={!command}>
            {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            {t("compute.onboard.install.copy")}
          </Button>
        </div>
        {command ? (
          <pre className="border-border-subtle bg-muted/60 overflow-x-auto rounded-lg border p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap">
            {command}
          </pre>
        ) : (
          <Skeleton className="h-24 w-full rounded-lg" />
        )}
        <p className="text-muted-foreground text-xs">{t("compute.onboard.install.rootHint")}</p>
      </div>

      <div className="border-border-subtle rounded-xl border p-4">
        {registeredHost ? (
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <span className="bg-success/15 text-success flex size-6 items-center justify-center rounded-full">
                <Check className="size-3.5" />
              </span>
              <span className="text-sm font-medium">{t("compute.onboard.install.joined")}</span>
            </div>
            <dl className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm md:grid-cols-3">
              <Field label={t("common.name")} value={registeredHost.metadata.name} />
              <Field label={t("compute.host.ip")} value={registeredHost.spec.reportedPrimaryIp} />
              <Field label={t("compute.host.os")} value={registeredHost.spec.os} />
              <Field label={t("compute.host.arch")} value={registeredHost.spec.arch} />
              <Field
                label={t("compute.host.cpu")}
                value={`${registeredHost.spec.cpuCores ?? 0} ${t("compute.host.cores")}`}
              />
              <Field
                label={t("compute.host.memory")}
                value={`${Math.round((registeredHost.spec.memoryMb ?? 0) / 1024)} GiB`}
              />
            </dl>
            <p className="text-muted-foreground text-xs">
              {t("compute.onboard.install.reportedHint")}
            </p>
            <Button asChild size="sm" variant="outline">
              <Link to={`/compute/hosts/${registeredHost.metadata.id}`}>
                {t("compute.onboard.install.viewHost")}
              </Link>
            </Button>
          </div>
        ) : (
          <div className="flex items-center gap-3">
            <Loader2 className="text-muted-foreground size-4 animate-spin" />
            <div>
              <p className="text-sm">{t("compute.onboard.install.waiting")}</p>
              <p className="text-muted-foreground text-xs">
                {t("compute.onboard.install.waitingHint")}
              </p>
            </div>
            <Badge variant="secondary" className="ml-auto font-normal">
              {t("compute.onboard.install.canLeave")}
            </Badge>
          </div>
        )}
      </div>
    </div>
  )
}

function Field({ label, value }: { label: string; value?: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="truncate">{value || "-"}</dd>
    </div>
  )
}
