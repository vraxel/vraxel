// One correctly-typed serialization of list/query params for ky's
// searchParams, replacing the 400+ `params as unknown as Record<...>`
// casts (plan.md 1.4). Skips undefined/null/"", stringifies numbers and
// booleans, repeats array keys.
export function toSearchParams(params?: object): URLSearchParams | undefined {
  if (!params) return undefined
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === "") continue
    if (Array.isArray(v)) {
      for (const item of v) {
        if (item === undefined || item === null || item === "") continue
        sp.append(k, String(item))
      }
      continue
    }
    sp.append(k, String(v))
  }
  return sp
}
