package iam

import (
	"context"
	"strings"
	"testing"

	apierrors "vraxel.io/vraxel/lib/api/errors"
	"vraxel.io/vraxel/lib/apiserver"
	"vraxel.io/vraxel/lib/list"
	modstore "vraxel.io/vraxel/pkg/apis/iam/store"
)

// --- TestNamespaceOps_Get ---

func TestNamespaceOps_Get(t *testing.T) {
	nsWithOwner := testNamespaceWithOwner(1, "my-namespace", 10, 100, "alice", "my-workspace")
	nsWithOwner.MemberCount = 5

	nsStore := &mockNamespaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.NamespaceWithOwnerRow, error) {
			if id != 1 {
				t.Fatalf("expected id 1, got %d", id)
			}
			return nsWithOwner, nil
		},
	}

	ops := namespaceOps{nsStore: nsStore}

	ns, err := ops.get(testCtx(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ns.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", ns.ObjectMeta.ID)
	}
	if ns.ObjectMeta.Name != "my-namespace" {
		t.Errorf("expected Name 'my-namespace', got %q", ns.ObjectMeta.Name)
	}
	if ns.TypeMeta.Kind != "Namespace" {
		t.Errorf("expected Kind 'Namespace', got %q", ns.TypeMeta.Kind)
	}
	if ns.Spec.OwnerID != "100" {
		t.Errorf("expected OwnerID '100', got %q", ns.Spec.OwnerID)
	}
	if ns.Spec.OwnerName != "alice" {
		t.Errorf("expected OwnerName 'alice', got %q", ns.Spec.OwnerName)
	}
	if ns.Spec.WorkspaceName != "my-workspace" {
		t.Errorf("expected WorkspaceName 'my-workspace', got %q", ns.Spec.WorkspaceName)
	}
	if ns.Spec.MemberCount != 5 {
		t.Errorf("expected MemberCount 5, got %d", ns.Spec.MemberCount)
	}
	if ns.Spec.WorkspaceID != "10" {
		t.Errorf("expected WorkspaceID '10', got %q", ns.Spec.WorkspaceID)
	}
	if ns.Spec.Status != "active" {
		t.Errorf("expected Status 'active', got %q", ns.Spec.Status)
	}
}

// --- TestNamespaceOps_List ---

func TestNamespaceOps_List(t *testing.T) {
	nsStore := &mockNamespaceStore{
		ListFn: func(ctx context.Context, query list.Query) (*list.Result[modstore.NamespaceWithOwnerRow], error) {
			return &list.Result[modstore.NamespaceWithOwnerRow]{
				Items: []modstore.NamespaceWithOwnerRow{
					{
						NamespaceRow: modstore.NamespaceRow{
							ID:          1,
							Name:        "ns-one",
							DisplayName: "Namespace One",
							WorkspaceID: 10,
							OwnerID:     100,
							Visibility:  "private",
							Status:      "active",
							CreatedAt:   testTime,
							UpdatedAt:   testTime,
						},
						OwnerUsername: "alice",
						WorkspaceName: "ws-one",
						MemberCount:   3,
					},
					{
						NamespaceRow: modstore.NamespaceRow{
							ID:          2,
							Name:        "ns-two",
							DisplayName: "Namespace Two",
							WorkspaceID: 20,
							OwnerID:     200,
							Visibility:  "public",
							Status:      "active",
							CreatedAt:   testTime,
							UpdatedAt:   testTime,
						},
						OwnerUsername: "bob",
						WorkspaceName: "ws-two",
						MemberCount:   1,
					},
				},
				TotalCount: 2,
			}, nil
		},
	}

	ops := namespaceOps{nsStore: nsStore}

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

	// Verify first namespace
	if result.Items[0].ObjectMeta.ID != "1" {
		t.Errorf("expected first item ID '1', got %q", result.Items[0].ObjectMeta.ID)
	}
	if result.Items[0].ObjectMeta.Name != "ns-one" {
		t.Errorf("expected first item Name 'ns-one', got %q", result.Items[0].ObjectMeta.Name)
	}
	if result.Items[0].Spec.OwnerName != "alice" {
		t.Errorf("expected first item OwnerName 'alice', got %q", result.Items[0].Spec.OwnerName)
	}
	if result.Items[0].Spec.WorkspaceName != "ws-one" {
		t.Errorf("expected first item WorkspaceName 'ws-one', got %q", result.Items[0].Spec.WorkspaceName)
	}
	if result.Items[0].Spec.MemberCount != 3 {
		t.Errorf("expected first item MemberCount 3, got %d", result.Items[0].Spec.MemberCount)
	}

	// Verify second namespace
	if result.Items[1].ObjectMeta.ID != "2" {
		t.Errorf("expected second item ID '2', got %q", result.Items[1].ObjectMeta.ID)
	}
	if result.Items[1].Spec.OwnerName != "bob" {
		t.Errorf("expected second item OwnerName 'bob', got %q", result.Items[1].Spec.OwnerName)
	}
	if result.Items[1].Spec.WorkspaceName != "ws-two" {
		t.Errorf("expected second item WorkspaceName 'ws-two', got %q", result.Items[1].Spec.WorkspaceName)
	}
}

// --- TestNamespaceOps_List_FilterByWorkspace ---

func TestNamespaceOps_List_FilterByWorkspace(t *testing.T) {
	var capturedQuery list.Query

	nsStore := &mockNamespaceStore{
		ListFn: func(ctx context.Context, query list.Query) (*list.Result[modstore.NamespaceWithOwnerRow], error) {
			capturedQuery = query
			return &list.Result[modstore.NamespaceWithOwnerRow]{
				Items:      []modstore.NamespaceWithOwnerRow{},
				TotalCount: 0,
			}, nil
		},
	}

	ops := namespaceOps{nsStore: nsStore}

	wsCtx := testCtx()
	wsCtx.Scope = apiserver.ScopeInfo{Level: apiserver.ScopeWorkspace, WorkspaceID: 42}

	_, err := ops.list(wsCtx, list.Query{
		Filters: map[string]any{},
		Pagination: list.Pagination{
			Page:     1,
			PageSize: 20,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wsFilter, ok := capturedQuery.Filters["workspace_id"]
	if !ok {
		t.Fatal("expected workspace_id filter to be added when called at workspace scope")
	}
	if wsFilter != "42" {
		t.Errorf("expected workspace_id filter '42', got %q", wsFilter)
	}
}

// --- TestNamespaceOps_Create ---

func TestNamespaceOps_Create(t *testing.T) {
	var nsCreateCalled, wsGetByIDCalled, userGetByIDCalled bool

	nsStore := &mockNamespaceStore{
		CreateFn: func(ctx context.Context, ns modstore.NamespaceCreateInput) (*modstore.NamespaceWithOwnerRow, error) {
			nsCreateCalled = true
			if ns.Name != "new-namespace" {
				t.Errorf("expected name 'new-namespace', got %q", ns.Name)
			}
			if ns.DisplayName != "New Namespace" {
				t.Errorf("expected displayName 'New Namespace', got %q", ns.DisplayName)
			}
			if ns.WorkspaceID != 10 {
				t.Errorf("expected workspaceID 10, got %d", ns.WorkspaceID)
			}
			if ns.OwnerID != 100 {
				t.Errorf("expected ownerID 100, got %d", ns.OwnerID)
			}
			if ns.Visibility != "private" {
				t.Errorf("expected visibility 'private', got %q", ns.Visibility)
			}
			if ns.Status != "active" {
				t.Errorf("expected status 'active', got %q", ns.Status)
			}
			return testNamespaceWithOwner(1, "new-namespace", 10, 100, "alice", "my-workspace"), nil
		},
	}

	wsStore := &mockWorkspaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.WorkspaceWithOwnerRow, error) {
			wsGetByIDCalled = true
			if id != 10 {
				t.Errorf("expected workspace id 10, got %d", id)
			}
			return testWorkspaceWithOwner(10, "my-workspace", 100, "alice"), nil
		},
	}

	userStore := &mockUserStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.UserRow, error) {
			userGetByIDCalled = true
			if id != 100 {
				t.Errorf("expected owner id 100, got %d", id)
			}
			return testUser(100, "alice", "alice@example.com"), nil
		},
	}

	ops := namespaceOps{nsStore: nsStore, wsStore: wsStore, userStore: userStore}

	inputNs := &Namespace{
		Spec: NamespaceSpec{
			DisplayName: "New Namespace",
			Description: "A test namespace",
			WorkspaceID: "10",
			OwnerID:     "100",
			Visibility:  "private",
			Status:      "active",
		},
	}
	inputNs.ObjectMeta.Name = "new-namespace"

	result, err := ops.create(testCtx(), inputNs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !wsGetByIDCalled {
		t.Error("expected wsStore.GetByID to be called to verify workspace exists")
	}
	if !userGetByIDCalled {
		t.Error("expected userStore.GetByID to be called to verify owner exists")
	}
	if !nsCreateCalled {
		t.Error("expected nsStore.Create to be called")
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
	if result.ObjectMeta.Name != "new-namespace" {
		t.Errorf("expected Name 'new-namespace', got %q", result.ObjectMeta.Name)
	}
	if result.TypeMeta.Kind != "Namespace" {
		t.Errorf("expected Kind 'Namespace', got %q", result.TypeMeta.Kind)
	}
	if result.Spec.OwnerName != "alice" {
		t.Errorf("expected OwnerName 'alice', got %q", result.Spec.OwnerName)
	}
	if result.Spec.WorkspaceName != "my-workspace" {
		t.Errorf("expected WorkspaceName 'my-workspace', got %q", result.Spec.WorkspaceName)
	}
	if result.Spec.WorkspaceID != "10" {
		t.Errorf("expected WorkspaceID '10', got %q", result.Spec.WorkspaceID)
	}
}

// --- TestNamespaceOps_Create_WorkspaceNotFound ---

func TestNamespaceOps_Create_WorkspaceNotFound(t *testing.T) {
	wsStore := &mockWorkspaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.WorkspaceWithOwnerRow, error) {
			return nil, apierrors.NewNotFound("workspace", "999")
		},
	}

	userStore := &mockUserStore{}
	nsStore := &mockNamespaceStore{}

	ops := namespaceOps{nsStore: nsStore, wsStore: wsStore, userStore: userStore}

	inputNs := &Namespace{
		Spec: NamespaceSpec{
			DisplayName: "Test Namespace",
			WorkspaceID: "999",
			OwnerID:     "100",
			Status:      "active",
		},
	}
	inputNs.ObjectMeta.Name = "test-namespace"

	_, err := ops.create(testCtx(), inputNs)
	if err == nil {
		t.Fatal("expected error when workspace not found, got nil")
	}

	statusErr, ok := err.(*apierrors.StatusError)
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	if statusErr.Status != 400 {
		t.Errorf("expected status 400, got %d", statusErr.Status)
	}
}

// --- TestNamespaceOps_Update ---

func TestNamespaceOps_Update(t *testing.T) {
	existingNsWithOwner := testNamespaceWithOwner(1, "my-namespace", 10, 100, "alice", "my-workspace")

	updatedDBNs := testNamespaceWithOwner(1, "updated-namespace", 10, 100, "alice", "my-workspace")
	updatedDBNs.DisplayName = "Updated Namespace"
	updatedDBNs.Description = "Updated description"

	var getByIDCalled, updateCalled bool

	nsStore := &mockNamespaceStore{
		GetByIDFn: func(ctx context.Context, id int64) (*modstore.NamespaceWithOwnerRow, error) {
			getByIDCalled = true
			return existingNsWithOwner, nil
		},
		UpdateFn: func(ctx context.Context, ns modstore.NamespaceUpdateInput) (*modstore.NamespaceWithOwnerRow, error) {
			updateCalled = true
			if ns.ID != 1 {
				t.Errorf("expected namespace ID 1, got %d", ns.ID)
			}
			if ns.DisplayName != "Updated Namespace" {
				t.Errorf("expected displayName 'Updated Namespace', got %q", ns.DisplayName)
			}
			if ns.OwnerID != 100 {
				t.Errorf("expected ownerID 100, got %d", ns.OwnerID)
			}
			return updatedDBNs, nil
		},
	}

	ops := namespaceOps{nsStore: nsStore}

	inputNs := &Namespace{
		Spec: NamespaceSpec{
			DisplayName: "Updated Namespace",
			Description: "Updated description",
			OwnerID:     "100",
			Visibility:  "private",
			Status:      "active",
		},
	}
	inputNs.ObjectMeta.Name = "updated-namespace"

	result, err := ops.update(testCtx(), 1, inputNs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if getByIDCalled {
		t.Error("nsStore.GetByID must not be called: workspace_id is immutable on update and SQL no longer touches the column")
	}
	if !updateCalled {
		t.Error("expected nsStore.Update to be called")
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
	if result.Spec.DisplayName != "Updated Namespace" {
		t.Errorf("expected DisplayName 'Updated Namespace', got %q", result.Spec.DisplayName)
	}
	if result.Spec.Description != "Updated description" {
		t.Errorf("expected Description 'Updated description', got %q", result.Spec.Description)
	}
	if result.TypeMeta.Kind != "Namespace" {
		t.Errorf("expected Kind 'Namespace', got %q", result.TypeMeta.Kind)
	}
}

// --- TestNamespaceOps_Patch ---

func TestNamespaceOps_Patch(t *testing.T) {
	patchedDBNs := testNamespaceWithOwner(1, "my-namespace", 10, 100, "alice", "my-workspace")
	patchedDBNs.DisplayName = "Patched Namespace"
	patchedDBNs.Description = "Original description"

	var patchCalled bool

	nsStore := &mockNamespaceStore{
		PatchFn: func(ctx context.Context, ns modstore.NamespaceUpdateInput) (*modstore.NamespaceWithOwnerRow, error) {
			patchCalled = true
			if ns.ID != 1 {
				t.Errorf("expected id 1, got %d", ns.ID)
			}
			if ns.DisplayName != "Patched Namespace" {
				t.Errorf("expected displayName 'Patched Namespace', got %q", ns.DisplayName)
			}
			// Description should be empty (zero value) since patch input did not set it
			if ns.Description != "" {
				t.Errorf("expected description '' (zero value), got %q", ns.Description)
			}
			return patchedDBNs, nil
		},
	}

	ops := namespaceOps{nsStore: nsStore}

	// Patch only the displayName
	result, err := ops.patch(testCtx(), 1, []byte(`{"spec":{"displayName":"Patched Namespace"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !patchCalled {
		t.Error("expected nsStore.Patch to be called")
	}

	if result.ObjectMeta.ID != "1" {
		t.Errorf("expected ID '1', got %q", result.ObjectMeta.ID)
	}
	if result.Spec.DisplayName != "Patched Namespace" {
		t.Errorf("expected DisplayName 'Patched Namespace', got %q", result.Spec.DisplayName)
	}
}

// --- TestNamespaceOps_Delete ---

func TestNamespaceOps_Delete(t *testing.T) {
	deleteCalled := false

	nsStore := &mockNamespaceStore{
		DeleteFn: func(ctx context.Context, id int64) error {
			deleteCalled = true
			if id != 1 {
				t.Errorf("expected id 1, got %d", id)
			}
			return nil
		},
	}

	ops := namespaceOps{nsStore: nsStore}

	if err := ops.delete(testCtx(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deleteCalled {
		t.Error("expected Delete to be called on the store")
	}
}

// --- TestNamespaceOps_BatchDelete ---

func TestNamespaceOps_BatchDelete(t *testing.T) {
	nsStore := &mockNamespaceStore{
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

	ops := namespaceOps{nsStore: nsStore}

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

// --- TestNamespaceOps_Delete_Blocked ---

func TestNamespaceOps_Delete_Blocked(t *testing.T) {
	deleteCalled := false
	nsStore := &mockNamespaceStore{
		ListBlockingResourcesFn: func(ctx context.Context, namespaceID int64) ([]modstore.BlockingResourceRow, error) {
			return []modstore.BlockingResourceRow{
				{Kind: "host", Count: 3},
				{Kind: "mysql_instance", Count: 2},
			}, nil
		},
		DeleteFn: func(ctx context.Context, id int64) error {
			deleteCalled = true
			return nil
		},
	}

	ops := namespaceOps{nsStore: nsStore}
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
	wantPrefix := "cannot delete namespace:"
	if !strings.HasPrefix(se.Message, wantPrefix) {
		t.Errorf("message should start with %q, got %q", wantPrefix, se.Message)
	}
	if !strings.Contains(se.Message, "host(3)") || !strings.Contains(se.Message, "mysql_instance(2)") {
		t.Errorf("message missing kind(count) tokens: %q", se.Message)
	}
	if deleteCalled {
		t.Error("Delete should not be called when blocking resources exist")
	}
}

// --- TestNamespaceOps_BatchDelete_Blocked ---

func TestNamespaceOps_BatchDelete_Blocked(t *testing.T) {
	deleteByIDsCalled := false
	nsStore := &mockNamespaceStore{
		ListBlockingResourcesFn: func(ctx context.Context, namespaceID int64) ([]modstore.BlockingResourceRow, error) {
			if namespaceID == 2 {
				return []modstore.BlockingResourceRow{{Kind: "host", Count: 1}}, nil
			}
			return nil, nil
		},
		DeleteByIDsFn: func(ctx context.Context, ids []int64) (int64, error) {
			deleteByIDsCalled = true
			return int64(len(ids)), nil
		},
	}

	ops := namespaceOps{nsStore: nsStore}
	_, err := ops.batchDelete(testCtx(), []int64{1, 2, 3})
	if err == nil {
		t.Fatal("expected blocking error, got nil")
	}
	if deleteByIDsCalled {
		t.Error("DeleteByIDs should not be called when any namespace has blocking resources")
	}
}
