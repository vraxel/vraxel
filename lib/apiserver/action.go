package apiserver

import (
	"encoding/json"
	"net/http"

	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/rest"
	"vraxel.io/vraxel/lib/runtime"
)

// ActionDef describes a mutation endpoint on a resource. Construct with
// Action / ActionAny / WSAction / RawAction — the constructors bind the
// typed handler at compile time, so ActionDef itself carries only a
// prebuilt factory (no `any`, no registration-time reflection).
type ActionDef struct {
	Name   string
	Method string
	// OnItem selects /{resource}/{id}/{name} (true, default for
	// constructors) vs the collection path /{resource}/{name}.
	OnItem bool
	// StatusCode overrides the 200 default for JSON actions (e.g. 202
	// for async agent install). Ignored by WS/Raw actions.
	StatusCode int
	// Sensitive skips request-body capture in the audit middleware
	// (password-carrying actions). Declarative replacement for v1's
	// hardcoded verb list in audit.go.
	Sensitive bool
	// Permission lists the codes checked by the route's authz link.
	// Required; Register panics when empty (v1 parity).
	Permission []string

	handler func(s *Server, m *routeMeta) http.Handler
}

// actionOpt tweaks an ActionDef produced by a constructor.
type actionOpt func(*ActionDef)

// OnCollection mounts the action on the collection path instead of the item path.
func OnCollection() actionOpt { return func(a *ActionDef) { a.OnItem = false } }

// WithStatus sets the success status code of a JSON action.
func WithStatus(code int) actionOpt { return func(a *ActionDef) { a.StatusCode = code } }

// MarkSensitive suppresses audit body capture for this action.
func MarkSensitive() actionOpt { return func(a *ActionDef) { a.Sensitive = true } }

// Action constructs a typed JSON action: the request body is decoded
// into Req, the response is serialized through the v1-compatible
// negotiated writer. Req and Resp are locked at compile time.
//
// GET/DELETE actions typically declare an empty Req and read caller
// options from ctx.Query.
func Action[Req, Resp any](name, method string, perm []string,
	fn func(Ctx, *Req) (*Resp, error), opts ...actionOpt) ActionDef {

	a := ActionDef{Name: name, Method: method, OnItem: true, Permission: perm}
	a.handler = func(s *Server, m *routeMeta) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, err := buildCtx(r, m)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			body, err := readBodyLimited(w, r, m.MaxBodyBytes)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			var req Req
			if len(body) > 0 {
				if err := json.Unmarshal(body, &req); err != nil {
					s.writeError(w, r, invalidJSONBody(err))
					return
				}
			}
			bindResponseHeaders(&ctx, r)
			resp, err := fn(ctx, &req)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			if resp == nil { // v1 parity: nil action result is a 204
				w.WriteHeader(http.StatusNoContent)
				return
			}
			status := m.StatusCode
			if status == 0 {
				status = http.StatusOK
			}
			s.writeActionResult(w, r, ctx, m, status, resp)
		})
	}
	for _, o := range opts {
		o(&a)
	}
	return a
}

// WSAction mounts a WebSocket action. The upgrade and the params-map
// contract are delegated to v1's HandleWebSocket so existing handlers
// (exec/console/watch/tail/progress) bridge unchanged.
func WSAction(name string, perm []string, h rest.WebSocketHandler, opts ...actionOpt) ActionDef {
	a := ActionDef{Name: name, Method: http.MethodGet, OnItem: true, Permission: perm}
	a.handler = func(s *Server, m *routeMeta) http.Handler {
		upgrade := rest.HandleWebSocket(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// v1 handlers read params via rest.PathParams(r); inject the
			// path values under their v1 names before delegating.
			upgrade(w, rest.WithPathParams(r, m.pathParams(r)))
		})
	}
	for _, o := range opts {
		o(&a)
	}
	return a
}

// RawAction mounts a raw streaming handler (uploads/downloads/proxies):
// full control of the request and response, no body limit, no JSON
// envelope — v1 RawHandlerFunc contract preserved.
func RawAction(name, method string, perm []string, fn rest.RawHandlerFunc, opts ...actionOpt) ActionDef {
	a := ActionDef{Name: name, Method: method, OnItem: true, Permission: perm}
	a.handler = func(s *Server, m *routeMeta) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fn(w, r, m.legacyParams(r))
		})
	}
	for _, o := range opts {
		o(&a)
	}
	return a
}

// verbHandler produces a list-style response for one read-only verb.
// rawID is the {id} segment; it stays a string because the bridged
// Lister contract allows non-numeric item ids.
type verbHandler func(s *Server, m *routeMeta, w http.ResponseWriter, r *http.Request, rawID string)

// VerbDef is a read-only view on a resource item, served at
// /{id}/{name}. Verbs inherit the parent's get permission.
type VerbDef struct {
	Name    string
	handler verbHandler
}

// Verb constructs a typed read-only view. The response is folded into
// the v1 list envelope with kind "{R}List". Typed verbs require a
// numeric item id; use VerbAny for a custom (non-list) wire shape.
func Verb[R any](name string, fn func(ctx Ctx, id int64, q list.Query) (*list.Result[R], error)) VerbDef {
	return VerbDef{Name: name, handler: func(s *Server, m *routeMeta, w http.ResponseWriter, r *http.Request, rawID string) {
		ctx, err := buildCtx(r, m) // parses ctx.ID or 400s
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		q := parseListQuery(r, m.MaxPageSize)
		res, err := fn(ctx, ctx.ID, q)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		s.writeActionResult(w, r, ctx, m, http.StatusOK, newListEnvelope(defaultListKind[R](), res))
	}}
}

// ActionAny is the typed escape hatch for actions that don't fit
// Action[Req,Resp]: no-body GETs, query-driven reads (ctx.Query),
// FileResponse downloads, polymorphic responses. The raw body passes
// through; the returned value serializes as-is (FileResponse handled).
func ActionAny(name, method string, perm []string, fn func(ctx Ctx, body []byte) (any, error), opts ...actionOpt) ActionDef {
	d := ActionDef{
		Name: name, Method: method, Permission: perm, OnItem: true,
		handler: func(s *Server, m *routeMeta) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, err := buildCtx(r, m)
				if err != nil {
					s.writeError(w, r, err)
					return
				}
				body, err := readBodyLimited(w, r, m.MaxBodyBytes)
				if err != nil {
					s.writeError(w, r, err)
					return
				}
				// Bind the ResponseHeaders conduit like Action/CreateAny: a
				// side-effectful *Any action may publish response headers;
				// writeActionResult flushes them, but without the bind they
				// are silently dropped.
				bindResponseHeaders(&ctx, r)
				res, err := fn(ctx, body)
				if err != nil {
					s.writeError(w, r, err)
					return
				}
				status := m.StatusCode
				if status == 0 {
					status = http.StatusOK
				}
				s.writeActionResult(w, r, ctx, m, status, res)
			})
		},
	}
	for _, o := range opts {
		o(&d)
	}
	return d
}

// VerbAny is the typed escape hatch for verbs whose wire shape is NOT
// the standard list envelope (custom {data}/{nodes,edges} responses,
// non-list objects). The returned value serializes as-is through the
// negotiated writer; apiVersion is stamped when it carries a TypeMeta.
func VerbAny(name string, fn func(ctx Ctx, id int64, q list.Query) (any, error)) VerbDef {
	return VerbDef{Name: name, handler: func(s *Server, m *routeMeta, w http.ResponseWriter, r *http.Request, rawID string) {
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		res, err := fn(ctx, ctx.ID, parseListQuery(r, m.MaxPageSize))
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		s.writeActionResult(w, r, ctx, m, http.StatusOK, res)
	}}
}

// stampAPIVersion sets apiVersion on the response when the value carries
// a TypeMeta — the opportunistic bridge that keeps v1 response bytes
// while T is no longer *required* to implement any interface.
func stampAPIVersion(v any, apiVersion string) {
	if obj, ok := v.(runtime.Object); ok && obj != nil {
		if tm := obj.GetTypeMeta(); tm != nil {
			tm.APIVersion = apiVersion
		}
	}
}
