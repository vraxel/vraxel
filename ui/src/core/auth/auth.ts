const CLIENT_ID = "vraxel-ui"
const RETURN_TO_KEY = "oidc_return_to"

// Auth-boundary reset registry. Identity stores (`useAuthStore` /
// `usePermissionStore`) self-register their clear callbacks here at module
// load. We don't import the stores directly because that would form a cycle
// with `api/client.ts` (which imports `startAuthFlow` from this file, and is
// transitively imported by every store via `api/iam/*`). Inverting the edge
// keeps `lib/auth.ts` free of `@/stores/*` imports.
const authBoundaryResets: Array<() => void> = []
export function registerAuthBoundaryReset(fn: () => void): void {
  authBoundaryResets.push(fn)
}

function generateRandomString(length: number): string {
  const array = new Uint8Array(length)
  crypto.getRandomValues(array)
  return Array.from(array, (b) => b.toString(16).padStart(2, "0")).join("")
}

async function sha256(plain: string): Promise<ArrayBuffer> {
  const encoder = new TextEncoder()
  const data = encoder.encode(plain)

  // crypto.subtle is only available in secure contexts (HTTPS or localhost)
  if (crypto.subtle) {
    return crypto.subtle.digest("SHA-256", data)
  }

  // Fallback: pure JS SHA-256 for non-secure contexts (e.g. HTTP with IP access)
  return sha256Fallback(data)
}

function sha256Fallback(data: Uint8Array): ArrayBuffer {
  const K: number[] = [
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
  ]

  const rotr = (n: number, x: number) => (x >>> n) | (x << (32 - n))
  const ch = (x: number, y: number, z: number) => (x & y) ^ (~x & z)
  const maj = (x: number, y: number, z: number) => (x & y) ^ (x & z) ^ (y & z)
  const sigma0 = (x: number) => rotr(2, x) ^ rotr(13, x) ^ rotr(22, x)
  const sigma1 = (x: number) => rotr(6, x) ^ rotr(11, x) ^ rotr(25, x)
  const gamma0 = (x: number) => rotr(7, x) ^ rotr(18, x) ^ (x >>> 3)
  const gamma1 = (x: number) => rotr(17, x) ^ rotr(19, x) ^ (x >>> 10)

  // Pre-processing: padding
  const bitLen = data.length * 8
  const padded = new Uint8Array(Math.ceil((data.length + 9) / 64) * 64)
  padded.set(data)
  padded[data.length] = 0x80
  const view = new DataView(padded.buffer)
  view.setUint32(padded.length - 4, bitLen, false)

  let [h0, h1, h2, h3, h4, h5, h6, h7] = [
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ]

  for (let offset = 0; offset < padded.length; offset += 64) {
    const W = new Array<number>(64)
    for (let t = 0; t < 16; t++) W[t] = view.getUint32(offset + t * 4, false)
    for (let t = 16; t < 64; t++)
      W[t] = (gamma1(W[t - 2]) + W[t - 7] + gamma0(W[t - 15]) + W[t - 16]) | 0

    let [a, b, c, d, e, f, g, h] = [h0, h1, h2, h3, h4, h5, h6, h7]
    for (let t = 0; t < 64; t++) {
      const T1 = (h + sigma1(e) + ch(e, f, g) + K[t] + W[t]) | 0
      const T2 = (sigma0(a) + maj(a, b, c)) | 0
      h = g
      g = f
      f = e
      e = (d + T1) | 0
      d = c
      c = b
      b = a
      a = (T1 + T2) | 0
    }
    h0 = (h0 + a) | 0
    h1 = (h1 + b) | 0
    h2 = (h2 + c) | 0
    h3 = (h3 + d) | 0
    h4 = (h4 + e) | 0
    h5 = (h5 + f) | 0
    h6 = (h6 + g) | 0
    h7 = (h7 + h) | 0
  }

  const result = new ArrayBuffer(32)
  const out = new DataView(result)
  ;[h0, h1, h2, h3, h4, h5, h6, h7].forEach((v, i) => out.setUint32(i * 4, v, false))
  return result
}

function base64UrlEncode(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let str = ""
  for (const b of bytes) {
    str += String.fromCharCode(b)
  }
  return btoa(str).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

// --- Cookie-based auth helpers (BFF pattern) ----------------------------

// Reads the non-HttpOnly CSRF token cookie that the server issues together
// with the HttpOnly session cookies. Returns "" if not set.
export function getCSRFToken(): string {
  const cookies = document.cookie ? document.cookie.split("; ") : []
  for (const c of cookies) {
    const eq = c.indexOf("=")
    if (eq < 0) continue
    if (c.slice(0, eq) === "vraxel_csrf") {
      return decodeURIComponent(c.slice(eq + 1))
    }
  }
  return ""
}

// Persisted localStorage keys that hold per-user UI state (selected
// workspace / namespace, last cloud template, in-flight transfers).
// They MUST be wiped at every authentication boundary so a different
// user's session cannot replay them.
const PERSISTED_USER_STATE_KEYS = [
  "vraxel-scope",
  "vraxel-workspace",
  "vraxel-transfers",
  "vraxel:cloud:lastImageRef",
]

function clearPersistedUserState() {
  for (const k of PERSISTED_USER_STATE_KEYS) {
    localStorage.removeItem(k)
  }
}

// Wipes everything that ties the current tab to the previous user: PKCE flow
// state, sessionStorage redirect target, persisted localStorage keys, and the
// in-memory zustand identity stores (registered via registerAuthBoundaryReset).
// Must run BEFORE any redirect so a stray re-render during navigation never
// paints user A's name or permissions for user B. Exported so the
// 401-refresh-failure path in `api/client.ts` can reset state before kicking
// off a fresh OIDC handshake.
export function clearLocalAuthState() {
  sessionStorage.removeItem("pkce_code_verifier")
  sessionStorage.removeItem("oidc_flow_pending")
  sessionStorage.removeItem(RETURN_TO_KEY)
  clearPersistedUserState()
  for (const reset of authBoundaryResets) reset()
}

// Paths that belong to the auth flow itself and must never be saved as a
// post-login redirect target (would produce loops or land the user on the
// login / callback / error page after signing in).
function isAuthFlowPath(pathname: string): boolean {
  return pathname === "/login" || pathname === "/auth/callback" || pathname === "/error"
}

function saveReturnTo() {
  const { pathname, search, hash } = window.location
  if (isAuthFlowPath(pathname)) return
  const returnTo = pathname + search + hash
  if (returnTo === "/") return
  sessionStorage.setItem(RETURN_TO_KEY, returnTo)
}

// Consume the saved post-login target. Validates same-origin relative path to
// prevent open-redirect (e.g. "//evil.com/x" which browsers treat as
// protocol-relative).
export function consumeReturnTo(): string {
  const raw = sessionStorage.getItem(RETURN_TO_KEY)
  sessionStorage.removeItem(RETURN_TO_KEY)
  if (raw && raw.startsWith("/") && !raw.startsWith("//")) {
    return raw
  }
  return "/"
}

// startAuthFlow kicks off the OIDC PKCE handshake. Pass `saveReturnTo: false`
// from logout: the current pathname belongs to the user being signed out, and
// must not be replayed to whoever logs in next.
export async function startAuthFlow(opts: { saveReturnTo?: boolean } = {}) {
  if (opts.saveReturnTo !== false) saveReturnTo()
  const codeVerifier = generateRandomString(64)
  sessionStorage.setItem("pkce_code_verifier", codeVerifier)
  sessionStorage.setItem("oidc_flow_pending", "1")

  const challengeBuffer = await sha256(codeVerifier)
  const codeChallenge = base64UrlEncode(challengeBuffer)

  const state = generateRandomString(16)

  const params = new URLSearchParams({
    response_type: "code",
    client_id: CLIENT_ID,
    scope: "openid profile email phone",
    state,
    code_challenge: codeChallenge,
    code_challenge_method: "S256",
  })

  window.location.href = `/oidc/authorize?${params.toString()}`
}

export async function loginWithCredentials(
  username: string,
  password: string,
  requestId: string,
): Promise<string> {
  const res = await fetch("/oidc/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify({ username, password, requestId }),
  })

  if (!res.ok) {
    const err = await res.json()
    throw new Error(err.error_description || "Login failed")
  }

  const data = await res.json()
  return data.redirectUri
}

// Exchange the authorization code for the session. The server sets HttpOnly
// cookies (vraxel_at, vraxel_rt, vraxel_csrf); the response body carries only the
// envelope (token_type, expires_in, scope) which we don't need to keep.
export async function exchangeCodeForTokens(code: string) {
  const codeVerifier = sessionStorage.getItem("pkce_code_verifier")
  if (!codeVerifier) {
    throw new Error("Missing PKCE code verifier")
  }

  const params = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    client_id: CLIENT_ID,
    code_verifier: codeVerifier,
  })

  const res = await fetch("/oidc/token", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    credentials: "include",
    body: params.toString(),
  })

  if (!res.ok) {
    const err = await res.json()
    throw new Error(err.error_description || "Token exchange failed")
  }

  // Drain the body so the browser releases the connection.
  await res.text()
  sessionStorage.removeItem("pkce_code_verifier")
}

// Rotate session cookies using the browser's existing vraxel_rt cookie.
// Returns true on success, false if the refresh cookie is missing/expired.
export async function refreshAccessToken(): Promise<boolean> {
  const params = new URLSearchParams({
    grant_type: "refresh_token",
    client_id: CLIENT_ID,
  })

  try {
    const csrfToken = getCSRFToken()
    const headers: Record<string, string> = {
      "Content-Type": "application/x-www-form-urlencoded",
    }
    if (csrfToken) headers["X-CSRF-Token"] = csrfToken

    const res = await fetch("/oidc/token", {
      method: "POST",
      headers,
      credentials: "include",
      body: params.toString(),
    })

    if (!res.ok) return false
    await res.text()
    return true
  } catch {
    return false
  }
}

export async function logout() {
  const csrfToken = getCSRFToken()
  try {
    await fetch("/oidc/logout", {
      method: "POST",
      headers: csrfToken ? { "X-CSRF-Token": csrfToken } : undefined,
      credentials: "include",
    })
  } catch {
    // best-effort; cookies may already be gone
  }
  clearLocalAuthState()
  await startAuthFlow({ saveReturnTo: false })
}
