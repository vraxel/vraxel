import { useEffect, useState } from "react"
import { useSearchParams } from "react-router"
import { Button } from "@/shared/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card"
import { Input } from "@/shared/ui/input"
import { Label } from "@/shared/ui/label"
import { LanguageSwitcher } from "@/shared/components/language-switcher"
import { useTranslation } from "@/i18n"
import { loginWithCredentials, startAuthFlow } from "@/core/auth/auth"

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
      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-2xl">{t("login.title")}</CardTitle>
            <LanguageSwitcher />
          </div>
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
              {loading ? "..." : t("login.signIn")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
