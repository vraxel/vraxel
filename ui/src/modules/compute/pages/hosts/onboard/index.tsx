import { useEffect, useMemo, useState } from "react"
import { Link, useNavigate, useParams } from "react-router"
import { ArrowLeft } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { useTranslation } from "@/i18n"
import { useWorkspaceStore } from "@/core/scope/workspace-store"
import { buildScopedPath } from "@/core/registry/nav-config"
import { WizardStepper, type WizardStep } from "@/modules/compute/components/wizard-stepper"
import { createJoinToken, pollForRegisteredHost } from "@/modules/compute/api/join-tokens"
import type { Host } from "@/modules/compute/api/types"
import { StepIdentity, type NamingMode } from "./step-identity"
import { StepInstall } from "./step-install"

// Host names follow the backend's rule (validation.go nameRegexp):
// alphanumerics, underscore and hyphen, 3-50 chars, alphanumeric at both
// ends. Checked here so a reserved name fails on this screen rather than
// on the host, minutes later, after the operator has walked to the rack.
const HOST_NAME_RE = /^[a-zA-Z0-9]([a-zA-Z0-9_-]{1,48}[a-zA-Z0-9])?$/

/**
 * Full-page host creation wizard.
 *
 * A page rather than a dialog because the flow is going to branch:
 * provisioning from a cloud pool will ask for a pool, a template, a spec,
 * networking and a confirmation. A modal that has to hold six steps ends
 * up as LCP's 1026-line host-form-dialog, where one component carries
 * several unrelated creation semantics at once.
 *
 * Today there is one way a host comes into existence, so there is no
 * method step -- a step with a single option is a click that asks
 * nothing. When a second way lands it goes back in front as step one and
 * that branch's steps append below; the shell, the stepper and the footer
 * render whatever the step list holds.
 */
export default function HostOnboardPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId, namespaceId } = useParams()
  const workspaceName = useWorkspaceStore((s) => s.currentWorkspaceName)

  const [stepIndex, setStepIndex] = useState(0)
  const [namingMode, setNamingMode] = useState<NamingMode>("auto")
  const [hostName, setHostName] = useState("")
  const [description, setDescription] = useState("")
  const [nameError, setNameError] = useState<string | undefined>()
  const [command, setCommand] = useState<string | null>(null)
  const [minting, setMinting] = useState(false)
  const [registeredHost, setRegisteredHost] = useState<Host | null>(null)

  const steps: WizardStep[] = useMemo(
    () => [
      { id: "identity", label: t("compute.onboard.step.identity") },
      { id: "install", label: t("compute.onboard.step.install") },
    ],
    [t],
  )

  // Named from what is actually to hand. The workspace name is in the
  // scope store; no namespace name is, and printing the raw id instead
  // shows the operator a number that means nothing to them. The scope
  // selector sits above this and already names the project, so the level
  // alone is enough here -- adding a namespace name to the shared store
  // would fork it from LCP for one label.
  const scopeLabel = namespaceId
    ? t("compute.onboard.scope.namespace", { workspace: workspaceName ?? workspaceId ?? "" })
    : workspaceId
      ? t("compute.onboard.scope.workspace", { name: workspaceName ?? workspaceId })
      : t("compute.onboard.scope.platform")

  const isLastStep = stepIndex === steps.length - 1
  // Back / cancel / done return to the list in the scope the wizard was
  // opened from, not to the platform one.
  const hostsPath = buildScopedPath("hosts", workspaceId ?? null, namespaceId ?? null)

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
    if (stepIndex === 0) {
      if (namingMode === "reserved" && !HOST_NAME_RE.test(hostName.trim())) {
        setNameError(t("compute.onboard.identity.nameInvalid"))
        return
      }
      setNameError(undefined)
      // Minting on the way in rather than on page load means an abandoned
      // wizard leaves no live credential behind.
      setMinting(true)
      setStepIndex(1)
      try {
        const token = await createJoinToken(
          { ws: workspaceId, ns: namespaceId },
          {
            hostName: namingMode === "reserved" ? hostName.trim() : undefined,
            name: description.trim() || undefined,
          },
        )
        setCommand(buildInstallCommand(token.spec.token ?? ""))
      } catch {
        toast.error(t("compute.onboard.install.mintFailed"))
        setStepIndex(0)
      } finally {
        setMinting(false)
      }
      return
    }
    navigate(hostsPath)
  }

  return (
    <div className="p-6">
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

        {/* The card runs the page's full width like every detail page; the
            step body caps itself so a single text field does not stretch
            across 1600px on a wide monitor. */}
        <div className="px-6 py-6">
          <div className="max-w-3xl">
            {stepIndex === 0 && (
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
            {stepIndex === 1 && (
              <StepInstall
                command={command}
                hostsPath={hostsPath}
                reservedName={namingMode === "reserved" ? hostName.trim() : undefined}
                registeredHost={registeredHost}
              />
            )}
          </div>
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
