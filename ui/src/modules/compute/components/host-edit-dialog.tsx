import { useEffect, useState } from "react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import { FormDialog } from "@/frameworks/form/form-dialog"
import { handleFormApiError } from "@/core/api/client"
import { useTranslation } from "@/i18n"
import { hostsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import type { ScopeRef } from "@/core/registry/resource"

interface HostEditFormValues {
  displayName: string
  description: string
}

export function HostEditDialog({
  host,
  scope,
  onClose,
  onSuccess,
}: {
  host: Host | null
  scope: ScopeRef
  onClose: () => void
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)

  const schema = z.object({
    displayName: z
      .string()
      .max(128, t("api.validation.maxLength", { max: 128 }))
      .optional(),
    description: z
      .string()
      .max(1000, t("api.validation.maxLength", { max: 1000 }))
      .optional(),
  })

  const form = useForm<HostEditFormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: { displayName: "", description: "" },
  })

  useEffect(() => {
    if (host) {
      form.reset({
        displayName: host.spec.displayName ?? "",
        description: host.spec.description ?? "",
      })
    }
  }, [host, form])

  const onSubmit = async (values: HostEditFormValues) => {
    if (!host) return
    setLoading(true)
    try {
      await hostsApi.update(scope, host.metadata.id, {
        spec: {
          displayName: values.displayName,
          description: values.description,
        },
      })
      toast.success(t("action.updateSuccess"))
      onClose()
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "host", "compute.host.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={!!host}
      onOpenChange={(v) => {
        if (!v) onClose()
      }}
      title={t("compute.host.edit")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:max-w-lg"
    >
      <FormField
        control={form.control}
        name="displayName"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("common.displayName")}</FormLabel>
            <FormControl>
              <Input {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="description"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("common.description")}</FormLabel>
            <FormControl>
              <Textarea rows={3} {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </FormDialog>
  )
}
