import { useEffect, useState } from "react"
import { Loader2 } from "lucide-react"
import { BrandMark } from "@/shared/components/brand-mark"
import { Link, useSearchParams } from "react-router"
import { Button } from "@/shared/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card"
import { Input } from "@/shared/ui/input"
import { Label } from "@/shared/ui/label"
import { PasswordInput } from "@/shared/ui/password-input"
import { LanguageSwitcher } from "@/shared/components/language-switcher"
import { SocialLogin } from "@/shared/components/social-login"
import { useTranslation } from "@/i18n"
import {
  fetchAuthConfig,
  registerWithCredentials,
  startAuthFlow,
  type AuthConfig,
} from "@/core/auth/auth"

export default function RegisterPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const requestId = searchParams.get("request_id")
  const [username, setUsername] = useState("")
  const [email, setEmail] = useState("")
  const [displayName, setDisplayName] = useState("")
  const [password, setPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [authConfig, setAuthConfig] = useState<AuthConfig | null>(null)

  // Same request_id handshake guard as the login page: registration completes
  // the pending OIDC authorization, so without one we restart the flow (which
  // lands the user on /login with a fresh request_id).
  useEffect(() => {
    if (!requestId || !sessionStorage.getItem("oidc_flow_pending")) {
      startAuthFlow()
    }
  }, [requestId])

  useEffect(() => {
    fetchAuthConfig()
      .then(setAuthConfig)
      .catch(() => setAuthConfig(null))
  }, [])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    if (password !== confirmPassword) {
      setError(t("register.error.passwordMismatch"))
      return
    }

    setLoading(true)
    try {
      const redirectUri = await registerWithCredentials(
        username,
        email,
        password,
        displayName,
        requestId!,
      )
      const url = new URL(redirectUri)
      window.location.href = url.pathname + url.search
    } catch (err) {
      const code = (err as Error & { code?: string }).code
      const msg = err instanceof Error ? err.message : ""
      if (code === "conflict") {
        setError(t("register.error.conflict"))
      } else if (code === "rate_limited") {
        setError(t("register.error.tooManyAttempts"))
      } else if (msg) {
        setError(msg)
      } else {
        setError(t("register.error.failed"))
      }
    } finally {
      setLoading(false)
    }
  }

  // While redirecting to /oidc/authorize, show nothing.
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
          <BrandMark className="text-primary size-11" />
          <CardTitle className="text-xl">{t("register.title")}</CardTitle>
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
              <Label htmlFor="email">{t("register.email")}</Label>
              <Input
                id="email"
                type="email"
                placeholder={t("register.emailPlaceholder")}
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="displayName">{t("register.displayName")}</Label>
              <Input
                id="displayName"
                placeholder={t("register.displayNamePlaceholder")}
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t("login.password")}</Label>
              <PasswordInput
                id="password"
                autoComplete="new-password"
                placeholder={t("login.passwordPlaceholder")}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirmPassword">{t("register.confirmPassword")}</Label>
              <PasswordInput
                id="confirmPassword"
                autoComplete="new-password"
                placeholder={t("register.confirmPasswordPlaceholder")}
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                required
              />
            </div>
            {error && <p className="text-destructive text-sm">{error}</p>}
            <Button className="w-full" type="submit" disabled={loading}>
              {loading && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
              {t("register.submit")}
            </Button>
          </form>

          {authConfig && authConfig.socialProviders.length > 0 && (
            <div className="mt-4">
              <SocialLogin providers={authConfig.socialProviders} requestId={requestId} />
            </div>
          )}

          <p className="text-muted-foreground mt-4 text-center text-sm">
            {t("register.haveAccount")}{" "}
            <Link
              to={`/login?request_id=${encodeURIComponent(requestId)}`}
              className="text-primary hover:underline"
            >
              {t("register.signIn")}
            </Link>
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
