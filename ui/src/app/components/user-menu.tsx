import { useState } from "react"
import { KeyRound, LogOut } from "lucide-react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog"
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
import { Avatar, AvatarFallback } from "@/shared/ui/avatar"
import { useAuthStore } from "@/core/auth/auth-store"
import { changePassword } from "@/modules/iam/api/users"
import { ApiError, formApiErrorMessage, translateDetailMessage } from "@/core/api/client"
import { logout } from "@/core/auth/auth"
import { useTranslation } from "@/i18n"

export function UserMenu() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const [passwordOpen, setPasswordOpen] = useState(false)

  const displayName = user?.name || user?.email || "User"
  const initials = displayName.charAt(0).toUpperCase()

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="sm" className="gap-2">
            <Avatar size="sm">
              <AvatarFallback>{initials}</AvatarFallback>
            </Avatar>
            <span className="max-w-24 truncate text-xs" title={displayName}>
              {displayName}
            </span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuLabel className="font-normal">
            <div className="flex flex-col gap-1">
              <p className="text-sm font-medium">{displayName}</p>
              {user?.email && <p className="text-muted-foreground text-xs">{user.email}</p>}
            </div>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => setPasswordOpen(true)}>
            <KeyRound className="mr-2 h-4 w-4" />
            {t("userMenu.changePassword")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => logout()}>
            <LogOut className="mr-2 h-4 w-4" />
            {t("userMenu.logout")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <ChangePasswordDialog open={passwordOpen} onOpenChange={setPasswordOpen} />
    </>
  )
}

function ChangePasswordDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user)
  const [loading, setLoading] = useState(false)

  const schema = z
    .object({
      // Optional: passwordless (social-login) accounts set their first
      // password with no old one. The server still rejects a blank old
      // password for accounts that already have one.
      oldPassword: z.string(),
      newPassword: z
        .string()
        .min(8, t("api.validation.password.length"))
        .max(72, t("api.validation.password.length"))
        .regex(/[A-Z]/, t("api.validation.password.uppercase"))
        .regex(/[a-z]/, t("api.validation.password.lowercase"))
        .regex(/[0-9]/, t("api.validation.password.digit")),
      confirmPassword: z.string(),
    })
    .refine((data) => data.newPassword !== data.oldPassword, {
      message: t("userMenu.passwordSameAsOld"),
      path: ["newPassword"],
    })
    .refine((data) => data.newPassword === data.confirmPassword, {
      message: t("userMenu.passwordMismatch"),
      path: ["confirmPassword"],
    })

  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onBlur",
    defaultValues: { oldPassword: "", newPassword: "", confirmPassword: "" },
  })

  const onSubmit = async (values: FormValues) => {
    if (!user?.sub) return
    setLoading(true)
    try {
      await changePassword(user.sub, {
        oldPassword: values.oldPassword,
        newPassword: values.newPassword,
      })
      onOpenChange(false)
      form.reset()
      logout()
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
      <DialogContent onOpenAutoFocus={(e) => e.preventDefault()} aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{t("userMenu.changePassword")}</DialogTitle>
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
              name="oldPassword"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("userMenu.oldPassword")}</FormLabel>
                  <FormControl>
                    <Input type="password" {...field} />
                  </FormControl>
                  <FormDescription>{t("userMenu.oldPasswordOptionalHint")}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="newPassword"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("userMenu.newPassword")}</FormLabel>
                  <FormControl>
                    <Input type="password" {...field} />
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
                  <FormLabel>{t("userMenu.confirmPassword")}</FormLabel>
                  <FormControl>
                    <Input type="password" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={loading}>
                {loading ? "..." : t("common.confirm")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
