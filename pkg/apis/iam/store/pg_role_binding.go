package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"vraxel.io/vraxel/lib/list"
	"vraxel.io/vraxel/lib/pgnotify"
	"vraxel.io/vraxel/pkg/db"
	"vraxel.io/vraxel/pkg/db/generated"
	"vraxel.io/vraxel/pkg/db/pgerrors"
)

// RBACInvalidateChannel broadcasts RBAC cache invalidation across
// vraxel-server instances. Payload is the affected user_id as JSON
// (which for int64 is just the digits, byte-compatible with the
// pre-pgnotify strconv format).
var RBACInvalidateChannel = pgnotify.NewChannel[int64]("rbac_invalidate")

type pgRoleBindingStore struct {
	db.Store
}

// NewPGRoleBindingStore creates a new PostgreSQL-backed RoleBindingStore.
func NewPGRoleBindingStore(d *db.DB) RoleBindingStore {
	return &pgRoleBindingStore{Store: db.Store{DB: d}}
}

// notifyUserChange fires a pg_notify so all vraxel-server instances (including
// this one) invalidate their RBAC cache entry for userID. Errors are logged
// inside RBACInvalidateChannel.Publish.
func (s *pgRoleBindingStore) notifyUserChange(ctx context.Context, userID int64) {
	RBACInvalidateChannel.Publish(ctx, s.DB.GetPool(), userID)
}

func roleBindingFromRaw(
	id, userID, roleID int64, scope string,
	workspaceID, namespaceID *int64, isOwner bool, createdAt time.Time,
) RoleBindingRow {
	return RoleBindingRow{
		ID:          id,
		UserID:      userID,
		RoleID:      roleID,
		Scope:       scope,
		WorkspaceID: workspaceID,
		NamespaceID: namespaceID,
		IsOwner:     isOwner,
		CreatedAt:   createdAt,
	}
}

func roleBindingFromBase(r generated.RoleBinding) RoleBindingRow {
	return roleBindingFromRaw(r.ID, r.UserID, r.RoleID, r.Scope, r.WorkspaceID, r.NamespaceID, r.IsOwner, r.CreatedAt)
}

func (s *pgRoleBindingStore) Create(ctx context.Context, in RoleBindingCreateInput) (*RoleBindingRow, error) {
	row, err := s.Q().CreateRoleBinding(ctx, generated.CreateRoleBindingParams{
		UserID:      in.UserID,
		RoleID:      in.RoleID,
		Scope:       in.Scope,
		WorkspaceID: in.WorkspaceID,
		NamespaceID: in.NamespaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("create role binding: %w", pgerrors.CheckPG(err))
	}
	s.notifyUserChange(ctx, in.UserID)
	out := roleBindingFromBase(row)
	return &out, nil
}

func (s *pgRoleBindingStore) CreateMany(ctx context.Context, inputs []RoleBindingCreateInput) (int, error) {
	if len(inputs) == 0 {
		return 0, nil
	}
	// The insert itself reports whether it created a row (ON CONFLICT DO
	// NOTHING affects 0 rows when the binding already exists), so the
	// "newly granted" count needs no separate pre-count -- which would
	// also be wrong under a concurrent grant of the same pair.
	created := 0
	if err := s.DB.WithTx(ctx, func(ctx context.Context, qtx *generated.Queries) error {
		created = 0
		for _, in := range inputs {
			n, err := qtx.CreateRoleBindingIfNotExists(ctx, generated.CreateRoleBindingIfNotExistsParams{
				UserID:      in.UserID,
				RoleID:      in.RoleID,
				Scope:       in.Scope,
				WorkspaceID: in.WorkspaceID,
				NamespaceID: in.NamespaceID,
			})
			if err != nil {
				return err
			}
			created += int(n)
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("create role bindings: %w", pgerrors.CheckPG(err))
	}
	// Invalidate after commit: a notification for a rolled-back grant
	// would make every instance re-read the old rules for nothing.
	for _, in := range inputs {
		s.notifyUserChange(ctx, in.UserID)
	}
	return created, nil
}

func (s *pgRoleBindingStore) Delete(ctx context.Context, id int64) error {
	rb, err := s.Q().GetRoleBindingByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("roleBinding %d: %w", id, pgerrors.ErrNotFound)
		}
		return fmt.Errorf("get role binding for delete: %w", err)
	}
	rowsAffected, err := s.Q().DeleteRoleBinding(ctx, id)
	if err != nil {
		return fmt.Errorf("delete role binding: %w", pgerrors.CheckPG(err))
	}
	if rowsAffected == 0 {
		return fmt.Errorf("rolebinding %d: %w", id, pgerrors.ErrNotFound)
	}
	s.notifyUserChange(ctx, rb.UserID)
	return nil
}

func (s *pgRoleBindingStore) DeleteByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	rows, err := s.Q().DeleteRoleBindingsByIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("delete role bindings by ids: %w", pgerrors.CheckPG(err))
	}
	// Notify each unique affected user (RBAC cache reload).
	seen := make(map[int64]struct{}, len(rows))
	for _, r := range rows {
		if _, ok := seen[r.UserID]; ok {
			continue
		}
		seen[r.UserID] = struct{}{}
		s.notifyUserChange(ctx, r.UserID)
	}
	return int64(len(rows)), nil
}

func (s *pgRoleBindingStore) GetByID(ctx context.Context, id int64) (*RoleBindingRow, error) {
	row, err := s.Q().GetRoleBindingByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("roleBinding %d: %w", id, pgerrors.ErrNotFound)
		}
		return nil, fmt.Errorf("get role binding by id: %w", err)
	}
	out := roleBindingFromBase(row)
	return &out, nil
}

type roleBindingFilters struct {
	Scope       *string `filter:"scope"`
	RoleID      *int64  `filter:"role_id"`
	IsOwner     *bool   `filter:"is_owner"`
	Status      *string `filter:"status"`
	Visibility  *string `filter:"visibility"`
	WorkspaceID *int64  `filter:"workspace_id"`
	NamespaceID *int64  `filter:"namespace_id"`
	Search      *string `filter:"search"`
}

func (s *pgRoleBindingStore) ListPlatform(ctx context.Context, q list.Query) (*list.Result[RoleBindingWithDetailsRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	f := list.Parse[roleBindingFilters](q.Filters)

	count, err := s.Q().CountRoleBindingsPlatform(ctx, generated.CountRoleBindingsPlatformParams{
		RoleID:  f.RoleID,
		IsOwner: f.IsOwner,
		Search:  f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count platform role bindings: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListRoleBindingsPlatform(ctx, generated.ListRoleBindingsPlatformParams{
		RoleID:     f.RoleID,
		IsOwner:    f.IsOwner,
		Search:     f.Search,
		SortField:  q.SortBy,
		SortOrder:  sortOrder,
		PageOffset: offset,
		PageSize:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list platform role bindings: %w", err)
	}

	items := make([]RoleBindingWithDetailsRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, RoleBindingWithDetailsRow{
			RoleBindingRow:  roleBindingFromRaw(r.ID, r.UserID, r.RoleID, r.Scope, r.WorkspaceID, r.NamespaceID, r.IsOwner, r.CreatedAt),
			Username:        r.Username,
			UserDisplayName: r.UserDisplayName,
			RoleName:        r.RoleName,
			RoleDisplayName: r.RoleDisplayName,
		})
	}

	return &list.Result[RoleBindingWithDetailsRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgRoleBindingStore) ListByWorkspaceID(ctx context.Context, workspaceID int64, q list.Query) (*list.Result[RoleBindingWithDetailsRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	wsID := &workspaceID
	f := list.Parse[roleBindingFilters](q.Filters)

	count, err := s.Q().CountRoleBindingsByWorkspaceID(ctx, generated.CountRoleBindingsByWorkspaceIDParams{
		WorkspaceID: wsID,
		RoleID:      f.RoleID,
		IsOwner:     f.IsOwner,
		Search:      f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count workspace role bindings: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListRoleBindingsByWorkspaceID(ctx, generated.ListRoleBindingsByWorkspaceIDParams{
		WorkspaceID: wsID,
		RoleID:      f.RoleID,
		IsOwner:     f.IsOwner,
		Search:      f.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list workspace role bindings: %w", err)
	}

	items := make([]RoleBindingWithDetailsRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, RoleBindingWithDetailsRow{
			RoleBindingRow:  roleBindingFromRaw(r.ID, r.UserID, r.RoleID, r.Scope, r.WorkspaceID, r.NamespaceID, r.IsOwner, r.CreatedAt),
			Username:        r.Username,
			UserDisplayName: r.UserDisplayName,
			RoleName:        r.RoleName,
			RoleDisplayName: r.RoleDisplayName,
		})
	}

	return &list.Result[RoleBindingWithDetailsRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgRoleBindingStore) ListByNamespaceID(ctx context.Context, namespaceID int64, q list.Query) (*list.Result[RoleBindingWithDetailsRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	nsID := &namespaceID
	f := list.Parse[roleBindingFilters](q.Filters)

	count, err := s.Q().CountRoleBindingsByNamespaceID(ctx, generated.CountRoleBindingsByNamespaceIDParams{
		NamespaceID: nsID,
		RoleID:      f.RoleID,
		IsOwner:     f.IsOwner,
		Search:      f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count namespace role bindings: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListRoleBindingsByNamespaceID(ctx, generated.ListRoleBindingsByNamespaceIDParams{
		NamespaceID: nsID,
		RoleID:      f.RoleID,
		IsOwner:     f.IsOwner,
		Search:      f.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list namespace role bindings: %w", err)
	}

	items := make([]RoleBindingWithDetailsRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, RoleBindingWithDetailsRow{
			RoleBindingRow:  roleBindingFromRaw(r.ID, r.UserID, r.RoleID, r.Scope, r.WorkspaceID, r.NamespaceID, r.IsOwner, r.CreatedAt),
			Username:        r.Username,
			UserDisplayName: r.UserDisplayName,
			RoleName:        r.RoleName,
			RoleDisplayName: r.RoleDisplayName,
		})
	}

	return &list.Result[RoleBindingWithDetailsRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgRoleBindingStore) ListByUserID(ctx context.Context, userID int64, q list.Query) (*list.Result[RoleBindingWithDetailsRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	f := list.Parse[roleBindingFilters](q.Filters)

	count, err := s.Q().CountRoleBindingsByUserID(ctx, generated.CountRoleBindingsByUserIDParams{
		UserID:      userID,
		Scope:       f.Scope,
		RoleID:      f.RoleID,
		WorkspaceID: f.WorkspaceID,
		NamespaceID: f.NamespaceID,
		Search:      f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count user role bindings: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListRoleBindingsByUserID(ctx, generated.ListRoleBindingsByUserIDParams{
		UserID:      userID,
		Scope:       f.Scope,
		RoleID:      f.RoleID,
		WorkspaceID: f.WorkspaceID,
		NamespaceID: f.NamespaceID,
		Search:      f.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list user role bindings: %w", err)
	}

	items := make([]RoleBindingWithDetailsRow, 0, len(rows))
	for _, r := range rows {
		item := RoleBindingWithDetailsRow{
			RoleBindingRow:  roleBindingFromRaw(r.ID, r.UserID, r.RoleID, r.Scope, r.WorkspaceID, r.NamespaceID, r.IsOwner, r.CreatedAt),
			RoleName:        r.RoleName,
			RoleDisplayName: r.RoleDisplayName,
		}
		if r.WorkspaceName != nil {
			item.WorkspaceName = *r.WorkspaceName
		}
		if r.NamespaceName != nil {
			item.NamespaceName = *r.NamespaceName
		}
		items = append(items, item)
	}

	return &list.Result[RoleBindingWithDetailsRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgRoleBindingStore) CountByRoleAndScope(ctx context.Context, roleID int64, scope string) (int64, error) {
	count, err := s.Q().CountRoleBindingsByRoleAndScope(ctx, generated.CountRoleBindingsByRoleAndScopeParams{
		RoleID: roleID,
		Scope:  scope,
	})
	if err != nil {
		return 0, fmt.Errorf("count role bindings by role and scope: %w", err)
	}
	return count, nil
}

func (s *pgRoleBindingStore) ListUserWorkspaces(ctx context.Context, userID int64, q list.Query) (*list.Result[WorkspaceWithOwnerAndRoleRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	f := list.Parse[roleBindingFilters](q.Filters)

	count, err := s.Q().CountUserWorkspaces(ctx, generated.CountUserWorkspacesParams{
		UserID: userID,
		Status: f.Status,
		Search: f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count user workspaces: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListUserWorkspaces(ctx, generated.ListUserWorkspacesParams{
		UserID:     userID,
		Status:     f.Status,
		Search:     f.Search,
		SortField:  q.SortBy,
		SortOrder:  sortOrder,
		PageOffset: offset,
		PageSize:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list user workspaces: %w", err)
	}

	items := make([]WorkspaceWithOwnerAndRoleRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, WorkspaceWithOwnerAndRoleRow{
			WorkspaceRow: WorkspaceRow{
				ID:          r.ID,
				Name:        r.Name,
				DisplayName: r.DisplayName,
				Description: r.Description,
				OwnerID:     r.OwnerID,
				Status:      r.Status,
				CreatedAt:   r.CreatedAt,
				UpdatedAt:   r.UpdatedAt,
			},
			OwnerUsername:    r.OwnerUsername,
			NamespaceCount:   r.NamespaceCount,
			MemberCount:      r.MemberCount,
			Roles:            r.RoleNames,
			RoleDisplayNames: r.RoleDisplayNames,
			JoinedAt:         r.JoinedAt,
		})
	}

	return &list.Result[WorkspaceWithOwnerAndRoleRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}

func (s *pgRoleBindingStore) ListUserNamespaces(ctx context.Context, userID int64, q list.Query) (*list.Result[NamespaceWithOwnerAndRoleRow], error) {
	offset, limit := list.PaginationToOffsetLimit(q.Pagination)
	f := list.Parse[roleBindingFilters](q.Filters)

	count, err := s.Q().CountUserNamespaces(ctx, generated.CountUserNamespacesParams{
		UserID:      userID,
		Status:      f.Status,
		Visibility:  f.Visibility,
		WorkspaceID: f.WorkspaceID,
		Search:      f.Search,
	})
	if err != nil {
		return nil, fmt.Errorf("count user namespaces: %w", err)
	}

	sortOrder := q.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	rows, err := s.Q().ListUserNamespaces(ctx, generated.ListUserNamespacesParams{
		UserID:      userID,
		Status:      f.Status,
		Visibility:  f.Visibility,
		WorkspaceID: f.WorkspaceID,
		Search:      f.Search,
		SortField:   q.SortBy,
		SortOrder:   sortOrder,
		PageOffset:  offset,
		PageSize:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list user namespaces: %w", err)
	}

	items := make([]NamespaceWithOwnerAndRoleRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, NamespaceWithOwnerAndRoleRow{
			NamespaceRow: NamespaceRow{
				ID:          r.ID,
				Name:        r.Name,
				DisplayName: r.DisplayName,
				Description: r.Description,
				WorkspaceID: r.WorkspaceID,
				OwnerID:     r.OwnerID,
				Visibility:  r.Visibility,
				MaxMembers:  r.MaxMembers,
				Status:      r.Status,
				CreatedAt:   r.CreatedAt,
				UpdatedAt:   r.UpdatedAt,
			},
			OwnerUsername:    r.OwnerUsername,
			WorkspaceName:    r.WorkspaceName,
			MemberCount:      r.MemberCount,
			Roles:            r.RoleNames,
			RoleDisplayNames: r.RoleDisplayNames,
			JoinedAt:         r.JoinedAt,
		})
	}

	return &list.Result[NamespaceWithOwnerAndRoleRow]{
		Items:      items,
		TotalCount: count,
	}, nil
}
