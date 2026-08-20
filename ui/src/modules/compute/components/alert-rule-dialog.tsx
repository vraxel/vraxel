import { useEffect, useState } from "react"
import { useForm } from "react-hook-form"
import { z } from "zod/v4"
import { zodResolver } from "@hookform/resolvers/zod"
import { toast } from "sonner"
import { Input } from "@/shared/ui/input"
import { Textarea } from "@/shared/ui/textarea"
import { Switch } from "@/shared/ui/switch"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select"
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/shared/ui/form"
import { FormDialog } from "@/frameworks/form/form-dialog"
import { handleFormApiError } from "@/core/api/client"
import { useTranslation } from "@/i18n"
import type { ScopeRef } from "@/core/registry/resource"
import type { HostAlertRule } from "@/generated/compute"
import { ALERT_METRICS, alertRulesApi } from "@/modules/compute/api/alert-rules"

const OPS = ["gt", "ge", "lt", "le"] as const
const SEVERITIES = ["info", "warning", "critical"] as const

interface AlertRuleFormValues {
  name: string
  description: string
  metric: string
  op: string
  threshold: string
  forSeconds: string
  severity: string
  enabled: boolean
}

const defaults: AlertRuleFormValues = {
  name: "",
  description: "",
  metric: "cpu_used_pct",
  op: "gt",
  threshold: "90",
  forSeconds: "60",
  severity: "warning",
  enabled: true,
}

/**
 * Create + edit in one dialog: `rule` set means edit, `createOpen` means
 * a blank create. Threshold and forSeconds live as strings in the form
 * (what an <input> holds) and parse at the boundary, where zod can say
 * which field is wrong.
 */
export function AlertRuleDialog({
  rule,
  createOpen,
  scope,
  onClose,
  onSuccess,
}: {
  rule: HostAlertRule | null
  createOpen: boolean
  scope: ScopeRef
  onClose: () => void
  onSuccess: () => void
}) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const open = createOpen || !!rule

  const schema = z.object({
    name: z
      .string()
      .min(1, t("api.validation.required"))
      .max(255, t("api.validation.maxLength", { max: 255 })),
    description: z.string().max(1000, t("api.validation.maxLength", { max: 1000 })),
    metric: z.string(),
    op: z.string(),
    threshold: z
      .string()
      .refine((v) => v.trim() !== "" && Number.isFinite(Number(v)), t("api.validation.required")),
    forSeconds: z
      .string()
      .refine(
        (v) => Number.isInteger(Number(v)) && Number(v) >= 0 && Number(v) <= 86400,
        t("compute.alertRule.forSecondsRange"),
      ),
    severity: z.string(),
    enabled: z.boolean(),
  })

  const form = useForm<AlertRuleFormValues>({
    resolver: zodResolver(schema) as never,
    mode: "onBlur",
    defaultValues: defaults,
  })

  useEffect(() => {
    if (rule) {
      form.reset({
        name: rule.metadata.name,
        description: rule.spec.description ?? "",
        metric: rule.spec.metric,
        op: rule.spec.op,
        threshold: String(rule.spec.threshold),
        forSeconds: String(rule.spec.forSeconds ?? 60),
        severity: rule.spec.severity,
        enabled: rule.spec.enabled !== false,
      })
    } else if (createOpen) {
      form.reset(defaults)
    }
  }, [rule, createOpen, form])

  const onSubmit = async (values: AlertRuleFormValues) => {
    setLoading(true)
    try {
      const body = {
        metadata: { name: values.name },
        spec: {
          description: values.description,
          metric: values.metric,
          op: values.op,
          threshold: Number(values.threshold),
          forSeconds: Number(values.forSeconds),
          severity: values.severity,
          enabled: values.enabled,
        },
      }
      if (rule) {
        await alertRulesApi.update(scope, rule.metadata.id, body)
        toast.success(t("action.updateSuccess"))
      } else {
        await alertRulesApi.create(scope, body)
        toast.success(t("action.createSuccess"))
      }
      onClose()
      onSuccess()
    } catch (err) {
      handleFormApiError(err, form, t, "host-alert-rule", "compute.alertRule.title")
    } finally {
      setLoading(false)
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose()
      }}
      title={rule ? t("compute.alertRule.edit") : t("compute.alertRule.create")}
      form={form}
      onSubmit={onSubmit}
      submitting={loading}
      widthClass="sm:max-w-lg"
    >
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("common.name")}</FormLabel>
            <FormControl>
              <Input {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <div className="grid grid-cols-3 gap-3">
        <FormField
          control={form.control}
          name="metric"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("compute.alertRule.metric")}</FormLabel>
              <Select value={field.value} onValueChange={field.onChange}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  {ALERT_METRICS.map((m) => (
                    <SelectItem key={m} value={m}>
                      {t(`compute.alertRule.metrics.${m}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="op"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("compute.alertRule.op")}</FormLabel>
              <Select value={field.value} onValueChange={field.onChange}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  {OPS.map((o) => (
                    <SelectItem key={o} value={o}>
                      {t(`compute.alertRule.ops.${o}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="threshold"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("compute.alertRule.threshold")}</FormLabel>
              <FormControl>
                <Input inputMode="decimal" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <FormField
          control={form.control}
          name="forSeconds"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("compute.alertRule.forSeconds")}</FormLabel>
              <FormControl>
                <Input inputMode="numeric" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="severity"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("compute.alertRule.severity")}</FormLabel>
              <Select value={field.value} onValueChange={field.onChange}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  {SEVERITIES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {t(`compute.alertRule.severities.${s}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <FormField
        control={form.control}
        name="description"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("common.description")}</FormLabel>
            <FormControl>
              <Textarea rows={2} {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name="enabled"
        render={({ field }) => (
          <FormItem className="flex flex-row items-center justify-between">
            <FormLabel>{t("compute.alertRule.enabled")}</FormLabel>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </FormItem>
        )}
      />
    </FormDialog>
  )
}
