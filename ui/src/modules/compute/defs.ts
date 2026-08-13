import { defineResource } from "@/core/registry/resource"

// Registry declarations for the compute module.
//
// Hosts exist at all three scopes: the backend registers them at
// platform, under a workspace and under a namespace, and a host's scope
// is fixed at registration by the join token that produced it.
//
// detailParam is hostId; the onboarding wizard is NOT derived from this
// def -- wizards stay explicit per module (see core/registry/resource.ts).
export const hostsDef = defineResource({
  module: "compute",
  name: "hosts",
  scopes: ["platform", "workspace", "namespace"],
  detailParam: "hostId",
})

// Join tokens have no list page of their own: they surface as a drawer on
// the host list. Declared here for query keys and permission derivation.
export const agentJoinTokensDef = defineResource({
  module: "compute",
  name: "agent-join-tokens",
  scopes: ["platform", "workspace", "namespace"],
})
