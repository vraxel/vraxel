import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { Link, Navigate, Outlet, useLocation, useNavigate } from "react-router"
import { LayoutDashboard, FileText, ChevronRight, Search } from "lucide-react"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible"
import { Input } from "@/shared/ui/input"
import { cn } from "@/shared/lib/utils"
import { TooltipProvider } from "@/shared/ui/tooltip"
import { LanguageSwitcher } from "@/shared/components/language-switcher"
import { UserMenu } from "@/app/components/user-menu"
import { AppBreadcrumb } from "@/app/components/app-breadcrumb"
import { ScopeSelector } from "@/app/components/scope-selector"
import { useTranslation } from "@/i18n"
import { startAuthFlow } from "@/core/auth/auth"
import { useAuthStore } from "@/core/auth/auth-store"
import { usePermissionStore } from "@/core/permission/permission-store"
import {
  usePermission,
  getDefaultPath,
  getFirstPermittedPath,
} from "@/core/permission/use-permission"
import { useScopeStore } from "@/core/scope/scope-store"
import { isModulePrefix } from "@/core/registry/modules"
import {
  NAV_ITEMS,
  buildScopedPath,
  buildPermScope,
  DEFAULT_PATH,
} from "@/core/registry/nav-config"
import type { ScopeLevel } from "@/core/registry/nav-config"
import { pinyinInitials } from "@/core/registry/pinyin"
import { translateTo } from "@/i18n"
import { useMenuSearch, type MenuSearchItem } from "@/app/hooks/use-menu-search"

interface NavItem {
  to: string
  labelKey: string
  icon: React.ComponentType<{ className?: string }>
  permission?: string
  permissionScope?: { workspaceId?: string; namespaceId?: string }
}

interface NavGroup {
  labelKey?: string
  items: NavItem[]
}

// A collapsible top-level section (e.g. Kubernetes): direct child links plus
// collapsible sub-sections. Only used when items declare a parentGroup.
interface NavParent {
  parentLabelKey: string
  directItems: NavItem[]
  subGroups: NavGroup[]
}

type NavEntry = NavGroup | NavParent
const isParent = (e: NavEntry): e is NavParent => "parentLabelKey" in e

// Focus shortcut hint: macOS uses Cmd, everything else Ctrl. The key handler
// accepts both (ctrlKey || metaKey); only the displayed label is platform-aware.
// iOS UAs contain "like Mac OS X" (and iPadOS desktop mode reports
// "Macintosh" with a touch screen), so bare /Mac/ would hint ⌘K on devices
// without a Cmd key — exclude them.
const IS_MAC =
  /Mac/i.test(navigator.userAgent) &&
  !/iPhone|iPad|iPod/i.test(navigator.userAgent) &&
  navigator.maxTouchPoints <= 1
const SEARCH_SHORTCUT = IS_MAC ? "⌘K" : "Ctrl+K"

function buildNavGroups(wsId: string | null, nsId: string | null): NavEntry[] {
  const scopeLevel: ScopeLevel = wsId && nsId ? "namespace" : wsId ? "workspace" : "platform"
  const scope = buildPermScope(wsId ?? undefined, nsId ?? undefined)

  const entries: NavEntry[] = []
  let currentGroup: NavGroup | null = null

  for (const item of NAV_ITEMS) {
    if (!item.scopes.includes(scopeLevel)) continue
    if (item.hideFromNav) continue

    const navItem: NavItem = {
      to: buildScopedPath(item.resource, wsId, nsId),
      labelKey: item.labelKey,
      icon: item.icon,
      permission: item.permission,
      permissionScope: scope,
    }

    if (item.parentGroup) {
      // Nest under a collapsible parent; `group` (if any) is a sub-section.
      currentGroup = null
      let parent = entries.find(
        (e): e is NavParent => isParent(e) && e.parentLabelKey === item.parentGroup,
      )
      if (!parent) {
        parent = { parentLabelKey: item.parentGroup, directItems: [], subGroups: [] }
        entries.push(parent)
      }
      if (!item.group) {
        parent.directItems.push(navItem)
      } else {
        let sub = parent.subGroups.find((g) => g.labelKey === item.group)
        if (!sub) {
          sub = { labelKey: item.group, items: [] }
          parent.subGroups.push(sub)
        }
        sub.items.push(navItem)
      }
    } else if (!item.group) {
      // Standalone item (e.g. overview) — its own group
      entries.push({ items: [navItem] })
      currentGroup = null
    } else if (currentGroup != null && currentGroup.labelKey === item.group) {
      currentGroup.items.push(navItem)
    } else {
      currentGroup = { labelKey: item.group, items: [navItem] }
      entries.push(currentGroup)
    }
  }

  return entries
}

// Searchable tokens for a nav label: both locale strings + pinyin initials of
// the Chinese label, so "主机" / "host" / "zj" all match the same item.
function buildSearchTokens(labelKey: string): string[] {
  const zh = translateTo("zh-CN", labelKey)
  const en = translateTo("en-US", labelKey)
  const toks = new Set<string>()
  if (zh) toks.add(zh.toLowerCase())
  if (en) toks.add(en.toLowerCase())
  const py = pinyinInitials(zh)
  if (py) toks.add(py)
  return [...toks]
}

// Flatten the rendered nav tree into visible items in visual (top-to-bottom)
// order so search navigation matches what the user sees.
function flattenVisibleItems(entries: NavEntry[], visible: (i: NavItem) => boolean): NavItem[] {
  const out: NavItem[] = []
  for (const e of entries) {
    if (isParent(e)) {
      for (const it of e.directItems) if (visible(it)) out.push(it)
      for (const s of e.subGroups) for (const it of s.items) if (visible(it)) out.push(it)
    } else {
      for (const it of e.items) if (visible(it)) out.push(it)
    }
  }
  return out
}

// Wrap the matched fragment of a label in <mark>. When the match came from an
// alias/pinyin token not present in the visible text, mark the whole label.
function highlightLabel(label: string, query: string): React.ReactNode {
  const q = query.trim().toLowerCase()
  if (!q) return label
  const markCls = "rounded-sm bg-warning/25 text-inherit"
  const idx = label.toLowerCase().indexOf(q)
  if (idx === -1) return <mark className={markCls}>{label}</mark>
  return (
    <>
      {label.slice(0, idx)}
      <mark className={markCls}>{label.slice(idx, idx + q.length)}</mark>
      {label.slice(idx + q.length)}
    </>
  )
}

export default function RootLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { t } = useTranslation()
  const fetchUser = useAuthStore((s) => s.fetchUser)
  const fetchPermissions = usePermissionStore((s) => s.fetchPermissions)
  const { hasPermission } = usePermission()
  const scopeWorkspaceId = useScopeStore((s) => s.workspaceId)
  const scopeNamespaceId = useScopeStore((s) => s.namespaceId)
  const setScope = useScopeStore((s) => s.setScope)

  // Sync scope store from URL when navigating via links or browser back/forward.
  // Uses useLayoutEffect so the scope is updated BEFORE any child useEffect fires
  // (e.g., list pages that fetch data based on scopeWorkspaceId).
  // /iam/workspaces/:id is a platform-level detail page — scope stays null.
  // /iam/workspaces/:id/<sub-resource> activates workspace scope.
  // /iam/workspaces/:id/namespaces/:nsId/<sub-resource> activates namespace scope.
  useLayoutEffect(() => {
    const segs = location.pathname.split("/").filter(Boolean)
    // Skip module prefix (e.g. "iam", "dashboard")
    const s = isModulePrefix(segs[0]) ? segs.slice(1) : segs
    let urlWsId: string | null = null
    let urlNsId: string | null = null

    if (s[0] === "workspaces" && s[1] && s.length > 2) {
      urlWsId = s[1]
      if (s[2] === "namespaces" && s[3] && s.length > 4) {
        urlNsId = s[3]
      }
    }

    if (urlWsId !== scopeWorkspaceId || urlNsId !== scopeNamespaceId) {
      setScope(urlWsId, urlNsId)
    }
  }, [location.pathname]) // eslint-disable-line react-hooks/exhaustive-deps
  const navGroups = useMemo(
    () => buildNavGroups(scopeWorkspaceId, scopeNamespaceId),
    [scopeWorkspaceId, scopeNamespaceId],
  )
  const permissions = usePermissionStore((s) => s.permissions)

  const navItemVisible = (item: NavItem) => {
    if (item.permission && !hasPermission(item.permission, item.permissionScope)) return false
    return true
  }
  const navItemActive = (to: string) => location.pathname.startsWith(to)

  // --- Quick search (browser-Ctrl+F-style) over the sidebar menu ---
  // Rebuilt every render (cheap; ~50 items). Not memoized on navGroups: the
  // first render happens with permissions still null, so navItemVisible filters
  // everything out; caching that empty snapshot (navGroups identity is stable
  // across the permission load) would leave search permanently matchless.
  const searchItems: MenuSearchItem[] = flattenVisibleItems(navGroups, navItemVisible).map(
    (it) => ({
      key: it.to,
      tokens: buildSearchTokens(it.labelKey),
    }),
  )
  const { query, setQuery, matchSet, currentKey, currentIndex, total, next, prev, clear } =
    useMenuSearch(searchItems)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const linkRefs = useRef(new Map<string, HTMLAnchorElement>())
  // Sub-groups (Kubernetes) collapse independently; force them open while a
  // search is active so no match is hidden, and remember manual toggles.
  const [openSubs, setOpenSubs] = useState<Record<string, boolean>>({})
  const searching = query.trim().length > 0

  // Scroll the current match into view as the user types / steps through hits.
  useEffect(() => {
    if (!currentKey) return
    linkRefs.current.get(currentKey)?.scrollIntoView({ block: "nearest" })
  }, [currentKey])

  // Ctrl/Cmd+K focuses the search box from anywhere in the app.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && (e.key === "k" || e.key === "K")) {
        e.preventDefault()
        searchInputRef.current?.focus()
        searchInputRef.current?.select()
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])

  const onSearchKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown") {
      e.preventDefault()
      next()
    } else if (e.key === "ArrowUp") {
      e.preventDefault()
      prev()
    } else if (e.key === "Enter") {
      e.preventDefault()
      if (currentKey) navigate(currentKey)
    } else if (e.key === "Escape") {
      e.preventDefault()
      clear()
      searchInputRef.current?.blur()
    }
  }

  // Track whether this is the initial mount (or a hard refresh) so we only
  // auto-scroll once; subsequent navigations are user-initiated and the
  // clicked item is already visible.
  const didInitialScroll = useRef(false)
  const renderNavLink = (item: NavItem) => {
    const active = navItemActive(item.to)
    const isCurrent = item.to === currentKey
    const isMatch = matchSet.has(item.to)
    return (
      <Link
        key={item.to}
        to={item.to}
        ref={(el) => {
          const m = linkRefs.current
          if (el) m.set(item.to, el)
          else m.delete(item.to)
          if (el && active && !didInitialScroll.current) {
            didInitialScroll.current = true
            el.scrollIntoView({ block: "nearest" })
          }
        }}
        className={cn(
          // before:* draws the left brand rail on the active item; it is the
          // primary "you are here" signal, the tinted background only supports it.
          "relative flex items-center gap-2.5 rounded-md px-3 py-2 text-sm transition-colors",
          "before:bg-primary before:absolute before:inset-y-1.5 before:left-0 before:w-0.5 before:rounded-full before:opacity-0",
          active
            ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium before:opacity-100"
            : "text-muted-foreground hover:bg-accent hover:text-foreground",
          isCurrent && "ring-primary ring-2 ring-inset",
        )}
      >
        <item.icon className={cn("h-4 w-4 shrink-0", active ? "text-primary" : "opacity-70")} />
        {isMatch ? highlightLabel(t(item.labelKey), query) : t(item.labelKey)}
      </Link>
    )
  }
  // Console icon must land on a route INSIDE the currently-selected scope so
  // the user is not silently teleported to a different project that happens
  // to grant a higher-priority permission (e.g. dashboard:overview:list).
  // Falls back to getDefaultPath only at the platform scope, where the
  // global default is correct.
  const homePath = useMemo(() => {
    if (!permissions) return DEFAULT_PATH
    if (scopeWorkspaceId) {
      return getFirstPermittedPath(permissions, scopeWorkspaceId, scopeNamespaceId)
    }
    return getDefaultPath(permissions)
  }, [permissions, scopeWorkspaceId, scopeNamespaceId])

  const [ready, setReady] = useState(false)

  useEffect(() => {
    ;(async () => {
      await fetchUser()
      const u = useAuthStore.getState().user
      if (!u) {
        // No valid session cookie -- start the OIDC login flow.
        startAuthFlow()
        return
      }
      if (u.sub) {
        await fetchPermissions(u.sub)
      }
      setReady(true)
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (!ready) {
    return null
  }

  // Logout / 401-refresh clears identity (permissions -> null) and then kicks a
  // full-page redirect to /oidc/authorize. That redirect is async (startAuthFlow
  // awaits sha256), so a React re-render lands here first. Without this guard we
  // flash /error?status=403 for a frame before the browser navigates away. The
  // oidc_flow_pending marker (set synchronously by startAuthFlow before its
  // await) means a handshake is in flight, so render nothing and let the
  // imminent navigation take over.
  if (!permissions && sessionStorage.getItem("oidc_flow_pending")) {
    return null
  }

  // Redirect to 403 if user has zero permissions (or fetchPermissions was never called)
  if (!permissions) {
    return <Navigate to="/error?status=403" replace />
  }
  const hasAny =
    permissions.isPlatformAdmin ||
    (permissions.platform?.length ?? 0) > 0 ||
    Object.keys(permissions.workspaces ?? {}).length > 0 ||
    Object.keys(permissions.namespaces ?? {}).length > 0
  if (!hasAny) {
    return <Navigate to="/error?status=403" replace />
  }

  return (
    <TooltipProvider>
      <div className="fixed inset-0 flex">
        <aside className="bg-sidebar text-sidebar-foreground flex w-60 shrink-0 flex-col overflow-hidden border-r">
          <div className="flex h-14 items-center border-b px-4">
            <Link to={homePath} className="flex items-center gap-2.5 font-semibold tracking-tight">
              <span className="bg-primary text-primary-foreground flex size-7 items-center justify-center rounded-md">
                <LayoutDashboard className="h-4 w-4" />
              </span>
              <span>Vraxel Console</span>
            </Link>
          </div>
          {/* px-2 + the selector's own px-1 + the trigger's px-2 puts the
              scope labels on the same 20px text inset as the nav links. */}
          <div className="border-b px-2 py-1.5">
            <ScopeSelector />
          </div>
          <div className="border-b px-2 py-1.5">
            <div className="relative">
              <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2" />
              <Input
                ref={searchInputRef}
                name="menu-search"
                autoComplete="off"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={onSearchKeyDown}
                placeholder={t("nav.searchPlaceholder", { key: SEARCH_SHORTCUT })}
                aria-label={t("nav.searchPlaceholder", { key: SEARCH_SHORTCUT })}
                className="h-8 rounded-md pr-14 pl-8"
              />
              {searching && (
                <span className="text-muted-foreground pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-xs tabular-nums">
                  {total > 0 ? `${currentIndex + 1}/${total}` : t("nav.searchNoMatch")}
                </span>
              )}
            </div>
          </div>
          <nav className="min-h-0 flex-1 space-y-3 overflow-y-auto p-2">
            {navGroups.map((group, gi) => {
              if (isParent(group)) {
                const directVis = group.directItems.filter(navItemVisible)
                const subsVis = group.subGroups
                  .map((s) => ({ labelKey: s.labelKey, items: s.items.filter(navItemVisible) }))
                  .filter((s) => s.items.length > 0)
                if (directVis.length === 0 && subsVis.length === 0) return null
                // The parent renders as a plain section header like every other
                // top-level group -- no icon, not collapsible. Only its
                // sub-groups collapse.
                return (
                  <div key={group.parentLabelKey}>
                    <div className="text-muted-foreground/80 px-3 pt-3 pb-1.5 text-xs font-semibold tracking-wider uppercase">
                      {t(group.parentLabelKey)}
                    </div>
                    <div className="space-y-0.5">
                      {directVis.map(renderNavLink)}
                      {subsVis.map((sub) => {
                        const subActive = sub.items.some((i) => navItemActive(i.to))
                        const subKey = sub.labelKey ?? ""
                        const defaultOpen = subActive
                        // Force open during search so matches inside a collapsed
                        // sub-group are visible; otherwise honor manual toggles.
                        const isOpen = searching ? true : (openSubs[subKey] ?? defaultOpen)
                        return (
                          <Collapsible
                            key={sub.labelKey}
                            open={isOpen}
                            onOpenChange={(o) => setOpenSubs((p) => ({ ...p, [subKey]: o }))}
                          >
                            <CollapsibleTrigger className="group text-foreground/75 hover:bg-accent hover:text-foreground flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors">
                              <ChevronRight className="h-4 w-4 transition-transform group-data-[state=open]:rotate-90" />
                              {sub.labelKey ? t(sub.labelKey) : ""}
                            </CollapsibleTrigger>
                            <CollapsibleContent className="mt-0.5 space-y-0.5 pl-2">
                              {sub.items.map(renderNavLink)}
                            </CollapsibleContent>
                          </Collapsible>
                        )
                      })}
                    </div>
                  </div>
                )
              }
              const visibleItems = group.items.filter(navItemVisible)
              if (visibleItems.length === 0) return null
              return (
                <div key={group.labelKey ?? `group-${gi}`}>
                  {group.labelKey && (
                    <div className="text-muted-foreground/80 px-3 pt-3 pb-1.5 text-xs font-semibold tracking-wider uppercase">
                      {t(group.labelKey)}
                    </div>
                  )}
                  <div className="space-y-0.5">{visibleItems.map(renderNavLink)}</div>
                </div>
              )
            })}
          </nav>
        </aside>
        <div className="flex min-w-0 flex-1 flex-col">
          <header className="bg-card flex h-14 shrink-0 items-center justify-between border-b px-6">
            <AppBreadcrumb />
            <div className="ml-auto flex items-center gap-1">
              <a
                href="/api-docs.html"
                target="_blank"
                rel="noopener noreferrer"
                className="text-muted-foreground hover:bg-accent hover:text-foreground inline-flex size-8 items-center justify-center rounded-md transition-colors"
                title={t("nav.apiDocs")}
              >
                <FileText className="h-4 w-4" />
              </a>
              <LanguageSwitcher />
              <UserMenu />
            </div>
          </header>
          <main className="flex-1 overflow-auto">
            {/* Cap the reading width: on ultrawide displays a full-bleed table
                stretches columns until related values are screens apart. */}
            <div className="mx-auto w-full max-w-[1600px]">
              <Outlet />
            </div>
          </main>
        </div>
      </div>
    </TooltipProvider>
  )
}
