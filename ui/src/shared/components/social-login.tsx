import { Button } from "@/shared/ui/button"
import { useTranslation, type TranslateKey } from "@/i18n"
import { socialLoginUrl } from "@/core/auth/auth"

const PROVIDER_LABEL: Record<string, TranslateKey> = {
  github: "login.social.github",
  google: "login.social.google",
}

// SocialLogin renders one button per configured external provider. Clicking a
// button navigates to the backend start endpoint, carrying the current
// request_id so the callback can resume the pending authorization.
export function SocialLogin({ providers, requestId }: { providers: string[]; requestId: string }) {
  const { t } = useTranslation()
  const known = providers.filter((p) => p in PROVIDER_LABEL)
  if (known.length === 0) return null

  return (
    <div className="space-y-3">
      <div className="relative">
        <div className="absolute inset-0 flex items-center">
          <span className="w-full border-t" />
        </div>
        <div className="relative flex justify-center text-xs uppercase">
          <span className="bg-card text-muted-foreground px-2">{t("login.orContinueWith")}</span>
        </div>
      </div>
      <div className="space-y-2">
        {known.map((p) => (
          <Button
            key={p}
            type="button"
            variant="outline"
            className="w-full"
            onClick={() => {
              window.location.href = socialLoginUrl(p, requestId)
            }}
          >
            <ProviderIcon provider={p} />
            {t(PROVIDER_LABEL[p])}
          </Button>
        ))}
      </div>
    </div>
  )
}

function ProviderIcon({ provider }: { provider: string }) {
  if (provider === "github") {
    return (
      <svg viewBox="0 0 24 24" className="h-4 w-4" fill="currentColor" aria-hidden="true">
        <path d="M12 .5C5.73.5.5 5.73.5 12a11.5 11.5 0 0 0 7.86 10.92c.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.54-3.88-1.54-.53-1.34-1.29-1.7-1.29-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.2 1.77 1.2 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.56-.29-5.25-1.28-5.25-5.7 0-1.26.45-2.29 1.2-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.8 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.11 3.05.75.81 1.2 1.84 1.2 3.1 0 4.43-2.7 5.4-5.27 5.69.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0 0 23.5 12C23.5 5.73 18.27.5 12 .5Z" />
      </svg>
    )
  }
  if (provider === "google") {
    return (
      <svg viewBox="0 0 24 24" className="h-4 w-4" aria-hidden="true">
        <path
          fill="#4285F4"
          d="M23.5 12.3c0-.8-.07-1.6-.2-2.3H12v4.5h6.5a5.6 5.6 0 0 1-2.4 3.6v3h3.9c2.3-2.1 3.5-5.2 3.5-8.8Z"
        />
        <path
          fill="#34A853"
          d="M12 24c3.2 0 5.9-1.06 7.9-2.9l-3.9-3c-1 .7-2.3 1.15-4 1.15-3.1 0-5.7-2.1-6.6-4.9H1.4v3.1A12 12 0 0 0 12 24Z"
        />
        <path
          fill="#FBBC05"
          d="M5.4 14.35a7.2 7.2 0 0 1 0-4.7v-3.1H1.4a12 12 0 0 0 0 10.9l4-3.1Z"
        />
        <path
          fill="#EA4335"
          d="M12 4.75c1.75 0 3.3.6 4.55 1.8l3.4-3.4A12 12 0 0 0 1.4 6.55l4 3.1C6.3 6.85 8.9 4.75 12 4.75Z"
        />
      </svg>
    )
  }
  return null
}
