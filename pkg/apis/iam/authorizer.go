package iam

import (
	"vraxel.io/vraxel/lib/rest/filters"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// NewAuthorizer creates the RBAC authorizer. Returns it plus the
// RBACChecker so the caller can wire cache invalidation. Route-level
// permission codes are baked in at registration by lib/apiserver, so
// there is no URL-based permission lookup anymore.
func NewAuthorizer(rbStore modstore.RoleBindingStore) (*filters.Authorizer, *RBACChecker) {
	checker := NewRBACChecker(rbStore)
	return &filters.Authorizer{Checker: checker}, checker
}
