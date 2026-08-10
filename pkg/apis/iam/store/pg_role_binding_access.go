package store

import (
	"context"
	"fmt"
)

func (s *pgRoleBindingStore) GetAccessibleWorkspaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	ptrs, err := s.Q().GetAccessibleWorkspaceIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get accessible workspace ids: %w", err)
	}
	ids := make([]int64, 0, len(ptrs))
	for _, p := range ptrs {
		if p != nil {
			ids = append(ids, *p)
		}
	}
	return ids, nil
}

func (s *pgRoleBindingStore) GetAccessibleNamespaceIDs(ctx context.Context, userID int64) ([]int64, error) {
	ptrs, err := s.Q().GetAccessibleNamespaceIDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get accessible namespace ids: %w", err)
	}
	ids := make([]int64, 0, len(ptrs))
	for _, p := range ptrs {
		if p != nil {
			ids = append(ids, *p)
		}
	}
	return ids, nil
}

func (s *pgRoleBindingStore) GetUserIDsByWorkspaceID(ctx context.Context, workspaceID int64) ([]int64, error) {
	wsID := &workspaceID
	ids, err := s.Q().GetUserIDsByWorkspaceID(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("get user ids by workspace: %w", err)
	}
	return ids, nil
}

func (s *pgRoleBindingStore) GetUserIDsByNamespaceID(ctx context.Context, namespaceID int64) ([]int64, error) {
	nsID := &namespaceID
	ids, err := s.Q().GetUserIDsByNamespaceID(ctx, nsID)
	if err != nil {
		return nil, fmt.Errorf("get user ids by namespace: %w", err)
	}
	return ids, nil
}

func (s *pgRoleBindingStore) LoadUserPermissionRules(ctx context.Context, userID int64) ([]UserPermissionRuleRow, error) {
	rows, err := s.Q().LoadUserPermissionRules(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load user permission rules: %w", err)
	}
	items := make([]UserPermissionRuleRow, len(rows))
	for i, r := range rows {
		items[i] = UserPermissionRuleRow{
			Scope:       r.Scope,
			WorkspaceID: r.WorkspaceID,
			NamespaceID: r.NamespaceID,
			Pattern:     r.Pattern,
		}
	}
	return items, nil
}

func (s *pgRoleBindingStore) GetUserRoleBindingsWithRules(ctx context.Context, userID int64) ([]UserRoleBindingWithRules, error) {
	rows, err := s.Q().GetUserRoleBindingsWithRules(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user role bindings with rules: %w", err)
	}
	items := make([]UserRoleBindingWithRules, len(rows))
	for i, r := range rows {
		items[i] = UserRoleBindingWithRules{
			Scope:       r.Scope,
			WorkspaceID: r.WorkspaceID,
			NamespaceID: r.NamespaceID,
			RoleName:    r.RoleName,
			Pattern:     r.Pattern,
		}
	}
	return items, nil
}
