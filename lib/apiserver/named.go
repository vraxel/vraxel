package apiserver

import (
	"encoding/json"
	"net/http"

	"vraxel.io/vraxel/lib/list"
)

// NamedOps is the operation set for resources whose item key is a name
// string rather than a numeric id — K8s objects addressed by object
// name, pods by pod name, settings by key. The handlers receive the raw
// path segment; everything else matches Ops[T].
type NamedOps[T any] struct {
	List   func(ctx Ctx, q list.Query) (*list.Result[T], error)
	Get    func(ctx Ctx, name string) (*T, error)
	Create func(ctx Ctx, in *T) (*T, error)
	Update func(ctx Ctx, name string, in *T) (*T, error)
	Delete func(ctx Ctx, name string) error

	// Patch receives the raw JSON body, same rationale as Ops.Patch.
	Patch func(ctx Ctx, name string, patch json.RawMessage) (*T, error)

	// ListAny is the polymorphic-list escape hatch (custom envelopes,
	// e.g. a load-bearing resourceVersion field). Mutually exclusive
	// with List.
	ListAny func(ctx Ctx, q list.Query) (any, error)
	// DeleteCollectionAny serves DELETE on the collection path with the
	// raw body (name-keyed batch deletes decode their own id list).
	DeleteCollectionAny func(ctx Ctx, body json.RawMessage) (any, error)
}

func (o NamedOps[T]) verbs() []string {
	var v []string
	if o.List != nil || o.ListAny != nil {
		v = append(v, "list")
	}
	if o.Get != nil {
		v = append(v, "get")
	}
	if o.Create != nil {
		v = append(v, "create")
	}
	if o.Update != nil {
		v = append(v, "update")
	}
	if o.Patch != nil {
		v = append(v, "patch")
	}
	if o.Delete != nil {
		v = append(v, "delete")
	}
	if o.DeleteCollectionAny != nil {
		v = append(v, "deleteCollection")
	}
	return v
}

// NamedDef describes a string-keyed resource. Identical to ResourceDef
// except the item segment passes through as Ctx.Name instead of being
// parsed into Ctx.ID.
type NamedDef[T any] struct {
	Group   string
	Version string // "" = v1
	Name    string
	Scopes  Scope
	Parent  string

	// IDParam names the item path parameter (required — string-keyed
	// resources always pinned it in v1, e.g. "objectName", "podName").
	IDParam string

	Ops NamedOps[T]

	Actions []ActionDef
	Verbs   []VerbDef

	PermissionTargets []string
	MaxPageSize       int32
	MaxBodyBytes      int64
	Sensitive         bool
	ListKind          string
}

// RegisterNamed registers a string-keyed resource. Routes, permission
// records and chains derive exactly like Register; the only difference
// is item-key handling (no int64 parse, Ctx.Name carries the segment).
func RegisterNamed[T any](s *Server, def NamedDef[T]) {
	if def.Name == "" {
		panic("apiserver: NamedDef.Name is required")
	}
	if def.IDParam == "" {
		panic("apiserver: NamedDef.IDParam is required for string-keyed resources")
	}
	if def.Scopes == 0 {
		def.Scopes = ScopePlatform
	}

	shadow := ResourceDef[T]{
		Group: def.Group, Version: def.Version, Name: def.Name,
		Scopes: def.Scopes, Parent: def.Parent,
	}

	parent := s.resolveParent(def.Group, def.Parent)
	relPath := def.Name
	if parent != nil {
		relPath = parent.RelPath + "/" + def.Name
	}
	reg := &resourceReg{
		Group: def.Group, Name: def.Name, RelPath: relPath, Parent: parent,
		Scopes: def.Scopes, PermTarget: def.PermissionTargets,
		Verbs:      def.Ops.verbs(),
		ActionDefs: def.Actions,
		APIVersion: shadow.apiVersion(), BasePath: shadow.basePath(),
		MaxPageSize: def.MaxPageSize, MaxBodyBytes: def.MaxBodyBytes,
		Sensitive: def.Sensitive, IDParam: def.IDParam,
		StringID: true,
	}
	if len(def.Verbs) > 0 && def.Ops.Get == nil {
		reg.Verbs = append(reg.Verbs, "get")
	}
	s.addResource(reg)

	for _, lv := range def.Scopes.levels() {
		registerNamedScoped(s, def, reg, lv)
	}
}

func registerNamedScoped[T any](s *Server, def NamedDef[T], reg *resourceReg, lv Scope) {
	base := reg.BasePath + lv.pathPrefix() + "/" + reg.parentPattern() + def.Name
	item := base + "/{" + sanitizeWildcard(reg.idParam()) + "}"
	kind := def.ListKind
	if kind == "" {
		kind = defaultListKind[T]()
	}
	label := singularLabel(def.Name)

	writeMeta := func(verb string, withID bool) *routeMeta {
		m := reg.meta(s, lv, verb, withID)
		m.Sensitive = def.Sensitive
		return m
	}

	if def.Ops.ListAny != nil {
		s.handle("GET "+base, reg.meta(s, lv, "list", false), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := metaOf(r)
			ctx, err := buildCtx(r, m)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			res, err := def.Ops.ListAny(ctx, parseListQuery(r, def.MaxPageSize))
			if err != nil {
				s.writeError(w, r, mapDomain(err, label))
				return
			}
			s.writeActionResult(w, r, ctx, m, http.StatusOK, res)
		}))
	}
	if def.Ops.DeleteCollectionAny != nil {
		s.handle("DELETE "+base, reg.meta(s, lv, "deleteCollection", false), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := metaOf(r)
			ctx, err := buildCtx(r, m)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			body, err := readBodyLimited(w, r, def.MaxBodyBytes)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			res, err := def.Ops.DeleteCollectionAny(ctx, body)
			if err != nil {
				s.writeError(w, r, mapDomain(err, label))
				return
			}
			s.writeActionResult(w, r, ctx, m, http.StatusOK, res)
		}))
	}
	if def.Ops.List != nil {
		s.handle("GET "+base, reg.meta(s, lv, "list", false), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := metaOf(r)
			ctx, err := buildCtx(r, m)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			res, err := def.Ops.List(ctx, parseListQuery(r, def.MaxPageSize))
			if err != nil {
				s.writeError(w, r, mapDomain(err, label))
				return
			}
			s.writeObject(w, r, m, http.StatusOK, newListEnvelope(kind, res))
		}))
	}
	if def.Ops.Get != nil {
		s.handle("GET "+item, reg.meta(s, lv, "get", true), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := metaOf(r)
			ctx, err := buildCtx(r, m)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			res, err := def.Ops.Get(ctx, ctx.Name)
			if err != nil {
				s.writeError(w, r, mapDomain(err, label))
				return
			}
			s.writeObject(w, r, m, http.StatusOK, asObject(res))
		}))
	}
	for _, v := range def.Verbs {
		vm := reg.meta(s, lv, "get", true)
		// Route-table kind, not the authz verb: without it the spec
		// describes the verb route as a plain item get with the parent's
		// response type.
		vm.Kind = "verb"
		s.handle("GET "+item+"/"+v.Name, vm, verbRoute(s, v.handler, reg.idParam()))
	}
	if def.Ops.Create != nil {
		s.handle("POST "+base, writeMeta("create", false), namedCreate(s, def, label))
	}
	if def.Ops.Update != nil {
		s.handle("PUT "+item, writeMeta("update", true), namedWrite(s, def, label, func(ctx Ctx, body []byte) (*T, error) {
			in := new(T)
			if err := json.Unmarshal(body, in); err != nil {
				return nil, invalidJSONBody(err)
			}
			return def.Ops.Update(ctx, ctx.Name, in)
		}))
	}
	if def.Ops.Patch != nil {
		s.handle("PATCH "+item, writeMeta("patch", true), namedWrite(s, def, label, func(ctx Ctx, body []byte) (*T, error) {
			return def.Ops.Patch(ctx, ctx.Name, body)
		}))
	}
	if def.Ops.Delete != nil {
		s.handle("DELETE "+item, reg.meta(s, lv, "delete", true), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := metaOf(r)
			ctx, err := buildCtx(r, m)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			if err := def.Ops.Delete(ctx, ctx.Name); err != nil {
				s.writeError(w, r, mapDomain(err, label))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	}

	for _, a := range def.Actions {
		pattern := a.Method + " " + item + "/" + a.Name
		if !a.OnItem {
			pattern = a.Method + " " + base + "/" + a.Name
		}
		m := reg.meta(s, lv, a.Name, a.OnItem)
		// Normalise the route-table kind: meta() seeds Kind with the raw
		// verb (the action name), which the spec generator cannot
		// classify. Verb keeps the action name for authz/audit.
		m.Kind = "action"
		m.PermCodes = a.Permission
		m.StatusCode = a.StatusCode
		m.Sensitive = a.Sensitive || def.Sensitive
		m.Interactive = a.Name == "exec" || a.Name == "console"
		s.handle(pattern, m, a.handler(s, m))
	}
}

// namedCreate mirrors wrapCreate for string-keyed resources.
func namedCreate[T any](s *Server, def NamedDef[T], label string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		body, err := readBodyLimited(w, r, def.MaxBodyBytes)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		in := new(T)
		if err := json.Unmarshal(body, in); err != nil {
			s.writeError(w, r, invalidJSONBody(err))
			return
		}
		res, err := def.Ops.Create(ctx, in)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		status := http.StatusCreated
		if ctx.DryRun {
			status = http.StatusOK
		}
		s.writeObject(w, r, m, status, asObject(res))
	})
}

// namedWrite serves PUT/PATCH on a string-keyed item.
func namedWrite[T any](s *Server, def NamedDef[T], label string, op func(ctx Ctx, body []byte) (*T, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := metaOf(r)
		ctx, err := buildCtx(r, m)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		body, err := readBodyLimited(w, r, def.MaxBodyBytes)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		res, err := op(ctx, body)
		if err != nil {
			s.writeError(w, r, mapDomain(err, label))
			return
		}
		s.writeObject(w, r, m, http.StatusOK, asObject(res))
	})
}
