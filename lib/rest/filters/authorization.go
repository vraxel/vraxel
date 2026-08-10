// Package filters provides the global HTTP middleware (authentication,
// CSRF, request logging) plus the RBAC contracts the route-level
// authorization in lib/apiserver enforces.
package filters

import (
	"context"
)

// PermissionChecker checks user permissions against RBAC policy.
type PermissionChecker interface {
	CheckPermission(ctx context.Context, userID int64, permCode string, scope string, workspaceID, namespaceID int64) (bool, error)
	// CheckAnyPermission checks whether a user has any of the given permissions.
	// Supports wildcard targets (e.g. "infra:hosts:*") via bidirectional matching.
	CheckAnyPermission(ctx context.Context, userID int64, permCodes []string, scope string, workspaceID, namespaceID int64) (bool, error)
	IsPlatformAdmin(ctx context.Context, userID int64) (bool, error)
	GetAccessibleWorkspaceIDs(ctx context.Context, userID int64) ([]int64, error)
	GetAccessibleNamespaceIDs(ctx context.Context, userID int64) ([]int64, error)
}

// AccessFilter holds accessible resource IDs for non-admin users.
// nil means no filter (admin sees everything); empty slice means no access.
type AccessFilter struct {
	WorkspaceIDs []int64
	NamespaceIDs []int64
}

type accessFilterContextKey struct{}

// WithAccessFilter stores the access filter in the context.
func WithAccessFilter(ctx context.Context, f *AccessFilter) context.Context {
	return context.WithValue(ctx, accessFilterContextKey{}, f)
}

// AccessFilterFromContext retrieves the access filter from the context.
// Returns nil when the request is unfiltered (admin or non-list route).
func AccessFilterFromContext(ctx context.Context) *AccessFilter {
	f, _ := ctx.Value(accessFilterContextKey{}).(*AccessFilter)
	return f
}

// Authorizer bundles the RBAC checker for the install chain.
type Authorizer struct {
	Checker PermissionChecker
}

// PermListCode is whitelisted for any authenticated user: every module
// needs to render permission pickers.
const PermListCode = "iam:permissions:list"
