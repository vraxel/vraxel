package iam

import (
	"context"
	"testing"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// testCtx builds a typed request context around context.Background().
func testCtx() apiserver.Ctx {
	return apiserver.Ctx{Context: context.Background()}
}

// --- TestUserOps_Get ---

func TestUserOps_Get(t *testing.T) {
	dbUser := testUser(1, "alice", "alice@example.com")

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			if id != 1 {
				t.Fatalf("expected id 1, got %d", id)
			}
			return dbUser, nil
		},
	}

	ops := userOps{dbStore: userStore}

	user, err := ops.get(testCtx(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", user.ObjectMeta.ID)
	}
	if user.ObjectMeta.Name != "alice" {
		t.Errorf("expected Name 'alice', got %q", user.ObjectMeta.Name)
	}
	if user.Spec.Username != "alice" {
		t.Errorf("expected Username 'alice', got %q", user.Spec.Username)
	}
	if user.Spec.Email != "alice@example.com" {
		t.Errorf("expected Email 'alice@example.com', got %q", user.Spec.Email)
	}
	if user.Spec.Status != "active" {
		t.Errorf("expected Status 'active', got %q", user.Spec.Status)
	}
	if user.TypeMeta.Kind != "User" {
		t.Errorf("expected Kind 'User', got %q", user.TypeMeta.Kind)
	}
}

func TestUserOps_Get_NotFound(t *testing.T) {
	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return nil, apierrors.NewNotFound("user", "999")
		},
	}

	ops := userOps{dbStore: userStore}

	_, err := ops.get(testCtx(), 999)
	if err == nil {
		t.Fatal("expected error for not found user, got nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

// --- TestUserOps_List ---

func TestUserOps_List(t *testing.T) {
	userStore := &mockUserStore{
		ListFn: func(ctx context.Context, query list.Query) (*list.Result[modstore.UserWithNamespacesRow], error) {
			return &list.Result[modstore.UserWithNamespacesRow]{
				Items: []modstore.UserWithNamespacesRow{
					{
						UserRow:        *testUser(1, "alice", "alice@example.com"),
						NamespaceNames: []string{"ns-one", "ns-two"},
					},
					{
						UserRow:        *testUser(2, "bob", "bob@example.com"),
						NamespaceNames: nil,
					},
				},
				TotalCount: 2,
			}, nil
		},
	}

	ops := userOps{dbStore: userStore}

	result, err := ops.list(testCtx(), list.Query{
		Filters: map[string]any{"status": "active"},
		Pagination: list.Pagination{
			Page:      1,
			PageSize:  20,
			SortBy:    "username",
			SortOrder: "asc",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalCount != 2 {
		t.Errorf("expected TotalCount 2, got %d", result.TotalCount)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}

	// First user should have namespace names
	if len(result.Items[0].Spec.Namespaces) != 2 {
		t.Errorf("expected 2 namespaces for first user, got %d", len(result.Items[0].Spec.Namespaces))
	}
	if result.Items[0].Spec.Username != "alice" {
		t.Errorf("expected first user 'alice', got %q", result.Items[0].Spec.Username)
	}

	// Second user should have no namespaces
	if len(result.Items[1].Spec.Namespaces) != 0 {
		t.Errorf("expected 0 namespaces for second user, got %d", len(result.Items[1].Spec.Namespaces))
	}
}

// --- TestUserOps_Create ---

func TestUserOps_Create(t *testing.T) {
	createdUser := testUser(1, "alice", "alice@example.com")
	createdUser.Phone = "13800138000"

	userStore := &mockUserStore{
		CreateFn: func(ctx context.Context, user modstore.UserCreateInput) (*modstore.UserRow, error) {
			if user.Username != "alice" {
				t.Errorf("expected username 'alice', got %q", user.Username)
			}
			if user.Email != "alice@example.com" {
				t.Errorf("expected email 'alice@example.com', got %q", user.Email)
			}
			return createdUser, nil
		},
	}

	ops := userOps{dbStore: userStore}

	inputUser := &User{
		Spec: UserSpec{
			Username: "alice",
			Email:    "alice@example.com",
			Phone:    "13800138000",
			Status:   "active",
		},
	}

	result, err := ops.create(testCtx(), inputUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
	if result.Spec.Username != "alice" {
		t.Errorf("expected Username 'alice', got %q", result.Spec.Username)
	}
	if result.TypeMeta.Kind != "User" {
		t.Errorf("expected Kind 'User', got %q", result.TypeMeta.Kind)
	}
}

func TestUserOps_Create_ValidationFails(t *testing.T) {
	ops := userOps{dbStore: &mockUserStore{}}

	// Missing required fields: username, email, phone
	inputUser := &User{
		Spec: UserSpec{},
	}

	_, err := ops.create(testCtx(), inputUser)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != 400 {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
}

func TestUserOps_Create_WithPassword(t *testing.T) {
	createdUser := testUser(1, "alice", "alice@example.com")
	createdUser.Phone = "13800138000"

	var capturedHash string
	var setPasswordCalled bool

	userStore := &mockUserStore{
		CreateFn: func(ctx context.Context, user modstore.UserCreateInput) (*modstore.UserRow, error) {
			return createdUser, nil
		},
		SetPasswordHashFn: func(ctx context.Context, id int64, hash string) error {
			setPasswordCalled = true
			capturedHash = hash
			if id != 1 {
				t.Errorf("expected user ID 1, got %d", id)
			}
			return nil
		},
	}

	hashCalled := false
	hasher := func(password string) (string, error) {
		hashCalled = true
		if password != "Test1234" {
			t.Errorf("expected password 'Test1234', got %q", password)
		}
		return "hashed-Test1234", nil
	}

	ops := userOps{dbStore: userStore, hashPasswd: hasher}

	inputUser := &User{
		Spec: UserSpec{
			Username: "alice",
			Email:    "alice@example.com",
			Phone:    "13800138000",
			Password: "Test1234",
			Status:   "active",
		},
	}

	result, err := ops.create(testCtx(), inputUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hashCalled {
		t.Error("expected password hasher to be called")
	}
	if !setPasswordCalled {
		t.Error("expected SetPasswordHash to be called")
	}
	if capturedHash != "hashed-Test1234" {
		t.Errorf("expected hash 'hashed-Test1234', got %q", capturedHash)
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
}

// --- TestUserOps_Update ---

func TestUserOps_Update(t *testing.T) {
	updatedUser := testUser(1, "alice_new", "alice_new@example.com")
	updatedUser.Phone = "13800138001"

	userStore := &mockUserStore{
		UpdateFn: func(ctx context.Context, user modstore.UserUpdateInput) (*modstore.UserRow, error) {
			if user.ID != 1 {
				t.Errorf("expected user ID 1, got %d", user.ID)
			}
			if user.Username != "alice_new" {
				t.Errorf("expected username 'alice_new', got %q", user.Username)
			}
			return updatedUser, nil
		},
	}

	ops := userOps{dbStore: userStore}

	inputUser := &User{
		Spec: UserSpec{
			Username: "alice_new",
			Email:    "alice_new@example.com",
			Phone:    "13800138001",
			Status:   "active",
		},
	}

	result, err := ops.update(testCtx(), 1, inputUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
	if result.Spec.Username != "alice_new" {
		t.Errorf("expected Username 'alice_new', got %q", result.Spec.Username)
	}
	if result.Spec.Email != "alice_new@example.com" {
		t.Errorf("expected Email 'alice_new@example.com', got %q", result.Spec.Email)
	}
}

// --- TestUserOps_Patch ---

func TestUserOps_Patch(t *testing.T) {
	patchedUser := testUser(1, "alice", "alice@example.com")
	patchedUser.DisplayName = "Alice Updated"

	userStore := &mockUserStore{
		PatchFn: func(ctx context.Context, user modstore.UserPatchInput) (*modstore.UserRow, error) {
			if user.ID != 1 {
				t.Errorf("expected id 1, got %d", user.ID)
			}
			if user.DisplayName == nil || *user.DisplayName != "Alice Updated" {
				t.Errorf("expected DisplayName 'Alice Updated', got %v", user.DisplayName)
			}
			return patchedUser, nil
		},
	}

	ops := userOps{dbStore: userStore}

	result, err := ops.patch(testCtx(), 1, []byte(`{"spec":{"displayName":"Alice Updated"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Spec.DisplayName != "Alice Updated" {
		t.Errorf("expected DisplayName 'Alice Updated', got %q", result.Spec.DisplayName)
	}
	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
}

// --- TestUserOps_Delete ---

func TestUserOps_Delete(t *testing.T) {
	deleteCalled := false

	userStore := &mockUserStore{
		DeleteFn: func(ctx context.Context, id int64) error {
			deleteCalled = true
			if id != 1 {
				t.Errorf("expected id 1, got %d", id)
			}
			return nil
		},
	}

	ops := userOps{dbStore: userStore}

	if err := ops.delete(testCtx(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deleteCalled {
		t.Error("expected Delete to be called on the store")
	}
}

// --- TestUserOps_BatchDelete ---

func TestUserOps_BatchDelete(t *testing.T) {
	userStore := &mockUserStore{
		DeleteByIDsFn: func(ctx context.Context, ids []int64) (int64, error) {
			if len(ids) != 3 {
				t.Errorf("expected 3 IDs, got %d", len(ids))
			}
			// Simulate that 2 out of 3 were actually deleted
			return 2, nil
		},
	}

	ops := userOps{dbStore: userStore}

	result, err := ops.batchDelete(testCtx(), []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SuccessCount != 2 {
		t.Errorf("expected SuccessCount 2, got %d", result.SuccessCount)
	}
	if result.FailedCount != 1 {
		t.Errorf("expected FailedCount 1, got %d", result.FailedCount)
	}
}
