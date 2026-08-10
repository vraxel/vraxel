/**
 * Registered frontend module prefixes.
 * Each module corresponds to a top-level route segment (e.g. "/iam/...", "/audit/...").
 * Adding a new module here makes it automatically recognised by breadcrumbs,
 * scope detection, and any other path-parsing logic.
 */
export const MODULE_PREFIXES = new Set(["iam", "audit"])

/** Check whether a path segment is a known module prefix. */
export function isModulePrefix(segment: string): boolean {
  return MODULE_PREFIXES.has(segment)
}
