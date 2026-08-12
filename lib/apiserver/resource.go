package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/runtime"
)

// BatchResult is the batch-delete response envelope
// (successCount/failedCount/failedIds). Returned by Ops.BatchDelete and
// serialized through the negotiated writer, so it carries a TypeMeta.
type BatchResult struct {
	runtime.TypeMeta `json:",inline"`
	SuccessCount     int      `json:"successCount"`
	FailedCount      int      `json:"failedCount"`
	FailedIDs        []string `json:"failedIds,omitempty"`
}

// GetTypeMeta implements runtime.Object.
func (r *BatchResult) GetTypeMeta() *runtime.TypeMeta { return &r.TypeMeta }

var _ runtime.Object = (*BatchResult)(nil)

// BatchDeleteRequest is the {"ids": [...]} request body for batch delete
// operations — the request-side counterpart of BatchResult.
type BatchDeleteRequest struct {
	IDs []string `json:"ids"`
}

// Ops declares which operations a resource exposes via explicit typed
// function fields. A nil field means the route is not registered — the
// v2 replacement for v1's "implement an interface to get a route"
// discovery and the StandardStorage all-or-nothing trap.
//
// Large resources keep literals tidy with method references:
//
//	Ops[Host]{Get: hs.get, List: hs.list, ...}
type Ops[T any] struct {
	List        func(ctx Ctx, q list.Query) (*list.Result[T], error)
	Get         func(ctx Ctx, id int64) (*T, error)
	Create      func(ctx Ctx, in *T) (*T, error)
	Update      func(ctx Ctx, id int64, in *T) (*T, error)
	Delete      func(ctx Ctx, id int64) error
	BatchDelete func(ctx Ctx, ids []int64) (*BatchResult, error)

	// Patch receives the raw JSON body: the absent-vs-zero distinction
	// needs a resource-specific patch type, and Go generics do not allow
	// a struct field to carry its own type parameter. Decode with
	// DecodePatch[P] in one line.
	Patch func(ctx Ctx, id int64, patch json.RawMessage) (*T, error)

	// ListAny is the escape hatch for polymorphic list endpoints whose
	// response shape depends on query params (?view=/?groupBy= dispatch
	// onto different envelopes). Mutually exclusive with List; the
	// returned value serializes as-is.
	ListAny func(ctx Ctx, q list.Query) (any, error)
	// CreateAny is the escape hatch for asymmetric creates (request and
	// response are different types, or the response shape is
	// polymorphic). Receives the raw body; mutually exclusive with
	// Create.
	CreateAny func(ctx Ctx, body json.RawMessage) (any, error)
}

// verbs lists the REST verbs the Ops set exposes, in v1 detectVerbs
// order so derived permission records diff clean against v1's.
func (o Ops[T]) verbs() []string {
	var v []string
	if o.List != nil || o.ListAny != nil {
		v = append(v, "list")
	}
	if o.Get != nil {
		v = append(v, "get")
	}
	if o.Create != nil || o.CreateAny != nil {
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
	if o.BatchDelete != nil {
		v = append(v, "deleteCollection")
	}
	return v
}

// ResourceDef completely describes one resource. Registration derives
// from it: ServeMux routes (per declared scope level), permission
// records, audit metadata baked into each route's handler chain, and
// later the OpenAPI schema (reflected from T).
type ResourceDef[T any] struct {
	Group   string // API group, e.g. "notify"
	Version string // API version; "" defaults to "v1"
	Name    string // plural kebab-case resource name, e.g. "channels"

	// Scopes declares the tenancy levels; zero value = ScopePlatform.
	Scopes Scope

	// Parent nests this resource under another one of the same group,
	// referenced by registered path ("hosts" or "hosts/nics"). The parent
	// must be registered first; unknown references panic at startup.
	Parent string

	// IDParam overrides the derived item path-parameter name
	// ("resource-sets" derives "resource-setId"; v1 declared
	// "resourceSetId"). Empty derives from Name.
	IDParam string

	Ops     Ops[T]
	Actions []ActionDef
	Verbs   []VerbDef

	// SingletonGet serves GET on the collection path returning ONE
	// object — the v1 pattern where a Lister returned a non-list
	// singleton (dashboard overview). The permission verb stays "list"
	// so derived codes match v1's Lister-based derivation. Mutually
	// exclusive with Ops.List.
	SingletonGet func(ctx Ctx) (*T, error)

	// PermissionTargets borrows an existing permission tree instead of
	// auto-deriving one (proxy resources). Same contract as v1.
	PermissionTargets []string

	// MaxPageSize overrides the default 100-per-page list cap (0 = default).
	MaxPageSize int32
	// MaxBodyBytes overrides the default 1 MiB request body cap (0 = default).
	MaxBodyBytes int64

	// Sensitive marks Create/Update/Patch bodies as secret-bearing: the
	// audit middleware skips request-body capture. The declarative
	// replacement for v1 audit.go's hardcoded verb list.
	Sensitive bool

	// ListKind overrides the list response kind. Empty derives
	// "{TypeName}List" (Certificate → CertificateList), which matches the
	// v1 convention across modules; set it only where a module deviated.
	ListKind string

	// ExtraAllow is an optional pre-permission allowance hook, evaluated
	// with the route's first permission code (absorbs v1's hardcoded
	// self-user rule for iam users).
	ExtraAllow func(permCode string, r *http.Request, userID int64) bool
	// ListAccessScope marks the platform-level list route for
	// binding-scoped AccessFilter injection instead of a permission
	// check ("workspaces" / "namespaces"; v1 iam special case).
	ListAccessScope string
	// PermScope overrides the natural scope for permission fan-out
	// (v1 derived scope from the URL, so iam's workspaces resource —
	// whose item path is literally /workspaces/{workspaceId} — produced
	// workspace-scope records despite registering at platform depth).
	PermScope Scope
	// ItemScopeResolver, when set, computes the authorization scope for
	// this resource's flat (platform-registered) ITEM routes from the
	// parsed item id. Needed by scope-defining resources (iam workspaces /
	// namespaces) whose item id IS the scope boundary v1 derived from the
	// URL: the permission check must run at that scope, not at platform,
	// or every non-platform-admin is denied on GET/PUT/DELETE
	// /workspaces/{id} (their binding is workspace-scoped). ctx is the
	// request context for an optional parent lookup (namespace ->
	// workspace). Ignored on collection routes and non-platform scopes.
	ItemScopeResolver func(ctx context.Context, itemID int64) ScopeInfo
}

// version returns the effective API version.
func (d ResourceDef[T]) version() string {
	if d.Version == "" {
		return "v1"
	}
	return d.Version
}

// apiVersion returns the wire apiVersion string ("group/version", or
// bare version for the core group).
func (d ResourceDef[T]) apiVersion() string {
	if d.Group == "" {
		return d.version()
	}
	return d.Group + "/" + d.version()
}

// basePath returns the URL prefix.
func (d ResourceDef[T]) basePath() string {
	if d.Group == "" {
		return "/api/" + d.version()
	}
	return "/api/" + d.Group + "/" + d.version()
}

// Register expands a ResourceDef into routes, permission records and
// route-level middleware on the server. It is a free function because Go
// methods cannot introduce type parameters.
func Register[T any](s *Server, def ResourceDef[T]) {
	if def.Name == "" {
		panic("apiserver: ResourceDef.Name is required")
	}
	if def.Scopes == 0 {
		def.Scopes = ScopePlatform
	}
	// Wire-compat: item-returning ops serialize *T through the
	// negotiated writer, which stamps TypeMeta — so T must carry one.
	// List-only resources serialize items inside the envelope and are
	// exempt (their item structs never carried kind on the wire).
	returnsItem := def.Ops.Get != nil || def.Ops.Create != nil || def.Ops.Update != nil ||
		def.Ops.Patch != nil || def.SingletonGet != nil
	if _, ok := any(new(T)).(runtime.Object); returnsItem && !ok {
		panic(fmt.Sprintf("apiserver: %T must embed runtime.TypeMeta (implement runtime.Object) for wire-compat", new(T)))
	}
	if def.Ops.List != nil && def.Ops.ListAny != nil {
		panic(fmt.Sprintf("apiserver: resource %q declares both List and ListAny", def.Name))
	}
	if def.Ops.Create != nil && def.Ops.CreateAny != nil {
		panic(fmt.Sprintf("apiserver: resource %q declares both Create and CreateAny", def.Name))
	}
	seenAction := make(map[string]bool, len(def.Actions))
	for _, a := range def.Actions {
		if a.Name == "" || a.handler == nil {
			panic(fmt.Sprintf("apiserver: resource %q has an ActionDef without name or handler (use Action/ActionAny/WSAction/RawAction constructors)", def.Name))
		}
		if len(a.Permission) == 0 {
			panic(fmt.Sprintf("apiserver: action %q on resource %q: Permission is required", a.Name, def.Name))
		}
		key := a.Method + " " + a.Name + itemKey(a.OnItem)
		if seenAction[key] {
			panic(fmt.Sprintf("apiserver: duplicate action %q (%s) on resource %q", a.Name, a.Method, def.Name))
		}
		seenAction[key] = true
	}
	seenVerb := make(map[string]bool, len(def.Verbs))
	for _, v := range def.Verbs {
		if v.Name == "" || v.handler == nil {
			panic(fmt.Sprintf("apiserver: resource %q has a VerbDef without name or handler (use Verb/VerbAny constructors)", def.Name))
		}
		if seenVerb[v.Name] {
			panic(fmt.Sprintf("apiserver: duplicate verb %q on resource %q", v.Name, def.Name))
		}
		seenVerb[v.Name] = true
	}

	parent := s.resolveParent(def.Group, def.Parent)
	relPath := def.Name
	if parent != nil {
		relPath = parent.RelPath + "/" + def.Name
	}

	if def.SingletonGet != nil && def.Ops.List != nil {
		panic(fmt.Sprintf("apiserver: resource %q declares both SingletonGet and Ops.List", def.Name))
	}

	verbs := def.Ops.verbs()
	// SingletonGet occupies the collection GET route; its permission
	// verb is "list" for v1 code compatibility.
	if def.SingletonGet != nil {
		verbs = append([]string{"list"}, verbs...)
	}
	// Verb routes check the parent's get code, so verb-only resources
	// (read views without Get) still need the get permission record —
	// routes and permission rows stay 1:1 by construction.
	if len(def.Verbs) > 0 && def.Ops.Get == nil {
		verbs = append(verbs, "get")
	}

	reg := &resourceReg{
		Group:        def.Group,
		Name:         def.Name,
		RelPath:      relPath,
		Parent:       parent,
		Scopes:       def.Scopes,
		PermTarget:   def.PermissionTargets,
		Verbs:        verbs,
		ActionDefs:   def.Actions,
		APIVersion:   def.apiVersion(),
		BasePath:     def.basePath(),
		MaxPageSize:  def.MaxPageSize,
		MaxBodyBytes: def.MaxBodyBytes,
		Sensitive:    def.Sensitive,
		IDParam:      def.IDParam,
		TypeName:     reflect.TypeOf((*T)(nil)).Elem().Name(),

		ExtraAllow:        def.ExtraAllow,
		ListAccessScope:   def.ListAccessScope,
		PermScope:         def.PermScope,
		ItemScopeResolver: def.ItemScopeResolver,
	}
	s.addResource(reg)

	for _, lv := range def.Scopes.levels() {
		registerScopedRoutes(s, def, reg, lv)
	}
	// Permission records are derived lazily by PermRecordsByModule with
	// whole-module knowledge (action-target coverage needs every
	// resource's verbs), so registration records nothing else here.
}

// registerScopedRoutes registers one scope level's routes for a resource.
func registerScopedRoutes[T any](s *Server, def ResourceDef[T], reg *resourceReg, lv Scope) {
	base := def.basePath() + lv.pathPrefix() + "/" + reg.parentPattern() + def.Name
	idParam := reg.idParam()
	item := base + "/{" + sanitizeWildcard(idParam) + "}"

	// Resource-level Sensitive redacts the bodies of the write verbs.
	writeMeta := func(verb string, withID bool) *routeMeta {
		m := reg.meta(s, lv, verb, withID)
		m.Sensitive = def.Sensitive
		return m
	}

	if def.Ops.List != nil {
		s.handle("GET "+base, reg.meta(s, lv, "list", false), wrapList(s, def))
	}
	if def.Ops.ListAny != nil {
		s.handle("GET "+base, reg.meta(s, lv, "list", false), wrapListAny(s, def))
	}
	if def.SingletonGet != nil {
		s.handle("GET "+base, reg.meta(s, lv, "list", false), wrapSingleton(s, def))
	}
	if def.Ops.Get != nil {
		s.handle("GET "+item, reg.meta(s, lv, "get", true), wrapGet(s, def, idParam))
	}
	// Read-only verbs are plain path segments under the item, checking
	// the parent's get permission (v1 CustomVerb semantics at the v2 URL).
	for _, v := range def.Verbs {
		vm := reg.meta(s, lv, "get", true)
		vm.Kind = "verb"
		s.handle("GET "+item+"/"+v.Name, vm, verbRoute(s, v.handler, idParam))
	}
	if def.Ops.Create != nil {
		s.handle("POST "+base, writeMeta("create", false), wrapCreate(s, def))
	}
	if def.Ops.CreateAny != nil {
		cm := writeMeta("create", false)
		// Asymmetric create: request and response are handler-defined,
		// not T. Mark the route so the spec generator emits generic
		// shapes instead of a typed $ref that misdescribes the contract
		// (a batch {ids,roleId} grant is not "create one RoleBinding").
		cm.Kind = "createAny"
		s.handle("POST "+base, cm, wrapCreateAny(s, def))
	}
	if def.Ops.Update != nil {
		s.handle("PUT "+item, writeMeta("update", true), wrapUpdate(s, def))
	}
	if def.Ops.Patch != nil {
		s.handle("PATCH "+item, writeMeta("patch", true), wrapPatch(s, def))
	}
	if def.Ops.Delete != nil {
		s.handle("DELETE "+item, reg.meta(s, lv, "delete", true), wrapDelete(s, def))
	}
	if def.Ops.BatchDelete != nil {
		s.handle("DELETE "+base, reg.meta(s, lv, "deleteCollection", false), wrapBatchDelete(s, def))
	}

	for _, a := range def.Actions {
		pattern := a.Method + " " + item + "/" + a.Name
		if !a.OnItem {
			pattern = a.Method + " " + base + "/" + a.Name
		}
		m := reg.meta(s, lv, a.Name, a.OnItem)
		m.Kind = "action"
		m.PermCodes = a.Permission
		m.StatusCode = a.StatusCode
		m.Sensitive = a.Sensitive || def.Sensitive
		m.Interactive = a.Name == "exec" || a.Name == "console"
		s.handle(pattern, m, a.handler(s, m))
	}
}

// itemKey disambiguates item vs collection actions in duplicate checks.
func itemKey(onItem bool) string {
	if onItem {
		return " item"
	}
	return " collection"
}

// DecodePatch decodes a raw patch body into a resource-specific patch
// type, translating malformed JSON into the same InvalidJSONBody error
// shape v1 produced.
func DecodePatch[P any](raw json.RawMessage) (*P, error) {
	var p P
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, invalidJSONBody(err)
		}
	}
	return &p, nil
}
