import { Navigate, useSearchParams } from "react-router"
import { useEffect } from "react"
import { Button } from "@/shared/ui/button"
import { useTranslation } from "@/i18n"
import { startAuthFlow, logout } from "@/core/auth/auth"
import { usePermissionStore } from "@/core/permission/permission-store"
import { useScopeStore } from "@/core/scope/scope-store"
import { getDefaultPath, getFirstPermittedPath } from "@/core/permission/use-permission"

const statusConfig: Record<string, { icon: string; titleKey: string; descKey: string }> = {
  "400": { icon: "⚠️", titleKey: "error.400.title", descKey: "error.400.desc" },
  "401": { icon: "🔒", titleKey: "error.401.title", descKey: "error.401.desc" },
  "403": { icon: "🚫", titleKey: "error.403.title", descKey: "error.403.desc" },
  "404": { icon: "📄", titleKey: "error.404.title", descKey: "error.404.desc" },
  "500": { icon: "⚙️", titleKey: "error.500.title", descKey: "error.500.desc" },
}

export default function ErrorPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const status = searchParams.get("status") || "404"
  const permissions = usePermissionStore((s) => s.permissions)
  const scopeWorkspaceId = useScopeStore((s) => s.workspaceId)
  const scopeNamespaceId = useScopeStore((s) => s.namespaceId)

  useEffect(() => {
    if (status === "401") {
      startAuthFlow()
    }
  }, [status])

  if (status === "401") {
    return null
  }

  // If permissions are already loaded and user has permissions, redirect away from 403.
  // Use scope-aware redirect when inside a workspace so the user does not get
  // teleported to a different project that happens to have higher-priority
  // NAV_ITEMS (same bug as the console icon, fixed in root-layout.tsx).
  if (status === "403" && permissions) {
    const hasAny =
      permissions.isPlatformAdmin ||
      (permissions.platform?.length ?? 0) > 0 ||
      Object.keys(permissions.workspaces ?? {}).length > 0 ||
      Object.keys(permissions.namespaces ?? {}).length > 0
    if (hasAny) {
      const target = scopeWorkspaceId
        ? getFirstPermittedPath(permissions, scopeWorkspaceId, scopeNamespaceId)
        : getDefaultPath(permissions)
      return <Navigate to={target} replace />
    }
  }

  const config = statusConfig[status] || statusConfig["500"]

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4">
      <span className="text-6xl">{config.icon}</span>
      <h1 className="text-2xl font-semibold">{t(config.titleKey)}</h1>
      <p className="text-muted-foreground">{t(config.descKey)}</p>
      <div className="flex gap-2">
        {status === "403" && (
          <Button variant="default" onClick={() => logout()}>
            {t("error.switchAccount")}
          </Button>
        )}
        <Button variant="outline" onClick={() => (window.location.href = "/")}>
          {t("error.backHome")}
        </Button>
      </div>
    </div>
  )
}
