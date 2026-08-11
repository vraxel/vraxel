package store

import "vraxel.io/vraxel/pkg/db"

// Stores aggregates every IAM Store impl. Adding a new store requires one
// field here + one line in NewStores.
type Stores struct {
	User         UserStore
	Workspace    WorkspaceStore
	Namespace    NamespaceStore
	RefreshToken RefreshTokenStore
	Permission   PermissionStore
	Role         RoleStore
	RoleBinding  RoleBindingStore
	Registration RegistrationStore
	OAuthState   OAuthStateStore
}

// NewStores creates all IAM store impls from a single *db.DB handle.
func NewStores(d *db.DB) Stores {
	return Stores{
		User:         NewPGUserStore(d),
		Workspace:    NewPGWorkspaceStore(d),
		Namespace:    NewPGNamespaceStore(d),
		RefreshToken: NewPGRefreshTokenStore(d),
		Permission:   NewPGPermissionStore(d),
		Role:         NewPGRoleStore(d),
		RoleBinding:  NewPGRoleBindingStore(d),
		Registration: NewPGRegistrationStore(d),
		OAuthState:   NewPGOAuthStateStore(d),
	}
}
