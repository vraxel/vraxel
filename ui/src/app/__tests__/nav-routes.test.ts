import { describe, expect, it } from "vitest"
import { matchRoutes } from "react-router"
import type { RouteObject } from "react-router"
import { routes } from "@/app/routes"
import { NAV_ITEMS, buildScopedPath } from "@/core/registry/nav-config"

// The failure this guards is silent, which is why it needs a test rather
// than a review habit.
//
// A nav item declares the scopes its resource appears at. The sidebar and
// the scope selector both navigate through buildScopedPath, so declaring
// "workspace" produces /{module}/workspaces/{id}/{resource}. If the
// module's routes.tsx never registered that path, the URL matches nothing
// but the `*` catch-all, which redirects to the user's default page --
// so picking a workspace appears to reset the selector back to "all"
// instead of erroring. Nothing in the type system connects the two lists.
//
// Real ids are irrelevant: the route table is matched structurally, and
// any non-empty segment exercises the same `:workspaceId` slot.
const WS = "1"
const NS = "2"

function pathsFor(resource: string, scope: string): string {
  if (scope === "namespace") return buildScopedPath(resource, WS, NS)
  if (scope === "workspace") return buildScopedPath(resource, WS, null)
  return buildScopedPath(resource, null, null)
}

// A match that only lands on the catch-all is the bug: it means no module
// route claimed the path.
function isRouted(all: RouteObject[], path: string): boolean {
  const matches = matchRoutes(all, path)
  if (!matches || matches.length === 0) return false
  return !matches.some((m) => m.route.path === "*")
}

describe("nav items are routable at every scope they declare", () => {
  it.each(NAV_ITEMS.flatMap((item) => item.scopes.map((scope) => [item.resource, scope] as const)))(
    "%s at %s scope",
    (resource, scope) => {
      const path = pathsFor(resource, scope)
      expect({ resource, scope, path, routed: isRouted(routes, path) }).toEqual({
        resource,
        scope,
        path,
        routed: true,
      })
    },
  )
})
