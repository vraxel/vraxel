import { useEffect } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { connectModuleWatch, useStatusWatch } from "@/core/watch/status-watch"
import { qk } from "@/core/query/keys"
import type { ScopeRef } from "@/core/registry/resource"
import { hostsDef } from "@/modules/compute/defs"

/**
 * Coalescing window. A single operator action can produce several events
 * (a machine registering writes the host row, then binds its agent, then
 * comes online), and a swept instance produces one per host it held.
 * Refetching per event would be a burst of identical requests.
 */
const COALESCE_MS = 300

/**
 * Keep compute's host queries fresh while a page is mounted.
 *
 * Invalidating the scope prefix -- not patching a row -- is what makes
 * one mechanism cover everything the page has to react to: an agent
 * flipping online, a machine onboarding itself into the list, a host
 * being deleted from another browser. Only the first of those is a
 * change to a row that is already on screen; the other two change which
 * rows exist, which no in-place patch can express.
 *
 * The socket is refcounted per module, so a list -> detail navigation
 * reuses it instead of reconnecting.
 */
export function useHostWatch(scope: ScopeRef): void {
  const qc = useQueryClient()
  const { ws, ns } = scope

  useEffect(() => {
    const release = connectModuleWatch("compute", "hosts/watch", ws, ns)
    let timer: ReturnType<typeof setTimeout> | null = null

    const unsub = useStatusWatch.getState().subscribe((ev) => {
      if (ev.entityType !== "host") return
      // Trailing edge, not leading: the last event of a burst is the one
      // whose state the refetch should reflect.
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => {
        timer = null
        qc.invalidateQueries({ queryKey: qk.scope(hostsDef, { ws, ns }) })
      }, COALESCE_MS)
    })

    return () => {
      unsub()
      if (timer) clearTimeout(timer)
      release()
    }
  }, [ws, ns, qc])
}
