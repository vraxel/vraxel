package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

func (s *pgRoleBindingStore) AddWorkspaceMember(ctx context.Context, userID, workspaceID int64, roleID int64) error {
	wsIDPtr := &workspaceID
	bindRoleID := roleID
	if bindRoleID == 0 {
		viewerRole, err := s.Q().GetRoleByNameAndWorkspace(ctx, generated.GetRoleByNameAndWorkspaceParams{
			Name:        RoleWorkspaceViewer,
			WorkspaceID: wsIDPtr,
		})
		if err != nil {
			return fmt.Errorf("get workspace-viewer role: %w", err)
		}
		bindRoleID = viewerRole.ID
	}

	if err := s.DB.WithTx(ctx, func(ctx context.Context, qtx *generated.Queries) error {
		return qtx.AddWorkspaceMemberRole(ctx, generated.AddWorkspaceMemberRoleParams{
			UserID:      userID,
			RoleID:      bindRoleID,
			WorkspaceID: wsIDPtr,
		})
	}); err != nil {
		return fmt.Errorf("add workspace member role: %w", err)
	}
	s.notifyUserChange(ctx, userID)
	return nil
}

func (s *pgRoleBindingStore) AddNamespaceMember(ctx context.Context, userID, namespaceID int64, roleID int64) error {
	wsID, err := s.Q().GetNamespaceWorkspaceID(ctx, namespaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("namespace %d: %w", namespaceID, pgerrors.ErrNotFound)
		}
		return fmt.Errorf("get namespace workspace: %w", err)
	}

	wsIDPtr := &wsID
	nsIDPtr := &namespaceID

	wsMemberRole, err := s.Q().GetRoleByNameAndWorkspace(ctx, generated.GetRoleByNameAndWorkspaceParams{
		Name:        RoleWorkspaceMember,
		WorkspaceID: wsIDPtr,
	})
	if err != nil {
		return fmt.Errorf("get workspace-member role: %w", err)
	}

	bindRoleID := roleID
	if bindRoleID == 0 {
		nsViewerRole, err := s.Q().GetRoleByNameAndNamespace(ctx, generated.GetRoleByNameAndNamespaceParams{
			Name:        RoleNamespaceViewer,
			NamespaceID: nsIDPtr,
		})
		if err != nil {
			return fmt.Errorf("get namespace-viewer role: %w", err)
		}
		bindRoleID = nsViewerRole.ID
	}

	if err := s.DB.WithTx(ctx, func(ctx context.Context, qtx *generated.Queries) error {
		if err := qtx.CreateRoleBindingIfNotExists(ctx, generated.CreateRoleBindingIfNotExistsParams{
			UserID:      userID,
			RoleID:      wsMemberRole.ID,
			Scope:       ScopeWorkspace,
			WorkspaceID: wsIDPtr,
		}); err != nil {
			return fmt.Errorf("auto-add workspace member: %w", err)
		}
		return qtx.AddNamespaceMemberRole(ctx, generated.AddNamespaceMemberRoleParams{
			UserID:      userID,
			RoleID:      bindRoleID,
			WorkspaceID: wsIDPtr,
			NamespaceID: nsIDPtr,
		})
	}); err != nil {
		return fmt.Errorf("add namespace member: %w", err)
	}
	s.notifyUserChange(ctx, userID)
	return nil
}

func (s *pgRoleBindingStore) RemoveWorkspaceMember(ctx context.Context, userID, workspaceID int64) error {
	wsID := &workspaceID
	n, err := s.Q().DeleteNonOwnerWorkspaceBindings(ctx, generated.DeleteNonOwnerWorkspaceBindingsParams{
		UserID:      userID,
		WorkspaceID: wsID,
	})
	if err != nil {
		return fmt.Errorf("remove workspace member: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cannot remove workspace owner: %w", pgerrors.ErrBadRequest)
	}
	s.notifyUserChange(ctx, userID)
	return nil
}

func (s *pgRoleBindingStore) RemoveNamespaceMember(ctx context.Context, userID, namespaceID int64) error {
	nsID := &namespaceID
	n, err := s.Q().DeleteNonOwnerNamespaceBindings(ctx, generated.DeleteNonOwnerNamespaceBindingsParams{
		UserID:      userID,
		NamespaceID: nsID,
	})
	if err != nil {
		return fmt.Errorf("remove namespace member: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("cannot remove namespace owner: %w", pgerrors.ErrBadRequest)
	}
	s.notifyUserChange(ctx, userID)
	return nil
}

func (s *pgRoleBindingStore) ListWorkspaceMembers(ctx context.Context, workspaceID int64, q list.Query) (*list.Result[UserWithRoleRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	wsID := &workspaceID

	countParams := generated.CountWorkspaceMembersParams{
		WorkspaceID: wsID,
		Status:      list.FilterStr(q.Filters, "status"),
		Search:      list.FilterStr(q.Filters, "search"),
	}

	count, err := s.Q().CountWorkspaceMembers(ctx, countParams)
	if err != nil {
		return nil, fmt.Errorf("count workspace members: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListWorkspaceMembers(ctx, generated.ListWorkspaceMembersParams{
		WorkspaceID: wsID,
		Status:      countParams.Status,
		Search:      countParams.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list workspace members: %w", err)
	}

	items := make([]UserWithRoleRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, UserWithRoleRow{
			UserRow: UserRow{
				ID:          r.ID,
				Username:    r.Username,
				Email:       r.Email,
				DisplayName: r.DisplayName,
				Phone:       r.Phone,
				AvatarURL:   r.AvatarUrl,
				Status:      r.Status,
				CreatedAt:   r.CreatedAt,
				UpdatedAt:   r.UpdatedAt,
			},
			Roles:    r.RoleNames,
			JoinedAt: r.JoinedAt,
		})
	}

	return &list.Result[UserWithRoleRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgRoleBindingStore) ListNamespaceMembers(ctx context.Context, namespaceID int64, q list.Query) (*list.Result[UserWithRoleRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	nsID := &namespaceID

	countParams := generated.CountNamespaceMembersParams{
		NamespaceID: nsID,
		Status:      list.FilterStr(q.Filters, "status"),
		Search:      list.FilterStr(q.Filters, "search"),
	}

	count, err := s.Q().CountNamespaceMembers(ctx, countParams)
	if err != nil {
		return nil, fmt.Errorf("count namespace members: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListNamespaceMembers(ctx, generated.ListNamespaceMembersParams{
		NamespaceID: nsID,
		Status:      countParams.Status,
		Search:      countParams.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list namespace members: %w", err)
	}

	items := make([]UserWithRoleRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, UserWithRoleRow{
			UserRow: UserRow{
				ID:          r.ID,
				Username:    r.Username,
				Email:       r.Email,
				DisplayName: r.DisplayName,
				Phone:       r.Phone,
				AvatarURL:   r.AvatarUrl,
				Status:      r.Status,
				CreatedAt:   r.CreatedAt,
				UpdatedAt:   r.UpdatedAt,
			},
			Roles:    r.RoleNames,
			JoinedAt: r.JoinedAt,
		})
	}

	return &list.Result[UserWithRoleRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgRoleBindingStore) ListWorkspaceNonMembers(ctx context.Context, workspaceID int64, q list.Query) (*list.Result[UserRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	wsID := &workspaceID

	countParams := generated.CountWorkspaceNonMembersParams{
		WorkspaceID: wsID,
		Status:      list.FilterStr(q.Filters, "status"),
		Search:      list.FilterStr(q.Filters, "search"),
	}

	count, err := s.Q().CountWorkspaceNonMembers(ctx, countParams)
	if err != nil {
		return nil, fmt.Errorf("count workspace non-members: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListWorkspaceNonMembers(ctx, generated.ListWorkspaceNonMembersParams{
		WorkspaceID: wsID,
		Status:      countParams.Status,
		Search:      countParams.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list workspace non-members: %w", err)
	}

	items := make([]UserRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, UserRow{
			ID:          r.ID,
			Username:    r.Username,
			Email:       r.Email,
			DisplayName: r.DisplayName,
			Phone:       r.Phone,
			AvatarURL:   r.AvatarUrl,
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		})
	}

	return &list.Result[UserRow]{Items: items, TotalCount: count}, nil
}

func (s *pgRoleBindingStore) ListNamespaceNonMembers(ctx context.Context, namespaceID int64, q list.Query) (*list.Result[UserRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	nsID := &namespaceID

	countParams := generated.CountNamespaceNonMembersParams{
		NamespaceID: nsID,
		Status:      list.FilterStr(q.Filters, "status"),
		Search:      list.FilterStr(q.Filters, "search"),
	}

	count, err := s.Q().CountNamespaceNonMembers(ctx, countParams)
	if err != nil {
		return nil, fmt.Errorf("count namespace non-members: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListNamespaceNonMembers(ctx, generated.ListNamespaceNonMembersParams{
		NamespaceID: nsID,
		Status:      countParams.Status,
		Search:      countParams.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list namespace non-members: %w", err)
	}

	items := make([]UserRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, UserRow{
			ID:          r.ID,
			Username:    r.Username,
			Email:       r.Email,
			DisplayName: r.DisplayName,
			Phone:       r.Phone,
			AvatarURL:   r.AvatarUrl,
			Status:      r.Status,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		})
	}

	return &list.Result[UserRow]{Items: items, TotalCount: count}, nil
}
