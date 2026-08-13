import { Badge } from "@/shared/ui/badge"
import { cn } from "@/shared/lib/utils"
import { useTranslation } from "@/i18n"
import { ONBOARD_METHODS, type OnboardMethodId } from "./methods"

interface Props {
  value: OnboardMethodId
  onChange: (id: OnboardMethodId) => void
}

export function StepMethod({ value, onChange }: Props) {
  const { t } = useTranslation()

  return (
    <div className="space-y-3">
      {ONBOARD_METHODS.map((m) => {
        const selected = m.id === value
        const Icon = m.icon
        return (
          <button
            key={m.id}
            type="button"
            disabled={!m.available}
            aria-pressed={selected}
            onClick={() => onChange(m.id)}
            className={cn(
              "flex w-full items-start gap-4 rounded-xl border p-4 text-left transition-colors",
              selected
                ? "border-primary bg-primary-subtle/40"
                : "border-border-subtle hover:border-border",
              !m.available && "hover:border-border-subtle cursor-not-allowed opacity-55",
            )}
          >
            <span
              className={cn(
                "flex size-10 shrink-0 items-center justify-center rounded-lg",
                selected ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground",
              )}
            >
              <Icon className="size-5" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-2">
                <span className="text-sm font-medium">{t(m.titleKey)}</span>
                {m.badgeKey && (
                  <Badge variant="secondary" className="font-normal">
                    {t(m.badgeKey)}
                  </Badge>
                )}
              </span>
              <span className="text-muted-foreground mt-1 block text-sm">{t(m.descKey)}</span>
              {/* Flow preview. On an unavailable method this is the only
                  thing telling an operator what that path will ask for. */}
              <span className="text-muted-foreground/80 mt-2 block text-xs">
                {t("compute.onboard.flowPrefix")}
                {m.stepKeys.map((k) => t(k)).join(" → ")}
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}
