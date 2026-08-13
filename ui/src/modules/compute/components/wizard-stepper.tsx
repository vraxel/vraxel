import { Check } from "lucide-react"
import { cn } from "@/shared/lib/utils"

export interface WizardStep {
  /** Stable id, used as the React key and by the page to switch content. */
  id: string
  label: string
}

interface Props {
  steps: WizardStep[]
  /** Index of the step being edited. */
  current: number
}

/**
 * Horizontal stepper for the full-page creation wizards.
 *
 * Steps are a prop rather than a constant because the set depends on the
 * chosen creation method: agent onboarding is three steps, provisioning
 * from a cloud pool will be six (pool, template, spec, network, confirm,
 * result). The stepper renders whatever it is handed, so adding that
 * branch is a change to the step list, not to this component.
 */
export function WizardStepper({ steps, current }: Props) {
  return (
    <ol className="flex items-center justify-center gap-2">
      {steps.map((step, i) => {
        const done = i < current
        const active = i === current
        return (
          <li key={step.id} className="flex items-center gap-2">
            <div className="flex items-center gap-2.5">
              <span
                className={cn(
                  "flex size-7 shrink-0 items-center justify-center rounded-full border text-xs font-medium transition-colors",
                  done && "border-primary bg-primary text-primary-foreground",
                  active && "border-primary text-primary bg-primary-subtle",
                  !done && !active && "border-border text-muted-foreground",
                )}
              >
                {done ? <Check className="size-3.5" /> : i + 1}
              </span>
              <span
                className={cn(
                  "text-sm whitespace-nowrap",
                  active ? "text-foreground font-medium" : "text-muted-foreground",
                )}
              >
                {step.label}
              </span>
            </div>
            {i < steps.length - 1 && (
              <span
                aria-hidden
                className={cn("mx-2 h-px w-10 md:w-16", done ? "bg-primary" : "bg-border")}
              />
            )}
          </li>
        )
      })}
    </ol>
  )
}
