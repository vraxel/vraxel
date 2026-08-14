import { Cloud, Info, Keyboard, Zap } from "lucide-react"
import type { ReactNode } from "react"
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group"
import { Badge } from "@/shared/ui/badge"
import { cn } from "@/shared/lib/utils"
import { useTranslation } from "@/i18n"

/** How the host record comes into existence. */
export type Method = "agent" | "import"
/** Where an imported record's facts come from. */
export type ImportSource = "manual" | "cloud"

interface Props {
  method: Method
  source: ImportSource
  scopeLabel: string
  onMethodChange: (m: Method) => void
  onSourceChange: (s: ImportSource) => void
}

/**
 * Step one: how this host joins.
 *
 * The two options are not two products, they are two orders of the same
 * two facts (how the record is created, and how the control plane
 * reaches the host). Agent onboarding does both at once; importing does
 * the record first and leaves the channel for later or never. Which is
 * why "install the agent" appears again as its own step under import,
 * and again on the host detail page: it is one operation with several
 * entry points, not a branch of creation.
 *
 * Agent is the default because it is the only path that asks for
 * nothing: no address, no credential, no route from the control plane
 * into the host.
 */
export function StepMethod({ method, source, scopeLabel, onMethodChange, onSourceChange }: Props) {
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
            {t("compute.onboard.method.scope")}
            <span className="ml-1.5 font-medium">{scopeLabel}</span>
          </p>
          <p className="text-muted-foreground text-xs">{t("compute.onboard.method.scopeHint")}</p>
        </div>
      </div>

      <RadioGroup
        value={method}
        onValueChange={(v) => onMethodChange(v as Method)}
        className="space-y-3"
      >
        <MethodOption
          value="agent"
          selected={method === "agent"}
          icon={<Zap className="size-4" />}
          title={t("compute.onboard.method.agent.title")}
          desc={t("compute.onboard.method.agent.desc")}
          badge={t("compute.onboard.method.recommended")}
        />
        <MethodOption
          value="import"
          selected={method === "import"}
          icon={<Keyboard className="size-4" />}
          title={t("compute.onboard.method.import.title")}
          desc={t("compute.onboard.method.import.desc")}
        >
          {method === "import" && (
            <div className="mt-4 space-y-2 pl-7">
              <p className="text-muted-foreground text-xs">
                {t("compute.onboard.method.import.sourceLabel")}
              </p>
              <RadioGroup
                value={source}
                onValueChange={(v) => onSourceChange(v as ImportSource)}
                className="grid gap-2 sm:grid-cols-2"
              >
                <SourceOption
                  value="manual"
                  selected={source === "manual"}
                  icon={<Keyboard className="size-3.5" />}
                  label={t("compute.onboard.method.import.manual")}
                />
                {/* Present and disabled rather than absent: the shape of
                    the flow is the point of this step, and a source that
                    silently appears later reads as a redesign instead of
                    the slot it was always going to fill. */}
                <SourceOption
                  value="cloud"
                  selected={false}
                  disabled
                  icon={<Cloud className="size-3.5" />}
                  label={t("compute.onboard.method.import.cloud")}
                  hint={t("compute.onboard.method.import.cloudSoon")}
                />
              </RadioGroup>
            </div>
          )}
        </MethodOption>
      </RadioGroup>
    </div>
  )
}

function MethodOption({
  value,
  selected,
  icon,
  title,
  desc,
  badge,
  children,
}: {
  value: string
  selected: boolean
  icon: ReactNode
  title: string
  desc: string
  badge?: string
  children?: ReactNode
}) {
  const id = `method-${value}`
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
          expanded controls are siblings: a nested control that the label
          does not point at makes the label's accessible name ambiguous. */}
      <label htmlFor={id} className="flex cursor-pointer items-start gap-3">
        <RadioGroupItem id={id} value={value} className="mt-0.5" />
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className={cn("shrink-0", selected ? "text-primary" : "text-muted-foreground")}>
              {icon}
            </span>
            <span className="text-sm font-medium">{title}</span>
            {badge && (
              <Badge variant="secondary" className="font-normal">
                {badge}
              </Badge>
            )}
          </span>
          <span className="text-muted-foreground mt-1 block text-sm">{desc}</span>
        </span>
      </label>
      {children}
    </div>
  )
}

function SourceOption({
  value,
  selected,
  disabled,
  icon,
  label,
  hint,
}: {
  value: string
  selected: boolean
  disabled?: boolean
  icon: ReactNode
  label: string
  hint?: string
}) {
  const id = `source-${value}`
  return (
    <label
      htmlFor={id}
      className={cn(
        "flex items-center gap-2.5 rounded-lg border px-3 py-2.5 text-sm transition-colors",
        disabled
          ? "border-border-subtle text-muted-foreground cursor-not-allowed opacity-60"
          : "cursor-pointer",
        selected && !disabled && "border-primary bg-primary-subtle/40",
        !selected && !disabled && "border-border-subtle hover:border-border",
      )}
    >
      <RadioGroupItem id={id} value={value} disabled={disabled} />
      <span className="shrink-0">{icon}</span>
      <span className="min-w-0">
        <span className="block truncate">{label}</span>
        {hint && <span className="text-muted-foreground block text-xs">{hint}</span>}
      </span>
    </label>
  )
}
