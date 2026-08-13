import { Cloud, HardDrive, Terminal } from "lucide-react"
import type { LucideIcon } from "lucide-react"

// The creation-method registry for the host onboarding wizard.
//
// One entry per way a host can come into existence. Adding provisioning
// from a cloud pool means flipping `available` and supplying that
// branch's step components -- the wizard shell reads `steps` from here,
// so nothing about the page layout, the stepper or the footer changes.
//
// Unavailable methods stay visible on purpose: an operator looking for
// "create a VM on VMware" needs to find where it will be, and the flow
// preview tells them what it will ask for.

export type OnboardMethodId = "agent" | "cloud" | "ssh"

export interface OnboardMethodDef {
  id: OnboardMethodId
  icon: LucideIcon
  titleKey: string
  descKey: string
  /** i18n keys for this branch's steps, method selection excluded. */
  stepKeys: readonly string[]
  available: boolean
  /** Shown on the card of a method that is not available yet. */
  badgeKey?: string
}

export const ONBOARD_METHODS: readonly OnboardMethodDef[] = [
  {
    id: "agent",
    icon: HardDrive,
    titleKey: "compute.onboard.method.agent.title",
    descKey: "compute.onboard.method.agent.desc",
    stepKeys: ["compute.onboard.step.identity", "compute.onboard.step.install"],
    available: true,
  },
  {
    id: "cloud",
    icon: Cloud,
    titleKey: "compute.onboard.method.cloud.title",
    descKey: "compute.onboard.method.cloud.desc",
    stepKeys: [
      "compute.onboard.step.pool",
      "compute.onboard.step.template",
      "compute.onboard.step.spec",
      "compute.onboard.step.network",
      "compute.onboard.step.confirm",
    ],
    available: false,
    badgeKey: "compute.onboard.comingSoon",
  },
  {
    id: "ssh",
    icon: Terminal,
    titleKey: "compute.onboard.method.ssh.title",
    descKey: "compute.onboard.method.ssh.desc",
    stepKeys: ["compute.onboard.step.connection", "compute.onboard.step.confirm"],
    available: false,
    badgeKey: "compute.onboard.comingSoon",
  },
] as const
