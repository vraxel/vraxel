import { Info } from "lucide-react"
import { Input } from "@/shared/ui/input"
import { Label } from "@/shared/ui/label"
import { Textarea } from "@/shared/ui/textarea"
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group"
import { cn } from "@/shared/lib/utils"
import { useTranslation } from "@/i18n"

export type NamingMode = "auto" | "reserved"

interface Props {
  mode: NamingMode
  hostName: string
  description: string
  scopeLabel: string
  nameError?: string
  onModeChange: (mode: NamingMode) => void
  onHostNameChange: (value: string) => void
  onDescriptionChange: (value: string) => void
}

export function StepIdentity({
  mode,
  hostName,
  description,
  scopeLabel,
  nameError,
  onModeChange,
  onHostNameChange,
  onDescriptionChange,
}: Props) {
  const { t } = useTranslation()

  return (
    <div className="space-y-6">
      {/* Placement is shown, never asked. It comes from the scope this
          page is open in, because that is what the backend derives the
          token's scope from -- a picker here would imply the frontend
          decides placement, and it does not. */}
      <div className="border-border-subtle bg-muted/40 flex items-start gap-3 rounded-lg border p-3">
        <Info className="text-muted-foreground mt-0.5 size-4 shrink-0" />
        <div className="space-y-0.5 text-sm">
          <p>
            {t("compute.onboard.identity.scope")}
            <span className="ml-1.5 font-medium">{scopeLabel}</span>
          </p>
          <p className="text-muted-foreground text-xs">
            {t("compute.onboard.identity.scopeHint")}
          </p>
        </div>
      </div>

      <RadioGroup
        value={mode}
        onValueChange={(v) => onModeChange(v as NamingMode)}
        className="space-y-3"
      >
        <NamingOption
          value="auto"
          selected={mode === "auto"}
          title={t("compute.onboard.identity.auto.title")}
          desc={t("compute.onboard.identity.auto.desc")}
        />
        <NamingOption
          value="reserved"
          selected={mode === "reserved"}
          title={t("compute.onboard.identity.reserved.title")}
          desc={t("compute.onboard.identity.reserved.desc")}
        >
          {mode === "reserved" && (
            <div className="mt-4 space-y-4 pl-7">
              <div className="space-y-1.5">
                <Label htmlFor="onboard-host-name">
                  {t("compute.onboard.identity.hostName")}
                </Label>
                <Input
                  id="onboard-host-name"
                  value={hostName}
                  autoComplete="off"
                  placeholder={t("compute.onboard.identity.hostNamePlaceholder")}
                  onChange={(e) => onHostNameChange(e.target.value)}
                  aria-invalid={!!nameError}
                />
                {nameError ? (
                  <p className="text-destructive text-xs">{nameError}</p>
                ) : (
                  <p className="text-muted-foreground text-xs">
                    {t("compute.onboard.identity.hostNameHint")}
                  </p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="onboard-host-desc">{t("common.description")}</Label>
                <Textarea
                  id="onboard-host-desc"
                  rows={2}
                  value={description}
                  onChange={(e) => onDescriptionChange(e.target.value)}
                />
              </div>
            </div>
          )}
        </NamingOption>
      </RadioGroup>

      {/* The point worth making loudly: there is no spec form. */}
      <p className="text-muted-foreground text-xs">
        {t("compute.onboard.identity.noSpecForm")}
      </p>
    </div>
  )
}

function NamingOption({
  value,
  selected,
  title,
  desc,
  children,
}: {
  value: string
  selected: boolean
  title: string
  desc: string
  children?: React.ReactNode
}) {
  const id = `naming-${value}`
  return (
    <div
      className={cn(
        "rounded-xl border p-4 transition-colors",
        selected
          ? "border-primary bg-primary-subtle/40"
          : "border-border-subtle hover:border-border",
      )}
    >
      {/* The label covers the radio and its copy, and stops there. The
          expanded fields are siblings: a text input nested inside a label
          that points at a different control makes the label's accessible
          name ambiguous, and every click in the field area becomes a
          candidate for the label's activation behaviour. */}
      <label htmlFor={id} className="flex cursor-pointer items-start gap-3">
        <RadioGroupItem id={id} value={value} className="mt-0.5" />
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium">{title}</span>
          <span className="text-muted-foreground mt-1 block text-sm">{desc}</span>
        </span>
      </label>
      {children}
    </div>
  )
}
