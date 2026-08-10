package filters

import (
	"fmt"
	"net/http"

	"vraxel.io/vraxel/lib/oidc"
)

// WithAuthentication returns middleware that validates the access token
// carried in the vraxel_at HttpOnly cookie (BFF pattern). Requests
// without a valid session receive 401 Unauthorized.
func WithAuthentication(provider *oidc.Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(oidc.CookieAccessToken)
			if err != nil || c.Value == "" {
				authError(w, "authentication required")
				return
			}
			userID, err := provider.VerifyBearerToken(c.Value)
			if err != nil {
				authError(w, "invalid or expired token")
				return
			}

			username, err := provider.CheckUserActive(r.Context(), userID)
			if err != nil {
				authError(w, "account is not active")
				return
			}

			r = oidc.WithUserID(r, userID)
			r = oidc.WithUsername(r, username)
			next.ServeHTTP(w, r)
		})
	}
}

func authError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Unauthorized","message":"%s"}`, message)
}
