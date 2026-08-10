package iam

import modstore "vraxel.io/vraxel/pkg/apis/iam/store"
import (
	"context"
	"time"

	"vraxel.io/vraxel/lib/list"
)

// --- Mock modstore.UserStore ---

type mockUserStore struct {
	CreateFn          func(ctx context.Context, in modstore.UserCreateInput) (*modstore.UserRow, error)
	GetByIDFn         func(ctx context.Context, id int64) (*modstore.UserRow, error)
	GetByUsernameFn   func(ctx context.Context, username string) (*modstore.UserRow, error)
	GetByEmailFn      func(ctx context.Context, email string) (*modstore.UserRow, error)
	GetByPhoneFn      func(ctx context.Context, phone string) (*modstore.UserRow, error)
	UpdateFn          func(ctx context.Context, in modstore.UserUpdateInput) (*modstore.UserRow, error)
	PatchFn           func(ctx context.Context, in modstore.UserPatchInput) (*modstore.UserRow, error)
	UpdateLastLoginFn func(ctx context.Context, id int64) error
	DeleteFn          func(ctx context.Context, id int64) error
	DeleteByIDsFn     func(ctx context.Context, ids []int64) (int64, error)
	ListFn            func(ctx context.Context, query list.Query) (*list.Result[modstore.UserWithNamespacesRow], error)
	GetUserForAuthFn  func(ctx context.Context, identifier string) (*modstore.UserAuthRow, error)
	SetPasswordHashFn func(ctx context.Context, id int64, hash string) error
	SetBuiltinFn      func(ctx context.Context, id int64, builtin bool) error
}

func (m *mockUserStore) Create(ctx context.Context, in modstore.UserCreateInput) (*modstore.UserRow, error) {
	return m.CreateFn(ctx, in)
}
func (m *mockUserStore) GetByID(ctx context.Context, id int64) (*modstore.UserRow, error) {
	return m.GetByIDFn(ctx, id)
}
func (m *mockUserStore) GetByUsername(ctx context.Context, username string) (*modstore.UserRow, error) {
	return m.GetByUsernameFn(ctx, username)
}
func (m *mockUserStore) GetByEmail(ctx context.Context, email string) (*modstore.UserRow, error) {
	return m.GetByEmailFn(ctx, email)
}
func (m *mockUserStore) GetByPhone(ctx context.Context, phone string) (*modstore.UserRow, error) {
	return m.GetByPhoneFn(ctx, phone)
}
func (m *mockUserStore) Update(ctx context.Context, in modstore.UserUpdateInput) (*modstore.UserRow, error) {
	return m.UpdateFn(ctx, in)
}
func (m *mockUserStore) Patch(ctx context.Context, in modstore.UserPatchInput) (*modstore.UserRow, error) {
	return m.PatchFn(ctx, in)
}
func (m *mockUserStore) UpdateLastLogin(ctx context.Context, id int64) error {
	return m.UpdateLastLoginFn(ctx, id)
}
func (m *mockUserStore) Delete(ctx context.Context, id int64) error {
	return m.DeleteFn(ctx, id)
}
func (m *mockUserStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	return m.DeleteByIDsFn(ctx, ids)
}
func (m *mockUserStore) List(ctx context.Context, query list.Query) (*list.Result[modstore.UserWithNamespacesRow], error) {
	return m.ListFn(ctx, query)
}
func (m *mockUserStore) GetUserForAuth(ctx context.Context, identifier string) (*modstore.UserAuthRow, error) {
	return m.GetUserForAuthFn(ctx, identifier)
}
func (m *mockUserStore) SetPasswordHash(ctx context.Context, id int64, hash string) error {
	return m.SetPasswordHashFn(ctx, id, hash)
}
func (m *mockUserStore) SetBuiltin(ctx context.Context, id int64, builtin bool) error {
	if m.SetBuiltinFn == nil {
		return nil
	}
	return m.SetBuiltinFn(ctx, id, builtin)
}

// --- Mock modstore.RefreshTokenStore ---

type mockRefreshTokenStore struct {
	CreateFn         func(ctx context.Context, in modstore.RefreshTokenCreateInput) (*modstore.RefreshTokenRow, error)
	GetByHashFn      func(ctx context.Context, tokenHash string) (*modstore.RefreshTokenRow, error)
	ConsumeByHashFn  func(ctx context.Context, tokenHash string) (*modstore.RefreshTokenRow, error)
	RevokeFn         func(ctx context.Context, tokenHash string) error
	RevokeByUserIDFn func(ctx context.Context, userID int64) error
	DeleteExpiredFn  func(ctx context.Context) error
}

func (m *mockRefreshTokenStore) Create(ctx context.Context, in modstore.RefreshTokenCreateInput) (*modstore.RefreshTokenRow, error) {
	return m.CreateFn(ctx, in)
}
func (m *mockRefreshTokenStore) GetByHash(ctx context.Context, tokenHash string) (*modstore.RefreshTokenRow, error) {
	return m.GetByHashFn(ctx, tokenHash)
}
func (m *mockRefreshTokenStore) ConsumeByHash(ctx context.Context, tokenHash string) (*modstore.RefreshTokenRow, error) {
	return m.ConsumeByHashFn(ctx, tokenHash)
}
func (m *mockRefreshTokenStore) Revoke(ctx context.Context, tokenHash string) error {
	return m.RevokeFn(ctx, tokenHash)
}
func (m *mockRefreshTokenStore) RevokeByUserID(ctx context.Context, userID int64) error {
	return m.RevokeByUserIDFn(ctx, userID)
}
func (m *mockRefreshTokenStore) DeleteExpired(ctx context.Context) error {
	return m.DeleteExpiredFn(ctx)
}

// --- Mock modstore.WorkspaceStore ---

type mockWorkspaceStore struct {
	CreateFn                func(ctx context.Context, in modstore.WorkspaceCreateInput) (*modstore.WorkspaceWithOwnerRow, error)
	GetByIDFn               func(ctx context.Context, id int64) (*modstore.WorkspaceWithOwnerRow, error)
	GetByNameFn             func(ctx context.Context, name string) (*modstore.WorkspaceRow, error)
	UpdateFn                func(ctx context.Context, in modstore.WorkspaceUpdateInput) (*modstore.WorkspaceWithOwnerRow, error)
	PatchFn                 func(ctx context.Context, in modstore.WorkspaceUpdateInput) (*modstore.WorkspaceWithOwnerRow, error)
	DeleteFn                func(ctx context.Context, id int64) error
	DeleteByIDsFn           func(ctx context.Context, ids []int64) (int64, error)
	ListFn                  func(ctx context.Context, query list.Query) (*list.Result[modstore.WorkspaceWithOwnerRow], error)
	CountNamespacesFn       func(ctx context.Context, workspaceID int64) (int64, error)
	ListBlockingResourcesFn func(ctx context.Context, workspaceID int64) ([]modstore.BlockingResourceRow, error)
}

func (m *mockWorkspaceStore) Create(ctx context.Context, in modstore.WorkspaceCreateInput) (*modstore.WorkspaceWithOwnerRow, error) {
	return m.CreateFn(ctx, in)
}
func (m *mockWorkspaceStore) GetByID(ctx context.Context, id int64) (*modstore.WorkspaceWithOwnerRow, error) {
	return m.GetByIDFn(ctx, id)
}
func (m *mockWorkspaceStore) GetByName(ctx context.Context, name string) (*modstore.WorkspaceRow, error) {
	return m.GetByNameFn(ctx, name)
}
func (m *mockWorkspaceStore) Update(ctx context.Context, in modstore.WorkspaceUpdateInput) (*modstore.WorkspaceWithOwnerRow, error) {
	return m.UpdateFn(ctx, in)
}
func (m *mockWorkspaceStore) Patch(ctx context.Context, in modstore.WorkspaceUpdateInput) (*modstore.WorkspaceWithOwnerRow, error) {
	return m.PatchFn(ctx, in)
}
func (m *mockWorkspaceStore) Delete(ctx context.Context, id int64) error {
	return m.DeleteFn(ctx, id)
}
func (m *mockWorkspaceStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	return m.DeleteByIDsFn(ctx, ids)
}
func (m *mockWorkspaceStore) List(ctx context.Context, query list.Query) (*list.Result[modstore.WorkspaceWithOwnerRow], error) {
	return m.ListFn(ctx, query)
}
func (m *mockWorkspaceStore) CountNamespaces(ctx context.Context, workspaceID int64) (int64, error) {
	return m.CountNamespacesFn(ctx, workspaceID)
}
func (m *mockWorkspaceStore) ListBlockingResources(ctx context.Context, workspaceID int64) ([]modstore.BlockingResourceRow, error) {
	if m.ListBlockingResourcesFn == nil {
		return nil, nil
	}
	return m.ListBlockingResourcesFn(ctx, workspaceID)
}

// --- Mock modstore.NamespaceStore ---

type mockNamespaceStore struct {
	CreateFn                func(ctx context.Context, in modstore.NamespaceCreateInput) (*modstore.NamespaceWithOwnerRow, error)
	GetByIDFn               func(ctx context.Context, id int64) (*modstore.NamespaceWithOwnerRow, error)
	GetByNameFn             func(ctx context.Context, name string) (*modstore.NamespaceRow, error)
	UpdateFn                func(ctx context.Context, in modstore.NamespaceUpdateInput) (*modstore.NamespaceWithOwnerRow, error)
	PatchFn                 func(ctx context.Context, in modstore.NamespaceUpdateInput) (*modstore.NamespaceWithOwnerRow, error)
	DeleteFn                func(ctx context.Context, id int64) error
	DeleteByIDsFn           func(ctx context.Context, ids []int64) (int64, error)
	ListFn                  func(ctx context.Context, query list.Query) (*list.Result[modstore.NamespaceWithOwnerRow], error)
	CountUsersFn            func(ctx context.Context, namespaceID int64) (int64, error)
	ListBlockingResourcesFn func(ctx context.Context, namespaceID int64) ([]modstore.BlockingResourceRow, error)
	WorkspaceIDOfFn         func(ctx context.Context, id int64) (int64, error)
}

func (m *mockNamespaceStore) Create(ctx context.Context, in modstore.NamespaceCreateInput) (*modstore.NamespaceWithOwnerRow, error) {
	return m.CreateFn(ctx, in)
}
func (m *mockNamespaceStore) GetByID(ctx context.Context, id int64) (*modstore.NamespaceWithOwnerRow, error) {
	return m.GetByIDFn(ctx, id)
}
func (m *mockNamespaceStore) GetByName(ctx context.Context, name string) (*modstore.NamespaceRow, error) {
	return m.GetByNameFn(ctx, name)
}
func (m *mockNamespaceStore) Update(ctx context.Context, in modstore.NamespaceUpdateInput) (*modstore.NamespaceWithOwnerRow, error) {
	return m.UpdateFn(ctx, in)
}
func (m *mockNamespaceStore) Patch(ctx context.Context, in modstore.NamespaceUpdateInput) (*modstore.NamespaceWithOwnerRow, error) {
	return m.PatchFn(ctx, in)
}
func (m *mockNamespaceStore) Delete(ctx context.Context, id int64) error {
	return m.DeleteFn(ctx, id)
}
func (m *mockNamespaceStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	return m.DeleteByIDsFn(ctx, ids)
}
func (m *mockNamespaceStore) List(ctx context.Context, query list.Query) (*list.Result[modstore.NamespaceWithOwnerRow], error) {
	return m.ListFn(ctx, query)
}
func (m *mockNamespaceStore) CountUsers(ctx context.Context, namespaceID int64) (int64, error) {
	return m.CountUsersFn(ctx, namespaceID)
}
func (m *mockNamespaceStore) ListBlockingResources(ctx context.Context, namespaceID int64) ([]modstore.BlockingResourceRow, error) {
	if m.ListBlockingResourcesFn == nil {
		return nil, nil
	}
	return m.ListBlockingResourcesFn(ctx, namespaceID)
}
func (m *mockNamespaceStore) WorkspaceIDOf(ctx context.Context, id int64) (int64, error) {
	if m.WorkspaceIDOfFn == nil {
		return 0, nil
	}
	return m.WorkspaceIDOfFn(ctx, id)
}

// mockRoleBindingStore provides a mock for modstore.RoleBindingStore.
type mockRoleBindingStore struct {
	LoadUserPermissionRulesFn      func(ctx context.Context, userID int64) ([]modstore.UserPermissionRuleRow, error)
	GetAccessibleWorkspaceIDsFn    func(ctx context.Context, userID int64) ([]int64, error)
	GetAccessibleNamespaceIDsFn    func(ctx context.Context, userID int64) ([]int64, error)
	GetUserRoleBindingsWithRulesFn func(ctx context.Context, userID int64) ([]modstore.UserRoleBindingWithRules, error)
	AddWorkspaceMemberFn           func(ctx context.Context, userID, workspaceID int64, roleID int64) error
	AddNamespaceMemberFn           func(ctx context.Context, userID, namespaceID int64, roleID int64) error
	RemoveWorkspaceMemberFn        func(ctx context.Context, userID, workspaceID int64) error
	RemoveNamespaceMemberFn        func(ctx context.Context, userID, namespaceID int64) error
	ListWorkspaceMembersFn         func(ctx context.Context, workspaceID int64, query list.Query) (*list.Result[modstore.UserWithRoleRow], error)
	ListNamespaceMembersFn         func(ctx context.Context, namespaceID int64, query list.Query) (*list.Result[modstore.UserWithRoleRow], error)
	ListWorkspaceNonMembersFn      func(ctx context.Context, workspaceID int64, query list.Query) (*list.Result[modstore.UserRow], error)
	ListNamespaceNonMembersFn      func(ctx context.Context, namespaceID int64, query list.Query) (*list.Result[modstore.UserRow], error)
	ListUserWorkspacesFn           func(ctx context.Context, userID int64, query list.Query) (*list.Result[modstore.WorkspaceWithOwnerAndRoleRow], error)
	ListUserNamespacesFn           func(ctx context.Context, userID int64, query list.Query) (*list.Result[modstore.NamespaceWithOwnerAndRoleRow], error)
}

func (m *mockRoleBindingStore) LoadUserPermissionRules(ctx context.Context, userID int64) ([]modstore.UserPermissionRuleRow, error) {
	return m.LoadUserPermissionRulesFn(ctx, userID)
}
func (m *mockRoleBindingStore) GetAccessibleWorkspaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	return m.GetAccessibleWorkspaceIDsFn(ctx, userID)
}
func (m *mockRoleBindingStore) GetAccessibleNamespaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	return m.GetAccessibleNamespaceIDsFn(ctx, userID)
}
func (m *mockRoleBindingStore) GetUserRoleBindingsWithRules(ctx context.Context, userID int64) ([]modstore.UserRoleBindingWithRules, error) {
	return m.GetUserRoleBindingsWithRulesFn(ctx, userID)
}
func (m *mockRoleBindingStore) Create(context.Context, modstore.RoleBindingCreateInput) (*modstore.RoleBindingRow, error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) Delete(context.Context, int64) error { panic("not implemented") }
func (m *mockRoleBindingStore) DeleteByIDs(context.Context, []int64) (int64, error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) GetByID(context.Context, int64) (*modstore.RoleBindingRow, error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListPlatform(context.Context, list.Query) (*list.Result[modstore.RoleBindingWithDetailsRow], error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListByWorkspaceID(context.Context, int64, list.Query) (*list.Result[modstore.RoleBindingWithDetailsRow], error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListByNamespaceID(context.Context, int64, list.Query) (*list.Result[modstore.RoleBindingWithDetailsRow], error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListByUserID(context.Context, int64, list.Query) (*list.Result[modstore.RoleBindingWithDetailsRow], error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) CountByRoleAndScope(context.Context, int64, string) (int64, error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) GetUserIDsByWorkspaceID(context.Context, int64) ([]int64, error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) GetUserIDsByNamespaceID(context.Context, int64) ([]int64, error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) TransferOwnership(context.Context, string, int64, int64, bool, int64, string) (int64, error) {
	panic("not implemented")
}
func (m *mockRoleBindingStore) AddWorkspaceMember(ctx context.Context, userID, workspaceID int64, roleID int64) error {
	if m.AddWorkspaceMemberFn != nil {
		return m.AddWorkspaceMemberFn(ctx, userID, workspaceID, roleID)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) AddNamespaceMember(ctx context.Context, userID, namespaceID int64, roleID int64) error {
	if m.AddNamespaceMemberFn != nil {
		return m.AddNamespaceMemberFn(ctx, userID, namespaceID, roleID)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) RemoveWorkspaceMember(ctx context.Context, userID, workspaceID int64) error {
	if m.RemoveWorkspaceMemberFn != nil {
		return m.RemoveWorkspaceMemberFn(ctx, userID, workspaceID)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) RemoveNamespaceMember(ctx context.Context, userID, namespaceID int64) error {
	if m.RemoveNamespaceMemberFn != nil {
		return m.RemoveNamespaceMemberFn(ctx, userID, namespaceID)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListWorkspaceMembers(ctx context.Context, workspaceID int64, query list.Query) (*list.Result[modstore.UserWithRoleRow], error) {
	if m.ListWorkspaceMembersFn != nil {
		return m.ListWorkspaceMembersFn(ctx, workspaceID, query)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListNamespaceMembers(ctx context.Context, namespaceID int64, query list.Query) (*list.Result[modstore.UserWithRoleRow], error) {
	if m.ListNamespaceMembersFn != nil {
		return m.ListNamespaceMembersFn(ctx, namespaceID, query)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListWorkspaceNonMembers(ctx context.Context, workspaceID int64, query list.Query) (*list.Result[modstore.UserRow], error) {
	if m.ListWorkspaceNonMembersFn != nil {
		return m.ListWorkspaceNonMembersFn(ctx, workspaceID, query)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListNamespaceNonMembers(ctx context.Context, namespaceID int64, query list.Query) (*list.Result[modstore.UserRow], error) {
	if m.ListNamespaceNonMembersFn != nil {
		return m.ListNamespaceNonMembersFn(ctx, namespaceID, query)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListUserWorkspaces(ctx context.Context, userID int64, query list.Query) (*list.Result[modstore.WorkspaceWithOwnerAndRoleRow], error) {
	if m.ListUserWorkspacesFn != nil {
		return m.ListUserWorkspacesFn(ctx, userID, query)
	}
	panic("not implemented")
}
func (m *mockRoleBindingStore) ListUserNamespaces(ctx context.Context, userID int64, query list.Query) (*list.Result[modstore.NamespaceWithOwnerAndRoleRow], error) {
	if m.ListUserNamespacesFn != nil {
		return m.ListUserNamespacesFn(ctx, userID, query)
	}
	panic("not implemented")
}

// --- Test data helpers ---

var testTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// testUser creates a modstore.UserRow with sensible defaults for testing.
func testUser(id int64, username, email string) *modstore.UserRow {
	return &modstore.UserRow{
		ID:           id,
		Username:     username,
		Email:        email,
		DisplayName:  username,
		Phone:        "",
		AvatarURL:    "",
		Status:       "active",
		PasswordHash: "",
		CreatedAt:    testTime,
		UpdatedAt:    testTime,
	}
}

// testWorkspace creates a modstore.WorkspaceRow with sensible defaults for testing.
func testWorkspace(id int64, name string, ownerID int64) *modstore.WorkspaceRow {
	return &modstore.WorkspaceRow{
		ID:          id,
		Name:        name,
		DisplayName: name,
		Description: "",
		OwnerID:     ownerID,
		Status:      "active",
		CreatedAt:   testTime,
		UpdatedAt:   testTime,
	}
}

// testNamespace creates a modstore.NamespaceRow with sensible defaults for testing.
func testNamespace(id int64, name string, workspaceID, ownerID int64) *modstore.NamespaceRow {
	return &modstore.NamespaceRow{
		ID:          id,
		Name:        name,
		DisplayName: name,
		Description: "",
		WorkspaceID: workspaceID,
		OwnerID:     ownerID,
		Visibility:  "private",
		MaxMembers:  0,
		Status:      "active",
		CreatedAt:   testTime,
		UpdatedAt:   testTime,
	}
}

// testWorkspaceWithOwner creates a modstore.WorkspaceWithOwnerRow for testing.
func testWorkspaceWithOwner(id int64, name string, ownerID int64, ownerUsername string) *modstore.WorkspaceWithOwnerRow {
	return &modstore.WorkspaceWithOwnerRow{
		WorkspaceRow: modstore.WorkspaceRow{
			ID:          id,
			Name:        name,
			DisplayName: name,
			Description: "",
			OwnerID:     ownerID,
			Status:      "active",
			CreatedAt:   testTime,
			UpdatedAt:   testTime,
		},
		OwnerUsername:  ownerUsername,
		NamespaceCount: 0,
		MemberCount:    0,
	}
}

// testNamespaceWithOwner creates a modstore.NamespaceWithOwnerRow for testing.
func testNamespaceWithOwner(id int64, name string, workspaceID, ownerID int64, ownerUsername, workspaceName string) *modstore.NamespaceWithOwnerRow {
	return &modstore.NamespaceWithOwnerRow{
		NamespaceRow: modstore.NamespaceRow{
			ID:          id,
			Name:        name,
			DisplayName: name,
			Description: "",
			WorkspaceID: workspaceID,
			OwnerID:     ownerID,
			Visibility:  "private",
			MaxMembers:  0,
			Status:      "active",
			CreatedAt:   testTime,
			UpdatedAt:   testTime,
		},
		OwnerUsername: ownerUsername,
		WorkspaceName: workspaceName,
		MemberCount:   0,
	}
}
