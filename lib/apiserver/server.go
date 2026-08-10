package apiserver

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"vraxel.io/vraxel/lib/audit"
	"vraxel.io/vraxel/lib/rest"
	"vraxel.io/vraxel/lib/rest/filters"
	"vraxel.io/vraxel/lib/runtime"
)

// Config wires the server's external dependencies. Every field is
// optional: a zero Config yields an unauthorized, unaudited server that
// still routes and serializes correctly (unit-test parity).
type Config struct {
	// Serializer produces the negotiated encoders/decoders. Defaults to
	// runtime.NewCodecFactory().
	Serializer runtime.NegotiatedSerializer
	// Checker enforces RBAC. Nil disables authorization (dev mode).
	Checker filters.PermissionChecker
	// AuditLogger receives write-operation audit events. Nil disables.
	AuditLogger audit.Logger
	// SkipPermCodes lists permission codes any authenticated user may
	// exercise (e.g. filters.PermListCode).
	SkipPermCodes []string
}

// Server owns the route table. It is an http.Handler the root router
// mounts under /api. Routing is the stdlib ServeMux — duplicate or
// ambiguous patterns panic at registration, which is exactly the
// fail-fast we want.
type Server struct {
	mux        *http.ServeMux
	serializer runtime.NegotiatedSerializer
	checker    filters.PermissionChecker
	auditLog   audit.Logger
	skipCodes  map[string]bool

	// resources: group → relPath ("hosts", "hosts/nics") → registration.
	resources map[string]map[string]*resourceReg
	// ordered preserves registration order so derived artifacts
	// (permission records) are deterministic — map iteration is not.
	ordered []*resourceReg
	// modules preserves module registration order for logging/introspection.
	modules []string
	// patterns records every registered pattern for tests / debugging.
	patterns []string
}

// New creates an empty server.
func New(cfg Config) *Server {
	ns := cfg.Serializer
	if ns == nil {
		ns = runtime.NewCodecFactory()
	}
	skip := make(map[string]bool, len(cfg.SkipPermCodes))
	for _, c := range cfg.SkipPermCodes {
		skip[c] = true
	}
	return &Server{
		mux:        http.NewServeMux(),
		serializer: ns,
		checker:    cfg.Checker,
		auditLog:   cfg.AuditLogger,
		skipCodes:  skip,
		resources:  make(map[string]map[string]*resourceReg),
	}
}

// Handler returns the mux for mounting under the root router. Unmatched
// paths 404 — fail-closed is structural, not a discipline.
func (s *Server) Handler() http.Handler { return s.mux }

// ServeHTTP makes the server directly mountable.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Modules returns the group names registered on this server, in
// registration order — the strangler routing table's source of truth.
func (s *Server) Modules() []string { return append([]string(nil), s.modules...) }

// Patterns returns every registered route pattern (tests / debugging).
func (s *Server) Patterns() []string { return append([]string(nil), s.patterns...) }

// Public registers a handler outside the resource model — health,
// discovery, spec endpoints. Being public is a declaration here, never
// the side effect of a parse failure.
func (s *Server) Public(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
	s.patterns = append(s.patterns, pattern)
}

// handle composes the route-level chain at registration time:
// (outer) audit → authz → meta-inject → handler (inner). Audit wraps
// authz so permission denials land in the audit log, matching v1's
// middleware order.
func (s *Server) handle(pattern string, m *routeMeta, h http.Handler) {
	chain := injectMeta(m, h)
	chain = s.authzMiddleware(m, chain)
	chain = s.auditMiddleware(m, chain)
	s.mux.Handle(pattern, chain)
	s.patterns = append(s.patterns, pattern)
}

// addResource records a registration; a duplicate path panics
// (fail-fast) unless the scope sets are disjoint — iam registers "users"
// three times with per-scope storages, and disjoint scopes mean disjoint
// URLs. The path index keeps the first registration for Parent lookups;
// resources that need children must register before their same-named
// scoped siblings.
func (s *Server) addResource(reg *resourceReg) {
	group := s.resources[reg.Group]
	if group == nil {
		group = make(map[string]*resourceReg)
		s.resources[reg.Group] = group
		s.modules = append(s.modules, reg.Group)
	}
	if existing, dup := group[reg.RelPath]; dup {
		overlap := existing.Scopes & reg.Scopes
		for _, other := range s.ordered {
			if other.Group == reg.Group && other.RelPath == reg.RelPath {
				overlap |= other.Scopes & reg.Scopes
			}
		}
		if overlap != 0 {
			panic(fmt.Sprintf("apiserver: duplicate resource %q in group %q (overlapping scopes)", reg.RelPath, reg.Group))
		}
	} else {
		group[reg.RelPath] = reg
	}
	s.ordered = append(s.ordered, reg)
}

// resolveParent looks up a Parent reference within the same group.
func (s *Server) resolveParent(group, parent string) *resourceReg {
	if parent == "" {
		return nil
	}
	reg := s.resources[group][parent]
	if reg == nil {
		panic(fmt.Sprintf("apiserver: resource parent %q not registered in group %q (register parents first)", parent, group))
	}
	return reg
}

// ─── permission records ───

// PermRecord mirrors iam's PermissionUpsertInput shape; the install
// chain converts and feeds it to the same PermissionStore.SyncModule the
// v1 sync uses, so migrated modules keep byte-identical permission rows.
type PermRecord struct {
	Code        string
	Method      string
	Path        string
	Scope       string
	Description string
}

// verbMethods matches v1 rbac_sync exactly.
var verbMethods = map[string]string{
	"list":             "GET",
	"get":              "GET",
	"create":           "POST",
	"update":           "PUT",
	"patch":            "PATCH",
	"delete":           "DELETE",
	"deleteCollection": "DELETE",
}

// scopesUpTo matches v1: permissions fan out from the natural scope up
// to platform.
func scopesUpTo(natural string) []string {
	switch natural {
	case "namespace":
		return []string{"namespace", "workspace", "platform"}
	case "workspace":
		return []string{"workspace", "platform"}
	default:
		return []string{"platform"}
	}
}

// rbacIDParam reproduces v1 rbac_sync.idParam — a SIMPLER singularizer
// than the installer's defaultIDParam (no es-suffix handling). Permission
// record paths must diff clean against v1 rows, so the quirk is kept.
func rbacIDParam(name string) string {
	return strings.TrimSuffix(name, "s") + "Id"
}

// PermRecordsByModule derives the permission rows for every registered
// resource, keyed by module. Computed on demand with whole-module
// knowledge so action targets already covered by resource verbs are
// skipped exactly like v1 generatePermissions.
func (s *Server) PermRecordsByModule() map[string][]PermRecord {
	// Registration-ordered per-group reg lists keep the output
	// deterministic for the migration harness's permission snapshot diff.
	byGroup := make(map[string][]*resourceReg, len(s.resources))
	for _, reg := range s.ordered {
		byGroup[reg.Group] = append(byGroup[reg.Group], reg)
	}

	out := make(map[string][]PermRecord, len(s.resources))
	for _, group := range s.modules {
		var records []PermRecord
		covered := make(map[string]bool)

		regs := byGroup[group]
		// Resource-verb records first (establishes coverage), then actions.
		for _, reg := range regs {
			if len(reg.PermTarget) > 0 {
				continue // borrowed permissions generate nothing (v1 parity)
			}
			natural := reg.permScope().name()
			base := reg.permBasePath()
			for _, verb := range reg.Verbs {
				code := permCode(reg.Group, reg.nameChain(), verb)
				covered[code] = true
				path := base
				switch verb {
				case "list", "create", "deleteCollection":
				default:
					path += "/{" + rbacIDParam(reg.Name) + "}"
				}
				for _, scope := range scopesUpTo(natural) {
					records = append(records, PermRecord{Code: code, Method: verbMethods[verb], Path: path, Scope: scope})
				}
			}
		}
		for _, reg := range regs {
			natural := reg.permScope().name()
			base := reg.permBasePath()
			seen := make(map[string]bool)
			for _, a := range reg.ActionDefs {
				if seen[a.Name] {
					continue // multi-method actions share permissions (v1 parity)
				}
				seen[a.Name] = true
				path := base + "/{" + rbacIDParam(reg.Name) + "}/" + a.Name
				if !a.OnItem {
					path = base + "/" + a.Name
				}
				for _, target := range a.Permission {
					if covered[target] {
						continue
					}
					for _, scope := range scopesUpTo(natural) {
						records = append(records, PermRecord{Code: target, Method: a.Method, Path: path, Scope: scope})
					}
				}
			}
		}
		out[moduleName(group)] = records
	}
	return out
}

// permBasePath builds the canonical (platform-level) URL prefix used in
// permission rows, matching v1 buildAPIPath for the shortest entry.
func (rr *resourceReg) permBasePath() string {
	var b strings.Builder
	b.WriteString(rr.BasePath)
	for _, anc := range rr.ancestors() {
		b.WriteString("/")
		b.WriteString(anc.Name)
		b.WriteString("/{")
		b.WriteString(rbacIDParam(anc.Name))
		b.WriteString("}")
	}
	b.WriteString("/")
	b.WriteString(rr.Name)
	return b.String()
}

// ─── response plumbing shared by wrappers ───

// writeError funnels every error through v1's negotiated error writer:
// StatusError keeps its code and JSON shape, everything else becomes the
// same 500 InternalError v1 produced.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	rest.ErrorNegotiated(w, r, s.serializer, err)
}

// writeObject stamps apiVersion and serializes through the negotiated
// writer — the single home of v1's setAPIVersion+WriteObjectNegotiated pair.
func (s *Server) writeObject(w http.ResponseWriter, r *http.Request, m *routeMeta, status int, obj runtime.Object) {
	stampAPIVersion(obj, m.APIVersion)
	rest.WriteObjectNegotiated(s.serializer, w, r, status, obj)
}

// writeActionResult serializes a typed action/verb result. Values that
// carry a TypeMeta go through the negotiated writer (apiVersion stamped);
// nil means 204 like v1.
func (s *Server) writeActionResult(w http.ResponseWriter, r *http.Request, ctx Ctx, m *routeMeta, status int, v any) {
	// The *Any escape hatches return `any`, so a handler returning a typed
	// nil pointer -- (*Foo)(nil), nil -- slips past `v == nil` (the
	// interface is non-nil) yet satisfies runtime.Object, and writeObject
	// would panic in GetTypeMeta's nil-receiver deref. Normalize it to
	// no-content, matching asObject on the typed path.
	if v == nil || isNilPointer(v) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	rest.ApplyResponseHeaders(ctx, w)
	if fr, ok := v.(*rest.FileResponse); ok {
		rest.WriteFile(w, status, fr)
		return
	}
	if obj, ok := v.(runtime.Object); ok {
		s.writeObject(w, r, m, status, obj)
		return
	}
	rest.WriteRawJSON(w, status, v)
}

// isNilPointer reports whether v is a non-nil interface wrapping a nil
// pointer -- the typed-nil an `any`-returning escape hatch can hand back.
func isNilPointer(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

// ─── meta context plumbing ───

type metaCtxKey struct{}

// injectMeta exposes the routeMeta to the innermost wrapper handlers.
func injectMeta(m *routeMeta, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), metaCtxKey{}, m)))
	})
}

// metaOf returns the routeMeta injected by the registration chain.
func metaOf(r *http.Request) *routeMeta {
	m, _ := r.Context().Value(metaCtxKey{}).(*routeMeta)
	return m
}
