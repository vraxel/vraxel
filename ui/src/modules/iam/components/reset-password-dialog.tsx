import { useEffect, useState } from "react"
import { Loader2 } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/ui/dialog"
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/shared/ui/form"
import { Button } from "@/shared/ui/button"
import { Input } from "@/shared/ui/input"
import { resetPassword } from "@/modules/iam/api/users"
import { ApiError, formApiErrorMessage, translateDetailMessage } from "@/core/api/client"
import { useTranslation } from "@/i18n"

interface ResetPasswordDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: string
  username: string
  onSuccess?: () => void
}

export function ResetPasswordDialog({
  open,
  onOpenChange,
  userId,
  username,
  onSuccess,
}: ResetPasswordDialogProps) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)

  const schema = z
    .object({
      newPassword: z
        .string()
        .min(8, t("api.validation.password.length"))
        .max(72, t("api.validation.password.length"))
        .regex(/[A-Z]/, t("api.validation.password.uppercase"))
        .regex(/[a-z]/, t("api.validation.password.lowercase"))
        .regex(/[0-9]/, t("api.validation.password.digit")),
      confirmPassword: z.string(),
    })
    .refine((d) => d.newPassword === d.confirmPassword, {
      message: t("userMenu.passwordMismatch"),
      path: ["confirmPassword"],
    })

  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onBlur",
    defaultValues: { newPassword: "", confirmPassword: "" },
  })

  // Reset form whenever dialog re-opens for a different user.
  useEffect(() => {
    if (open) form.reset({ newPassword: "", confirmPassword: "" })
  }, [open, userId, form])

  const onSubmit = async (values: FormValues) => {
    setLoading(true)
    try {
      await resetPassword(userId, { newPassword: values.newPassword })
      toast.success(t("action.resetPasswordSuccess"))
      onOpenChange(false)
      onSuccess?.()
    } catch (err) {
      if (err instanceof ApiError && err.details?.length) {
        for (const d of err.details) {
          const i18nKey = translateDetailMessage(d.message)
          form.setError(d.field as keyof FormValues, {
            message: i18nKey !== d.message ? t(i18nKey) : d.message,
          })
        }
      } else if (err instanceof ApiError) {
        form.setError("root", { message: formApiErrorMessage(err, t) })
      } else {
        form.setError("root", { message: t("api.error.internalError") })
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) form.reset()
      }}
    >
      <DialogContent onOpenAutoFocus={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>{t("user.resetPasswordTitle", { name: username })}</DialogTitle>
          <DialogDescription>{t("user.resetPasswordHint")}</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            {form.formState.errors.root && (
              <div className="bg-destructive/10 text-destructive rounded-md px-3 py-2 text-sm break-words whitespace-pre-line">
                {form.formState.errors.root.message}
              </div>
            )}
            <FormField
              control={form.control}
              name="newPassword"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t("userMenu.newPassword")}</FormLabel>
                  <FormControl>
                    <Input type="password" autoComplete="new-password" {...field} />
                  </FormControl>
                  <FormDescription>{t("api.validation.password.hint")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="confirmPassword"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t("userMenu.confirmPassword")}</FormLabel>
                  <FormControl>
                    <Input type="password" autoComplete="new-password" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" variant="destructive" disabled={loading}>
                {loading && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
                {t("user.resetPassword")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
