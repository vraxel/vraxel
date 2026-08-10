package iam

import modstore "vraxel.io/vraxel/pkg/apis/iam/store"
import (
	"context"
	"sync/atomic"
	"testing"
)

func newTestChecker(rules []modstore.UserPermissionRuleRow) (*RBACChecker, *atomic.Int32) {
	var loadCount atomic.Int32
	store := &mockRoleBindingStore{
		LoadUserPermissionRulesFn: func(_ context.Context, _ int64) ([]modstore.UserPermissionRuleRow, error) {
			loadCount.Add(1)
			return rules, nil
		},
	}
	return NewRBACChecker(store), &loadCount
}

func ptr[T any](v T) *T { return &v }

func TestRBACChecker_PlatformAdmin(t *testing.T) {
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopePlatform, Pattern: "*:*"},
	})
	ctx := context.Background()

	isAdmin, err := checker.IsPlatformAdmin(ctx, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isAdmin {
		t.Error("expected platform admin")
	}

	// Platform admin should match any permission at any scope
	tests := []struct {
		code  string
		scope string
		wsID  int64
		nsID  int64
	}{
		{"iam:users:list", modstore.ScopePlatform, 0, 0},
		{"iam:workspaces:get", modstore.ScopeWorkspace, 10, 0},
		{"iam:namespaces:delete", modstore.ScopeNamespace, 10, 100},
		{"infra:hosts:create", modstore.ScopePlatform, 0, 0},
	}
	for _, tt := range tests {
		ok, err := checker.CheckPermission(ctx, 1, tt.code, tt.scope, tt.wsID, tt.nsID)
		if err != nil {
			t.Fatalf("CheckPermission(%s): %v", tt.code, err)
		}
		if !ok {
			t.Errorf("platform admin should have %s at %s scope", tt.code, tt.scope)
		}
	}
}

func TestRBACChecker_PlatformMember(t *testing.T) {
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopePlatform, Pattern: "iam:workspaces:list"},
		{Scope: modstore.ScopePlatform, Pattern: "iam:namespaces:list"},
		{Scope: modstore.ScopePlatform, Pattern: "iam:users:change-password"},
	})
	ctx := context.Background()

	isAdmin, _ := checker.IsPlatformAdmin(ctx, 1)
	if isAdmin {
		t.Error("should not be platform admin")
	}

	ok, _ := checker.CheckPermission(ctx, 1, "iam:workspaces:list", modstore.ScopePlatform, 0, 0)
	if !ok {
		t.Error("should have iam:workspaces:list")
	}

	ok, _ = checker.CheckPermission(ctx, 1, "iam:users:list", modstore.ScopePlatform, 0, 0)
	if ok {
		t.Error("should not have iam:users:list")
	}
}

func TestRBACChecker_WorkspaceScope(t *testing.T) {
	var wsID int64 = 10
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopeWorkspace, WorkspaceID: &wsID, Pattern: "iam:namespaces:*"},
		{Scope: modstore.ScopeWorkspace, WorkspaceID: &wsID, Pattern: "iam:workspaces:get"},
	})
	ctx := context.Background()

	// Has permission within workspace 10
	ok, _ := checker.CheckPermission(ctx, 1, "iam:namespaces:list", modstore.ScopeWorkspace, 10, 0)
	if !ok {
		t.Error("should match wildcard iam:namespaces:* in ws 10")
	}

	// Workspace rule does NOT apply to a different workspace
	ok, _ = checker.CheckPermission(ctx, 1, "iam:namespaces:list", modstore.ScopeWorkspace, 20, 0)
	if ok {
		t.Error("should not match in ws 20")
	}

	// Workspace rules inherit to namespace scope (scope chain)
	ok, _ = checker.CheckPermission(ctx, 1, "iam:namespaces:get", modstore.ScopeNamespace, 10, 100)
	if !ok {
		t.Error("workspace rule should inherit to namespace scope")
	}
}

func TestRBACChecker_NamespaceScope(t *testing.T) {
	var nsID int64 = 100
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopeNamespace, NamespaceID: &nsID, Pattern: "iam:namespaces:get"},
		{Scope: modstore.ScopeNamespace, NamespaceID: &nsID, Pattern: "iam:namespaces:users:list"},
	})
	ctx := context.Background()

	ok, _ := checker.CheckPermission(ctx, 1, "iam:namespaces:get", modstore.ScopeNamespace, 10, 100)
	if !ok {
		t.Error("should match namespace rule for ns 100")
	}

	ok, _ = checker.CheckPermission(ctx, 1, "iam:namespaces:get", modstore.ScopeNamespace, 10, 200)
	if ok {
		t.Error("should not match for ns 200")
	}

	// Namespace rules don't apply to workspace scope
	ok, _ = checker.CheckPermission(ctx, 1, "iam:namespaces:get", modstore.ScopeWorkspace, 10, 0)
	if ok {
		t.Error("namespace rules should not apply to workspace scope")
	}
}

func TestRBACChecker_NoBindings(t *testing.T) {
	checker, _ := newTestChecker(nil)
	ctx := context.Background()

	ok, _ := checker.CheckPermission(ctx, 1, "iam:users:list", modstore.ScopePlatform, 0, 0)
	if ok {
		t.Error("user without bindings should have no permissions")
	}

	isAdmin, _ := checker.IsPlatformAdmin(ctx, 1)
	if isAdmin {
		t.Error("user without bindings should not be admin")
	}
}

func TestHasAnyPermission_WildcardTarget(t *testing.T) {
	// User has a specific infra:hosts:create permission at workspace level
	var wsID int64 = 10
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopeWorkspace, WorkspaceID: &wsID, Pattern: "infra:hosts:create"},
	})
	ctx := context.Background()

	// PermissionTargets: ["infra:hosts:*"] — wildcard target should match specific user rule
	ok, err := checker.CheckAnyPermission(ctx, 1, []string{"infra:hosts:*"}, modstore.ScopeWorkspace, 10, 0)
	if err != nil {
		t.Fatalf("CheckAnyPermission: %v", err)
	}
	if !ok {
		t.Error("infra:hosts:* should match user with infra:hosts:create")
	}

	// Should NOT match at a different workspace
	ok, _ = checker.CheckAnyPermission(ctx, 1, []string{"infra:hosts:*"}, modstore.ScopeWorkspace, 20, 0)
	if ok {
		t.Error("should not match at ws 20")
	}
}

func TestHasAnyPermission_BroadUserRule(t *testing.T) {
	// User has a broad infra:* rule at platform level
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopePlatform, Pattern: "infra:*"},
	})
	ctx := context.Background()

	// infra:* should cover infra:hosts:*
	ok, err := checker.CheckAnyPermission(ctx, 1, []string{"infra:hosts:*"}, modstore.ScopePlatform, 0, 0)
	if err != nil {
		t.Fatalf("CheckAnyPermission: %v", err)
	}
	if !ok {
		t.Error("user with infra:* should match target infra:hosts:*")
	}
}

func TestHasAnyPermission_MultipleTargets(t *testing.T) {
	var wsID int64 = 10
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopeWorkspace, WorkspaceID: &wsID, Pattern: "infra:hosts:ips:create"},
	})
	ctx := context.Background()

	// User has infra:hosts:ips:create, target list includes this
	ok, _ := checker.CheckAnyPermission(ctx, 1,
		[]string{"infra:hosts:create", "infra:hosts:ips:create"},
		modstore.ScopeWorkspace, 10, 0)
	if !ok {
		t.Error("should match second target")
	}

	// Neither target matches
	ok, _ = checker.CheckAnyPermission(ctx, 1,
		[]string{"infra:hosts:create", "iam:users:list"},
		modstore.ScopeWorkspace, 10, 0)
	if ok {
		t.Error("neither target should match")
	}
}

func TestHasAnyPermission_NoMatch(t *testing.T) {
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopePlatform, Pattern: "iam:users:list"},
	})
	ctx := context.Background()

	ok, _ := checker.CheckAnyPermission(ctx, 1, []string{"infra:hosts:*"}, modstore.ScopePlatform, 0, 0)
	if ok {
		t.Error("iam:users:list should not match infra:hosts:*")
	}
}

func TestHasAnyPermission_PlatformAdmin(t *testing.T) {
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopePlatform, Pattern: "*:*"},
	})
	ctx := context.Background()

	ok, _ := checker.CheckAnyPermission(ctx, 1, []string{"infra:hosts:*"}, modstore.ScopePlatform, 0, 0)
	if !ok {
		t.Error("platform admin (*:*) should match any target")
	}
}

func TestHasAnyPermission_NamespaceScopeInheritance(t *testing.T) {
	var wsID int64 = 10
	var nsID int64 = 100
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopeWorkspace, WorkspaceID: &wsID, Pattern: "infra:hosts:create"},
	})
	ctx := context.Background()

	// Workspace rule should inherit to namespace scope
	ok, _ := checker.CheckAnyPermission(ctx, 1, []string{"infra:hosts:*"}, modstore.ScopeNamespace, 10, nsID)
	if !ok {
		t.Error("workspace rule should inherit to namespace scope for wildcard target")
	}
}

func TestRBACChecker_ScopeChainInheritance(t *testing.T) {
	var wsID int64 = 10
	var nsID int64 = 100
	checker, _ := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopePlatform, Pattern: "iam:workspaces:list"},
		{Scope: modstore.ScopeWorkspace, WorkspaceID: &wsID, Pattern: "iam:namespaces:list"},
		{Scope: modstore.ScopeNamespace, NamespaceID: &nsID, Pattern: "iam:namespaces:users:list"},
	})
	ctx := context.Background()

	// Platform rule available at namespace level
	ok, _ := checker.CheckPermission(ctx, 1, "iam:workspaces:list", modstore.ScopeNamespace, 10, 100)
	if !ok {
		t.Error("platform rule should be available at namespace level")
	}

	// Workspace rule available at namespace level
	ok, _ = checker.CheckPermission(ctx, 1, "iam:namespaces:list", modstore.ScopeNamespace, 10, 100)
	if !ok {
		t.Error("workspace rule should be available at namespace level")
	}

	// Namespace rule only at namespace level
	ok, _ = checker.CheckPermission(ctx, 1, "iam:namespaces:users:list", modstore.ScopeNamespace, 10, 100)
	if !ok {
		t.Error("namespace rule should match at namespace level")
	}

	// Namespace rule NOT at platform level
	ok, _ = checker.CheckPermission(ctx, 1, "iam:namespaces:users:list", modstore.ScopePlatform, 0, 0)
	if ok {
		t.Error("namespace rule should not apply at platform level")
	}
}

func TestRBACChecker_InvalidateUser(t *testing.T) {
	checker, loadCount := newTestChecker([]modstore.UserPermissionRuleRow{
		{Scope: modstore.ScopePlatform, Pattern: "iam:users:list"},
	})
	ctx := context.Background()

	// First call: should load from store
	_, err := checker.CheckPermission(ctx, 1, "iam:users:list", modstore.ScopePlatform, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loadCount.Load() != 1 {
		t.Fatalf("expected 1 load, got %d", loadCount.Load())
	}

	// Second call: should hit cache, no additional load
	_, err = checker.CheckPermission(ctx, 1, "iam:users:list", modstore.ScopePlatform, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loadCount.Load() != 1 {
		t.Fatalf("expected 1 load (cached), got %d", loadCount.Load())
	}

	// Invalidate the user's cache
	checker.InvalidateUser(1)

	// Third call: cache was invalidated, should reload from store
	_, err = checker.CheckPermission(ctx, 1, "iam:users:list", modstore.ScopePlatform, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loadCount.Load() != 2 {
		t.Fatalf("expected 2 loads after invalidation, got %d", loadCount.Load())
	}

	// Invalidating a different user should not affect user 1's cache
	checker.InvalidateUser(999)
	_, err = checker.CheckPermission(ctx, 1, "iam:users:list", modstore.ScopePlatform, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loadCount.Load() != 2 {
		t.Fatalf("expected 2 loads (user 1 still cached), got %d", loadCount.Load())
	}
}
