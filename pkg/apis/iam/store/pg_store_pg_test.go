package store_test

import (
	"context"
	"errors"
	"testing"

	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/apis/iam/store"
	"vraxel.io/vraxel/pkg/db/dbtest"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

func newUser(t *testing.T, s store.Stores, name string) *store.UserRow {
	t.Helper()
	u, err := s.User.Create(context.Background(), store.UserCreateInput{
		Username: name, Email: name + "@vraxel.io", Phone: "138" + name[:min(8, len(name))], Status: "active",
	})
	if err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	return u
}

func TestUserRoundtripAndConflict(t *testing.T) {
	s := store.NewStores(dbtest.New(t))
	ctx := context.Background()

	u := newUser(t, s, "alice-it")
	got, err := s.User.GetByID(ctx, u.ID)
	if err != nil || got.Username != "alice-it" {
		t.Fatalf("GetByID: %+v, %v", got, err)
	}

	// Real partial-unique semantics, not a mock: duplicate username must
	// surface as the domain conflict error the REST layer maps to 409.
	_, err = s.User.Create(ctx, store.UserCreateInput{
		Username: "alice-it", Email: "other@vraxel.io", Phone: "13800000099", Status: "active",
	})
	if !errors.Is(err, pgerrors.ErrConflict) {
		t.Fatalf("duplicate username: got %v, want ErrConflict", err)
	}

	res, err := s.User.List(ctx, list.Query{Pagination: list.Pagination{Page: 1, PageSize: 10}, Filters: map[string]any{"search": "alice"}})
	if err != nil || res.TotalCount != 1 {
		t.Fatalf("List search: %+v, %v", res, err)
	}
}

func TestRoleBindingPartialUniqueIndex(t *testing.T) {
	s := store.NewStores(dbtest.New(t))
	ctx := context.Background()

	u := newUser(t, s, "bob-it")
	role, err := s.Role.Create(ctx, store.RoleCreateInput{Name: "it-role", Scope: "platform"})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	in := store.RoleBindingCreateInput{UserID: u.ID, RoleID: role.ID, Scope: "platform"}
	if _, err := s.RoleBinding.Create(ctx, in); err != nil {
		t.Fatalf("first binding: %v", err)
	}
	// uk_role_bindings_platform is a partial index (WHERE scope='platform');
	// only a real database proves it fires.
	if _, err := s.RoleBinding.Create(ctx, in); !errors.Is(err, pgerrors.ErrConflict) {
		t.Fatalf("duplicate binding: got %v, want ErrConflict", err)
	}
}

func TestWorkspaceDeletePrecheckIsEmpty(t *testing.T) {
	s := store.NewStores(dbtest.New(t))
	ctx := context.Background()

	u := newUser(t, s, "carol-it")
	ws, err := s.Workspace.Create(ctx, store.WorkspaceCreateInput{Name: "ws-it", OwnerID: u.ID, Status: "active"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	// No business module registers blocking resources yet; the neutral
	// query must compile against the live schema and return zero rows.
	rows, err := s.Workspace.ListBlockingResources(ctx, ws.ID)
	if err != nil {
		t.Fatalf("ListBlockingResources: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no blocking resources, got %+v", rows)
	}
}
