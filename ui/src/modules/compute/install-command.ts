/**
 * buildInstallCommand renders the one-liner an operator pastes.
 *
 * The address comes from the server (server.externalUrl), not from
 * window.location.origin. The browser's origin is where the OPERATOR
 * reached the UI; this command runs on a different machine, which needs
 * to know where to reach the SERVER. They coincide in a plain production
 * deployment and nowhere else -- a dev browser sees the vite port, a
 * tunnelled one sees localhost, an admin-VLAN one sees a name the fleet
 * cannot resolve -- and every one of those pastes into a host as a
 * command pointing the agent back at itself.
 *
 * No fallback to the origin when serverUrl is empty: externalUrl always
 * has a value (lib/config defaults it), so an empty one means something
 * is badly wrong, and a visibly broken command beats a silently wrong
 * one.
 *
 * One shape for every situation -- first onboarding, adopting an
 * imported host, recovering one whose credential the server no longer
 * honours. install-agent.sh always registers, so there is nothing for
 * the operator to choose between and no flag to get wrong.
 *
 * Its own module rather than a second export from the panel: a file that
 * exports both a component and a plain function opts out of fast refresh
 * for that component (react-refresh/only-export-components).
 */
export function buildInstallCommand(serverUrl: string, token: string): string {
  const base = serverUrl.replace(/\/+$/, "")
  return `curl -fsSL ${base}/install-agent.sh | sh -s -- \\\n  --server ${base} \\\n  --token ${token}`
}
