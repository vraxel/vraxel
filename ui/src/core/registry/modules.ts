import { NAV_ITEMS } from "@/core/registry/nav-config"

/**
 * Registered frontend module prefixes: the top-level route segment each
 * module lives under ("/iam/...", "/compute/...").
 *
 * Derived from NAV_ITEMS rather than listed by hand, because a missing
 * entry fails silently. Path parsing strips the module segment before it
 * looks for "workspaces", so an unregistered prefix makes
 * /compute/workspaces/1/hosts parse as platform scope: the URL is right
 * and the page still loads scoped data from useParams, but the scope
 * store, the sidebar highlight and the breadcrumb all read platform.
 * Nothing throws. `compute` was missing in exactly that way.
 *
 * NAV_ITEMS already names each item's module, so a hand-kept set is the
 * same fact written twice. This keeps it written once.
 */
export const MODULE_PREFIXES = new Set(NAV_ITEMS.map((item) => item.module))

/** Check whether a path segment is a known module prefix. */
export function isModulePrefix(segment: string): boolean {
  return MODULE_PREFIXES.has(segment)
}
