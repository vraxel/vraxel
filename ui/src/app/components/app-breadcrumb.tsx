import React from "react"
import { Link, useLocation } from "react-router"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/shared/ui/breadcrumb"
import { useTranslation } from "@/i18n"
import { useWorkspaceStore } from "@/core/scope/workspace-store"
import { isModulePrefix } from "@/core/registry/modules"
import { RESOURCE_LABEL_KEYS } from "@/core/registry/nav-config"

interface BreadcrumbEntry {
  label: string
  href?: string
}

/** Label keys for sub-resources not in NAV_ITEMS (nested resource paths embedded
 *  in a parent detail page). These link to the parent path, not their own. */
const SUB_RESOURCE_LABEL_KEYS: Record<string, string> = {}

/** Label keys for intermediate path segments that form compound resource paths.
 *  Unlike sub-resources, these link to their own path normally. */
const INTERMEDIATE_LABEL_KEYS: Record<string, string> = {}

/** Resolve a path segment to its i18n label key. Module prefixes use `nav.{name}` convention. */
function segmentLabelKey(seg: string): string | undefined {
  if (isModulePrefix(seg)) return `nav.${seg}`
  return RESOURCE_LABEL_KEYS[seg] ?? INTERMEDIATE_LABEL_KEYS[seg] ?? SUB_RESOURCE_LABEL_KEYS[seg]
}

export function AppBreadcrumb() {
  const { t } = useTranslation()
  const location = useLocation()
  const workspaceName = useWorkspaceStore((s) => s.currentWorkspaceName)

  const allSegments = location.pathname.split("/").filter(Boolean)

  // Don't render breadcrumb on root / index
  if (allSegments.length === 0) return null

  // Skip module prefix (e.g. "iam", "dashboard")
  const hasModule = isModulePrefix(allSegments[0])
  const rawSegments = hasModule ? allSegments.slice(1) : allSegments
  const modulePrefix = hasModule ? `/${allSegments[0]}` : ""

  if (rawSegments.length === 0) return null

  // For scoped routes, strip the scope prefix from breadcrumb display
  // but preserve it in link hrefs so navigation stays within scope.
  // e.g. /iam/workspaces/4/namespaces/4/roles/35
  //   → display: Roles > 35
  //   → hrefs:   /iam/workspaces/4/namespaces/4/roles, (current page)
  let segments = rawSegments
  let scopePrefix = modulePrefix
  if (rawSegments[0] === "workspaces" && rawSegments[1]) {
    // /iam/workspaces/:id/namespaces/:nsId/... → strip first 4, show from sub-resource
    if (rawSegments[2] === "namespaces" && rawSegments[3] && rawSegments.length > 4) {
      scopePrefix = `${modulePrefix}/${rawSegments.slice(0, 4).join("/")}`
      segments = rawSegments.slice(4)
    }
    // /iam/workspaces/:id/... → strip first 2, show from resource onward
    else if (rawSegments.length > 2) {
      scopePrefix = `${modulePrefix}/${rawSegments.slice(0, 2).join("/")}`
      segments = rawSegments.slice(2)
    }
  }

  const items: BreadcrumbEntry[] = []
  let pathAccum = scopePrefix

  for (let i = 0; i < segments.length; i++) {
    const seg = segments[i]

    // Compound resource segment: a resource may register a two-segment name
    // (e.g. "mysql/instances"); collapse the pair into one breadcrumb item so
    // the intermediate segment does not leak in unlocalized.
    const next = segments[i + 1]
    const compoundKey = next ? RESOURCE_LABEL_KEYS[`${seg}/${next}`] : undefined
    if (compoundKey) {
      pathAccum += `/${seg}/${next}`
      i++
      items.push({ label: t(compoundKey), href: i === segments.length - 1 ? undefined : pathAccum })
      continue
    }

    const parentPath = pathAccum
    pathAccum += "/" + seg
    const isLast = i === segments.length - 1

    const labelKey = segmentLabelKey(seg)
    if (labelKey) {
      // Sub-resources (e.g. "subnets") are embedded in the parent detail page,
      // so link to the parent path instead of the non-existent standalone path.
      const isSubResource = seg in SUB_RESOURCE_LABEL_KEYS
      const href = isLast ? undefined : isSubResource ? parentPath : pathAccum
      items.push({ label: t(labelKey), href })
    } else {
      // Dynamic segment (e.g. workspace ID, namespace ID, resource ID)
      const parentSeg = segments[i - 1]
      const displayLabel = parentSeg === "workspaces" ? (workspaceName ?? seg) : seg
      items.push({
        label: displayLabel,
        href: isLast ? undefined : pathAccum,
      })
    }
  }

  if (items.length === 0) return null

  return (
    <Breadcrumb>
      <BreadcrumbList>
        {items.map((item, i) => {
          const isLast = i === items.length - 1
          return (
            <React.Fragment key={i}>
              {i > 0 && <BreadcrumbSeparator />}
              <BreadcrumbItem className="max-w-[200px]">
                {isLast ? (
                  <BreadcrumbPage className="truncate">{item.label}</BreadcrumbPage>
                ) : (
                  <BreadcrumbLink asChild>
                    <Link to={item.href!} className="block truncate">
                      {item.label}
                    </Link>
                  </BreadcrumbLink>
                )}
              </BreadcrumbItem>
            </React.Fragment>
          )
        })}
      </BreadcrumbList>
    </Breadcrumb>
  )
}
