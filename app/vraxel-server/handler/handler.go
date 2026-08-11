package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/oidc"
	"vraxel.io/vraxel/lib/rest/filters"
)

// APIServerConfig holds the configuration for creating an API server handler.
type APIServerConfig struct {
	Name         string
	OIDCProvider *oidc.Provider
	AuditLogger  audit.Logger // nil = no audit logging

	// Server serves every API module; routes carry their own authz/audit
	// chains composed at registration.
	Server *apiserver.Server
}

// APIServerHandler is the /api handler: the apiserver wrapped in the
// global middleware chain.
type APIServerHandler struct {
	FullHandlerChain http.Handler
}

func NewAPIServerHandler(cfg APIServerConfig) *APIServerHandler {
	return &APIServerHandler{FullHandlerChain: buildChain(cfg)}
}

// ServeHTTP makes it an http.Handler.
func (a *APIServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.FullHandlerChain.ServeHTTP(w, r)
}

// RootHandlerConfig holds the components needed by the top-level request router.
type RootHandlerConfig struct {
	APIHandler      *APIServerHandler
	OIDCMux         http.Handler
	OpenAPISpec     []byte
	FrontendFS      fs.FS
	ReadinessChecks []ReadinessCheck // dependencies probed by /readyz and /healthz
}

// rootRouter holds the precomputed handlers and config the top-level
// request router dispatches against.
type rootRouter struct {
	cfg           RootHandlerConfig
	oidcMux       http.Handler
	staticHandler http.Handler
	openapiETag   string
}

// NewRootHandler creates the top-level request handler that routes between
// OIDC public endpoints, OpenAPI spec, API handler, and frontend static files.
func NewRootHandler(cfg RootHandlerConfig) func(http.ResponseWriter, *http.Request) bool {
	rt := &rootRouter{cfg: cfg}

	// Apply CSRF protection to OIDC endpoints too. The middleware only enforces
	// when vraxel_csrf cookie is present (browser session), so /oidc/authorize,
	// /oidc/login (pre-session) and first-time /oidc/token are unaffected.
	if cfg.OIDCMux != nil {
		rt.oidcMux = filters.WithCSRF()(cfg.OIDCMux)
	}

	if cfg.FrontendFS != nil {
		rt.staticHandler = http.FileServer(http.FS(cfg.FrontendFS))
	}

	// Compute the OpenAPI spec ETag once at startup. The spec is fixed
	// for the process lifetime (built into the binary or loaded at boot
	// before NewRootHandler runs), so the fingerprint is stable per
	// deploy and a per-request hash would be wasted CPU. 8 bytes of
	// SHA-256 (16 hex chars) is comfortably collision-resistant for a
	// single-resource ETag.
	if len(cfg.OpenAPISpec) > 0 {
		sum := sha256.Sum256(cfg.OpenAPISpec)
		rt.openapiETag = `"` + hex.EncodeToString(sum[:8]) + `"`
	}

	return rt.route
}

// route tries each route group in priority order, returning as soon as one
// handles the request; an unhandled request falls through to the SPA frontend.
func (rt *rootRouter) route(w http.ResponseWriter, r *http.Request) bool {
	urlPath := r.URL.Path

	// Baseline security headers on every response. CSP is intentionally
	// absent: vite injects inline styles, so a policy would need
	// unsafe-inline and add noise without adding protection.
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

	if rt.routeHealth(w, r, urlPath) {
		return true
	}
	if rt.routePublic(w, r, urlPath) {
		return true
	}

	// API requests go through the API handler (with auth middleware).
	// no-store: API responses are JSON RPC-style payloads tied to
	// authenticated session state. Browser / proxy caching of them
	// would let one user see another's stale data, leak data after
	// logout, or serve stale 200 OKs after the underlying state
	// changed. Inner handlers that need to opt out (long-poll, SSE)
	// can overwrite Cache-Control before flushing.
	if strings.HasPrefix(urlPath, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
		rt.cfg.APIHandler.ServeHTTP(w, r)
		return true
	}

	// Serve frontend static files; fallback to index.html for SPA routes
	if rt.staticHandler != nil {
		serveFrontend(w, r, rt.cfg.FrontendFS, rt.staticHandler)
	}
	return true
}

// routeHealth handles the liveness / readiness probes. They must run
// before any other branch so the SPA fallback never swallows them
// (no auth, no audit, no CSRF).
func (rt *rootRouter) routeHealth(w http.ResponseWriter, r *http.Request, urlPath string) bool {
	switch urlPath {
	case "/livez":
		writeLivez(w)
		return true
	case "/readyz", "/healthz":
		writeReadiness(w, r, rt.cfg.ReadinessChecks)
		return true
	}
	return false
}

// routePublic handles the unauthenticated public surface served before
// the API auth chain: the OIDC public mux and the OpenAPI spec.
func (rt *rootRouter) routePublic(w http.ResponseWriter, r *http.Request, urlPath string) bool {
	// Route OIDC endpoints to public mux (no auth middleware)
	if rt.oidcMux != nil && (strings.HasPrefix(urlPath, "/.well-known/") || strings.HasPrefix(urlPath, "/oidc/")) {
		rt.oidcMux.ServeHTTP(w, r)
		return true
	}

	// Serve OpenAPI spec (no auth).
	// SECURITY: This endpoint is served without authentication. The OpenAPI spec
	// exposes the full API surface (paths, parameters, schemas) which may help
	// attackers understand the system. In production deployments where the spec
	// should not be publicly accessible, set OpenAPISpec to nil in the config
	// or gate access behind a reverse proxy rule.
	if urlPath == "/docs/openapi.json" && rt.cfg.OpenAPISpec != nil {
		return rt.serveOpenAPISpec(w, r)
	}
	return false
}

// serveOpenAPISpec writes the OpenAPI spec with ETag revalidation.
func (rt *rootRouter) serveOpenAPISpec(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Content-Type", "application/json")
	// no-cache (not no-store): forces revalidation, but the body
	// is safe to keep in disk cache and serve via 304 when the
	// ETag matches. The ETag was fingerprinted once at startup
	// from the immutable spec bytes, so this round-trip costs a
	// header compare and nothing else.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", rt.openapiETag)
	if r.Header.Get("If-None-Match") == rt.openapiETag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	_, _ = w.Write(rt.cfg.OpenAPISpec)
	return true
}

// buildChain wraps the apiserver in the global middleware chain
// (innermost -> outermost): routes -> WithCSRF -> WithAuthentication ->
// WithRequestLog. Route-level authorization and audit live inside the
// apiserver, composed per-route at registration.
func buildChain(cfg APIServerConfig) http.Handler {
	handler := cfg.Server.Handler()
	handler = filters.WithCSRF()(handler)
	if cfg.OIDCProvider != nil {
		handler = filters.WithAuthentication(cfg.OIDCProvider)(handler)
	}
	handler = filters.WithRequestLog(handler)
	return handler
}

// serveFrontend serves static files from the embedded frontend.
// If the requested file exists, it is served directly.
// Otherwise, index.html is served to support SPA client-side routing.
//
// Cache-Control follows the standard SPA hashed-asset pattern:
//   - vite emits assets/* with content-hashed filenames; once published,
//     a given hash always resolves to the exact same bytes, so the
//     browser may keep them forever (immutable + 1 year max-age).
//   - index.html is the only mutable entry point: it carries the
//     <script src=".../[hash].js"> references that change on every
//     deploy. It must NOT be heuristically cached, or users keep
//     hitting the previous deploy's chunk references after upgrade.
//     no-cache forces revalidation; the response body itself can
//     still be served from cache via 304 when ETag matches.
//
// Without these headers, http.FileServer sets only Last-Modified +
// ETag, leaving everything to the browser's heuristic cache (~10%
// of file age). That makes post-deploy UI updates land at random
// times per user and forces hard refresh as the only reliable way
// to get the latest bundle.
func serveFrontend(w http.ResponseWriter, r *http.Request, distFS fs.FS, staticHandler http.Handler) {
	filePath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if filePath == "" {
		filePath = "index.html"
	}

	if f, err := distFS.Open(filePath); err == nil {
		_ = f.Close()
		setFrontendCacheHeaders(w, filePath)
		staticHandler.ServeHTTP(w, r)
		return
	}

	// Extensionless alias for secondary HTML entries: /api-docs ->
	// api-docs.html. Checked before the SPA fallback so multi-entry pages
	// keep clean URLs without being swallowed by index.html.
	if !strings.Contains(path.Base(filePath), ".") {
		htmlPath := filePath + ".html"
		if f, err := distFS.Open(htmlPath); err == nil {
			_ = f.Close()
			r.URL.Path = "/" + htmlPath
			setFrontendCacheHeaders(w, htmlPath)
			staticHandler.ServeHTTP(w, r)
			return
		}
	}

	// SPA fallback: serve index.html for all non-file routes
	r.URL.Path = "/"
	setFrontendCacheHeaders(w, "index.html")
	staticHandler.ServeHTTP(w, r)
}

// setFrontendCacheHeaders applies the SPA cache policy described on
// serveFrontend. assets/ entries are content-hashed by vite -> safe to
// pin; everything else (index.html, favicon, robots, ...) is mutable
// per deploy -> must revalidate.
func setFrontendCacheHeaders(w http.ResponseWriter, filePath string) {
	if strings.HasPrefix(filePath, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}
