import { create } from "zustand"
import { persist } from "zustand/middleware"
import type { Locale, Messages } from "./types"
import zhCN, { type MessageKey } from "./locales/zh-CN"
import enUS from "./locales/en-US"

// Editor autocomplete for every catalog key while still admitting
// dynamically-built keys (error maps, `role.${name}` templates). The
// `string & {}` half keeps arbitrary strings assignable without
// collapsing the union, so literals autocomplete but nothing breaks.
export type TranslateKey = MessageKey | (string & {})

const messages: Record<Locale, Messages> = {
  "zh-CN": zhCN,
  "en-US": enUS,
}

interface I18nState {
  locale: Locale
  setLocale: (locale: Locale) => void
}

export const useI18nStore = create<I18nState>()(
  persist(
    (set) => ({
      locale: "zh-CN",
      setLocale: (locale) => set({ locale }),
    }),
    { name: "vraxel-locale" },
  ),
)

function translateWith(
  locale: Locale,
  key: string,
  vars?: Record<string, string | number>,
): string {
  const raw = messages[locale]?.[key]
  let msg = raw ?? (vars?.defaultValue != null ? String(vars.defaultValue) : key)
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      if (k === "defaultValue") continue
      // replaceAll, not replace: a placeholder may appear more than once in
      // one message (e.g. service.deploy.error.singleFileBadEntryPoint
      // mentions {field}/{argv0} twice). String.replace only swaps the first
      // occurrence, leaving the rest rendered literally as "{field}".
      msg = msg.replaceAll(`{${k}}`, String(v))
    }
  }
  return msg
}

export function useTranslation() {
  const { locale, setLocale } = useI18nStore()
  const t = (key: TranslateKey, vars?: Record<string, string | number>): string =>
    translateWith(locale, key, vars)
  return { t, locale, setLocale }
}

// Non-hook translator for code that runs outside React (e.g. the global
// TanStack Query error handler in core/query/client.ts). Reads the current
// locale directly from the zustand store.
export function translate(key: TranslateKey, vars?: Record<string, string | number>): string {
  return translateWith(useI18nStore.getState().locale, key, vars)
}

// Translate into a specific locale regardless of the active UI locale. Used by
// the sidebar quick-search to build cross-locale alias tokens (an English term
// can match a Chinese label and vice versa).
export function translateTo(locale: Locale, key: TranslateKey): string {
  return translateWith(locale, key)
}

// Mirrors pkg/apis/iam/store/rbac_builtin.go built-in role name constants.
// Backend stores English display_name/description; user-facing text comes from
// this FE i18n catalog. Search needs to match the localized strings, so list
// pages pass these names to the backend as `extra_names` so SQL OR-matches them
// even when the typed term only appears in the localized form.
const BUILTIN_ROLE_NAMES = [
  "platform-admin",
  "platform-viewer",
  "workspace-admin",
  "workspace-viewer",
  "workspace-member",
  "namespace-admin",
  "namespace-viewer",
] as const

export function findBuiltinRoleNamesMatching(searchTerm: string, locale: Locale): string[] {
  const term = searchTerm.trim().toLowerCase()
  if (!term) return []
  const msgs = messages[locale]
  if (!msgs) return []
  const out: string[] = []
  for (const name of BUILTIN_ROLE_NAMES) {
    const display = (msgs[`role.${name}`] ?? "").toLowerCase()
    const desc = (msgs[`role.desc.${name}`] ?? "").toLowerCase()
    if (display.includes(term) || desc.includes(term)) {
      out.push(name)
    }
  }
  return out
}

export type { Locale } from "./types"
