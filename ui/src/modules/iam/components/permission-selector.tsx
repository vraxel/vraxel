import { useState, useMemo, useCallback, memo } from "react"
import { ChevronRight, ChevronDown, Search } from "lucide-react"
import { Checkbox } from "@/shared/ui/checkbox"
import { Input } from "@/shared/ui/input"
import { useTranslation } from "@/i18n"
import type { Permission } from "@/modules/iam/api/types"

// --- Constants ---

const VERB_GROUPS = [
  { key: "read", verbs: ["list", "get"] },
  { key: "create", verbs: ["create"] },
  { key: "update", verbs: ["update", "patch"] },
  { key: "delete", verbs: ["delete", "deleteCollection"] },
] as const

const READ_VERBS = ["list", "get"] as const
const WRITE_VERB_GROUP_KEYS = ["create", "update", "delete"] as const
const WRITE_VERBS = new Set<string>(
  VERB_GROUPS.filter((g) => (WRITE_VERB_GROUP_KEYS as readonly string[]).includes(g.key)).flatMap(
    (g) => g.verbs,
  ),
)

const SCOPE_LEVELS: Record<string, number> = {
  platform: 0,
  workspace: 1,
  namespace: 2,
}

const STANDARD_VERBS = new Set(VERB_GROUPS.flatMap((g) => g.verbs))

function getVerb(code: string): string {
  return code.split(":").pop() || ""
}

// --- Types ---

interface GroupNode {
  key: string
  wildcardPattern: string
  i18nKey: string
  children: GroupNode[]
  permissions: PermNode[]
}

interface PermNode {
  code: string
  i18nKey: string
}

// --- Helpers ---

// eslint-disable-next-line react-refresh/only-export-components
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

function isSelected(rules: string[], code: string): boolean {
  if (rules.includes(code)) return true
  return rules.some((r) => r.includes("*") && patternCovers(r, code))
}

function isLocked(rules: string[], code: string): boolean {
  return rules.some((r) => r !== code && r.includes("*") && patternCovers(r, code))
}

function isCoarserOrEqual(a: string, b: string): boolean {
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
// eslint-disable-next-line react-refresh/only-export-components
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

function buildTree(perms: Permission[]): GroupNode {
  // Deduplicate by code — same code may appear at multiple scopes
  const seen = new Set<string>()
  const unique = perms.filter((p) => {
    if (seen.has(p.spec.code)) return false
    seen.add(p.spec.code)
    return true
  })

  const moduleMap = new Map<string, Map<string, Permission[]>>()

  for (const p of unique) {
    const parts = p.spec.code.split(":")
    const module = parts[0]
    const resourceKey = parts.slice(0, -1).join(":")

    if (!moduleMap.has(module)) moduleMap.set(module, new Map())
    const resourceMap = moduleMap.get(module)!
    if (!resourceMap.has(resourceKey)) resourceMap.set(resourceKey, [])
    resourceMap.get(resourceKey)!.push(p)
  }

  const root: GroupNode = {
    key: "root",
    wildcardPattern: "*:*",
    i18nKey: "perm.group.all",
    children: [],
    permissions: [],
  }

  for (const [module, resourceMap] of moduleMap) {
    const moduleNode: GroupNode = {
      key: module,
      wildcardPattern: `${module}:*`,
      i18nKey: `perm.group.${module}`,
      children: [],
      permissions: [],
    }

    const topResourceMap = new Map<
      string,
      { key: string; subResources: Map<string, Permission[]> }
    >()

    for (const [resourceKey, permsInGroup] of resourceMap) {
      const parts = resourceKey.split(":")
      const topResource = parts[1]
      if (!topResourceMap.has(topResource)) {
        topResourceMap.set(topResource, { key: topResource, subResources: new Map() })
      }
      topResourceMap.get(topResource)!.subResources.set(resourceKey, permsInGroup)
    }

    for (const [topResource, { subResources }] of topResourceMap) {
      const resourceNode: GroupNode = {
        key: `${module}:${topResource}`,
        wildcardPattern: `${module}:${topResource}:*`,
        i18nKey: `perm.group.${module}.${topResource}`,
        children: [],
        permissions: [],
      }

      for (const [resourceKey, permsInGroup] of subResources) {
        const parts = resourceKey.split(":")
        if (parts.length <= 2) {
          for (const p of permsInGroup) {
            resourceNode.permissions.push({
              code: p.spec.code,
              i18nKey: `perm.${p.spec.code}`,
            })
          }
        } else {
          const subResourceName = parts.slice(2).join(":")
          const subNode: GroupNode = {
            key: resourceKey,
            wildcardPattern: `${resourceKey}:*`,
            i18nKey: `perm.group.${module}.${topResource}.${subResourceName.replace(/:/g, ".")}`,
            children: [],
            permissions: permsInGroup.map((p) => ({
              code: p.spec.code,
              i18nKey: `perm.${p.spec.code}`,
            })),
          }
          resourceNode.children.push(subNode)
        }
      }

      moduleNode.children.push(resourceNode)
    }

    root.children.push(moduleNode)
  }

  return root
}

function getAllCodes(node: GroupNode): string[] {
  const codes: string[] = []
  for (const p of node.permissions) codes.push(p.code)
  for (const child of node.children) codes.push(...getAllCodes(child))
  return codes
}

// --- Components ---

export function PermissionSelector({
  permissions,
  value,
  onChange,
  readOnly,
  scope,
}: {
  permissions: Permission[]
  value: string[]
  onChange?: (rules: string[]) => void
  readOnly?: boolean
  scope?: "platform" | "workspace" | "namespace"
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState("")
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set(["root"]))

  const filteredPermissions = useMemo(() => {
    if (!scope || scope === "platform") return permissions
    const minLevel = SCOPE_LEVELS[scope] ?? 0
    return permissions.filter((p) => (SCOPE_LEVELS[p.spec.scope] ?? 0) >= minLevel)
  }, [permissions, scope])

  const tree = useMemo(() => buildTree(filteredPermissions), [filteredPermissions])

  // Expand root + module level whenever a new tree arrives
  // (adjust-during-render, not an effect).
  const [prevTree, setPrevTree] = useState(tree)
  if (prevTree !== tree) {
    setPrevTree(tree)
    const initial = new Set(["root"])
    for (const child of tree.children) {
      initial.add(child.key)
    }
    setExpanded(initial)
  }

  const toggleExpand = useCallback((key: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])

  const noop = useCallback(() => {}, [])

  const toggleWildcard = useCallback(
    (pattern: string, allCodes: string[]) => {
      if (!onChange) return
      if (value.includes(pattern)) {
        onChange(value.filter((r) => r !== pattern))
      } else {
        const cleaned = value.filter(
          (r) => !allCodes.includes(r) && !(r.includes("*") && isCoarserOrEqual(pattern, r)),
        )
        onChange([...cleaned, pattern])
      }
    },
    [value, onChange],
  )

  const togglePermission = useCallback(
    (code: string) => {
      if (!onChange) return
      if (value.includes(code)) {
        onChange(value.filter((r) => r !== code))
      } else {
        onChange([...value, code])
      }
    },
    [value, onChange],
  )

  const matchingCodes = useMemo(() => {
    if (!search) return null
    const lower = search.toLowerCase()
    const codes = new Set<string>()
    const verbGroupLabels = new Map<string, string>()
    for (const g of VERB_GROUPS) {
      const label = t(`perm.verbGroup.${g.key}`).toLowerCase()
      for (const v of g.verbs) verbGroupLabels.set(v, label)
    }
    for (const p of filteredPermissions) {
      const desc = t(`perm.${p.spec.code}`, { defaultValue: p.spec.description || p.spec.code })
      const groupLabel = verbGroupLabels.get(getVerb(p.spec.code)) ?? ""
      if (
        p.spec.code.toLowerCase().includes(lower) ||
        desc.toLowerCase().includes(lower) ||
        groupLabel.includes(lower)
      ) {
        codes.add(p.spec.code)
      }
    }
    const walkGroup = (node: GroupNode) => {
      const label = t(node.i18nKey, { defaultValue: node.key }).toLowerCase()
      if (label.includes(lower)) {
        for (const c of getAllCodes(node)) codes.add(c)
      }
      for (const child of node.children) walkGroup(child)
    }
    for (const child of tree.children) walkGroup(child)
    return codes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search, filteredPermissions, tree])

  if (readOnly) {
    return (
      <div className="py-1">
        <MemoTreeNode
          node={tree}
          value={value}
          onChange={noop}
          toggleWildcard={toggleWildcard}
          toggleExpand={toggleExpand}
          togglePermission={togglePermission}
          expanded={expanded}
          matchingCodes={null}
          depth={0}
          readOnly
        />
      </div>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="relative">
        <Search className="text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4" />
        <Input
          name="permission-search"
          placeholder={t("common.search")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-9 pl-9"
        />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto rounded-md border py-1">
        <MemoTreeNode
          node={tree}
          value={value}
          onChange={onChange ?? noop}
          toggleWildcard={toggleWildcard}
          toggleExpand={toggleExpand}
          togglePermission={togglePermission}
          expanded={expanded}
          matchingCodes={matchingCodes}
          depth={0}
          readOnly={readOnly}
        />
      </div>
    </div>
  )
}

function TreeNode({
  node,
  value,
  onChange,
  toggleWildcard,
  toggleExpand,
  togglePermission,
  expanded,
  matchingCodes,
  depth,
  readOnly,
}: {
  node: GroupNode
  value: string[]
  onChange: (rules: string[]) => void
  toggleWildcard: (pattern: string, allCodes: string[]) => void
  toggleExpand: (key: string) => void
  togglePermission: (code: string) => void
  expanded: Set<string>
  matchingCodes: Set<string> | null
  depth: number
  readOnly?: boolean
}) {
  const { t } = useTranslation()
  const allCodes = useMemo(() => getAllCodes(node), [node])
  const wildcardSelected =
    isSelected(value, node.wildcardPattern) || value.includes(node.wildcardPattern)
  const checked: boolean | "indeterminate" = useMemo(() => {
    if (wildcardSelected) return true
    const someSelected = allCodes.some((c) => isSelected(value, c))
    if (!someSelected) return false
    const allSelected = allCodes.every((c) => isSelected(value, c))
    return allSelected ? true : "indeterminate"
  }, [wildcardSelected, allCodes, value])
  const locked = isLocked(value, node.wildcardPattern)

  const isOpen = expanded.has(node.key)
  const isRoot = node.key === "root"

  const verbGroupData = useMemo(() => {
    return VERB_GROUPS.map((group) => {
      const codes = allCodes.filter((c) => (group.verbs as readonly string[]).includes(getVerb(c)))
      return { ...group, codes }
    }).filter((g) => g.codes.length > 0)
  }, [allCodes])

  const customPerms = useMemo(() => {
    return node.permissions.filter((p) => !(STANDARD_VERBS as Set<string>).has(getVerb(p.code)))
  }, [node.permissions])

  const hasChildren = node.children.length > 0 || verbGroupData.length > 0 || customPerms.length > 0

  const toggleVerbGroup = useCallback(
    (group: { key: string; verbs: readonly string[]; codes: string[] }) => {
      if (isRoot) {
        const patterns = group.verbs.map((v) => `*:${v}`)
        const allPatternsSelected = patterns.every((p) => value.includes(p))
        let next: string[]
        if (allPatternsSelected) {
          next = value.filter((r) => !patterns.includes(r))
        } else {
          const newPatterns = patterns.filter((p) => !value.includes(p))
          const cleaned = value.filter((r) => !group.codes.includes(r))
          next = [...cleaned, ...newPatterns]
        }
        // Root cascade operates on wildcard codes (*:list, *:create, ...).
        // Reuse applyVerbGroupCascade by treating the wildcard list as the scope.
        const scope = VERB_GROUPS.flatMap((g) => g.verbs).map((v) => `*:${v}`)
        next = applyVerbGroupCascade(value, next, scope)
        onChange(next)
      } else {
        const allSelected = group.codes.every((c) => isSelected(value, c))
        let next: string[]
        if (allSelected) {
          next = value.filter((r) => !group.codes.includes(r))
        } else {
          const toAdd = group.codes.filter((c) => !isSelected(value, c))
          next = [...value, ...toAdd]
        }
        next = applyVerbGroupCascade(value, next, allCodes)
        onChange(next)
      }
    },

    [isRoot, value, onChange, allCodes],
  )

  const hasMatch = matchingCodes === null || allCodes.some((c) => matchingCodes.has(c))
  if (!hasMatch) return null

  return (
    <div>
      {/* Group header */}
      <div
        className={`flex items-center gap-2 rounded px-2 py-1 ${readOnly ? "" : "hover:bg-accent cursor-pointer"}`}
        style={{ paddingLeft: `${depth * 16 + 8}px` }}
      >
        <button
          type="button"
          className="flex h-4 w-4 shrink-0 items-center justify-center"
          onClick={() => toggleExpand(node.key)}
        >
          {hasChildren ? (
            isOpen ? (
              <ChevronDown className="h-3.5 w-3.5" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5" />
            )
          ) : (
            <span className="w-3.5" />
          )}
        </button>
        <Checkbox
          name={`perm-node-${node.key}`}
          checked={checked}
          disabled={readOnly || locked}
          onCheckedChange={
            readOnly ? undefined : () => toggleWildcard(node.wildcardPattern, allCodes)
          }
        />
        <span className="text-sm font-medium">{t(node.i18nKey, { defaultValue: node.key })}</span>
        {!readOnly && (
          <span className="text-muted-foreground font-mono text-xs">{node.wildcardPattern}</span>
        )}
      </div>

      {/* Expanded content */}
      {isOpen && (
        <>
          {(verbGroupData.length > 0 || customPerms.length > 0) && (
            <div
              className="flex flex-wrap gap-x-1 gap-y-0.5 py-0.5"
              style={{ paddingLeft: `${(depth + 1) * 16 + 28}px`, paddingRight: 8 }}
            >
              {verbGroupData.map((group) => {
                let groupChecked: boolean | "indeterminate"
                let groupLocked: boolean

                if (isRoot) {
                  const patterns = group.verbs.map((v) => `*:${v}`)
                  const allPatternsSelected = patterns.every((p) => value.includes(p))
                  const somePatternsSelected = patterns.some((p) => value.includes(p))
                  groupChecked = allPatternsSelected || value.includes("*:*")
                  if (
                    !groupChecked &&
                    (somePatternsSelected || group.codes.some((c) => isSelected(value, c)))
                  ) {
                    groupChecked = "indeterminate"
                  }
                  groupLocked = value.includes("*:*")
                } else {
                  const allSelected = group.codes.every((c) => isSelected(value, c))
                  const someSelected = group.codes.some((c) => isSelected(value, c))
                  groupChecked = allSelected
                  if (!allSelected && someSelected) groupChecked = "indeterminate"
                  groupLocked = group.codes.every((c) => isLocked(value, c))
                }

                return (
                  <div
                    key={group.key}
                    className={`bg-muted/50 flex items-center gap-1 rounded px-2 py-0.5 text-sm select-none ${readOnly ? "" : "hover:bg-accent cursor-pointer"}`}
                    title={
                      readOnly
                        ? undefined
                        : isRoot
                          ? group.verbs.map((v) => `*:${v}`).join(", ")
                          : group.codes.join(", ")
                    }
                  >
                    <Checkbox
                      name={`perm-group-${node.key}-${group.key}`}
                      className="h-3.5 w-3.5"
                      checked={groupChecked}
                      disabled={readOnly || groupLocked}
                      onCheckedChange={readOnly ? undefined : () => toggleVerbGroup(group)}
                    />
                    <span
                      className="font-medium whitespace-nowrap"
                      onClick={readOnly || groupLocked ? undefined : () => toggleVerbGroup(group)}
                    >
                      {t(`perm.verbGroup.${group.key}`)}
                    </span>
                  </div>
                )
              })}

              {customPerms.map((perm) => {
                const show = matchingCodes === null || matchingCodes.has(perm.code)
                if (!show) return null
                const permChecked = isSelected(value, perm.code)
                const permLocked = isLocked(value, perm.code)
                const desc = t(perm.i18nKey, { defaultValue: perm.code })
                return (
                  <div
                    key={perm.code}
                    className={`flex items-center gap-1 rounded px-1.5 py-0.5 text-sm select-none ${readOnly ? "" : "hover:bg-accent cursor-pointer"}`}
                    title={readOnly ? undefined : perm.code}
                  >
                    <Checkbox
                      name={`perm-code-${perm.code}`}
                      className="h-3.5 w-3.5"
                      checked={permChecked}
                      disabled={readOnly || permLocked}
                      onCheckedChange={readOnly ? undefined : () => togglePermission(perm.code)}
                    />
                    <span
                      className="text-muted-foreground whitespace-nowrap"
                      onClick={
                        readOnly || permLocked ? undefined : () => togglePermission(perm.code)
                      }
                    >
                      {desc}
                    </span>
                  </div>
                )
              })}
            </div>
          )}

          {node.children.map((child) => (
            <MemoTreeNode
              key={child.key}
              node={child}
              value={value}
              onChange={onChange}
              toggleWildcard={toggleWildcard}
              toggleExpand={toggleExpand}
              togglePermission={togglePermission}
              expanded={expanded}
              matchingCodes={matchingCodes}
              depth={depth + 1}
              readOnly={readOnly}
            />
          ))}
        </>
      )}
    </div>
  )
}

const MemoTreeNode = memo(TreeNode)
