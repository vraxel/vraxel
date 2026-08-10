import ky, { HTTPError, TimeoutError } from "ky"
import { toast } from "sonner"
import { clearLocalAuthState, getCSRFToken, refreshAccessToken, startAuthFlow } from "@/core/auth/auth"
import { useScopeStore } from "@/core/scope/scope-store"
import type { StatusResponse, StatusResponseDetail } from "./types"

export class ApiError extends Error {
  status: number
  reason: string
  details?: StatusResponseDetail[]

  constructor(response: StatusResponse) {
    super(response.message)
    this.name = "ApiError"
    this.status = typeof response.status === "number" ? response.status : parseInt(String(response.status), 10)
    this.reason = response.reason
    this.details = response.details
  }
}

/** Deduplicate concurrent 401 token refresh attempts. */
let refreshPromise: Promise<boolean> | null = null

async function refreshTokenOnce(): Promise<boolean> {
  if (refreshPromise) return refreshPromise
  refreshPromise = refreshAccessToken().finally(() => {
    refreshPromise = null
  })
  return refreshPromise
}

/** Throttle scope invalidation to avoid 403 → invalidate → re-fetch → 403 loops. */
let lastScopeInvalidateAt = 0
const SCOPE_INVALIDATE_COOLDOWN_MS = 10_000

function isSafeMethod(method: string): boolean {
  const m = method.toUpperCase()
  return m === "GET" || m === "HEAD" || m === "OPTIONS"
}

export const api = ky.create({
  prefix: "/api",
  credentials: "include",
  // ky retries safe methods (GET/HEAD/OPTIONS) twice by default on
  // TimeoutError + 408/413/429/5xx — three round-trips per click, i.e.
  // three identical error toasts before the user can react. Endpoints
  // that genuinely benefit from a retry can opt back in per-call.
  retry: 0,
  // 10s default; endpoints that proxy a slow upstream raise it per-call
  // (SLOW_UPSTREAM_TIMEOUT_MS) so a real backend hang still surfaces fast.
  timeout: 10000,
  hooks: {
    beforeRequest: [
      ({ request }) => {
        if (!isSafeMethod(request.method)) {
          const csrf = getCSRFToken()
          if (csrf) request.headers.set("X-CSRF-Token", csrf)
        }
        // Save a clone BEFORE fetch() consumes the body stream.
        // The afterResponse 401-retry path needs this to rebuild
        // the request; `new Request(consumed)` throws on POST/PATCH.
        // Stash it on a typed-augmented Request (same idiom as the
        // _apiBody marker below) rather than `any`.
        ;(request as Request & { _retryClone?: Request })._retryClone = request.clone()
      },
    ],
    beforeError: [
      ({ error }) => {
        // In ky 2.0, response body is auto-consumed into error.data
        if (error instanceof HTTPError) {
          const body = error.data as StatusResponse | undefined
          if (body && body.reason) {
            (error as HTTPError & { _apiBody: StatusResponse })._apiBody = body
          }
        }
        return error
      },
    ],
    afterResponse: [
      async ({ request, response }) => {
        const isRetry = request.headers.get("X-Retry") === "true"
        if (response.status === 401 && !isRetry) {
          const refreshed = await refreshTokenOnce()
          if (refreshed) {
            // Use the pre-fetch clone saved in beforeRequest; the
            // original request's body stream was consumed by fetch().
            const saved = (request as Request & { _retryClone?: Request })._retryClone
            const retryRequest = saved ? new Request(saved) : new Request(request.url, { method: request.method, headers: request.headers })
            retryRequest.headers.set("X-Retry", "true")
            if (!isSafeMethod(retryRequest.method)) {
              const csrf = getCSRFToken()
              if (csrf) retryRequest.headers.set("X-CSRF-Token", csrf)
            }
            return ky(retryRequest, { credentials: "include" })
          }
          // Session expired or first visit with no cookie -- restart OIDC.
          // Wipe local auth state, then redirect with returnTo saved so the
          // user lands back on the current page after login. Cross-user URL
          // "replay" is safe: returnTo is just a path, RBAC gates data
          // access after login. Explicit logout (auth.ts:logout) already
          // passes saveReturnTo:false separately.
          clearLocalAuthState()
          await startAuthFlow()
          return response
        }
        if (response.status === 403) {
          const now = Date.now()
          if (now - lastScopeInvalidateAt > SCOPE_INVALIDATE_COOLDOWN_MS) {
            lastScopeInvalidateAt = now
            useScopeStore.getState().invalidate()
          }
        }
        return response
      },
    ],
  },
})

// For endpoints that proxy a slow upstream (external providers, log
// stores, ...), pass a raised per-call `timeout` instead of loosening
// the 10s default globally -- a real backend hang should surface fast.
export const SLOW_UPSTREAM_TIMEOUT_MS = 30000

/**
 * Wraps a ky request promise. Catches HTTPError (4xx) and converts to ApiError.
 */
export async function apiRequest<T>(request: Promise<T>): Promise<T> {
  try {
    return await request
  } catch (err) {
    if (err instanceof TimeoutError) {
      throw new ApiError({
        apiVersion: "",
        kind: "Status",
        status: 408,
        reason: "Timeout",
        message: "request timeout",
      })
    }
    if (err instanceof HTTPError) {
      const apiBody = (err as HTTPError & { _apiBody?: StatusResponse })._apiBody
      if (apiBody) {
        // Use HTTP status code (numeric) instead of body's status field ("Failure" string)
        throw new ApiError({ ...apiBody, status: err.response.status })
      }
      throw new ApiError({
        apiVersion: "",
        kind: "Status",
        status: err.response.status,
        reason: err.response.statusText,
        message: err.response.statusText,
      })
    }
    // Wrap network-layer failures so showApiError can pick a meaningful
    // headline instead of falling through to its non-ApiError branch
    // ("api.error.internalError" -> "服务器内部错误，请稍后重试"). The most
    // common case (bug #234) is fetch() throwing TypeError "Failed to
    // fetch" when the Vraxel server pod restarts mid-request -- the user
    // clicked Undeploy during the brief unreachable window and saw a
    // misleading "internal server error" instead of an actionable
    // "service unreachable, please retry" message. Other network
    // primitives (connection reset, ECONNREFUSED, DNS failure) surface
    // as TypeError as well in modern browsers, so a single catch-all on
    // TypeError covers the practical surface area without needing to
    // enumerate every cause.
    if (err instanceof TypeError) {
      throw new ApiError({
        apiVersion: "",
        kind: "Status",
        status: 0,
        reason: "NetworkError",
        message: err.message || "network request failed",
      })
    }
    throw err
  }
}

// Map backend English error messages to i18n keys for frontend
// translation. Exact-match first, then prefix, then substring; entries
// must mirror strings actually emitted by lib/api/validation and the
// modules' validation.go files -- do not invent entries speculatively.
const detailMessageMap: Record<string, string> = {
  "is required": "api.validation.required",
  "must not be empty": "api.validation.required",
  "must be 3-50 alphanumeric characters, underscores, or hyphens": "api.validation.username.format",
  "is not a valid email address": "api.validation.email.format",
  "must be a valid Chinese mobile number (e.g. 13800138000)": "api.validation.phone.format",
  "must be 8-72 bytes": "api.validation.password.length",
  "must contain at least one uppercase letter": "api.validation.password.uppercase",
  "must contain at least one lowercase letter": "api.validation.password.lowercase",
  "must contain at least one digit": "api.validation.password.digit",
  "must be 'active' or 'inactive'": "api.validation.status.format",
  "must be 3-50 lowercase alphanumeric characters or hyphens": "api.validation.name.format",
  "must be between 0 and 1000000": "api.validation.memberRange",
}

const detailMessagePrefixMap: Record<string, string> = {
  "invalid pattern": "api.validation.name.format",
  "must be at most ": "api.validation.description.tooLong",
  "must match ": "api.validation.name.format",
  "pattern ": "api.validation.name.format",
}

const messageMap: Record<string, string> = {
  "old password is incorrect": "api.error.oldPasswordIncorrect",
  "oldPassword and newPassword are required": "api.error.badRequest",
  "cannot remove workspace owner": "api.error.cannotRemoveOwner",
  "cannot remove namespace owner": "api.error.cannotRemoveOwner",
  "cannot delete built-in role": "api.error.cannotDeleteBuiltinRole",
  "cannot delete role with active bindings": "api.error.cannotDeleteRoleWithBindings",
}

const messagePrefixMap: Record<string, string> = {
  "namespace member limit exceeded": "api.error.memberLimitExceeded",
  "cannot delete workspace": "api.error.cannotDeleteWorkspace",
  "cannot delete namespace": "api.error.cannotDeleteNamespace",
  "value too long for ": "api.error.valueTooLong",
}

const reasonMessageMap: Record<string, string> = {
  Conflict: "api.error.conflict",
  NotFound: "api.error.notFound",
  BadRequest: "api.error.badRequest",
  InvalidJSONBody: "api.error.invalidJSONBody",
  Forbidden: "api.error.forbidden",
  Timeout: "api.error.timeout",
  BadGateway: "api.error.badGateway",
  GatewayTimeout: "api.error.gatewayTimeout",
  // 401 with refresh failure -- the afterResponse hook redirects to the
  // OIDC login flow, but the rejected request still bubbles up to the
  // caller's catch + showApiError. Use a session-expired message so the
  // brief toast before the navigate matches reality instead of falling
  // through to the raw "authentication required" backend message.
  Unauthorized: "api.error.sessionExpired",
  // Network-layer failures (fetch TypeError) get wrapped here so the
  // caller sees a meaningful "service unreachable" message instead of
  // the generic "internal server error" non-ApiError fallback.
  NetworkError: "api.error.networkError",
}

export function translateDetailMessage(message: string): string {
  if (detailMessageMap[message]) return detailMessageMap[message]
  for (const [prefix, key] of Object.entries(detailMessagePrefixMap)) {
    if (message.startsWith(prefix)) return key
  }
  return message
}

// Substring matches: for backend messages whose variable part precedes
// the recognisable text. Empty today; add entries alongside the backend
// message they mirror.
const messageContainsMap: Record<string, string> = {
}

export function translateApiError(err: ApiError): string {
  if (messageMap[err.message]) return messageMap[err.message]
  for (const [prefix, key] of Object.entries(messagePrefixMap)) {
    if (err.message.startsWith(prefix)) return key
  }
  for (const [substr, key] of Object.entries(messageContainsMap)) {
    if (err.message.includes(substr)) return key
  }
  return reasonMessageMap[err.reason] ?? err.message
}

// isReasonOnlyMatch returns true when the error's translation came from the
// generic reasonMessageMap fallback (no specific message-level mapping
// matched). In that state the i18n text is a generic headline like "操作冲突"
// / "请求参数错误" -- it tells the user the HTTP class but not why the call
// was rejected. Callers surfacing the error to a user-visible toast use this
// to decide whether to attach the raw backend message as a description.
function isReasonOnlyMatch(err: ApiError): boolean {
  if (messageMap[err.message]) return false
  for (const prefix of Object.keys(messagePrefixMap)) {
    if (err.message.startsWith(prefix)) return false
  }
  for (const substr of Object.keys(messageContainsMap)) {
    if (err.message.includes(substr)) return false
  }
  return !!reasonMessageMap[err.reason]
}

// extractMessageSuffix pulls the detail suffix from an error whose
// prefix was translated to an i18n template. Example:
//   "cannot delete cloud_provider: referenced by 2 network(s), 1 host(s)"
//   prefix: "cannot delete cloud_provider"
//   → suffix: "referenced by 2 network(s), 1 host(s)"
// The suffix is appended to the toast so the user sees exactly which
// dependents blocked the action, rather than the generic template alone.
function extractMessageSuffix(err: ApiError): string {
  for (const prefix of Object.keys(messagePrefixMap)) {
    if (err.message.startsWith(prefix)) {
      return err.message.slice(prefix.length).replace(/^[:\s]+/, "").trim()
    }
  }
  return ""
}

// Maps the backend resource-kind tokens emitted by the workspace / namespace
// delete pre-check to i18n keys for localized rendering. Keep in sync with
// pkg/db/query/scope_resource_check.sql.
const BLOCKING_RESOURCE_KIND_KEYS: Record<string, string> = {
  // e.g. host: "blockingResource.host" -- add one entry per branch
  // registered in CountWorkspace/NamespaceBlockingResources.
}

// Translates the suffix "host(3), mysql_instance(2)" into a localized list
// like "主机(3), MySQL 实例(2)". Returns the suffix unchanged when no token
// matches the kind map (the suffix came from a different error pattern).
function localizeBlockingKindsSuffix(suffix: string, t: (key: string) => string): string {
  if (!suffix) return suffix
  const tokens = suffix.split(/,\s*/)
  let matched = false
  const out = tokens.map((tok) => {
    const m = tok.match(/^([a-z_]+)\((\d+)\)$/)
    if (!m) return tok
    const key = BLOCKING_RESOURCE_KIND_KEYS[m[1]]
    if (!key) return tok
    matched = true
    return `${t(key)}(${m[2]})`
  })
  return matched ? out.join(", ") : suffix
}

// Decompose an ApiError into a translated headline plus the raw backend
// message that should be shown verbatim (e.g. as sonner's `description` slot
// or as a second line in a form-root banner). Returns `description: ""` when
// the translation is specific enough to fully describe the error, so callers
// can render a single-line variant.
//
// Shared by showApiError (toast) and formApiErrorMessage (form root banner);
// keeping the rules in one place ensures the toast / dialog / banner surfaces
// stay consistent. Field-level details (err.details[0]) are handled by callers
// because they need form context to bind the message to the right field.
function decomposeApiError(
  err: ApiError,
  t: (key: string, params?: Record<string, string | number>) => string,
  resourceKey?: string,
): { headline: string; description: string } {
  const i18nKey = translateApiError(err)
  if (i18nKey === err.message) {
    // No translation matched at all. Surface the raw backend message.
    return { headline: err.message, description: "" }
  }
  const params = resourceKey ? { resource: t(resourceKey) } : undefined
  const headline = t(i18nKey, params)
  const rawSuffix = extractMessageSuffix(err)
  // Reason-only fallback: the translation is a generic HTTP-class headline
  // (e.g. "操作冲突" for a Conflict whose message didn't match any specific
  // prefix). The user needs the raw backend message to know which precondition
  // failed - "操作冲突" alone tells them the HTTP class but not why. Skip when
  // extractMessageSuffix produced a recognised structured detail (Kafka broker
  // code, deletion-blockers list, ...) because those localize cleanly below.
  if (isReasonOnlyMatch(err) && !rawSuffix && err.message) {
    return { headline, description: err.message }
  }
  if (rawSuffix) {
    const localized = localizeBlockingKindsSuffix(rawSuffix, t)
    return { headline: `${headline} (${localized})`, description: "" }
  }
  return { headline, description: "" }
}

/**
 * Build a form-root error message from a caught exception. Mirrors showApiError's
 * translation + reason-only-fallback logic, with the description sub-line
 * (sonner's `description` slot) concatenated with "\n" so a single string can
 * carry both lines into form.setError("root", { message }). FormErrorBanner
 * renders the result with `whitespace-pre-line` so the "\n" produces a real
 * line break.
 *
 * @param err - The caught error
 * @param t - The i18n translation function
 * @param resourceKey - Optional i18n key for the resource name (e.g. "user.title")
 */
export function formApiErrorMessage(
  err: unknown,
  t: (key: string, params?: Record<string, string | number>) => string,
  resourceKey?: string,
): string {
  if (!(err instanceof ApiError)) {
    return t("api.error.internalError")
  }
  // Field-level details: surface the first one with its label so the user
  // sees the actionable field name when the caller doesn't have the form
  // context to setError on the specific field (e.g. setError("root") sites
  // outside handleFormApiError that still want a useful root banner).
  if (err.details?.length) {
    const d = err.details[0]
    const fieldLabel = d.field.replace(/^(metadata|spec)\./, "")
    const i18nKey = translateDetailMessage(d.message)
    return i18nKey !== d.message
      ? t(i18nKey, { field: fieldLabel })
      : `${fieldLabel}: ${d.message}`
  }
  const { headline, description } = decomposeApiError(err, t, resourceKey)
  return description ? `${headline}\n${description}` : headline
}

/**
 * Show a toast error for a caught exception. Handles both ApiError and unknown errors.
 * @param err - The caught error
 * @param t - The i18n translation function
 * @param resourceKey - Optional i18n key for the resource name (e.g. "user.title")
 */
export function showApiError(err: unknown, t: (key: string, params?: Record<string, string | number>) => string, resourceKey?: string) {
  if (err instanceof ApiError) {
    // Field-level details (FieldError list) carry the actionable message
    // ("disks[0].mountPoint: duplicate ..."). The bare reason ("BadRequest"
    // -> "请求参数错误") tells the user nothing. Surface the first detail
    // verbatim through the same i18n map handleFormApiError uses; the
    // mounting toast is a fallback for callers without RHF setError.
    if (err.details?.length) {
      const d = err.details[0]
      const fieldLabel = d.field.replace(/^(metadata|spec)\./, "")
      const i18nKey = translateDetailMessage(d.message)
      const text = i18nKey !== d.message
        ? t(i18nKey, { field: fieldLabel })
        : `${fieldLabel}: ${d.message}`
      toast.error(text)
      return
    }
    const { headline, description } = decomposeApiError(err, t, resourceKey)
    if (description) {
      toast.error(headline, { description })
    } else {
      toast.error(headline)
    }
  } else {
    toast.error(t("api.error.internalError"))
  }
}

/**
 * Handle API errors in form submissions by mapping backend errors to form field errors.
 * @param err - The caught error
 * @param form - react-hook-form's form instance (must have setError)
 * @param t - i18n translation function
 * @param i18nPrefix - The i18n key prefix for field names (e.g., "region", "site", "location")
 * @param resourceKey - The i18n key for the resource title (e.g., "region.title")
 */
export function handleFormApiError(
  err: unknown,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  form: { setError: (name: any, error: { message: string }) => void },
  t: (key: string, params?: Record<string, string | number>) => string,
  i18nPrefix: string,
  resourceKey: string,
) {
  if (err instanceof ApiError && err.details?.length) {
    for (const d of err.details) {
      // RHF setError uses dot paths (`arr.0.x`) for nested arrays; the
      // backend emits bracket notation (`arr[0].x`) under a metadata./
      // spec. envelope the flat zod schemas do not carry. Normalize so
      // FormMessage on the matching FormField actually renders.
      const field = d.field
        .replace(/^(metadata|spec)\./, "")
        .replace(/\[(\d+)\]/g, ".$1")
      const i18nKey = translateDetailMessage(d.message)
      let message: string
      if (i18nKey !== d.message) {
        message = t(i18nKey, { field: t(`${i18nPrefix}.${field}`) || field })
      } else {
        const rangeMatch = d.message.match(/^must be between (-?\d+) and (\d+)$/)
        message = rangeMatch
          ? t("api.validation.intRange", { min: rangeMatch[1], max: rangeMatch[2] })
          : d.message
      }
      form.setError(field, { message })
    }
  } else if (err instanceof ApiError) {
    // PG unique-violation message is "<table>_<column>_key already exists"
    // (with a leading space when domainErr drops the resource arg). Surface
    // it as a field-level error when the column has a per-field "taken"
    // i18n key, instead of the generic Conflict banner that would otherwise
    // come from the reason fallback.
    const uniqueMatch = err.message.match(/(?:^|[\s_])([a-z]+)_key already exists$/)
    if (uniqueMatch) {
      const field = uniqueMatch[1]
      const takenKey = `api.validation.${field}.taken`
      const taken = t(takenKey)
      if (taken !== takenKey) {
        form.setError(field, { message: taken })
        return
      }
    }
    // Bug #219: previously this branch dropped err.message whenever
    // translateApiError fell back to the reason-only key (e.g. Conflict ->
    // "操作冲突"), so the user saw only the HTTP class with no detail. Route
    // through formApiErrorMessage which keeps the raw backend message on a
    // second line for the reason-only case.
    form.setError("root", { message: formApiErrorMessage(err, t, resourceKey) })
  } else {
    form.setError("root", { message: t("api.error.internalError") })
  }
}

/** Default page size for select/dropdown data fetches (e.g., loading all regions for a select). */
export const SELECT_PAGE_SIZE = 200
