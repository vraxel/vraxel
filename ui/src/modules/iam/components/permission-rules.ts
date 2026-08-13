// Pure permission-pattern logic, split out of permission-selector.tsx.
//
// It lives in its own module so that file exports components and nothing
// else: a non-component export breaks its React Fast Refresh boundary,
// and every edit that reaches it then forces the role pages to remount
// instead of hot-updating -- losing whatever the operator had half
// filled in. The two eslint-disable lines this replaces were paying that
// cost permanently.

export const VERB_GROUPS = [
  { key: "read", verbs: ["list", "get"] },
  { key: "create", verbs: ["create"] },
  { key: "update", verbs: ["update", "patch"] },
  { key: "delete", verbs: ["delete", "deleteCollection"] },
] as const

export const READ_VERBS = ["list", "get"] as const
export const WRITE_VERB_GROUP_KEYS = ["create", "update", "delete"] as const
export const WRITE_VERBS = new Set<string>(
  VERB_GROUPS.filter((g) => (WRITE_VERB_GROUP_KEYS as readonly string[]).includes(g.key)).flatMap(
    (g) => g.verbs,
  ),
)

export const SCOPE_LEVELS: Record<string, number> = {
  platform: 0,
  workspace: 1,
  namespace: 2,
}

export const STANDARD_VERBS = new Set(VERB_GROUPS.flatMap((g) => g.verbs))

export function getVerb(code: string): string {
  return code.split(":").pop() || ""
}

export function patternCovers(pattern: string, code: string): boolean {
  if (pattern === "*:*") return true
  const starIdx = pattern.indexOf("*")
  if (starIdx === -1) return pattern === code
  const prefix = pattern.slice(0, starIdx)
  const suffix = pattern.slice(starIdx + 1)
  return (
    code.startsWith(prefix) && code.endsWith(suffix) && code.length > prefix.length + suffix.length
  )
}

export function isSelected(rules: string[], code: string): boolean {
  if (rules.includes(code)) return true
  return rules.some((r) => r.includes("*") && patternCovers(r, code))
}

export function isLocked(rules: string[], code: string): boolean {
  return rules.some((r) => r !== code && r.includes("*") && patternCovers(r, code))
}

export function isCoarserOrEqual(a: string, b: string): boolean {
  if (a === b) return true
  if (!a.includes("*")) return false
  return patternCovers(a, b)
}

// Read-implies-write cascade: any write verb (create / update / patch / delete / deleteCollection)
// requires read verbs (list / get) to be useful — without read the UI lists items the role cannot fetch.
//
// The cascade is applied per-resource. A "resource bucket" is the prefix before the trailing verb
// (e.g. `pki:credentials` for `pki:credentials:create`, `*` for `*:create`). For each bucket in
// `allCodesInScope`:
//   - If a write verb in the bucket became selected, also select every read verb in that bucket.
//   - If a read verb in the bucket became unselected, also unselect every write verb in that bucket.
//
// `nextRules` is the rules list AFTER the user's primary toggle; this function only adds the
// dependent additions/removals. Codes not present in `allCodesInScope` are left untouched.
//
export function applyVerbGroupCascade(
  prevRules: string[],
  nextRules: string[],
  allCodesInScope: string[],
): string[] {
  const prev = new Set(prevRules)
  const next = new Set(nextRules)

  // Group scope codes by resource bucket.
  const bucketByCode = new Map<string, string>()
  const codesByBucket = new Map<string, string[]>()
  for (const c of allCodesInScope) {
    const idx = c.lastIndexOf(":")
    const bucket = idx === -1 ? c : c.slice(0, idx)
    bucketByCode.set(c, bucket)
    if (!codesByBucket.has(bucket)) codesByBucket.set(bucket, [])
    codesByBucket.get(bucket)!.push(c)
  }

  const writeAddedBuckets = new Set<string>()
  const readRemovedBuckets = new Set<string>()
  for (const c of allCodesInScope) {
    const verb = getVerb(c)
    const bucket = bucketByCode.get(c)!
    if (next.has(c) && !prev.has(c) && WRITE_VERBS.has(verb)) {
      writeAddedBuckets.add(bucket)
    }
    if (prev.has(c) && !next.has(c) && (READ_VERBS as readonly string[]).includes(verb)) {
      readRemovedBuckets.add(bucket)
    }
  }

  if (writeAddedBuckets.size === 0 && readRemovedBuckets.size === 0) return nextRules

  const toAdd: string[] = []
  for (const bucket of writeAddedBuckets) {
    for (const c of codesByBucket.get(bucket) ?? []) {
      if ((READ_VERBS as readonly string[]).includes(getVerb(c)) && !next.has(c)) toAdd.push(c)
    }
  }
  const toRemove = new Set<string>()
  for (const bucket of readRemovedBuckets) {
    for (const c of codesByBucket.get(bucket) ?? []) {
      if (WRITE_VERBS.has(getVerb(c))) toRemove.add(c)
    }
  }

  let result = nextRules
  if (toRemove.size > 0) result = result.filter((c) => !toRemove.has(c))
  if (toAdd.length > 0) result = [...result, ...toAdd]
  return result
}
