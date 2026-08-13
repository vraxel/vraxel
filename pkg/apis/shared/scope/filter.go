// Package scope carries the scope filter extracted from REST URL path
// segments down to store-layer queries. Stores apply it to their item-
// loading SQL (GetByID / Patch / Delete) so a request that addresses
// /workspaces/{w}/namespaces/{n}/{resource}/{id} only resolves rows
// whose actual workspace_id / namespace_id match those segments.
//
// This is the storage-side defense against cross-scope privilege
// escalation: a namespace-admin@N cannot use the namespace-N URL
// prefix to act on a workspace-scoped resource that does not live
// in N -- the SQL filter returns zero rows -> NotFound.
package scope

import "strconv"

// Filter encodes the workspace/namespace constraint derived from URL
// path params. nil pointer means "URL did not claim this scope level
// -> do not constrain"; non-nil means "URL claimed this id -> SQL
// must enforce equality (NULL columns also do not match)".
type Filter struct {
	WorkspaceID *int64
	NamespaceID *int64
}

// PartsForCreate returns the (scope, workspaceID, namespaceID) triple stores
// use to stamp a new row's scope: the level is derived from which ids the
// URL claimed (namespace id set → "namespace"; only workspace → "workspace";
// neither → "platform"). The single home for the per-module
// scopePartsForCreate copies.
func (f Filter) PartsForCreate() (string, *int64, *int64) {
	if f.NamespaceID != nil {
		return Namespace, f.WorkspaceID, f.NamespaceID
	}
	if f.WorkspaceID != nil {
		return Workspace, f.WorkspaceID, nil
	}
	return Platform, nil, nil
}

// The values of the scope column on every scoped table.
const (
	Platform  = "platform"
	Workspace = "workspace"
	Namespace = "namespace"
)

// From builds a Filter from REST PathParams. The keys "workspaceId"
// and "namespaceId" are the framework-standard names produced by
// lib/rest's installer (see defaultIDParam). Empty / unparsable
// values are treated as "URL did not claim this scope".
func From(params map[string]string) Filter {
	return Filter{
		WorkspaceID: parseID(params["workspaceId"]),
		NamespaceID: parseID(params["namespaceId"]),
	}
}

func parseID(s string) *int64 {
	if s == "" {
		return nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return &id
}

// FromIDs builds a Filter from already-parsed workspace/namespace ids
// (0 = "URL did not claim this scope level"). Typed apiserver handlers
// use it to translate Ctx.Scope into the store-layer filter without a
// params-map round trip.
func FromIDs(workspaceID, namespaceID int64) Filter {
	var f Filter
	if workspaceID > 0 {
		ws := workspaceID
		f.WorkspaceID = &ws
	}
	if namespaceID > 0 {
		ns := namespaceID
		f.NamespaceID = &ns
	}
	return f
}
