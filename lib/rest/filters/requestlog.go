package filters

import (
	"net/http"
	"strings"
)

// WithRequestLog redacts sensitive query parameters (e.g., token) from the request URI.
// Request logging is handled by the audit middleware; this filter only sanitizes.
func WithRequestLog(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.RequestURI, "token=") {
			q := r.URL.Query()
			if q.Has("token") {
				q.Set("token", "[REDACTED]")
				r.RequestURI = r.URL.Path + "?" + q.Encode()
			}
		}
		handler.ServeHTTP(w, r)
	})
}
