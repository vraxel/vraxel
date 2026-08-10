package iam

import modstore "vraxel.io/vraxel/pkg/apis/iam/store"
import (
	"context"
	"fmt"
	"testing"

	"vraxel.io/vraxel/lib/list"
)

// --- mock modstore.RoleStore for SeedRBAC ---

type mockRoleStoreForSeed struct {
	roles  map[string]*modstore.RoleRow
	rules  map[int64][]string
	nextID int64
}

func newMockRoleStoreForSeed() *mockRoleStoreForSeed {
	return &mockRoleStoreForSeed{
		roles:  make(map[string]*modstore.RoleRow),
		rules:  make(map[int64][]string),
		nextID: 1,
	}
}

func (m *mockRoleStoreForSeed) Create(_ context.Context, in modstore.RoleCreateInput) (*modstore.RoleRow, error) {
	m.nextID++
	role := &modstore.RoleRow{
		ID:          m.nextID,
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
		Scope:       in.Scope,
		WorkspaceID: in.WorkspaceID,
		NamespaceID: in.NamespaceID,
		Builtin:     in.Builtin,
	}
	m.roles[role.Name] = role
	return role, nil
}

func (m *mockRoleStoreForSeed) GetByID(_ context.Context, id int64) (*modstore.RoleWithRulesRow, error) {
	for _, r := range m.roles {
		if r.ID == id {
			return &modstore.RoleWithRulesRow{RoleRow: *r, Rules: m.rules[id]}, nil
		}
	}
	return nil, fmt.Errorf("role %d not found", id)
}

func (m *mockRoleStoreForSeed) GetByName(_ context.Context, name string) (*modstore.RoleRow, error) {
	if r, ok := m.roles[name]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("role %q not found", name)
}

func (m *mockRoleStoreForSeed) Update(_ context.Context, in modstore.RoleUpdateInput) (*modstore.RoleRow, error) {
	for _, r := range m.roles {
		if r.ID == in.ID {
			r.DisplayName = in.DisplayName
			r.Description = in.Description
			return r, nil
		}
	}
	return nil, fmt.Errorf("role %d not found", in.ID)
}

func (m *mockRoleStoreForSeed) Upsert(_ context.Context, in modstore.RoleCreateInput) (*modstore.RoleRow, error) {
	if existing, ok := m.roles[in.Name]; ok {
		existing.DisplayName = in.DisplayName
		existing.Description = in.Description
		existing.Scope = in.Scope
		existing.Builtin = in.Builtin
		return existing, nil
	}
	m.nextID++
	role := &modstore.RoleRow{
		ID:          m.nextID,
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
		Scope:       in.Scope,
		WorkspaceID: in.WorkspaceID,
		NamespaceID: in.NamespaceID,
		Builtin:     in.Builtin,
	}
	m.roles[role.Name] = role
	return role, nil
}

func (m *mockRoleStoreForSeed) Delete(_ context.Context, _ int64) error { return nil }

func (m *mockRoleStoreForSeed) List(_ context.Context, _ list.Query) (*list.Result[modstore.RoleListItem], error) {
	return nil, nil
}

func (m *mockRoleStoreForSeed) SetPermissionRules(_ context.Context, roleID int64, patterns []string) error {
	m.rules[roleID] = patterns
	return nil
}

func (m *mockRoleStoreForSeed) GetByNameAndWorkspace(_ context.Context, name string, _ int64) (*modstore.RoleRow, error) {
	if r, ok := m.roles[name]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("role %q not found", name)
}

func (m *mockRoleStoreForSeed) GetByNameAndNamespace(_ context.Context, name string, _ int64) (*modstore.RoleRow, error) {
	if r, ok := m.roles[name]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("role %q not found", name)
}

func (m *mockRoleStoreForSeed) SeedRBAC(_ context.Context, roles []modstore.BuiltinRoleDef, _ string) error {
	for _, def := range roles {
		role, _ := m.Upsert(context.Background(), modstore.RoleCreateInput{
			Name:        def.Name,
			DisplayName: def.DisplayName,
			Description: def.Description,
			Scope:       def.Scope,
			Builtin:     true,
		})
		m.rules[role.ID] = def.Rules
	}
	return nil
}

// --- tests ---

func TestSeedRBAC(t *testing.T) {
	store := newMockRoleStoreForSeed()

	if err := SeedRBAC(context.Background(), store); err != nil {
		t.Fatalf("SeedRBAC: %v", err)
	}

	// SeedRBAC now only seeds platform roles (2 roles)
	if len(store.roles) != 2 {
		t.Fatalf("expected 2 platform roles, got %d", len(store.roles))
	}

	// All roles should be builtin
	for name, role := range store.roles {
		if !role.Builtin {
			t.Errorf("role %q should be builtin", name)
		}
	}

	// Check platform admin role has "*:*" rule
	adminRole := store.roles[modstore.RolePlatformAdmin]
	if adminRole == nil {
		t.Fatalf("%s role not found", modstore.RolePlatformAdmin)
	}
	adminRules := store.rules[adminRole.ID]
	if len(adminRules) != 1 || adminRules[0] != "*:*" {
		t.Errorf("%s rules = %v, want [*:*]", modstore.RolePlatformAdmin, adminRules)
	}

	// Check platform viewer role has "*:list" and "*:get" rules
	viewerRole := store.roles[modstore.RolePlatformViewer]
	if viewerRole == nil {
		t.Fatalf("%s role not found", modstore.RolePlatformViewer)
	}
	viewerRules := store.rules[viewerRole.ID]
	if len(viewerRules) != 2 {
		t.Errorf("%s rules = %v, want [*:list *:get]", modstore.RolePlatformViewer, viewerRules)
	} else {
		ruleSet := map[string]bool{viewerRules[0]: true, viewerRules[1]: true}
		if !ruleSet["*:list"] || !ruleSet["*:get"] {
			t.Errorf("%s rules = %v, want [*:list *:get]", modstore.RolePlatformViewer, viewerRules)
		}
	}

	// Check scopes
	expectedScopes := map[string]string{
		modstore.RolePlatformAdmin:  modstore.ScopePlatform,
		modstore.RolePlatformViewer: modstore.ScopePlatform,
	}
	for name, scope := range expectedScopes {
		role := store.roles[name]
		if role == nil {
			t.Errorf("role %q not found", name)
			continue
		}
		if role.Scope != scope {
			t.Errorf("role %q scope = %q, want %q", name, role.Scope, scope)
		}
	}
}

func TestSeedRBACIdempotent(t *testing.T) {
	store := newMockRoleStoreForSeed()

	// Seed twice
	if err := SeedRBAC(context.Background(), store); err != nil {
		t.Fatalf("first SeedRBAC: %v", err)
	}
	if err := SeedRBAC(context.Background(), store); err != nil {
		t.Fatalf("second SeedRBAC: %v", err)
	}

	// Should still have exactly 2 platform roles (no duplicates)
	if len(store.roles) != 2 {
		t.Fatalf("expected 2 roles after double seed, got %d", len(store.roles))
	}

	// Rules should be correct (overwritten, not accumulated)
	adminRole := store.roles[modstore.RolePlatformAdmin]
	adminRules := store.rules[adminRole.ID]
	if len(adminRules) != 1 || adminRules[0] != "*:*" {
		t.Errorf("%s rules after double seed = %v, want [*:*]", modstore.RolePlatformAdmin, adminRules)
	}
}

func TestBuiltinRoleHelpers(t *testing.T) {
	platform := modstore.PlatformBuiltinRoles()
	if len(platform) != 2 {
		t.Errorf("modstore.PlatformBuiltinRoles() returned %d roles, want 2", len(platform))
	}
	for _, r := range platform {
		if r.Scope != modstore.ScopePlatform {
			t.Errorf("modstore.PlatformBuiltinRoles() role %q has scope %q, want platform", r.Name, r.Scope)
		}
	}

	workspace := modstore.WorkspaceBuiltinRoles()
	if len(workspace) != 3 {
		t.Errorf("modstore.WorkspaceBuiltinRoles() returned %d roles, want 3", len(workspace))
	}
	for _, r := range workspace {
		if r.Scope != modstore.ScopeWorkspace {
			t.Errorf("modstore.WorkspaceBuiltinRoles() role %q has scope %q, want workspace", r.Name, r.Scope)
		}
	}

	namespace := modstore.NamespaceBuiltinRoles()
	if len(namespace) != 2 {
		t.Errorf("modstore.NamespaceBuiltinRoles() returned %d roles, want 2", len(namespace))
	}
	for _, r := range namespace {
		if r.Scope != modstore.ScopeNamespace {
			t.Errorf("modstore.NamespaceBuiltinRoles() role %q has scope %q, want namespace", r.Name, r.Scope)
		}
	}
}
