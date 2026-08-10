package iam

import (
	"context"
	"strings"
	"testing"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// --- TestWorkspaceOps_Get ---

func TestWorkspaceOps_Get(t *testing.T) {
	wsWithOwner := testWorkspaceWithOwner(1, "my-workspace", 10, "alice")
	wsWithOwner.NamespaceCount = 3
	wsWithOwner.MemberCount = 5

	wsStore := &mockWorkspaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.WorkspaceWithOwnerRow, error) {
			if id != 1 {
				t.Fatalf("expected id 1, got %d", id)
			}
			return wsWithOwner, nil
		},
	}

	ops := workspaceOps{wsStore: wsStore}

	ws, err := ops.get(testCtx(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ws.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", ws.ObjectMeta.ID)
	}
	if ws.ObjectMeta.Name != "my-workspace" {
		t.Errorf("expected Name 'my-workspace', got %q", ws.ObjectMeta.Name)
	}
	if ws.TypeMeta.Kind != "Workspace" {
		t.Errorf("expected Kind 'Workspace', got %q", ws.TypeMeta.Kind)
	}
	if ws.Spec.OwnerID != "10" {
		t.Errorf("expected OwnerID '10', got %q", ws.Spec.OwnerID)
	}
	if ws.Spec.OwnerName != "alice" {
		t.Errorf("expected OwnerName 'alice', got %q", ws.Spec.OwnerName)
	}
	if ws.Spec.NamespaceCount != 3 {
		t.Errorf("expected NamespaceCount 3, got %d", ws.Spec.NamespaceCount)
	}
	if ws.Spec.MemberCount != 5 {
		t.Errorf("expected MemberCount 5, got %d", ws.Spec.MemberCount)
	}
	if ws.Spec.Status != "active" {
		t.Errorf("expected Status 'active', got %q", ws.Spec.Status)
	}
}

// --- TestWorkspaceOps_List ---

func TestWorkspaceOps_List(t *testing.T) {
	wsStore := &mockWorkspaceStore{
		ListFn: func(ctx context.Context, query list.Query) (*list.Result[modstore.WorkspaceWithOwnerRow], error) {
			return &list.Result[modstore.WorkspaceWithOwnerRow]{
				Items: []modstore.WorkspaceWithOwnerRow{
					{
						WorkspaceRow: modstore.WorkspaceRow{
							ID:          1,
							Name:        "ws-one",
							DisplayName: "Workspace One",
							OwnerID:     10,
							Status:      "active",
							CreatedAt:   testTime,
							UpdatedAt:   testTime,
						},
						OwnerUsername:  "alice",
						NamespaceCount: 2,
						MemberCount:    3,
					},
					{
						WorkspaceRow: modstore.WorkspaceRow{
							ID:          2,
							Name:        "ws-two",
							DisplayName: "Workspace Two",
							OwnerID:     20,
							Status:      "active",
							CreatedAt:   testTime,
							UpdatedAt:   testTime,
						},
						OwnerUsername:  "bob",
						NamespaceCount: 1,
						MemberCount:    1,
					},
				},
				TotalCount: 2,
			}, nil
		},
	}

	ops := workspaceOps{wsStore: wsStore}

	result, err := ops.list(testCtx(), list.Query{
		Filters: map[string]any{"status": "active"},
		Pagination: list.Pagination{
			Page:      1,
			PageSize:  20,
			SortBy:    "name",
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

	// Verify first workspace
	if result.Items[0].ObjectMeta.ID != "1" {
		t.Errorf("expected first item ID '1', got %q", result.Items[0].ObjectMeta.ID)
	}
	if result.Items[0].ObjectMeta.Name != "ws-one" {
		t.Errorf("expected first item Name 'ws-one', got %q", result.Items[0].ObjectMeta.Name)
	}
	if result.Items[0].Spec.OwnerName != "alice" {
		t.Errorf("expected first item OwnerName 'alice', got %q", result.Items[0].Spec.OwnerName)
	}
	if result.Items[0].Spec.NamespaceCount != 2 {
		t.Errorf("expected first item NamespaceCount 2, got %d", result.Items[0].Spec.NamespaceCount)
	}
	if result.Items[0].Spec.MemberCount != 3 {
		t.Errorf("expected first item MemberCount 3, got %d", result.Items[0].Spec.MemberCount)
	}

	// Verify second workspace
	if result.Items[1].ObjectMeta.ID != "2" {
		t.Errorf("expected second item ID '2', got %q", result.Items[1].ObjectMeta.ID)
	}
	if result.Items[1].Spec.OwnerName != "bob" {
		t.Errorf("expected second item OwnerName 'bob', got %q", result.Items[1].Spec.OwnerName)
	}
}

// --- TestWorkspaceOps_Create ---

func TestWorkspaceOps_Create(t *testing.T) {
	var createCalled, userGetByIDCalled bool

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			userGetByIDCalled = true
			if id != 10 {
				t.Errorf("expected owner id 10, got %d", id)
			}
			return testUser(10, "alice", "alice@example.com"), nil
		},
	}

	wsStore := &mockWorkspaceStore{
		CreateFn: func(ctx context.Context, ws modstore.WorkspaceCreateInput) (*modstore.WorkspaceWithOwnerRow, error) {
			createCalled = true
			if ws.Name != "new-workspace" {
				t.Errorf("expected name 'new-workspace', got %q", ws.Name)
			}
			if ws.DisplayName != "New Workspace" {
				t.Errorf("expected displayName 'New Workspace', got %q", ws.DisplayName)
			}
			if ws.OwnerID != 10 {
				t.Errorf("expected ownerID 10, got %d", ws.OwnerID)
			}
			if ws.Status != "active" {
				t.Errorf("expected status 'active', got %q", ws.Status)
			}
			return testWorkspaceWithOwner(1, "new-workspace", 10, "alice"), nil
		},
	}

	ops := workspaceOps{wsStore: wsStore, userStore: userStore}

	inputWs := &Workspace{
		Spec: WorkspaceSpec{
			DisplayName: "New Workspace",
			Description: "A test workspace",
			OwnerID:     "10",
			Status:      "active",
		},
	}
	inputWs.ObjectMeta.Name = "new-workspace"

	result, err := ops.create(testCtx(), inputWs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !userGetByIDCalled {
		t.Error("expected userStore.GetByID to be called to verify owner")
	}
	if !createCalled {
		t.Error("expected wsStore.Create to be called")
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
	if result.ObjectMeta.Name != "new-workspace" {
		t.Errorf("expected Name 'new-workspace', got %q", result.ObjectMeta.Name)
	}
	if result.TypeMeta.Kind != "Workspace" {
		t.Errorf("expected Kind 'Workspace', got %q", result.TypeMeta.Kind)
	}
	if result.Spec.OwnerName != "alice" {
		t.Errorf("expected OwnerName 'alice', got %q", result.Spec.OwnerName)
	}
}

// --- TestWorkspaceOps_Create_OwnerNotFound ---

func TestWorkspaceOps_Create_OwnerNotFound(t *testing.T) {
	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			return nil, apierrors.NewNotFound("user", "999")
		},
	}

	wsStore := &mockWorkspaceStore{}

	ops := workspaceOps{wsStore: wsStore, userStore: userStore}

	inputWs := &Workspace{
		Spec: WorkspaceSpec{
			DisplayName: "Test Workspace",
			OwnerID:     "999",
			Status:      "active",
		},
	}
	inputWs.ObjectMeta.Name = "test-workspace"

	_, err := ops.create(testCtx(), inputWs)
	if err == nil {
		t.Fatal("expected error when owner not found, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != 400 {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
}

// --- TestWorkspaceOps_Update ---

func TestWorkspaceOps_Update(t *testing.T) {
	updatedDBWs := testWorkspaceWithOwner(1, "updated-workspace", 10, "owner")
	updatedDBWs.DisplayName = "Updated Workspace"
	updatedDBWs.Description = "Updated description"

	wsStore := &mockWorkspaceStore{
		UpdateFn: func(ctx context.Context, ws modstore.WorkspaceUpdateInput) (*modstore.WorkspaceWithOwnerRow, error) {
			if ws.ID != 1 {
				t.Errorf("expected workspace ID 1, got %d", ws.ID)
			}
			if ws.DisplayName != "Updated Workspace" {
				t.Errorf("expected displayName 'Updated Workspace', got %q", ws.DisplayName)
			}
			if ws.OwnerID != 10 {
				t.Errorf("expected ownerID 10, got %d", ws.OwnerID)
			}
			return updatedDBWs, nil
		},
	}

	ops := workspaceOps{wsStore: wsStore}

	inputWs := &Workspace{
		Spec: WorkspaceSpec{
			DisplayName: "Updated Workspace",
			Description: "Updated description",
			OwnerID:     "10",
			Status:      "active",
		},
	}
	inputWs.ObjectMeta.Name = "updated-workspace"

	result, err := ops.update(testCtx(), 1, inputWs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
	if result.Spec.DisplayName != "Updated Workspace" {
		t.Errorf("expected DisplayName 'Updated Workspace', got %q", result.Spec.DisplayName)
	}
	if result.Spec.Description != "Updated description" {
		t.Errorf("expected Description 'Updated description', got %q", result.Spec.Description)
	}
	if result.TypeMeta.Kind != "Workspace" {
		t.Errorf("expected Kind 'Workspace', got %q", result.TypeMeta.Kind)
	}
}

// --- TestWorkspaceOps_Patch ---

func TestWorkspaceOps_Patch(t *testing.T) {
	patchedDBWs := testWorkspaceWithOwner(1, "my-workspace", 10, "owner")
	patchedDBWs.DisplayName = "Patched Workspace"
	patchedDBWs.Description = "Original description"

	var patchCalled bool

	wsStore := &mockWorkspaceStore{
		PatchFn: func(ctx context.Context, ws modstore.WorkspaceUpdateInput) (*modstore.WorkspaceWithOwnerRow, error) {
			patchCalled = true
			if ws.ID != 1 {
				t.Errorf("expected id 1, got %d", ws.ID)
			}
			if ws.DisplayName != "Patched Workspace" {
				t.Errorf("expected displayName 'Patched Workspace', got %q", ws.DisplayName)
			}
			// Description should be empty (zero value) since patch input did not set it
			if ws.Description != "" {
				t.Errorf("expected description '' (zero value), got %q", ws.Description)
			}
			return patchedDBWs, nil
		},
	}

	ops := workspaceOps{wsStore: wsStore}

	// Patch only the displayName
	result, err := ops.patch(testCtx(), 1, []byte(`{"spec":{"displayName":"Patched Workspace"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !patchCalled {
		t.Error("expected wsStore.Patch to be called")
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
	if result.Spec.DisplayName != "Patched Workspace" {
		t.Errorf("expected DisplayName 'Patched Workspace', got %q", result.Spec.DisplayName)
	}
}

// --- TestWorkspaceOps_Delete ---

func TestWorkspaceOps_Delete(t *testing.T) {
	deleteCalled := false

	wsStore := &mockWorkspaceStore{
		DeleteFn: func(ctx context.Context, id int64) error {
			deleteCalled = true
			if id != 1 {
				t.Errorf("expected id 1, got %d", id)
			}
			return nil
		},
	}

	ops := workspaceOps{wsStore: wsStore}

	if err := ops.delete(testCtx(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deleteCalled {
		t.Error("expected Delete to be called on the store")
	}
}

// --- TestWorkspaceOps_BatchDelete ---

func TestWorkspaceOps_BatchDelete(t *testing.T) {
	wsStore := &mockWorkspaceStore{
		DeleteByIDsFn: func(ctx context.Context, ids []int64) (int64, error) {
			if len(ids) != 3 {
				t.Errorf("expected 3 IDs, got %d", len(ids))
			}
			expectedIDs := []int64{1, 2, 3}
			for i, id := range ids {
				if id != expectedIDs[i] {
					t.Errorf("expected ID %d at index %d, got %d", expectedIDs[i], i, id)
				}
			}
			// Simulate that 2 out of 3 were actually deleted
			return 2, nil
		},
	}

	ops := workspaceOps{wsStore: wsStore}

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

// --- TestWorkspaceOps_Delete_Blocked ---

func TestWorkspaceOps_Delete_Blocked(t *testing.T) {
	deleteCalled := false
	wsStore := &mockWorkspaceStore{
		ListBlockingResourcesFn: func(ctx context.Context, workspaceID int64) ([]modstore.BlockingResourceRow, error) {
			return []modstore.BlockingResourceRow{
				{Kind: "host", Count: 5},
				{Kind: "kafka_instance", Count: 1},
			}, nil
		},
		DeleteFn: func(ctx context.Context, id int64) error {
			deleteCalled = true
			return nil
		},
	}

	ops := workspaceOps{wsStore: wsStore}
	err := ops.delete(testCtx(), 1)
	if err == nil {
		t.Fatal("expected blocking error, got nil")
	}
	se, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if se.Status != 409 {
		t.Errorf("expected status 409 Conflict, got %d", se.Status)
	}
	wantPrefix := "cannot delete workspace:"
	if !strings.HasPrefix(se.Message, wantPrefix) {
		t.Errorf("message should start with %q, got %q", wantPrefix, se.Message)
	}
	if !strings.Contains(se.Message, "host(5)") || !strings.Contains(se.Message, "kafka_instance(1)") {
		t.Errorf("message missing kind(count) tokens: %q", se.Message)
	}
	if deleteCalled {
		t.Error("Delete should not be called when blocking resources exist")
	}
}

// --- TestWorkspaceOps_BatchDelete_Blocked ---

func TestWorkspaceOps_BatchDelete_Blocked(t *testing.T) {
	deleteByIDsCalled := false
	wsStore := &mockWorkspaceStore{
		ListBlockingResourcesFn: func(ctx context.Context, workspaceID int64) ([]modstore.BlockingResourceRow, error) {
			if workspaceID == 3 {
				return []modstore.BlockingResourceRow{{Kind: "service", Count: 2}}, nil
			}
			return nil, nil
		},
		DeleteByIDsFn: func(ctx context.Context, ids []int64) (int64, error) {
			deleteByIDsCalled = true
			return int64(len(ids)), nil
		},
	}

	ops := workspaceOps{wsStore: wsStore}
	_, err := ops.batchDelete(testCtx(), []int64{1, 2, 3})
	if err == nil {
		t.Fatal("expected blocking error, got nil")
	}
	if deleteByIDsCalled {
		t.Error("DeleteByIDs should not be called when any workspace has blocking resources")
	}
}
