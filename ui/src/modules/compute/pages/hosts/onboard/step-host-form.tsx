import { Input } from "@/shared/ui/input"
import { Label } from "@/shared/ui/label"
import { Textarea } from "@/shared/ui/textarea"
import { Checkbox } from "@/shared/ui/checkbox"
import { useTranslation } from "@/i18n"

export interface HostDraft {
  name: string
  description: string
  ip: string
  sshPort: string
  autoInstallAgent: boolean
}

interface Props {
  draft: HostDraft
  nameError?: string
  onChange: (patch: Partial<HostDraft>) => void
}

/**
 * Step two of the import path: the host record itself.
 *
 * The address is optional. A host that will only ever be reached through
 * an outbound agent has no address the control plane can use, and
 * demanding one would force operators to invent a value for a field
 * nothing reads. It becomes required only when something here has to
 * dial the host, which today means ticking auto-install.
 *
 * Nothing about CPU / memory / disk / OS is asked: those arrive from the
 * agent, and a hand-typed copy would be wrong within a quarter.
 */
export function StepHostForm({ draft, nameError, onChange }: Props) {
  const { t } = useTranslation()
  const ipRequired = draft.autoInstallAgent

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 sm:items-start">
        <div className="space-y-1.5">
          <Label htmlFor="import-host-name">
            {t("compute.onboard.form.name")}
            <span className="text-destructive ml-0.5">*</span>
          </Label>
          <Input
            id="import-host-name"
            value={draft.name}
            autoComplete="off"
            placeholder={t("compute.onboard.form.namePlaceholder")}
            onChange={(e) => onChange({ name: e.target.value })}
            aria-invalid={!!nameError}
          />
          {nameError ? (
            <p className="text-destructive text-xs">{nameError}</p>
          ) : (
            <p className="text-muted-foreground text-xs">{t("compute.onboard.form.nameHint")}</p>
          )}
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="import-host-ip">
            {t("compute.onboard.form.ip")}
            {ipRequired && <span className="text-destructive ml-0.5">*</span>}
          </Label>
          <Input
            id="import-host-ip"
            value={draft.ip}
            autoComplete="off"
            placeholder="10.1.1.12"
            onChange={(e) => onChange({ ip: e.target.value })}
          />
          <p className="text-muted-foreground text-xs">
            {ipRequired ? t("compute.onboard.form.ipRequired") : t("compute.onboard.form.ipHint")}
          </p>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 sm:items-start">
        <div className="space-y-1.5">
          <Label htmlFor="import-host-port">{t("compute.onboard.form.sshPort")}</Label>
          <Input
            id="import-host-port"
            value={draft.sshPort}
            inputMode="numeric"
            onChange={(e) => onChange({ sshPort: e.target.value })}
          />
        </div>
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="import-host-desc">{t("common.description")}</Label>
        <Textarea
          id="import-host-desc"
          rows={2}
          value={draft.description}
          onChange={(e) => onChange({ description: e.target.value })}
        />
      </div>

      <div className="border-border-subtle rounded-xl border p-4">
        <label htmlFor="import-auto-install" className="flex cursor-pointer items-start gap-3">
          <Checkbox
            id="import-auto-install"
            checked={draft.autoInstallAgent}
            onCheckedChange={(v) => onChange({ autoInstallAgent: v === true })}
            className="mt-0.5"
          />
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-medium">
              {t("compute.onboard.form.autoInstall")}
            </span>
            <span className="text-muted-foreground mt-1 block text-sm">
              {t("compute.onboard.form.autoInstallDesc")}
            </span>
          </span>
        </label>
      </div>

      <p className="text-muted-foreground text-xs">{t("compute.onboard.form.noSpecForm")}</p>
    </div>
  )
}
