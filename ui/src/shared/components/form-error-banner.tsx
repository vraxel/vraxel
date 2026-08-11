import type { FieldErrors, FieldError } from "react-hook-form"
import { useTranslation } from "@/i18n"

/**
 * Renders the form-level (root) error as a styled banner above form fields.
 * Extracts the repeated pattern from 30+ form dialogs into one place so
 * spacing, colors, and layout stay consistent project-wide.
 *
 * When there is no root error but per-field validation has produced errors
 * (zod / superRefine failed on submit) we still surface a single-line
 * summary banner. Without this, dialogs with many fields could fail
 * validation on a field below the fold while `shouldFocusError` either
 * focuses a disabled / hidden input or fails to scroll the form area —
 * the user clicks Save and sees no feedback at all. The summary banner
 * keeps the failure visible at the top of the form even when individual
 * `<FormMessage />`s are scrolled off-screen.
 *
 * Bug #193: some dialogs (probes / lifecycle) bind inputs via
 * `form.register()` without a wrapping `<FormField>`, so even when zod
 * adds an issue against that path, no `<FormMessage>` renders for it.
 * The user then saw "form has N errors" with no field highlighted and
 * no way to know which field. Surface each error's message inline in
 * the banner so the user has at least one actionable hint regardless
 * of whether the offending field has a `<FormMessage>` slot.
 *
 * Usage:
 *   <FormErrorBanner errors={form.formState.errors} />
 */
export function FormErrorBanner({ errors }: { errors: FieldErrors }) {
  const { t } = useTranslation()
  if (errors.root?.message) {
    // formApiErrorMessage embeds the raw backend message on a second line
    // (separated by "\n") for the reason-only fallback case (bug #219). Use
    // whitespace-pre-line so that line break renders; without it the two
    // lines collapse into a single run and the user sees "操作冲突 uk_...
    // already exists" mashed together.
    return (
      <div className="bg-destructive/10 text-destructive mb-4 shrink-0 rounded-md px-3 py-2 text-sm break-words whitespace-pre-line">
        {errors.root.message as string}
      </div>
    )
  }
  const messages = collectErrorMessages(errors)
  if (messages.length === 0) return null
  return (
    <div className="bg-destructive/10 text-destructive mb-4 shrink-0 space-y-1 rounded-md px-3 py-2 text-sm">
      <div>{t("api.validation.formHasErrors", { count: messages.length })}</div>
      {messages.length > 0 && (
        <ul className="list-disc space-y-0.5 pl-5 text-xs">
          {messages.map((m, i) => (
            <li key={i}>{m}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

// collectErrorMessages walks the react-hook-form errors tree and returns
// every leaf `.message` (deduplicated, in insertion order). RHF nests
// errors to mirror the schema shape:
// FieldErrors is `{ [name]: FieldError | FieldErrors | (FieldError | FieldErrors)[] }`.
// A leaf is anything whose `.message` is a string.
function collectErrorMessages(node: unknown): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  walk(node, seen, out)
  return out
}

function walk(node: unknown, seen: Set<string>, out: string[]): void {
  if (node == null || typeof node !== "object") return
  if (Array.isArray(node)) {
    for (const item of node) walk(item, seen, out)
    return
  }
  const obj = node as Record<string, unknown>
  const msg = (obj as FieldError).message
  if (typeof msg === "string" && msg.trim()) {
    if (!seen.has(msg)) {
      seen.add(msg)
      out.push(msg)
    }
    return
  }
  for (const key of Object.keys(obj)) {
    if (key === "root") continue // root is handled separately (banner-level)
    walk(obj[key], seen, out)
  }
}
