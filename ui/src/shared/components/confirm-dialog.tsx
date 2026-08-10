import { useState } from "react"
import { Loader2 } from "lucide-react"
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/shared/ui/dialog"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"
import { useTranslation } from "@/i18n"

interface ConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  // onConfirm may be sync or async. While the returned promise is
  // pending, both buttons are disabled and ESC / overlay close are
  // blocked so callers cannot fire a duplicate request.
  onConfirm: () => void | Promise<unknown>
  variant?: "destructive" | "default"
  confirmText?: string
  // When set, the confirm button stays disabled until the operator types
  // this exact string. Used for high-risk destructive ops (delete/drain a
  // k8s cluster / node / namespace) where a bare "确定?" dialog is too easy
  // to fat-finger (bug #533).
  requireNameConfirmation?: string
}

export function ConfirmDialog({
  open, onOpenChange, title, description, onConfirm,
  variant = "destructive", confirmText, requireNameConfirmation,
}: ConfirmDialogProps) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [typed, setTyped] = useState("")
  // Clear the typed name whenever the dialog opens/closes or the target
  // changes, so a fresh confirmation is always required
  // (adjust-during-render, not an effect).
  const [prevResetKey, setPrevResetKey] = useState<string>(`${open}|${requireNameConfirmation ?? ""}`)
  const resetKey = `${open}|${requireNameConfirmation ?? ""}`
  if (prevResetKey !== resetKey) {
    setPrevResetKey(resetKey)
    setTyped("")
  }
  const nameMismatch = requireNameConfirmation !== undefined && typed !== requireNameConfirmation
  const handleConfirm = async () => {
    if (busy || nameMismatch) return
    setBusy(true)
    try {
      await onConfirm()
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={(v) => { if (!busy) onOpenChange(v) }}>
      <DialogContent
        onCloseAutoFocus={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => { if (busy) e.preventDefault() }}
        onPointerDownOutside={(e) => { if (busy) e.preventDefault() }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {/* description typically interpolates a user-supplied resource name
              (e.g. nacos namespace name, config dataId). Without an explicit
              word-break rule, a long unbroken name with no whitespace
              overflows DialogContent horizontally and clips past the right
              edge -- shadcn's DialogDescription only sets text-sm
              text-muted-foreground by default. break-all is intentional
              over break-words because user-supplied IDs are often pure ASCII
              with no natural break opportunities. (bug #227) */}
          <DialogDescription className="break-all whitespace-pre-line">{description}</DialogDescription>
        </DialogHeader>
        {requireNameConfirmation !== undefined && (
          <div className="space-y-1.5">
            <p className="text-sm text-muted-foreground break-all">
              {t("common.confirmByName.label", { name: requireNameConfirmation })}
            </p>
            <Input
              name="confirmName"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              placeholder={requireNameConfirmation}
              autoComplete="off"
              disabled={busy}
            />
            {typed.length > 0 && nameMismatch && (
              <p className="text-sm text-destructive">{t("common.confirmByName.mismatch")}</p>
            )}
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={() => onOpenChange(false)}>{t("common.cancel")}</Button>
          <Button variant={variant} disabled={busy || nameMismatch} onClick={handleConfirm}>
            {busy && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
            {confirmText ?? t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
