package apiserver

import (
	"context"
	"net/http"
	"strings"
)

// resourceReg is the registration-time record of one resource, kept so
// child resources (Parent references) and permission collection can see
// the whole tree without re-walking anything.
type resourceReg struct {
	Group        string
	Name         string
	RelPath      string // registration path within the group, e.g. "hosts/nics"
	Parent       *resourceReg
	Scopes       Scope
	PermTarget   []string
	Verbs        []string
	ActionDefs   []ActionDef
	APIVersion   string
	BasePath     string
	MaxPageSize  int32
	MaxBodyBytes int64
	Sensitive    bool   // resource-level body redaction for audit
	IDParam      string // explicit item param name; "" derives from Name
	StringID     bool   // item key is a name string; buildCtx skips the int64 parse
	// TypeName is the Go name of the resource payload type (ResourceDef's
	// T). Carried so Server.Routes can hand the spec generator the exact
	// schema each route serves instead of guessing from the URL.
	TypeName string

	// ExtraAllow is the resource-level allowance hook; meta() binds each
	// route's first permission code into the per-route closure.
	ExtraAllow func(permCode string, r *http.Request, userID int64) bool
	// ListAccessScope marks the platform list route for AccessFilter
	// injection ("workspaces" / "namespaces"); see routeMeta.AccessScope.
	ListAccessScope string
	// PermScope overrides the natural scope permission records fan out
	// from. Zero = derive from Scopes.deepest(). Needed only by iam's
	// workspaces/namespaces resources, whose item URLs ARE the scope
	// segments v1's URL-based derivation keyed on.
	PermScope Scope
	// ItemScopeResolver overrides the runtime authz scope for flat
	// (platform-registered) item routes; see ResourceDef.ItemScopeResolver.
	ItemScopeResolver func(ctx context.Context, itemID int64) ScopeInfo
}

// permScope returns the natural scope permission records fan out from:
// the explicit override, or the deepest registered level.
func (rr *resourceReg) permScope() Scope {
	if rr.PermScope != 0 {
		return rr.PermScope
	}
	return rr.Scopes.deepest()
}

// idParam returns the item path-parameter name, honoring the explicit
// override (v1 ResourceInfo.IDParam, e.g. "resourceSetId" where the
// derived form would be "resource-setId").
func (rr *resourceReg) idParam() string {
	if rr.IDParam != "" {
		return rr.IDParam
	}
	return singularID(rr.Name)
}

// parentPattern returns the URL fragment contributed by the parent
// chain, e.g. "hosts/{hostId}/nics/{nicId}/" for a grandchild — empty
// for top-level resources. Parameter names follow v1's defaultIDParam.
func (rr *resourceReg) parentPattern() string {
	if rr.Parent == nil {
		return ""
	}
	var b strings.Builder
	for _, anc := range rr.ancestors() {
		b.WriteString(anc.Name)
		b.WriteString("/{")
		b.WriteString(sanitizeWildcard(anc.idParam()))
		b.WriteString("}/")
	}
	return b.String()
}

// ancestors returns the parent chain ordered outermost → innermost.
func (rr *resourceReg) ancestors() []*resourceReg {
	var chain []*resourceReg
	for p := rr.Parent; p != nil; p = p.Parent {
		chain = append([]*resourceReg{p}, chain...)
	}
	return chain
}

// nameChain returns the resource-name chain (parents + self), the code
// base for permission derivation: ["hosts","nics"] → "compute:hosts:nics:{verb}".
func (rr *resourceReg) nameChain() []string {
	var parts []string
	for _, anc := range rr.ancestors() {
		parts = append(parts, anc.Name)
	}
	return append(parts, rr.Name)
}

// routeMeta is the per-route metadata composed into the handler chain at
// registration. Authorization and audit read it directly — the v2
// replacement for v1's URL reverse-parsing.
type routeMeta struct {
	Module   string // group name; audit "module" field
	Resource string // leaf resource name
	Chain    string // v1 audit-compatible resource chain incl. scope segments
	Verb     string // list/get/create/... or the action name
	// Kind classifies the route for spec generation: a CRUD op name, or
	// "action" / "verb". Verb alone is not enough -- read verbs reuse the
	// parent's "get" code.
	Kind       string
	TypeName   string
	ScopeLevel Scope
	PermCodes  []string // codes checked by the authz link (may contain wildcards)
	APIVersion string

	// ParentParams are the parent id parameter names in outer→inner
	// order ("hostId", "nicId"); buildCtx parses them into Ctx.Parents.
	ParentParams []string
	// IDParam is the item parameter name ("channelId"); empty on
	// collection routes.
	IDParam string
	// StringID marks name-keyed items: buildCtx fills Ctx.Name and
	// leaves Ctx.ID zero instead of int64-parsing the segment.
	StringID bool

	MaxPageSize  int32
	MaxBodyBytes int64
	// StatusCode is the success status of a JSON action (0 = 200).
	// Carried on meta so post-construction ActionDef field edits take
	// effect uniformly with Sensitive/OnItem.
	StatusCode int

	// Sensitive: audit must not capture the request body.
	Sensitive bool
	// Interactive: WebSocket route that opens a shell/console — the only
	// GET-method routes v1 audited.
	Interactive bool

	// ExtraAllow is an optional per-route allowance evaluated before the
	// permission check (the hook that absorbs v1's hardcoded self-user
	// rule when iam migrates). Nil for every other resource.
	ExtraAllow func(r *http.Request, userID int64) bool

	// AccessScope marks the platform-level list route of iam workspaces /
	// namespaces: any authenticated user may call it, and the authz link
	// injects an AccessFilter with the IDs they hold bindings for instead
	// of a permission check (v1 serveWorkspaceListAccessFilter).
	AccessScope string

	// ItemScopeResolver overrides the authz scope for a flat item route of
	// a scope-defining resource (iam workspaces/namespaces); see
	// ResourceDef.ItemScopeResolver. Nil for every other route.
	ItemScopeResolver func(ctx context.Context, itemID int64) ScopeInfo

	server *Server
}

// meta builds the routeMeta for one (scope level, verb) route. withID
// declares whether the route's pattern carries the item {id} segment —
// explicit because action names can't be classified by verb string
// (collection actions share the default branch otherwise, poisoning the
// bridged params map and audit ResourceID with an empty entry).
func (rr *resourceReg) meta(s *Server, lv Scope, verb string, withID bool) *routeMeta {
	chainParts := append(lv.scopeChainParts(), rr.nameChain()...)

	m := &routeMeta{
		Module:       moduleName(rr.Group),
		Resource:     rr.Name,
		Chain:        strings.Join(chainParts, ":"),
		Verb:         verb,
		Kind:         verb,
		TypeName:     rr.TypeName,
		ScopeLevel:   lv,
		APIVersion:   rr.APIVersion,
		MaxPageSize:  rr.MaxPageSize,
		MaxBodyBytes: rr.MaxBodyBytes,
		server:       s,
	}
	for _, anc := range rr.ancestors() {
		m.ParentParams = append(m.ParentParams, anc.idParam())
	}
	if withID {
		m.IDParam = rr.idParam()
		m.StringID = rr.StringID
	}

	if len(rr.PermTarget) > 0 {
		m.PermCodes = rr.PermTarget
	} else {
		m.PermCodes = []string{permCode(rr.Group, rr.nameChain(), verb)}
	}
	if rr.ExtraAllow != nil {
		code, hook := m.PermCodes[0], rr.ExtraAllow
		m.ExtraAllow = func(r *http.Request, userID int64) bool {
			return hook(code, r, userID)
		}
	}
	if verb == "list" && lv == ScopePlatform {
		m.AccessScope = rr.ListAccessScope
	}
	m.ItemScopeResolver = rr.ItemScopeResolver
	return m
}

// moduleName mirrors v1: the core group ("") is module "core".
func moduleName(group string) string {
	if group == "" {
		return "core"
	}
	return group
}

// permCode builds the canonical permission code, byte-compatible with
// v1 canonicalCode (scope segments never appear in v2 name chains, so no
// stripping is needed).
func permCode(group string, nameChain []string, verb string) string {
	return moduleName(group) + ":" + strings.Join(nameChain, ":") + ":" + verb
}

// scopeParamNames returns the scope path parameter names present at a level.
func (m *routeMeta) scopeParamNames() []string {
	switch m.ScopeLevel {
	case ScopeWorkspace:
		return []string{"workspaceId"}
	case ScopeNamespace:
		return []string{"workspaceId", "namespaceId"}
	default:
		return nil
	}
}

// pathParams reconstructs the v1-style path params map from the matched
// pattern's values — the bridge contract for Legacy/WS/Raw handlers.
func (m *routeMeta) pathParams(r *http.Request) map[string]string {
	params := make(map[string]string, len(m.ParentParams)+3)
	for _, name := range m.scopeParamNames() {
		params[name] = pathValue(r, name)
	}
	for _, name := range m.ParentParams {
		params[name] = pathValue(r, name)
	}
	if m.IDParam != "" {
		params[m.IDParam] = pathValue(r, m.IDParam)
	}
	return params
}

// legacyParams merges query params over path params exactly as v1's
// HandleAction did (path params win on conflict).
func (m *routeMeta) legacyParams(r *http.Request) map[string]string {
	params := m.pathParams(r)
	for k, vals := range r.URL.Query() {
		if _, exists := params[k]; !exists && len(vals) > 0 {
			params[k] = vals[0]
		}
	}
	return params
}

// scope resolves the ScopeInfo for authorization from the matched
// pattern's values; parse failures surface as zero IDs and are rejected
// later by buildCtx with a proper 400.
func (m *routeMeta) scope(r *http.Request) ScopeInfo {
	// Scope-defining resource: a platform-registered ITEM route whose id
	// IS the scope boundary v1 derived from the URL (iam workspaces /
	// namespaces). The permission check must run at that scope so a
	// workspace/namespace-scoped binding matches; without this it runs at
	// platform and every non-platform-admin is denied on /workspaces/{id}.
	// Gated to platform item routes: the nested route already carries the
	// scope in its path, and the collection route keeps its AccessFilter.
	if m.ItemScopeResolver != nil && m.IDParam != "" && m.ScopeLevel == ScopePlatform {
		if id, err := parsePathID(r, m.IDParam); err == nil {
			return m.ItemScopeResolver(r.Context(), id)
		}
	}
	info := ScopeInfo{Level: m.ScopeLevel}
	if m.ScopeLevel&(ScopeWorkspace|ScopeNamespace) != 0 {
		if id, err := parsePathID(r, "workspaceId"); err == nil {
			info.WorkspaceID = id
		}
	}
	if m.ScopeLevel&ScopeNamespace != 0 {
		if id, err := parsePathID(r, "namespaceId"); err == nil {
			info.NamespaceID = id
		}
	}
	return info
}
