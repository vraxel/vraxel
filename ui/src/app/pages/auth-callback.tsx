import { useEffect, useRef, useState } from "react"
import { useNavigate, useSearchParams } from "react-router"
import { consumeReturnTo, exchangeCodeForTokens, startAuthFlow } from "@/core/auth/auth"
import { useTranslation } from "@/i18n"

export default function AuthCallbackPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)
  const exchanged = useRef(false)

  // A missing code is knowable synchronously from the URL -- derive it at
  // render instead of setting state from the effect.
  const missingCode = !searchParams.get("code")

  useEffect(() => {
    if (exchanged.current) return
    exchanged.current = true

    const code = searchParams.get("code")
    if (!code) return

    exchangeCodeForTokens(code)
      .then(() => navigate(consumeReturnTo(), { replace: true }))
      .catch(async (err) => {
        if (err instanceof Error && err.message === "Missing PKCE code verifier") {
          // PKCE verifier lost (new tab, page refresh, etc.) -- restart auth flow
          await startAuthFlow()
          return
        }
        setError(err.message)
      })
  }, [searchParams, navigate])

  const shownError = error ?? (missingCode ? t("auth.missingCode") : null)
  if (shownError) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-destructive">{shownError}</p>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center">
      <p className="text-muted-foreground">{t("auth.authenticating")}</p>
    </div>
  )
}
