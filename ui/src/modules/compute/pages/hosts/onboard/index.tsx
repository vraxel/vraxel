import { useEffect, useMemo, useState } from "react"
import { Link, useNavigate, useParams } from "react-router"
import { ArrowLeft } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/shared/ui/button"
import { useTranslation } from "@/i18n"
import { useWorkspaceStore } from "@/core/scope/workspace-store"
import { buildScopedPath } from "@/core/registry/nav-config"
import { WizardStepper, type WizardStep } from "@/modules/compute/components/wizard-stepper"
import { AgentInstallPanel } from "@/modules/compute/components/agent-install-panel"
import { buildInstallCommand } from "@/modules/compute/install-command"
import { joinTokensApi } from "@/modules/compute/api/join-tokens"
import { hostsApi } from "@/modules/compute/api/hosts"
import type { Host } from "@/modules/compute/api/types"
import { StepMethod, type ImportSource, type Method } from "./step-method"
import { StepHostForm, type HostDraft } from "./step-host-form"
import { StepAgent } from "./step-agent"

// Host names follow the backend's rule (validation.go nameRegexp):
// alphanumerics, underscore and hyphen, 3-50 chars, alphanumeric at both
// ends. Checked here so a bad name fails on this screen rather than on
// submit, after the operator has filled in the rest of the form.
const HOST_NAME_RE = /^[a-zA-Z0-9]([a-zA-Z0-9_-]{1,48}[a-zA-Z0-9])?$/

// An operator is watching this screen, so the interval is short. It
// stops as soon as a host is found, and the whole effect is scoped to
// the wizard being open.
const POLL_INTERVAL_MS = 3000

const EMPTY_DRAFT: HostDraft = {
  name: "",
  description: "",
  ip: "",
  sshPort: "22",
  autoInstallAgent: true,
}

/**
 * Full-page host creation wizard.
 *
 * A page rather than a dialog because the flow branches: the import path
 * is three steps today and provisioning from a cloud pool will be six. A
 * modal that has to hold all of them ends up as LCP's 1026-line
 * host-form-dialog, where one component carries several unrelated
 * creation semantics at once.
 *
 * The two methods differ only in when the host record is written. Agent
 * onboarding writes it on registration, so its last step waits for the
 * machine to call home. Importing writes it at the end of step two, so
 * step three is already optional follow-up work -- which is why the
 * footer button reads "create host" on that step, and why the agent step
 * can be walked away from without losing anything.
 */
export default function HostOnboardPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId, namespaceId } = useParams()
  const workspaceName = useWorkspaceStore((s) => s.currentWorkspaceName)

  const [stepIndex, setStepIndex] = useState(0)
  const [method, setMethod] = useState<Method>("agent")
  const [source, setSource] = useState<ImportSource>("manual")
  const [draft, setDraft] = useState<HostDraft>(EMPTY_DRAFT)
  const [nameError, setNameError] = useState<string | undefined>()
  const [createdHost, setCreatedHost] = useState<Host | null>(null)
  const [command, setCommand] = useState<string | null>(null)
  const [tokenId, setTokenId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [registeredHost, setRegisteredHost] = useState<Host | null>(null)

  const importing = method === "import"

  const steps: WizardStep[] = useMemo(
    () =>
      importing
        ? [
            { id: "method", label: t("compute.onboard.step.method") },
            { id: "host", label: t("compute.onboard.step.host") },
            { id: "agent", label: t("compute.onboard.step.agent") },
          ]
        : [
            { id: "method", label: t("compute.onboard.step.method") },
            { id: "install", label: t("compute.onboard.step.install") },
          ],
    [importing, t],
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

  // Waiting for the machine to answer.
  //
  // The token is what gets polled, not the host list. /register stamps
  // the consumed token with the host it onboarded, so this learns both
  // "did it happen" and "which host" from one row -- the register
  // response itself goes to the agent, not to this browser. Guessing
  // from the list ("the newest host created since I minted this") would
  // show the wrong machine's facts whenever two operators onboard at
  // once.
  useEffect(() => {
    if (!tokenId || registeredHost) return
    let live = true
    const scope = { ws: workspaceId, ns: namespaceId }
    const timer = setInterval(async () => {
      try {
        const token = await joinTokensApi.get(scope, tokenId)
        // usedCount is the half that means "a machine actually answered".
        // targetHostId alone is not: a token minted against an imported
        // host carries it from birth, so keying on it made the import
        // path report the agent online before anyone had installed one.
        const hostId = token.spec.targetHostId
        if (!live || !token.spec.usedCount || !hostId) return
        const host = await hostsApi.get(scope, hostId)
        if (live) setRegisteredHost(host)
      } catch {
        // A poll that fails changes nothing: the next tick retries, and
        // the operator's real feedback is the host list either way.
      }
    }, POLL_INTERVAL_MS)
    return () => {
      live = false
      clearInterval(timer)
    }
  }, [tokenId, registeredHost, workspaceId, namespaceId])

  const mintToken = async (targetHostId?: string, name?: string) => {
    try {
      const token = await joinTokensApi.create(
        { ws: workspaceId, ns: namespaceId },
        { metadata: { name }, spec: { targetHostId } },
      )
      setCommand(buildInstallCommand(token.spec.serverUrl ?? "", token.spec.token ?? ""))
      setTokenId(token.metadata.id)
      return true
    } catch {
      toast.error(t("compute.onboard.install.mintFailed"))
      return false
    }
  }

  const goNext = async () => {
    // Step one. Importing moves on to the form and mints nothing: there
    // is no host to bind a token to yet. Agent onboarding mints on the
    // way in rather than on page load, so an abandoned wizard leaves no
    // live credential behind.
    if (stepIndex === 0) {
      if (importing) {
        setStepIndex(1)
        return
      }
      setStepIndex(1)
      setBusy(true)
      const ok = await mintToken()
      setBusy(false)
      if (!ok) setStepIndex(0)
      return
    }

    // Step two of the import path: this is where the host is written.
    if (importing && stepIndex === 1) {
      if (!HOST_NAME_RE.test(draft.name.trim())) {
        setNameError(t("compute.onboard.form.nameInvalid"))
        return
      }
      if (draft.autoInstallAgent && !draft.ip.trim()) {
        toast.error(t("compute.onboard.form.ipRequiredError"))
        return
      }
      setNameError(undefined)
      setBusy(true)
      try {
        const host = await hostsApi.create(
          { ws: workspaceId, ns: namespaceId },
          {
            metadata: { name: draft.name.trim() },
            spec: {
              description: draft.description.trim() || undefined,
              ip: draft.ip.trim() || undefined,
              sshPort: Number(draft.sshPort) || 22,
            },
          },
        )
        setCreatedHost(host)
        setStepIndex(2)
        // The SSH push needs a credential this deployment cannot hold
        // yet, so the token is minted for the manual fallback. When the
        // credential slice lands this becomes: probe, push, and mint only
        // when the probe or the push fails.
        if (draft.autoInstallAgent) await mintToken(host.metadata.id, host.metadata.name)
      } catch {
        toast.error(t("compute.onboard.form.createFailed"))
      } finally {
        setBusy(false)
      }
      return
    }

    navigate(createdHost ? `${hostsPath}/${createdHost.metadata.id}` : hostsPath)
  }

  const nextLabel =
    importing && stepIndex === 1 ? t("compute.onboard.create") : t("compute.onboard.next")

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
            {steps[stepIndex].id === "method" && (
              <StepMethod
                method={method}
                source={source}
                scopeLabel={scopeLabel}
                onMethodChange={setMethod}
                onSourceChange={setSource}
              />
            )}
            {steps[stepIndex].id === "host" && (
              <StepHostForm
                draft={draft}
                nameError={nameError}
                onChange={(patch) => {
                  setDraft((d) => ({ ...d, ...patch }))
                  if (patch.name !== undefined) setNameError(undefined)
                }}
              />
            )}
            {steps[stepIndex].id === "install" && (
              <AgentInstallPanel
                command={command}
                hostsPath={hostsPath}
                registeredHost={registeredHost}
              />
            )}
            {steps[stepIndex].id === "agent" && createdHost && (
              <StepAgent
                createdHost={createdHost}
                hostsPath={hostsPath}
                autoInstall={draft.autoInstallAgent}
                sshFailure={
                  draft.autoInstallAgent ? t("compute.onboard.agent.noCredential") : undefined
                }
                command={command}
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
            {/* No way back once the host is written or a token is handed
                out: the previous step has already had an irreversible
                effect, and a Back button that silently does not undo it
                is worse than no Back button at all. */}
            {stepIndex > 0 && !isLastStep && !createdHost && (
              <Button variant="outline" onClick={() => setStepIndex((i) => i - 1)}>
                {t("compute.onboard.prev")}
              </Button>
            )}
            <Button onClick={goNext} disabled={busy}>
              {isLastStep ? t("compute.onboard.done") : nextLabel}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
