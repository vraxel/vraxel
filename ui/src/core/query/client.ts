import { QueryCache, QueryClient } from "@tanstack/react-query"
import { ApiError, showApiError } from "@/core/api/client"
import { translate } from "@/i18n"

// Per-query cooldown for the global error toast: a polling query
// (refetchInterval) against a down backend fails every tick, so without this
// it would spam one toast per tick. Keyed by queryHash; first failure surfaces
// immediately, repeats within the window are swallowed.
const lastErrorToastAt = new Map<string, number>()
const ERROR_TOAST_COOLDOWN_MS = 10_000

// Behavior defaults follow plan.md 1.3's decision table -- chosen to
// match the pre-refactor behavior exactly, so migrated pages do not
// change what users see:
//   staleTime 15s   -- lists feel instant on back-nav without going stale
//   retry 0         -- consistent with the ky client (see core/api/client.ts:
//                      retries against a local BFF are pure noise)
//   no focus refetch -- pages did not refetch on window focus before;
//                      status-bearing resources opt in per query
//   refetchOnMount "always" is NOT set: staleTime governs re-fetch.
export const queryClient = new QueryClient({
  // Global read-error surface. Before this, useApiQuery had no onError, so a
  // failed query rendered an empty state silently where the pre-refactor page
  // toasted in its useEffect catch (audit found this in iam users/detail,
  // ai call-logs, etc.). This restores the toast for ALL queries at once.
  // Query-only on purpose: mutations surface errors at their ~641 call sites
  // via showApiError; a MutationCache handler would double-toast the ~79
  // existing mutation onError callbacks. Skips: 404 (detail pages render their
  // own inline "not found"), 401 (the api client handles auth refresh /
  // redirect), and any query opting out via meta.skipGlobalError.
  queryCache: new QueryCache({
    onError: (error, query) => {
      if (query.meta?.skipGlobalError) return
      if (error instanceof ApiError && (error.status === 404 || error.status === 401)) return
      const now = Date.now()
      if (now - (lastErrorToastAt.get(query.queryHash) ?? 0) < ERROR_TOAST_COOLDOWN_MS) return
      lastErrorToastAt.set(query.queryHash, now)
      showApiError(error, translate, query.meta?.errorResourceKey as string | undefined)
    },
  }),
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: 0,
      refetchOnWindowFocus: false,
    },
    mutations: {
      retry: 0,
    },
  },
})

// Emergency switch (plan.md 第 3 节回滚表): flipping staleTime/gcTime to 0
// degrades the cache to always-refetch, which is exactly the pre-TQ
// behavior. Keep as a one-line change here.
