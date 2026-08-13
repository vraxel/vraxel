import { Trash2 } from "lucide-react"
import { formatDateTime } from "@/shared/lib/format"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet"
import { Badge } from "@/shared/ui/badge"
import { Button } from "@/shared/ui/button"
import { Skeleton } from "@/shared/ui/skeleton"
import { useApiQuery } from "@/core/query/hooks"
import { qk } from "@/core/query/keys"
import { useTranslation } from "@/i18n"
import { joinTokensApi } from "@/modules/compute/api/join-tokens"
import { agentJoinTokensDef } from "@/modules/compute/defs"
import type { ScopeRef } from "@/core/registry/resource"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Tokens are scoped exactly like the hosts they mint, so the drawer
   *  lists the ones belonging to the scope the page is open in. */
  scope: ScopeRef
}

/**
 * Join tokens that have been handed out but not yet redeemed.
 *
 * A drawer rather than a page: minting one is a step inside onboarding,
 * not a resource an operator curates. It exists at all because the
 * plaintext is shown exactly once and may have been pasted somewhere it
 * should not be -- a credential you cannot withdraw is not acceptable.
 */
export function PendingTokensSheet({ open, onOpenChange, scope }: Props) {
  const { t } = useTranslation()

  const query = useApiQuery({
    queryKey: qk.list(agentJoinTokensDef, scope, {}),
    queryFn: () => joinTokensApi.list(scope),
    enabled: open,
  })
  const items = query.data?.items ?? []

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{t("compute.token.pending")}</SheetTitle>
          <SheetDescription>{t("compute.token.pendingHint")}</SheetDescription>
        </SheetHeader>

        <div className="space-y-3 overflow-y-auto px-4 pb-6">
          {query.isPending && <Skeleton className="h-20 w-full rounded-lg" />}

          {!query.isPending && items.length === 0 && (
            <p className="text-muted-foreground py-10 text-center text-sm">
              {t("compute.token.nonePending")}
            </p>
          )}

          {items.map((tok) => (
            <div key={tok.metadata.id} className="border-border-subtle rounded-lg border p-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {tok.metadata.name || t("compute.token.unnamed")}
                  </p>
                  {tok.spec.hostName && (
                    <p className="text-muted-foreground mt-0.5 text-xs">
                      {t("compute.token.reserves")}
                      <span className="text-foreground ml-1 font-medium">{tok.spec.hostName}</span>
                    </p>
                  )}
                </div>
                <Button variant="ghost" size="icon" aria-label={t("compute.token.revoke")}>
                  <Trash2 className="size-4" />
                </Button>
              </div>
              <div className="text-muted-foreground mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
                <Badge variant="outline" className="font-normal">
                  {t("compute.token.uses", {
                    used: String(tok.spec.usedCount ?? 0),
                    max: String(tok.spec.maxUses ?? 1),
                  })}
                </Badge>
                <span>
                  {t("compute.token.expiresAt")} {formatDateTime(tok.spec.expiresAt)}
                </span>
                <span>
                  {t("common.createdBy")} {tok.spec.createdByName || "-"}
                </span>
              </div>
            </div>
          ))}
        </div>
      </SheetContent>
    </Sheet>
  )
}
