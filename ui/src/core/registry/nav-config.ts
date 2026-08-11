import { Users, Building2, FolderKanban, Shield, ScrollText } from "lucide-react"
import type { LucideIcon } from "lucide-react"

export type ScopeLevel = "platform" | "workspace" | "namespace"

export interface NavItemConfig {
  /** URL segment: "hosts", "users", etc. */
  resource: string
  /** Module prefix: "iam", "infra", "dashboard", "audit" */
  module: string
  /** Permission code for list access */
  permission: string
  /** i18n key for nav label */
  labelKey: string
  /** Lucide icon component */
  icon: LucideIcon
  /** Nav group label key (omit for standalone items like overview) */
  group?: string
  /** Optional top-level collapsible parent: items sharing a
   * parentGroup nest under one collapsible section, with `group` forming
   * collapsible sub-sections inside it (and no `group` = a direct child). */
  parentGroup?: string
  /** Hide from the sidebar while keeping the item registered (routes,
   *  breadcrumbs, permission checks still resolve). */
  hideFromNav?: boolean
  /** Which scope levels this item appears in the sidebar */
  scopes: ScopeLevel[]
}

// Icon for a collapsible parent section (keyed by parentGroup label key).
export const PARENT_GROUP_ICON: Record<string, LucideIcon> = {}

/**
 * Single source of truth for all navigable resources.
 * Used by: sidebar nav, scope selector, permission checks, breadcrumbs.
 */
export const NAV_ITEMS: NavItemConfig[] = [
  // IAM
  {
    resource: "workspaces",
    module: "iam",
    permission: "iam:workspaces:list",
    labelKey: "nav.workspaces",
    icon: Building2,
    group: "nav.iam",
    scopes: ["platform"],
  },
  {
    resource: "namespaces",
    module: "iam",
    permission: "iam:namespaces:list",
    labelKey: "nav.namespaces",
    icon: FolderKanban,
    group: "nav.iam",
    scopes: ["platform", "workspace"],
  },
  {
    resource: "users",
    module: "iam",
    permission: "iam:users:list",
    labelKey: "nav.users",
    icon: Users,
    group: "nav.iam",
    scopes: ["platform", "workspace", "namespace"],
  },
  {
    resource: "roles",
    module: "iam",
    permission: "iam:roles:list",
    labelKey: "nav.roles",
    icon: Shield,
    group: "nav.iam",
    scopes: ["platform", "workspace", "namespace"],
  },
  // Audit
  {
    resource: "logs",
    module: "audit",
    permission: "audit:logs:list",
    labelKey: "nav.auditLogs",
    icon: ScrollText,
    group: "nav.audit",
    scopes: ["platform"],
  },
]

// --- Derived maps ---

/** resource name -> module prefix (e.g. "users" -> "iam") */
const RESOURCE_MODULE_MAP: Record<string, string> = Object.fromEntries(
  NAV_ITEMS.map((item) => [item.resource, item.module]),
)

/** resource name -> list permission code (e.g. "users" -> "iam:users:list") */
const RESOURCE_PERMISSION_MAP: Record<string, string> = Object.fromEntries(
  NAV_ITEMS.map((item) => [item.resource, item.permission]),
)

/** All known resource names, derived from NAV_ITEMS. */
export const KNOWN_RESOURCES: string[] = Object.keys(RESOURCE_PERMISSION_MAP)

/** resource name -> i18n label key (e.g. "users" -> "nav.users"), used by breadcrumbs. */
export const RESOURCE_LABEL_KEYS: Record<string, string> = Object.fromEntries(
  NAV_ITEMS.map((item) => [item.resource, item.labelKey]),
)

// --- Navigation helpers ---

/** Look up the list permission code for a URL resource segment (e.g. "users" -> "iam:users:list"). */
export function getResourcePermission(resource: string): string | undefined {
  return RESOURCE_PERMISSION_MAP[resource]
}

/** Determine the scope level from workspace/namespace IDs. */
export function getScopeLevel(wsId: string | null, nsId: string | null): ScopeLevel {
  if (wsId && nsId) return "namespace"
  if (wsId) return "workspace"
  return "platform"
}

/** Check whether a resource has a route at the given scope level. */
export function isResourceAtScope(resource: string, scopeLevel: ScopeLevel): boolean {
  const item = NAV_ITEMS.find((i) => i.resource === resource)
  return item?.scopes.includes(scopeLevel) ?? false
}

/** Extract the resource name the user is currently viewing from the URL path. */
export function detectResource(pathname: string): string | null {
  const segments = pathname.split("/").filter(Boolean)
  // Check compound (two-segment) resource names first
  for (let i = segments.length - 1; i >= 1; i--) {
    const compound = `${segments[i - 1]}/${segments[i]}`
    if (KNOWN_RESOURCES.includes(compound)) return compound
  }
  // Then check single-segment resources
  for (let i = segments.length - 1; i >= 0; i--) {
    if (KNOWN_RESOURCES.includes(segments[i])) return segments[i]
  }
  return null
}

// Landing target when no resource is known (unrecognised path, or the user
// has no permission on anything). Kept as a pair so every scope level
// resolves to a route that actually exists in routes.tsx.
const FALLBACK_MODULE_PATH = "/iam"
const FALLBACK_RESOURCE = "users"

/** The platform-scope landing path. */
export const DEFAULT_PATH = `${FALLBACK_MODULE_PATH}/${FALLBACK_RESOURCE}`

/** Build a scope-aware URL path for a given resource. Derives module from NAV_ITEMS. */
export function buildScopedPath(
  resource: string | null,
  wsId: string | null,
  nsId: string | null,
): string {
  const module = resource ? RESOURCE_MODULE_MAP[resource] : undefined
  if (!resource || !module) {
    if (wsId && nsId)
      return `${FALLBACK_MODULE_PATH}/workspaces/${wsId}/namespaces/${nsId}/${FALLBACK_RESOURCE}`
    if (wsId) return `${FALLBACK_MODULE_PATH}/workspaces/${wsId}/${FALLBACK_RESOURCE}`
    return `${FALLBACK_MODULE_PATH}/${FALLBACK_RESOURCE}`
  }
  if (wsId && nsId) return `/${module}/workspaces/${wsId}/namespaces/${nsId}/${resource}`
  if (wsId) return `/${module}/workspaces/${wsId}/${resource}`
  return `/${module}/${resource}`
}

/** Build permission scope object from URL params. */
export function buildPermScope(
  wsId?: string,
  nsId?: string,
): { workspaceId: string; namespaceId?: string } | undefined {
  if (nsId) return { workspaceId: wsId!, namespaceId: nsId }
  if (wsId) return { workspaceId: wsId }
  return undefined
}

/**
 * Dispatch an API call to the correct scope-level function based on wsId/nsId.
 * Eliminates the repetitive if/else if/else pattern across infra pages.
 */
export async function scopedApiCall<T>(
  wsId: string | undefined,
  nsId: string | undefined,
  platformFn: () => Promise<T>,
  workspaceFn: (wsId: string) => Promise<T>,
  namespaceFn: (wsId: string, nsId: string) => Promise<T>,
): Promise<T> {
  if (wsId && nsId) return namespaceFn(wsId, nsId)
  if (wsId) return workspaceFn(wsId)
  return platformFn()
}
