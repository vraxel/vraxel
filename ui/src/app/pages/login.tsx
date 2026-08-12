import { useEffect, useState } from "react"
import { Loader2, LayoutDashboard } from "lucide-react"
import { Link, useSearchParams } from "react-router"
import { Button } from "@/shared/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card"
import { Input } from "@/shared/ui/input"
import { Label } from "@/shared/ui/label"
import { LanguageSwitcher } from "@/shared/components/language-switcher"
import { SocialLogin } from "@/shared/components/social-login"
import { useTranslation } from "@/i18n"
import {
  fetchAuthConfig,
  loginWithCredentials,
  startAuthFlow,
  type AuthConfig,
} from "@/core/auth/auth"

const loginErrorMap: Record<string, string> = {
  "invalid credentials": "login.error.invalidCredentials",
  "account is not active": "login.error.accountInactive",
  "too many failed login attempts": "login.error.tooManyAttempts",
  "invalid or expired request_id": "login.error.sessionExpired",
}

export default function LoginPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const requestId = searchParams.get("request_id")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [authConfig, setAuthConfig] = useState<AuthConfig | null>(null)

  useEffect(() => {
    fetchAuthConfig()
      .then(setAuthConfig)
      .catch(() => setAuthConfig(null))
  }, [])

  // A request_id is only meaningful if THIS browser session started the
  // handshake that produced it; otherwise (direct navigation, a stale
  // bookmark) it is likely expired and we start a fresh flow.
  //
  // The marker is read, never consumed: exchangeCodeForTokens clears it
  // when the handshake completes. Consuming it here made the effect
  // non-idempotent -- a second run found no flag, restarted the flow,
  // and the new page did the same, forever.
  useEffect(() => {
    if (!requestId || !sessionStorage.getItem("oidc_flow_pending")) {
      startAuthFlow()
    }
  }, [requestId])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    setLoading(true)
    try {
      const redirectUri = await loginWithCredentials(username, password, requestId!)
      // Navigate using relative path to stay on the same origin,
      // preserving sessionStorage (PKCE code_verifier) across the redirect.
      const url = new URL(redirectUri)
      window.location.href = url.pathname + url.search
    } catch (err) {
      const msg = err instanceof Error ? err.message : ""
      const key = loginErrorMap[msg.toLowerCase()]
      if (msg.toLowerCase() === "invalid or expired request_id") {
        setError(t("login.error.sessionExpired"))
        setTimeout(() => startAuthFlow(), 1500)
        return
      }
      setError(key ? t(key) : t("login.error.failed"))
    } finally {
      setLoading(false)
    }
  }

  // While redirecting to /oidc/authorize, show nothing
  if (!requestId) {
    return null
  }

  return (
    <div
      className="flex min-h-screen items-center justify-center bg-cover bg-center bg-no-repeat"
      style={{ backgroundImage: "url('/login-bg.svg')" }}
    >
      <Card className="relative w-full max-w-sm shadow-lg">
        <div className="absolute top-3 right-3">
          <LanguageSwitcher />
        </div>
        <CardHeader className="justify-items-center gap-3 text-center">
          <span className="bg-primary text-primary-foreground flex size-10 items-center justify-center rounded-xl">
            <LayoutDashboard className="h-5 w-5" />
          </span>
          <CardTitle className="text-xl">{t("login.title")}</CardTitle>
          <CardDescription>Vraxel Console</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">{t("login.username")}</Label>
              <Input
                id="username"
                placeholder={t("login.usernamePlaceholder")}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t("login.password")}</Label>
              <Input
                id="password"
                type="password"
                autoComplete="off"
                placeholder={t("login.passwordPlaceholder")}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </div>
            {error && <p className="text-destructive text-sm">{error}</p>}
            <Button className="w-full" type="submit" disabled={loading}>
              {loading && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
              {t("login.signIn")}
            </Button>
          </form>

          {authConfig && authConfig.socialProviders.length > 0 && (
            <div className="mt-4">
              <SocialLogin providers={authConfig.socialProviders} requestId={requestId} />
            </div>
          )}

          {authConfig?.selfRegistration && (
            <p className="text-muted-foreground mt-4 text-center text-sm">
              {t("login.noAccount")}{" "}
              <Link
                to={`/register?request_id=${encodeURIComponent(requestId)}`}
                className="text-primary hover:underline"
              >
                {t("login.createAccount")}
              </Link>
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
