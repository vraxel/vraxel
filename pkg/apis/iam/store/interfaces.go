package store

import (
	"context"

	"vraxel.io/vraxel/lib/list"
)

// UserStore defines database operations on users. Domain-typed.
type UserStore interface {
	Create(ctx context.Context, input UserCreateInput) (*UserRow, error)
	GetByID(ctx context.Context, id int64) (*UserRow, error)
	GetByUsername(ctx context.Context, username string) (*UserRow, error)
	Update(ctx context.Context, input UserUpdateInput) (*UserRow, error)
	Patch(ctx context.Context, input UserPatchInput) (*UserRow, error)
	UpdateLastLogin(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64) error
	DeleteByIDs(ctx context.Context, ids []int64) (int64, error)
	List(ctx context.Context, query list.Query) (*list.Result[UserWithNamespacesRow], error)
	GetUserForAuth(ctx context.Context, identifier string) (*UserAuthRow, error)
	SetPasswordHash(ctx context.Context, id int64, hash string) error
	SetBuiltin(ctx context.Context, id int64, builtin bool) error
}

// RefreshTokenStore defines database operations on refresh tokens.
type RefreshTokenStore interface {
	Create(ctx context.Context, input RefreshTokenCreateInput) (*RefreshTokenRow, error)
	GetByHash(ctx context.Context, tokenHash string) (*RefreshTokenRow, error)
	ConsumeByHash(ctx context.Context, tokenHash string) (*RefreshTokenRow, error)
	Revoke(ctx context.Context, tokenHash string) error
	RevokeByUserID(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
}

// WorkspaceStore defines database operations on workspaces. Domain-typed.
type WorkspaceStore interface {
	Create(ctx context.Context, input WorkspaceCreateInput) (*WorkspaceWithOwnerRow, error)
	GetByID(ctx context.Context, id int64) (*WorkspaceWithOwnerRow, error)
	Update(ctx context.Context, input WorkspaceUpdateInput) (*WorkspaceWithOwnerRow, error)
	Patch(ctx context.Context, input WorkspaceUpdateInput) (*WorkspaceWithOwnerRow, error)
	Delete(ctx context.Context, id int64) error
	DeleteByIDs(ctx context.Context, ids []int64) (int64, error)
	List(ctx context.Context, query list.Query) (*list.Result[WorkspaceWithOwnerRow], error)
	CountNamespaces(ctx context.Context, workspaceID int64) (int64, error)
	ListBlockingResources(ctx context.Context, workspaceID int64) ([]BlockingResourceRow, error)
}

// NamespaceStore defines database operations on namespaces. Domain-typed.
type NamespaceStore interface {
	Create(ctx context.Context, input NamespaceCreateInput) (*NamespaceWithOwnerRow, error)
	GetByID(ctx context.Context, id int64) (*NamespaceWithOwnerRow, error)
	Update(ctx context.Context, input NamespaceUpdateInput) (*NamespaceWithOwnerRow, error)
	Patch(ctx context.Context, input NamespaceUpdateInput) (*NamespaceWithOwnerRow, error)
	Delete(ctx context.Context, id int64) error
	DeleteByIDs(ctx context.Context, ids []int64) (int64, error)
	List(ctx context.Context, query list.Query) (*list.Result[NamespaceWithOwnerRow], error)
	CountUsers(ctx context.Context, namespaceID int64) (int64, error)
	ListBlockingResources(ctx context.Context, namespaceID int64) ([]BlockingResourceRow, error)
	// WorkspaceIDOf returns the parent workspace id of a namespace. Used
	// by the authz layer to scope a flat /namespaces/{id} route.
	WorkspaceIDOf(ctx context.Context, id int64) (int64, error)
}

// PermissionStore defines database operations on permissions. Write paths take
// PermissionUpsertInput (callers build these from static metadata); read paths
// return the domain PermissionRow.
type PermissionStore interface {
	Upsert(ctx context.Context, in PermissionUpsertInput) (*PermissionRow, error)
	DeleteByModuleNotInCodeScopes(ctx context.Context, modulePrefix string, keepCodeScopes []string) error
	GetByCode(ctx context.Context, code, scope string) (*PermissionRow, error)
	List(ctx context.Context, query list.Query) (*list.Result[PermissionRow], error)
	ListAllCodes(ctx context.Context) ([]string, error)
	ListCodeScopes(ctx context.Context) ([]PermissionCodeScope, error)
	SyncModule(ctx context.Context, modulePrefix string, perms []PermissionUpsertInput) error
}

// RoleStore defines database operations on roles. Domain-typed.
type RoleStore interface {
	Create(ctx context.Context, input RoleCreateInput) (*RoleRow, error)
	GetByID(ctx context.Context, id int64) (*RoleWithRulesRow, error)
	GetByName(ctx context.Context, name string) (*RoleRow, error)
	GetByNameAndWorkspace(ctx context.Context, name string, workspaceID int64) (*RoleRow, error)
	GetByNameAndNamespace(ctx context.Context, name string, namespaceID int64) (*RoleRow, error)
	Update(ctx context.Context, input RoleUpdateInput) (*RoleRow, error)
	Upsert(ctx context.Context, input RoleCreateInput) (*RoleRow, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, query list.Query) (*list.Result[RoleListItem], error)
	SetPermissionRules(ctx context.Context, roleID int64, patterns []string) error
	SeedRBAC(ctx context.Context, roles []BuiltinRoleDef, adminUsername string) error
}

// RoleBindingStore defines database operations on role bindings.
type RoleBindingStore interface {
	Create(ctx context.Context, input RoleBindingCreateInput) (*RoleBindingRow, error)
	// CreateMany binds one role to many users in a single transaction:
	// either every binding lands or none does, so a failure halfway
	// through a 20-user grant cannot leave a partial authorization set.
	// Insertion is idempotent (ON CONFLICT DO NOTHING), so re-granting an
	// existing binding is a no-op rather than an error; the returned count
	// is the number of rows actually inserted.
	CreateMany(ctx context.Context, inputs []RoleBindingCreateInput) (int, error)
	Delete(ctx context.Context, id int64) error
	// DeleteByIDs batch-deletes non-owner bindings. Returns the count of rows
	// actually deleted; bindings flagged is_owner=true are silently skipped
	// so the call cannot strip the workspace/namespace owner.
	DeleteByIDs(ctx context.Context, ids []int64) (int64, error)
	GetByID(ctx context.Context, id int64) (*RoleBindingRow, error)
	ListPlatform(ctx context.Context, query list.Query) (*list.Result[RoleBindingWithDetailsRow], error)
	ListByWorkspaceID(ctx context.Context, workspaceID int64, query list.Query) (*list.Result[RoleBindingWithDetailsRow], error)
	ListByNamespaceID(ctx context.Context, namespaceID int64, query list.Query) (*list.Result[RoleBindingWithDetailsRow], error)
	ListByUserID(ctx context.Context, userID int64, query list.Query) (*list.Result[RoleBindingWithDetailsRow], error)
	CountByRoleAndScope(ctx context.Context, roleID int64, scope string) (int64, error)
	GetAccessibleWorkspaceIDs(ctx context.Context, userID int64) ([]int64, error)
	GetAccessibleNamespaceIDs(ctx context.Context, userID int64) ([]int64, error)
	GetUserIDsByWorkspaceID(ctx context.Context, workspaceID int64) ([]int64, error)
	GetUserIDsByNamespaceID(ctx context.Context, namespaceID int64) ([]int64, error)
	LoadUserPermissionRules(ctx context.Context, userID int64) ([]UserPermissionRuleRow, error)
	GetUserRoleBindingsWithRules(ctx context.Context, userID int64) ([]UserRoleBindingWithRules, error)
	TransferOwnership(ctx context.Context, scope string, resourceID int64, callerID int64, callerIsPlatformAdmin bool, newOwnerUserID int64, adminRoleName string) (oldOwnerUserID int64, err error)
	AddWorkspaceMember(ctx context.Context, userID, workspaceID int64, roleID int64) error
	AddNamespaceMember(ctx context.Context, userID, namespaceID int64, roleID int64) error
	RemoveWorkspaceMember(ctx context.Context, userID, workspaceID int64) error
	RemoveNamespaceMember(ctx context.Context, userID, namespaceID int64) error
	ListWorkspaceMembers(ctx context.Context, workspaceID int64, query list.Query) (*list.Result[UserWithRoleRow], error)
	ListNamespaceMembers(ctx context.Context, namespaceID int64, query list.Query) (*list.Result[UserWithRoleRow], error)
	// ListWorkspaceNonMembers / ListNamespaceNonMembers return platform users
	// that are NOT yet members of the given workspace / namespace, used by the
	// add-member dialog. The returned UserRow has no role / joinedAt context
	// because these users have no relevant role binding for this scope.
	ListWorkspaceNonMembers(ctx context.Context, workspaceID int64, query list.Query) (*list.Result[UserRow], error)
	ListNamespaceNonMembers(ctx context.Context, namespaceID int64, query list.Query) (*list.Result[UserRow], error)
	ListUserWorkspaces(ctx context.Context, userID int64, query list.Query) (*list.Result[WorkspaceWithOwnerAndRoleRow], error)
	ListUserNamespaces(ctx context.Context, userID int64, query list.Query) (*list.Result[NamespaceWithOwnerAndRoleRow], error)
}
