package store

import "time"

// UserRow is the domain view of a users row.
type UserRow struct {
	ID           int64
	Username     string
	Email        string
	Phone        string
	DisplayName  string
	AvatarURL    string
	PasswordHash string
	Status       string
	Builtin      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserWithNamespacesRow extends UserRow with associated namespace names.
type UserWithNamespacesRow struct {
	UserRow
	NamespaceNames []string
}

// UserWithRoleRow bundles a user with their role in a namespace/workspace.
type UserWithRoleRow struct {
	UserRow
	Role     string
	JoinedAt time.Time
}

// UserCreateInput bundles CreateUser inputs.
type UserCreateInput struct {
	Username     string
	Email        string
	Phone        string
	DisplayName  string
	AvatarURL    string
	PasswordHash string
	Status       string
}

// UserUpdateInput bundles UpdateUser inputs. Username is immutable
// at the API level but part of the SQL UPDATE SET list, so the caller
// passes the existing value here.
type UserUpdateInput struct {
	ID          int64
	Username    string
	Email       string
	Phone       string
	DisplayName string
	AvatarURL   string
	Status      string
}

// UserPatchInput bundles PatchUser inputs; zero-valued fields mean
// "do not change".
type UserPatchInput struct {
	ID          int64
	Email       *string
	Phone       *string
	DisplayName *string
	AvatarURL   *string
	Status      *string
}

// WorkspaceRow is the domain view of a workspaces row.
type WorkspaceRow struct {
	ID          int64
	Name        string
	DisplayName string
	Description string
	OwnerID     int64
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkspaceWithOwnerRow extends WorkspaceRow with owner + count aggregates.
type WorkspaceWithOwnerRow struct {
	WorkspaceRow
	OwnerUsername    string
	CreatorName      string
	NamespaceCount   int64
	MemberCount      int64
	RoleBindingCount int64
}

// WorkspaceWithOwnerAndRoleRow extends the owner view with the caller's role.
type WorkspaceWithOwnerAndRoleRow struct {
	WorkspaceRow
	OwnerUsername   string
	NamespaceCount  int64
	MemberCount     int64
	Role            string
	RoleDisplayName string
	JoinedAt        time.Time
}

// WorkspaceCreateInput bundles CreateWorkspace inputs.
type WorkspaceCreateInput struct {
	Name        string
	DisplayName string
	Description string
	OwnerID     int64
	Status      string
}

// WorkspaceUpdateInput bundles UpdateWorkspace inputs.
type WorkspaceUpdateInput struct {
	ID          int64
	Name        string
	DisplayName string
	Description string
	OwnerID     int64
	Status      string
}

// NamespaceRow is the domain view of a namespaces row.
type NamespaceRow struct {
	ID          int64
	Name        string
	DisplayName string
	Description string
	WorkspaceID int64
	OwnerID     int64
	Visibility  string
	MaxMembers  int32
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NamespaceWithOwnerRow extends NamespaceRow with owner + stats.
type NamespaceWithOwnerRow struct {
	NamespaceRow
	OwnerUsername    string
	WorkspaceName    string
	CreatorName      string
	MemberCount      int64
	RoleBindingCount int64
}

// NamespaceWithOwnerAndRoleRow extends owner view with caller's role.
type NamespaceWithOwnerAndRoleRow struct {
	NamespaceRow
	OwnerUsername   string
	WorkspaceName   string
	MemberCount     int64
	Role            string
	RoleDisplayName string
	JoinedAt        time.Time
}

// NamespaceCreateInput bundles CreateNamespace inputs.
type NamespaceCreateInput struct {
	Name        string
	DisplayName string
	Description string
	WorkspaceID int64
	OwnerID     int64
	Visibility  string
	MaxMembers  int32
	Status      string
}

// NamespaceUpdateInput bundles UpdateNamespace inputs.
type NamespaceUpdateInput struct {
	ID          int64
	Name        string
	DisplayName string
	Description string
	OwnerID     int64
	Visibility  string
	MaxMembers  int32
	Status      string
}

// RefreshTokenRow is the domain view of a refresh_tokens row.
type RefreshTokenRow struct {
	ID        int64
	TokenHash string
	UserID    int64
	ClientID  string
	Scope     string
	ExpiresAt time.Time
	Revoked   bool
	CreatedAt time.Time
}

// RefreshTokenCreateInput bundles CreateRefreshToken inputs.
type RefreshTokenCreateInput struct {
	TokenHash string
	UserID    int64
	ClientID  string
	Scope     string
	ExpiresAt time.Time
}

// UserAuthRow is the domain view of the OIDC password-grant lookup.
type UserAuthRow struct {
	ID           int64
	Username     string
	Email        string
	DisplayName  string
	Phone        string
	Status       string
	PasswordHash string
}

// PermissionUpsertInput bundles Upsert / SyncModule inputs. Callers build these
// from static metadata (permission code, HTTP method/path, scope).
type PermissionUpsertInput struct {
	Code        string
	Method      string
	Path        string
	Scope       string
	Description string
}

// PermissionRow is the domain view of a permissions row.
type PermissionRow struct {
	ID          int64
	Code        string
	Method      string
	Path        string
	Scope       string
	Description string
	CreatedAt   time.Time
}

// PermissionCodeScope holds a permission code and its scope.
type PermissionCodeScope struct {
	Code  string
	Scope string
}

// RoleRow is the domain view of a roles row.
type RoleRow struct {
	ID          int64
	Name        string
	DisplayName string
	Description string
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	Builtin     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RoleListItem extends RoleRow with rule_count used by List handlers.
type RoleListItem struct {
	RoleRow
	RuleCount int32
}

// RoleWithRulesRow extends RoleRow with its permission rule patterns.
type RoleWithRulesRow struct {
	RoleRow
	Rules []string
}

// RoleCreateInput bundles CreateRole inputs.
type RoleCreateInput struct {
	Name        string
	DisplayName string
	Description string
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	Builtin     bool
}

// RoleUpdateInput bundles UpdateRole inputs.
type RoleUpdateInput struct {
	ID          int64
	Name        string
	DisplayName string
	Description string
}

// RoleBindingRow is the domain view of a role_bindings row.
type RoleBindingRow struct {
	ID          int64
	UserID      int64
	RoleID      int64
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	IsOwner     bool
	CreatedAt   time.Time
}

// RoleBindingCreateInput bundles CreateRoleBinding inputs.
type RoleBindingCreateInput struct {
	UserID      int64
	RoleID      int64
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
}

// RoleBindingWithDetailsRow extends RoleBindingRow with user + role display info.
type RoleBindingWithDetailsRow struct {
	RoleBindingRow
	Username        string
	UserDisplayName string
	RoleName        string
	RoleDisplayName string
	WorkspaceName   string
	NamespaceName   string
}

// UserPermissionRuleRow represents a single (scope, resource, pattern) row for cache loading.
type UserPermissionRuleRow struct {
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	Pattern     string
}

// UserRoleBindingWithRules represents a binding row with role name and pattern for the permissions API.
type UserRoleBindingWithRules struct {
	Scope       string
	WorkspaceID *int64
	NamespaceID *int64
	RoleName    string
	Pattern     string
}

// BlockingResourceRow represents one (kind, count) row from the
// ws/ns delete pre-check. Only kinds with count > 0 are returned.
type BlockingResourceRow struct {
	Kind  string
	Count int64
}
