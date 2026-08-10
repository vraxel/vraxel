import type { ComponentType } from "react"
import type { RouteObject } from "react-router"
import { createElement } from "react"

// The frontend counterpart of the backend's install.go resource
// declaration (plan.md 1.2). One defineResource() call derives, for the
// standard shape only: the three-scope routes, URL paths, permission
// codes and query keys. Non-standard routes (kube's meta-router, cicd's
// tab layouts, wizards, full-screen editors) stay explicit per module
// and are merged by the router assembly -- do not force them in here.

export type Scope = "platform" | "workspace" | "namespace"

// Scope reference carried by every API call; defaults come from the
// route params (useScopeRef) but callers may always pass one explicitly
// (cross-scope calls are legitimate).
export interface ScopeRef {
  ws?: string
  ns?: string
}

export interface ResourceDef {
  /** Backend GroupName, e.g. "pki". */
  module: string
  /** Backend Resource.Name: plural kebab-case, never module-prefixed. */
  name: string
  scopes: readonly Scope[]
  /**
   * Route param for the detail segment, e.g. "credentialId" -- must match
   * what the existing detail pages read from useParams().
   */
  detailParam?: string
  pages?: {
    List?: ComponentType
    Detail?: ComponentType
    /** Extra child routes mounted under the detail path (rare). */
    detailChildren?: RouteObject[]
  }
}

export function defineResource(def: ResourceDef): ResourceDef {
  return def
}

const SCOPE_PREFIX: Record<Scope, (s: ScopeRef) => string> = {
  platform: () => "",
  workspace: (s) => `workspaces/${s.ws}/`,
  namespace: (s) => `workspaces/${s.ws}/namespaces/${s.ns}/`,
}

export function scopeOf(s: ScopeRef): Scope {
  if (s.ws && s.ns) return "namespace"
  if (s.ws) return "workspace"
  return "platform"
}

/**
 * API path for a resource collection/item under a scope, relative to the
 * shared ky prefix "/api". Segments append as path parts; a trailing
 * { verb } produces the backend CustomVerb colon form `:{verb}`.
 */
export function resourcePath(
  def: ResourceDef,
  s: ScopeRef,
  ...segments: (string | number | { verb: string })[]
): string {
  let p = `${def.module}/v1/${SCOPE_PREFIX[scopeOf(s)](s)}${def.name}`
  for (const seg of segments) {
    if (typeof seg === "object") {
      p += `:${seg.verb}`
    } else {
      p += `/${seg}`
    }
  }
  return p
}

export type Verb = "list" | "get" | "create" | "update" | "patch" | "delete" | "deleteCollection"

export function perm(def: ResourceDef, verb: Verb | (string & {})): string {
  return `${def.module}:${def.name}:${verb}`
}

/** Frontend route path prefix for a scope (no leading slash). */
function routeScopePrefix(scope: Scope): string {
  if (scope === "workspace") return "workspaces/:workspaceId/"
  if (scope === "namespace") return "workspaces/:workspaceId/namespaces/:namespaceId/"
  return ""
}

/**
 * Standard three-scope list/detail routes for a set of resources,
 * mounted under the module's top-level path by the router assembly.
 */
export function buildResourceRoutes(defs: readonly ResourceDef[]): RouteObject[] {
  const routes: RouteObject[] = []
  for (const def of defs) {
    for (const scope of def.scopes) {
      const prefix = routeScopePrefix(scope)
      if (def.pages?.List) {
        routes.push({ path: `${prefix}${def.name}`, element: createElement(def.pages.List) })
      }
      if (def.pages?.Detail) {
        const param = def.detailParam ?? "id"
        const detail: RouteObject = {
          path: `${prefix}${def.name}/:${param}`,
          element: createElement(def.pages.Detail),
        }
        if (def.pages.detailChildren?.length) detail.children = def.pages.detailChildren
        routes.push(detail)
      }
    }
  }
  return routes
}

/** Frontend navigation path to a resource list/detail under a scope. */
export function resourceHref(def: ResourceDef, s: ScopeRef, id?: string | number): string {
  const scope = scopeOf(s)
  const prefix =
    scope === "namespace"
      ? `workspaces/${s.ws}/namespaces/${s.ns}/`
      : scope === "workspace"
        ? `workspaces/${s.ws}/`
        : ""
  return `/${def.module}/${prefix}${def.name}${id != null ? `/${id}` : ""}`
}
