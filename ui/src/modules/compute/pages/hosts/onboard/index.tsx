import { useEffect, useMemo, useState } from "react"
import { Link, useNavigate, useParams } from "react-router"
import { ArrowLeft } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { useTranslation } from "@/i18n"
import { useWorkspaceStore } from "@/core/scope/workspace-store"
import { WizardStepper, type WizardStep } from "@/modules/compute/components/wizard-stepper"
import { createJoinToken, pollForRegisteredHost } from "@/modules/compute/api/join-tokens"
import type { Host } from "@/modules/compute/api/types"
import { ONBOARD_METHODS, type OnboardMethodId } from "./methods"
import { StepMethod } from "./step-method"
import { StepIdentity, type NamingMode } from "./step-identity"
import { StepInstall } from "./step-install"

// Host names follow the backend's rule (validation.go nameRegexp):
// alphanumerics, underscore and hyphen, 3-50 chars, alphanumeric at both
// ends. Checked here so a reserved name fails on this screen rather than
// on the host, minutes later, after the operator has walked to the rack.
const HOST_NAME_RE = /^[a-zA-Z0-9]([a-zA-Z0-9_-]{1,48}[a-zA-Z0-9])?$/

/**
 * Full-page host onboarding wizard.
 *
 * A page rather than a dialog because the flow branches: agent
 * onboarding is short, but provisioning from a cloud pool will ask for a
 * pool, a template, a spec, networking and a confirmation. A modal that
 * has to hold six steps ends up as LCP's 1026-line host-form-dialog,
 * where one component carries several unrelated creation semantics.
 *
 * The step list comes from the chosen method (see methods.ts), so that
 * branch lands as new step components plus one registry entry.
 */
export default function HostOnboardPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId, namespaceId } = useParams()
  const workspaceName = useWorkspaceStore((s) => s.currentWorkspaceName)

  const [stepIndex, setStepIndex] = useState(0)
  const [method, setMethod] = useState<OnboardMethodId>("agent")
  const [namingMode, setNamingMode] = useState<NamingMode>("auto")
  const [hostName, setHostName] = useState("")
  const [description, setDescription] = useState("")
  const [nameError, setNameError] = useState<string | undefined>()
  const [command, setCommand] = useState<string | null>(null)
  const [minting, setMinting] = useState(false)
  const [registeredHost, setRegisteredHost] = useState<Host | null>(null)

  const methodDef = ONBOARD_METHODS.find((m) => m.id === method) ?? ONBOARD_METHODS[0]

  const steps: WizardStep[] = useMemo(
    () => [
      { id: "method", label: t("compute.onboard.step.method") },
      ...methodDef.stepKeys.map((k) => ({ id: k, label: t(k) })),
    ],
    [methodDef, t],
  )

  const scopeLabel = namespaceId
    ? t("compute.onboard.scope.namespace", { name: namespaceId })
    : workspaceId
      ? t("compute.onboard.scope.workspace", { name: workspaceName ?? workspaceId })
      : t("compute.onboard.scope.platform")

  const isLastStep = stepIndex === steps.length - 1
  const hostsPath = "/compute/hosts"

  // DEMO: stands in for the host list refetch that will reveal the row
  // once an agent redeems the token.
  useEffect(() => {
    if (!command || registeredHost) return
    let live = true
    void pollForRegisteredHost().then((host) => {
      if (live) setRegisteredHost(host)
    })
    return () => {
      live = false
    }
  }, [command, registeredHost])

  const goNext = async () => {
    // Step 0 -> method chosen.
    if (stepIndex === 0) {
      setStepIndex(1)
      return
    }
    // Last editable step of the agent branch: mint the token, then reveal
    // the command. Minting on the way in (rather than on page load) means
    // an abandoned wizard leaves no live credential behind.
    if (stepIndex === 1) {
      if (namingMode === "reserved" && !HOST_NAME_RE.test(hostName.trim())) {
        setNameError(t("compute.onboard.identity.nameInvalid"))
        return
      }
      setNameError(undefined)
      setMinting(true)
      setStepIndex(2)
      try {
        const token = await createJoinToken(
          {},
          {
            hostName: namingMode === "reserved" ? hostName.trim() : undefined,
            name: description.trim() || undefined,
          },
        )
        setCommand(buildInstallCommand(token.spec.token ?? ""))
      } catch {
        toast.error(t("compute.onboard.install.mintFailed"))
        setStepIndex(1)
      } finally {
        setMinting(false)
      }
      return
    }
    navigate(hostsPath)
  }

  return (
    <div className="mx-auto w-full max-w-4xl p-6">
      <div className="mb-6 flex items-center gap-3">
        <Button asChild variant="ghost" size="icon" aria-label={t("common.cancel")}>
          <Link to={hostsPath}>
            <ArrowLeft className="size-4" />
          </Link>
        </Button>
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("compute.onboard.title")}</h1>
          <p className="text-muted-foreground text-sm">{t("compute.onboard.subtitle")}</p>
        </div>
      </div>

      <div className="border-border-subtle rounded-xl border shadow-sm">
        <div className="border-border-subtle border-b px-6 py-5">
          <WizardStepper steps={steps} current={stepIndex} />
        </div>

        <div className="px-6 py-6">
          {stepIndex === 0 && <StepMethod value={method} onChange={setMethod} />}
          {stepIndex === 1 && (
            <StepIdentity
              mode={namingMode}
              hostName={hostName}
              description={description}
              scopeLabel={scopeLabel}
              nameError={nameError}
              onModeChange={setNamingMode}
              onHostNameChange={(v) => {
                setHostName(v)
                setNameError(undefined)
              }}
              onDescriptionChange={setDescription}
            />
          )}
          {stepIndex === 2 && (
            <StepInstall
              command={command}
              reservedName={namingMode === "reserved" ? hostName.trim() : undefined}
              registeredHost={registeredHost}
            />
          )}
        </div>

        <div className="border-border-subtle flex items-center justify-between border-t px-6 py-4">
          <Button asChild variant="ghost">
            <Link to={hostsPath}>{t("common.cancel")}</Link>
          </Button>
          <div className="flex items-center gap-2">
            {stepIndex > 0 && !isLastStep && (
              <Button variant="outline" onClick={() => setStepIndex((i) => i - 1)}>
                {t("compute.onboard.prev")}
              </Button>
            )}
            <Button onClick={goNext} disabled={minting}>
              {isLastStep ? t("compute.onboard.done") : t("compute.onboard.next")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

// buildInstallCommand renders the one-liner an operator pastes. The
// server origin is taken from the browser, so a deployment behind a
// different external URL still produces a command that resolves.
function buildInstallCommand(token: string): string {
  const origin = window.location.origin
  return `curl -fsSL ${origin}/install-agent.sh | sh -s -- \\\n  --server ${origin} \\\n  --token ${token}`
}
