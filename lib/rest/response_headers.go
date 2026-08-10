package rest

import (
	"context"
	"net/http"
	"sync"
)

// ResponseHeaders is the conduit by which handlers (create / update /
// patch / list / get / action) can publish HTTP response headers without
// having direct access to http.ResponseWriter.
//
// Pattern:
//
//	hdrs := rest.ResponseHeadersFromContext(ctx)
//	if hdrs != nil {
//	    hdrs.Set("X-Vraxel-Trace-Id", traceID)
//	}
//
// The framework's per-route handler installs a fresh ResponseHeaders
// into the context before calling the storage method, then flushes
// whatever the storage set onto the real http.Header just before
// writing the response body. Headers set by storage win over framework
// defaults — operators can override Content-Type etc. if they need to.
type ResponseHeaders struct {
	mu sync.Mutex
	m  http.Header
}

// Set stages a header pair. Like http.Header.Set, repeated calls with
// the same key replace the previous value rather than appending.
func (r *ResponseHeaders) Set(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = http.Header{}
	}
	r.m.Set(key, value)
}

// snapshot returns a copy of the staged headers; the framework calls
// this immediately before WriteHeader so the caller can no longer
// mutate the live header set after response start.
func (r *ResponseHeaders) snapshot() http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.m) == 0 {
		return nil
	}
	out := make(http.Header, len(r.m))
	for k, v := range r.m {
		out[k] = append([]string(nil), v...)
	}
	return out
}

type responseHeadersCtxKey struct{}

// WithResponseHeaders binds a fresh ResponseHeaders to the context.
// Called by the framework's per-route handler.
func WithResponseHeaders(ctx context.Context, hdrs *ResponseHeaders) context.Context {
	return context.WithValue(ctx, responseHeadersCtxKey{}, hdrs)
}

// ResponseHeadersFromContext retrieves the ResponseHeaders previously
// bound by WithResponseHeaders. Returns nil if the context was not
// initialized — storage methods MUST handle nil to stay usable from
// unit tests that don't go through the REST handler chain.
func ResponseHeadersFromContext(ctx context.Context) *ResponseHeaders {
	v, _ := ctx.Value(responseHeadersCtxKey{}).(*ResponseHeaders)
	return v
}

// applyResponseHeaders copies any staged headers from ctx onto w.
// Called by the framework just before WriteHeader / Write so headers
// land in the response. No-op when nothing was staged.
func applyResponseHeaders(ctx context.Context, w http.ResponseWriter) {
	hdrs := ResponseHeadersFromContext(ctx)
	if hdrs == nil {
		return
	}
	snap := hdrs.snapshot()
	for k, vs := range snap {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
}

// ApplyResponseHeaders is the exported form of applyResponseHeaders for
// use by the v2 apiserver handler wrappers (lib/apiserver), which share
// the ResponseHeaders context conduit with the v1 installer.
func ApplyResponseHeaders(ctx context.Context, w http.ResponseWriter) {
	applyResponseHeaders(ctx, w)
}
