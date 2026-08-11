package filters

import (
	"crypto/subtle"
	"fmt"
	"net/http"

	"vraxel.io/vraxel/lib/oidc"
)

// preSessionPaths are OIDC endpoints that establish a new session. They must
// never be gated by CSRF — a client reaching them by definition has no live
// session on this server, even if the browser still holds stale vraxel_at /
// vraxel_csrf cookies from a prior session (e.g. after server-side session
// invalidation, DB reset, or signing-key rotation).
var preSessionPaths = map[string]struct{}{
	"/oidc/login":    {},
	"/oidc/register": {},
	"/oidc/token":    {},
}

// WithCSRF returns middleware that enforces the double-submit cookie pattern
// for browser (cookie-authenticated) requests. External API clients that
// authenticate via Authorization: Bearer and do not carry the session
// cookies are passed through unchecked.
//
// Rules:
//   - GET / HEAD / OPTIONS are always allowed (safe methods).
//   - Pre-session OIDC endpoints (see preSessionPaths) are always allowed;
//     they bootstrap the session and cannot require a valid CSRF token.
//   - Enforcement requires BOTH vraxel_at (active session) AND vraxel_csrf
//     (double-submit token). If either is absent, skip: the request either
//     has no session to protect or it's an external API client on the
//     Bearer path.
//   - Otherwise, X-CSRF-Token header must equal the vraxel_csrf cookie value.
func WithCSRF() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveCSRF(next, w, r)
		})
	}
}

// serveCSRF enforces the double-submit cookie check for a single request,
// lifted verbatim from the WithCSRF closure body. Returns by calling next or
// writing csrfError; never both.
func serveCSRF(next http.Handler, w http.ResponseWriter, r *http.Request) {
	expected, enforce := csrfExpectedToken(r)
	if !enforce {
		next.ServeHTTP(w, r)
		return
	}

	header := r.Header.Get("X-CSRF-Token")
	if header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(expected)) != 1 {
		csrfError(w)
		return
	}
	next.ServeHTTP(w, r)
}

// csrfExpectedToken decides whether CSRF enforcement applies to r. It returns
// (expectedToken, true) when both the session cookie and the double-submit
// cookie are present and the request is neither a safe method nor a
// pre-session path; otherwise ("", false) meaning the request is passed
// through unchecked. Each skip branch is preserved verbatim from WithCSRF.
func csrfExpectedToken(r *http.Request) (string, bool) {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return "", false
	}

	if _, ok := preSessionPaths[r.URL.Path]; ok {
		return "", false
	}

	at, err := r.Cookie(oidc.CookieAccessToken)
	if err != nil || at.Value == "" {
		return "", false
	}

	c, err := r.Cookie(oidc.CookieCSRFToken)
	if err != nil || c.Value == "" {
		return "", false
	}

	return c.Value, true
}

func csrfError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","message":"CSRF token missing or invalid"}`)
}
