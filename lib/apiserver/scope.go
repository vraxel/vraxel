package apiserver

import (
	"net/http"
	"strconv"
	"strings"

	"vraxel.io/vraxel/lib/list"
)

// Scope is a bitmask declaring at which tenancy levels a resource is
// exposed. The framework expands one ResourceDef into one route set per
// declared level; v1 expressed the same thing by duplicating the whole
// ResourceInfo subtree under workspaces/namespaces parents.
type Scope uint8

const (
	// ScopePlatform exposes /api/{group}/{version}/{resource}.
	ScopePlatform Scope = 1 << iota
	// ScopeWorkspace exposes /api/{group}/{version}/workspaces/{workspaceId}/{resource}.
	ScopeWorkspace
	// ScopeNamespace exposes /api/{group}/{version}/workspaces/{workspaceId}/namespaces/{namespaceId}/{resource}.
	ScopeNamespace

	// ScopeAll exposes a resource at every level.
	ScopeAll = ScopePlatform | ScopeWorkspace | ScopeNamespace
)

// levels returns the declared scope levels in platform → namespace order.
func (s Scope) levels() []Scope {
	var out []Scope
	for _, lv := range []Scope{ScopePlatform, ScopeWorkspace, ScopeNamespace} {
		if s&lv != 0 {
			out = append(out, lv)
		}
	}
	return out
}

// name returns the RBAC scope string for a single level, matching the
// values v1's scopeForStorage produced ("platform" / "workspace" / "namespace").
func (s Scope) name() string {
	switch s {
	case ScopeNamespace:
		return "namespace"
	case ScopeWorkspace:
		return "workspace"
	default:
		return "platform"
	}
}

// deepest returns the deepest declared level; it is the "natural scope"
// that permission records fan out from (v1 scopesUpTo semantics).
func (s Scope) deepest() Scope {
	if s&ScopeNamespace != 0 {
		return ScopeNamespace
	}
	if s&ScopeWorkspace != 0 {
		return ScopeWorkspace
	}
	return ScopePlatform
}

// pathPrefix returns the URL segments this level nests under, using the
// SAME parameter names v1 routes used ("workspaceId"/"namespaceId") so
// bridged v1 handlers (WebSocket/Raw) keep reading the params they expect.
func (s Scope) pathPrefix() string {
	switch s {
	case ScopeWorkspace:
		return "/workspaces/{workspaceId}"
	case ScopeNamespace:
		return "/workspaces/{workspaceId}/namespaces/{namespaceId}"
	default:
		return ""
	}
}

// scopeChainParts returns the resource-chain segments contributed by the
// scope level, mirroring how v1's URL reverse-parser included the scope
// resources in the chain (e.g. "workspaces:namespaces:users"). Needed for
// byte-identical audit rows during the migration.
func (s Scope) scopeChainParts() []string {
	switch s {
	case ScopeWorkspace:
		return []string{"workspaces"}
	case ScopeNamespace:
		return []string{"workspaces", "namespaces"}
	default:
		return nil
	}
}

// ScopeInfo is the typed request scope, filled by the route wrapper from
// the matched pattern's path values — never guessed from URL strings.
type ScopeInfo struct {
	Level       Scope
	WorkspaceID int64 // 0 = none (platform level)
	NamespaceID int64
}

// Parts returns the (scope, workspaceID, namespaceID) triple stores use for
// scoped reads: the canonical scope string plus nil-or-set id pointers by
// level (platform → both nil, workspace → ws only, namespace → both). The
// single home for the per-module scopeTriple copies.
func (s ScopeInfo) Parts() (string, *int64, *int64) {
	switch s.Level {
	case ScopeNamespace:
		ws, ns := s.WorkspaceID, s.NamespaceID
		return s.Level.name(), &ws, &ns
	case ScopeWorkspace:
		ws := s.WorkspaceID
		return s.Level.name(), &ws, nil
	default:
		return s.Level.name(), nil, nil
	}
}

// ScopeFilter mirrors pkg/apis/shared/scope.Filter: nil pointer = "do not
// filter on this dimension". Ops handlers hand it to store methods.
type ScopeFilter struct {
	WorkspaceID *int64
	NamespaceID *int64
}

// Filter converts the scope to store-layer filter pointers. At platform
// level both pointers are nil, eliminating the "zero value accidentally
// used as a filter" class of bug.
func (s ScopeInfo) Filter() ScopeFilter {
	var f ScopeFilter
	if s.WorkspaceID > 0 {
		ws := s.WorkspaceID
		f.WorkspaceID = &ws
	}
	if s.NamespaceID > 0 {
		ns := s.NamespaceID
		f.NamespaceID = &ns
	}
	return f
}

// ApplyTo injects exact-match workspace_id / namespace_id filter keys into
// a list query. Platform level is a no-op. Deliberately NOT automatic in
// the framework: resources with inheritance semantics (rows visible from
// the current level plus parent levels) must assemble their own filters.
func (s ScopeInfo) ApplyTo(q *list.Query) {
	if q.Filters == nil {
		q.Filters = make(map[string]any)
	}
	if s.WorkspaceID > 0 {
		q.Filters["workspace_id"] = strconv.FormatInt(s.WorkspaceID, 10)
	}
	if s.NamespaceID > 0 {
		q.Filters["namespace_id"] = strconv.FormatInt(s.NamespaceID, 10)
	}
}

// singularID derives the item path-parameter name from a plural resource
// name, byte-compatible with v1's defaultIDParam ("users" → "userId",
// "boxes" → "boxId").
func singularID(plural string) string {
	var singular string
	if strings.HasSuffix(plural, "ses") || strings.HasSuffix(plural, "xes") || strings.HasSuffix(plural, "zes") {
		singular = strings.TrimSuffix(plural, "es")
	} else {
		singular = strings.TrimSuffix(plural, "s")
	}
	if singular == "" {
		singular = plural
	}
	return singular + "Id"
}

// sanitizeWildcard maps a v1 param name to a legal ServeMux wildcard:
// Go's mux requires identifier-shaped names, but v1's derivation yields
// hyphenated ones for hyphenated resources ("call-logs" → "call-logId")
// and bridged storages read exactly that key from their params map. The
// pattern carries the sanitized form; the bridge maps values back under
// the original name.
func sanitizeWildcard(param string) string {
	return strings.ReplaceAll(param, "-", "_")
}

// pathValue reads a path value by its v1 param name, translating to the
// sanitized wildcard the pattern actually declared.
func pathValue(r *http.Request, param string) string {
	return r.PathValue(sanitizeWildcard(param))
}

// singularLabel derives the human-readable singular used in error
// messages ("channels" → "channel").
func singularLabel(plural string) string {
	if strings.HasSuffix(plural, "ses") || strings.HasSuffix(plural, "xes") || strings.HasSuffix(plural, "zes") {
		return strings.TrimSuffix(plural, "es")
	}
	if s := strings.TrimSuffix(plural, "s"); s != "" {
		return s
	}
	return plural
}
