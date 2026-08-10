import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryKey,
  type UseMutationOptions,
  type UseQueryOptions,
} from "@tanstack/react-query"

// Thin project wrappers. Deliberately minimal in W2: framework-level
// sugar (list state, URL sync, error surfaces) lands with frameworks/list
// in W3. These exist so pages import from @/core/query -- swapping or
// instrumenting the underlying library stays a one-file change.

export function useApiQuery<TData, TError = Error>(options: UseQueryOptions<TData, TError>) {
  return useQuery(options)
}

type InvalidateSpec = readonly QueryKey[]

export function useApiMutation<TData = unknown, TVariables = void, TError = Error>(
  options: UseMutationOptions<TData, TError, TVariables> & {
    /**
     * Query-key prefixes to invalidate after a successful mutation.
     * Await-ed before onSuccess resolves so dialogs that close on success
     * never reveal stale rows (plan.md 1.3 行为语义表).
     */
    invalidates?: InvalidateSpec | ((data: TData, vars: TVariables) => InvalidateSpec)
  },
) {
  const qc = useQueryClient()
  const { invalidates, onSuccess, ...rest } = options
  return useMutation<TData, TError, TVariables>({
    ...rest,
    onSuccess: async (data, vars, context, meta) => {
      if (invalidates) {
        const keys = typeof invalidates === "function" ? invalidates(data, vars) : invalidates
        await Promise.all(keys.map((queryKey) => qc.invalidateQueries({ queryKey })))
      }
      await onSuccess?.(data, vars, context, meta)
    },
  })
}
