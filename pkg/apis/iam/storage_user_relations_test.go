package iam

import (
	"context"
	"testing"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// wsCtx builds a typed context at workspace scope.
func wsCtx(wsID int64) apiserver.Ctx {
	c := testCtx()
	c.Scope = apiserver.ScopeInfo{Level: apiserver.ScopeWorkspace, WorkspaceID: wsID}
	return c
}

// nsCtx builds a typed context at namespace scope.
func nsCtx(wsID, nsID int64) apiserver.Ctx {
	c := testCtx()
	c.Scope = apiserver.ScopeInfo{Level: apiserver.ScopeNamespace, WorkspaceID: wsID, NamespaceID: nsID}
	return c
}

// ===== workspaceUserOps tests =====

// --- TestWorkspaceUserOps_List ---

func TestWorkspaceUserOps_List(t *testing.T) {
	rbStore := &mockRoleBindingStore{
		ListWorkspaceMembersFn: func(ctx context.Context, workspaceID int64, query list.Query) (*list.Result[modstore.UserWithRoleRow], error) {
			if workspaceID != 1 {
				t.Errorf("expected workspaceID 1, got %d", workspaceID)
			}
			return &list.Result[modstore.UserWithRoleRow]{
				Items: []modstore.UserWithRoleRow{
					{
						UserRow: modstore.UserRow{
							ID:          10,
							Username:    "alice",
							Email:       "alice@example.com",
							DisplayName: "Alice",
							Status:      "active",
							CreatedAt:   testTime,
							UpdatedAt:   testTime,
						},
						Role:     modstore.RoleWorkspaceAdmin,
						JoinedAt: testTime,
					},
					{
						UserRow: modstore.UserRow{
							ID:          20,
							Username:    "bob",
							Email:       "bob@example.com",
							DisplayName: "Bob",
							Status:      "active",
							CreatedAt:   testTime,
							UpdatedAt:   testTime,
						},
						Role:     modstore.RoleWorkspaceViewer,
						JoinedAt: testTime,
					},
				},
				TotalCount: 2,
			}, nil
		},
	}

	ops := workspaceUserOps{rbStore: rbStore}

	result, err := ops.list(wsCtx(1), list.Query{
		Filters:    map[string]any{},
		Pagination: list.Pagination{Page: 1, PageSize: 20},
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

	// Verify first user
	if result.Items[0].ObjectMeta.ID != "10" {
		t.Errorf("expected first item ID '10', got %q", result.Items[0].ObjectMeta.ID)
	}
	if result.Items[0].ObjectMeta.Name != "alice" {
		t.Errorf("expected first item Name 'alice', got %q", result.Items[0].ObjectMeta.Name)
	}
	if result.Items[0].Spec.Email != "alice@example.com" {
		t.Errorf("expected first item Email 'alice@example.com', got %q", result.Items[0].Spec.Email)
	}

	// Verify second user
	if result.Items[1].ObjectMeta.ID != "20" {
		t.Errorf("expected second item ID '20', got %q", result.Items[1].ObjectMeta.ID)
	}
	if result.Items[1].ObjectMeta.Name != "bob" {
		t.Errorf("expected second item Name 'bob', got %q", result.Items[1].ObjectMeta.Name)
	}
}

// --- TestWorkspaceUserOps_Create ---

func TestWorkspaceUserOps_Create(t *testing.T) {
	var addCalls []int64
	var getUserCalls []int64

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			getUserCalls = append(getUserCalls, id)
			return testUser(id, "user", "user@example.com"), nil
		},
	}

	rbStore := &mockRoleBindingStore{
		AddWorkspaceMemberFn: func(ctx context.Context, userID, workspaceID int64, roleID int64) error {
			addCalls = append(addCalls, userID)
			if workspaceID != 1 {
				t.Errorf("expected workspaceID 1, got %d", workspaceID)
			}
			return nil
		},
	}

	ops := workspaceUserOps{rbStore: rbStore, userStore: userStore}

	obj, err := ops.create(wsCtx(1), []byte(`{"ids":["10","20","30"]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify each user was verified
	if len(getUserCalls) != 3 {
		t.Errorf("expected 3 userStore.GetByID calls, got %d", len(getUserCalls))
	}

	// Verify each user was added
	if len(addCalls) != 3 {
		t.Errorf("expected 3 rbStore.AddWorkspaceMember calls, got %d", len(addCalls))
	}

	result, ok := obj.(*apiserver.BatchResult)
	if !ok {
		t.Fatalf("expected *apiserver.BatchResult, got %T", obj)
	}
	if result.SuccessCount != 3 {
		t.Errorf("expected SuccessCount 3, got %d", result.SuccessCount)
	}
}

// --- TestWorkspaceUserOps_Create_UserNotFound ---

func TestWorkspaceUserOps_Create_UserNotFound(t *testing.T) {
	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return nil, apierrors.NewNotFound("user", "999")
		},
	}

	rbStore := &mockRoleBindingStore{}

	ops := workspaceUserOps{rbStore: rbStore, userStore: userStore}

	_, err := ops.create(wsCtx(1), []byte(`{"ids":["999"]}`))
	if err == nil {
		t.Fatal("expected error when user not found, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != 400 {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
}

// --- TestWorkspaceUserOps_BatchDelete ---

func TestWorkspaceUserOps_BatchDelete(t *testing.T) {
	var removeCalls []int64

	rbStore := &mockRoleBindingStore{
		RemoveWorkspaceMemberFn: func(ctx context.Context, userID, workspaceID int64) error {
			removeCalls = append(removeCalls, userID)
			if workspaceID != 1 {
				t.Errorf("expected workspaceID 1, got %d", workspaceID)
			}
			return nil
		},
	}

	ops := workspaceUserOps{rbStore: rbStore}

	result, err := ops.batchDelete(wsCtx(1), []int64{10, 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(removeCalls) != 2 {
		t.Errorf("expected 2 rbStore.RemoveWorkspaceMember calls, got %d", len(removeCalls))
	}
	if removeCalls[0] != 10 {
		t.Errorf("expected first Remove call with userID 10, got %d", removeCalls[0])
	}
	if removeCalls[1] != 20 {
		t.Errorf("expected second Remove call with userID 20, got %d", removeCalls[1])
	}

	if result.SuccessCount != 2 {
		t.Errorf("expected SuccessCount 2, got %d", result.SuccessCount)
	}
}

// ===== namespaceUserOps tests =====

// --- TestNamespaceUserOps_List ---

func TestNamespaceUserOps_List(t *testing.T) {
	rbStore := &mockRoleBindingStore{
		ListNamespaceMembersFn: func(ctx context.Context, namespaceID int64, query list.Query) (*list.Result[modstore.UserWithRoleRow], error) {
			if namespaceID != 5 {
				t.Errorf("expected namespaceID 5, got %d", namespaceID)
			}
			return &list.Result[modstore.UserWithRoleRow]{
				Items: []modstore.UserWithRoleRow{
					{
						UserRow: modstore.UserRow{
							ID:          10,
							Username:    "alice",
							Email:       "alice@example.com",
							DisplayName: "Alice",
							Status:      "active",
							CreatedAt:   testTime,
							UpdatedAt:   testTime,
						},
						Role:     modstore.RoleNamespaceAdmin,
						JoinedAt: testTime,
					},
					{
						UserRow: modstore.UserRow{
							ID:          20,
							Username:    "bob",
							Email:       "bob@example.com",
							DisplayName: "Bob",
							Status:      "active",
							CreatedAt:   testTime,
							UpdatedAt:   testTime,
						},
						Role:     modstore.RoleNamespaceViewer,
						JoinedAt: testTime,
					},
				},
				TotalCount: 2,
			}, nil
		},
	}

	ops := namespaceUserOps{rbStore: rbStore}

	result, err := ops.list(nsCtx(1, 5), list.Query{
		Filters:    map[string]any{},
		Pagination: list.Pagination{Page: 1, PageSize: 20},
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

	// Verify first user
	if result.Items[0].ObjectMeta.ID != "10" {
		t.Errorf("expected first item ID '10', got %q", result.Items[0].ObjectMeta.ID)
	}
	if result.Items[0].ObjectMeta.Name != "alice" {
		t.Errorf("expected first item Name 'alice', got %q", result.Items[0].ObjectMeta.Name)
	}
	if result.Items[0].Spec.Email != "alice@example.com" {
		t.Errorf("expected first item Email 'alice@example.com', got %q", result.Items[0].Spec.Email)
	}

	// Verify second user
	if result.Items[1].ObjectMeta.ID != "20" {
		t.Errorf("expected second item ID '20', got %q", result.Items[1].ObjectMeta.ID)
	}
	if result.Items[1].Spec.Username != "bob" {
		t.Errorf("expected second item Username 'bob', got %q", result.Items[1].Spec.Username)
	}
}

// --- TestNamespaceUserOps_Create ---

func TestNamespaceUserOps_Create(t *testing.T) {
	var addCalls []int64
	var getUserCalls []int64

	nsStore := &mockNamespaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.NamespaceWithOwnerRow, error) {
			if id != 5 {
				t.Errorf("expected namespace ID 5, got %d", id)
			}
			// MaxMembers=0 means unlimited
			return testNamespaceWithOwner(5, "my-ns", 1, 10, "alice", "my-ws"), nil
		},
		CountUsersFn: func(ctx context.Context, namespaceID int64) (int64, error) {
			t.Error("CountUsers should not be called when MaxMembers is 0")
			return 0, nil
		},
	}

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			getUserCalls = append(getUserCalls, id)
			return testUser(id, "user", "user@example.com"), nil
		},
	}

	rbStore := &mockRoleBindingStore{
		AddNamespaceMemberFn: func(ctx context.Context, userID, namespaceID int64, roleID int64) error {
			addCalls = append(addCalls, userID)
			if namespaceID != 5 {
				t.Errorf("expected namespaceID 5, got %d", namespaceID)
			}
			return nil
		},
	}

	ops := namespaceUserOps{rbStore: rbStore, nsStore: nsStore, userStore: userStore}

	obj, err := ops.create(nsCtx(1, 5), []byte(`{"ids":["10","20"]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify each user was verified
	if len(getUserCalls) != 2 {
		t.Errorf("expected 2 userStore.GetByID calls, got %d", len(getUserCalls))
	}

	// Verify each user was added
	if len(addCalls) != 2 {
		t.Errorf("expected 2 rbStore.AddNamespaceMember calls, got %d", len(addCalls))
	}

	result, ok := obj.(*apiserver.BatchResult)
	if !ok {
		t.Fatalf("expected *apiserver.BatchResult, got %T", obj)
	}
	if result.SuccessCount != 2 {
		t.Errorf("expected SuccessCount 2, got %d", result.SuccessCount)
	}
}

// --- TestNamespaceUserOps_Create_ExceedsMaxUsers ---

func TestNamespaceUserOps_Create_ExceedsMaxUsers(t *testing.T) {
	nsWithOwner := testNamespaceWithOwner(5, "my-ns", 1, 10, "alice", "my-ws")
	nsWithOwner.MaxMembers = 5 // max 5 members

	nsStore := &mockNamespaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.NamespaceWithOwnerRow, error) {
			return nsWithOwner, nil
		},
		CountUsersFn: func(ctx context.Context, namespaceID int64) (int64, error) {
			return 3, nil // currently 3 members
		},
	}

	ops := namespaceUserOps{rbStore: &mockRoleBindingStore{}, nsStore: nsStore, userStore: &mockUserStore{}}

	// Try to add 3 users when there are already 3 and max is 5 (3+3=6 > 5)
	_, err := ops.create(nsCtx(1, 5), []byte(`{"ids":["20","30","40"]}`))
	if err == nil {
		t.Fatal("expected error when exceeding max members, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != 400 {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
}

// --- TestNamespaceUserOps_Create_WithinMaxUsers ---

func TestNamespaceUserOps_Create_WithinMaxUsers(t *testing.T) {
	nsWithOwner := testNamespaceWithOwner(5, "my-ns", 1, 10, "alice", "my-ws")
	nsWithOwner.MaxMembers = 10 // max 10 members

	var countUsersCalled bool

	nsStore := &mockNamespaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.NamespaceWithOwnerRow, error) {
			return nsWithOwner, nil
		},
		CountUsersFn: func(ctx context.Context, namespaceID int64) (int64, error) {
			countUsersCalled = true
			return 3, nil // currently 3 members, adding 2 = 5 <= 10
		},
	}

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return testUser(id, "user", "user@example.com"), nil
		},
	}

	rbStore := &mockRoleBindingStore{
		AddNamespaceMemberFn: func(ctx context.Context, userID, namespaceID int64, roleID int64) error {
			return nil
		},
	}

	ops := namespaceUserOps{rbStore: rbStore, nsStore: nsStore, userStore: userStore}

	obj, err := ops.create(nsCtx(1, 5), []byte(`{"ids":["20","30"]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !countUsersCalled {
		t.Error("expected nsStore.CountUsers to be called when MaxMembers > 0")
	}

	result, ok := obj.(*apiserver.BatchResult)
	if !ok {
		t.Fatalf("expected *apiserver.BatchResult, got %T", obj)
	}
	if result.SuccessCount != 2 {
		t.Errorf("expected SuccessCount 2, got %d", result.SuccessCount)
	}
}

// --- TestNamespaceUserOps_Create_UserNotFound ---

func TestNamespaceUserOps_Create_UserNotFound(t *testing.T) {
	nsStore := &mockNamespaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.NamespaceWithOwnerRow, error) {
			return testNamespaceWithOwner(5, "my-ns", 1, 10, "alice", "my-ws"), nil
		},
	}

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return nil, apierrors.NewNotFound("user", "999")
		},
	}

	ops := namespaceUserOps{rbStore: &mockRoleBindingStore{}, nsStore: nsStore, userStore: userStore}

	_, err := ops.create(nsCtx(1, 5), []byte(`{"ids":["999"]}`))
	if err == nil {
		t.Fatal("expected error when user not found, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != 400 {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
}

// --- TestNamespaceUserOps_BatchDelete ---

func TestNamespaceUserOps_BatchDelete(t *testing.T) {
	var removeCalls []int64

	rbStore := &mockRoleBindingStore{
		RemoveNamespaceMemberFn: func(ctx context.Context, userID, namespaceID int64) error {
			removeCalls = append(removeCalls, userID)
			if namespaceID != 5 {
				t.Errorf("expected namespaceID 5, got %d", namespaceID)
			}
			return nil
		},
	}

	ops := namespaceUserOps{rbStore: rbStore}

	result, err := ops.batchDelete(nsCtx(1, 5), []int64{10, 20, 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(removeCalls) != 3 {
		t.Errorf("expected 3 rbStore.RemoveNamespaceMember calls, got %d", len(removeCalls))
	}
	expectedIDs := []int64{10, 20, 30}
	for i, id := range removeCalls {
		if id != expectedIDs[i] {
			t.Errorf("expected Remove call %d with userID %d, got %d", i, expectedIDs[i], id)
		}
	}

	if result.SuccessCount != 3 {
		t.Errorf("expected SuccessCount 3, got %d", result.SuccessCount)
	}
}
