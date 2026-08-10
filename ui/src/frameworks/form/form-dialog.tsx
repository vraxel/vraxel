import type { ReactNode } from "react"
import type { UseFormReturn, FieldValues } from "react-hook-form"
import { Loader2 } from "lucide-react"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog"
import { Form } from "@/shared/ui/form"
import { Button } from "@/shared/ui/button"
import { FormErrorBanner } from "@/shared/components/form-error-banner"
import { useTranslation } from "@/i18n"

export interface FormDialogProps<T extends FieldValues> {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  form: UseFormReturn<T>
  onSubmit: (values: T) => void | Promise<void>
  submitting?: boolean
  /** Width class; must carry the sm: prefix. Default 4xl. */
  widthClass?: string
  /** Body-container class. Default: single scrollable column. Override for
   *  multi-pane bodies that manage their own overflow (e.g. the role
   *  editor's fields + permission-tree split). */
  bodyClassName?: string
  /** Save-button label override (default common.save). */
  submitLabel?: ReactNode
  /** Extra footer controls, left of Cancel/Save. */
  footerExtra?: ReactNode
  children: ReactNode
}

/**
 * The standard form-Dialog shell (frameworks/form). Implements the
 * fixed-height DialogContent + single scrollable body + non-scrolling
 * footer sandwich exactly once, so create/edit forms stop copying it by
 * hand. The page owns the RHF form + zod schema + fields; this only
 * renders the frame.
 */
export function FormDialog<T extends FieldValues>({
  open,
  onOpenChange,
  title,
  form,
  onSubmit,
  submitting,
  widthClass = "sm:max-w-4xl",
  bodyClassName = "-mx-1 min-h-0 flex-1 space-y-4 overflow-y-auto px-1",
  submitLabel,
  footerExtra,
  children,
}: FormDialogProps<T>) {
  const { t } = useTranslation()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={`${widthClass} flex max-h-[85vh] flex-col overflow-hidden`}
        onOpenAutoFocus={(e) => e.preventDefault()}
        onCloseAutoFocus={(e) => e.preventDefault()}
        aria-describedby={undefined}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="flex min-h-0 flex-1 flex-col">
            {/* Outside the scroll container (like the paas shell): a root
                error must stay visible when a long form is scrolled to the
                bottom at submit time (review finding C16). */}
            <FormErrorBanner errors={form.formState.errors} />
            <div className={bodyClassName}>{children}</div>
            <DialogFooter className="mt-6 shrink-0 border-t pt-4">
              {footerExtra}
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {submitLabel ?? t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
